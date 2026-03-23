package evm

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"

	"address-monitor/internal/chain"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

type EVMFetcher struct {
	chainName string
	client    *ethclient.Client
	backup    *ethclient.Client
	mu        sync.RWMutex
	failCount int
}

func NewEVMFetcher(chainName, rpcURL, backupURL string) (*EVMFetcher, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("连接主路 RPC 失败 [%s]: %w", chainName, err)
	}

	var backup *ethclient.Client
	if backupURL != "" {
		backup, _ = ethclient.Dial(backupURL)
	}

	return &EVMFetcher{
		chainName: chainName,
		client:    client,
		backup:    backup,
	}, nil
}

func (f *EVMFetcher) ChainName() string { return f.chainName }

func (f *EVMFetcher) LatestBlockNum(ctx context.Context) (uint64, error) {
	num, err := f.getClient().BlockNumber(ctx)
	if err != nil {
		f.recordFailure()
		return 0, err
	}
	f.resetFailure()
	return num, nil
}

func (f *EVMFetcher) FetchBlock(ctx context.Context, num uint64) ([]chain.RawEvent, error) {
	blockNum := big.NewInt(int64(num))
	client := f.getClient()

	var (
		block    *types.Block
		logs     []types.Log
		blockErr error
		wg       sync.WaitGroup
	)

	// 并行拉取区块数据和合约事件
	wg.Add(2)
	go func() {
		defer wg.Done()
		block, blockErr = client.BlockByNumber(ctx, blockNum)
	}()
	go func() {
		defer wg.Done()
		logs, _ = client.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: blockNum,
			ToBlock:   blockNum,
			// 不传 Addresses，拉全量 log，在 matcher 层过滤
		})
	}()
	wg.Wait()

	if blockErr != nil {
		f.recordFailure()
		return nil, fmt.Errorf("拉取块 %d 失败: %w", num, blockErr)
	}
	f.resetFailure()

	var events []chain.RawEvent

	// 路径一：原生币转账（遍历 tx）
	chainID := big.NewInt(f.chainIDInt())
	signer := types.LatestSignerForChainID(chainID)

	for _, tx := range block.Transactions() {
		if tx.Value() == nil || tx.Value().Sign() <= 0 || tx.To() == nil {
			continue
		}
		from, err := types.Sender(signer, tx)
		if err != nil {
			continue
		}
		txData, _ := json.Marshal(map[string]interface{}{
			"hash":  tx.Hash().Hex(),
			"from":  from.Hex(),
			"to":    tx.To().Hex(),
			"value": tx.Value().String(),
			"input": hex.EncodeToString(tx.Data()),
		})
		events = append(events, chain.RawEvent{
			Chain:     f.chainName,
			TxHash:    tx.Hash().Hex(),
			BlockNum:  num,
			BlockTime: int64(block.Time()),
			Type:      "tx",
			Data:      txData,
		})
	}

	// 路径二：合约事件（eth_getLogs）
	for _, log := range logs {
		logData, _ := json.Marshal(log)
		events = append(events, chain.RawEvent{
			Chain:     f.chainName,
			TxHash:    log.TxHash.Hex(),
			BlockNum:  num,
			BlockTime: int64(block.Time()),
			Type:      "log",
			Data:      logData,
		})
	}

	zap.L().Debug("拉取块完成",
		zap.String("chain", f.chainName),
		zap.Uint64("block", num),
		zap.Int("events", len(events)),
	)

	return events, nil
}

func (f *EVMFetcher) HealthCheck(ctx context.Context) bool {
	_, err := f.client.BlockNumber(ctx)
	return err == nil
}

// getClient 根据失败次数决定用主路还是备路
func (f *EVMFetcher) getClient() *ethclient.Client {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.failCount >= 3 && f.backup != nil {
		return f.backup
	}
	return f.client
}

func (f *EVMFetcher) recordFailure() {
	f.mu.Lock()
	f.failCount++
	if f.failCount == 3 {
		zap.L().Warn("主路 RPC 连续失败，切换备路", zap.String("chain", f.chainName))
	}
	f.mu.Unlock()
}

func (f *EVMFetcher) resetFailure() {
	f.mu.Lock()
	if f.failCount > 0 {
		f.failCount = 0
	}
	f.mu.Unlock()
}

// chainIDInt 返回链的 chain ID
func (f *EVMFetcher) chainIDInt() int64 {
	switch f.chainName {
	case "ETH":
		return 1
	case "BSC":
		return 56
	default:
		return 1
	}
}
