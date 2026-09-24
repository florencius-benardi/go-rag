package models

import "time"

type Catalogs struct {
	ID           int32      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CategoriesID int32      `gorm:"column:categories_id;not null" json:"categories_id"`
	Title        *string    `gorm:"column:title;type:varchar(255)" json:"title"`
	Intro        *string    `gorm:"column:intro;type:text" json:"intro"`
	Description  *string    `gorm:"column:description;type:text" json:"description"`
	Image        *string    `gorm:"column:image;type:varchar(255)" json:"image"`
	Price        *int32     `gorm:"column:price" json:"price"`
	Stock        int32      `gorm:"column:stock;not null;default:0" json:"stock"`
	SKU          string     `gorm:"column:sku;type:varchar(255);not null;unique" json:"sku"`
	SortID       int32      `gorm:"column:sort_id;not null;default:1" json:"sort_id"`
	IsActive     int16      `gorm:"column:is_active;not null;default:0" json:"is_active"`
	CreatedAt    *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt    *time.Time `gorm:"column:updated_at" json:"updated_at"`
	Flag         string     `gorm:"column:flag;type:varchar(255);not null" json:"flag"`

	Category *CatalogCategories `gorm:"foreignKey:CategoriesID;references:ID" json:"category,omitempty"`
}

func (Catalogs) TableName() string {
	return "catalogs"
}
