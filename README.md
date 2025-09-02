# Telegram Reader

Этот проект представляет собой Go-модуль для чтения истории сообщений из Telegram-каналов за определенный период (по умолчанию за последний месяц) с использованием MTProto API (`gotd/td`). Полученные сообщения сохраняются в JSON-файл.

## 🎯 Цель проекта

Предоставить удобный инструмент для получения и сохранения полной истории текстовых сообщений из указанных Telegram-каналов в формате JSON.

## 🏗️ Архитектура

```
tg-reader/
├── cmd/main.go              # Точка входа: демонстрационное приложение для получения истории
├── internal/
│   ├── config/              # Конфигурация приложения
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
    go build -o ./bin/tg.exe ./cmd
    ```

2.  **Запуск приложения:**

    ```bash
    ./bin/tg.exe -config configs/config.yaml
    ```

    При первом запуске вам будет предложено ввести номер телефона и код авторизации из Telegram. Следуйте инструкциям в терминале.

    Приложение получит сообщения из всех указанных в конфигурации каналов за последний месяц и сохранит их в отдельные JSON-файлы в корневой директории проекта. Имя файла будет иметь формат `messages_<channel>_<YYYY-MM>.json` (например: `messages_wind_sower_2025-09.json`).

    Для остановки программы используйте `Ctrl+C`.

## 📝 API пакета `internal/telegram`

Пакет `internal/telegram` предоставляет интерфейс `Reader` для взаимодействия с Telegram-каналами. В демонстрационном приложении `cmd/main.go` используется метод `GetMessagesFromDate`.

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

