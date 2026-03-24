package matcher

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

func (m *Matcher) StartAddressEventSubscriber(ctx context.Context) {
	go func() {
		sub := m.rdb.Subscribe(ctx, AddressEventChannel)
		defer sub.Close()

		zap.L().Info("开始订阅地址变更事件",
			zap.String("channel", AddressEventChannel),
		)

		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				zap.L().Info("地址事件订阅已停止")
				return
			case msg, ok := <-ch:
				if !ok {
					zap.L().Warn("订阅 channel 关闭，重新订阅")
					sub = m.rdb.Subscribe(ctx, AddressEventChannel)
					ch = sub.Channel()
					continue
				}
				m.handleAddressEvent(ctx, msg.Payload)
			}
		}
	}()
}

func (m *Matcher) handleAddressEvent(ctx context.Context, payload string) {
	event, err := DecodeAddressEvent(payload)
	if err != nil {
		zap.L().Warn("解析地址事件失败",
			zap.String("payload", payload),
			zap.Error(err),
		)
		return
	}

	switch event.Type {
	case EventTypeAdd:
		// 单个地址添加
		m.AddToBF(event.Chain, event.Address)

	case EventTypeBatchAdd:
		// 批量添加：从增量日志读取最新 count 条地址补入 BF
		m.handleBatchAdd(ctx, event.Chain, event.Count)

	case EventTypeRemove:
		// 从热集合移除（BF 不支持删除）
		m.rdb.SRem(ctx,
			fmt.Sprintf("watch:hot:%s", event.Chain),
			event.Address,
		)
		zap.L().Info("收到删除地址事件，已从热集合移除",
			zap.String("chain", event.Chain),
			zap.String("address", event.Address),
		)

	default:
		zap.L().Warn("未知地址事件类型", zap.String("type", event.Type))
	}
}

// AddToBF Worker 收到 Pub/Sub 消息后调用，更新 BF
func (m *Matcher) AddToBF(chain, address string) {
	key := chain + strings.ToLower(address)
	m.bf.Add(key)
	zap.L().Info("新地址加入 Bloom Filter",
		zap.String("chain", chain),
		zap.String("address", address),
	)
}

func (m *Matcher) handleBatchAdd(ctx context.Context, chain string, count int) {
	if count <= 0 {
		return
	}

	incrKey := fmt.Sprintf("bf:incremental:%s", chain)

	// 读取最新 count 条增量日志
	addrs, err := m.rdb.LRange(ctx, incrKey, 0, int64(count-1)).Result()
	if err != nil {
		zap.L().Error("读取增量日志失败",
			zap.String("chain", chain),
			zap.Error(err),
		)
		return
	}

	added := 0
	for _, addr := range addrs {
		key := chain + addr
		if !m.bf.Test(key) {
			m.bf.Add(key)
			added++
		}
	}

	zap.L().Info("批量导入地址已更新 BF",
		zap.String("chain", chain),
		zap.Int("received", count),
		zap.Int("added_to_bf", added),
	)
}
