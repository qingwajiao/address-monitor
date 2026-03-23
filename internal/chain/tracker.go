package chain

import (
	"context"
	"fmt"
	"sync/atomic"

	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type BlockTracker struct {
	chain      string
	instanceID string
	rdb        *redis.Client
	syncStore  *store.ChainSyncStore
	current    atomic.Uint64
	counter    atomic.Int64
}

func NewBlockTracker(chain, instanceID string, rdb *redis.Client, syncStore *store.ChainSyncStore) *BlockTracker {
	return &BlockTracker{
		chain:      chain,
		instanceID: instanceID,
		rdb:        rdb,
		syncStore:  syncStore,
	}
}

// Init 启动时确定起始块号，三级降级
// fetchLatest 由各链提供，用于获取链上当前最新块号
func (t *BlockTracker) Init(ctx context.Context, fetchLatest func() (uint64, error)) (uint64, error) {
	key := fmt.Sprintf("last_block:%s", t.chain)

	// 第一级：Redis
	if val, err := t.rdb.Get(ctx, key).Uint64(); err == nil && val > 0 {
		start := val - safeBuffer(t.chain)
		t.current.Store(start)
		zap.L().Info("从 Redis 恢复块号",
			zap.String("chain", t.chain),
			zap.Uint64("last", val),
			zap.Uint64("start", start),
		)
		return start, nil
	}

	// 第二级：MySQL
	if last, err := t.syncStore.GetLastBlock(ctx, t.chain, t.instanceID); err == nil && last > 0 {
		start := last - safeBuffer(t.chain)
		t.current.Store(start)
		zap.L().Info("从 MySQL 恢复块号",
			zap.String("chain", t.chain),
			zap.Uint64("last", last),
			zap.Uint64("start", start),
		)
		return start, nil
	}

	// 第三级：从链上最新块开始
	latest, err := fetchLatest()
	if err != nil {
		return 0, fmt.Errorf("获取最新块号失败: %w", err)
	}
	t.current.Store(latest)
	zap.L().Info("首次启动，从最新块开始",
		zap.String("chain", t.chain),
		zap.Uint64("latest", latest),
	)
	return latest, nil
}

// Update 每块处理完成后调用
func (t *BlockTracker) Update(ctx context.Context, blockNum uint64) {
	t.current.Store(blockNum)

	// 实时写 Redis
	key := fmt.Sprintf("last_block:%s", t.chain)
	t.rdb.Set(ctx, key, blockNum, 0)

	// 每 100 块异步持久化到 MySQL
	if t.counter.Add(1)%100 == 0 {
		go func() {

			if err := t.syncStore.UpsertLastBlock(
				context.Background(),
				t.chain,
				t.instanceID,
				blockNum,
			); err != nil {
				zap.L().Warn("同步块号到 MySQL 失败",
					zap.String("chain", t.chain),
					zap.Error(err),
				)
			}
		}()
	}
}

// Get 获取当前处理的块号
func (t *BlockTracker) Get() uint64 {
	return t.current.Load()
}

// safeBuffer 重启时往前退几个块，防止漏块
func safeBuffer(chain string) uint64 {
	switch chain {
	case "ETH":
		return 12
	case "BSC":
		return 15
	case "TRON":
		return 20
	case "SOL":
		return 100
	default:
		return 10
	}
}
