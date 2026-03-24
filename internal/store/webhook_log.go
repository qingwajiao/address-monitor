package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type WebhookLog struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	EventID      string    `gorm:"column:event_id;not null"`
	AppID        uint64    `gorm:"column:app_id;not null"`
	AddressID    uint64    `gorm:"column:address_id;not null"`
	Chain        string    `gorm:"column:chain;not null"`
	TxHash       string    `gorm:"column:tx_hash;not null"`
	EventType    string    `gorm:"column:event_type;not null"`
	Payload      string    `gorm:"column:payload;not null"`
	Status       string    `gorm:"column:status;default:pending"`
	RetryCount   int       `gorm:"column:retry_count;default:0"`
	CallbackURL  string    `gorm:"column:callback_url;not null"`
	ResponseCode *int      `gorm:"column:response_code"`
	ResponseBody *string   `gorm:"column:response_body"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (WebhookLog) TableName() string { return "webhook_logs" }

type WebhookLogStore struct{ db *gorm.DB }

func NewWebhookLogStore(db *gorm.DB) *WebhookLogStore {
	return &WebhookLogStore{db: db}
}

func (s *WebhookLogStore) Create(ctx context.Context, log *WebhookLog) error {
	return s.db.WithContext(ctx).Create(log).Error
}

func (s *WebhookLogStore) ExistsByEventAndAddress(ctx context.Context, eventID string, addressID uint64) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&WebhookLog{}).
		Where("event_id = ? AND address_id = ?", eventID, addressID).
		Count(&count).Error
	return count > 0, err
}

func (s *WebhookLogStore) GetByID(ctx context.Context, id uint64) (*WebhookLog, error) {
	var log WebhookLog
	if err := s.db.WithContext(ctx).First(&log, id).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

func (s *WebhookLogStore) UpdateStatus(ctx context.Context, id uint64, status string, code int, body string) error {
	return s.db.WithContext(ctx).
		Model(&WebhookLog{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"response_code": code,
			"response_body": body,
		}).Error
}

func (s *WebhookLogStore) IncrRetryCount(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&WebhookLog{}).
		Where("id = ?", id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1")).Error
}

func (s *WebhookLogStore) MarkDead(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&WebhookLog{}).
		Where("id = ?", id).
		Update("status", "dead").Error
}

func (s *WebhookLogStore) ListByApp(ctx context.Context, appID uint64, chain, status string, page, size int) ([]*WebhookLog, int64, error) {
	var logs []*WebhookLog
	var total int64
	offset := (page - 1) * size

	db := s.db.WithContext(ctx).Model(&WebhookLog{}).
		Where("app_id = ?", appID)
	if chain != "" {
		db = db.Where("chain = ?", chain)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Order("created_at DESC").
		Offset(offset).Limit(size).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
