package mq

import amqp "github.com/rabbitmq/amqp091-go"

// DeclareTopology 声明所有 Exchange、Queue、Binding
// Worker 和 Dispatcher 启动时都需要调用此函数
func DeclareTopology(ch *amqp.Channel) error {
	// 声明三个 Exchange
	exchanges := []struct{ name, kind string }{
		{"matched.exchange", "direct"},
		{"dispatch.exchange", "direct"},
		{"dlx.exchange", "direct"},
	}
	for _, ex := range exchanges {
		if err := ch.ExchangeDeclare(
			ex.name, ex.kind,
			true, false, false, false, nil,
		); err != nil {
			return err
		}
	}

	// matched.events 队列
	if _, err := ch.QueueDeclare("matched.events", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind("matched.events", "matched", "matched.exchange", false, nil); err != nil {
		return err
	}

	// dispatch.tasks 主队列（失败消息进 dlx.exchange）
	if _, err := ch.QueueDeclare("dispatch.tasks", true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": "dlx.exchange",
	}); err != nil {
		return err
	}
	if err := ch.QueueBind("dispatch.tasks", "dispatch", "dispatch.exchange", false, nil); err != nil {
		return err
	}

	// 四个延迟重试队列（TTL 到期后路由回 dispatch.exchange）
	retries := []struct {
		name, key string
		ttl       int32
	}{
		{"dispatch.retry.1m", "retry.1", 60000},
		{"dispatch.retry.5m", "retry.2", 300000},
		{"dispatch.retry.30m", "retry.3", 1800000},
		{"dispatch.retry.2h", "retry.4", 7200000},
	}
	for _, r := range retries {
		if _, err := ch.QueueDeclare(r.name, true, false, false, false, amqp.Table{
			"x-message-ttl":             r.ttl,
			"x-dead-letter-exchange":    "dispatch.exchange",
			"x-dead-letter-routing-key": "dispatch",
		}); err != nil {
			return err
		}
		if err := ch.QueueBind(r.name, r.key, "dlx.exchange", false, nil); err != nil {
			return err
		}
	}

	// dispatch.dead 死信终点队列
	if _, err := ch.QueueDeclare("dispatch.dead", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind("dispatch.dead", "dead", "dlx.exchange", false, nil); err != nil {
		return err
	}

	return nil
}
