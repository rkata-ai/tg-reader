package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"traiding/internal/config"
	"traiding/internal/telegram"
)

func main() {
	// Парсинг флагов командной строки
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to configuration file (required)")
	flag.Parse()

	// Проверяем, что флаг config передан
	if configPath == "" {
		log.Fatalf("Usage: ./bin/trading.exe -config <path_to_config>")
	}

	log.Println("Starting Telegram reader service")
	log.Printf("Using config file: %s", configPath)

	// Загрузка конфигурации
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Создание контекста с отменой
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Инициализация Telegram клиента
	client, err := telegram.NewClient(cfg.Telegram)
	if err != nil {
		log.Fatalf("Failed to create Telegram client: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("Error closing Telegram client: %v", err)
		}
	}()

	// Запуск клиента в отдельной горутине
	go func() {
		if err := client.Start(ctx); err != nil {
			log.Fatalf("Failed to start Telegram client: %v", err)
		}
	}()

	log.Println("Telegram client initialized.")

	// Ждем, пока клиент Telegram будет готов (после авторизации)
	log.Println("Ожидание готовности Telegram клиента...")
	select {
	case <-ctx.Done():
		log.Fatalf("Контекст отменен во время ожидания готовности клиента.")
	case <-client.Ready():
		log.Println("Telegram клиент готов.")
	}

	// Пример использования: получение последних 5 сообщений из каждого канала
	for _, channel := range cfg.Telegram.Channels {
		log.Printf("\n--- Последние 5 сообщений из канала: %s ---", channel)
		messages, err := client.GetLastMessages(ctx, channel, 5)
		if err != nil {
			log.Printf("Ошибка при получении последних сообщений из %s: %v", channel, err)
			continue
		}

		log.Printf("Успешно получено %d последних сообщений из %s", len(messages), channel)
		for i, msg := range messages {
			log.Printf("  Сообщение %d (ID: %d, Дата: %s): %s",
				i+1, msg.ID, msg.Date.Format("2006-01-02 15:04:05"), truncateString(msg.Text, 100))
		}

		// Пример: получение сообщений, начиная с определенной даты (например, с 1 января 2025 года)
		targetDate := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
		log.Printf("\n--- Сообщения из канала %s, начиная с %s до настоящего времени ---",
			channel, targetDate.Format("2006-01-02"))
		messagesFromDate, err := client.GetMessagesFromDate(ctx, channel, targetDate)
		if err != nil {
			log.Printf("Ошибка при получении сообщений из %s, начиная с %s: %v", channel, targetDate.Format("2006-01-02"), err)
			continue
		}
		log.Printf("Успешно получено %d сообщений из %s, начиная с %s",
			len(messagesFromDate), channel, targetDate.Format("2006-01-02"))
		for i, msg := range messagesFromDate {
			log.Printf("  Сообщение %d (ID: %d, Дата: %s): %s",
				i+1, msg.ID, msg.Date.Format("2006-01-02 15:04:05"), truncateString(msg.Text, 100))
		}

		// Пример: получение сообщений в диапазоне дат (например, с 1 января 2025 по 31 января 2025)
		startDateRange := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
		endDateRange := time.Date(2025, time.January, 31, 23, 59, 59, 0, time.UTC)
		log.Printf("\n--- Сообщения из канала %s в диапазоне с %s по %s ---",
			channel, startDateRange.Format("2006-01-02"), endDateRange.Format("2006-01-02"))
		messagesInRange, err := client.GetMessagesInDateRange(ctx, channel, startDateRange, endDateRange)
		if err != nil {
			log.Printf("Ошибка при получении сообщений из %s в диапазоне дат: %v", channel, err)
			continue
		}
		log.Printf("Успешно получено %d сообщений из %s в диапазоне с %s по %s",
			len(messagesInRange), channel, startDateRange.Format("2006-01-02"), endDateRange.Format("2006-01-02"))
		for i, msg := range messagesInRange {
			log.Printf("  Сообщение %d (ID: %d, Дата: %s): %s",
				i+1, msg.ID, msg.Date.Format("2006-01-02 15:04:05"), truncateString(msg.Text, 100))
		}
	}

	log.Println("\nНачинаем подписку на новые сообщения...")

	// Пример использования: подписка на новые сообщения из каждого канала
	messageChannels := make([]<-chan telegram.Message, 0, len(cfg.Telegram.Channels))
	for _, channel := range cfg.Telegram.Channels {
		msgChan, err := client.SubscribeToChannel(ctx, channel)
		if err != nil {
			log.Printf("Ошибка при подписке на канал %s: %v", channel, err)
			continue
		}
		messageChannels = append(messageChannels, msgChan)
		log.Printf("Подписались на новые сообщения из канала: %s", channel)
	}

	// Горутина для обработки новых сообщений из всех подписанных каналов
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-mergeChannels(ctx, messageChannels):
				if !ok {
					log.Println("Все каналы сообщений закрыты.")
					return
				}
				log.Printf("\n--- НОВОЕ СООБЩЕНИЕ из %s ---", msg.Channel)
				log.Printf("ID: %d", msg.ID)
				log.Printf("Текст: %s", truncateString(msg.Text, 100))
				log.Printf("Дата: %s", msg.Date.Format("2006-01-02 15:04:05"))
				log.Printf("Переслано: %t", msg.IsForward)
				log.Printf("Тип: %s", msg.Type)
			}
		}
	}()

	// Ожидание сигнала завершения
	log.Println("\nPress Ctrl+C to exit...")
	<-sigChan
	log.Println("Shutting down...")
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

// mergeChannels объединяет несколько каналов сообщений в один.
func mergeChannels(ctx context.Context, channels []<-chan telegram.Message) <-chan telegram.Message {
	var wg sync.WaitGroup
	out := make(chan telegram.Message)

	// Функция для копирования сообщений из одного канала в выходной канал
	multiplex := func(c <-chan telegram.Message) {
		defer wg.Done()
		for {
			select {
			case msg, ok := <-c:
				if !ok {
					return
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}

	wg.Add(len(channels))
	for _, c := range channels {
		go multiplex(c)
	}

	// Запускаем горутину, которая закроет выходной канал после завершения всех копирований.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
