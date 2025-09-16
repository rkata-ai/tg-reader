# Telegram Reader

Этот проект представляет собой Go-модуль для чтения истории сообщений из Telegram-каналов за определенный период (с начала текущего года) с использованием MTProto API (`gotd/td`). Полученные сообщения сохраняются в базу данных PostgreSQL для дальнейшего анализа с помощью AI.

## 🎯 Цель проекта

Предоставить удобный инструмент для получения и сохранения полной истории текстовых сообщений из указанных Telegram-каналов в базу данных PostgreSQL в структурированном формате для последующего анализа.

## 🏗️ Архитектура

```
tg-reader/
├── cmd/main.go              # Точка входа: демонстрационное приложение для получения истории
├── internal/
│   ├── config/              # Конфигурация приложения
│   ├── storage/             # Логика для работы с базой данных (PostgreSQL)
│   │   ├── models.go        # Модели данных (Channel, Message)
│   │   ├── postgres.go      # Реализация хранилища PostgreSQL
│   │   └── storage.go       # Интерфейс хранилища
│   └── telegram/            # API для работы с Telegram MTProto
├── configs/                 # Конфигурационные файлы
└── go.mod                   # Зависимости Go
```

## 🚀 Быстрый старт

### Предварительные требования

1.  **Go 1.24.3+** - [скачать](https://golang.org/dl/)
2.  **Telegram API ID и API Hash**:
    *   Перейдите на [my.telegram.org](https://my.telegram.org/).
    *   Войдите в свою учетную запись Telegram.
    *   Нажмите "API development tools" и заполните форму.
    *   Вы получите `App api_id` и `App api_hash`.

### Установка зависимостей

```bash
go mod tidy
```

### Настройка конфигурации

1.  Отредактируйте файл `configs/config.yaml` или создайте `configs/config.local.yaml` (предпочтительно) со следующими параметрами:

    ```yaml
    telegram:
      api_id: "ВАШ_API_ID"        # Ваш API ID из my.telegram.org
      api_hash: "ВАШ_API_HASH"    # Ваш API Hash из my.telegram.org
      channels:
        - "channel_name_1"         # Имена каналов без @ (например: "trading_signals")
        - "channel_name_2"         # Добавьте каналы, историю которых хотите получить
    ```

    *   `api_id` и `api_hash` являются обязательными для авторизации через MTProto.
    *   `channels` - список каналов, историю которых вы хотите получить.

### Запуск приложения

1.  **Сборка приложения:**

    ```bash
    go build -o ./bin/tg-reader.exe ./cmd
    ```

2.  **Запуск приложения с флагами:**

    Сервис теперь имеет два режима работы, определяемые флагом отладки `-d` (или `--debugConsole`):

    ### Режим отладки: `-d` или `--debugConsole`
    В этом режиме сервис получает указанное количество последних сообщений из **первого канала, указанного в `config.yaml`**. Сообщения **НЕ сохраняются в базу данных**.
    В консоль (или в файл, если используется `-f`) выводится детальная информация о каждом полученном сообщении (ID, время, текст).
    После вывода сообщений приложение завершает работу.

    **Флаги специфичные для режима отладки:**
    *   `-l <лимит>`: Количество последних сообщений для получения. По умолчанию: 10.

    **Пример запуска:**
    ```bash
    ./bin/tg-reader.exe -c configs/config.yaml -d -l 5
    # С отладочным выводом в файл (запишет 10 последних сообщений по умолчанию):
    ./bin/tg-reader.exe -c configs/config.yaml -f last_messages.log
    ```

    ### Режим полной обработки (по умолчанию):
    Если флаг `-d` **не указан**, сервис работает в этом режиме. Он читает каналы из файла конфигурации и получает сообщения, начиная с даты, указанной в `Telegram.StartDate` в `config.yaml`. Все полученные сообщения **сохраняются в базу данных PostgreSQL**.
    Для каждого обработанного канала в консоль выводятся логи о процессе. Если флаг `-f` используется, то отладочные сообщения о сохранении в БД будут записаны в файл.

    **Пример запуска:**
    ```bash
    ./bin/tg-reader.exe -c configs/config.yaml
    # С отладочным выводом в файл:
    ./bin/tg-reader.exe -c configs/config.yaml -f full_processing.log
    ```

    При первом запуске вам будет предложено ввести номер телефона и код авторизации из Telegram. Следуйте инструкциям в терминале.

    Для остановки программы используйте `Ctrl+C`.

## 📝 API пакета `internal/telegram`

Пакет `internal/telegram` предоставляет интерфейс `Reader` для взаимодействия с Telegram-каналами. В демонстрационном приложении `cmd/main.go` используются методы `GetMessagesFromDate` и `GetLastMessages`.

### Интерфейс `Reader`

```go
type Reader interface {
	GetLastMessages(ctx context.Context, channel string, limit int) ([]storage.Message, error)
	GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time) error
	GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time) error
	SubscribeToChannel(ctx context.Context, channel string) (<-chan storage.Message, error)
	Close() error
}
```

*   `GetLastMessages(ctx context.Context, channel string, limit int) ([]storage.Message, error)`: Получает последние `limit` сообщений из указанного `channel` **без сохранения в базу данных**. Возвращает слайс сообщений для отладки и выводит сообщения напрямую в консоль/файл.
*   `GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time)`: Получает сообщения из указанного `channel`, начиная с `startDate` до настоящего времени, и **сохраняет их в базу данных PostgreSQL**.
*   `GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time)`: Получает сообщения из указанного `channel` в диапазоне от `startDate` до `endDate`, и **сохраняет их в базу данных PostgreSQL**.
*   `SubscribeToChannel(ctx context.Context, channel string)`: Подписывается на новые сообщения из указанного `channel`. Возвращает канал `<-chan storage.Message`, в который будут поступать новые сообщения (эти сообщения **сохраняются в БД PostgreSQL автоматически**).
*   `Close()`: Закрывает соединение с Telegram-клиентом.

### Интерфейс `Storage`

Пакет `internal/storage` предоставляет интерфейс `Storage` для взаимодействия с базой данных PostgreSQL:

```go
type Storage interface {
	// Channels
	SaveChannel(ctx context.Context, channel *Channel) error
	GetChannelByUsername(ctx context.Context, username string) (*Channel, error)

	// Messages
	SaveMessage(ctx context.Context, message *Message) error

	// Health Check
	Close() error

	// Migration
	Migrate(ctx context.Context) error
}
```

### Структуры данных пакета `internal/storage`

Проект использует следующие структуры для хранения данных:

```go
type Channel struct {
	ID        int64
	Username  string
	Title     sql.NullString
	Link      sql.NullString
	CreatedAt time.Time
}

type Message struct {
	ID             int64          // Уникальный идентификатор сообщения в БД
	TelegramID     int64          // ID сообщения в Telegram
	ChannelID      int64          // ID канала в БД
	Text           sql.NullString // Текст сообщения
	SentAt         time.Time      // Дата и время отправки сообщения
	SenderUsername sql.NullString // Имя пользователя отправившего сообщение, если доступно
	IsForward      sql.NullBool   // Флаг, указывающий, является ли сообщение пересланным
	MessageType    sql.NullString // Тип сообщения (например, "text", "photo", "video" и т.д.)
	RawData        []byte         // Сырые данные сообщения в JSONB формате
	CreatedAt      time.Time      // Дата создания записи в БД
}

```

## 📞 Поддержка

По вопросам и предложениям создавайте Issues в репозитории.

