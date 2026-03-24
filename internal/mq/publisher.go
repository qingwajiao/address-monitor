package mq

import (
	"fmt"
	_ "sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	confirmTimeout = 5 * time.Second
	maxRetries     = 3
)

type Publisher struct {
	conn *Connection
}

func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}

// Publish 发布持久化消息，带 Publisher Confirms 保证投递可靠性
func (p *Publisher) Publish(exchange, routingKey string, body []byte, headers amqp.Table) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := p.publishOnce(exchange, routingKey, body, headers); err != nil {
			lastErr = err
			zap.L().Warn("消息发布失败，准备重试",
				zap.String("exchange", exchange),
				zap.String("routing_key", routingKey),
				zap.Int("attempt", i+1),
				zap.Error(err),
			)
			time.Sleep(time.Duration(i+1) * 200 * time.Millisecond)
			continue
		}
		return nil
	}
	zap.L().Error("消息发布最终失败",
		zap.String("exchange", exchange),
		zap.String("routing_key", routingKey),
		zap.Error(lastErr),
	)
	return fmt.Errorf("消息发布失败（重试%d次）: %w", maxRetries, lastErr)
}

func (p *Publisher) publishOnce(exchange, routingKey string, body []byte, headers amqp.Table) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("创建 Channel 失败: %w", err)
	}
	defer ch.Close()

	// 开启 Publisher Confirms 模式
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("开启 Confirm 模式失败: %w", err)
	}

	// 监听 ack/nack
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	// 发布消息
	if err := ch.Publish(
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Headers:      headers,
		},
	); err != nil {
		return fmt.Errorf("发布消息失败: %w", err)
	}

	// 等待 Broker 确认
	select {
	case confirm := <-confirms:
		if !confirm.Ack {
			return fmt.Errorf("Broker 返回 nack，消息未被接受")
		}
		zap.L().Debug("消息已确认",
			zap.String("exchange", exchange),
			zap.String("routing_key", routingKey),
		)
		return nil
	case <-time.After(confirmTimeout):
		return fmt.Errorf("等待 Broker 确认超时（%s）", confirmTimeout)
	}
}
