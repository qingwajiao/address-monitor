package distlock

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

var (
	ErrLockNotHeld = errors.New("lock not held by this instance")

	// Lua 脚本：只有 value 匹配才删除，保证不误删其他实例的锁
	unlockScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`)

	// Lua 脚本：只有 value 匹配才续期
	renewScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)
)

type Lock struct {
	rdb   *redis.Client
	key   string
	value string // 当前实例的唯一标识
}

func New(rdb *redis.Client) *Lock {
	return &Lock{rdb: rdb}
}

// Lock 尝试获取分布式锁
// 返回 true 表示获取成功，false 表示锁已被其他实例持有
func (l *Lock) Lock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	value := uuid.New().String()
	ok, err := l.rdb.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, err
	}
	if ok {
		l.key = key
		l.value = value
	}
	return ok, nil
}

// Unlock 释放锁，只有持锁方才能释放
func (l *Lock) Unlock(ctx context.Context, key string) error {
	result, err := unlockScript.Run(ctx, l.rdb, []string{key}, l.value).Int()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// Renew 续期，只有持锁方才能续期
func (l *Lock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	result, err := renewScript.Run(ctx, l.rdb,
		[]string{key},
		l.value,
		int(ttl.Seconds()),
	).Int()
	if err != nil {
		return err
	}
	if result == 0 {
		return ErrLockNotHeld
	}
	return nil
}

// RenewLoop 在 goroutine 中持续续期，直到 ctx 取消
func (l *Lock) RenewLoop(ctx context.Context, key string, ttl, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := l.Renew(ctx, key, ttl); err != nil {
				return
			}
		}
	}
}
