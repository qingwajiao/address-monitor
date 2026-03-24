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

type DispatchTask struct {
	WebhookLogID uint64                 `json:"webhook_log_id"`
	AppID        uint64                 `json:"app_id"`
	AddressID    uint64                 `json:"address_id"`
	CallbackURL  string                 `json:"callback_url"`
	Secret       string                 `json:"secret"`
	Event        parser.NormalizedEvent `json:"event"`
}

type Expander struct {
	publisher    *mq.Publisher
	addrStore    *store.WatchedAddressStore
	webhookStore *store.WebhookLogStore
	appStore     *store.AppStore
	rdb          *redis.Client
}

func NewExpander(
	publisher *mq.Publisher,
	addrStore *store.WatchedAddressStore,
	webhookStore *store.WebhookLogStore,
	appStore *store.AppStore,
	rdb *redis.Client,
) *Expander {
	return &Expander{
		publisher:    publisher,
		addrStore:    addrStore,
		webhookStore: webhookStore,
		appStore:     appStore,
		rdb:          rdb,
	}
}

func (e *Expander) Handle(msg amqp.Delivery) {
	var event parser.NormalizedEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		zap.L().Error("解析 matched.events 消息失败", zap.Error(err))
		msg.Ack(false)
		return
	}

	ctx := context.Background()

	zap.L().Debug("收到 matched.events 消息",
		zap.String("event_id", event.EventID),
		zap.String("chain", event.Chain),
		zap.String("type", string(event.EventType)),
		zap.String("address", event.WatchedAddress),
	)

	was := e.getWatchedAddresses(ctx, event.Chain, event.WatchedAddress)

	for _, wa := range was {
		// 幂等检查
		exists, err := e.webhookStore.ExistsByEventAndAddress(ctx, event.EventID, wa.ID)
		if err != nil {
			zap.L().Error("幂等检查失败", zap.Error(err))
			continue
		}
		if exists {
			continue
		}

		// 从 apps 表获取回调地址和签名密钥
		callbackURL, secret, err := e.getAppInfo(ctx, wa.AppID)
		if err != nil {
			zap.L().Error("获取 App 信息失败",
				zap.Uint64("app_id", wa.AppID),
				zap.Error(err),
			)
			continue
		}

		if callbackURL == "" {
			zap.L().Warn("App 未配置回调地址，跳过推送",
				zap.Uint64("app_id", wa.AppID),
			)
			continue
		}

		task := DispatchTask{
			AppID:       wa.AppID,
			AddressID:   wa.ID,
			CallbackURL: callbackURL,
			Secret:      secret,
			Event:       event,
		}

		payload, _ := json.Marshal(task)

		// 先写 webhook_log(pending)
		wl := &store.WebhookLog{
			EventID:     event.EventID,
			AppID:       wa.AppID,
			AddressID:   wa.ID,
			Chain:       event.Chain,
			TxHash:      event.TxHash,
			EventType:   string(event.EventType),
			Payload:     string(payload),
			Status:      "pending",
			CallbackURL: callbackURL,
		}
		if err := e.webhookStore.Create(ctx, wl); err != nil {
			zap.L().Error("写入 webhook_log 失败", zap.Error(err))
			continue
		}

		// 补充 WebhookLogID 后重新序列化
		task.WebhookLogID = wl.ID
		payload, _ = json.Marshal(task)

		// 再发 dispatch.tasks
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
			zap.Uint64("app_id", wa.AppID),
			zap.Uint64("address_id", wa.ID),
			zap.String("callback", callbackURL),
		)
	}

	msg.Ack(false)
}

func (e *Expander) getWatchedAddresses(ctx context.Context, chain, address string) []*store.WatchedAddress {
	addr := strings.ToLower(address)
	cacheKey := fmt.Sprintf("sub_cache:%s:%s", chain, addr)

	if cached, err := e.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
		var was []*store.WatchedAddress
		if json.Unmarshal(cached, &was) == nil {
			return was
		}
	}

	was, err := e.addrStore.ListByChainAddress(ctx, chain, addr)
	if err != nil || len(was) == 0 {
		return nil
	}

	if data, err := json.Marshal(was); err == nil {
		e.rdb.Set(ctx, cacheKey, data, 5*time.Minute)
	}
	return was
}

func (e *Expander) getAppInfo(ctx context.Context, appID uint64) (callbackURL, secret string, err error) {
	cacheKey := fmt.Sprintf("app_info:%d", appID)

	vals, err := e.rdb.HMGet(ctx, cacheKey, "callback_url", "secret").Result()
	if err == nil && len(vals) == 2 && vals[0] != nil && vals[1] != nil {
		return vals[0].(string), vals[1].(string), nil
	}

	app, err := e.appStore.GetByID(ctx, appID)
	if err != nil {
		return "", "", err
	}

	e.rdb.HSet(ctx, cacheKey,
		"callback_url", app.CallbackURL,
		"secret", app.Secret,
	)
	e.rdb.Expire(ctx, cacheKey, 10*time.Minute)

	return app.CallbackURL, app.Secret, nil
}

// 确保 uuid 包被使用
var _ = uuid.New
