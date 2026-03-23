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

	// 先用默认 logger 输出配置加载信息
	tmpLogger, _ := zap.NewProduction()
	zap.ReplaceGlobals(tmpLogger)
	// 加载再配置
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

	// 初始化各组件
	publisher := mq.NewPublisher(mqConn)
	consumer := mq.NewConsumer(mqConn)
	subStore := store.NewSubscriptionStore(db)
	deliveryStore := store.NewDeliveryStore(db)
	httpClient := httputil.New(cfg.Dispatcher.TimeoutSeconds)

	// 初始化 Handler
	expander := dispatcher.NewExpander(publisher, subStore, deliveryStore, rdb)
	sender := dispatcher.NewSender(publisher, deliveryStore, httpClient)
	deadHandler := dispatcher.NewDeadLetterHandler(deliveryStore)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 启动三个消费者
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
