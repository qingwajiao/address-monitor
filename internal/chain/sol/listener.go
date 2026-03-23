package sol

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"address-monitor/internal/chain"
	"address-monitor/internal/config"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"go.uber.org/zap"
)

type SOLListener struct {
	cfg     config.ChainConfig
	tracker *chain.BlockTracker
}

func NewSOLListener(name string, cfg config.ChainConfig, tracker *chain.BlockTracker) chain.Listener {
	return &SOLListener{cfg: cfg, tracker: tracker}
}

func (l *SOLListener) Chain() string { return "SOL" }

func (l *SOLListener) Start(ctx context.Context, eventCh chan<- chain.RawEvent, errCh chan<- error) error {
	// 初始化起始 slot
	_, err := l.tracker.Init(ctx, func() (uint64, error) {
		client := rpc.New(l.cfg.HTTPRPCURL)
		return client.GetSlot(ctx, rpc.CommitmentFinalized)
	})
	if err != nil {
		return fmt.Errorf("[SOL] 初始化 slot 失败: %w", err)
	}

	go l.wsLoop(ctx, eventCh, errCh)
	return nil
}

func (l *SOLListener) wsLoop(ctx context.Context, eventCh chan<- chain.RawEvent, errCh chan<- error) {
	for {
		if err := l.runWS(ctx, eventCh); err != nil {
			errCh <- fmt.Errorf("[SOL] ws 断开: %w", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			zap.L().Info("[SOL] 5s 后重连 WebSocket")
		}
	}
}

func (l *SOLListener) runWS(ctx context.Context, eventCh chan<- chain.RawEvent) error {
	wsClient, err := ws.Connect(ctx, l.cfg.RPCURL)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}
	defer wsClient.Close()

	// 订阅 Token Program（SPL Token 转账）
	tokenProgramID := solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	tokenSub, err := wsClient.LogsSubscribeMentions(tokenProgramID, rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("订阅 Token Program 失败: %w", err)
	}
	defer tokenSub.Unsubscribe()

	// 订阅 System Program（原生 SOL 转账）
	systemProgramID := solana.MustPublicKeyFromBase58("11111111111111111111111111111111")
	systemSub, err := wsClient.LogsSubscribeMentions(systemProgramID, rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("订阅 System Program 失败: %w", err)
	}
	defer systemSub.Unsubscribe()

	zap.L().Info("[SOL] WebSocket 订阅成功")

	httpClient := rpc.New(l.cfg.HTTPRPCURL)

	// 用 channel 合并两个订阅的消息
	sigCh := make(chan solana.Signature, 100)

	go func() {
		for {
			got, err := tokenSub.Recv(ctx)
			if err != nil {
				return
			}
			if got.Value.Err == nil {
				sigCh <- got.Value.Signature
			}
		}
	}()

	go func() {
		for {
			got, err := systemSub.Recv(ctx)
			if err != nil {
				return
			}
			if got.Value.Err == nil {
				sigCh <- got.Value.Signature
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case sig := <-sigCh:
			go l.fetchAndEmit(ctx, httpClient, sig, eventCh)
		}
	}
}

func (l *SOLListener) fetchAndEmit(
	ctx context.Context,
	client *rpc.Client,
	sig solana.Signature,
	eventCh chan<- chain.RawEvent,
) {
	maxVersion := uint64(0)
	tx, err := client.GetTransaction(ctx, sig, &rpc.GetTransactionOpts{
		Encoding:                       solana.EncodingJSON,
		MaxSupportedTransactionVersion: &maxVersion,
	})
	if err != nil || tx == nil {
		return
	}

	data, err := json.Marshal(tx)
	if err != nil {
		return
	}

	var blockTime int64
	if tx.BlockTime != nil {
		blockTime = int64(*tx.BlockTime)
	}

	eventCh <- chain.RawEvent{
		Chain:     "SOL",
		TxHash:    sig.String(),
		BlockNum:  uint64(tx.Slot),
		BlockTime: blockTime,
		Type:      "sol_tx",
		Data:      data,
	}

	l.tracker.Update(ctx, uint64(tx.Slot))
}

func (l *SOLListener) Stop() error { return nil }

func (l *SOLListener) HealthCheck(ctx context.Context) bool {
	client := rpc.New(l.cfg.HTTPRPCURL)
	_, err := client.GetSlot(ctx, rpc.CommitmentFinalized)
	return err == nil
}
