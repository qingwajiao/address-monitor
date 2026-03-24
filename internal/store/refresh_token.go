package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type RefreshToken struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"column:user_id;not null"`
	TokenHash string    `gorm:"column:token_hash;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"`
	Revoked   int       `gorm:"column:revoked;default:0"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

type RefreshTokenStore struct{ db *gorm.DB }

func NewRefreshTokenStore(db *gorm.DB) *RefreshTokenStore {
	return &RefreshTokenStore{db: db}
}

func (s *RefreshTokenStore) Create(ctx context.Context, t *RefreshToken) error {
	return s.db.WithContext(ctx).Create(t).Error
}

func (s *RefreshTokenStore) GetByTokenHash(ctx context.Context, hash string) (*RefreshToken, error) {
	var t RefreshToken
	if err := s.db.WithContext(ctx).
		Where("token_hash = ? AND revoked = 0 AND expires_at > ?", hash, time.Now()).
		First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// Revoke 撤销单个 Token
func (s *RefreshTokenStore) Revoke(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&RefreshToken{}).
		Where("id = ?", id).
		Update("revoked", 1).Error
}

// RevokeAllByUser 撤销某用户所有 Token（强制下线所有设备）
func (s *RefreshTokenStore) RevokeAllByUser(ctx context.Context, userID uint64) error {
	return s.db.WithContext(ctx).
		Model(&RefreshToken{}).
		Where("user_id = ?", userID).
		Update("revoked", 1).Error
}

// DeleteExpired 清理过期 Token
func (s *RefreshTokenStore) DeleteExpired(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&RefreshToken{}).Error
}
