package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/mq"
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

var (
	ErrWebhookLogNotFound  = errors.New("推送记录不存在")
	ErrWebhookLogForbidden = errors.New("无权操作此记录")
)

type WebhookService struct {
	webhookStore *store.WebhookLogStore
	publisher    *mq.Publisher
	rdb          *redis.Client
}

func NewWebhookService(
	webhookStore *store.WebhookLogStore,
	publisher *mq.Publisher,
	rdb *redis.Client,
) *WebhookService {
	return &WebhookService{
		webhookStore: webhookStore,
		publisher:    publisher,
		rdb:          rdb,
	}
}

// SetWebhookURL 设置全局 Webhook URL
func (s *WebhookService) SetWebhookURL(ctx context.Context, appID uint64, req *dto.SetWebhookURLReq) error {
	key := fmt.Sprintf("webhook:url:%d", appID)
	if err := s.rdb.Set(ctx, key, req.URL, 0).Err(); err != nil {
		return err
	}
	zap.L().Info("设置 Webhook URL",
		zap.Uint64("app_id", appID),
		zap.String("url", req.URL),
	)
	return nil
}

// WebhookLogs 查询推送记录
func (s *WebhookService) ListLogs(ctx context.Context, appID uint64, req *dto.ListWebhookLogReq) (*dto.ListWebhookLogResp, error) {
	page, size := normalizePage(req.Page, req.Size)
	chain := strings.ToUpper(req.Chain)

	logs, total, err := s.webhookStore.ListByApp(ctx, appID, chain, req.Status, page, size)
	if err != nil {
		return nil, err
	}

	list := make([]*dto.WebhookLogResp, 0, len(logs))
	for _, log := range logs {
		list = append(list, toWebhookLogResp(log))
	}

	return &dto.ListWebhookLogResp{
		List:  list,
		Total: total,
		Page:  page,
		Size:  size,
	}, nil
}

// GetWebhookURL 查询全局 Webhook URL
func (s *WebhookService) GetWebhookURL(ctx context.Context, appID uint64) (*dto.WebhookURLResp, error) {
	key := fmt.Sprintf("webhook:url:%d", appID)
	url, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return &dto.WebhookURLResp{URL: ""}, nil
	}
	return &dto.WebhookURLResp{URL: url}, nil
}

// Resend 加上 appID 参数做鉴权
func (s *WebhookService) Resend(ctx context.Context, appID, id uint64) error {
	log, err := s.webhookStore.GetByID(ctx, id)
	if err != nil {
		return ErrWebhookLogNotFound
	}
	if log.AppID != appID {
		return ErrWebhookLogForbidden
	}

	if err := s.publisher.Publish(
		"dispatch.exchange",
		"dispatch",
		[]byte(log.Payload),
		amqp.Table{"x-retry-count": int32(0)},
	); err != nil {
		return err
	}

	zap.L().Info("手动重推",
		zap.Uint64("webhook_log_id", id),
		zap.String("chain", log.Chain),
		zap.String("tx_hash", log.TxHash),
	)
	return nil
}

// ── 转换函数 ──────────────────────────────────────────────

func toWebhookLogResp(log *store.WebhookLog) *dto.WebhookLogResp {
	return &dto.WebhookLogResp{
		ID:           log.ID,
		EventID:      log.EventID,
		Chain:        log.Chain,
		TxHash:       log.TxHash,
		EventType:    log.EventType,
		Status:       log.Status,
		RetryCount:   log.RetryCount,
		CallbackURL:  log.CallbackURL,
		ResponseCode: log.ResponseCode,
		CreatedAt:    log.CreatedAt,
	}
}
