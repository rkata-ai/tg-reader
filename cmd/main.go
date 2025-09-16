package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"rkata-ai/tg-reader/internal/config"
	"rkata-ai/tg-reader/internal/storage"
	"rkata-ai/tg-reader/internal/telegram"
)

func main() {
	// Парсинг флагов командной строки
	var configPath string
	var debugConsole bool
	var debugFilePath string
	// var mode string
	var testMessageLimit int

	flag.StringVar(&configPath, "c", "", "Path to configuration file (required)")
	flag.BoolVar(&debugConsole, "d", false, "Enable debug output to console")
	flag.StringVar(&debugFilePath, "f", "", "Path to file for debug output")
	flag.IntVar(&testMessageLimit, "l", 10, "Number of last messages to fetch for 'last' mode. Default: 10")
	flag.Parse()

	// Проверяем, что флаг config передан
	if configPath == "" {
		log.Fatalf("Usage: ./bin/tg-reader.exe -c <path_to_config> [-d] [-f <path>] [-l <limit>]")
	}

	log.Println("Starting Telegram reader service")
	log.Printf("Using config file: %s", configPath)

	// Загрузка конфигурации
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Инициализация хранилища PostgreSQL
	store, err := storage.NewPostgresStorage(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to create Postgres storage: %v", err)
	}

	store.Migrate(context.Background())

	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("Error closing Postgres storage: %v", err)
		}
	}()

	// Создание контекста с отменой, который будет отменен при получении сигналов ОС
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Добавляем горутину для отслеживания отмены контекста
	go func() {
		<-ctx.Done()
		log.Println("Главный контекст отменен (Ctrl+C или SIGTERM). Завершение программы...")
	}()

	// Инициализация Telegram клиента
	client, err := telegram.NewClient(cfg.Telegram, store)
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

	// Отладочный вывод, если включен
	var debugFile *os.File
	if debugFilePath != "" {
		var fileErr error
		debugFile, fileErr = os.Create(debugFilePath)
		if fileErr != nil {
			log.Fatalf("Failed to create debug file %s: %v", debugFilePath, fileErr)
		}
		defer func() {
			if closeErr := debugFile.Close(); closeErr != nil {
				log.Printf("Error closing debug file: %v", closeErr)
			}
		}()
		log.Printf("Debug output enabled to file: %s", debugFilePath)
	} else if debugConsole {
		log.Println("Debug output enabled to console.")
	}

	if debugConsole {
		// Логика для режима 'last' (отладка)
		if len(cfg.Telegram.Channels) == 0 {
			log.Fatalf("No channels specified in config for debug mode.")
		}
		testChannelName := cfg.Telegram.Channels[0]
		lastMessages, err := client.GetLastMessages(ctx, testChannelName, testMessageLimit)
		if err != nil {
			log.Fatalf("Failed to fetch last messages in debug mode: %v", err)
		} else {
			log.Printf("Последние %d сообщений из канала %s (получены напрямую из Telegram):", len(lastMessages), testChannelName)
			for i, msg := range lastMessages {
				output := fmt.Sprintf("%d. [%s] %s: %s", i+1, msg.SentAt.Format("2006-01-02 15:04:05"), testChannelName, msg.Text.String)
				if debugConsole {
					fmt.Println(output)
				} else if debugFilePath != "" && debugFile != nil {
					if _, writeErr := debugFile.WriteString(output + "\n"); writeErr != nil {
						log.Printf("Ошибка записи в файл отладки: %v", writeErr)
					}
				}
			}
		}
		log.Println("Режим отладки завершен.")
	} else {
		// Логика для режима 'full'
		startDate, err := time.Parse("2006-01-02", cfg.Telegram.StartDate)
		if err != nil {
			log.Fatalf("Invalid start_date in config: %v. Expected format YYYY-MM-DD.", err)
		}

		for _, channel := range cfg.Telegram.Channels {
			log.Printf("Получение сообщений из канала %s с %s и сохранение в БД...", channel, startDate.Format("2006-01-02"))
			err = client.GetMessagesFromDate(ctx, channel, startDate)
			if err != nil {
				log.Printf("Ошибка при получении и сохранении сообщений из %s: %v", channel, err)
			}

			if debugConsole || debugFilePath != "" {
				debugMsg := fmt.Sprintf("Сообщения из канала %s были сохранены в базу данных начиная с %s.", channel, startDate.Format("2006-01-02"))
				if debugConsole {
					fmt.Println(debugMsg)
				} else if debugFilePath != "" {
					if _, writeErr := debugFile.WriteString(debugMsg + "\n"); writeErr != nil {
						log.Printf("Ошибка записи в файл отладки: %v", writeErr)
					}
				}
			}
		}
		log.Println("Полная обработка завершена. Запускается режим подписки на новые сообщения...")

		// Логика подписки на новые сообщения
		var wg sync.WaitGroup
		for _, channel := range cfg.Telegram.Channels {
			currentChannel := channel // Создаем локальную копию переменной для горутины
			wg.Add(1)
			go func() {
				defer wg.Done()
				log.Printf("Подписка на канал: %s", currentChannel)
				msgChan, err := client.SubscribeToChannel(ctx, currentChannel)
				if err != nil {
					log.Printf("Ошибка подписки на канал %s: %v", currentChannel, err)
					return
				}
				for {
					select {
					case msg := <-msgChan:
						// Сообщение уже сохранено в БД функцией SubscribeToChannel
						// Можно добавить дополнительную логику обработки или логирования здесь
						log.Printf("Новое сообщение в %s: ID=%d (сохранено в БД)", currentChannel, msg.TelegramID)
					case <-ctx.Done():
						log.Printf("Отмена подписки на канал %s.", currentChannel)
						return
					}
				}
			}()
		}
		wg.Wait() // Ждем завершения всех горутин подписки при отмене контекста
	}

	log.Println("\nPress Ctrl+C to exit...")
	<-ctx.Done() // Ждем отмены контекста
	log.Println("Shutting down...")
}
