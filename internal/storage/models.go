package storage

import (
	"database/sql"
	"time"
)

type Channel struct {
	ID        int64          `db:"id"`
	Username  string         `db:"username"`
	Title     sql.NullString `db:"title"`
	Link      sql.NullString `db:"link"`
	CreatedAt time.Time      `db:"created_at"`
}

type Message struct {
	ID             int64          `db:"id"`
	TelegramID     int64          `db:"telegram_id"`
	ChannelID      int64          `db:"channel_id"`
	Text           sql.NullString `db:"text"`
	SentAt         time.Time      `db:"sent_at"`
	SenderUsername sql.NullString `db:"sender_username"`
	IsForward      sql.NullBool   `db:"is_forward"`
	MessageType    sql.NullString `db:"message_type"`
	RawData        []byte         `db:"raw_data"` // JSONB field
	CreatedAt      time.Time      `db:"created_at"`
}

