package parser

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// 已知合约事件的 topic hash（keccak256 计算结果，硬编码）
var (
	// keccak256("Transfer(address,address,uint256)")
	TopicERC20Transfer = common.HexToHash(
		"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	// keccak256("Swap(address,uint256,uint256,uint256,uint256,address)") Uniswap V2
	TopicUniswapV2Swap = common.HexToHash(
		"0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822")

	// keccak256("Swap(address,address,int256,int256,uint160,uint128,int24)") Uniswap V3
	TopicUniswapV3Swap = common.HexToHash(
		"0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67")

	// keccak256("Deposit(address,uint256)")
	TopicDeposit = common.HexToHash(
		"0xe1fffcc4923d04b559f4d29a8bfc6cda04eb5b0d3c460751c2402c5c5cc9109c")

	// keccak256("Withdraw(address,uint256,address)")
	TopicWithdraw = common.HexToHash(
		"0x884edad9ce6fa2440d8a54cc123490eb96d2768479d49ff9c7366125a9424364")
)

// nativeSymbols 各链原生币符号
var nativeSymbols = map[string]string{
	"ETH": "ETH",
	"BSC": "BNB",
}

type EVMParser struct {
	chain string
}

func NewEVMParser(chain string) *EVMParser {
	return &EVMParser{chain: strings.ToUpper(chain)}
}

func (p *EVMParser) Parse(ctx context.Context, raw RawEvent) ([]*NormalizedEvent, error) {
	switch raw.Type {
	case "tx":
		return p.parseTx(raw)
	case "log":
		return p.parseLog(raw)
	default:
		return nil, nil
	}
}

// parseTx 解析原生币转账
type evmTxData struct {
	Hash  string `json:"hash"`
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"` // 十进制字符串
	Input string `json:"input"`
}

func (p *EVMParser) parseTx(raw RawEvent) ([]*NormalizedEvent, error) {
	var tx evmTxData
	if err := json.Unmarshal(raw.Data, &tx); err != nil {
		return nil, err
	}

	value := new(big.Int)
	value.SetString(tx.Value, 10)
	if value.Sign() <= 0 {
		return nil, nil
	}

	symbol := nativeSymbols[p.chain]
	if symbol == "" {
		symbol = p.chain
	}

	asset := &AssetInfo{
		Symbol:   symbol,
		Amount:   tx.Value,
		Decimals: 18,
	}

	from := strings.ToLower(tx.From)
	to := strings.ToLower(tx.To)
	rawStr := string(raw.Data)

	events := []*NormalizedEvent{
		{
			EventID:        GenerateEventID(p.chain, raw.TxHash, -2), // -2 = OUT
			Chain:          p.chain,
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
			EventID:        GenerateEventID(p.chain, raw.TxHash, -1), // -1 = IN
			Chain:          p.chain,
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
	}
	return events, nil
}

// parseLog 解析合约事件
func (p *EVMParser) parseLog(raw RawEvent) ([]*NormalizedEvent, error) {
	var log types.Log
	if err := json.Unmarshal(raw.Data, &log); err != nil {
		return nil, err
	}
	if len(log.Topics) == 0 {
		return nil, nil
	}

	rawStr := string(raw.Data)
	topic0 := log.Topics[0]

	switch topic0 {
	case TopicERC20Transfer:
		return p.parseERC20Transfer(raw, log, rawStr)
	case TopicUniswapV2Swap, TopicUniswapV3Swap:
		return p.parseSwap(raw, log, rawStr)
	case TopicDeposit:
		return p.parseStake(raw, log, rawStr)
	case TopicWithdraw:
		return p.parseUnstake(raw, log, rawStr)
	default:
		// 兜底：未知合约调用
		return []*NormalizedEvent{{
			EventID:     GenerateEventID(p.chain, raw.TxHash, int(log.Index)),
			Chain:       p.chain,
			TxHash:      raw.TxHash,
			BlockNumber: raw.BlockNum,
			BlockTime:   raw.BlockTime,
			EventType:   EventTypeContractCall,
			From:        strings.ToLower(log.Address.Hex()),
			Raw:         rawStr,
		}}, nil
	}
}

func (p *EVMParser) parseERC20Transfer(raw RawEvent, log types.Log, rawStr string) ([]*NormalizedEvent, error) {
	if len(log.Topics) < 3 {
		return nil, nil
	}
	from := strings.ToLower(common.BytesToAddress(log.Topics[1].Bytes()).Hex())
	to := strings.ToLower(common.BytesToAddress(log.Topics[2].Bytes()).Hex())
	amount := new(big.Int).SetBytes(log.Data).String()
	contract := strings.ToLower(log.Address.Hex())

	asset := &AssetInfo{
		ContractAddress: contract,
		Amount:          amount,
		Decimals:        0, // 调用方可通过合约查询，此处留 0
	}

	return []*NormalizedEvent{
		{
			EventID:        GenerateEventID(p.chain, raw.TxHash, int(log.Index)*10+0),
			Chain:          p.chain,
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
		{
			EventID:        GenerateEventID(p.chain, raw.TxHash, int(log.Index)*10+1),
			Chain:          p.chain,
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
	}, nil
}

func (p *EVMParser) parseSwap(raw RawEvent, log types.Log, rawStr string) ([]*NormalizedEvent, error) {
	contract := strings.ToLower(log.Address.Hex())
	return []*NormalizedEvent{{
		EventID:        GenerateEventID(p.chain, raw.TxHash, int(log.Index)),
		Chain:          p.chain,
		TxHash:         raw.TxHash,
		BlockNumber:    raw.BlockNum,
		BlockTime:      raw.BlockTime,
		EventType:      EventTypeSwap,
		WatchedAddress: contract,
		From:           contract,
		Raw:            rawStr,
	}}, nil
}

func (p *EVMParser) parseStake(raw RawEvent, log types.Log, rawStr string) ([]*NormalizedEvent, error) {
	contract := strings.ToLower(log.Address.Hex())
	var user string
	if len(log.Topics) >= 2 {
		user = strings.ToLower(common.BytesToAddress(log.Topics[1].Bytes()).Hex())
	}
	amount := ""
	if len(log.Data) >= 32 {
		amount = new(big.Int).SetBytes(log.Data[:32]).String()
	}
	return []*NormalizedEvent{{
		EventID:        GenerateEventID(p.chain, raw.TxHash, int(log.Index)),
		Chain:          p.chain,
		TxHash:         raw.TxHash,
		BlockNumber:    raw.BlockNum,
		BlockTime:      raw.BlockTime,
		EventType:      EventTypeStake,
		WatchedAddress: user,
		From:           user,
		To:             contract,
		Asset:          &AssetInfo{ContractAddress: contract, Amount: amount},
		Raw:            rawStr,
	}}, nil
}

func (p *EVMParser) parseUnstake(raw RawEvent, log types.Log, rawStr string) ([]*NormalizedEvent, error) {
	contract := strings.ToLower(log.Address.Hex())
	var user string
	if len(log.Topics) >= 2 {
		user = strings.ToLower(common.BytesToAddress(log.Topics[1].Bytes()).Hex())
	}
	amount := ""
	if len(log.Data) >= 32 {
		amount = new(big.Int).SetBytes(log.Data[:32]).String()
	}
	return []*NormalizedEvent{{
		EventID:        GenerateEventID(p.chain, raw.TxHash, int(log.Index)),
		Chain:          p.chain,
		TxHash:         raw.TxHash,
		BlockNumber:    raw.BlockNum,
		BlockTime:      raw.BlockTime,
		EventType:      EventTypeUnstake,
		WatchedAddress: user,
		From:           contract,
		To:             user,
		Asset:          &AssetInfo{ContractAddress: contract, Amount: amount},
		Raw:            rawStr,
	}}, nil
}

// 确保 hex 包被使用（避免编译报错）
var _ = hex.EncodeToString
