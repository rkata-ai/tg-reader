package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"rkata-ai/tg-reader/internal/config"

	// "github.com/google/uuid"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresStorage реализует интерфейс Storage для PostgreSQL
type PostgresStorage struct {
	db *sql.DB
}

// NewPostgresStorage создает новое хранилище PostgreSQL
func NewPostgresStorage(cfg *config.DatabaseConfig) (Storage, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
	if cfg.ConnectionString != "" {
		connStr = cfg.ConnectionString
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err = db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &PostgresStorage{db: db}, nil
}

// Close закрывает соединение с базой данных
func (p *PostgresStorage) Close() error {
	return p.db.Close()
}

// Migrate выполняет SQL-скрипты для создания таблиц
func (p *PostgresStorage) Migrate(ctx context.Context) error {
	schema := `
	    DROP TABLE IF EXISTS messages CASCADE;
	    DROP TABLE IF EXISTS channels CASCADE;

	    CREATE TABLE channels (
	        id BIGSERIAL PRIMARY KEY,
	        username TEXT UNIQUE NOT NULL,
	        title TEXT,
	        link TEXT,
	        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	    );

	    CREATE TABLE messages (
	        id BIGSERIAL PRIMARY KEY,
	        telegram_id BIGINT UNIQUE NOT NULL,
	        channel_id BIGINT REFERENCES channels(id) ON DELETE CASCADE NOT NULL,
	        text TEXT,
	        sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
	        sender_username TEXT,
	        is_forward BOOLEAN,
	        message_type TEXT,
	        raw_data JSONB,
	        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	    );

	    CREATE INDEX idx_channels_username ON channels (username);
	    CREATE INDEX idx_messages_channel_id ON messages (channel_id);
	    CREATE INDEX idx_messages_sent_at ON messages (sent_at DESC);
	`

	_, err := p.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to execute migration script: %w", err)
	}
	log.Println("Database migration completed successfully.")
	return nil
}

// --- Реализации методов интерфейса Storage ---

func (p *PostgresStorage) SaveChannel(ctx context.Context, channel *Channel) error {
	query := `INSERT INTO channels (id, username, title, link, created_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (username) DO UPDATE SET title = EXCLUDED.title, link = EXCLUDED.link RETURNING id`
	err := p.db.QueryRowContext(ctx, query, channel.ID, channel.Username, channel.Title, channel.Link, channel.CreatedAt).Scan(&channel.ID)
	if err != nil {
		return fmt.Errorf("failed to save channel: %w", err)
	}
	return nil
}

func (p *PostgresStorage) GetChannelByUsername(ctx context.Context, username string) (*Channel, error) {
	channel := &Channel{}
	query := `SELECT id, username, title, link, created_at FROM channels WHERE username = $1`
	err := p.db.QueryRowContext(ctx, query, username).Scan(&channel.ID, &channel.Username, &channel.Title, &channel.Link, &channel.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Канал не найден
		}
		return nil, fmt.Errorf("failed to get channel by username: %w", err)
	}
	return channel, nil
}

func (p *PostgresStorage) SaveMessage(ctx context.Context, message *Message) error {
	query := `
		INSERT INTO messages (telegram_id, channel_id, text, sent_at, sender_username, is_forward, message_type, raw_data, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (telegram_id) DO NOTHING
		RETURNING id
	`
	// Если ID не установлен, генерируем его
	// if message.ID == 0 {
	// }

	err := p.db.QueryRowContext(ctx, query,
		// message.ID,
		message.TelegramID,
		message.ChannelID,
		message.Text,
		message.SentAt,
		message.SenderUsername,
		message.IsForward,
		message.MessageType,
		message.RawData,
		message.CreatedAt,
	).Scan(&message.ID) // Scan back the ID in case of an insert (or existing for conflict)

	if err != nil {
		if err == sql.ErrNoRows { // Это может произойти при ON CONFLICT DO NOTHING, если запись уже существовала
			return nil // Сообщение уже существует, ничего не делаем.
		}
		return fmt.Errorf("failed to save message: %w", err)
	}
	return nil
}
