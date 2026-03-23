package sol

import (
	"address-monitor/internal/chain"
	"address-monitor/internal/config"
)

func init() {
	chain.RegisterType("sol", func(name string, cfg config.ChainConfig, tracker *chain.BlockTracker) chain.Listener {
		return NewSOLListener(name, cfg, tracker)
	})
}
