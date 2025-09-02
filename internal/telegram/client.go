package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"traiding/internal/config"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	// "github.com/gotd/td/tg/tgerr" // Комментируем импорт для tgerr.FloodWaitError
)

// Reader defines the interface for reading messages from Telegram channels.
type Reader interface {
	GetLastMessages(ctx context.Context, channel string, limit int) ([]Message, error)
	GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time) ([]Message, error)
	GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time) ([]Message, error)
	SubscribeToChannel(ctx context.Context, channel string) (<-chan Message, error)
	Close() error
}

type Client struct {
	client *telegram.Client
	config config.TelegramConfig

	// Карта для отслеживания открытых каналов подписки
	subscriptions sync.Map
	// Карта для хранения channel ID по имени канала
	channelIDs sync.Map

	// Контекст для остановки клиента
	cancel context.CancelFunc

	ready chan struct{} // Channel to signal when client is authenticated and ready
}

type Message struct {
	ID        int64
	Text      string
	Date      time.Time
	Channel   string
	Username  string
	IsForward bool
	Type      string
}

func NewClient(cfg config.TelegramConfig) (*Client, error) {
	// Проверяем наличие необходимых данных для MTProto
	if cfg.APIID == "" || cfg.APIHash == "" {
		return nil, fmt.Errorf("API ID and API Hash are required for MTProto client")
	}

	apiID, err := strconv.Atoi(cfg.APIID)
	if err != nil {
		return nil, fmt.Errorf("invalid API ID: %w", err)
	}

	log.Printf("Создание клиента с API ID: %d", apiID)

	c := &Client{
		config:        cfg,
		subscriptions: sync.Map{},
		channelIDs:    sync.Map{},
		ready:         make(chan struct{}), // Initialize the ready channel
	}

	// Создаем MTProto клиент с обработчиком обновлений
	client := telegram.NewClient(apiID, cfg.APIHash, telegram.Options{
		UpdateHandler: telegram.UpdateHandlerFunc(c.handleUpdate),
	})
	c.client = client

	return c, nil
}

// Start запускает Telegram клиента.
func (c *Client) Start(ctx context.Context) error {
	log.Println("Запуск Telegram клиента...")

	// Создаем дочерний контекст для возможности остановки клиента
	clientCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	err := c.client.Run(clientCtx, func(ctx context.Context) error {
		// Авторизация
		if err := c.auth(ctx); err != nil {
			log.Printf("Ошибка авторизации для фонового клиента: %v", err)
			return fmt.Errorf("auth failed for background client: %w", err)
		}
		log.Println("Авторизация успешна для фонового клиента.")
		close(c.ready) // Signal that the client is ready after authentication
		<-ctx.Done()   // Block until the clientCtx is cancelled
		return nil
	})

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("ошибка при работе фонового клиента Telegram: %w", err)
	}
	log.Println("Фоновый клиент Telegram остановлен.")
	return nil
}

// GetLastMessages получает последние сообщения из канала
func (c *Client) GetLastMessages(ctx context.Context, channel string, limit int) ([]Message, error) {
	// Wait for client to be ready
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ready: // Block until the client is authenticated and ready
		// Client is ready, proceed
	}

	var messages []Message

	channelName := strings.TrimPrefix(channel, "@")

	log.Printf("Начинаем обработку канала: %s", channelName)

	// Получаем информацию о канале и peer
	channelPeer, err := c.resolveChannelPeer(ctx, channelName)
	if err != nil {
		return nil, err
	}

	log.Printf("Отправляем запрос на получение истории сообщений...")

	request := &tg.MessagesGetHistoryRequest{
		Peer:  channelPeer,
		Limit: limit,
	}

	// Получаем сообщения из канала
	historyResult, err := c.client.API().MessagesGetHistory(ctx, request)
	if err != nil {
		log.Printf("Ошибка при получении истории сообщений: %v", err)
		return nil, fmt.Errorf("failed to get history for %s: %w", channelName, err)
	}
	log.Printf("История сообщений успешно получена.")

	// Обрабатываем полученные сообщения
	switch result := historyResult.(type) {
	case *tg.MessagesMessages:
		log.Printf("Получено %d сообщений.", len(result.Messages))
		for _, msg := range result.Messages {
			if len(messages) >= limit {
				break
			}
			message, err := c.parseMessage(msg, channelName)
			if err != nil {
				log.Printf("Ошибка при парсинге сообщения: %v", err)
				continue
			}
			messages = append(messages, message)
		}
	case *tg.MessagesChannelMessages:
		log.Printf("Получено %d сообщений.", len(result.Messages))
		for _, msg := range result.Messages {
			if len(messages) >= limit {
				break
			}
			message, err := c.parseMessage(msg, channelName)
			if err != nil {
				log.Printf("Ошибка при парсинге сообщения: %v", err)
				continue
			}
			messages = append(messages, message)
		}
	}

	return messages, nil
}

// resolveChannelPeer получает tg.InputPeerClass для заданного имени канала.
func (c *Client) resolveChannelPeer(ctx context.Context, channelName string) (tg.InputPeerClass, error) {
	api := c.client.API()
	log.Printf("Отправляем запрос на разрешение имени пользователя %s...", channelName)
	resolveResult, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: channelName,
	})
	if err != nil {
		log.Printf("Ошибка при разрешении имени пользователя: %v", err)
		return nil, fmt.Errorf("failed to resolve username %s: %w", channelName, err)
	}
	log.Printf("Имя пользователя %s успешно разрешено.", channelName)

	var channelPeer tg.InputPeerClass
	switch p := resolveResult.Peer.(type) {
	case *tg.PeerChannel:
		var channel tg.Channel
		for _, chat := range resolveResult.Chats {
			ch, ok := chat.(*tg.Channel)
			if ok && ch.ID == p.ChannelID {
				channel = *ch
				break
			}
		}
		channelPeer = &tg.InputPeerChannel{
			ChannelID:  p.ChannelID,
			AccessHash: channel.AccessHash,
		}
		c.channelIDs.Store(channelName, p.ChannelID) // Сохраняем ChannelID
	case *tg.PeerChat:
		channelPeer = &tg.InputPeerChat{
			ChatID: p.ChatID,
		}
		c.channelIDs.Store(channelName, p.ChatID) // Сохраняем ChatID
	default:
		log.Printf("Неожиданный тип peer для канала: %T", p)
		return nil, fmt.Errorf("unexpected peer type for channel %s", channelName)
	}
	return channelPeer, nil
}

// GetMessagesFromDate получает сообщения из канала, начиная с указанной даты, до настоящего момента.
func (c *Client) GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time) ([]Message, error) {
	// Wait for client to be ready
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ready:
		// Client is ready, proceed
	}

	channelName := strings.TrimPrefix(channel, "@")
	log.Printf("Начинаем обработку канала %s с %s...", channelName, startDate.Format("2006-01-02"))

	channelPeer, err := c.resolveChannelPeer(ctx, channelName)
	if err != nil {
		return nil, err
	}

	var allMessages []Message
	const batchSize = 100 // Максимальное количество сообщений за один запрос

	// Объявляем currentBatchMessages и historyResult здесь, чтобы они были доступны во всей функции.
	// var currentBatchMessages []tg.MessageClass // Удаляем объявление здесь
	// var historyResult tg.MessagesMessagesClass // Удаляем объявление здесь

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		request := &tg.MessagesGetHistoryRequest{
			Peer:       channelPeer,
			Limit:      batchSize,
			OffsetDate: int(startDate.Unix()),
		}

		if len(allMessages) > 0 {
			// Используем OffsetID для пагинации
			request.OffsetID = int(allMessages[len(allMessages)-1].ID)
		}

		// Удаляем цикл повторных попыток для FloodWaitError
		historyResult, err := c.client.API().MessagesGetHistory(ctx, request)
		if err != nil {
			log.Printf("Ошибка при получении истории сообщений для %s: %v", channelName, err)
			return nil, fmt.Errorf("failed to get history for %s: %w", channelName, err)
		}

		var currentBatchMessages []tg.MessageClass
		switch result := historyResult.(type) {
		case *tg.MessagesMessages:
			currentBatchMessages = result.Messages
		case *tg.MessagesChannelMessages:
			currentBatchMessages = result.Messages
		default:
			return nil, fmt.Errorf("unexpected history result type: %T", result)
		}

		if len(currentBatchMessages) == 0 {
			break // Больше нет сообщений
		}

		for _, msg := range currentBatchMessages {
			parsedMsg, err := c.parseMessage(msg, channelName)
			if err != nil {
				log.Printf("Ошибка при парсинге сообщения: %v", err)
				continue
			}
			if parsedMsg.Date.Before(startDate) {
				// Мы получили сообщения старше startDate, значит, достигли начала диапазона.
				// Можно прекратить сбор или отфильтровать лишние.
				// Здесь мы прекращаем сбор, так как OffsetDate уже должен был отфильтровать.
				// Но эта проверка нужна для надежности, если API вернет сообщения вне OffsetDate.
				break
			}
			allMessages = append(allMessages, parsedMsg)
		}

		if len(currentBatchMessages) < batchSize {
			break // Получили меньше, чем запросили, значит, достигли конца истории
		}
	}
	// Telegram возвращает сообщения от новых к старым.
	// Если мы хотим от старых к новым, нужно реверсировать.
	reverseMessages(allMessages)
	return allMessages, nil
}

// GetMessagesInDateRange получает сообщения из канала в заданном диапазоне дат.
func (c *Client) GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time) ([]Message, error) {
	// Wait for client to be ready
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ready:
		// Client is ready, proceed
	}

	channelName := strings.TrimPrefix(channel, "@")
	log.Printf("Начинаем обработку канала %s с %s по %s...",
		channelName, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	channelPeer, err := c.resolveChannelPeer(ctx, channelName)
	if err != nil {
		return nil, err
	}

	var allMessages []Message
	const batchSize = 100

	// Объявляем currentBatchMessages и historyResult здесь, чтобы они были доступны во всей функции.
	// var currentBatchMessages []tg.MessageClass // Удаляем объявление здесь
	// var historyResult tg.MessagesMessagesClass // Удаляем объявление здесь

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		request := &tg.MessagesGetHistoryRequest{
			Peer:       channelPeer,
			Limit:      batchSize,
			OffsetDate: int(endDate.Unix()), // Начинаем с конца диапазона
		}

		if len(allMessages) > 0 {
			request.OffsetID = int(allMessages[len(allMessages)-1].ID)
		}

		// Удаляем цикл повторных попыток для FloodWaitError
		historyResult, err := c.client.API().MessagesGetHistory(ctx, request)
		if err != nil {
			log.Printf("Ошибка при получении истории сообщений для %s: %v", channelName, err)
			return nil, fmt.Errorf("failed to get history for %s: %w", channelName, err)
		}

		var currentBatchMessages []tg.MessageClass
		switch result := historyResult.(type) {
		case *tg.MessagesMessages:
			currentBatchMessages = result.Messages
		case *tg.MessagesChannelMessages:
			currentBatchMessages = result.Messages
		default:
			return nil, fmt.Errorf("unexpected history result type: %T", result)
		}

		if len(currentBatchMessages) == 0 {
			break // Больше нет сообщений
		}

		for _, msg := range currentBatchMessages {
			parsedMsg, err := c.parseMessage(msg, channelName)
			if err != nil {
				log.Printf("Ошибка при парсинге сообщения: %v", err)
				continue
			}
			// Отфильтровываем сообщения, которые выходят за пределы startDate
			if parsedMsg.Date.Before(startDate) {
				break // Достигли начала диапазона, прекращаем сбор
			}
			if parsedMsg.Date.After(endDate) {
				continue // Пропускаем сообщения, которые позже endDate (это может произойти из-за OffsetDate)
			}
			allMessages = append(allMessages, parsedMsg)
		}

		if len(currentBatchMessages) < batchSize {
			break // Получили меньше, чем запросили, значит, достигли конца истории
		}
	}
	// Telegram возвращает сообщения от новых к старым. Если мы хотим от старых к новым, нужно реверсировать.
	reverseMessages(allMessages)
	return allMessages, nil
}

// reverseMessages реверсирует порядок сообщений в слайсе.
func reverseMessages(messages []Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}

// extractMessages извлекает сообщения из результата MessagesGetHistory.
// Удаляем эту функцию полностью.
// func extractMessages(historyResult tg.MessagesMessagesClass) ([]tg.MessageClass, error) {
// 	switch result := historyResult.(type) {
// 	case *tg.MessagesMessages:
// 		return result.Messages, nil
// 	case *tg.MessagesChannelMessages:
// 		return result.Messages, nil
// 	default:
// 		return nil, fmt.Errorf("unexpected history result type: %T", result)
// 	}
// }

// SubscribeToChannel подписывается на новые сообщения из указанного канала.
// Возвращает канал Go, куда будут поступать новые сообщения.
func (c *Client) SubscribeToChannel(ctx context.Context, channelName string) (<-chan Message, error) {
	// Wait for client to be ready
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ready: // Block until the client is authenticated and ready
		// Client is ready, proceed
	}

	channelName = strings.TrimPrefix(channelName, "@")

	// Проверяем, не подписаны ли мы уже на этот канал
	if _, loaded := c.subscriptions.Load(channelName); loaded {
		return nil, fmt.Errorf("уже подписаны на канал: %s", channelName)
	}

	msgChan := make(chan Message)
	// Добавляем канал в карту подписок
	c.subscriptions.Store(channelName, msgChan)

	// Разрешаем имя канала и сохраняем ChannelID
	api := c.client.API()
	resolveResult, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: channelName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve username %s: %w", channelName, err)
	}

	switch p := resolveResult.Peer.(type) {
	case *tg.PeerChannel:
		c.channelIDs.Store(channelName, p.ChannelID)
	case *tg.PeerChat:
		c.channelIDs.Store(channelName, p.ChatID)
	default:
		return nil, fmt.Errorf("unexpected peer type for channel %s: %T", channelName, p)
	}

	return msgChan, nil
}

func (c *Client) handleUpdate(ctx context.Context, updates tg.UpdatesClass) error {
	switch u := updates.(type) {
	case *tg.Updates:
		for _, update := range u.Updates {
			switch upd := update.(type) {
			case *tg.UpdateNewMessage:
				if msg, ok := upd.Message.(*tg.Message); ok {
					peerChannel, isChannel := msg.PeerID.(*tg.PeerChannel)
					if !isChannel {
						continue
					}

					// Находим имя канала по ChannelID
					var targetChannelName string
					// Проходим по channelIDs, чтобы найти соответствующее имя канала
					c.channelIDs.Range(func(key, value interface{}) bool {
						if id, ok := value.(int64); ok && id == peerChannel.ChannelID {
							targetChannelName = key.(string)
							return false // Останавливаем итерацию, так как нашли совпадение
						}
						return true
					})

					if targetChannelName != "" {
						if ch, loaded := c.subscriptions.Load(targetChannelName); loaded {
							msgChan := ch.(chan Message)
							parsedMsg, err := c.parseMessage(msg, targetChannelName)
							if err != nil {
								log.Printf("Ошибка парсинга сообщения для подписки: %v", err)
								return nil
							}
							select {
							case msgChan <- parsedMsg:
							case <-ctx.Done():
								return ctx.Err()
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// auth выполняет авторизацию в Telegram
func (c *Client) auth(ctx context.Context) error {
	log.Println("Запускаем процесс авторизации...")
	flow := auth.NewFlow(
		TermAuth{},
		auth.SendCodeOptions{},
	)

	if err := c.client.Auth().IfNecessary(ctx, flow); err != nil {
		log.Printf("Ошибка в процессе авторизации: %v", err)
		return fmt.Errorf("auth flow failed: %w", err)
	}
	return nil
}

// parseMessage парсит сообщение из MTProto в нашу структуру
func (c *Client) parseMessage(msg tg.MessageClass, channelName string) (Message, error) {
	switch m := msg.(type) {
	case *tg.Message:
		// Проверка на пересланное сообщение
		isForward := m.FwdFrom.Date != 0
		return Message{
			ID:        int64(m.ID),
			Text:      m.Message,
			Date:      time.Unix(int64(m.Date), 0),
			Channel:   channelName,
			Username:  "",
			IsForward: isForward,
			Type:      getMessageType(m),
		}, nil
	case *tg.MessageService:
		return Message{
			ID:        int64(m.ID),
			Text:      "Service message",
			Date:      time.Unix(int64(m.Date), 0),
			Channel:   channelName,
			Username:  "",
			IsForward: false,
			Type:      "service",
		}, nil
	default:
		return Message{}, fmt.Errorf("unknown message type: %T", msg)
	}
}

// getMessageType определяет тип сообщения
func getMessageType(msg *tg.Message) string {
	if msg.Message != "" {
		return "text"
	}
	if msg.Media != nil {
		switch msg.Media.(type) {
		case *tg.MessageMediaPhoto:
			return "photo"
		case *tg.MessageMediaDocument:
			return "document"
		case *tg.MessageMediaGeo:
			return "geo"
		default:
			return "media"
		}
	}
	return "unknown"
}

// Close закрывает соединение с клиентом
func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// Ready returns a channel that is closed when the client is authenticated and ready.
func (c *Client) Ready() <-chan struct{} {
	return c.ready
}
