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
	bf        *bloom.Filter
	rdb       *redis.Client
	addrStore *store.WatchedAddressStore
}

func New(rdb *redis.Client, addrStore *store.WatchedAddressStore) *Matcher {
	return &Matcher{
		bf:        bloom.New(10_000_000, 0.001),
		rdb:       rdb,
		addrStore: addrStore,
	}
}

// IsWatched 三层漏斗
func (m *Matcher) IsWatched(ctx context.Context, chain, address string) ([]*store.WatchedAddress, error) {
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
		return m.addrStore.ListByChainAddress(ctx, chain, addr)
	}

	// 第三层：MySQL 冷地址
	was, err := m.addrStore.ListByChainAddress(ctx, chain, addr)
	if err != nil || len(was) == 0 {
		zap.L().Debug("BF 误判",
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
	return was, nil
}

// AddToBF Worker 收到 Pub/Sub 消息后调用
func (m *Matcher) AddToBF(chain, address string) {
	key := chain + strings.ToLower(address)
	m.bf.Add(key)
	zap.L().Info("新地址加入 Bloom Filter",
		zap.String("chain", chain),
		zap.String("address", address),
	)
}

// LoadFromDB Worker 启动时全量加载
func (m *Matcher) LoadFromDB(ctx context.Context) error {
	page := 1
	size := 10000
	total := 0
	for {
		was, count, err := m.addrStore.ListByApp(ctx, 0, "", page, size)
		if err != nil {
			return err
		}
		for _, wa := range was {
			key := wa.Chain + strings.ToLower(wa.Address)
			m.bf.Add(key)
		}
		total += len(was)
		if int64(page*size) >= count {
			break
		}
		page++
	}
	zap.L().Info("BF 全量构建完成", zap.Int("total", total))
	return nil
}

// SnapshotBF 序列化 BF 到 Redis
func (m *Matcher) SnapshotBF(ctx context.Context) error {
	data, err := m.bf.Encode()
	if err != nil {
		return err
	}
	return m.rdb.Set(ctx, "bf:snapshot", data, 0).Err()
}

// RestoreBF 从快照恢复
func (m *Matcher) RestoreBF(ctx context.Context) (bool, error) {
	data, err := m.rdb.Get(ctx, "bf:snapshot").Bytes()
	if err != nil {
		return false, nil
	}
	if err := m.bf.Decode(data); err != nil {
		return false, err
	}
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
	zap.L().Info("BF 从快照恢复成功")
	return true, nil
}

// StartSnapshotJob 定时快照（每5分钟）
func (m *Matcher) StartSnapshotJob(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.SnapshotBF(ctx); err != nil {
				zap.L().Warn("BF 快照失败", zap.Error(err))
			}
		}
	}
}

// StartIncrementalSync 定时读增量日志补偿（每30秒）
func (m *Matcher) StartIncrementalSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncIncremental(ctx)
		}
	}
}

func (m *Matcher) syncIncremental(ctx context.Context) {
	for _, chain := range []string{"ETH", "BSC", "TRON", "SOL"} {
		incrKey := fmt.Sprintf("bf:incremental:%s", chain)
		addrs, err := m.rdb.LRange(ctx, incrKey, 0, -1).Result()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			key := chain + addr
			if !m.bf.Test(key) {
				m.bf.Add(key)
				zap.L().Debug("增量日志补偿",
					zap.String("chain", chain),
					zap.String("address", addr),
				)
			}
		}
	}
}

// StartColdDowngradeJob 热降冷定时任务（每天）
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
	removed := 0
	for _, addr := range members {
		lastActive, err := m.rdb.HGet(ctx, lastActiveKey, addr).Int64()
		if err != nil || lastActive < threshold {
			m.rdb.SRem(ctx, hotKey, addr)
			removed++
		}
	}
	if removed > 0 {
		zap.L().Info("热降冷完成",
			zap.String("chain", chain),
			zap.Int("removed", removed),
		)
	}
}

func (m *Matcher) promoteToHot(ctx context.Context, chain, address string) {
	m.rdb.SAdd(ctx, fmt.Sprintf("watch:hot:%s", chain), address)
	m.rdb.HSet(ctx, fmt.Sprintf("hot:last_active:%s", chain), address, time.Now().Unix())
}

func (m *Matcher) UpdateLastActive(ctx context.Context, chain, address string) {
	m.rdb.HSet(ctx,
		fmt.Sprintf("hot:last_active:%s", chain),
		strings.ToLower(address),
		time.Now().Unix(),
	)
}

func (m *Matcher) AddToHotSet(ctx context.Context, chain, address string) {
	m.rdb.SAdd(ctx, fmt.Sprintf("watch:hot:%s", chain), strings.ToLower(address))
}

func (m *Matcher) GetHotAddresses(ctx context.Context, chain string) ([]string, error) {
	return m.rdb.SMembers(ctx, fmt.Sprintf("watch:hot:%s", chain)).Result()
}

var _ = json.Marshal
