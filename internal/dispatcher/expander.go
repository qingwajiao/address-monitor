package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"address-monitor/internal/mq"
	"address-monitor/internal/parser"
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Expander struct {
	publisher     *mq.Publisher
	subStore      *store.SubscriptionStore
	deliveryStore *store.DeliveryStore
	rdb           *redis.Client
}

func NewExpander(
	publisher *mq.Publisher,
	subStore *store.SubscriptionStore,
	deliveryStore *store.DeliveryStore,
	rdb *redis.Client,
) *Expander {
	return &Expander{
		publisher:     publisher,
		subStore:      subStore,
		deliveryStore: deliveryStore,
		rdb:           rdb,
	}
}

// DispatchTask 推送任务，写入 dispatch.tasks 队列
type DispatchTask struct {
	TaskID         string                 `json:"task_id"`
	SubscriptionID uint64                 `json:"subscription_id"`
	CallbackURL    string                 `json:"callback_url"`
	Secret         string                 `json:"secret"`
	Event          parser.NormalizedEvent `json:"event"`
}

func (e *Expander) Handle(msg amqp.Delivery) {
	var event parser.NormalizedEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		zap.L().Error("解析 matched.events 消息失败", zap.Error(err))
		msg.Ack(false)
		return
	}

	zap.L().Debug("收到 matched.events 消息",
		zap.String("event_id", event.EventID),
		zap.String("chain", event.Chain),
		zap.String("type", string(event.EventType)),
		zap.String("address", event.WatchedAddress),
	)

	ctx := context.Background()
	subs := e.getSubscriptions(ctx, event.Chain, event.WatchedAddress)

	for _, sub := range subs {
		// 幂等检查
		exists, err := e.deliveryStore.ExistsByEventAndSub(ctx, event.EventID, sub.ID)
		if err != nil {
			zap.L().Error("幂等检查失败", zap.Error(err))
			continue
		}
		if exists {
			zap.L().Debug("事件已处理，跳过",
				zap.String("event_id", event.EventID),
				zap.Uint64("sub_id", sub.ID),
			)
			continue
		}

		task := DispatchTask{
			TaskID:         uuid.New().String(),
			SubscriptionID: sub.ID,
			CallbackURL:    sub.CallbackURL,
			Secret:         sub.Secret,
			Event:          event,
		}

		payload, _ := json.Marshal(task)

		// 先写 delivery_log(pending)，再发 MQ
		if err := e.deliveryStore.Create(ctx, &store.DeliveryLog{
			EventID:        event.EventID,
			SubscriptionID: sub.ID,
			Status:         "pending",
			Payload:        string(payload),
			CallbackURL:    sub.CallbackURL,
			Chain:          event.Chain,
			TxHash:         event.TxHash,
			EventType:      string(event.EventType),
		}); err != nil {
			zap.L().Error("写入 delivery_log 失败", zap.Error(err))
			continue
		}

		if err := e.publisher.Publish(
			"dispatch.exchange",
			"dispatch",
			payload,
			amqp.Table{"x-retry-count": int32(0)},
		); err != nil {
			zap.L().Error("发布 dispatch.tasks 失败",
				zap.String("event_id", event.EventID),
				zap.Error(err),
			)
			continue
		}

		zap.L().Info("推送任务已创建",
			zap.String("event_id", event.EventID),
			zap.Uint64("sub_id", sub.ID),
			zap.String("callback", sub.CallbackURL),
		)
	}

	msg.Ack(false)
}

// getSubscriptions 先查 Redis 缓存，未命中再查 MySQL
func (e *Expander) getSubscriptions(ctx context.Context, chain, address string) []*store.Subscription {
	addr := strings.ToLower(address)
	cacheKey := fmt.Sprintf("sub_cache:%s:%s", chain, addr)

	// 查缓存
	if cached, err := e.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
		var subs []*store.Subscription
		if json.Unmarshal(cached, &subs) == nil {
			return subs
		}
	}

	// 查 MySQL
	subs, err := e.subStore.ListByChainAddress(ctx, chain, addr)
	if err != nil || len(subs) == 0 {
		return nil
	}

	// 写缓存，TTL 5 分钟
	if data, err := json.Marshal(subs); err == nil {
		e.rdb.Set(ctx, cacheKey, data, 5*time.Minute)
	}

	return subs
}
