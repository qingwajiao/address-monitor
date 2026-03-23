package dispatcher

import (
	"context"
	"encoding/json"

	"address-monitor/internal/store"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type DeadLetterHandler struct {
	deliveryStore *store.DeliveryStore
}

func NewDeadLetterHandler(deliveryStore *store.DeliveryStore) *DeadLetterHandler {
	return &DeadLetterHandler{deliveryStore: deliveryStore}
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

	// 标记为死信
	if err := d.deliveryStore.MarkDead(ctx, task.SubscriptionID); err != nil {
		zap.L().Error("标记死信失败", zap.Error(err))
	}

	// 打告警日志（生产环境可接入告警系统）
	zap.L().Error("!!推送彻底失败，需人工介入!!",
		zap.String("task_id", task.TaskID),
		zap.String("event_id", task.Event.EventID),
		zap.String("chain", task.Event.Chain),
		zap.String("tx_hash", task.Event.TxHash),
		zap.String("callback_url", task.CallbackURL),
		zap.Int32("retry_count", retryCount),
	)

	msg.Ack(false)
}
