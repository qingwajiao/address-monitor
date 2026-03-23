package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type EventType string

const (
	EventTypeNativeTransfer EventType = "NATIVE_TRANSFER"
	EventTypeTokenTransfer  EventType = "TOKEN_TRANSFER"
	EventTypeSwap           EventType = "SWAP"
	EventTypeStake          EventType = "STAKE"
	EventTypeUnstake        EventType = "UNSTAKE"
	EventTypeContractCall   EventType = "CONTRACT_CALL"
	EventTypeReorg          EventType = "REORG"
)

type AssetInfo struct {
	Symbol          string `json:"symbol"`
	ContractAddress string `json:"contract_address"`
	Amount          string `json:"amount"` // 原始精度字符串，不做除法
	Decimals        int    `json:"decimals"`
}

type NormalizedEvent struct {
	EventID        string     `json:"event_id"`
	Chain          string     `json:"chain"`
	TxHash         string     `json:"tx_hash"`
	BlockNumber    uint64     `json:"block_number"`
	BlockTime      int64      `json:"block_time"`
	EventType      EventType  `json:"event_type"`
	WatchedAddress string     `json:"watched_address"`
	Direction      string     `json:"direction"` // "IN" | "OUT"
	From           string     `json:"from"`
	To             string     `json:"to"`
	Asset          *AssetInfo `json:"asset,omitempty"`
	Raw            string     `json:"raw"`
}

// RawEvent 从链上获取的原始数据
type RawEvent struct {
	Chain     string
	TxHash    string
	BlockNum  uint64
	BlockTime int64
	Type      string // "tx" | "log" | "sol_tx"
	Data      json.RawMessage
}

// GenerateEventID 生成全局幂等键
// logIndex 为 -1 表示原生币转账（非合约事件）
func GenerateEventID(chain, txHash string, logIndex int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", chain, txHash, logIndex)))
	return hex.EncodeToString(h[:])
}
