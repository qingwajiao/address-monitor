package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig
	MySQL      MySQLConfig
	Redis      RedisConfig
	RabbitMQ   RabbitMQConfig
	Chains     map[string]ChainConfig
	Dispatcher DispatcherConfig
	Log        LogConfig
}

type LogConfig struct {
	Level string // "debug" | "info" | "warn" | "error"
}

type ServerConfig struct {
	Port   int
	APIKey string `mapstructure:"api_key"`
}

type MySQLConfig struct{ DSN string }

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type RabbitMQConfig struct{ URL string }

type ChainConfig struct {
	Enabled             bool
	Type                string
	ChainID             int64  `mapstructure:"chain_id"`
	RPCURL              string `mapstructure:"rpc_url"`
	BackupRPCURL        string `mapstructure:"backup_rpc_url"`
	HTTPRPCURL          string `mapstructure:"http_rpc_url"`
	PollIntervalSeconds int    `mapstructure:"poll_interval_seconds"`
	Confirmations       uint64
}

type DispatcherConfig struct {
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
	MaxRetries     int `mapstructure:"max_retries"`
}

func Load() (*Config, error) {
	// 读取环境变量 APP_ENV，默认 dev
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}

	configFile := fmt.Sprintf("configs/config.%s.yaml", env)

	// 检查环境配置文件是否存在，不存在则降级到 config.dev.yaml
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		configFile = "configs/config.dev.yaml"
	}

	viper.SetConfigFile(configFile)
	viper.AutomaticEnv()

	// 环境变量覆盖
	viper.BindEnv("mysql.dsn", "MYSQL_DSN")
	viper.BindEnv("redis.addr", "REDIS_ADDR")
	viper.BindEnv("rabbitmq.url", "RABBITMQ_URL")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("加载配置文件 %s 失败: %w", configFile, err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	fmt.Printf("[config] 已加载配置文件: %s (APP_ENV=%s)\n", configFile, env)
	return &cfg, nil
}
