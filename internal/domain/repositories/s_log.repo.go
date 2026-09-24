package repositories

import (
	"errors"
	"go-rag/internal/constants"
	"go-rag/internal/domain/models"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LogFilter mirrors validated query filters for the list page.
// Fields are optional; zero values mean "no filter on that dimension".
type LogFilter struct {
	Search       string
	Methods      []string
	StatusGroups []string
	Module       string
	Action       string
	DateFrom     string
	DateTo       string
}

type LogRepository interface {
	Reads(p constants.PaginationParams, f LogFilter) ([]models.TransactionLogs, constants.PaginationParams, error)
	Show(ID string) (*models.TransactionLogs, error)
	Insert(log *models.TransactionLogs) error
	UpdateResponse(id string, fields map[string]interface{}) error
	DeleteOlderThan(days int) (int64, error)
}

// allowedLogSortFields whitelists columns the client may sort by. Anything else
// is silently rejected to keep the query injection-safe.
var allowedLogSortFields = map[string]string{
	"req_time":   "req_time",
	"created_at": "req_time",
	"res_status": "res_status",
	"latency":    "latency",
	"req_method": "req_method",
	"module":     "module",
	"action":     "action",
}

type LogRepo struct {
	*BaseRepository
}

func NewLogRepo(db *gorm.DB) LogRepository {
	return &LogRepo{
		BaseRepository: NewBaseRepository(db),
	}
}

func (r *LogRepo) Reads(p constants.PaginationParams, f LogFilter) (
	[]models.TransactionLogs,
	constants.PaginationParams,
	error,
) {
	var rows []models.TransactionLogs
	var count int64

	q := r.BaseRepository.db.Model(&models.TransactionLogs{})
	q = applyLogFilters(q, f)

	if err := q.Count(&count).Error; err != nil {
		return rows, p, err
	}

	sortCol, ok := allowedLogSortFields[strings.ToLower(p.OrderBy)]
	if !ok {
		sortCol = "req_time"
	}
	sortDir := strings.ToUpper(p.SortOrder)
	if sortDir != "ASC" && sortDir != "DESC" {
		sortDir = "DESC"
	}

	if err := q.Preload(clause.Associations).
		Order(sortCol + " " + sortDir).
		Offset(p.Offset).
		Limit(p.Limit).
		Find(&rows).Error; err != nil {
		return rows, p, err
	}

	if p.Limit > 0 {
		p.LastPage = int(math.Ceil(float64(count) / float64(p.Limit)))
	}
	p.Total = int(count)
	return rows, p, nil
}

// applyLogFilters layers the LogFilter onto the GORM query. Each clause is
// added only when the corresponding filter is set, so an empty filter returns
// every row.
func applyLogFilters(q *gorm.DB, f LogFilter) *gorm.DB {
	if f.Search != "" {
		like := "%" + f.Search + "%"
		q = q.Where("req_end_point LIKE ?", like)
	}
	if len(f.Methods) > 0 {
		q = q.Where("req_method IN ?", f.Methods)
	}
	if len(f.StatusGroups) > 0 {
		var clauses []string
		var args []interface{}
		for _, g := range f.StatusGroups {
			switch g {
			case "2xx":
				clauses = append(clauses, "(res_status >= ? AND res_status < ?)")
				args = append(args, 200, 300)
			case "3xx":
				clauses = append(clauses, "(res_status >= ? AND res_status < ?)")
				args = append(args, 300, 400)
			case "4xx":
				clauses = append(clauses, "(res_status >= ? AND res_status < ?)")
				args = append(args, 400, 500)
			case "5xx":
				clauses = append(clauses, "(res_status >= ? AND res_status < ?)")
				args = append(args, 500, 600)
			}
		}
		if len(clauses) > 0 {
			q = q.Where(strings.Join(clauses, " OR "), args...)
		}
	}
	if f.Module != "" {
		q = q.Where("module = ?", f.Module)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.DateFrom != "" {
		q = q.Where("req_time >= ?", f.DateFrom+" 00:00:00")
	}
	if f.DateTo != "" {
		q = q.Where("req_time <= ?", f.DateTo+" 23:59:59")
	}
	return q
}

func (r *LogRepo) Show(ID string) (*models.TransactionLogs, error) {
	var Log models.TransactionLogs
	if err := r.BaseRepository.FindByUUID(&Log, ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("Log not found.")
		}
		return nil, err
	}

	return &Log, nil

}

func (r *LogRepo) Insert(log *models.TransactionLogs) error {
	return r.BaseRepository.db.Create(log).Error
}

func (r *LogRepo) UpdateResponse(id string, fields map[string]interface{}) error {
	return r.BaseRepository.db.
		Model(&models.TransactionLogs{}).
		Where("id = ?", id).
		Updates(fields).Error
}

func (r *LogRepo) DeleteOlderThan(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	result := r.BaseRepository.db.
		Where("req_time < ?", cutoff).
		Delete(&models.TransactionLogs{})
	return result.RowsAffected, result.Error
}
