package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"strings"

	"address-monitor/internal/chain"
	appconfig "address-monitor/internal/config"
	"address-monitor/internal/matcher"
	"address-monitor/internal/mq"
	"address-monitor/internal/store"
	"address-monitor/pkg/distlock"

	// 触发各链 init() 注册
	_ "address-monitor/internal/chain/evm"
	_ "address-monitor/internal/chain/sol"
	_ "address-monitor/internal/chain/tron"

	"go.uber.org/zap"
)

func main() {

	// 先用默认 logger 输出配置加载信息
	tmpLogger, _ := zap.NewProduction()
	zap.ReplaceGlobals(tmpLogger)
	// 加载配置
	cfg, err := appconfig.Load()
	if err != nil {
		zap.L().Fatal("加载配置失败", zap.Error(err))
	}

	// 根据配置初始化正式 logger
	logger, err := appconfig.InitLogger(cfg)
	if err != nil {
		zap.L().Fatal("初始化日志失败", zap.Error(err))
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync()

	db, err := appconfig.InitMySQL(cfg)
	if err != nil {
		zap.L().Fatal("初始化 MySQL 失败", zap.Error(err))
	}

	rdb, err := appconfig.InitRedis(cfg)
	if err != nil {
		zap.L().Fatal("初始化 Redis 失败", zap.Error(err))
	}

	mqConn, err := mq.NewConnection(cfg.RabbitMQ.URL)
	if err != nil {
		zap.L().Fatal("初始化 RabbitMQ 失败", zap.Error(err))
	}

	// 声明 RabbitMQ 拓扑
	ch, err := mqConn.Channel()
	if err != nil {
		zap.L().Fatal("创建 Channel 失败", zap.Error(err))
	}
	if err := mq.DeclareTopology(ch); err != nil {
		zap.L().Fatal("声明 RabbitMQ 拓扑失败", zap.Error(err))
	}
	ch.Close()

	// 从配置提取已启用的链名
	var chainNames []string
	for name, chainCfg := range cfg.Chains {
		if chainCfg.Enabled {
			chainNames = append(chainNames, strings.ToUpper(name))
		}
	}

	// 初始化 Matcher
	addrStore := store.NewWatchedAddressStore(db)
	m := matcher.New(rdb, addrStore, chainNames)

	// 恢复 Bloom Filter
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ok, err := m.RestoreBF(ctx)
	if err != nil {
		zap.L().Warn("恢复 BF 快照失败，全量构建", zap.Error(err))
	}
	if !ok {
		zap.L().Info("BF 快照不存在，从 MySQL 全量构建")
		if err := m.LoadFromDB(ctx); err != nil {
			zap.L().Warn("全量构建 BF 失败", zap.Error(err))
		}
	}

	// 启动 BF 定时快照（每 5 分钟）
	go m.StartSnapshotJob(ctx)

	// 启动地址变更事件订阅（API 新增/删除地址时实时更新 BF）
	m.StartAddressEventSubscriber(ctx)

	// 启动 MySQL 增量同步（兜底 Redis 不可用时的漏检，每 5 分钟）
	go m.StartDBIncrementalSync(ctx)

	// 启动热降冷定时任务（每天）
	go m.StartColdDowngradeJob(ctx)

	// 初始化 Publisher 和分布式锁
	publisher := mq.NewPublisher(mqConn)
	lock := distlock.New(rdb)

	// 启动 Supervisor
	supervisor := chain.NewSupervisor(cfg, rdb, db, publisher, lock, m)
	supervisor.Run(ctx)

	// 等待退出信号
	<-ctx.Done()
	zap.L().Info("Worker 正在关闭...")
	time.Sleep(2 * time.Second) // 等待处理中的事件完成
	zap.L().Info("Worker 已关闭")
}
