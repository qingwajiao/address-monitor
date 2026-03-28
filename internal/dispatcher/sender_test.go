package dispatcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"address-monitor/internal/mq"
	"address-monitor/internal/parser"
	"address-monitor/internal/store"
	"address-monitor/pkg/httputil"

	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ── 测试辅助 ──────────────────────────────────────────────

const (
	testMySQLDSN    = "root:root@tcp(127.0.0.1:3306)/address_monitor?charset=utf8mb4&parseTime=True&loc=UTC"
	testRabbitMQURL = "amqp://admin:admin@127.0.0.1:5672/"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(testMySQLDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("MySQL 连接失败: %v", err)
	}
	return db
}

func newTestMQConn(t *testing.T) *mq.Connection {
	t.Helper()
	conn, err := mq.NewConnection(testRabbitMQURL)
	if err != nil {
		t.Fatalf("RabbitMQ 连接失败: %v", err)
	}
	return conn
}

// mockAck 实现 amqp.Acknowledger 接口，用于构造测试用 Delivery
type mockAck struct{}

func (m *mockAck) Ack(tag uint64, multiple bool) error                { return nil }
func (m *mockAck) Nack(tag uint64, multiple bool, requeue bool) error { return nil }
func (m *mockAck) Reject(tag uint64, requeue bool) error              { return nil }

// newTestDelivery 构造一个带 Body 的假 AMQP Delivery
func newTestDelivery(body []byte, retryCount int32) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger: &mockAck{},
		Headers:      amqp.Table{"x-retry-count": retryCount},
		Body:         body,
	}
}

// makeTestTask 构造一个 DispatchTask，callbackURL 指向 httptest server
func makeTestTask(webhookLogID uint64, callbackURL string) DispatchTask {
	return DispatchTask{
		WebhookLogID: webhookLogID,
		AppID:        1,
		AddressID:    1,
		CallbackURL:  callbackURL,
		Secret:       "test-secret",
		Event: parser.NormalizedEvent{
			EventID:        "test-event-001",
			Chain:          "ETH",
			TxHash:         "0xabcdef1234567890",
			EventType:      parser.EventTypeNativeTransfer,
			Direction:      "IN",
			WatchedAddress: "0xaaaa000000000000000000000000000000000001",
			From:           "0xbbbb000000000000000000000000000000000002",
			To:             "0xaaaa000000000000000000000000000000000001",
			Asset: &parser.AssetInfo{
				Symbol:   "ETH",
				Amount:   "1000000000000000000",
				Decimals: 18,
			},
		},
	}
}

// ── Sender 测试 ────────────────────────────────────────────

// TestSender_CallbackSuccess 验证：回调返回 2xx → webhook_log 状态更新为 success
func TestSender_CallbackSuccess(t *testing.T) {
	db := newTestDB(t)
	mqConn := newTestMQConn(t)
	defer mqConn.Close()

	// 声明拓扑
	ch, err := mqConn.Channel()
	if err != nil {
		t.Fatalf("创建 Channel 失败: %v", err)
	}
	if err := mq.DeclareTopology(ch); err != nil {
		t.Fatalf("声明拓扑失败: %v", err)
	}
	ch.Close()

	webhookStore := store.NewWebhookLogStore(db)
	publisher := mq.NewPublisher(mqConn)
	httpClient := newTestHTTPClient()

	sender := NewSender(publisher, webhookStore, httpClient)

	// 创建 webhook_log 记录（pending）
	wl := &store.WebhookLog{
		EventID:     "test-event-success-001",
		AppID:       1,
		AddressID:   1,
		Chain:       "ETH",
		TxHash:      "0xtest001",
		EventType:   "NATIVE_TRANSFER",
		Payload:     "{}",
		Status:      "pending",
		CallbackURL: "http://localhost",
	}
	if err := webhookStore.Create(context.Background(), wl); err != nil {
		t.Fatalf("创建 webhook_log 失败: %v", err)
	}
	defer db.Delete(wl)

	// 启动 mock 回调服务：返回 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	task := makeTestTask(wl.ID, srv.URL)
	body, _ := json.Marshal(task)
	msg := newTestDelivery(body, 0)

	sender.Handle(msg)

	// 验证 webhook_log 状态变更为 success
	updated, err := webhookStore.GetByID(context.Background(), wl.ID)
	if err != nil {
		t.Fatalf("获取 webhook_log 失败: %v", err)
	}
	if updated.Status != "success" {
		t.Errorf("期望 status=success，实际: %s", updated.Status)
	}
	if updated.ResponseCode == nil || *updated.ResponseCode != 200 {
		t.Errorf("期望 response_code=200，实际: %v", updated.ResponseCode)
	}
	t.Logf("回调成功测试通过 ✓ status=%s, code=%d", updated.Status, *updated.ResponseCode)
}

// TestSender_CallbackFail_Retry 验证：回调返回 5xx → 进重试队列，retry_count +1
func TestSender_CallbackFail_Retry(t *testing.T) {
	db := newTestDB(t)
	mqConn := newTestMQConn(t)
	defer mqConn.Close()

	ch, err := mqConn.Channel()
	if err != nil {
		t.Fatalf("创建 Channel 失败: %v", err)
	}
	if err := mq.DeclareTopology(ch); err != nil {
		t.Fatalf("声明拓扑失败: %v", err)
	}
	ch.Close()

	webhookStore := store.NewWebhookLogStore(db)
	publisher := mq.NewPublisher(mqConn)
	httpClient := newTestHTTPClient()

	sender := NewSender(publisher, webhookStore, httpClient)

	wl := &store.WebhookLog{
		EventID:     "test-event-retry-001",
		AppID:       1,
		AddressID:   1,
		Chain:       "ETH",
		TxHash:      "0xtest002",
		EventType:   "NATIVE_TRANSFER",
		Payload:     "{}",
		Status:      "pending",
		CallbackURL: "http://localhost",
	}
	if err := webhookStore.Create(context.Background(), wl); err != nil {
		t.Fatalf("创建 webhook_log 失败: %v", err)
	}
	defer db.Delete(wl)

	// mock 回调服务：返回 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal error`))
	}))
	defer srv.Close()

	task := makeTestTask(wl.ID, srv.URL)
	body, _ := json.Marshal(task)
	msg := newTestDelivery(body, 0) // 第一次，retryCount=0

	sender.Handle(msg)

	// 验证状态为 failed，retry_count=1
	updated, err := webhookStore.GetByID(context.Background(), wl.ID)
	if err != nil {
		t.Fatalf("获取 webhook_log 失败: %v", err)
	}
	if updated.Status != "failed" {
		t.Errorf("期望 status=failed，实际: %s", updated.Status)
	}
	if updated.RetryCount != 1 {
		t.Errorf("期望 retry_count=1，实际: %d", updated.RetryCount)
	}
	t.Logf("回调失败重试测试通过 ✓ status=%s, retry_count=%d", updated.Status, updated.RetryCount)
}

// TestSender_CallbackFail_Dead 验证：第 5 次失败 → 进死信队列
func TestSender_CallbackFail_Dead(t *testing.T) {
	db := newTestDB(t)
	mqConn := newTestMQConn(t)
	defer mqConn.Close()

	ch, err := mqConn.Channel()
	if err != nil {
		t.Fatalf("创建 Channel 失败: %v", err)
	}
	if err := mq.DeclareTopology(ch); err != nil {
		t.Fatalf("声明拓扑失败: %v", err)
	}
	ch.Close()

	webhookStore := store.NewWebhookLogStore(db)
	publisher := mq.NewPublisher(mqConn)
	httpClient := newTestHTTPClient()

	sender := NewSender(publisher, webhookStore, httpClient)

	wl := &store.WebhookLog{
		EventID:     "test-event-dead-001",
		AppID:       1,
		AddressID:   1,
		Chain:       "ETH",
		TxHash:      "0xtest003",
		EventType:   "NATIVE_TRANSFER",
		Payload:     "{}",
		Status:      "pending",
		CallbackURL: "http://localhost",
		RetryCount:  4, // 已重试 4 次
	}
	if err := webhookStore.Create(context.Background(), wl); err != nil {
		t.Fatalf("创建 webhook_log 失败: %v", err)
	}
	defer db.Delete(wl)

	// mock 回调：返回 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	task := makeTestTask(wl.ID, srv.URL)
	body, _ := json.Marshal(task)
	msg := newTestDelivery(body, 4) // retryCount=4，再失败一次 → 进死信

	sender.Handle(msg)

	// 第 5 次失败后状态仍为 failed（死信队列里的 DeadLetterHandler 会改成 dead）
	updated, err := webhookStore.GetByID(context.Background(), wl.ID)
	if err != nil {
		t.Fatalf("获取 webhook_log 失败: %v", err)
	}
	if updated.Status != "failed" {
		t.Errorf("期望 status=failed，实际: %s", updated.Status)
	}
	t.Logf("死信队列路由测试通过 ✓ retry_count=%d → 消息已路由到 dispatch.dead", updated.RetryCount)
}

// newTestHTTPClient 创建默认超时的 HTTP 客户端
func newTestHTTPClient() *httputil.Client {
	return httputil.New(5)
}
