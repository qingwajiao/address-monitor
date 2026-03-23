package matcher

import (
	"context"
	"testing"

	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newTestDeps(t *testing.T) (*redis.Client, *store.SubscriptionStore) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Redis 连接失败: %v", err)
	}

	dsn := "root:root@tcp(127.0.0.1:3306)/address_monitor?charset=utf8mb4&parseTime=True&loc=UTC"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("MySQL 连接失败: %v", err)
	}

	return rdb, store.NewSubscriptionStore(db)
}

func TestMatcher_AddAndIsWatched(t *testing.T) {
	rdb, subStore := newTestDeps(t)
	ctx := context.Background()
	m := New(rdb, subStore)

	// 清理测试数据
	rdb.SRem(ctx, "watch:hot:ETH", "0xtest001")
	rdb.Del(ctx, "bf:incremental:ETH")

	sub := &store.Subscription{
		UserID:      "test_user",
		Chain:       "ETH",
		Address:     "0xTEST001", // 大写，应该自动转小写
		CallbackURL: "https://example.com/callback",
		Secret:      "test-secret",
		Status:      1,
	}

	// 添加地址
	if err := m.Add(ctx, sub); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	t.Logf("地址添加成功，ID: %d", sub.ID)

	// 第一层：Bloom Filter 应该能命中
	if !m.bf.Test("ETH" + "0xtest001") {
		t.Error("Bloom Filter 应该命中")
	}
	t.Log("Bloom Filter 命中 ✓")

	// 加入热集合
	m.AddToHotSet(ctx, "ETH", "0xtest001")

	// IsWatched 应该返回订阅
	subs, err := m.IsWatched(ctx, "ETH", "0xTEST001") // 大写测试
	if err != nil {
		t.Fatalf("IsWatched 失败: %v", err)
	}
	if len(subs) == 0 {
		t.Fatal("应该找到订阅")
	}
	t.Logf("IsWatched 命中，找到 %d 个订阅 ✓", len(subs))

	// 不存在的地址不应该命中
	subs, _ = m.IsWatched(ctx, "ETH", "0xnotexist999")
	if len(subs) != 0 {
		t.Error("不存在的地址不应该命中")
	}
	t.Log("不存在地址正确返回空 ✓")

	// 清理
	m.Remove(ctx, sub.ID, "ETH", "0xtest001")
	t.Log("Matcher AddAndIsWatched 测试通过 ✓")
}

func TestMatcher_SnapshotAndRestore(t *testing.T) {
	rdb, subStore := newTestDeps(t)
	ctx := context.Background()
	m := New(rdb, subStore)

	// 添加一些地址到 BF
	for _, addr := range []string{"0xaaa", "0xbbb", "0xccc"} {
		m.bf.Add("ETH" + addr)
	}

	// 快照
	if err := m.SnapshotBF(ctx); err != nil {
		t.Fatalf("快照失败: %v", err)
	}
	t.Log("BF 快照成功 ✓")

	// 新建 Matcher，从快照恢复
	m2 := New(rdb, subStore)
	ok, err := m2.RestoreBF(ctx)
	if err != nil || !ok {
		t.Fatalf("恢复快照失败: ok=%v, err=%v", ok, err)
	}
	t.Log("BF 快照恢复成功 ✓")

	// 验证恢复后数据一致
	for _, addr := range []string{"0xaaa", "0xbbb", "0xccc"} {
		if !m2.bf.Test("ETH" + addr) {
			t.Errorf("恢复后地址应该能命中: %s", addr)
		}
	}
	t.Log("恢复后数据一致 ✓")

	// 清理
	rdb.Del(ctx, "bf:snapshot")
	t.Log("Matcher SnapshotAndRestore 测试通过 ✓")
}
