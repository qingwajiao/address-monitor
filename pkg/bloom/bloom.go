package bloom

import (
	"sync"

	stdbloom "github.com/bits-and-blooms/bloom/v3"
)

type Filter struct {
	mu sync.RWMutex
	bf *stdbloom.BloomFilter
}

// New 创建 Bloom Filter
// n: 预计存储的元素数量
// fp: 期望的误判率，建议 0.001（0.1%）
func New(n uint, fp float64) *Filter {
	return &Filter{
		bf: stdbloom.NewWithEstimates(n, fp),
	}
}

// Add 添加元素
func (f *Filter) Add(item string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bf.AddString(item)
}

// Test 判断元素是否可能存在
// 返回 false 表示一定不存在
// 返回 true 表示可能存在（有误判率）
func (f *Filter) Test(item string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.bf.TestString(item)
}

// Encode 序列化为字节切片，用于持久化到 Redis
func (f *Filter) Encode() ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.bf.MarshalBinary()
}

// Decode 从字节切片反序列化，用于从 Redis 恢复
func (f *Filter) Decode(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bf.UnmarshalBinary(data)
}

// Count 返回已添加的元素估计数量
func (f *Filter) Count() uint {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return uint(f.bf.ApproximatedSize())
}
