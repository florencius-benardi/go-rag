package models

import "time"

type CatalogCategories struct {
	ID          int32      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ParentID    int32      `gorm:"column:parent_id;not null;default:0" json:"parent_id"`
	HotMenu     int32      `gorm:"column:hot_menu;not null;default:0" json:"hot_menu"`
	Image       *string    `gorm:"column:image;type:varchar(255)" json:"image"`
	Title       *string    `gorm:"column:title;type:varchar(255)" json:"title"`
	SortID      int32      `gorm:"column:sort_id;not null;default:1" json:"sort_id"`
	IsActive    int16      `gorm:"column:is_active;not null;default:0" json:"is_active"`
	CreatedAt   *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   *time.Time `gorm:"column:updated_at" json:"updated_at"`
	Description string     `gorm:"column:description;type:text;not null" json:"description"`
	Flag        string     `gorm:"column:flag;type:varchar(255);not null;default:'none'" json:"flag"`
}

func (CatalogCategories) TableName() string {
	return "catalog_categories"
}
