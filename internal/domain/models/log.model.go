package models

import (
	"time"

	"github.com/google/uuid"
)

type TransactionLogs struct {
	ID          uuid.UUID `gorm:"type:varchar(60);primaryKey" json:"uuid"`
	ReqTime     time.Time `gorm:"column:req_time" json:"req_time"` // 2026-01-23 18:00:00
	ReqIP       string    `gorm:"column:req_ip" json:"req_ip"`     // Inbound IP
	ReqHeader   string    `gorm:"type:longtext;column:req_header" json:"req_header"`
	ReqBody     string    `gorm:"type:longtext;column:req_body" json:"req_body"`
	ReqEndPoint *string   `gorm:"column:req_end_point" json:"req_end_point"`
	ReqMethod   *string   `gorm:"column:req_method" json:"req_method"`
	ReqModule   *string   `gorm:"column:module" json:"module"`
	ReqAction   *string   `gorm:"column:action" json:"action"`
	ResURL      *string   `gorm:"column:res_end_point" json:"res_end_point"`
	ResMethod   *string   `gorm:"column:res_method" json:"res_method"`
	ResStatus   *int16    `gorm:"column:res_status" json:"res_status"`
	ResServerIP *string   `gorm:"column:res_server_ip" json:"res_server_ip"`
	ResHeader   *string   `gorm:"type:longtext;column:res_header" json:"res_header"`
	ResBody     *string   `gorm:"type:longtext;column:res_body" json:"res_body"`
	ResTime     *int64    `gorm:"column:res_time" json:"res_time"`
	Latency     *int64    `gorm:"column:latency" json:"latency"`
}

func (TransactionLogs) TableName() string {
	return "transaction_logs"
}
