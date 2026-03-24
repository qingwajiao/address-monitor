package store

import (
	"context"
	"time"
	_ "time"

	"gorm.io/gorm"
)

type Subscription struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	UserID      string    `gorm:"column:user_id;not null"`
	Chain       string    `gorm:"column:chain;not null"`
	Address     string    `gorm:"column:address;not null"`
	Label       string    `gorm:"column:label"`
	CallbackURL string    `gorm:"column:callback_url;not null"`
	Secret      string    `gorm:"column:secret;not null"`
	Status      int       `gorm:"column:status;default:1"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Subscription) TableName() string { return "subscriptions" }

type SubscriptionStore struct {
	db *gorm.DB
}

func NewSubscriptionStore(db *gorm.DB) *SubscriptionStore {
	return &SubscriptionStore{db: db}
}

func (s *SubscriptionStore) Create(ctx context.Context, sub *Subscription) error {
	return s.db.WithContext(ctx).Create(sub).Error
}

// Delete 软删除，将 status 置为 0
func (s *SubscriptionStore) Delete(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&Subscription{}).
		Where("id = ?", id).
		Update("status", 0).Error
}

func (s *SubscriptionStore) GetByID(ctx context.Context, id uint64) (*Subscription, error) {
	var sub Subscription
	err := s.db.WithContext(ctx).First(&sub, id).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// ListByUser 分页查询某用户的所有订阅
func (s *SubscriptionStore) ListByUser(ctx context.Context, userID string, page, size int) ([]*Subscription, int64, error) {
	var subs []*Subscription
	var total int64
	offset := (page - 1) * size

	db := s.db.WithContext(ctx).Model(&Subscription{}).Where("user_id = ?", userID)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Offset(offset).Limit(size).Find(&subs).Error; err != nil {
		return nil, 0, err
	}
	return subs, total, nil
}

// ListByChainAddress 查询监控某地址的所有有效订阅（status=1）
func (s *SubscriptionStore) ListByChainAddress(ctx context.Context, chain, address string) ([]*Subscription, error) {
	var subs []*Subscription
	err := s.db.WithContext(ctx).
		Where("chain = ? AND address = ? AND status = 1", chain, address).
		Find(&subs).Error
	return subs, err
}

func (s *SubscriptionStore) UpdateStatus(ctx context.Context, id uint64, status int) error {
	return s.db.WithContext(ctx).
		Model(&Subscription{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// BatchCreate 批量插入，使用 GORM 的 CreateInBatches
func (s *SubscriptionStore) BatchCreate(ctx context.Context, subs []*Subscription) error {
	return s.db.WithContext(ctx).
		CreateInBatches(subs, 500).Error
}
