package chain

import (
	"context"
	"encoding/json"
)

// RawEvent 从链上获取的原始数据
type RawEvent struct {
	Chain     string
	TxHash    string
	BlockNum  uint64
	BlockTime int64
	Type      string // "tx" | "log" | "sol_tx"
	Data      json.RawMessage
}

// Listener 每条链必须实现此接口
type Listener interface {
	Chain() string
	Start(ctx context.Context, eventCh chan<- RawEvent, errCh chan<- error) error
	Stop() error
	HealthCheck(ctx context.Context) bool
}
