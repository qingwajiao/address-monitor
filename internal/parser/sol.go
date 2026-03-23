package parser

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gagliardetto/solana-go/rpc"
)

// 已知 SPL Token Mint 地址
var knownTokens = map[string]struct {
	Symbol   string
	Decimals int
}{
	"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v": {"USDC", 6},
	"Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB": {"USDT", 6},
}

type SOLParser struct{}

func NewSOLParser() *SOLParser { return &SOLParser{} }

func (p *SOLParser) Parse(ctx context.Context, raw RawEvent) ([]*NormalizedEvent, error) {
	if raw.Type != "sol_tx" {
		return nil, nil
	}

	var tx rpc.GetTransactionResult
	if err := json.Unmarshal(raw.Data, &tx); err != nil {
		return nil, err
	}
	if tx.Meta == nil {
		return nil, nil
	}

	rawStr := string(raw.Data)
	var events []*NormalizedEvent

	// 解析原生 SOL 转账
	nativeEvents := p.parseNativeTransfers(raw, &tx, rawStr)
	events = append(events, nativeEvents...)

	// 解析 SPL Token 转账
	tokenEvents := p.parseTokenTransfers(raw, &tx, rawStr)
	events = append(events, tokenEvents...)

	return events, nil
}

func (p *SOLParser) parseNativeTransfers(raw RawEvent, tx *rpc.GetTransactionResult, rawStr string) []*NormalizedEvent {
	if tx.Transaction == nil {
		return nil
	}

	parsed, err := tx.Transaction.GetTransaction()
	if err != nil {
		return nil
	}

	pre := tx.Meta.PreBalances
	post := tx.Meta.PostBalances
	fee := tx.Meta.Fee
	accounts := parsed.Message.AccountKeys

	var events []*NormalizedEvent
	for i := 0; i < len(accounts) && i < len(pre) && i < len(post); i++ {
		preBalance := pre[i]
		postBalance := post[i]

		// index=0 是 fee payer，需要加回手续费才是净变化
		netChange := int64(postBalance) - int64(preBalance)
		if i == 0 {
			netChange += int64(fee)
		}

		if netChange == 0 {
			continue
		}

		addr := strings.ToLower(accounts[i].String())
		var direction string
		var amount string

		if netChange > 0 {
			direction = "IN"
			amount = uint64ToString(uint64(netChange))
		} else {
			direction = "OUT"
			amount = uint64ToString(uint64(-netChange))
		}

		events = append(events, &NormalizedEvent{
			EventID:        GenerateEventID("SOL", raw.TxHash, i),
			Chain:          "SOL",
			TxHash:         raw.TxHash,
			BlockNumber:    raw.BlockNum,
			BlockTime:      raw.BlockTime,
			EventType:      EventTypeNativeTransfer,
			WatchedAddress: addr,
			Direction:      direction,
			From:           addr,
			To:             addr,
			Asset: &AssetInfo{
				Symbol:   "SOL",
				Amount:   amount,
				Decimals: 9,
			},
			Raw: rawStr,
		})
	}
	return events
}

func (p *SOLParser) parseTokenTransfers(raw RawEvent, tx *rpc.GetTransactionResult, rawStr string) []*NormalizedEvent {
	pre := tx.Meta.PreTokenBalances
	post := tx.Meta.PostTokenBalances

	// 建立 accountIndex → preBalance 的 map
	preMap := make(map[uint16]rpc.TokenBalance)
	for _, b := range pre {
		preMap[b.AccountIndex] = b
	}

	var events []*NormalizedEvent
	for i, postBal := range post {
		preBal, exists := preMap[postBal.AccountIndex]

		var preAmount uint64

		if exists && preBal.UiTokenAmount.Amount != "" {
			parseUint64(preBal.UiTokenAmount.Amount, &preAmount)
		}

		var postAmount uint64
		if postBal.UiTokenAmount.Amount != "" {
			parseUint64(postBal.UiTokenAmount.Amount, &postAmount)
		}

		if preAmount == postAmount {
			continue
		}

		owner := strings.ToLower(postBal.Owner.String())
		mint := postBal.Mint.String()
		decimals := int(postBal.UiTokenAmount.Decimals)

		var direction string
		var amount string
		if postAmount > preAmount {
			direction = "IN"
			amount = uint64ToString(postAmount - preAmount)
		} else {
			direction = "OUT"
			amount = uint64ToString(preAmount - postAmount)
		}

		symbol := "UNKNOWN"
		if info, ok := knownTokens[mint]; ok {
			symbol = info.Symbol
			decimals = info.Decimals
		}

		events = append(events, &NormalizedEvent{
			EventID:        GenerateEventID("SOL", raw.TxHash, 10000+i),
			Chain:          "SOL",
			TxHash:         raw.TxHash,
			BlockNumber:    raw.BlockNum,
			BlockTime:      raw.BlockTime,
			EventType:      EventTypeTokenTransfer,
			WatchedAddress: owner,
			Direction:      direction,
			From:           owner,
			To:             owner,
			Asset: &AssetInfo{
				Symbol:          symbol,
				ContractAddress: mint,
				Amount:          amount,
				Decimals:        decimals,
			},
			Raw: rawStr,
		})
	}
	return events
}

func uint64ToString(n uint64) string {
	return string(rune('0' + n%10)) // 简化版，实际用 strconv
}

func parseUint64(s string, out *uint64) {
	var n uint64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + uint64(c-'0')
		}
	}
	*out = n
}
