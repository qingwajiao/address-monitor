package parser

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/btcsuite/btcutil/base58"
	"github.com/ethereum/go-ethereum/common"
)

type TRONParser struct{}

func NewTRONParser() *TRONParser { return &TRONParser{} }

// TRON 交易结构
type tronTx struct {
	TxID    string `json:"txID"`
	RawData struct {
		Contract []struct {
			Type      string `json:"type"`
			Parameter struct {
				Value struct {
					Amount          int64  `json:"amount"`
					OwnerAddress    string `json:"owner_address"`
					ToAddress       string `json:"to_address"`
					ContractAddress string `json:"contract_address"`
					Data            string `json:"data"`
				} `json:"value"`
			} `json:"parameter"`
		} `json:"contract"`
		Timestamp int64 `json:"timestamp"`
	} `json:"raw_data"`
	Ret []struct {
		ContractRet string `json:"contractRet"`
	} `json:"ret"`
	Log []struct {
		Address string   `json:"address"`
		Topics  []string `json:"topics"`
		Data    string   `json:"data"`
	} `json:"log"`
}

func (p *TRONParser) Parse(ctx context.Context, raw RawEvent) ([]*NormalizedEvent, error) {
	var tx tronTx
	if err := json.Unmarshal(raw.Data, &tx); err != nil {
		return nil, err
	}

	// 只处理成功的交易
	if len(tx.Ret) == 0 || tx.Ret[0].ContractRet != "SUCCESS" {
		return nil, nil
	}

	if len(tx.RawData.Contract) == 0 {
		return nil, nil
	}

	contract := tx.RawData.Contract[0]
	rawStr := string(raw.Data)

	switch contract.Type {
	case "TransferContract":
		return p.parseNativeTransfer(raw, tx, rawStr)
	case "TriggerSmartContract":
		return p.parseLogs(raw, tx, rawStr)
	default:
		return nil, nil
	}
}

func (p *TRONParser) parseNativeTransfer(raw RawEvent, tx tronTx, rawStr string) ([]*NormalizedEvent, error) {
	v := tx.RawData.Contract[0].Parameter.Value
	from := normalizeHexAddress(v.OwnerAddress)
	to := normalizeHexAddress(v.ToAddress)
	amount := big.NewInt(v.Amount).String()

	asset := &AssetInfo{
		Symbol:   "TRX",
		Amount:   amount,
		Decimals: 6, // 1 TRX = 1,000,000 SUN
	}

	return []*NormalizedEvent{
		{
			EventID:        GenerateEventID("TRON", raw.TxHash, -2),
			Chain:          "TRON",
			TxHash:         raw.TxHash,
			BlockNumber:    raw.BlockNum,
			BlockTime:      raw.BlockTime,
			EventType:      EventTypeNativeTransfer,
			WatchedAddress: from,
			Direction:      "OUT",
			From:           from,
			To:             to,
			Asset:          asset,
			Raw:            rawStr,
		},
		{
			EventID:        GenerateEventID("TRON", raw.TxHash, -1),
			Chain:          "TRON",
			TxHash:         raw.TxHash,
			BlockNumber:    raw.BlockNum,
			BlockTime:      raw.BlockTime,
			EventType:      EventTypeNativeTransfer,
			WatchedAddress: to,
			Direction:      "IN",
			From:           from,
			To:             to,
			Asset:          asset,
			Raw:            rawStr,
		},
	}, nil
}

func (p *TRONParser) parseLogs(raw RawEvent, tx tronTx, rawStr string) ([]*NormalizedEvent, error) {
	var events []*NormalizedEvent
	for i, log := range tx.Log {
		if len(log.Topics) < 3 {
			continue
		}
		// topics[0] == ERC20 Transfer topic hash（TRON 复用 EVM 标准）
		topic0 := common.HexToHash("0x" + log.Topics[0])
		if topic0 != TopicERC20Transfer {
			continue
		}

		from := normalizeHexAddress(log.Topics[1])
		to := normalizeHexAddress(log.Topics[2])

		dataBytes, _ := hex.DecodeString(log.Data)
		amount := new(big.Int).SetBytes(dataBytes).String()
		contract := normalizeHexAddress(log.Address)

		asset := &AssetInfo{
			ContractAddress: contract,
			Amount:          amount,
			Decimals:        0,
		}

		events = append(events,
			&NormalizedEvent{
				EventID:        GenerateEventID("TRON", raw.TxHash, i*10+0),
				Chain:          "TRON",
				TxHash:         raw.TxHash,
				BlockNumber:    raw.BlockNum,
				BlockTime:      raw.BlockTime,
				EventType:      EventTypeTokenTransfer,
				WatchedAddress: from,
				Direction:      "OUT",
				From:           from,
				To:             to,
				Asset:          asset,
				Raw:            rawStr,
			},
			&NormalizedEvent{
				EventID:        GenerateEventID("TRON", raw.TxHash, i*10+1),
				Chain:          "TRON",
				TxHash:         raw.TxHash,
				BlockNumber:    raw.BlockNum,
				BlockTime:      raw.BlockTime,
				EventType:      EventTypeTokenTransfer,
				WatchedAddress: to,
				Direction:      "IN",
				From:           from,
				To:             to,
				Asset:          asset,
				Raw:            rawStr,
			},
		)
	}
	return events, nil
}

// normalizeHexAddress 将 TRON hex 地址（41开头）转为小写 hex
func normalizeHexAddress(addr string) string {
	addr = strings.TrimPrefix(addr, "0x")
	addr = strings.TrimPrefix(addr, "41")
	return strings.ToLower(addr)
}

// HexToBase58 将 TRON hex 地址转换为 Base58Check 格式（T 开头）
func HexToBase58(hexAddr string) string {
	hexAddr = strings.TrimPrefix(hexAddr, "0x")
	b, err := hex.DecodeString(hexAddr)
	if err != nil || len(b) == 0 {
		return hexAddr
	}
	return base58.CheckEncode(b[1:], b[0])
}
