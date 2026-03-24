package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	appconfig "address-monitor/internal/config"
	"address-monitor/internal/dispatcher"
	"address-monitor/internal/mq"
	"address-monitor/internal/store"
	"address-monitor/pkg/httputil"

	"go.uber.org/zap"
)

func main() {
	tmpLogger, _ := zap.NewProduction()
	zap.ReplaceGlobals(tmpLogger)

	cfg, err := appconfig.Load()
	if err != nil {
		zap.L().Fatal("加载配置失败", zap.Error(err))
	}

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

	ch, err := mqConn.Channel()
	if err != nil {
		zap.L().Fatal("创建 Channel 失败", zap.Error(err))
	}
	if err := mq.DeclareTopology(ch); err != nil {
		zap.L().Fatal("声明 RabbitMQ 拓扑失败", zap.Error(err))
	}
	ch.Close()

	// 初始化 Store（使用新表名）
	addrStore := store.NewWatchedAddressStore(db)
	webhookStore := store.NewWebhookLogStore(db)
	appStore := store.NewAppStore(db)

	publisher := mq.NewPublisher(mqConn)
	consumer := mq.NewConsumer(mqConn)
	httpClient := httputil.New(cfg.Dispatcher.TimeoutSeconds)

	expander := dispatcher.NewExpander(publisher, addrStore, webhookStore, appStore, rdb)
	sender := dispatcher.NewSender(publisher, webhookStore, httpClient)
	deadHandler := dispatcher.NewDeadLetterHandler(webhookStore)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		if err := consumer.Consume(ctx, "matched.events", expander.Handle); err != nil {
			zap.L().Error("matched.events 消费者退出", zap.Error(err))
		}
	}()

	go func() {
		if err := consumer.Consume(ctx, "dispatch.tasks", sender.Handle); err != nil {
			zap.L().Error("dispatch.tasks 消费者退出", zap.Error(err))
		}
	}()

	go func() {
		if err := consumer.Consume(ctx, "dispatch.dead", deadHandler.Handle); err != nil {
			zap.L().Error("dispatch.dead 消费者退出", zap.Error(err))
		}
	}()

	zap.L().Info("Dispatcher Service 启动成功")
	<-ctx.Done()
	zap.L().Info("Dispatcher Service 已关闭")
}
