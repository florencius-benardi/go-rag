package models

import "time"

type MerchantCatalogs struct {
	ID          int32      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MerchantID  int32      `gorm:"column:merchant_id;not null" json:"merchant_id"`
	CatalogID   int32      `gorm:"column:catalog_id;not null" json:"catalog_id"`
	Price       int32      `gorm:"column:price;not null" json:"price"`
	IsAvailable *int16     `gorm:"column:is_available;default:1" json:"is_available"`
	IsActive    *int16     `gorm:"column:is_active;default:0" json:"is_active"`
	CreatedAt   *time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   *time.Time `gorm:"column:updated_at" json:"updated_at"`

	Catalog *Catalogs `gorm:"foreignKey:CatalogID;references:ID" json:"catalog,omitempty"`
}

func (MerchantCatalogs) TableName() string {
	return "merchant_catalogs"
}
