package service

import (
	"context"
	"errors"
	"fmt"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/store"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrAppNotFound  = errors.New("应用不存在")
	ErrAppForbidden = errors.New("无权操作此应用")
)

type AppService struct {
	appStore *store.AppStore
}

func NewAppService(appStore *store.AppStore) *AppService {
	return &AppService{appStore: appStore}
}

func (s *AppService) Create(ctx context.Context, userID uint64, req *dto.CreateAppReq) (*dto.AppResp, error) {
	app := &store.App{
		UserID:      userID,
		Name:        req.Name,
		APIKey:      generateAPIKey(),
		Secret:      generateSecret(),
		CallbackURL: req.CallbackURL,
		Status:      1,
	}

	if err := s.appStore.Create(ctx, app); err != nil {
		return nil, err
	}

	zap.L().Info("创建应用",
		zap.Uint64("user_id", userID),
		zap.Uint64("app_id", app.ID),
		zap.String("name", app.Name),
	)

	return toAppResp(app, true), nil
}

func (s *AppService) List(ctx context.Context, userID uint64) ([]*dto.AppResp, error) {
	apps, err := s.appStore.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	list := make([]*dto.AppResp, 0, len(apps))
	for _, app := range apps {
		list = append(list, toAppResp(app, false))
	}
	return list, nil
}

func (s *AppService) Get(ctx context.Context, userID, appID uint64) (*dto.AppResp, error) {
	app, err := s.appStore.GetByID(ctx, appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, err
	}
	if app.UserID != userID {
		return nil, ErrAppForbidden
	}
	return toAppResp(app, false), nil
}

func (s *AppService) Update(ctx context.Context, userID, appID uint64, req *dto.UpdateAppReq) (*dto.AppResp, error) {
	app, err := s.appStore.GetByID(ctx, appID)
	if err != nil {
		return nil, ErrAppNotFound
	}
	if app.UserID != userID {
		return nil, ErrAppForbidden
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.CallbackURL != "" {
		updates["callback_url"] = req.CallbackURL
	}

	if err := s.appStore.Update(ctx, appID, updates); err != nil {
		return nil, err
	}

	// 更新缓存失效
	app.Name = req.Name
	app.CallbackURL = req.CallbackURL
	return toAppResp(app, false), nil
}

func (s *AppService) Delete(ctx context.Context, userID, appID uint64) error {
	app, err := s.appStore.GetByID(ctx, appID)
	if err != nil {
		return ErrAppNotFound
	}
	if app.UserID != userID {
		return ErrAppForbidden
	}
	return s.appStore.Delete(ctx, appID)
}

// ResetAPIKey 重置 API Key，旧 key 立即失效
func (s *AppService) ResetAPIKey(ctx context.Context, userID, appID uint64) (*dto.AppResp, error) {
	app, err := s.appStore.GetByID(ctx, appID)
	if err != nil {
		return nil, ErrAppNotFound
	}
	if app.UserID != userID {
		return nil, ErrAppForbidden
	}

	newKey := generateAPIKey()
	if err := s.appStore.Update(ctx, appID, map[string]interface{}{
		"api_key": newKey,
	}); err != nil {
		return nil, err
	}

	app.APIKey = newKey
	zap.L().Info("重置 API Key",
		zap.Uint64("app_id", appID),
	)
	return toAppResp(app, true), nil
}

// ResetSecret 重置签名密钥
func (s *AppService) ResetSecret(ctx context.Context, userID, appID uint64) (*dto.AppResp, error) {
	app, err := s.appStore.GetByID(ctx, appID)
	if err != nil {
		return nil, ErrAppNotFound
	}
	if app.UserID != userID {
		return nil, ErrAppForbidden
	}

	newSecret := generateSecret()
	if err := s.appStore.Update(ctx, appID, map[string]interface{}{
		"secret": newSecret,
	}); err != nil {
		return nil, err
	}

	app.Secret = newSecret
	zap.L().Info("重置 Secret",
		zap.Uint64("app_id", appID),
	)
	return toAppResp(app, true), nil
}

// ── 工具函数 ──────────────────────────────────────────────

func toAppResp(app *store.App, withSecret bool) *dto.AppResp {
	resp := &dto.AppResp{
		ID:          app.ID,
		Name:        app.Name,
		APIKey:      app.APIKey,
		CallbackURL: app.CallbackURL,
		Status:      app.Status,
		CreatedAt:   app.CreatedAt,
	}
	if withSecret {
		resp.Secret = app.Secret
	}
	return resp
}

func generateAPIKey() string {
	return fmt.Sprintf("ak_%s", uuid.New().String())
}

func generateSecret() string {
	return fmt.Sprintf("sk_%s", uuid.New().String())
}
