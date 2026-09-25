package models

// CatalogCategoriesMultiple is the join table linking a catalog to the
// (multiple) categories it belongs to, in addition to its primary
// Catalogs.CategoriesID.
type CatalogCategoriesMultiple struct {
	ID           int32  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CatalogID    *int32 `gorm:"column:catalog_id" json:"catalog_id"`
	CategoriesID *int32 `gorm:"column:categories_id" json:"categories_id"`

	Catalog  *Catalogs          `gorm:"foreignKey:CatalogID;references:ID" json:"catalog,omitempty"`
	Category *CatalogCategories `gorm:"foreignKey:CategoriesID;references:ID" json:"category,omitempty"`
}

func (CatalogCategoriesMultiple) TableName() string {
	return "catalog_categories_multiple"
}
