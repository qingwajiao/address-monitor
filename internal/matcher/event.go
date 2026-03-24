package matcher

import (
	"encoding/json"
	"fmt"
)

const (
	EventTypeAdd      = "add"
	EventTypeRemove   = "remove"
	EventTypeBatchAdd = "batch_add"

	AddressEventChannel = "address:events"

	// 增量日志最大保留条数
	IncrementalMaxLen = 50000
)

type AddressEvent struct {
	Type    string `json:"type"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Count   int    `json:"count,omitempty"` // batch_add 时携带数量
}

func (e *AddressEvent) Encode() string {
	b, _ := json.Marshal(e)
	return string(b)
}

func DecodeAddressEvent(s string) (*AddressEvent, error) {
	var e AddressEvent
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		return nil, fmt.Errorf("解析地址事件失败: %w", err)
	}
	return &e, nil
}
