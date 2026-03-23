package mq

import (
	"testing"
)

func TestDeclareTopology(t *testing.T) {
	conn, err := NewConnection("amqp://admin:admin@127.0.0.1:5672/")
	if err != nil {
		t.Fatalf("连接 RabbitMQ 失败: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("创建 Channel 失败: %v", err)
	}
	defer ch.Close()

	if err := DeclareTopology(ch); err != nil {
		t.Fatalf("声明拓扑失败: %v", err)
	}
	t.Log("RabbitMQ 拓扑声明成功 ✓")
}
