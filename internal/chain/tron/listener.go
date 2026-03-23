package tron

import (
	"time"

	"address-monitor/internal/chain"
	"address-monitor/internal/config"
)

func NewTRONListener(name string, cfg config.ChainConfig, tracker *chain.BlockTracker) chain.Listener {
	fetcher := NewTRONFetcher("TRON", cfg.RPCURL)
	interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
	if interval == 0 {
		interval = 3 * time.Second
	}
	return chain.NewPollingListener(fetcher, tracker, interval)
}
