package migrations

import (
	"crypto/md5"
	"fmt"
	"go-rag/internal/domain/models"
	"sort"
	"time"

	"gorm.io/gorm"
)

type Migration struct {
	Version     string
	Description string
	Up          func(db *gorm.DB) error
	Down        func(db *gorm.DB) error
}

type Migrator struct {
	db         *gorm.DB
	migrations []Migration
}

func NewMigrator(db *gorm.DB) *Migrator {
	m := &Migrator{db: db}
	m.registerMigrations()
	return m
}

func (m *Migrator) RunPending() ([]string, error) {
	if err := m.db.AutoMigrate(&models.Migrations{}); err != nil {
		return nil, fmt.Errorf("failed to create version table: %w", err)
	}
	applied := m.getAppliedVersions()

	var ran []string
	for _, migration := range m.migrations {
		if _, exists := applied[migration.Version]; exists {
			continue
		}

		fmt.Printf("  → Running migration: %s - %s\n", migration.Version, migration.Description)

		err := m.db.Transaction(func(tx *gorm.DB) error {
			if err := migration.Up(tx); err != nil {
				fmt.Printf("%s", fmt.Sprintf("FAILURE MIGRATE : %s", err.Error()))
				// m.log.Error().Msg(fmt.Sprintf("FAILURE MIGRATE : %s", err.Error()))
				return err
			}

			return tx.Create(&models.Migrations{
				Version:     migration.Version,
				Description: migration.Description,
				AppliedAt:   time.Now(),
				Checksum:    m.generateChecksum(migration.Version),
			}).Error
		})

		if err != nil {
			return ran, fmt.Errorf("migration %s failed: %w", migration.Version, err)
		}

		ran = append(ran, migration.Version)
	}
	return ran, nil
}

func (m *Migrator) Rollback() (string, error) {
	var lastVersion models.Migrations
	if err := m.db.Order("applied_at DESC").First(&lastVersion).Error; err != nil {
		return "", fmt.Errorf("no migrations to rollback")
	}

	// Find migration
	var migration *Migration
	for _, mig := range m.migrations {
		if mig.Version == lastVersion.Version {
			migration = &mig
			break
		}
	}

	if migration == nil {
		return "", fmt.Errorf("migration %s not found", lastVersion.Version)
	}

	fmt.Printf("→ Rolling back: %s\n", migration.Version)

	err := m.db.Transaction(func(tx *gorm.DB) error {
		if err := migration.Down(tx); err != nil {
			return err
		}
		return tx.Delete(&lastVersion).Error
	})

	if err != nil {
		return "", err
	}

	return migration.Version, nil
}

// =================================
// VALIDATION VERSIONING DATABASE
// =================================

func (m *Migrator) GetStatus() map[string]interface{} {
	applied := m.getAppliedVersions()

	var pending []string
	var appliedList []string

	for _, mig := range m.migrations {
		if _, exists := applied[mig.Version]; exists {
			appliedList = append(appliedList, mig.Version)
		} else {
			pending = append(pending, mig.Version)
		}
	}

	return map[string]interface{}{
		"total":         len(m.migrations),
		"applied":       len(appliedList),
		"pending":       len(pending),
		"pending_list":  pending,
		"applied_list":  appliedList,
		"current":       m.getCurrentVersion(),
		"needs_migrate": len(pending) > 0,
	}
}

func (m *Migrator) getAppliedVersions() map[string]bool {
	var versions []models.Migrations
	m.db.Find(&versions)

	applied := make(map[string]bool)
	for _, v := range versions {
		applied[v.Version] = true
	}
	return applied
}

func (m *Migrator) getCurrentVersion() string {
	var lastVersion models.Migrations
	if err := m.db.Order("applied_at DESC").First(&lastVersion).Error; err != nil {
		return ""
	}
	return lastVersion.Version
}

func (m *Migrator) generateChecksum(version string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(version)))
}

func (m *Migrator) registerMigrations() {
	m.migrations = []Migration{
		{
			Version:     "2024_01_01_000001",
			Description: "Create initial tables",
			Up:          m.migration20000101001up,
			Down:        m.migration20000101001down,
		},
		{
			Version:     "2026_09_24_000002",
			Description: "Add versioned RAG chunks and conversation turns",
			Up: func(db *gorm.DB) error {
				if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
					return err
				}
				if err := db.AutoMigrate(&models.RagChunks{}, &models.RagTurn{}); err != nil {
					return err
				}
				return db.Exec("CREATE INDEX IF NOT EXISTS idx_rag_chunks_metadata ON rag_chunks USING GIN (metadata)").Error
			},
			Down: func(db *gorm.DB) error { return db.Migrator().DropTable(&models.RagTurn{}) },
		},
	}

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})
}
