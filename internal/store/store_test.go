package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	dsn := "root:root@tcp(127.0.0.1:3306)/address_monitor?charset=utf8mb4&parseTime=True&loc=UTC"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接数据库失败: %v", err)
	}
	return db
}

// ── Subscription ──────────────────────────────────────────

func TestSubscriptionStore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := NewSubscriptionStore(db)

	// 创建
	sub := &Subscription{
		UserID:      "user_test_001",
		Chain:       "ETH",
		Address:     "0xabc123",
		Label:       "测试地址",
		CallbackURL: "https://example.com/callback",
		Secret:      "test-secret",
		Status:      1,
	}
	if err := s.Create(ctx, sub); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	t.Logf("创建订阅成功，ID: %d", sub.ID)

	// 查询单条
	got, err := s.GetByID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetByID 失败: %v", err)
	}
	if got.Address != "0xabc123" {
		t.Fatalf("地址不匹配: %s", got.Address)
	}
	t.Log("查询单条成功 ✓")

	// 按链和地址查询
	subs, err := s.ListByChainAddress(ctx, "ETH", "0xabc123")
	if err != nil || len(subs) == 0 {
		t.Fatalf("ListByChainAddress 失败: %v", err)
	}
	t.Logf("按链地址查询成功，找到 %d 条 ✓", len(subs))

	// 按用户分页查询
	list, total, err := s.ListByUser(ctx, "user_test_001", 1, 10)
	if err != nil || total == 0 {
		t.Fatalf("ListByUser 失败: %v", err)
	}
	t.Logf("按用户查询成功，共 %d 条 ✓", total)
	_ = list

	// 软删除
	if err := s.Delete(ctx, sub.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	// 删除后 ListByChainAddress 应该查不到（status=0 被过滤）
	subs, _ = s.ListByChainAddress(ctx, "ETH", "0xabc123")
	if len(subs) != 0 {
		t.Fatal("软删除后不应该查到数据")
	}
	t.Log("软删除成功 ✓")

	t.Log("SubscriptionStore 全部测试通过 ✓")
}

// ── DeliveryLog ───────────────────────────────────────────

func TestDeliveryStore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := NewDeliveryStore(db)

	eventID := fmt.Sprintf("test-event-%d", time.Now().UnixNano())

	log := &DeliveryLog{
		EventID:        eventID,
		SubscriptionID: 1,
		Chain:          "ETH",
		TxHash:         "0xtesthash",
		EventType:      "TOKEN_TRANSFER",
		Payload:        `{"test":"data"}`,
		Status:         "pending",
		CallbackURL:    "https://example.com/callback",
	}
	if err := s.Create(ctx, log); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	t.Logf("创建 DeliveryLog 成功，ID: %d", log.ID)

	// 幂等检查
	exists, err := s.ExistsByEventAndSub(ctx, eventID, 1)
	if err != nil || !exists {
		t.Fatalf("ExistsByEventAndSub 失败: %v", err)
	}
	t.Log("幂等检查成功 ✓")

	// 不存在的 eventID
	exists, _ = s.ExistsByEventAndSub(ctx, "not-exist", 1)
	if exists {
		t.Fatal("不存在的 eventID 不应该命中")
	}
	t.Log("不存在的 eventID 正确返回 false ✓")

	// 更新状态
	code := 200
	if err := s.UpdateStatus(ctx, log.ID, "success", code, `{"ok":true}`); err != nil {
		t.Fatalf("UpdateStatus 失败: %v", err)
	}
	t.Log("更新状态成功 ✓")

	// 增加重试次数
	if err := s.IncrRetryCount(ctx, log.ID); err != nil {
		t.Fatalf("IncrRetryCount 失败: %v", err)
	}
	t.Log("增加重试次数成功 ✓")

	// 标记死信
	if err := s.MarkDead(ctx, log.ID); err != nil {
		t.Fatalf("MarkDead 失败: %v", err)
	}
	t.Log("标记死信成功 ✓")

	t.Log("DeliveryStore 全部测试通过 ✓")
}

// ── RawEvent ──────────────────────────────────────────────

func TestRawEventStore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := NewRawEventStore(db)

	e := &RawEvent{
		Chain:       "ETH",
		TxHash:      "0xtesthash123",
		BlockNumber: 12345678,
		BlockTime:   uint32(time.Now().Unix()),
		EventType:   "TOKEN_TRANSFER",
		RawData:     `{"raw":"data"}`,
	}
	if err := s.Insert(ctx, e); err != nil {
		t.Fatalf("Insert 失败: %v", err)
	}
	t.Logf("插入 RawEvent 成功，ID: %d", e.ID)

	// 按时间范围查询
	from := time.Now().Add(-1 * time.Minute)
	to := time.Now().Add(1 * time.Minute)
	events, err := s.ListByTimeRange(ctx, "ETH", from, to)
	if err != nil || len(events) == 0 {
		t.Fatalf("ListByTimeRange 失败: %v", err)
	}
	t.Logf("按时间范围查询成功，找到 %d 条 ✓", len(events))

	// 清理测试数据
	affected, err := s.DeleteBefore(ctx, time.Now().Add(1*time.Minute))
	if err != nil {
		t.Fatalf("DeleteBefore 失败: %v", err)
	}
	t.Logf("清理成功，删除 %d 条 ✓", affected)

	t.Log("RawEventStore 全部测试通过 ✓")
}

// ── ChainSyncStatus ───────────────────────────────────────

func TestChainSyncStore(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := NewChainSyncStore(db)

	chain := "ETH"
	instanceID := "worker-eth-test-001"

	// 首次查询，应该返回错误（记录不存在）
	_, err := s.GetLastBlock(ctx, chain, instanceID)
	if err == nil {
		t.Log("记录已存在，继续测试")
	} else {
		t.Log("首次查询记录不存在（符合预期）✓")
	}

	// Upsert 第一次（插入）
	if err := s.UpsertLastBlock(ctx, chain, instanceID, 1000); err != nil {
		t.Fatalf("UpsertLastBlock 插入失败: %v", err)
	}
	t.Log("Upsert 插入成功 ✓")

	// 查询
	block, err := s.GetLastBlock(ctx, chain, instanceID)
	if err != nil || block != 1000 {
		t.Fatalf("GetLastBlock 失败: block=%d, err=%v", block, err)
	}
	t.Logf("GetLastBlock 成功，块号: %d ✓", block)

	// Upsert 第二次（更新）
	if err := s.UpsertLastBlock(ctx, chain, instanceID, 2000); err != nil {
		t.Fatalf("UpsertLastBlock 更新失败: %v", err)
	}
	block, _ = s.GetLastBlock(ctx, chain, instanceID)
	if block != 2000 {
		t.Fatalf("更新后块号应为 2000，实际: %d", block)
	}
	t.Logf("Upsert 更新成功，块号: %d ✓", block)

	// 清理测试数据
	db.Where("chain = ? AND instance_id = ?", chain, instanceID).Delete(&ChainSyncStatus{})
	t.Log("ChainSyncStore 全部测试通过 ✓")
}
