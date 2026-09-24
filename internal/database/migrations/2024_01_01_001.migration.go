package migrations

import (
	"go-rag/internal/domain/models"

	"gorm.io/gorm"
)

func (m *Migrator) migration20000101001up(tx *gorm.DB) error {
	if err := tx.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return err
	}
	return tx.AutoMigrate(
		&models.Settings{},
		&models.Catalogs{},
		&models.CatalogCategories{},
		&models.CatalogCategoriesMultiple{},
		&models.RagChunks{},
	)
}

func (m *Migrator) migration20000101001down(tx *gorm.DB) error {
	tables := []string{
		"settings",
	}
	for _, table := range tables {
		tx.Migrator().DropTable(table)
	}
	return nil
}

func (m *Migrator) migration20000101002up(tx *gorm.DB) error {
	return tx.AutoMigrate(
		&models.TransactionLogs{},
	)
}

func (m *Migrator) migration20000101002down(tx *gorm.DB) error {
	tables := []string{
		"settings",
	}
	for _, table := range tables {
		tx.Migrator().DropTable(table)
	}
	return nil
}
