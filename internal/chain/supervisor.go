package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"address-monitor/internal/config"
	"address-monitor/internal/matcher"
	"address-monitor/internal/mq"
	"address-monitor/internal/parser"
	"address-monitor/internal/store"
	"address-monitor/pkg/distlock"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Supervisor struct {
	cfg       *config.Config
	rdb       *redis.Client
	db        *gorm.DB
	publisher *mq.Publisher
	lock      *distlock.Lock
	matcher   *matcher.Matcher
	parsers   map[string]parser.Parser
}

func NewSupervisor(
	cfg *config.Config,
	rdb *redis.Client,
	db *gorm.DB,
	publisher *mq.Publisher,
	lock *distlock.Lock,
	m *matcher.Matcher,
) *Supervisor {
	return &Supervisor{
		cfg:       cfg,
		rdb:       rdb,
		db:        db,
		publisher: publisher,
		lock:      lock,
		matcher:   m,
		parsers: map[string]parser.Parser{
			"ETH":  parser.NewEVMParser("ETH"),
			"BSC":  parser.NewEVMParser("BSC"),
			"TRON": parser.NewTRONParser(),
			"SOL":  parser.NewSOLParser(),
		},
	}
}

func (s *Supervisor) Run(ctx context.Context) {
	enabledChains := os.Getenv("ENABLED_CHAINS")
	if enabledChains == "" {
		//enabledChains = "eth,bsc,tron,sol" // 默认启用全部
		enabledChains = "eth"
	}

	eventCh := make(chan RawEvent, 1000)
	errCh := make(chan error, 100)

	syncStore := store.NewChainSyncStore(s.db)
	rawStore := store.NewChainRawEventStore(s.db)

	// 按配置启动各链 Listener
	for _, name := range strings.Split(enabledChains, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		chainCfg, ok := s.cfg.Chains[name]
		if !ok || !chainCfg.Enabled {
			zap.L().Warn("链未配置或未启用", zap.String("chain", name))
			continue
		}

		// 实例化 BlockTracker
		instanceID := fmt.Sprintf("worker-%s-%s", name, getInstanceID())
		tracker := NewBlockTracker(
			strings.ToUpper(name),
			instanceID,
			s.rdb,
			syncStore,
		)

		// 构建 Listener
		l, err := Build(name, chainCfg, tracker)
		if err != nil {
			zap.L().Fatal("构建 Listener 失败",
				zap.String("chain", name),
				zap.Error(err),
			)
		}

		// 尝试获取分布式锁（主备选举）
		lockKey := fmt.Sprintf("listener:lock:%s", name)
		ok2, err := s.lock.Lock(ctx, lockKey, 15*time.Second)
		if err != nil || !ok2 {
			zap.L().Info("备机待命，未获取到锁", zap.String("chain", name))
			continue
		}

		// 持锁方启动续期
		go s.lock.RenewLoop(ctx, lockKey, 15*time.Second, 5*time.Second)

		// 启动 Listener
		go func(l Listener, name string) {
			if err := l.Start(ctx, eventCh, errCh); err != nil {
				zap.L().Error("Listener 启动失败",
					zap.String("chain", name),
					zap.Error(err),
				)
			}
		}(l, name)

		zap.L().Info("Listener 已启动", zap.String("chain", name))
	}

	// 消费 eventCh：解析 → 地址匹配 → 发 RabbitMQ → 旁路写 raw_events
	go s.processEvents(ctx, eventCh, rawStore)

	// 消费 errCh：打日志
	go s.handleErrors(ctx, errCh)
}

func (s *Supervisor) processEvents(
	ctx context.Context,
	eventCh <-chan RawEvent,
	rawStore *store.ChainRawEventStore,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw := <-eventCh:
			s.handleRawEvent(ctx, raw, rawStore)
		}
	}
}

func (s *Supervisor) handleRawEvent(
	ctx context.Context,
	raw RawEvent,
	rawStore *store.ChainRawEventStore,
) {
	p, ok := s.parsers[raw.Chain]
	if !ok {
		zap.L().Warn("未找到对应的 Parser", zap.String("chain", raw.Chain))
		return
	}

	// 解析原始事件
	events, err := p.Parse(ctx, parser.RawEvent{
		Chain:     raw.Chain,
		TxHash:    raw.TxHash,
		BlockNum:  raw.BlockNum,
		BlockTime: raw.BlockTime,
		Type:      raw.Type,
		Data:      raw.Data,
	})
	if err != nil {
		zap.L().Warn("事件解析失败",
			zap.String("chain", raw.Chain),
			zap.String("tx", raw.TxHash),
			zap.Error(err),
		)
		return
	}

	for _, event := range events {
		// 地址匹配（三层漏斗）
		subs, err := s.matcher.IsWatched(ctx, event.Chain, event.WatchedAddress)
		if err != nil {
			zap.L().Warn("地址匹配失败", zap.Error(err))
			continue
		}

		if event.BlockNumber == 10491419 {
			zap.L().Info("目标区块")
		}
		if event.WatchedAddress == "0x947faa43661d3d672b54aa858619272d208bdb5a" {
			zap.L().Info("命中地址")
		}
		if len(subs) == 0 {
			continue // 不在监控列表，跳过
		}

		// 更新地址最后活跃时间
		s.matcher.UpdateLastActive(ctx, event.Chain, event.WatchedAddress)

		// 发布到 RabbitMQ matched.events
		body, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if err := s.publisher.Publish("matched.exchange", "matched", body, nil); err != nil {
			zap.L().Error("发布 matched.events 失败",
				zap.String("event_id", event.EventID),
				zap.Error(err),
			)
			continue
		}

		zap.L().Info("事件命中，已发布",
			zap.String("chain", event.Chain),
			zap.String("tx", event.TxHash),
			zap.String("type", string(event.EventType)),
			zap.String("address", event.WatchedAddress),
		)

		// 旁路写 raw_events（异步 fire-and-forget）
		go func(r RawEvent, e *parser.NormalizedEvent) {
			rawStore.Insert(context.Background(), r.Chain, &store.ChainRawEvent{
				TxHash:      r.TxHash,
				BlockNumber: r.BlockNum,
				BlockTime:   uint32(r.BlockTime),
				EventType:   string(e.EventType),
				RawData:     string(r.Data),
			})
		}(raw, event)
	}
}

func (s *Supervisor) handleErrors(ctx context.Context, errCh <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			zap.L().Error("链监听错误", zap.Error(err))
		}
	}
}

// getInstanceID 获取实例标识，优先用 POD_NAME 环境变量
func getInstanceID() string {
	if podName := os.Getenv("POD_NAME"); podName != "" {
		return podName
	}
	hostname, _ := os.Hostname()
	return hostname
}
