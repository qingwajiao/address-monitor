package matcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"address-monitor/internal/store"
	"address-monitor/pkg/bloom"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type Matcher struct {
	bf       *bloom.Filter
	rdb      *redis.Client
	subStore *store.SubscriptionStore
}

func New(rdb *redis.Client, subStore *store.SubscriptionStore) *Matcher {
	return &Matcher{
		bf:       bloom.New(10_000_000, 0.001), // 1000万地址，0.1% 误判率
		rdb:      rdb,
		subStore: subStore,
	}
}

// IsWatched 三层漏斗，返回命中的订阅列表
func (m *Matcher) IsWatched(ctx context.Context, chain, address string) ([]*store.Subscription, error) {
	addr := strings.ToLower(address)
	key := chain + addr

	// 第一层：Bloom Filter
	if !m.bf.Test(key) {
		zap.L().Debug("BF 未命中",
			zap.String("chain", chain),
			zap.String("address", addr),
		)
		return nil, nil
	}

	// 第二层：Redis 热集合
	hotKey := fmt.Sprintf("watch:hot:%s", chain)
	isMember, err := m.rdb.SIsMember(ctx, hotKey, addr).Result()
	if err == nil && isMember {
		zap.L().Debug("热地址命中",
			zap.String("chain", chain),
			zap.String("address", addr),
		)
		return m.subStore.ListByChainAddress(ctx, chain, addr)
	}

	// 第三层：MySQL 冷地址
	subs, err := m.subStore.ListByChainAddress(ctx, chain, addr)
	if err != nil || len(subs) == 0 {
		zap.L().Debug("BF 误判（地址不在监控列表）",
			zap.String("chain", chain),
			zap.String("address", addr),
		)
		return nil, err
	}

	zap.L().Info("冷地址命中，异步升热",
		zap.String("chain", chain),
		zap.String("address", addr),
	)
	go m.promoteToHot(context.Background(), chain, addr)
	return subs, nil
}

// Add 新增监控地址 todo
func (m *Matcher) Add(ctx context.Context, sub *store.Subscription) error {
	addr := strings.ToLower(sub.Address)
	sub.Address = addr
	key := sub.Chain + addr

	if err := m.subStore.Create(ctx, sub); err != nil {
		return err
	}

	// 写 Bloom Filter
	m.bf.Add(key)

	zap.L().Info("新地址加入 Bloom Filter",
		zap.String("chain", sub.Chain),
		zap.String("address", addr),
	)

	// 写增量日志（Worker 重启时补偿）
	incrKey := fmt.Sprintf("bf:incremental:%s", sub.Chain)
	m.rdb.LPush(ctx, incrKey, addr)

	return nil
}

// Remove 删除监控地址
func (m *Matcher) Remove(ctx context.Context, id uint64, chain, address string) error {
	addr := strings.ToLower(address)

	if err := m.subStore.Delete(ctx, id); err != nil {
		return err
	}

	// 从热集合移除
	m.rdb.SRem(ctx, fmt.Sprintf("watch:hot:%s", chain), addr)

	// 清 Dispatcher 订阅缓存
	m.rdb.Del(ctx, fmt.Sprintf("sub_cache:%s:%s", chain, addr))

	return nil
}

// LoadFromDB Worker 启动时从 MySQL 全量加载地址到 Bloom Filter
func (m *Matcher) LoadFromDB(ctx context.Context) error {
	page := 1
	size := 10000
	for {
		subs, total, err := m.subStore.ListByUser(ctx, "", page, size)
		if err != nil {
			return err
		}
		for _, sub := range subs {
			key := sub.Chain + strings.ToLower(sub.Address)
			m.bf.Add(key)
		}
		if int64(page*size) >= total {
			break
		}
		page++
	}
	return nil
}

// SnapshotBF 序列化 Bloom Filter 到 Redis
func (m *Matcher) SnapshotBF(ctx context.Context) error {
	data, err := m.bf.Encode()
	if err != nil {
		return err
	}
	return m.rdb.Set(ctx, "bf:snapshot", data, 0).Err()
}

// RestoreBF 从 Redis 快照恢复 Bloom Filter
// 返回 true 表示恢复成功，false 表示需要全量构建
func (m *Matcher) RestoreBF(ctx context.Context) (bool, error) {
	data, err := m.rdb.Get(ctx, "bf:snapshot").Bytes()
	if err != nil {
		return false, nil // 快照不存在
	}
	if err := m.bf.Decode(data); err != nil {
		return false, err
	}

	// 补充增量日志（快照之后新增的地址）
	for _, chain := range []string{"ETH", "BSC", "TRON", "SOL"} {
		incrKey := fmt.Sprintf("bf:incremental:%s", chain)
		addrs, err := m.rdb.LRange(ctx, incrKey, 0, -1).Result()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			m.bf.Add(chain + addr)
		}
	}
	return true, nil
}

// AddToHotSet 直接加入热地址集合（API 新增地址时调用）
func (m *Matcher) AddToHotSet(ctx context.Context, chain, address string) {
	addr := strings.ToLower(address)
	m.rdb.SAdd(ctx, fmt.Sprintf("watch:hot:%s", chain), addr)
}

// StartSnapshotJob 每 5 分钟定时快照 BF
func (m *Matcher) StartSnapshotJob(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.SnapshotBF(ctx)
		}
	}
}

// StartColdDowngradeJob 每天清理超过 7 天未活跃的热地址
func (m *Matcher) StartColdDowngradeJob(ctx context.Context, chains []string) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, chain := range chains {
				m.downgradeHotToCold(ctx, chain)
			}
		}
	}
}

func (m *Matcher) downgradeHotToCold(ctx context.Context, chain string) {
	hotKey := fmt.Sprintf("watch:hot:%s", chain)
	lastActiveKey := fmt.Sprintf("hot:last_active:%s", chain)

	members, err := m.rdb.SMembers(ctx, hotKey).Result()
	if err != nil {
		return
	}

	threshold := time.Now().Add(-7 * 24 * time.Hour).Unix()
	for _, addr := range members {
		lastActive, err := m.rdb.HGet(ctx, lastActiveKey, addr).Int64()
		if err != nil || lastActive < threshold {
			m.rdb.SRem(ctx, hotKey, addr)
		}
	}
}

func (m *Matcher) promoteToHot(ctx context.Context, chain, address string) {
	hotKey := fmt.Sprintf("watch:hot:%s", chain)
	lastActiveKey := fmt.Sprintf("hot:last_active:%s", chain)
	m.rdb.SAdd(ctx, hotKey, address)
	m.rdb.HSet(ctx, lastActiveKey, address, time.Now().Unix())
}

// UpdateLastActive 更新地址最后活跃时间（地址命中时调用）
func (m *Matcher) UpdateLastActive(ctx context.Context, chain, address string) {
	lastActiveKey := fmt.Sprintf("hot:last_active:%s", chain)
	m.rdb.HSet(ctx, lastActiveKey, strings.ToLower(address), time.Now().Unix())
}

// GetHotAddresses 获取某链的热地址列表（用于 eth_getLogs 过滤参数）
func (m *Matcher) GetHotAddresses(ctx context.Context, chain string) ([]string, error) {
	hotKey := fmt.Sprintf("watch:hot:%s", chain)
	return m.rdb.SMembers(ctx, hotKey).Result()
}

// 确保 json 包被使用
var _ = json.Marshal
