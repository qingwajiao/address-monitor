package evm

import (
	"time"

	"address-monitor/internal/chain"
	"address-monitor/internal/config"
)

type EVMListener struct {
	*chain.PollingListener
	name string
}

func NewEVMListener(name string, cfg config.ChainConfig, tracker *chain.BlockTracker) chain.Listener {
	fetcher, err := NewEVMFetcher(
		toUpperChainName(name),
		cfg.RPCURL,
		cfg.BackupRPCURL,
		cfg.ChainID,
	)
	if err != nil {
		// 如果主路失败，尝试备路
		fetcher, err = NewEVMFetcher(
			toUpperChainName(name),
			cfg.BackupRPCURL,
			cfg.RPCURL,
			cfg.ChainID,
		)
		if err != nil {
			panic("EVM fetcher 初始化失败: " + err.Error())
		}
	}

	interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
	if interval == 0 {
		interval = 6 * time.Second // 默认 6s
	}

	return chain.NewPollingListener(fetcher, tracker, interval)
}

func toUpperChainName(name string) string {
	switch name {
	case "eth":
		return "ETH"
	case "bsc":
		return "BSC"
	default:
		return name
	}
}

// 确保实现了 Listener 接口
var _ chain.Listener = (*chain.PollingListener)(nil)
