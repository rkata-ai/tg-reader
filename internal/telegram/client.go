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

	"rkata-ai/tg-reader/internal/config"
	"rkata-ai/tg-reader/internal/storage"

	"github.com/google/uuid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	// "github.com/gotd/td/tg/tgerr" // Удаляем импорт для tgerr.FloodWaitError
	"database/sql"
	"encoding/json"
)

// Reader defines the interface for reading messages from Telegram channels.
type Reader interface {
	GetLastMessages(ctx context.Context, channel string, limit int) error
	GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time) error
	GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time) error
	SubscribeToChannel(ctx context.Context, channel string) (<-chan storage.Message, error)
	Close() error
}

type Client struct {
	client  *telegram.Client
	config  config.TelegramConfig
	storage storage.Storage // Добавляем зависимость от хранилища

	// Карта для отслеживания открытых каналов подписки
	subscriptions sync.Map
	// Карта для хранения channel ID по имени канала
	channelIDs sync.Map

	// Контекст для остановки клиента
	cancel context.CancelFunc

	ready chan struct{} // Channel to signal when client is authenticated and ready
}

// Удаляем локальную структуру Message, используем storage.Message

func NewClient(cfg config.TelegramConfig, store storage.Storage) (*Client, error) {
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
		storage:       store, // Передаем хранилище
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

// GetLastMessages получает последние сообщения из канала для отладки, не сохраняя их в БД.
func (c *Client) GetLastMessages(ctx context.Context, channel string, limit int) ([]storage.Message, error) {
	// Wait for client to be ready
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ready: // Block until the client is authenticated and ready
		// Client is ready, proceed
	}

	channelName := strings.TrimPrefix(channel, "@")

	log.Printf("Режим отладки: Получение последних %d сообщений из канала %s (без сохранения в БД)...", limit, channelName)

	// Для GetLastMessages в режиме отладки нам не нужен dbChannel.ID для parseMessage, т.к. не сохраняем в БД. Создадим "пустой" UUID.
	dummyChannelID := uuid.Nil

	channelPeer, err := c.resolveChannelPeer(ctx, channelName)
	if err != nil {
		return nil, err
	}

	request := &tg.MessagesGetHistoryRequest{
		Peer:  channelPeer,
		Limit: limit,
	}

	historyResult, err := c.client.API().MessagesGetHistory(ctx, request)

	if err != nil {
		log.Printf("Ошибка при получении истории сообщений для отладки: %v", err)
		return nil, fmt.Errorf("failed to get history for %s: %w", channelName, err)
	}

	currentBatchTgMessages, err := extractMessages(historyResult)
	if err != nil {
		return nil, err
	}

	var fetchedMessages []storage.Message
	for _, msg := range currentBatchTgMessages {
		parsedMsg, err := c.parseMessage(msg, dummyChannelID, channelName)
		if err != nil {
			log.Printf("Ошибка при парсинге сообщения для отладки: %v", err)
			continue
		}
		fetchedMessages = append(fetchedMessages, parsedMsg)
	}

	// Возвращаем сообщения в хронологическом порядке (старые первыми)
	reverseMessages(fetchedMessages)

	log.Printf("Завершено получение %d сообщений из канала %s.", len(fetchedMessages), channelName)
	return fetchedMessages, nil
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
			if ch, ok := chat.(*tg.Channel); ok && ch.ID == p.ChannelID {
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

// GetMessagesFromDate получает сообщения из канала, начиная с указанной даты, до настоящего момента, и сохраняет их в БД.
func (c *Client) GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time) error {
	// Wait for client to be ready
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ready:
		// Client is ready, proceed
	}

	channelName := strings.TrimPrefix(channel, "@")
	log.Printf("Начинаем обработку канала %s с %s...", channelName, startDate.Format("2006-01-02"))

	// 1. Получаем или создаем канал в БД
	dbChannel, err := c.storage.GetChannelByUsername(ctx, channelName)
	if err != nil {
		return fmt.Errorf("failed to get channel from db: %w", err)
	}
	if dbChannel == nil {
		dbChannel = &storage.Channel{
			ID:        uuid.New(),
			Username:  channelName,
			Title:     sql.NullString{String: channelName, Valid: true},
			CreatedAt: time.Now(),
		}
		if err := c.storage.SaveChannel(ctx, dbChannel); err != nil {
			return fmt.Errorf("failed to save new channel to db: %w", err)
		}
	}

	channelPeer, err := c.resolveChannelPeer(ctx, channelName)
	if err != nil {
		return err
	}

	var fetchedMessages []storage.Message // Для временного хранения сообщений из пачки перед сохранением
	const batchSize = 100                 // Максимальное количество сообщений за один запрос

	// Переменные для пагинации
	var offsetID int
	var offsetDate int // Используем 0 для первого запроса, чтобы получить самые новые
	var totalSaved int

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		request := &tg.MessagesGetHistoryRequest{
			Peer:  channelPeer,
			Limit: batchSize,
		}

		if offsetID != 0 {
			request.OffsetID = offsetID
		}
		if offsetDate != 0 {
			request.OffsetDate = offsetDate
		}

		historyResult, err := c.client.API().MessagesGetHistory(ctx, request)

		if err != nil {
			// Проверяем на FloodWaitError с нашей собственной реализацией
			if wait, ok := isFloodWaitError(err); ok {
				log.Printf("Получена FLOOD_WAIT ошибка. Ожидание %v перед повторной попыткой...", wait)
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue // Повторяем запрос
			}
			log.Printf("Ошибка при получении истории сообщений для %s: %v", channelName, err)
			return fmt.Errorf("failed to get history for %s: %w", channelName, err)
		}

		currentBatchTgMessages, err := extractMessages(historyResult)
		if err != nil {
			return err
		}

		if len(currentBatchTgMessages) == 0 {
			break // Больше нет сообщений
		}

		fetchedMessages = fetchedMessages[:0] // Очищаем слайс для новой пачки
		var foundMessagesOlderThanStartDate bool
		for _, msg := range currentBatchTgMessages {
			dbMessage, err := c.parseMessage(msg, dbChannel.ID, channelName)
			if err != nil {
				log.Printf("Ошибка при парсинге сообщения: %v", err)
				continue
			}

			// Если сообщение старше startDate, мы дошли до начала нужного диапазона
			if dbMessage.SentAt.Before(startDate) {
				foundMessagesOlderThanStartDate = true
				break // Прекращаем обработку текущей пачки, т.к. остальные сообщения будут еще старше
			}
			fetchedMessages = append(fetchedMessages, dbMessage)
		}

		// Если есть сообщения для сохранения, сохраняем их
		for _, msgToSave := range fetchedMessages {
			if err := c.storage.SaveMessage(ctx, &msgToSave); err != nil {
				log.Printf("Ошибка при сохранении сообщения в БД (GetMessagesFromDate): %v", err)
				continue
			}
			totalSaved++
		}

		// Обновляем offsetID и offsetDate для следующего запроса
		// Используем данные самого старого сообщения в текущем пакете из оригинальных TG-сообщений
		oldestTgMsgInBatch := currentBatchTgMessages[len(currentBatchTgMessages)-1]
		parsedOldestMsg, err := c.parseMessage(oldestTgMsgInBatch, dbChannel.ID, channelName)
		if err != nil {
			return fmt.Errorf("failed to parse oldest message for pagination: %w", err)
		}
		offsetID = int(parsedOldestMsg.TelegramID)
		offsetDate = int(parsedOldestMsg.SentAt.Unix())

		// Если мы уже нашли сообщения старше startDate, прерываем основной цикл пагинации
		if foundMessagesOlderThanStartDate {
			break
		}

		// Добавляем небольшую задержку после каждого успешного запроса, чтобы избежать FLOOD_WAIT.
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	log.Printf("Успешно получено и сохранено %d сообщений из %s, начиная с %s.", totalSaved, channelName, startDate.Format("2006-01-02"))
	return nil
}

// GetMessagesInDateRange получает сообщения из канала в заданном диапазоне дат и сохраняет их в БД.
func (c *Client) GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time) error {
	// Wait for client to be ready
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ready:
		// Client is ready, proceed
	}

	channelName := strings.TrimPrefix(channel, "@")
	log.Printf("Начинаем обработку канала %s с %s по %s...",
		channelName, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// 1. Получаем или создаем канал в БД
	dbChannel, err := c.storage.GetChannelByUsername(ctx, channelName)
	if err != nil {
		return fmt.Errorf("failed to get channel from db: %w", err)
	}
	if dbChannel == nil {
		dbChannel = &storage.Channel{
			ID:        uuid.New(),
			Username:  channelName,
			Title:     sql.NullString{String: channelName, Valid: true},
			CreatedAt: time.Now(),
		}
		if err := c.storage.SaveChannel(ctx, dbChannel); err != nil {
			return fmt.Errorf("failed to save new channel to db: %w", err)
		}
	}

	channelPeer, err := c.resolveChannelPeer(ctx, channelName)
	if err != nil {
		return err
	}

	var fetchedMessages []storage.Message // Для временного хранения сообщений из пачки перед сохранением
	const batchSize = 100

	// Переменные для пагинации
	var offsetID int
	var offsetDate = int(endDate.Unix()) // Начинаем пагинацию назад с endDate
	var totalSaved int

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		request := &tg.MessagesGetHistoryRequest{
			Peer:  channelPeer,
			Limit: batchSize,
		}

		if offsetID != 0 {
			request.OffsetID = offsetID
		}
		request.OffsetDate = offsetDate // Всегда используем OffsetDate для этого режима

		historyResult, err := c.client.API().MessagesGetHistory(ctx, request)

		if err != nil {
			// Проверяем на FloodWaitError с нашей собственной реализацией
			if wait, ok := isFloodWaitError(err); ok {
				log.Printf("Получена FLOOD_WAIT ошибка. Ожидание %v перед повторной попыткой...", wait)
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return ctx.Err()
				}
				continue // Повторяем запрос
			}
			log.Printf("Ошибка при получении истории сообщений для %s: %v", channelName, err)
			return fmt.Errorf("failed to get history for %s: %w", channelName, err)
		}

		currentBatchTgMessages, err := extractMessages(historyResult)
		if err != nil {
			return err
		}

		if len(currentBatchTgMessages) == 0 {
			break // Больше нет сообщений
		}

		fetchedMessages = fetchedMessages[:0] // Очищаем слайс для новой пачки
		var stopPagination bool
		for _, msg := range currentBatchTgMessages {
			dbMessage, err := c.parseMessage(msg, dbChannel.ID, channelName)
			if err != nil {
				log.Printf("Ошибка при парсинге сообщения: %v", err)
				continue
			}

			// Если сообщение старше startDate, мы дошли до начала нужного диапазона
			if dbMessage.SentAt.Before(startDate) {
				stopPagination = true
				break // Прекращаем обработку текущей пачки
			}

			// Пропускаем сообщения, которые позже endDate (это может произойти из-за OffsetDate)
			if dbMessage.SentAt.After(endDate) {
				continue
			}

			fetchedMessages = append(fetchedMessages, dbMessage)
		}

		// Если есть сообщения для сохранения, сохраняем их
		for _, msgToSave := range fetchedMessages {
			if err := c.storage.SaveMessage(ctx, &msgToSave); err != nil {
				log.Printf("Ошибка при сохранении сообщения в БД (GetMessagesInDateRange): %v", err)
				continue
			}
			totalSaved++
		}

		// Обновляем offsetID и offsetDate для следующего запроса
		// Используем данные самого старого сообщения в текущем пакете из оригинальных TG-сообщений
		oldestTgMsgInBatch := currentBatchTgMessages[len(currentBatchTgMessages)-1]
		parsedOldestMsg, err := c.parseMessage(oldestTgMsgInBatch, dbChannel.ID, channelName)
		if err != nil {
			return fmt.Errorf("failed to parse oldest message for pagination: %w", err)
		}
		offsetID = int(parsedOldestMsg.TelegramID)
		offsetDate = int(parsedOldestMsg.SentAt.Unix())

		// Если мы уже нашли сообщения старше startDate, прерываем основной цикл пагинации
		if stopPagination {
			break
		}

		// Добавляем небольшую задержку после каждого успешного запроса, чтобы избежать FLOOD_WAIT.
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	log.Printf("Успешно получено и сохранено %d сообщений из %s в диапазоне с %s по %s.",
		totalSaved, channelName, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	return nil
}

// reverseMessages реверсирует порядок сообщений в слайсе.
func reverseMessages(messages []storage.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}

// extractMessages извлекает сообщения из результата MessagesGetHistory.
func extractMessages(historyResult tg.MessagesMessagesClass) ([]tg.MessageClass, error) {
	switch result := historyResult.(type) {
	case *tg.MessagesMessages:
		return result.Messages, nil
	case *tg.MessagesChannelMessages:
		return result.Messages, nil
	default:
		return nil, fmt.Errorf("unexpected history result type: %T", result)
	}
}

// isFloodWaitError проверяет, является ли ошибка FloodWaitError, и извлекает время ожидания.
func isFloodWaitError(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}

	s := err.Error()
	if strings.Contains(s, "rpc error code 420: FLOOD_WAIT") {
		// Пример: "rpc error code 420: FLOOD_WAIT (19)"
		parts := strings.Split(s, "(")
		if len(parts) > 1 {
			waitStr := strings.TrimSuffix(strings.TrimSpace(parts[1]), ")")
			if seconds, parseErr := strconv.Atoi(waitStr); parseErr == nil {
				return time.Duration(seconds) * time.Second, true
			}
		}
		// Если не удалось распарсить время, по умолчанию ждем 5 секунд
		return 5 * time.Second, true
	}
	return 0, false
}

// SubscribeToChannel подписывается на новые сообщения из указанного канала.
// Возвращает канал Go, куда будут поступать новые сообщения (сохраняются в БД автоматически).
func (c *Client) SubscribeToChannel(ctx context.Context, channelName string) (<-chan storage.Message, error) {
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

	msgChan := make(chan storage.Message)
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

	// 1. Получаем или создаем канал в БД для подписки
	dbChannel, err := c.storage.GetChannelByUsername(ctx, channelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel from db for subscription: %w", err)
	}
	if dbChannel == nil {
		dbChannel = &storage.Channel{
			ID:        uuid.New(),
			Username:  channelName,
			Title:     sql.NullString{String: channelName, Valid: true},
			CreatedAt: time.Now(),
		}
		if err := c.storage.SaveChannel(ctx, dbChannel); err != nil {
			return nil, fmt.Errorf("failed to save new channel to db for subscription: %w", err)
		}
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
							msgChan := ch.(chan storage.Message)

							// Получаем ID канала из БД, чтобы передать его в parseMessage
							dbChannel, err := c.storage.GetChannelByUsername(ctx, targetChannelName)
							if err != nil {
								log.Printf("Ошибка получения канала из БД для handleUpdate: %v", err)
								return nil // Продолжаем обрабатывать другие обновления
							}
							if dbChannel == nil {
								log.Printf("Канал %s не найден в БД при handleUpdate", targetChannelName)
								return nil
							}
							parsedMsg, err := c.parseMessage(msg, dbChannel.ID, targetChannelName)
							if err != nil {
								log.Printf("Ошибка парсинга сообщения для подписки: %v", err)
								return nil
							}

							// Сохраняем новое сообщение в БД
							if err := c.storage.SaveMessage(ctx, &parsedMsg); err != nil {
								log.Printf("Ошибка сохранения нового сообщения из подписки в БД: %v", err)
								// Продолжаем, даже если не удалось сохранить, чтобы не блокировать поток обновлений
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

// parseMessage парсит сообщение из MTProto в нашу структуру storage.Message
func (c *Client) parseMessage(msg tg.MessageClass, channelID uuid.UUID, channelName string) (storage.Message, error) {
	var text string
	var isForward bool
	var senderUsername sql.NullString
	var messageType string
	var messageDate time.Time

	// Сохраняем оригинальные сырые данные сообщения в JSONB
	rawData, err := json.Marshal(msg)
	if err != nil {
		return storage.Message{}, fmt.Errorf("failed to marshal raw message data: %w", err)
	}

	switch m := msg.(type) {
	case *tg.Message:
		text = m.Message
		isForward = m.FwdFrom.Date != 0
		messageDate = time.Unix(int64(m.Date), 0)
		messageType = getMessageType(m)
		// TODO: Извлекать Username отправителя, если это не канал
		// if m.FromID != nil {
		// 	// Нужно разрешать Peer (User, Chat) для получения Username
		// }
	case *tg.MessageService:
		text = "Service message"
		isForward = false
		messageDate = time.Unix(int64(m.Date), 0)
		messageType = "service"
	default:
		return storage.Message{}, fmt.Errorf("unknown message type: %T", msg)
	}

	return storage.Message{
		ID:             uuid.New(),
		TelegramID:     int64(msg.GetID()),
		ChannelID:      channelID,
		Text:           sql.NullString{String: text, Valid: text != ""},
		SentAt:         messageDate,
		SenderUsername: senderUsername,
		IsForward:      sql.NullBool{Bool: isForward, Valid: true},
		MessageType:    sql.NullString{String: messageType, Valid: messageType != ""},
		RawData:        rawData,
		CreatedAt:      time.Now(),
	}, nil
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
