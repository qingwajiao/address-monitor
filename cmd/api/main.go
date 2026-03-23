package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"address-monitor/internal/api"
	appconfig "address-monitor/internal/config"
	"address-monitor/internal/matcher"
	"address-monitor/internal/mq"
	"address-monitor/internal/store"

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

	// 初始化 MySQL
	db, err := appconfig.InitMySQL(cfg)
	if err != nil {
		zap.L().Fatal("初始化 MySQL 失败", zap.Error(err))
	}

	// 初始化 Redis
	rdb, err := appconfig.InitRedis(cfg)
	if err != nil {
		zap.L().Fatal("初始化 Redis 失败", zap.Error(err))
	}

	// 初始化 RabbitMQ
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

	// 初始化各 Store
	subStore := store.NewSubscriptionStore(db)
	deliveryStore := store.NewDeliveryStore(db)

	// 初始化 Matcher
	m := matcher.New(rdb, subStore)

	// 初始化 Publisher
	publisher := mq.NewPublisher(mqConn)

	// 启动 HTTP 服务
	router := api.NewRouter(cfg, subStore, deliveryStore, m, rdb, publisher)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// 优雅退出
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		zap.L().Info("API Service 启动", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("HTTP 服务启动失败", zap.Error(err))
		}
	}()

	<-ctx.Done()
	zap.L().Info("API Service 正在关闭...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)

	zap.L().Info("API Service 已关闭")
}
