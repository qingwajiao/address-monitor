package bloom

import (
	"fmt"
	"testing"
)

func TestAddAndTest(t *testing.T) {
	f := New(1000000, 0.001) // 100万元素，0.1% 误判率

	// 添加一批地址
	addresses := []string{
		"eth0xabc123",
		"eth0xdef456",
		"bsc0x789abc",
		"tronTAbc123",
	}
	for _, addr := range addresses {
		f.Add(addr)
	}

	// 验证已添加的地址都能命中
	for _, addr := range addresses {
		if !f.Test(addr) {
			t.Errorf("已添加的地址应该能命中: %s", addr)
		}
	}
	t.Log("已添加地址全部命中 ✓")

	// 验证未添加的地址不命中（理论上极少误判）
	notAdded := []string{
		"eth0xffffff",
		"eth0x000000",
		"sol_address_xyz",
	}
	falsePositives := 0
	for _, addr := range notAdded {
		if f.Test(addr) {
			falsePositives++
			t.Logf("误判（正常现象，概率极低）: %s", addr)
		}
	}
	t.Logf("未添加地址误判数: %d/%d", falsePositives, len(notAdded))
	t.Log("地址查询测试通过 ✓")
}

func TestEncodeAndDecode(t *testing.T) {
	original := New(1000000, 0.001)

	// 添加一些数据
	for i := 0; i < 1000; i++ {
		original.Add(fmt.Sprintf("eth0xaddress%d", i))
	}

	// 序列化
	data, err := original.Encode()
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	t.Logf("序列化大小: %d bytes (%.2f MB)", len(data), float64(len(data))/1024/1024)

	// 反序列化到新的 Filter
	restored := New(1000000, 0.001)
	if err := restored.Decode(data); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	// 验证恢复后数据一致
	for i := 0; i < 1000; i++ {
		addr := fmt.Sprintf("eth0xaddress%d", i)
		if !restored.Test(addr) {
			t.Errorf("恢复后地址应该能命中: %s", addr)
		}
	}
	t.Log("序列化/反序列化测试通过 ✓")
}

func TestConcurrentSafety(t *testing.T) {
	f := New(1000000, 0.001)
	done := make(chan struct{})

	// 并发写
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				f.Add(fmt.Sprintf("eth0xaddr%d_%d", n, j))
			}
			done <- struct{}{}
		}(i)
	}

	// 并发读
	for i := 0; i < 5; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				f.Test(fmt.Sprintf("eth0xaddr%d_%d", n, j))
			}
			done <- struct{}{}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 15; i++ {
		<-done
	}
	t.Log("并发安全测试通过 ✓")
}
