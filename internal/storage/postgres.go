package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"rkata-ai/tg-reader/internal/config"

	"github.com/google/uuid"
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

// Ping проверяет доступность базы данных
func (p *PostgresStorage) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// Migrate выполняет SQL-скрипты для создания таблиц
func (p *PostgresStorage) Migrate(ctx context.Context) error {
	schema := `
	    DROP TABLE IF EXISTS predictions CASCADE;
	    DROP TABLE IF EXISTS messages CASCADE;
	    DROP TABLE IF EXISTS channels CASCADE;
	    DROP TABLE IF EXISTS stocks CASCADE;
	    DROP TABLE IF EXISTS industries CASCADE;

	    CREATE TABLE industries (
	        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	        name TEXT UNIQUE NOT NULL,
	        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	    );

	    CREATE TABLE stocks (
	        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	        ticker TEXT UNIQUE NOT NULL,
	        name TEXT,
	        industry_id UUID REFERENCES industries(id) ON DELETE SET NULL,
	        exchange TEXT,
	        currency TEXT,
	        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	    );

	    CREATE TABLE channels (
	        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	        username TEXT UNIQUE NOT NULL,
	        title TEXT,
	        link TEXT,
	        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	    );

	    CREATE TABLE messages (
	        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	        telegram_id BIGINT UNIQUE NOT NULL,
	        channel_id UUID REFERENCES channels(id) ON DELETE CASCADE NOT NULL,
	        text TEXT,
	        sent_at TIMESTAMP WITH TIME ZONE NOT NULL,
	        sender_username TEXT,
	        is_forward BOOLEAN,
	        message_type TEXT,
	        raw_data JSONB,
	        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	    );

	    CREATE TABLE predictions (
	        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	        message_id UUID REFERENCES messages(id) ON DELETE CASCADE NOT NULL,
	        stock_id UUID REFERENCES stocks(id) ON DELETE CASCADE NOT NULL,
	        prediction_type TEXT,
	        target_price NUMERIC,
	        target_change_percent NUMERIC,
	        period TEXT,
	        recommendation TEXT,
	        direction TEXT,
	        justification_text TEXT,
	        predicted_at TIMESTAMP WITH TIME ZONE NOT NULL,
	        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	    );

	    CREATE INDEX idx_channels_username ON channels (username);
	    CREATE INDEX idx_messages_channel_id ON messages (channel_id);
	    CREATE INDEX idx_messages_sent_at ON messages (sent_at DESC);
	    CREATE INDEX idx_stocks_ticker ON stocks (ticker);
	    CREATE INDEX idx_predictions_message_id ON predictions (message_id);
	    CREATE INDEX idx_predictions_stock_id ON predictions (stock_id);
	    CREATE INDEX idx_predictions_predicted_at ON predictions (predicted_at DESC);
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

func (p *PostgresStorage) GetChannelByID(ctx context.Context, id uuid.UUID) (*Channel, error) {
	channel := &Channel{}
	query := `SELECT id, username, title, link, created_at FROM channels WHERE id = $1`
	err := p.db.QueryRowContext(ctx, query, id).Scan(&channel.ID, &channel.Username, &channel.Title, &channel.Link, &channel.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get channel by ID: %w", err)
	}
	return channel, nil
}

func (p *PostgresStorage) SaveMessage(ctx context.Context, message *Message) error {
	query := `
		INSERT INTO messages (id, telegram_id, channel_id, text, sent_at, sender_username, is_forward, message_type, raw_data, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (telegram_id) DO NOTHING
		RETURNING id
	`
	// Если ID не установлен, генерируем его
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}

	err := p.db.QueryRowContext(ctx, query,
		message.ID,
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

func (p *PostgresStorage) GetMessageByID(ctx context.Context, id uuid.UUID) (*Message, error) {
	message := &Message{}
	query := `SELECT id, telegram_id, channel_id, text, sent_at, sender_username, is_forward, message_type, raw_data, created_at FROM messages WHERE id = $1`
	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&message.ID,
		&message.TelegramID,
		&message.ChannelID,
		&message.Text,
		&message.SentAt,
		&message.SenderUsername,
		&message.IsForward,
		&message.MessageType,
		&message.RawData,
		&message.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get message by ID: %w", err)
	}
	return message, nil
}

func (p *PostgresStorage) GetMessagesWithoutPredictions(ctx context.Context, limit int) ([]Message, error) {
	messages := []Message{}
	query := `
		SELECT
			m.id, m.telegram_id, m.channel_id, m.text, m.sent_at, m.sender_username, m.is_forward, m.message_type, m.raw_data, m.created_at
		FROM
			messages m
		LEFT JOIN
			predictions p ON m.id = p.message_id
		WHERE
			p.id IS NULL
		ORDER BY
			m.sent_at ASC
		LIMIT $1
	`
	rows, err := p.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages without predictions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		message := Message{}
		err := rows.Scan(
			&message.ID,
			&message.TelegramID,
			&message.ChannelID,
			&message.Text,
			&message.SentAt,
			&message.SenderUsername,
			&message.IsForward,
			&message.MessageType,
			&message.RawData,
			&message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}
		messages = append(messages, message)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return messages, nil
}

func (p *PostgresStorage) GetLastMessagesForChannel(ctx context.Context, channelID uuid.UUID, limit int) ([]Message, error) {
	messages := []Message{}
	query := `
		SELECT id, telegram_id, channel_id, text, sent_at, sender_username, is_forward, message_type, raw_data, created_at
		FROM messages
		WHERE channel_id = $1
		ORDER BY sent_at DESC, telegram_id DESC
		LIMIT $2
	`
	rows, err := p.db.QueryContext(ctx, query, channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get last messages for channel: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		message := Message{}
		err := rows.Scan(
			&message.ID,
			&message.TelegramID,
			&message.ChannelID,
			&message.Text,
			&message.SentAt,
			&message.SenderUsername,
			&message.IsForward,
			&message.MessageType,
			&message.RawData,
			&message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message row (GetLastMessagesForChannel): %w", err)
		}
		messages = append(messages, message)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration (GetLastMessagesForChannel): %w", err)
	}

	// Возвращаем сообщения в хронологическом порядке (от старых к новым)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (p *PostgresStorage) SaveIndustry(ctx context.Context, industry *Industry) error {
	query := `INSERT INTO industries (id, name, created_at) VALUES ($1, $2, $3) ON CONFLICT (name) DO NOTHING RETURNING id`
	if industry.ID == uuid.Nil {
		industry.ID = uuid.New()
	}
	err := p.db.QueryRowContext(ctx, query, industry.ID, industry.Name, industry.CreatedAt).Scan(&industry.ID)
	if err != nil {
		if err == sql.ErrNoRows { // Запись уже существует, ничего не делаем
			return nil
		}
		return fmt.Errorf("failed to save industry: %w", err)
	}
	return nil
}

func (p *PostgresStorage) GetIndustryByName(ctx context.Context, name string) (*Industry, error) {
	industry := &Industry{}
	query := `SELECT id, name, created_at FROM industries WHERE name = $1`
	err := p.db.QueryRowContext(ctx, query, name).Scan(&industry.ID, &industry.Name, &industry.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get industry by name: %w", err)
	}
	return industry, nil
}

func (p *PostgresStorage) GetIndustryByID(ctx context.Context, id uuid.UUID) (*Industry, error) {
	industry := &Industry{}
	query := `SELECT id, name, created_at FROM industries WHERE id = $1`
	err := p.db.QueryRowContext(ctx, query, id).Scan(&industry.ID, &industry.Name, &industry.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get industry by ID: %w", err)
	}
	return industry, nil
}

func (p *PostgresStorage) SaveStock(ctx context.Context, stock *Stock) error {
	query := `INSERT INTO stocks (id, ticker, name, industry_id, exchange, currency, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (ticker) DO NOTHING RETURNING id`
	if stock.ID == uuid.Nil {
		stock.ID = uuid.New()
	}
	err := p.db.QueryRowContext(ctx, query, stock.ID, stock.Ticker, stock.Name, stock.IndustryID, stock.Exchange, stock.Currency, stock.CreatedAt).Scan(&stock.ID)
	if err != nil {
		if err == sql.ErrNoRows { // Запись уже существует, ничего не делаем
			return nil
		}
		return fmt.Errorf("failed to save stock: %w", err)
	}
	return nil
}

func (p *PostgresStorage) GetStockByTicker(ctx context.Context, ticker string) (*Stock, error) {
	stock := &Stock{}
	query := `SELECT id, ticker, name, industry_id, exchange, currency, created_at FROM stocks WHERE ticker = $1`
	err := p.db.QueryRowContext(ctx, query, ticker).Scan(&stock.ID, &stock.Ticker, &stock.Name, &stock.IndustryID, &stock.Exchange, &stock.Currency, &stock.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get stock by ticker: %w", err)
	}
	return stock, nil
}

func (p *PostgresStorage) GetStockByID(ctx context.Context, id uuid.UUID) (*Stock, error) {
	stock := &Stock{}
	query := `SELECT id, ticker, name, industry_id, exchange, currency, created_at FROM stocks WHERE id = $1`
	err := p.db.QueryRowContext(ctx, query, id).Scan(&stock.ID, &stock.Ticker, &stock.Name, &stock.IndustryID, &stock.Exchange, &stock.Currency, &stock.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get stock by ID: %w", err)
	}
	return stock, nil
}

func (p *PostgresStorage) SavePrediction(ctx context.Context, prediction *Prediction) error {
	query := `
		INSERT INTO predictions (id, message_id, stock_id, prediction_type, target_price, target_change_percent, period, recommendation, direction, justification_text, predicted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`
	if prediction.ID == uuid.Nil {
		prediction.ID = uuid.New()
	}

	err := p.db.QueryRowContext(ctx, query,
		prediction.ID,
		prediction.MessageID,
		prediction.StockID,
		prediction.PredictionType,
		prediction.TargetPrice,
		prediction.TargetChangePercent,
		prediction.Period,
		prediction.Recommendation,
		prediction.Direction,
		prediction.JustificationText,
		prediction.PredictedAt,
		prediction.CreatedAt,
	).Scan(&prediction.ID)
	if err != nil {
		return fmt.Errorf("failed to save prediction: %w", err)
	}
	return nil
}

func (p *PostgresStorage) GetPredictionsByMessageID(ctx context.Context, messageID uuid.UUID) ([]Prediction, error) {
	predictions := []Prediction{}
	query := `SELECT id, message_id, stock_id, prediction_type, target_price, target_change_percent, period, recommendation, direction, justification_text, predicted_at, created_at FROM predictions WHERE message_id = $1 ORDER BY created_at ASC`
	rows, err := p.db.QueryContext(ctx, query, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get predictions by message ID: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		prediction := Prediction{}
		err := rows.Scan(
			&prediction.ID,
			&prediction.MessageID,
			&prediction.StockID,
			&prediction.PredictionType,
			&prediction.TargetPrice,
			&prediction.TargetChangePercent,
			&prediction.Period,
			&prediction.Recommendation,
			&prediction.Direction,
			&prediction.JustificationText,
			&prediction.PredictedAt,
			&prediction.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan prediction row: %w", err)
		}
		predictions = append(predictions, prediction)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return predictions, nil
}

func (p *PostgresStorage) GetPredictionsByStockID(ctx context.Context, stockID uuid.UUID) ([]Prediction, error) {
	predictions := []Prediction{}
	query := `SELECT id, message_id, stock_id, prediction_type, target_price, target_change_percent, period, recommendation, direction, justification_text, predicted_at, created_at FROM predictions WHERE stock_id = $1 ORDER BY created_at ASC`
	rows, err := p.db.QueryContext(ctx, query, stockID)
	if err != nil {
		return nil, fmt.Errorf("failed to get predictions by stock ID: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		prediction := Prediction{}
		err := rows.Scan(
			&prediction.ID,
			&prediction.MessageID,
			&prediction.StockID,
			&prediction.PredictionType,
			&prediction.TargetPrice,
			&prediction.TargetChangePercent,
			&prediction.Period,
			&prediction.Recommendation,
			&prediction.Direction,
			&prediction.JustificationText,
			&prediction.PredictedAt,
			&prediction.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan prediction row: %w", err)
		}
		predictions = append(predictions, prediction)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return predictions, nil
}
