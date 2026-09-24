package seeders

import (
	"gorm.io/gorm"
)

type Seeder struct {
	db *gorm.DB
}

func NewSeeder(db *gorm.DB) *Seeder {
	return &Seeder{db: db}
}

func (s *Seeder) RunAll() error {
	seeders := []func() error{}
	for _, seeder := range seeders {
		if err := seeder(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) isEmpty(model interface{}) bool {
	var count int64
	s.db.Model(model).Count(&count)
	return count == 0
}
