package chain

import (
	"fmt"

	"address-monitor/internal/config"
)

// factory 函数签名：链名称 + 链配置 + BlockTracker → Listener
type Factory func(name string, cfg config.ChainConfig, tracker *BlockTracker) Listener

var registry = map[string]Factory{}

// RegisterType 注册链类型，各链在 init() 里调用
func RegisterType(chainType string, f Factory) {
	registry[chainType] = f
}

// Build 根据链名称和配置构建 Listener 实例
func Build(name string, cfg config.ChainConfig, tracker *BlockTracker) (Listener, error) {
	f, ok := registry[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown chain type: %s (chain: %s)", cfg.Type, name)
	}
	return f(name, cfg, tracker), nil
}

// RegisteredTypes 返回已注册的链类型列表
func RegisteredTypes() []string {
	types := make([]string, 0, len(registry))
	for k := range registry {
		types = append(types, k)
	}
	return types
}
