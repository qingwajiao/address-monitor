package store

import (
	"context"
	"time"
	_ "time"

	"gorm.io/gorm"
)

type ChainSyncStatus struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	Chain      string    `gorm:"column:chain;not null"`
	InstanceID string    `gorm:"column:instance_id;not null"`
	LastBlock  uint64    `gorm:"column:last_block;default:0"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ChainSyncStatus) TableName() string { return "chain_sync_status" }

type ChainSyncStore struct {
	db *gorm.DB
}

func NewChainSyncStore(db *gorm.DB) *ChainSyncStore {
	return &ChainSyncStore{db: db}
}

// GetLastBlock 获取某条链某实例的最后处理块号
func (s *ChainSyncStore) GetLastBlock(ctx context.Context, chain, instanceID string) (uint64, error) {
	var status ChainSyncStatus
	err := s.db.WithContext(ctx).
		Where("chain = ? AND instance_id = ?", chain, instanceID).
		First(&status).Error
	if err != nil {
		return 0, err
	}
	return status.LastBlock, nil
}

// UpsertLastBlock 插入或更新块号
func (s *ChainSyncStore) UpsertLastBlock(ctx context.Context, chain, instanceID string, blockNum uint64) error {
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO chain_sync_status (chain, instance_id, last_block)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE last_block = ?, updated_at = NOW()
	`, chain, instanceID, blockNum, blockNum).Error
}
