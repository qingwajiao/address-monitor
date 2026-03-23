package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type RawEvent struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Chain       string    `gorm:"column:chain;not null"`
	TxHash      string    `gorm:"column:tx_hash;not null"`
	BlockNumber uint64    `gorm:"column:block_number;not null"`
	BlockTime   uint32    `gorm:"column:block_time;not null"`
	EventType   string    `gorm:"column:event_type;not null"`
	RawData     string    `gorm:"column:raw_data;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (RawEvent) TableName() string { return "raw_events" }

type RawEventStore struct {
	db *gorm.DB
}

func NewRawEventStore(db *gorm.DB) *RawEventStore {
	return &RawEventStore{db: db}
}

// Insert 写入原始事件，调用方应异步 fire-and-forget
func (s *RawEventStore) Insert(ctx context.Context, e *RawEvent) error {
	return s.db.WithContext(ctx).Create(e).Error
}

func (s *RawEventStore) ListByTimeRange(ctx context.Context, chain string, from, to time.Time) ([]*RawEvent, error) {
	var events []*RawEvent
	err := s.db.WithContext(ctx).
		Where("chain = ? AND created_at BETWEEN ? AND ?", chain, from, to).
		Order("created_at ASC").
		Find(&events).Error
	return events, err
}

// DeleteBefore 清理 N 天前的数据，分批删除避免锁表
func (s *RawEventStore) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	result := s.db.WithContext(ctx).
		Where("created_at < ?", before).
		Limit(10000).
		Delete(&RawEvent{})
	return result.RowsAffected, result.Error
}
