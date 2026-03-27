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

type rawEventItem struct {
	chain string
	event *store.ChainRawEvent
}

type Supervisor struct {
	cfg        *config.Config
	rdb        *redis.Client
	db         *gorm.DB
	publisher  *mq.Publisher
	lock       *distlock.Lock
	matcher    *matcher.Matcher
	parsers    map[string]parser.Parser
	rawWriteCh chan rawEventItem
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
		rawWriteCh: make(chan rawEventItem, 2000),
	}
}

func (s *Supervisor) Run(ctx context.Context) {
	enabledChains := os.Getenv("ENABLED_CHAINS")
	if enabledChains == "" {
		enabledChains = "eth"
	}

	errCh := make(chan error, 100)
	syncStore := store.NewChainSyncStore(s.db)
	rawStore := store.NewChainRawEventStore(s.db)

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

		// 每链独立的 eventCh，容量 = worker 数 * 200
		workerCount := chainCfg.WorkerCount
		if workerCount <= 0 {
			workerCount = 4
		}
		eventCh := make(chan RawEvent, workerCount*200)

		instanceID := fmt.Sprintf("worker-%s-%s", name, getInstanceID())
		tracker := NewBlockTracker(strings.ToUpper(name), instanceID, chainCfg.Confirmations, s.rdb, syncStore)

		l, err := Build(name, chainCfg, tracker)
		if err != nil {
			zap.L().Fatal("构建 Listener 失败", zap.String("chain", name), zap.Error(err))
		}

		// 分布式锁主备选举
		lockKey := fmt.Sprintf("listener:lock:%s", name)
		ok2, err := s.lock.Lock(ctx, lockKey, 15*time.Second)
		if err != nil || !ok2 {
			zap.L().Info("备机待命，未获取到锁", zap.String("chain", name))
			continue
		}
		go s.lock.RenewLoop(ctx, lockKey, 15*time.Second, 5*time.Second)

		// 启动 Listener（写入该链专属 eventCh）
		go func(l Listener, ch chan RawEvent, n string) {
			if err := l.Start(ctx, ch, errCh); err != nil {
				zap.L().Error("Listener 启动失败", zap.String("chain", n), zap.Error(err))
			}
		}(l, eventCh, name)

		// 启动该链的 worker pool
		for i := 0; i < workerCount; i++ {
			go s.processEvents(ctx, eventCh)
		}

		zap.L().Info("Listener 已启动",
			zap.String("chain", name),
			zap.Int("workers", workerCount),
		)
	}

	go s.runRawEventBatchWriter(ctx, rawStore)
	go s.handleErrors(ctx, errCh)
}

func (s *Supervisor) processEvents(ctx context.Context, eventCh <-chan RawEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw := <-eventCh:
			s.handleRawEvent(ctx, raw)
		}
	}
}

func (s *Supervisor) handleRawEvent(ctx context.Context, raw RawEvent) {
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

		// 旁路写 raw_events（异步批量写入）
		item := rawEventItem{
			chain: raw.Chain,
			event: &store.ChainRawEvent{
				TxHash:      raw.TxHash,
				BlockNumber: raw.BlockNum,
				BlockTime:   uint32(raw.BlockTime),
				EventType:   string(event.EventType),
				RawData:     string(raw.Data),
			},
		}
		select {
		case s.rawWriteCh <- item:
		default:
			zap.L().Warn("raw_events 写入队列已满，丢弃",
				zap.String("chain", raw.Chain),
				zap.String("tx", raw.TxHash),
			)
		}
	}
}

func (s *Supervisor) runRawEventBatchWriter(ctx context.Context, rawStore *store.ChainRawEventStore) {
	const maxBatch = 100
	const flushInterval = 500 * time.Millisecond

	// per-chain buffers
	buffers := make(map[string][]*store.ChainRawEvent)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func(chain string) {
		items := buffers[chain]
		if len(items) == 0 {
			return
		}
		buffers[chain] = nil
		if err := rawStore.BatchInsert(context.Background(), chain, items); err != nil {
			zap.L().Warn("raw_events 批量写入失败",
				zap.String("chain", chain),
				zap.Int("count", len(items)),
				zap.Error(err),
			)
		}
	}

	for {
		select {
		case <-ctx.Done():
			// flush remaining
			for chain := range buffers {
				flush(chain)
			}
			return
		case item := <-s.rawWriteCh:
			buffers[item.chain] = append(buffers[item.chain], item.event)
			if len(buffers[item.chain]) >= maxBatch {
				flush(item.chain)
			}
		case <-ticker.C:
			for chain := range buffers {
				flush(chain)
			}
		}
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
