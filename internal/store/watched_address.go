package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type WatchedAddress struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	AppID     uint64    `gorm:"column:app_id;not null"`
	Chain     string    `gorm:"column:chain;not null"`
	Address   string    `gorm:"column:address;not null"`
	Label     string    `gorm:"column:label;not null;default:''"`
	Status    int       `gorm:"column:status;default:1"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (WatchedAddress) TableName() string { return "watched_addresses" }

type WatchedAddressStore struct{ db *gorm.DB }

func NewWatchedAddressStore(db *gorm.DB) *WatchedAddressStore {
	return &WatchedAddressStore{db: db}
}

func (s *WatchedAddressStore) Create(ctx context.Context, wa *WatchedAddress) error {
	return s.db.WithContext(ctx).Create(wa).Error
}

func (s *WatchedAddressStore) BatchCreate(ctx context.Context, was []*WatchedAddress) error {
	return s.db.WithContext(ctx).CreateInBatches(was, 500).Error
}

func (s *WatchedAddressStore) Delete(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&WatchedAddress{}).
		Where("id = ?", id).
		Update("status", 0).Error
}

func (s *WatchedAddressStore) GetByID(ctx context.Context, id uint64) (*WatchedAddress, error) {
	var wa WatchedAddress
	if err := s.db.WithContext(ctx).First(&wa, id).Error; err != nil {
		return nil, err
	}
	return &wa, nil
}

func (s *WatchedAddressStore) ListByApp(ctx context.Context, appID uint64, chain string, page, size int) ([]*WatchedAddress, int64, error) {
	var was []*WatchedAddress
	var total int64
	offset := (page - 1) * size

	db := s.db.WithContext(ctx).Model(&WatchedAddress{}).
		Where("app_id = ? AND status = 1", appID)

	if appID > 0 {
		db = db.Where("app_id = ?", appID) // appID=0 时查全部
	}
	if chain != "" {
		db = db.Where("chain = ?", chain)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Offset(offset).Limit(size).
		Order("created_at DESC").Find(&was).Error; err != nil {
		return nil, 0, err
	}
	return was, total, nil
}

// ListByChainAddress 地址匹配时使用，查询命中的所有有效监控记录
func (s *WatchedAddressStore) ListByChainAddress(ctx context.Context, chain, address string) ([]*WatchedAddress, error) {
	var was []*WatchedAddress
	if err := s.db.WithContext(ctx).
		Where("chain = ? AND address = ? AND status = 1", chain, address).
		Find(&was).Error; err != nil {
		return nil, err
	}
	return was, nil
}
