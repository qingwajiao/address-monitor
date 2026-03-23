package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPost(t *testing.T) {
	// 启动一个本地 mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("期望 POST，实际 %s", r.Method)
		}
		if r.Header.Get("X-Test-Header") != "test-value" {
			t.Errorf("header 不匹配")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := New(10)
	payload := []byte(`{"key":"value"}`)
	headers := map[string]string{
		"Content-Type":  "application/json",
		"X-Test-Header": "test-value",
	}

	code, body, err := client.Post(server.URL, payload, headers)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if code != 200 {
		t.Fatalf("期望状态码 200，实际 %d", code)
	}
	t.Logf("状态码: %d，响应: %s", code, string(body))

	// 测试超时：用一个会立即关闭的 server 模拟连接失败
	shortClient := New(20)
	_, _, err = shortClient.Post("http://127.0.0.1:19999", payload, nil)
	if err == nil {
		t.Fatal("应该触发连接失败")
	}
	t.Logf("连接失败（符合预期）: %v", err)
	t.Log("所有 HTTP 客户端测试通过")
}
