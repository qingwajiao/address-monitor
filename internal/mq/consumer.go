package mq

import (
	"context"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Consumer struct {
	conn *Connection
}

func NewConsumer(conn *Connection) *Consumer {
	return &Consumer{conn: conn}
}

// Consume 开始消费指定队列
// 断线后自动重新注册消费，直到 ctx 取消
func (c *Consumer) Consume(ctx context.Context, queue string, handler func(amqp.Delivery)) error {
	for {
		if err := c.runConsume(ctx, queue, handler); err != nil {
			zap.L().Warn("消费者异常，2s 后重试",
				zap.String("queue", queue),
				zap.Error(err),
			)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Consumer) runConsume(ctx context.Context, queue string, handler func(amqp.Delivery)) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	// 每次最多预取 10 条，控制并发消费数量
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}

	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	zap.L().Info("开始消费队列", zap.String("queue", queue))

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil // channel 关闭，触发重连
			}
			handler(msg)
		}
	}
}
