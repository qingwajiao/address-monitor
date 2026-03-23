package mq

import (
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Connection struct {
	url  string
	conn *amqp.Connection
	mu   sync.RWMutex
}

func NewConnection(url string) (*Connection, error) {
	c := &Connection{url: url}
	if err := c.connect(); err != nil {
		return nil, err
	}
	go c.reconnectLoop()
	return c, nil
}

func (c *Connection) connect() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

func (c *Connection) reconnectLoop() {
	for {
		c.mu.RLock()
		closeCh := c.conn.NotifyClose(make(chan *amqp.Error, 1))
		c.mu.RUnlock()

		<-closeCh
		zap.L().Warn("RabbitMQ 连接断开，尝试重连")

		delay := time.Second
		for {
			if err := c.connect(); err == nil {
				zap.L().Info("RabbitMQ 重连成功")
				break
			}
			zap.L().Warn("RabbitMQ 重连失败，等待后重试", zap.Duration("delay", delay))
			time.Sleep(delay)
			if delay < 30*time.Second {
				delay *= 2
			}
		}
	}
}

func (c *Connection) Channel() (*amqp.Channel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn.Channel()
}

func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}
