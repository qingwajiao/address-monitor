package parser

import "context"

// Parser 将 RawEvent 解析为 0 到多个 NormalizedEvent
// 一笔交易可能产生多个事件（如同时有转入和转出）
type Parser interface {
	Parse(ctx context.Context, raw RawEvent) ([]*NormalizedEvent, error)
}
