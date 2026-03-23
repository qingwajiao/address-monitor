package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type DeliveryLog struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	EventID        string    `gorm:"column:event_id;not null"`
	SubscriptionID uint64    `gorm:"column:subscription_id;not null"`
	Chain          string    `gorm:"column:chain;not null"`
	TxHash         string    `gorm:"column:tx_hash;not null"`
	EventType      string    `gorm:"column:event_type;not null"`
	Payload        string    `gorm:"column:payload;not null"`
	Status         string    `gorm:"column:status;default:pending"`
	RetryCount     int       `gorm:"column:retry_count;default:0"`
	CallbackURL    string    `gorm:"column:callback_url;not null"`
	ResponseCode   *int      `gorm:"column:response_code"`
	ResponseBody   *string   `gorm:"column:response_body"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (DeliveryLog) TableName() string { return "delivery_logs" }

type DeliveryStore struct {
	db *gorm.DB
}

func NewDeliveryStore(db *gorm.DB) *DeliveryStore {
	return &DeliveryStore{db: db}
}

func (s *DeliveryStore) Create(ctx context.Context, log *DeliveryLog) error {
	return s.db.WithContext(ctx).Create(log).Error
}

// ExistsByEventAndSub 幂等检查，避免重复推送
func (s *DeliveryStore) ExistsByEventAndSub(ctx context.Context, eventID string, subID uint64) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&DeliveryLog{}).
		Where("event_id = ? AND subscription_id = ?", eventID, subID).
		Count(&count).Error
	return count > 0, err
}

func (s *DeliveryStore) UpdateStatus(ctx context.Context, id uint64, status string, code int, body string) error {
	return s.db.WithContext(ctx).
		Model(&DeliveryLog{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"response_code": code,
			"response_body": body,
		}).Error
}

func (s *DeliveryStore) IncrRetryCount(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&DeliveryLog{}).
		Where("id = ?", id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1")).Error
}

func (s *DeliveryStore) MarkDead(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&DeliveryLog{}).
		Where("id = ?", id).
		Update("status", "dead").Error
}

// ListByUser 查询某用户的推送记录，支持按链和状态过滤
func (s *DeliveryStore) ListByUser(ctx context.Context, userID string, chain string, status string, page, size int) ([]*DeliveryLog, int64, error) {
	var logs []*DeliveryLog
	var total int64
	offset := (page - 1) * size

	subQuery := s.db.Model(&Subscription{}).
		Select("id").
		Where("user_id = ?", userID)

	db := s.db.WithContext(ctx).Model(&DeliveryLog{}).
		Where("subscription_id IN (?)", subQuery)

	// 按链过滤
	if chain != "" {
		db = db.Where("chain = ?", chain)
	}
	// 按状态过滤
	if status != "" {
		db = db.Where("status = ?", status)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Order("created_at DESC").Offset(offset).Limit(size).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (s *DeliveryStore) GetByID(ctx context.Context, id uint64) (*DeliveryLog, error) {
	var log DeliveryLog
	err := s.db.WithContext(ctx).First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}
