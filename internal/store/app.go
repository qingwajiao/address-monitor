package store

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type App struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	UserID      uint64    `gorm:"column:user_id;not null"`
	Name        string    `gorm:"column:name;not null"`
	APIKey      string    `gorm:"column:api_key;not null;uniqueIndex"`
	Secret      string    `gorm:"column:secret;not null"`
	CallbackURL string    `gorm:"column:callback_url;not null;default:''"`
	Status      int       `gorm:"column:status;default:1"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (App) TableName() string { return "apps" }

type AppStore struct{ db *gorm.DB }

func NewAppStore(db *gorm.DB) *AppStore { return &AppStore{db: db} }

func (s *AppStore) Create(ctx context.Context, app *App) error {
	return s.db.WithContext(ctx).Create(app).Error
}

func (s *AppStore) GetByID(ctx context.Context, id uint64) (*App, error) {
	var app App
	if err := s.db.WithContext(ctx).First(&app, id).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (s *AppStore) GetByAPIKey(ctx context.Context, apiKey string) (*App, error) {
	var app App
	if err := s.db.WithContext(ctx).
		Where("api_key = ? AND status = 1", apiKey).
		First(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (s *AppStore) ListByUserID(ctx context.Context, userID uint64) ([]*App, error) {
	var apps []*App
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND status = 1", userID).
		Order("created_at DESC").
		Find(&apps).Error; err != nil {
		return nil, err
	}
	return apps, nil
}

func (s *AppStore) Update(ctx context.Context, id uint64, updates map[string]interface{}) error {
	return s.db.WithContext(ctx).
		Model(&App{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (s *AppStore) Delete(ctx context.Context, id uint64) error {
	return s.db.WithContext(ctx).
		Model(&App{}).
		Where("id = ?", id).
		Update("status", 0).Error
}

// NameExists 检查同一用户下是否已有同名应用，excludeID 用于 Update 时排除自身
func (s *AppStore) NameExists(ctx context.Context, userID uint64, name string, excludeID uint64) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&App{}).
		Where("user_id = ? AND name = ? AND status = 1 AND id != ?", userID, name, excludeID).
		Count(&count).Error
	return count > 0, err
}

// CallbackURLExists 检查同一用户下是否已有相同 CallbackURL 的应用，excludeID 用于 Update 时排除自身
func (s *AppStore) CallbackURLExists(ctx context.Context, userID uint64, url string, excludeID uint64) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&App{}).
		Where("user_id = ? AND callback_url = ? AND status = 1 AND id != ?", userID, url, excludeID).
		Count(&count).Error
	return count > 0, err
}
