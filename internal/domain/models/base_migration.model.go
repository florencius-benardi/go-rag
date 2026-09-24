package models

import "time"

type Migrations struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Version     string    `gorm:"column:version;uniqueIndex;not null" json:"version"`
	Description string    `gorm:"column:description" json:"description"`
	AppliedAt   time.Time `gorm:"column:applied_at;not null" json:"applied_at"`
	Checksum    string    `gorm:"column:checksum" json:"checksum"`
}

func (Migrations) TableName() string {
	return "00_migrations"
}
