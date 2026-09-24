package models

import (
	"time"

	"gorm.io/gorm"
)

type Settings struct {
	ID        int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Sequence  int32          `gorm:"type:bigint;not null" json:"sequence"`
	Type      string         `gorm:"type:varchar(20);not null" json:"type"`
	Key       string         `gorm:"type:varchar(255);not null;unique" json:"key"`
	Name      string         `gorm:"type:varchar(255);not null" json:"name"`
	Value     *string        `gorm:"type:text" json:"value"`
	Reference string         `gorm:"type:varchar(50)" json:"reference"`
	ReadOnly  bool           `gorm:"type:boolean" json:"read_only"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Settings) TableName() string {
	return "settings"
}
