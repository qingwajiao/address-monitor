package tron

import (
	"address-monitor/internal/chain"
	"address-monitor/internal/config"
)

func init() {
	chain.RegisterType("tron", func(name string, cfg config.ChainConfig, tracker *chain.BlockTracker) chain.Listener {
		return NewTRONListener(name, cfg, tracker)
	})
}
