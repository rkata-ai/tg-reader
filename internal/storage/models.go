package storage

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Channel struct {
	ID        uuid.UUID      `db:"id"`
	Username  string         `db:"username"`
	Title     sql.NullString `db:"title"`
	Link      sql.NullString `db:"link"`
	CreatedAt time.Time      `db:"created_at"`
}

type Message struct {
	ID             uuid.UUID      `db:"id"`
	TelegramID     int64          `db:"telegram_id"`
	ChannelID      uuid.UUID      `db:"channel_id"`
	Text           sql.NullString `db:"text"`
	SentAt         time.Time      `db:"sent_at"`
	SenderUsername sql.NullString `db:"sender_username"`
	IsForward      sql.NullBool   `db:"is_forward"`
	MessageType    sql.NullString `db:"message_type"`
	RawData        []byte         `db:"raw_data"` // JSONB field
	CreatedAt      time.Time      `db:"created_at"`
}

type Industry struct {
	ID        uuid.UUID `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

type Stock struct {
	ID         uuid.UUID      `db:"id"`
	Ticker     string         `db:"ticker"`
	Name       sql.NullString `db:"name"`
	IndustryID uuid.NullUUID  `db:"industry_id"`
	Exchange   sql.NullString `db:"exchange"`
	Currency   sql.NullString `db:"currency"`
	CreatedAt  time.Time      `db:"created_at"`
}

type Prediction struct {
	ID                  uuid.UUID       `db:"id"`
	MessageID           uuid.UUID       `db:"message_id"`
	StockID             uuid.UUID       `db:"stock_id"`
	PredictionType      sql.NullString  `db:"prediction_type"`
	TargetPrice         sql.NullFloat64 `db:"target_price"`
	TargetChangePercent sql.NullFloat64 `db:"target_change_percent"`
	Period              sql.NullString  `db:"period"`
	Recommendation      sql.NullString  `db:"recommendation"`
	Direction           sql.NullString  `db:"direction"`
	JustificationText   sql.NullString  `db:"justification_text"`
	PredictedAt         time.Time       `db:"predicted_at"`
	CreatedAt           time.Time       `db:"created_at"`
}
