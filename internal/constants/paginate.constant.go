package constants

import "time"

type PaginationParams struct {
	Page      int    `json:"page"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	SortOrder string `json:"sort_order"`
	OrderBy   string `json:"order_by"`
	Total     int    `json:"total"`
	LastPage  int    `json:"last_page"`
}

type QueryParams struct {
	Q      *string    `json:"q"  form:"q"`
	Status *int16     `json:"status" form:"status"`
	Date   *time.Time `json:"date" form:"date"`
}
