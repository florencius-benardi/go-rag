package models

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// Halfvec maps a Postgres pgvector "halfvec" column to a slice of float32.
// GORM has no native halfvec support, so values round-trip through
// pgvector's text form "[v1,v2,...]" via Scan/Value.
type Halfvec []float32

func (v *Halfvec) Scan(value interface{}) error {
	if value == nil {
		*v = nil
		return nil
	}

	var raw string
	switch val := value.(type) {
	case string:
		raw = val
	case []byte:
		raw = string(val)
	default:
		return fmt.Errorf("halfvec: unsupported scan type %T", value)
	}

	raw = strings.Trim(raw, "[]")
	if raw == "" {
		*v = Halfvec{}
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make(Halfvec, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return fmt.Errorf("halfvec: invalid component %q: %w", p, err)
		}
		out[i] = float32(f)
	}
	*v = out
	return nil
}

func (v Halfvec) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}

	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

type RagChunks struct {
	ID             int32          `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ChunkID        *string        `gorm:"column:chunk_id;type:text;uniqueIndex" json:"chunk_id"`
	Source         string         `gorm:"column:source;type:text;not null;index" json:"source"`
	Field          string         `gorm:"column:field;type:text;index" json:"field"`
	ContentHash    string         `gorm:"column:content_hash;type:text" json:"content_hash"`
	EmbeddingModel string         `gorm:"column:embedding_model;type:text" json:"embedding_model"`
	Content        string         `gorm:"column:content;type:text;not null" json:"content"`
	Metadata       map[string]any `gorm:"column:metadata;type:jsonb;serializer:json" json:"metadata"`
	Embedding      Halfvec        `gorm:"column:embedding;type:halfvec" json:"embedding,omitempty"`
}

func (RagChunks) TableName() string {
	return "rag_chunks"
}
