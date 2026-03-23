package config

import (
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitMySQL(cfg *Config) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	return db, nil
}

func InitLogger(cfg *Config) (*zap.Logger, error) {
	var zapCfg zap.Config

	level := cfg.Log.Level
	if level == "" {
		level = "info"
	}

	if level == "debug" {
		zapCfg = zap.NewDevelopmentConfig()
		// dev 模式：彩色输出，人类可读格式
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zapCfg = zap.NewProductionConfig()
	}

	// 设置日志级别
	switch level {
	case "debug":
		zapCfg.Level.SetLevel(zap.DebugLevel)
	case "info":
		zapCfg.Level.SetLevel(zap.InfoLevel)
	case "warn":
		zapCfg.Level.SetLevel(zap.WarnLevel)
	case "error":
		zapCfg.Level.SetLevel(zap.ErrorLevel)
	}

	logger, err := zapCfg.Build()
	if err != nil {
		return nil, err
	}
	zap.ReplaceGlobals(logger)
	return logger, nil
}

func InitRedis(cfg *Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	return rdb, nil
}

func RunMigrations(db *gorm.DB) error {
	return db.AutoMigrate(
		&store.Subscription{},
		&store.DeliveryLog{},
		&store.RawEvent{},
		&store.ChainSyncStatus{},
	)
}
