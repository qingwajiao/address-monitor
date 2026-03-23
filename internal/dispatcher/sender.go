package dispatcher

import (
	"context"
	"encoding/json"

	"address-monitor/internal/mq"
	"address-monitor/internal/parser"
	"address-monitor/internal/store"
	"address-monitor/pkg/httputil"
	"address-monitor/pkg/signature"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Sender struct {
	publisher     *mq.Publisher
	deliveryStore *store.DeliveryStore
	httpClient    *httputil.Client
}

func NewSender(
	publisher *mq.Publisher,
	deliveryStore *store.DeliveryStore,
	httpClient *httputil.Client,
) *Sender {
	return &Sender{
		publisher:     publisher,
		deliveryStore: deliveryStore,
		httpClient:    httpClient,
	}
}

func (s *Sender) Handle(msg amqp.Delivery) {
	var task DispatchTask
	if err := json.Unmarshal(msg.Body, &task); err != nil {
		zap.L().Error("解析 dispatch.tasks 消息失败", zap.Error(err))
		msg.Ack(false)
		return
	}

	// 读取重试次数
	retryCount := int32(0)
	if v, ok := msg.Headers["x-retry-count"]; ok {
		if rc, ok := v.(int32); ok {
			retryCount = rc
		}
	}

	ctx := context.Background()

	// 构造推送 payload（只推 event 部分）
	eventPayload, err := json.Marshal(task.Event)
	if err != nil {
		msg.Ack(false)
		return
	}

	// HMAC-SHA256 签名
	sig := signature.Sign(eventPayload, task.Secret)

	zap.L().Debug("开始推送回调",
		zap.String("event_id", task.Event.EventID),
		zap.String("callback", task.CallbackURL),
		zap.Int32("retry_count", retryCount),
	)

	// HTTP POST 推送
	code, body, err := s.httpClient.Post(
		task.CallbackURL,
		eventPayload,
		map[string]string{
			"Content-Type": "application/json",
			"X-Signature":  sig,
			"X-Event-ID":   task.Event.EventID,
		},
	)

	if err == nil && code >= 200 && code < 300 {
		// 推送成功
		s.deliveryStore.UpdateStatus(ctx, task.SubscriptionID, "success", code, string(body))
		zap.L().Info("推送成功",
			zap.String("event_id", task.Event.EventID),
			zap.String("callback", task.CallbackURL),
			zap.Int("code", code),
		)
		msg.Ack(false)
		return
	}

	// 推送失败
	responseBody := ""
	if body != nil {
		responseBody = string(body)
	}
	if err != nil {
		responseBody = err.Error()
	}
	s.deliveryStore.UpdateStatus(ctx, task.SubscriptionID, "failed", code, responseBody)
	s.deliveryStore.IncrRetryCount(ctx, task.SubscriptionID)

	retryCount++
	zap.L().Warn("推送失败，进入重试队列",
		zap.String("event_id", task.Event.EventID),
		zap.Int32("retry_count", retryCount),
		zap.Int("code", code),
	)

	if retryCount >= 3 {
		// 超过最大重试次数，发到死信终点
		s.publisher.Publish("", "dispatch.dead", msg.Body,
			amqp.Table{"x-retry-count": retryCount},
		)
		zap.L().Error("推送彻底失败，进入死信队列",
			zap.String("event_id", task.Event.EventID),
			zap.String("callback", task.CallbackURL),
		)
		msg.Ack(false)
		return
	}

	// 路由到对应延迟队列
	delayRoutes := []string{"retry.1", "retry.2", "retry.3", "retry.4"}
	s.publisher.Publish(
		"dlx.exchange",
		delayRoutes[retryCount-1],
		msg.Body,
		amqp.Table{"x-retry-count": retryCount},
	)
	msg.Ack(false)
}

// 确保 parser 包被引用
var _ = parser.EventTypeNativeTransfer
