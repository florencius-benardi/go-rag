package api_controllers

import (
	"fmt"
	"go-rag/internal/constants"
	"go-rag/internal/domain/services"
	"go-rag/internal/infrastructure/logger"
	"strconv"

	"github.com/gin-gonic/gin"
)

type BaseController struct {
	log logger.Logger
	App *AppController
}

func NewController(
	svc *services.ServiceContainer,
	log logger.Logger,
) *BaseController {
	return &BaseController{
		log: log.With().Component("WEB").Logger(),
		App: NewAppController(log),
	}
}

// parseUserID safely converts the user_id claim from JWT context to int64.
// JWT decodes JSON numbers as float64; strings are also handled for safety.
func parseUserID(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case string:
		n, err := strconv.ParseInt(val, 10, 64)
		return n, err == nil
	case int64:
		return val, true
	case int:
		return int64(val), true
	}
	return 0, false
}

func GetPaginationParams(ctx *gin.Context) constants.PaginationParams {

	limit, err := strconv.ParseInt(ctx.Query("per_page"), 10, 64)
	if err != nil {
		limit = 25
	}

	page, err := strconv.ParseInt(ctx.Query("page"), 10, 64)
	if err != nil {
		fmt.Println(err)

		page = 1
	}

	orderBy := ctx.Query("sort_field")
	if orderBy == "" {
		orderBy = "id"
	}

	sortOrder := ctx.Query("sort_order")
	if sortOrder == "" {
		sortOrder = "DESC"
	}

	offset := (page - 1) * limit

	params := constants.PaginationParams{
		Page:      int(page),
		Offset:    int(offset),
		Limit:     int(limit),
		SortOrder: sortOrder,
		OrderBy:   orderBy,
	}

	return params
}

func SuccessResponse(c *gin.Context, data interface{}, message string, httpCode int) {
	c.JSON(
		httpCode,
		gin.H{
			"status":  true,
			"message": message,
			"data":    data,
		},
	)
}

func PaginateResponse(
	c *gin.Context,
	paginate constants.PaginationParams,
	data interface{},
	message string,
	httpCode int,
) {
	pageData := map[string]interface{}{
		"per_page":   paginate.Limit,
		"last_page":  paginate.LastPage,
		"page":       paginate.Page,
		"total":      paginate.Total,
		"sort_field": paginate.OrderBy,
		"sort_order": paginate.SortOrder,
		"data":       data,
	}

	c.JSON(
		httpCode,
		gin.H{
			"status":  true,
			"message": message,
			"data":    pageData,
		},
	)
}

func ErrorResponse(c *gin.Context, errors interface{}, message string, httpCode int) {
	c.JSON(
		httpCode,
		gin.H{
			"status":  false,
			"message": message,
			"errors":  errors,
		},
	)
}
