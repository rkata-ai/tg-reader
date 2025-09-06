package storage

import (
	"context"

	"github.com/google/uuid"
)

// Storage определяет интерфейс для взаимодействия с базой данных
type Storage interface {
	// Channels
	SaveChannel(ctx context.Context, channel *Channel) error
	GetChannelByUsername(ctx context.Context, username string) (*Channel, error)
	GetChannelByID(ctx context.Context, id uuid.UUID) (*Channel, error)

	// Messages
	SaveMessage(ctx context.Context, message *Message) error
	GetMessageByID(ctx context.Context, id uuid.UUID) (*Message, error)
	GetMessagesWithoutPredictions(ctx context.Context, limit int) ([]Message, error)
	GetLastMessagesForChannel(ctx context.Context, channelID uuid.UUID, limit int) ([]Message, error)

	// Industries
	SaveIndustry(ctx context.Context, industry *Industry) error
	GetIndustryByName(ctx context.Context, name string) (*Industry, error)
	GetIndustryByID(ctx context.Context, id uuid.UUID) (*Industry, error)

	// Stocks
	SaveStock(ctx context.Context, stock *Stock) error
	GetStockByTicker(ctx context.Context, ticker string) (*Stock, error)
	GetStockByID(ctx context.Context, id uuid.UUID) (*Stock, error)

	// Predictions
	SavePrediction(ctx context.Context, prediction *Prediction) error
	GetPredictionsByMessageID(ctx context.Context, messageID uuid.UUID) ([]Prediction, error)
	GetPredictionsByStockID(ctx context.Context, stockID uuid.UUID) ([]Prediction, error)

	// Health Check
	Ping(ctx context.Context) error
	Close() error

	// Migration
	Migrate(ctx context.Context) error
}
