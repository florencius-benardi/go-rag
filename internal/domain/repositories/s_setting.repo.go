package repositories

import (
	"errors"
	"go-rag/internal/constants"
	"go-rag/internal/domain/models"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettingRepository interface {
	Reads(p constants.PaginationParams) ([]models.Settings, constants.PaginationParams, error)
	ShowByKey(Code string) (*models.Settings, error)
	Show(ID int64) (*models.Settings, error)
	Update(trx *gorm.DB, data *models.Settings, ID int64) error
}

type SettingRepo struct {
	*BaseRepository
}

func NewSettingRepo(db *gorm.DB) SettingRepository {
	return &SettingRepo{
		BaseRepository: NewBaseRepository(db),
	}
}

func (db *SettingRepo) Reads(p constants.PaginationParams) (
	[]models.Settings,
	constants.PaginationParams,
	error,
) {
	var settings []models.Settings
	var count int64

	dtx := db.BaseRepository.db
	dtx.Model(&models.Settings{}).Count(&count)
	dtx.Preload(clause.Associations).
		Scopes(db.Paginate(p)).
		Find(&settings)

	if dtx.Error != nil {
		return settings, p, dtx.Error
	}

	p.LastPage = int(math.Ceil(float64(count) / float64(p.Limit)))
	p.Total = int(count)
	return settings, p, nil
}

func (db *SettingRepo) Update(
	trx *gorm.DB,
	data *models.Settings,
	ID int64,
) error {
	var Setting models.Settings
	var result = trx.Model(&Setting).Where(models.Settings{ID: ID}).
		Select(
			"value",
			"updated_by_id",
		).Updates(data)

	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (r *SettingRepo) Show(ID int64) (*models.Settings, error) {
	var Setting models.Settings

	if err := r.BaseRepository.FindByUUID(&Setting, ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("Setting not found.")
		}
		return nil, err
	}

	return &Setting, nil

}

func (r *SettingRepo) ShowByKey(code string) (*models.Settings, error) {
	var Setting models.Settings

	if err := r.BaseRepository.FindOne(&Setting, models.Settings{Key: code}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("Setting not found.")
		}
		return nil, err
	}

	return &Setting, nil

}

func (r *SettingRepo) BulkUpdate(
	trx *gorm.DB,
	data []*models.Settings) error {

	err := trx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_by_id", "updated_at"}),
	}).Create(&data).Error

	return err

}
