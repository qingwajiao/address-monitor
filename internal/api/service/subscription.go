package service

import (
	"context"
	"fmt"
	"strings"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/matcher"
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type SubscriptionService struct {
	subStore *store.SubscriptionStore
	rdb      *redis.Client
}

func NewSubscriptionService(
	subStore *store.SubscriptionStore,
	rdb *redis.Client,
) *SubscriptionService {
	return &SubscriptionService{
		subStore: subStore,
		rdb:      rdb,
	}
}

// getCallbackURL 获取回调 URL，优先用请求里的，没有则读全局配置
func (s *SubscriptionService) getCallbackURL(ctx context.Context, userID, reqURL string) (string, error) {
	if reqURL != "" {
		return reqURL, nil
	}
	globalURL, err := s.rdb.Get(ctx, fmt.Sprintf("webhook:url:%s", userID)).Result()
	if err != nil || globalURL == "" {
		return "", fmt.Errorf("未设置 callback_url，且未配置全局 Webhook URL，请先调用 POST /v1/webhook/url 设置")
	}
	return globalURL, nil
}

// Create 新增单个监控地址
func (s *SubscriptionService) Create(ctx context.Context, userID string, req *dto.CreateSubReq) (*dto.SubResp, error) {
	callbackURL, err := s.getCallbackURL(ctx, userID, req.CallbackURL)
	if err != nil {
		return nil, err
	}

	sub := &store.Subscription{
		UserID:      userID,
		Chain:       strings.ToUpper(req.Chain),
		Address:     strings.ToLower(req.Address),
		Label:       req.Label,
		CallbackURL: callbackURL,
		Secret:      uuid.New().String(),
		Status:      1,
	}

	if err := s.subStore.Create(ctx, sub); err != nil {
		return nil, err
	}

	// 写 Redis 热集合
	s.rdb.SAdd(ctx, fmt.Sprintf("watch:hot:%s", sub.Chain), sub.Address)

	// 写增量日志
	s.rdb.LPush(ctx, fmt.Sprintf("bf:incremental:%s", sub.Chain), sub.Address)

	// 发 Pub/Sub 通知 Worker 更新 BF
	event := &matcher.AddressEvent{
		Type:    matcher.EventTypeAdd,
		Chain:   sub.Chain,
		Address: sub.Address,
	}
	s.rdb.Publish(ctx, matcher.AddressEventChannel, event.Encode())

	zap.L().Info("新增监控地址",
		zap.String("chain", sub.Chain),
		zap.String("address", sub.Address),
		zap.String("user_id", userID),
	)

	return toSubResp(sub, true), nil
}

// BatchCreate 批量新增监控地址
func (s *SubscriptionService) BatchCreate(ctx context.Context, userID string, req *dto.BatchCreateSubReq) (*dto.BatchCreateSubResp, error) {
	callbackURL, err := s.getCallbackURL(ctx, userID, req.CallbackURL)
	if err != nil {
		return nil, err
	}

	chain := strings.ToUpper(req.Chain)

	// 去重 + 格式化
	seen := make(map[string]struct{})
	var validAddrs []string
	for _, addr := range req.Addresses {
		addr = strings.TrimSpace(strings.ToLower(addr))
		if addr == "" {
			continue
		}
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		validAddrs = append(validAddrs, addr)
	}

	if len(validAddrs) == 0 {
		return nil, fmt.Errorf("没有有效地址")
	}

	// 分批写 MySQL（每批 500 条）
	var successList []dto.SubResp
	var failList []dto.FailItem
	const batchSize = 500

	for i := 0; i < len(validAddrs); i += batchSize {
		end := i + batchSize
		if end > len(validAddrs) {
			end = len(validAddrs)
		}
		batch := validAddrs[i:end]

		subs := make([]*store.Subscription, 0, len(batch))
		for _, addr := range batch {
			subs = append(subs, &store.Subscription{
				UserID:      userID,
				Chain:       chain,
				Address:     addr,
				Label:       req.Label,
				CallbackURL: callbackURL,
				Secret:      uuid.New().String(),
				Status:      1,
			})
		}

		if err := s.subStore.BatchCreate(ctx, subs); err != nil {
			for _, addr := range batch {
				failList = append(failList, dto.FailItem{
					Address: addr,
					Error:   err.Error(),
				})
			}
			continue
		}

		for _, sub := range subs {
			successList = append(successList, *toSubResp(sub, false))
		}
	}

	// Redis Pipeline 批量操作
	if len(successList) > 0 {
		hotKey := fmt.Sprintf("watch:hot:%s", chain)
		incrKey := fmt.Sprintf("bf:incremental:%s", chain)

		hotMembers := make([]interface{}, len(validAddrs))
		incrMembers := make([]interface{}, len(validAddrs))
		for i, addr := range validAddrs {
			hotMembers[i] = addr
			incrMembers[i] = addr
		}

		pipe := s.rdb.Pipeline()
		pipe.SAdd(ctx, hotKey, hotMembers...)
		pipe.LPush(ctx, incrKey, incrMembers...)
		pipe.LTrim(ctx, incrKey, 0, matcher.IncrementalMaxLen-1)
		pipe.Publish(ctx, matcher.AddressEventChannel, (&matcher.AddressEvent{
			Type:  matcher.EventTypeBatchAdd,
			Chain: chain,
			Count: len(validAddrs),
		}).Encode())
		if _, err := pipe.Exec(ctx); err != nil {
			zap.L().Warn("Redis Pipeline 执行失败", zap.Error(err))
		}
	}

	zap.L().Info("批量添加监控地址完成",
		zap.String("chain", chain),
		zap.String("user_id", userID),
		zap.Int("success", len(successList)),
		zap.Int("fail", len(failList)),
	)

	return &dto.BatchCreateSubResp{
		Total:   len(validAddrs),
		Success: successList,
		Fail:    failList,
	}, nil
}

// Delete 删除监控地址
func (s *SubscriptionService) Delete(ctx context.Context, userID string, id uint64) error {
	sub, err := s.subStore.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("subscription not found")
	}

	if err := s.subStore.Delete(ctx, id); err != nil {
		return err
	}

	// 从热集合移除
	s.rdb.SRem(ctx, fmt.Sprintf("watch:hot:%s", sub.Chain), sub.Address)

	// 清 Dispatcher 订阅缓存
	s.rdb.Del(ctx, fmt.Sprintf("sub_cache:%s:%s", sub.Chain, sub.Address))

	// 发 Pub/Sub 通知 Worker
	event := &matcher.AddressEvent{
		Type:    matcher.EventTypeRemove,
		Chain:   sub.Chain,
		Address: sub.Address,
	}
	s.rdb.Publish(ctx, matcher.AddressEventChannel, event.Encode())

	zap.L().Info("删除监控地址",
		zap.Uint64("sub_id", id),
		zap.String("chain", sub.Chain),
		zap.String("address", sub.Address),
	)
	return nil
}

// GetByID 查询单条
func (s *SubscriptionService) GetByID(ctx context.Context, id uint64) (*dto.SubResp, error) {
	sub, err := s.subStore.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("subscription not found")
	}
	return toSubResp(sub, false), nil
}

// List 分页查询
func (s *SubscriptionService) List(ctx context.Context, userID string, req *dto.ListSubReq) (*dto.ListSubResp, error) {
	page, size := normalizePage(req.Page, req.Size)
	subs, total, err := s.subStore.ListByUser(ctx, userID, page, size)
	if err != nil {
		return nil, err
	}

	list := make([]*dto.SubResp, 0, len(subs))
	for _, sub := range subs {
		list = append(list, toSubResp(sub, false))
	}

	return &dto.ListSubResp{
		List:  list,
		Total: total,
		Page:  page,
		Size:  size,
	}, nil
}

// ── 转换函数 ──────────────────────────────────────────────

func toSubResp(sub *store.Subscription, withSecret bool) *dto.SubResp {
	resp := &dto.SubResp{
		ID:          sub.ID,
		Chain:       sub.Chain,
		Address:     sub.Address,
		Label:       sub.Label,
		CallbackURL: sub.CallbackURL,
		Status:      sub.Status,
		CreatedAt:   sub.CreatedAt,
	}
	if withSecret {
		resp.Secret = sub.Secret // 只在创建时返回 secret
	}
	return resp
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
