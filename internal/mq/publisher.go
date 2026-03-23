package mq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	conn *Connection
}

func NewPublisher(conn *Connection) *Publisher {
	return &Publisher{conn: conn}
}

// Publish 发布持久化消息
// exchange 为空字符串时直接发到 routingKey 指定的队列
func (p *Publisher) Publish(exchange, routingKey string, body []byte, headers amqp.Table) error {
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	return ch.Publish(
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
	)
}
