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
	bfs          map[string]*bloom.Filter // 每条链独立一个 BF
	lastDBSyncID uint64                   // MySQL 增量同步水位线
	rdb          *redis.Client
	addrStore    *store.WatchedAddressStore
}

func New(rdb *redis.Client, addrStore *store.WatchedAddressStore, chains []string) *Matcher {
	bfs := make(map[string]*bloom.Filter, len(chains))
	for _, chain := range chains {
		bfs[strings.ToUpper(chain)] = bloom.New(10_000_000, 0.001)
	}
	return &Matcher{
		bfs:       bfs,
		rdb:       rdb,
		addrStore: addrStore,
	}
}

// IsWatched 三层漏斗
func (m *Matcher) IsWatched(ctx context.Context, chain, address string) ([]*store.WatchedAddress, error) {
	addr := strings.ToLower(address)

	// 第一层：Bloom Filter（按链独立）
	bf, ok := m.bfs[chain]
	if !ok || !bf.Test(addr) {
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
	bf, ok := m.bfs[chain]
	if !ok {
		zap.L().Warn("AddToBF: 未知链", zap.String("chain", chain))
		return
	}
	bf.Add(strings.ToLower(address))
	zap.L().Info("新地址加入 Bloom Filter",
		zap.String("chain", chain),
		zap.String("address", address),
	)
}

// LoadFromDB Worker 启动时全量加载，完成后设置 lastDBSyncID 水位线
func (m *Matcher) LoadFromDB(ctx context.Context) error {
	page := 1
	size := 10000
	total := 0
	var maxID uint64
	for {
		was, count, err := m.addrStore.ListByApp(ctx, 0, "", page, size)
		if err != nil {
			return err
		}
		for _, wa := range was {
			bf, ok := m.bfs[wa.Chain]
			if !ok {
				continue
			}
			bf.Add(strings.ToLower(wa.Address))
			if wa.ID > maxID {
				maxID = wa.ID
			}
		}
		total += len(was)
		if int64(page*size) >= count {
			break
		}
		page++
	}
	m.lastDBSyncID = maxID
	zap.L().Info("BF 全量构建完成", zap.Int("total", total), zap.Uint64("max_id", maxID))
	return nil
}

// SnapshotBF 将每条链的 BF 独立序列化到 Redis，同时保存 lastDBSyncID 水位线
func (m *Matcher) SnapshotBF(ctx context.Context) error {
	var lastErr error
	for chain, bf := range m.bfs {
		data, err := bf.Encode()
		if err != nil {
			zap.L().Warn("BF 序列化失败", zap.String("chain", chain), zap.Error(err))
			lastErr = err
			continue
		}
		if err := m.rdb.Set(ctx, "bf:snapshot:"+chain, data, 0).Err(); err != nil {
			zap.L().Warn("BF 快照写入 Redis 失败", zap.String("chain", chain), zap.Error(err))
			lastErr = err
		}
	}
	// 保存水位线，供重启后增量补偿使用
	if err := m.rdb.Set(ctx, "bf:snapshot:maxid", m.lastDBSyncID, 0).Err(); err != nil {
		zap.L().Warn("BF 水位线写入 Redis 失败", zap.Error(err))
		lastErr = err
	}
	return lastErr
}

// RestoreBF 从各链独立快照恢复，恢复后从 MySQL 补偿快照之后的新增地址
func (m *Matcher) RestoreBF(ctx context.Context) (bool, error) {
	anyRestored := false
	for chain, bf := range m.bfs {
		data, err := m.rdb.Get(ctx, "bf:snapshot:"+chain).Bytes()
		if err != nil {
			continue // 该链无快照，跳过
		}
		if err := bf.Decode(data); err != nil {
			zap.L().Warn("BF 快照恢复失败", zap.String("chain", chain), zap.Error(err))
			continue
		}
		zap.L().Info("BF 快照恢复成功", zap.String("chain", chain))
		anyRestored = true
	}

	if !anyRestored {
		return false, nil
	}

	// 读取快照时的水位线，从 MySQL 补偿快照之后新增的地址
	snapshotMaxID, err := m.rdb.Get(ctx, "bf:snapshot:maxid").Uint64()
	if err != nil {
		zap.L().Warn("读取 BF 水位线失败，跳过增量补偿", zap.Error(err))
		return true, nil
	}
	m.lastDBSyncID = snapshotMaxID
	m.syncFromDB(ctx)

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

// StartDBIncrementalSync 定时从 MySQL 增量同步（每5分钟），兜底 Redis 不可用时的漏检
func (m *Matcher) StartDBIncrementalSync(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncFromDB(ctx)
		}
	}
}

// syncFromDB 从 MySQL 拉取 lastDBSyncID 之后的新增地址补入 BF，并推进水位线
func (m *Matcher) syncFromDB(ctx context.Context) {
	sinceID := m.lastDBSyncID
	was, err := m.addrStore.ListAfterID(ctx, sinceID)
	if err != nil {
		zap.L().Warn("MySQL 增量同步失败", zap.Error(err))
		return
	}
	if len(was) == 0 {
		return
	}

	added := 0
	var maxID uint64
	for _, wa := range was {
		if bf, ok := m.bfs[wa.Chain]; ok {
			bf.Add(strings.ToLower(wa.Address))
			added++
		}
		if wa.ID > maxID {
			maxID = wa.ID
		}
	}
	m.lastDBSyncID = maxID

	zap.L().Info("MySQL 增量同步完成",
		zap.Uint64("since_id", sinceID),
		zap.Uint64("max_id", maxID),
		zap.Int("added", added),
	)
}

func (m *Matcher) syncIncremental(ctx context.Context) {
	for chain, bf := range m.bfs {
		incrKey := fmt.Sprintf("bf:incremental:%s", chain)
		addrs, err := m.rdb.LRange(ctx, incrKey, 0, -1).Result()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if !bf.Test(addr) {
				bf.Add(addr)
				zap.L().Debug("增量日志补偿",
					zap.String("chain", chain),
					zap.String("address", addr),
				)
			}
		}
	}
}

// StartColdDowngradeJob 热降冷定时任务（每天）
func (m *Matcher) StartColdDowngradeJob(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for chain := range m.bfs {
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
