package chain

import (
	"context"
	"strings"
	"sync"
	"time"

	"address-monitor/internal/store"

	"go.uber.org/zap"
)

// ContractFilter 系统级合约地址白名单过滤器
// 某条链没有配置任何合约 = 该链不做过滤，全部放行
type ContractFilter struct {
	mu      sync.RWMutex
	allowed map[string]map[string]struct{} // chain -> set of contract_address
	store   *store.AllowedContractStore
}

func NewContractFilter(s *store.AllowedContractStore) *ContractFilter {
	return &ContractFilter{
		allowed: make(map[string]map[string]struct{}),
		store:   s,
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
func (f *ContractFilter) IsAllowed(chain, contractAddress string) bool {
	if contractAddress == "" {
		return true
	}

	f.mu.RLock()
	contracts, hasConfig := f.allowed[strings.ToUpper(chain)]
	f.mu.RUnlock()

	if !hasConfig || len(contracts) == 0 {
		return true
	}
	_, ok := contracts[strings.ToLower(contractAddress)]
	return ok
}

func (f *ContractFilter) reload(contracts []*store.AllowedContract) {
	newAllowed := make(map[string]map[string]struct{})
	for _, c := range contracts {
		chain := strings.ToUpper(c.Chain)
		if newAllowed[chain] == nil {
			newAllowed[chain] = make(map[string]struct{})
		}
		newAllowed[chain][strings.ToLower(c.ContractAddress)] = struct{}{}
	}
	f.mu.Lock()
	f.allowed = newAllowed
	f.mu.Unlock()
}
