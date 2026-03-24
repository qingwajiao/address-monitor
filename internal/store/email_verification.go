package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type EmailVerification struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"column:user_id;not null"`
	Token     string    `gorm:"column:token;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"`
	Used      int       `gorm:"column:used;default:0"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (EmailVerification) TableName() string { return "email_verifications" }

type EmailVerificationStore struct{ db *gorm.DB }

func NewEmailVerificationStore(db *gorm.DB) *EmailVerificationStore {
	return &EmailVerificationStore{db: db}
}

func (s *EmailVerificationStore) Create(ctx context.Context, v *EmailVerification) error {
	return s.db.WithContext(ctx).Create(v).Error
}

func (s *EmailVerificationStore) GetByToken(ctx context.Context, token string) (*EmailVerification, error) {
	var v EmailVerification
	if err := s.db.WithContext(ctx).Where("token = ?", token).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *EmailVerificationStore) MarkUsed(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&EmailVerification{}).
		Where("id = ?", id).
		Update("used", 1).Error
}

// DeleteExpired 清理过期的验证记录
func (s *EmailVerificationStore) DeleteExpired(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Where("expires_at < ? OR used = 1", time.Now()).
		Delete(&EmailVerification{}).Error
}
