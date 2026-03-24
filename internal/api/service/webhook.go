package service

import (
	"context"
	"fmt"
	"strings"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/mq"
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type WebhookService struct {
	deliveryStore *store.DeliveryStore
	publisher     *mq.Publisher
	rdb           *redis.Client
}

func NewWebhookService(
	deliveryStore *store.DeliveryStore,
	publisher *mq.Publisher,
	rdb *redis.Client,
) *WebhookService {
	return &WebhookService{
		deliveryStore: deliveryStore,
		publisher:     publisher,
		rdb:           rdb,
	}
}

// SetWebhookURL 设置全局 Webhook URL
func (s *WebhookService) SetWebhookURL(ctx context.Context, userID string, req *dto.SetWebhookURLReq) error {
	key := fmt.Sprintf("webhook:url:%s", userID)
	if err := s.rdb.Set(ctx, key, req.URL, 0).Err(); err != nil {
		return err
	}
	zap.L().Info("设置全局 Webhook URL",
		zap.String("user_id", userID),
		zap.String("url", req.URL),
	)
	return nil
}

// GetWebhookURL 查询全局 Webhook URL
func (s *WebhookService) GetWebhookURL(ctx context.Context, userID string) (*dto.WebhookURLResp, error) {
	key := fmt.Sprintf("webhook:url:%s", userID)
	url, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return &dto.WebhookURLResp{URL: ""}, nil
	}
	return &dto.WebhookURLResp{URL: url}, nil
}

// ListDeliveries 查询推送记录
func (s *WebhookService) ListDeliveries(ctx context.Context, userID string, req *dto.ListDeliveryReq) (*dto.ListDeliveryResp, error) {
	page, size := normalizePage(req.Page, req.Size)
	chain := strings.ToUpper(req.Chain)

	logs, total, err := s.deliveryStore.ListByUser(ctx, userID, chain, req.Status, page, size)
	if err != nil {
		return nil, err
	}

	list := make([]*dto.DeliveryResp, 0, len(logs))
	for _, log := range logs {
		list = append(list, toDeliveryResp(log))
	}

	return &dto.ListDeliveryResp{
		List:  list,
		Total: total,
		Page:  page,
		Size:  size,
	}, nil
}

// Resend 手动重推
func (s *WebhookService) Resend(ctx context.Context, id uint64) error {
	log, err := s.deliveryStore.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delivery log not found")
	}

	if err := s.publisher.Publish(
		"dispatch.exchange",
		"dispatch",
		[]byte(log.Payload),
		amqp.Table{"x-retry-count": int32(0)},
	); err != nil {
		return err
	}

	zap.L().Info("手动重推推送任务",
		zap.Uint64("delivery_id", id),
		zap.String("chain", log.Chain),
		zap.String("tx_hash", log.TxHash),
	)
	return nil
}

// ── 转换函数 ──────────────────────────────────────────────

func toDeliveryResp(log *store.DeliveryLog) *dto.DeliveryResp {
	return &dto.DeliveryResp{
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
