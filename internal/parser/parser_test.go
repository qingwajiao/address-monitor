package parser

import (
	"context"
	"encoding/json"
	"testing"
)

// SOL 的测试因为数据结构复杂，依赖真实的 RPC 响应格式，等 SOL Listener 写后再做端到端验证，这里先完成跳过

// ── EVM Parser 测试 ──────────────────────────────────────

func TestEVMParser_NativeTransfer(t *testing.T) {
	p := NewEVMParser("ETH")
	ctx := context.Background()

	txData, _ := json.Marshal(map[string]interface{}{
		"hash":  "0xabc123",
		"from":  "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"to":    "0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		"value": "1000000000000000000", // 1 ETH
		"input": "",
	})

	raw := RawEvent{
		Chain:     "ETH",
		TxHash:    "0xabc123",
		BlockNum:  12345678,
		BlockTime: 1710000000,
		Type:      "tx",
		Data:      txData,
	}

	events, err := p.Parse(ctx, raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("原生转账应该生成 2 个事件（IN+OUT），实际: %d", len(events))
	}

	// 验证 OUT 事件
	outEvent := events[0]
	if outEvent.Direction != "OUT" {
		t.Errorf("第一个事件应该是 OUT，实际: %s", outEvent.Direction)
	}
	if outEvent.EventType != EventTypeNativeTransfer {
		t.Errorf("事件类型应该是 NATIVE_TRANSFER，实际: %s", outEvent.EventType)
	}
	if outEvent.Asset.Symbol != "ETH" {
		t.Errorf("资产符号应该是 ETH，实际: %s", outEvent.Asset.Symbol)
	}
	if outEvent.EventID == "" {
		t.Error("EventID 不能为空")
	}
	t.Logf("OUT 事件: EventID=%s, From=%s, Amount=%s", outEvent.EventID, outEvent.From, outEvent.Asset.Amount)

	// 验证 IN 事件
	inEvent := events[1]
	if inEvent.Direction != "IN" {
		t.Errorf("第二个事件应该是 IN，实际: %s", inEvent.Direction)
	}
	t.Logf("IN 事件: EventID=%s, To=%s, Amount=%s", inEvent.EventID, inEvent.To, inEvent.Asset.Amount)

	// 两个事件的 EventID 应该不同
	if outEvent.EventID == inEvent.EventID {
		t.Error("OUT 和 IN 事件的 EventID 应该不同")
	}

	t.Log("EVM 原生转账解析测试通过 ✓")
}

func TestEVMParser_ERC20Transfer(t *testing.T) {
	p := NewEVMParser("ETH")
	ctx := context.Background()

	from := "0x000000000000000000000000AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	to := "0x000000000000000000000000BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	amount := "0x00000000000000000000000000000000000000000000000000000000000F4240"

	logData, _ := json.Marshal(map[string]interface{}{
		"address": "0xdac17f958d2ee523a2206206994597c13d831ec7",
		"topics": []string{
			TopicERC20Transfer.Hex(),
			from,
			to,
		},
		"data":             amount,
		"blockNumber":      "0xBC614E",
		"transactionHash":  "0xdef456def456def456def456def456def456def456def456def456def456def4",
		"transactionIndex": "0x0",
		"blockHash":        "0x1111111111111111111111111111111111111111111111111111111111111111",
		"logIndex":         "0x0",
		"removed":          false,
	})

	raw := RawEvent{
		Chain:     "ETH",
		TxHash:    "0xdef456",
		BlockNum:  12345678,
		BlockTime: 1710000000,
		Type:      "log",
		Data:      logData,
	}

	events, err := p.Parse(ctx, raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ERC20 转账应该生成 2 个事件，实际: %d", len(events))
	}
	for _, e := range events {
		if e.EventType != EventTypeTokenTransfer {
			t.Errorf("事件类型应该是 TOKEN_TRANSFER，实际: %s", e.EventType)
		}
		if e.Asset == nil {
			t.Error("Asset 不能为空")
		}
		t.Logf("事件: Direction=%s, From=%s, To=%s, Amount=%s",
			e.Direction, e.From, e.To, e.Asset.Amount)
	}
	t.Log("EVM ERC20 转账解析测试通过 ✓")
}

func TestEVMParser_ZeroValue(t *testing.T) {
	p := NewEVMParser("ETH")
	ctx := context.Background()

	// value=0 的交易不应该生成事件
	txData, _ := json.Marshal(map[string]interface{}{
		"hash":  "0xzero",
		"from":  "0xAAAA",
		"to":    "0xBBBB",
		"value": "0",
		"input": "0x1234",
	})

	raw := RawEvent{
		Chain: "ETH", TxHash: "0xzero",
		BlockNum: 1, BlockTime: 1, Type: "tx", Data: txData,
	}

	events, err := p.Parse(ctx, raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("value=0 的交易不应该生成事件，实际生成: %d", len(events))
	}
	t.Log("value=0 过滤测试通过 ✓")
}

func TestEVMParser_BSC(t *testing.T) {
	p := NewEVMParser("BSC")

	txData, _ := json.Marshal(map[string]interface{}{
		"hash":  "0xbsc001",
		"from":  "0xAAAA",
		"to":    "0xBBBB",
		"value": "500000000000000000", // 0.5 BNB
		"input": "",
	})

	raw := RawEvent{
		Chain: "BSC", TxHash: "0xbsc001",
		BlockNum: 1, BlockTime: 1, Type: "tx", Data: txData,
	}

	events, _ := p.Parse(context.Background(), raw)
	if len(events) != 2 {
		t.Fatalf("应该生成 2 个事件，实际: %d", len(events))
	}
	if events[0].Asset.Symbol != "BNB" {
		t.Errorf("BSC 原生币符号应该是 BNB，实际: %s", events[0].Asset.Symbol)
	}
	t.Log("BSC 原生转账符号测试通过 ✓")
}

// ── GenerateEventID 测试 ──────────────────────────────────

func TestGenerateEventID(t *testing.T) {
	// 相同参数生成相同 ID
	id1 := GenerateEventID("ETH", "0xabc", 0)
	id2 := GenerateEventID("ETH", "0xabc", 0)
	if id1 != id2 {
		t.Error("相同参数应该生成相同 EventID")
	}

	// 不同参数生成不同 ID
	id3 := GenerateEventID("ETH", "0xabc", 1)
	id4 := GenerateEventID("BSC", "0xabc", 0)
	id5 := GenerateEventID("ETH", "0xdef", 0)

	ids := []string{id1, id3, id4, id5}
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Error("不同参数生成了相同的 EventID")
		}
		seen[id] = true
	}

	t.Logf("EventID 示例: %s", id1)
	t.Log("GenerateEventID 测试通过 ✓")
}

// ── TRON Parser 测试 ──────────────────────────────────────

func TestTRONParser_NativeTransfer(t *testing.T) {
	p := NewTRONParser()
	ctx := context.Background()

	txData, _ := json.Marshal(map[string]interface{}{
		"txID": "tronhash001",
		"ret":  []map[string]string{{"contractRet": "SUCCESS"}},
		"raw_data": map[string]interface{}{
			"contract": []map[string]interface{}{
				{
					"type": "TransferContract",
					"parameter": map[string]interface{}{
						"value": map[string]interface{}{
							"amount":        1000000, // 1 TRX
							"owner_address": "41aabbccdd",
							"to_address":    "41eeff1122",
						},
					},
				},
			},
			"timestamp": 1710000000000,
		},
		"log": []interface{}{},
	})

	raw := RawEvent{
		Chain:     "TRON",
		TxHash:    "tronhash001",
		BlockNum:  50000000,
		BlockTime: 1710000000,
		Type:      "tx",
		Data:      txData,
	}

	events, err := p.Parse(ctx, raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("TRON 原生转账应该生成 2 个事件，实际: %d", len(events))
	}
	if events[0].Asset.Symbol != "TRX" {
		t.Errorf("资产符号应该是 TRX，实际: %s", events[0].Asset.Symbol)
	}
	if events[0].Asset.Decimals != 6 {
		t.Errorf("TRX decimals 应该是 6，实际: %d", events[0].Asset.Decimals)
	}
	t.Logf("TRX 转账: amount=%s, decimals=%d", events[0].Asset.Amount, events[0].Asset.Decimals)
	t.Log("TRON 原生转账解析测试通过 ✓")
}

func TestTRONParser_FailedTx(t *testing.T) {
	p := NewTRONParser()
	ctx := context.Background()

	// 失败的交易不应该生成事件
	txData, _ := json.Marshal(map[string]interface{}{
		"txID": "tronhash_fail",
		"ret":  []map[string]string{{"contractRet": "REVERT"}},
		"raw_data": map[string]interface{}{
			"contract": []map[string]interface{}{
				{
					"type": "TransferContract",
					"parameter": map[string]interface{}{
						"value": map[string]interface{}{
							"amount":        1000000,
							"owner_address": "41aabbccdd",
							"to_address":    "41eeff1122",
						},
					},
				},
			},
			"timestamp": 1710000000000,
		},
	})

	raw := RawEvent{
		Chain: "TRON", TxHash: "tronhash_fail",
		BlockNum: 1, BlockTime: 1, Type: "tx", Data: txData,
	}

	events, _ := p.Parse(ctx, raw)
	if len(events) != 0 {
		t.Errorf("失败交易不应该生成事件，实际: %d", len(events))
	}
	t.Log("TRON 失败交易过滤测试通过 ✓")
}
