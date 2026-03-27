package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ChainRawEvent struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	TxHash      string    `gorm:"column:tx_hash;not null"`
	BlockNumber uint64    `gorm:"column:block_number;not null"`
	BlockTime   uint32    `gorm:"column:block_time;not null"`
	EventType   string    `gorm:"column:event_type;not null"`
	RawData     string    `gorm:"column:raw_data;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 动态表名，根据链名称路由到对应的表
func tableNameForChain(chain string) string {
	return fmt.Sprintf("chain_raw_events_%s", strings.ToLower(chain))
}

type ChainRawEventStore struct{ db *gorm.DB }

func NewChainRawEventStore(db *gorm.DB) *ChainRawEventStore {
	return &ChainRawEventStore{db: db}
}

func (s *ChainRawEventStore) Insert(ctx context.Context, chain string, e *ChainRawEvent) error {
	return s.db.WithContext(ctx).
		Table(tableNameForChain(chain)).
		Create(e).Error
}

func (s *ChainRawEventStore) BatchInsert(ctx context.Context, chain string, events []*ChainRawEvent) error {
	return s.db.WithContext(ctx).
		Table(tableNameForChain(chain)).
		CreateInBatches(events, 500).Error
}

func (s *ChainRawEventStore) ListByTimeRange(ctx context.Context, chain string, from, to time.Time) ([]*ChainRawEvent, error) {
	var events []*ChainRawEvent
	err := s.db.WithContext(ctx).
		Table(tableNameForChain(chain)).
		Where("created_at BETWEEN ? AND ?", from, to).
		Order("created_at ASC").
		Find(&events).Error
	return events, err
}

func (s *ChainRawEventStore) DeleteBefore(ctx context.Context, chain string, before time.Time) (int64, error) {
	result := s.db.WithContext(ctx).
		Table(tableNameForChain(chain)).
		Where("created_at < ?", before).
		Limit(10000).
		Delete(&ChainRawEvent{})
	return result.RowsAffected, result.Error
}
