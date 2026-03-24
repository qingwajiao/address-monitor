package config

import (
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
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

func InitRedis(cfg *Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	return rdb, nil
}

// RunMigrations 自动执行未执行的迁移文件
func RunMigrations(dsn string) error {
	// golang-migrate 需要的 DSN 格式和 GORM 略有不同，需要加 multiStatements=true
	migrateDSN := fmt.Sprintf("mysql://%s", addMultiStatements(dsn))

	m, err := migrate.New("file://migrations", migrateDSN)
	if err != nil {
		return fmt.Errorf("初始化迁移失败: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("执行迁移失败: %w", err)
	}

	version, dirty, _ := m.Version()
	zap.L().Info("数据库迁移完成",
		zap.Uint("version", version),
		zap.Bool("dirty", dirty),
	)
	return nil
}

// addMultiStatements 在 DSN 里加上 multiStatements=true
// 支持一个迁移文件里写多条 SQL（如 000007 里建四张表）
func addMultiStatements(dsn string) string {
	if len(dsn) == 0 {
		return dsn
	}
	if contains(dsn, "multiStatements") {
		return dsn
	}
	if contains(dsn, "?") {
		return dsn + "&multiStatements=true"
	}
	return dsn + "?multiStatements=true"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func InitLogger(cfg *Config) (*zap.Logger, error) {
	var zapCfg zap.Config
	level := cfg.Log.Level
	if level == "" {
		level = "info"
	}
	if level == "debug" {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		zapCfg = zap.NewProductionConfig()
	}
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
