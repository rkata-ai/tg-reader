package main

import (
	"context"
	"encoding/json" // Добавляем импорт для работы с JSON
	"flag"
	"fmt" // Добавляем импорт для fmt.Printf
	"log"
	"os"
	"os/signal"

	// "sync" // Удаляем, так как mergeChannels и sync.Map больше не используются
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

	// Создание контекста с отменой, который будет отменен при получении сигналов ОС
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Добавляем горутину для отслеживания отмены контекста
	go func() {
		<-ctx.Done()
		log.Println("Главный контекст отменен (Ctrl+C или SIGTERM). Завершение программы...")
	}()

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

	// Пример использования: получение сообщений, начиная с определенной даты (например, с 1 месяца назад)
	for _, channel := range cfg.Telegram.Channels {
		now := time.Now()
		oneMonthAgo := now.AddDate(0, -1, 0)                                                            // Один месяц назад
		targetDate := time.Date(oneMonthAgo.Year(), oneMonthAgo.Month(), 1, 0, 0, 0, 0, now.Location()) // Начало месяца один месяц назад
		log.Printf("\n--- Сообщения из канала %s, начиная с %s до настоящего времени ---",
			channel, targetDate.Format("2006-01-02"))
		messagesFromDate, err := client.GetMessagesFromDate(ctx, channel, targetDate)
		if err != nil {
			log.Printf("Ошибка при получении сообщений из %s, начиная с %s: %v", channel, targetDate.Format("2006-01-02"), err)
			continue
		}
		log.Printf("Успешно получено %d сообщений из %s, начиная с %s.",
			len(messagesFromDate), channel, targetDate.Format("2006-01-02"))

		// Вывод сообщений в формате JSON
		jsonData, err := json.MarshalIndent(messagesFromDate, "", "  ")
		if err != nil {
			log.Printf("Ошибка при сериализации сообщений в JSON: %v", err)
		} else {
			// Сохраняем JSON в файл
			filename := fmt.Sprintf("messages_%s_%s.json", channel, targetDate.Format("2006-01"))
			file, err := os.Create(filename)
			if err != nil {
				log.Printf("Ошибка при создании файла %s: %v", filename, err)
			} else {
				defer file.Close()
				_, err = file.Write(jsonData)
				if err != nil {
					log.Printf("Ошибка при записи JSON в файл %s: %v", filename, err)
				} else {
					log.Printf("Сообщения успешно сохранены в файл: %s", filename)
				}
			}
		}
	}

	log.Println("\nPress Ctrl+C to exit...")
	<-ctx.Done() // Ждем отмены контекста
	log.Println("Shutting down...")
}

// truncateString функция больше не нужна
// func truncateString(s string, maxLen int) string {
// 	runes := []rune(s)
// 	if len(runes) > maxLen {
// 		return string(runes[:maxLen]) + "..."
// 	}
// 	return s
// }

// mergeChannels функция больше не нужна
// func mergeChannels(ctx context.Context, channels []<-chan telegram.Message) <-chan telegram.Message {
// 	var wg sync.WaitGroup
// 	out := make(chan telegram.Message)
//
// 	multiplex := func(c <-chan telegram.Message) {
// 		defer wg.Done()
// 		for {
// 			select {
// 			case msg, ok := <-c:
// 				if !ok {
// 					return
// 				}
// 				select {
// 				case out <- msg:
// 				case <-ctx.Done():
// 					return
// 				}
// 			case <-ctx.Done():
// 				return
// 			}
// 		}
// 	}
//
// 	wg.Add(len(channels))
// 	for _, c := range channels {
// 		go multiplex(c)
// 	}
//
// 	go func() {
// 		wg.Wait()
// 		close(out)
// 	}()
//
// 	return out
// }
