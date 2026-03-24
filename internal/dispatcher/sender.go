package dispatcher

import (
	"context"
	"encoding/json"

	"address-monitor/internal/mq"
	"address-monitor/internal/store"
	"address-monitor/pkg/httputil"
	"address-monitor/pkg/signature"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Sender struct {
	publisher    *mq.Publisher
	webhookStore *store.WebhookLogStore
	httpClient   *httputil.Client
}

func NewSender(
	publisher *mq.Publisher,
	webhookStore *store.WebhookLogStore,
	httpClient *httputil.Client,
) *Sender {
	return &Sender{
		publisher:    publisher,
		webhookStore: webhookStore,
		httpClient:   httpClient,
	}
}

func (s *Sender) Handle(msg amqp.Delivery) {
	var task DispatchTask
	if err := json.Unmarshal(msg.Body, &task); err != nil {
		zap.L().Error("解析 dispatch.tasks 消息失败", zap.Error(err))
		msg.Ack(false)
		return
	}

	retryCount := int32(0)
	if v, ok := msg.Headers["x-retry-count"]; ok {
		if rc, ok := v.(int32); ok {
			retryCount = rc
		}
	}

	ctx := context.Background()

	eventPayload, err := json.Marshal(task.Event)
	if err != nil {
		msg.Ack(false)
		return
	}

	sig := signature.Sign(eventPayload, task.Secret)

	zap.L().Debug("开始推送回调",
		zap.String("event_id", task.Event.EventID),
		zap.String("callback", task.CallbackURL),
		zap.Int32("retry_count", retryCount),
	)

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
		s.webhookStore.UpdateStatus(ctx, task.WebhookLogID, "success", code, string(body))
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
	s.webhookStore.UpdateStatus(ctx, task.WebhookLogID, "failed", code, responseBody)
	s.webhookStore.IncrRetryCount(ctx, task.WebhookLogID)

	retryCount++
	zap.L().Warn("推送失败，进入重试队列",
		zap.String("event_id", task.Event.EventID),
		zap.Int32("retry_count", retryCount),
		zap.Int("code", code),
	)

	if retryCount >= 5 {
		s.publisher.Publish("", "dispatch.dead", msg.Body,
			amqp.Table{"x-retry-count": retryCount},
		)
		zap.L().Error("推送彻底失败，进入死信队列",
			zap.String("event_id", task.Event.EventID),
		)
		msg.Ack(false)
		return
	}

	routes := []string{"retry.1", "retry.2", "retry.3", "retry.4"}
	s.publisher.Publish(
		"dlx.exchange",
		routes[retryCount-1],
		msg.Body,
		amqp.Table{"x-retry-count": retryCount},
	)
	msg.Ack(false)
}
