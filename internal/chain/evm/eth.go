package evm

import (
	"address-monitor/internal/chain"
	"address-monitor/internal/config"
)

func init() {
	chain.RegisterType("evm", func(name string, cfg config.ChainConfig, tracker *chain.BlockTracker) chain.Listener {
		return NewEVMListener(name, cfg, tracker)
	})
}
