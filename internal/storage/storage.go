package storage

import (
	"context"
	// "github.com/google/uuid"
)

// Storage определяет интерфейс для взаимодействия с базой данных
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
