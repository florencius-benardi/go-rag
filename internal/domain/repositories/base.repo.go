package repositories

import (
	"go-rag/internal/constants"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BaseRepository struct {
	db *gorm.DB
}
type RepositoriesContainer struct {
	SettingRepo *SettingRepository
}

func NewBaseRepository(db *gorm.DB) *BaseRepository {
	return &BaseRepository{
		db: db,
	}
}

func (r *BaseRepository) GetDB(ctx *gin.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *BaseRepository) Paginate(p constants.PaginationParams) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(p.Offset).Limit(p.Limit).Order(p.OrderBy + " " + strings.ToUpper(p.SortOrder))
	}
}

func (r *BaseRepository) Create(tx *gorm.DB, model interface{}) error {
	return tx.Preload(clause.Associations).Create(model).Error
}

func (r *BaseRepository) Update(tx *gorm.DB, model interface{}) error {
	return tx.Preload(clause.Associations).Save(model).Error
}

func (r *BaseRepository) FindByUUID(model interface{}, id interface{}) error {
	return r.db.Preload(clause.Associations).First(model, id).Error
}

func (r *BaseRepository) FindOne(model interface{}, conditions interface{}) error {
	return r.db.Preload(clause.Associations).Where(conditions).First(model).Error
}

func (r *BaseRepository) RawQuery(query string, params interface{}, accumulator interface{}) (*gorm.DB, error, interface{}) {
	result := r.db.Raw(query, params).
		Scan(&accumulator)

	if result.Error != nil {
		return nil, result.Error, accumulator
	}

	return nil, nil, accumulator
}

func (r *BaseRepository) WithTransaction(fn func(tx *gorm.DB) error) error {
	tx := r.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}
