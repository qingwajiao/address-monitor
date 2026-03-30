package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	ErrAppNotFound          = errors.New("应用不存在")
	ErrAppForbidden         = errors.New("无权操作此应用")
	ErrAppNameExists        = errors.New("应用名称已存在")
	ErrAppCallbackURLExists = errors.New("CallbackURL 已被其他应用使用")
)

type AppService struct {
	appStore *store.AppStore
	rdb      *redis.Client
}

func NewAppService(appStore *store.AppStore, rdb *redis.Client) *AppService {
	return &AppService{appStore: appStore, rdb: rdb}
}

func (s *AppService) Create(ctx context.Context, userID uint64, req *dto.CreateAppReq) (*dto.AppResp, error) {
	if err := s.checkDuplicate(ctx, userID, req.Name, req.CallbackURL, 0); err != nil {
		return nil, err
	}

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
	app, err := s.getOwned(ctx, userID, appID)
	if err != nil {
		return nil, err
	}
	return toAppResp(app, false), nil
}

func (s *AppService) Update(ctx context.Context, userID, appID uint64, req *dto.UpdateAppReq) (*dto.AppResp, error) {
	app, err := s.getOwned(ctx, userID, appID)
	if err != nil {
		return nil, err
	}

	if err := s.checkDuplicate(ctx, userID, req.Name, req.CallbackURL, appID); err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
		app.Name = req.Name
	}
	if req.CallbackURL != "" {
		updates["callback_url"] = req.CallbackURL
		app.CallbackURL = req.CallbackURL
	}

	if err := s.appStore.Update(ctx, appID, updates); err != nil {
		return nil, err
	}
	return toAppResp(app, false), nil
}

func (s *AppService) Delete(ctx context.Context, userID, appID uint64) error {
	if _, err := s.getOwned(ctx, userID, appID); err != nil {
		return err
	}
	return s.appStore.Delete(ctx, appID)
}

// ResetAPIKey 重置 API Key，旧 key 立即失效
func (s *AppService) ResetAPIKey(ctx context.Context, userID, appID uint64) (*dto.AppResp, error) {
	app, err := s.getOwned(ctx, userID, appID)
	if err != nil {
		return nil, err
	}

	newKey := generateAPIKey()
	if err := s.appStore.Update(ctx, appID, map[string]interface{}{"api_key": newKey}); err != nil {
		return nil, err
	}

	app.APIKey = newKey
	zap.L().Info("重置 API Key", zap.Uint64("app_id", appID))
	return toAppResp(app, true), nil
}

// ResetSecret 重置签名密钥
func (s *AppService) ResetSecret(ctx context.Context, userID, appID uint64) (*dto.AppResp, error) {
	app, err := s.getOwned(ctx, userID, appID)
	if err != nil {
		return nil, err
	}

	newSecret := generateSecret()
	if err := s.appStore.Update(ctx, appID, map[string]interface{}{"secret": newSecret}); err != nil {
		return nil, err
	}

	app.Secret = newSecret
	zap.L().Info("重置 Secret", zap.Uint64("app_id", appID))
	return toAppResp(app, true), nil
}

// UpdateAllowedContracts 更新 App 级合约白名单，同时使 Dispatcher 缓存失效
func (s *AppService) UpdateAllowedContracts(ctx context.Context, userID, appID uint64, req *dto.UpdateAllowedContractsReq) error {
	if _, err := s.getOwned(ctx, userID, appID); err != nil {
		return err
	}

	// 地址统一小写，链名统一大写
	normalized := make(map[string][]string, len(req.AllowedContracts))
	for chain, addrs := range req.AllowedContracts {
		lower := make([]string, 0, len(addrs))
		for _, a := range addrs {
			lower = append(lower, strings.ToLower(a))
		}
		normalized[strings.ToUpper(chain)] = lower
	}

	var raw string
	if len(normalized) > 0 {
		b, _ := json.Marshal(normalized)
		raw = string(b)
	}

	if err := s.appStore.Update(ctx, appID, map[string]interface{}{"allowed_contracts": raw}); err != nil {
		return err
	}

	// 使 Dispatcher 的 app_info 缓存立即失效，下次读取会重新从 DB 加载
	s.rdb.Del(ctx, fmt.Sprintf("app_info:%d", appID))

	zap.L().Info("更新 App 合约白名单",
		zap.Uint64("app_id", appID),
		zap.String("contracts", raw),
	)
	return nil
}

// checkDuplicate 检查 name 和 callbackURL 在同一用户下是否重复
// excludeID 为 0 表示 Create，非 0 表示 Update（排除自身）
func (s *AppService) checkDuplicate(ctx context.Context, userID uint64, name, callbackURL string, excludeID uint64) error {
	if name != "" {
		exists, err := s.appStore.NameExists(ctx, userID, name, excludeID)
		if err != nil {
			return err
		}
		if exists {
			return ErrAppNameExists
		}
	}
	if callbackURL != "" {
		exists, err := s.appStore.CallbackURLExists(ctx, userID, callbackURL, excludeID)
		if err != nil {
			return err
		}
		if exists {
			return ErrAppCallbackURLExists
		}
	}
	return nil
}

// getOwned 获取并校验 app 归属权
func (s *AppService) getOwned(ctx context.Context, userID, appID uint64) (*store.App, error) {
	app, err := s.appStore.GetByID(ctx, appID)
	if err != nil {
		return nil, ErrAppNotFound
	}
	if app.UserID != userID {
		return nil, ErrAppForbidden
	}
	return app, nil
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
	if app.AllowedContracts != "" {
		var m map[string][]string
		if json.Unmarshal([]byte(app.AllowedContracts), &m) == nil {
			resp.AllowedContracts = m
		}
	}
	return resp
}

func generateAPIKey() string {
	return fmt.Sprintf("ak_%s", uuid.New().String())
}

func generateSecret() string {
	return fmt.Sprintf("sk_%s", uuid.New().String())
}
