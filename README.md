# Telegram Reader

Этот проект представляет собой Go-модуль для чтения сообщений из Telegram-каналов с использованием MTProto API (`gotd/td`). Он предоставляет простой API для получения последних сообщений и подписки на новые сообщения из указанных каналов.

## 🎯 Цель проекта

Предоставить удобный и надежный способ взаимодействия с Telegram-каналами для получения текстовых сообщений.

## 🏗️ Архитектура

```
tg-reader/
├── cmd/main.go              # Демонстрационное приложение
├── internal/
│   ├── config/              # Конфигурация приложения
│   └── telegram/            # API для работы с Telegram MTProto
├── configs/                 # Конфигурационные файлы
└── go.mod                   # Зависимости Go
```

## 🚀 Быстрый старт

### Предварительные требования

1.  **Go 1.21+** - [скачать](https://golang.org/dl/)
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
        - "channel_name_2"         # Добавьте каналы, которые хотите анализировать (опционально)
    ```

    *   `api_id` и `api_hash` являются обязательными для авторизации через MTProto.
    *   `channels` - список каналов, из которых вы хотите получать сообщения. Это опционально для запуска, но необходимо для демонстрации получения сообщений.

### Запуск демонстрационного приложения

1.  **Сборка приложения:**

    ```bash
    go build -o ./bin/tg.exe ./cmd
    ```

2.  **Запуск приложения:**

    ```bash
    ./bin/tg.exe -config configs/config.yaml
    ```

    При первом запуске вам будет предложено ввести номер телефона и код авторизации из Telegram. Следуйте инструкциям в терминале.

## 📝 API пакета `internal/telegram`

Пакет `internal/telegram` предоставляет интерфейс `Reader` для взаимодействия с Telegram-каналами.

### Интерфейс `Reader`

```go
type Reader interface {
	GetLastMessages(ctx context.Context, channel string, limit int) ([]Message, error)
	GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time) ([]Message, error)
	GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time) ([]Message, error)
	SubscribeToChannel(ctx context.Context, channel string) (<-chan Message, error)
	Close() error
}
```

*   `GetLastMessages(ctx context.Context, channel string, limit int)`: Получает последние `limit` сообщений из указанного `channel`.
*   `GetMessagesFromDate(ctx context.Context, channel string, startDate time.Time)`: Получает сообщения из указанного `channel`, начиная с `startDate` до настоящего времени.
*   `GetMessagesInDateRange(ctx context.Context, channel string, startDate time.Time, endDate time.Time)`: Получает сообщения из указанного `channel` в диапазоне от `startDate` до `endDate`.
*   `SubscribeToChannel(ctx context.Context, channel string)`: Подписывается на новые сообщения из указанного `channel`. Возвращает канал `<-chan Message`, в который будут поступать новые сообщения.
*   `Close()`: Закрывает соединение с Telegram-клиентом.

### Структура `Message`

```go
type Message struct {
	ID        int64
	Text      string
	Date      time.Time
	Channel   string
	Username  string
	IsForward bool
	Type      string
}
```

## 📞 Поддержка

По вопросам и предложениям создавайте Issues в репозитории.

