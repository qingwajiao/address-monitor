package distlock

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func newTestRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
}

func TestLockAndUnlock(t *testing.T) {
	rdb := newTestRedis()
	ctx := context.Background()

	// 验证 Redis 连通性
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("Redis 连接失败: %v", err)
	}

	key := "test:lock:key"
	rdb.Del(ctx, key) // 清理残留

	lock1 := New(rdb)
	lock2 := New(rdb)

	// lock1 获取锁
	ok, err := lock1.Lock(ctx, key, 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("lock1 应该获取锁成功: %v", err)
	}
	t.Log("lock1 获取锁成功")

	// lock2 获取同一个锁，应该失败
	ok, err = lock2.Lock(ctx, key, 10*time.Second)
	if err != nil || ok {
		t.Fatal("lock2 不应该获取到锁")
	}
	t.Log("lock2 获取锁失败（符合预期）")

	// lock1 续期
	if err := lock1.Renew(ctx, key, 10*time.Second); err != nil {
		t.Fatalf("lock1 续期失败: %v", err)
	}
	t.Log("lock1 续期成功")

	// lock2 不能续期 lock1 的锁
	if err := lock2.Renew(ctx, key, 10*time.Second); err == nil {
		t.Fatal("lock2 不应该能续期 lock1 的锁")
	}
	t.Log("lock2 续期失败（符合预期）")

	// lock1 释放锁
	if err := lock1.Unlock(ctx, key); err != nil {
		t.Fatalf("lock1 释放锁失败: %v", err)
	}
	t.Log("lock1 释放锁成功")

	// lock2 现在可以获取锁了
	ok, err = lock2.Lock(ctx, key, 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("lock2 现在应该能获取锁: %v", err)
	}
	t.Log("lock2 获取锁成功")

	// 清理
	lock2.Unlock(ctx, key)
	t.Log("所有分布式锁测试通过")
}
