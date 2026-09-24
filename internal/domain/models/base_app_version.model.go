package models

import (
	"time"
)

type AppVersions struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Version   string    `gorm:"column:version;not null" json:"version"`
	DBVersion string    `gorm:"column:db_version;not null" json:"db_version"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AppVersions) TableName() string {
	return "00_app_versions"
}
