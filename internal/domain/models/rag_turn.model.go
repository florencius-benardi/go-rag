package models

import "time"

type RagTurn struct {
	ID             int64    `gorm:"primaryKey;autoIncrement"`
	RequestID      string   `gorm:"type:text;uniqueIndex;not null"`
	ConversationID string   `gorm:"type:text;index;not null"`
	UserMessage    string   `gorm:"type:text;not null"`
	Answer         string   `gorm:"type:text;not null"`
	Intent         string   `gorm:"type:text;not null"`
	ModelUsed      string   `gorm:"type:text"`
	Sources        []string `gorm:"type:jsonb;serializer:json"`
	Citations      []string `gorm:"type:jsonb;serializer:json"`
	// Trace holds either the current step objects or, for rows written before
	// the trace gained structure, a plain array of node names.
	Trace          []any     `gorm:"type:jsonb;serializer:json"`
	FallbackReason string    `gorm:"type:text"`
	CreatedAt      time.Time `gorm:"index"`
}

func (RagTurn) TableName() string { return "rag_turns" }
