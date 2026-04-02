package addrcodec

import (
	"encoding/hex"
	"strings"

	"github.com/btcsuite/btcutil/base58"
)

// AddressCodec 负责将地址归一化为该链的标准存储格式。
// EVM  : 0x + 小写 hex
// TRON : Base58Check（T 开头）
// SOL  : base58 原样（大小写敏感，pass-through）
type AddressCodec interface {
	Normalize(addr string) string
}

// Get 按链名返回对应 Codec，未知链返回 evmCodec 兜底
func Get(chain string) AddressCodec {
	if c, ok := codecs[strings.ToUpper(chain)]; ok {
		return c
	}
	return evmCodec{}
}

var codecs = map[string]AddressCodec{
	"ETH":  evmCodec{},
	"BSC":  evmCodec{},
	"TRON": tronCodec{},
	"SOL":  solCodec{},
}

// ── EVM ──────────────────────────────────────────────────────────────────────

type evmCodec struct{}

func (evmCodec) Normalize(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if !strings.HasPrefix(addr, "0x") {
		addr = "0x" + addr
	}
	return addr
}

// ── TRON ─────────────────────────────────────────────────────────────────────

type tronCodec struct{}

// Normalize 将任意 TRON 地址格式统一为 Base58Check（T 开头）：
//   - Base58（T...）  → 直接返回
//   - hex-with-41     → Base58
//   - hex-without-41  → 补 41 前缀 → Base58
func (tronCodec) Normalize(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}

	// Base58Check 格式（T 开头）
	if strings.HasPrefix(addr, "T") || strings.HasPrefix(addr, "t") {
		decoded := base58.Decode(addr)
		if len(decoded) >= 21 {
			return base58.CheckEncode(decoded[1:21], decoded[0])
		}
	}

	// Hex 格式 → Base58
	addr = strings.ToLower(strings.TrimPrefix(addr, "0x"))
	if !strings.HasPrefix(addr, "41") {
		addr = "41" + addr
	}
	b, err := hex.DecodeString(addr)
	if err != nil || len(b) != 21 {
		return addr
	}
	return base58.CheckEncode(b[1:], b[0])
}

// HexToBase58 将 TRON hex 地址转为 Base58Check，供 Parser 直接调用
func HexToBase58(hexAddr string) string {
	return tronCodec{}.Normalize(hexAddr)
}

// ── SOL ──────────────────────────────────────────────────────────────────────

type solCodec struct{}

func (solCodec) Normalize(addr string) string {
	return strings.TrimSpace(addr)
}
