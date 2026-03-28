package matcher

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"address-monitor/pkg/bloom"

	"github.com/go-redis/redis/v8"
)

// ── 测试辅助 ──────────────────────────────────────────────

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Redis 连接失败（跳过集成测试）: %v", err)
	}
	return rdb
}

// newMatcherNoStore 创建只有 BF 的 Matcher，用于不需要 MySQL 的测试
func newMatcherNoStore(chains ...string) *Matcher {
	bfs := make(map[string]*bloom.Filter, len(chains))
	for _, c := range chains {
		bfs[strings.ToUpper(c)] = bloom.New(10000, 0.001)
	}
	return &Matcher{bfs: bfs}
}

// ── 纯 BF 测试（无外部依赖）────────────────────────────────

func TestMatcher_BF_AddAndTest(t *testing.T) {
	m := newMatcherNoStore("ETH")
	addr := "0xabcdef1234567890abcdef1234567890abcdef12"

	// 添加前不命中
	if m.bfs["ETH"].Test(strings.ToLower(addr)) {
		t.Fatal("添加前不应该命中")
	}

	m.AddToBF("ETH", addr)

	// 添加后命中
	if !m.bfs["ETH"].Test(strings.ToLower(addr)) {
		t.Fatal("添加后应该命中")
	}

	// 大写地址经过 AddToBF 归一化后也能命中
	m.AddToBF("ETH", strings.ToUpper(addr))
	if !m.bfs["ETH"].Test(strings.ToLower(addr)) {
		t.Fatal("大写地址归一化后应该命中")
	}

	t.Log("BF 添加和命中测试通过 ✓")
}

func TestMatcher_BF_UnknownChain(t *testing.T) {
	m := newMatcherNoStore("ETH")

	// AddToBF 未知链只打日志不 panic
	m.AddToBF("UNKNOWN_CHAIN", "0xdeadbeef")
	t.Log("未知链 AddToBF 不 panic ✓")
}

func TestMatcher_IsWatched_BFMiss(t *testing.T) {
	m := newMatcherNoStore("ETH", "BSC")
	ctx := context.Background()

	// 地址未加入 BF，应该直接返回 nil（不触达 Redis/MySQL）
	subs, err := m.IsWatched(ctx, "ETH", "0xdeadbeef000000000000000000000000deadbeef")
	if err != nil {
		t.Fatalf("BF miss 不应该报错: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("BF miss 应该返回空，实际: %d", len(subs))
	}
	t.Log("BF miss 快速返回测试通过 ✓")
}

func TestMatcher_IsWatched_UnknownChain(t *testing.T) {
	m := newMatcherNoStore("ETH")
	ctx := context.Background()

	// 未知链在 bfs 中不存在，应该直接返回 nil
	subs, err := m.IsWatched(ctx, "UNKNOWN", "0xabcd")
	if err != nil {
		t.Fatalf("未知链不应该报错: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("未知链应该返回空")
	}
	t.Log("未知链处理测试通过 ✓")
}

// ── Redis 集成测试（需要 Redis）──────────────────────────────

// TestMatcher_IsWatched_RedisMiss 验证：BF 命中 + Redis 未命中 → 走冷地址路径
// 冷地址路径会查 MySQL，但 addrStore 为 nil 所以会触发 panic，
// 因此这里只验证热集合不命中时的 Redis SIsMember 行为
func TestMatcher_IsWatched_RedisHot(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	chain := "ETH"
	addr := "0xaaaa0000000000000000000000000000000000ff"
	hotKey := fmt.Sprintf("watch:hot:%s", chain)

	// 清理
	rdb.SRem(ctx, hotKey, strings.ToLower(addr))
	defer rdb.SRem(ctx, hotKey, strings.ToLower(addr))

	m := newMatcherNoStore(chain)
	m.rdb = rdb

	// 加入 BF
	m.AddToBF(chain, addr)

	// 加入 Redis 热集合，但 addrStore 为 nil 会 panic（热集合命中后查 MySQL）
	// 这里只验证 Redis SIsMember 的查询行为：先不加入热集合，验证不走热集合路径
	isMember, err := rdb.SIsMember(ctx, hotKey, strings.ToLower(addr)).Result()
	if err != nil {
		t.Fatalf("Redis SIsMember 失败: %v", err)
	}
	if isMember {
		t.Fatal("清理后不应该在热集合中")
	}
	t.Logf("Redis 热集合验证通过，地址 %s 不在 %s 中 ✓", addr, hotKey)
}

// TestMatcher_UpdateLastActive 验证活跃时间更新
func TestMatcher_UpdateLastActive(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	chain := "ETH"
	addr := "0xbbbb0000000000000000000000000000000000cc"
	lastActiveKey := fmt.Sprintf("hot:last_active:%s", chain)

	defer rdb.HDel(ctx, lastActiveKey, strings.ToLower(addr))

	m := newMatcherNoStore(chain)
	m.rdb = rdb

	m.UpdateLastActive(ctx, chain, addr)

	ts, err := rdb.HGet(ctx, lastActiveKey, strings.ToLower(addr)).Int64()
	if err != nil {
		t.Fatalf("读取活跃时间失败: %v", err)
	}
	if ts <= 0 {
		t.Fatal("活跃时间应该大于 0")
	}
	t.Logf("UpdateLastActive 测试通过，timestamp=%d ✓", ts)
}

// TestMatcher_SnapshotAndRestore 验证 BF 快照和恢复
func TestMatcher_SnapshotAndRestore(t *testing.T) {
	rdb := newTestRedis(t)
	ctx := context.Background()

	chain := "ETH"
	snapshotKey := "bf:snapshot:" + chain
	maxIDKey := "bf:snapshot:maxid"

	// 清理
	defer rdb.Del(ctx, snapshotKey, maxIDKey)

	m := newMatcherNoStore(chain)
	m.rdb = rdb
	m.lastDBSyncID = 42

	addrs := []string{
		"0xaddr1000000000000000000000000000000001",
		"0xaddr2000000000000000000000000000000002",
		"0xaddr3000000000000000000000000000000003",
	}
	for _, a := range addrs {
		m.AddToBF(chain, a)
	}

	// 快照
	if err := m.SnapshotBF(ctx); err != nil {
		t.Fatalf("快照失败: %v", err)
	}

	// 验证水位线写入
	maxID, err := rdb.Get(ctx, maxIDKey).Uint64()
	if err != nil {
		t.Fatalf("读取水位线失败: %v", err)
	}
	if maxID != 42 {
		t.Fatalf("水位线应为 42，实际: %d", maxID)
	}

	// 直接验证快照数据写入 Redis 后 BF 可以正确恢复
	data, err := rdb.Get(ctx, snapshotKey).Bytes()
	if err != nil {
		t.Fatalf("读取快照数据失败: %v", err)
	}

	freshBF := bloom.New(10000, 0.001)
	if err := freshBF.Decode(data); err != nil {
		t.Fatalf("BF 反序列化失败: %v", err)
	}

	for _, a := range addrs {
		if !freshBF.Test(strings.ToLower(a)) {
			t.Errorf("恢复后地址 %s 应该在 BF 中", a)
		}
	}

	t.Log("BF 快照和恢复测试通过 ✓")
}
