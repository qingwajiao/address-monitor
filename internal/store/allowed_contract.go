package store

import (
	"context"
	"strings"
	"time"

	"address-monitor/pkg/addrcodec"

	"gorm.io/gorm"
)

type AllowedContract struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement"`
	Chain           string    `gorm:"column:chain;not null"`
	ContractAddress string    `gorm:"column:contract_address;not null"`
	Symbol          string    `gorm:"column:symbol;not null;default:''"`
	Decimals        int       `gorm:"column:decimals;not null;default:0"`
	Enabled         int       `gorm:"column:enabled;default:1"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (AllowedContract) TableName() string { return "allowed_contracts" }

type AllowedContractStore struct{ db *gorm.DB }

func NewAllowedContractStore(db *gorm.DB) *AllowedContractStore {
	return &AllowedContractStore{db: db}
}

// ListEnabled 返回所有启用的合约白名单（供 Worker 使用）
func (s *AllowedContractStore) ListEnabled(ctx context.Context) ([]*AllowedContract, error) {
	var contracts []*AllowedContract
	err := s.db.WithContext(ctx).
		Where("enabled = 1").
		Find(&contracts).Error
	return contracts, err
}

// ListAll 返回全部合约（含禁用，供 Admin API 使用）
func (s *AllowedContractStore) ListAll(ctx context.Context) ([]*AllowedContract, error) {
	var contracts []*AllowedContract
	err := s.db.WithContext(ctx).Order("id asc").Find(&contracts).Error
	return contracts, err
}

func (s *AllowedContractStore) Update(ctx context.Context, id uint64, updates map[string]interface{}) error {
	return s.db.WithContext(ctx).
		Model(&AllowedContract{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (s *AllowedContractStore) Create(ctx context.Context, c *AllowedContract) error {
	c.Chain = strings.ToUpper(c.Chain)
	c.ContractAddress = addrcodec.Get(c.Chain).Normalize(c.ContractAddress)
	return s.db.WithContext(ctx).Create(c).Error
}

func (s *AllowedContractStore) SetEnabled(ctx context.Context, id uint64, enabled int) error {
	return s.db.WithContext(ctx).
		Model(&AllowedContract{}).
		Where("id = ?", id).
		Update("enabled", enabled).Error
}

func (s *AllowedContractStore) Delete(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).Delete(&AllowedContract{}, id).Error
}
