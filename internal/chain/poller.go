package chain

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// BlockFetcher 各链实现此接口，提供拉取数据的能力
type BlockFetcher interface {
	ChainName() string
	LatestBlockNum(ctx context.Context) (uint64, error)
	FetchBlock(ctx context.Context, num uint64) ([]RawEvent, error)
	HealthCheck(ctx context.Context) bool
}

// PollingListener 通用轮询框架，ETH/BSC/TRON 共用
type PollingListener struct {
	fetcher  BlockFetcher
	tracker  *BlockTracker
	interval time.Duration
}

func NewPollingListener(fetcher BlockFetcher, tracker *BlockTracker, interval time.Duration) *PollingListener {
	return &PollingListener{
		fetcher:  fetcher,
		tracker:  tracker,
		interval: interval,
	}
}

func (l *PollingListener) Chain() string { return l.fetcher.ChainName() }

func (l *PollingListener) Start(ctx context.Context, eventCh chan<- RawEvent, errCh chan<- error) error {

	key := fmt.Sprintf("last_block:%s", l.tracker.chain)
	l.tracker.rdb.Set(ctx, key, 10491419, 0)
	// 初始化起始块号
	_, err := l.tracker.Init(ctx, func() (uint64, error) {
		return l.fetcher.LatestBlockNum(ctx)
	})
	if err != nil {
		return fmt.Errorf("[%s] 初始化块号失败: %w", l.fetcher.ChainName(), err)
	}

	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()

	zap.L().Info("链监听启动",
		zap.String("chain", l.fetcher.ChainName()),
		zap.Duration("interval", l.interval),
		zap.Uint64("start_block", l.tracker.Get()),
	)

	for {
		select {
		case <-ctx.Done():
			zap.L().Info("链监听停止", zap.String("chain", l.fetcher.ChainName()))
			return nil
		case <-ticker.C:
			if err := l.poll(ctx, eventCh); err != nil {
				errCh <- err
			}
		}
	}
}

func (l *PollingListener) poll(ctx context.Context, eventCh chan<- RawEvent) error {
	latest, err := l.fetcher.LatestBlockNum(ctx)
	if err != nil {
		return fmt.Errorf("[%s] 获取最新块号失败: %w", l.fetcher.ChainName(), err)
	}

	last := l.tracker.Get()
	if latest <= last {
		zap.L().Debug("暂无新块",
			zap.String("chain", l.fetcher.ChainName()),
			zap.Uint64("current", last),
			zap.Uint64("latest", latest),
		)
		return nil
	}

	zap.L().Debug("发现新块，开始处理",
		zap.String("chain", l.fetcher.ChainName()),
		zap.Uint64("from", last+1),
		zap.Uint64("to", latest),
		zap.Uint64("count", latest-last),
	)

	for i := last + 1; i <= latest; i++ {
		events, err := l.fetcher.FetchBlock(ctx, i)
		if err != nil {
			return fmt.Errorf("[%s] 拉取块 %d 失败: %w", l.fetcher.ChainName(), i, err)
		}

		zap.L().Debug("块处理完成",
			zap.String("chain", l.fetcher.ChainName()),
			zap.Uint64("block", i),
			zap.Int("raw_events", len(events)),
		)

		for _, e := range events {
			select {
			case eventCh <- e:
			case <-ctx.Done():
				return nil
			}
		}
		// 更新 last_block
		l.tracker.Update(ctx, i)
	}
	return nil
}

func (l *PollingListener) Stop() error { return nil }

func (l *PollingListener) HealthCheck(ctx context.Context) bool {
	return l.fetcher.HealthCheck(ctx)
}
