package chain

import (
	"context"
	"strings"
	"sync"
	"time"

	"address-monitor/internal/store"
	"address-monitor/pkg/addrcodec"

	"go.uber.org/zap"
)

// ContractFilter 系统级合约地址白名单过滤器
// 某条链没有配置任何合约 = 该链不做过滤，全部放行
type ContractFilter struct {
	mu       sync.RWMutex
	allowed  map[string]map[string]struct{} // chain -> set of contract_address
	symbols  map[string]map[string]string   // chain -> contract_address -> symbol
	decimals map[string]map[string]int      // chain -> contract_address -> decimals
	store    *store.AllowedContractStore
}

func NewContractFilter(s *store.AllowedContractStore) *ContractFilter {
	return &ContractFilter{
		allowed:  make(map[string]map[string]struct{}),
		symbols:  make(map[string]map[string]string),
		decimals: make(map[string]map[string]int),
		store:    s,
	}
}

// Load 从数据库加载白名单到内存，Worker 启动时调用
func (f *ContractFilter) Load(ctx context.Context) error {
	contracts, err := f.store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	f.reload(contracts)
	zap.L().Info("合约白名单已加载", zap.Int("count", len(contracts)))
	return nil
}

// StartRefreshJob 每隔 refreshInterval 从数据库重新加载一次
func (f *ContractFilter) StartRefreshJob(ctx context.Context, refreshInterval time.Duration) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			contracts, err := f.store.ListEnabled(ctx)
			if err != nil {
				zap.L().Warn("合约白名单刷新失败", zap.Error(err))
				continue
			}
			f.reload(contracts)
			zap.L().Debug("合约白名单已刷新", zap.Int("count", len(contracts)))
		}
	}
}

// IsAllowed 判断事件是否允许通过
// - 原生转账（contractAddress 为空）始终放行
// - 该链没有配置白名单，放行所有
// - 合约地址在白名单中，放行；否则过滤
func (f *ContractFilter) IsAllowed(chainName, contractAddress string) bool {
	if contractAddress == "" {
		return true
	}

	f.mu.RLock()
	contracts, hasConfig := f.allowed[strings.ToUpper(chainName)]
	f.mu.RUnlock()

	if !hasConfig || len(contracts) == 0 {
		return true
	}
	// 入参已是 Parser 输出的链原生格式，用 codec 归一化后查表
	_, ok := contracts[addrcodec.Get(chainName).Normalize(contractAddress)]
	return ok
}

// GetSymbol 返回合约地址对应的 token symbol，未找到返回空字符串
func (f *ContractFilter) GetSymbol(chainName, contractAddress string) string {
	if contractAddress == "" {
		return ""
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if syms, ok := f.symbols[strings.ToUpper(chainName)]; ok {
		return syms[contractAddress]
	}
	return ""
}

// GetDecimals 返回合约地址对应的 token decimals，未找到返回 0
func (f *ContractFilter) GetDecimals(chainName, contractAddress string) int {
	if contractAddress == "" {
		return 0
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if decs, ok := f.decimals[strings.ToUpper(chainName)]; ok {
		return decs[contractAddress]
	}
	return 0
}

func (f *ContractFilter) reload(contracts []*store.AllowedContract) {
	newAllowed := make(map[string]map[string]struct{})
	newSymbols := make(map[string]map[string]string)
	newDecimals := make(map[string]map[string]int)
	for _, c := range contracts {
		chainKey := strings.ToUpper(c.Chain)
		if newAllowed[chainKey] == nil {
			newAllowed[chainKey] = make(map[string]struct{})
			newSymbols[chainKey] = make(map[string]string)
			newDecimals[chainKey] = make(map[string]int)
		}
		newAllowed[chainKey][c.ContractAddress] = struct{}{}
		if c.Symbol != "" {
			newSymbols[chainKey][c.ContractAddress] = c.Symbol
		}
		if c.Decimals > 0 {
			newDecimals[chainKey][c.ContractAddress] = c.Decimals
		}
	}
	f.mu.Lock()
	f.allowed = newAllowed
	f.symbols = newSymbols
	f.decimals = newDecimals
	f.mu.Unlock()
}
