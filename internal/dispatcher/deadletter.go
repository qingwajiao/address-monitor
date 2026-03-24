package dispatcher

import (
	"context"
	"encoding/json"

	"address-monitor/internal/store"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type DeadLetterHandler struct {
	webhookStore *store.WebhookLogStore
}

func NewDeadLetterHandler(webhookStore *store.WebhookLogStore) *DeadLetterHandler {
	return &DeadLetterHandler{webhookStore: webhookStore}
}

func (d *DeadLetterHandler) Handle(msg amqp.Delivery) {
	var task DispatchTask
	if err := json.Unmarshal(msg.Body, &task); err != nil {
		zap.L().Error("解析死信消息失败", zap.Error(err))
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
	d.webhookStore.MarkDead(ctx, task.WebhookLogID)

	zap.L().Error("!! 推送彻底失败，需人工介入 !!",
		zap.Uint64("webhook_log_id", task.WebhookLogID),
		zap.String("event_id", task.Event.EventID),
		zap.String("chain", task.Event.Chain),
		zap.String("tx_hash", task.Event.TxHash),
		zap.String("callback_url", task.CallbackURL),
		zap.Int32("retry_count", retryCount),
	)

	msg.Ack(false)
}
