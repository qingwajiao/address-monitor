package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	Email        string    `gorm:"column:email;not null;uniqueIndex"`
	PasswordHash string    `gorm:"column:password_hash;not null"`
	Status       int       `gorm:"column:status;default:0"`
	Role         string    `gorm:"column:role;not null;default:'user'"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (User) TableName() string { return "users" }

type UserStore struct{ db *gorm.DB }

func NewUserStore(db *gorm.DB) *UserStore { return &UserStore{db: db} }

func (s *UserStore) Create(ctx context.Context, user *User) error {
	return s.db.WithContext(ctx).Create(user).Error
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserStore) GetByID(ctx context.Context, id uint64) (*User, error) {
	var user User
	if err := s.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserStore) UpdateStatus(ctx context.Context, id uint64, status int) error {
	return s.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		Update("status", status).Error
}
