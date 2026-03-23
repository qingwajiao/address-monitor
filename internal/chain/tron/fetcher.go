package tron

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"address-monitor/internal/chain"
	"address-monitor/pkg/httputil"
)

type TRONFetcher struct {
	chainName string
	baseURL   string
	client    *httputil.Client
}

func NewTRONFetcher(chainName, rpcURL string) *TRONFetcher {
	return &TRONFetcher{
		chainName: chainName,
		baseURL:   strings.TrimRight(rpcURL, "/"),
		client:    httputil.New(10),
	}
}

func (f *TRONFetcher) ChainName() string { return f.chainName }

func (f *TRONFetcher) LatestBlockNum(ctx context.Context) (uint64, error) {
	_, body, err := f.client.Post(
		f.baseURL+"/walletsolidity/getnowblock",
		[]byte("{}"),
		map[string]string{"Content-Type": "application/json"},
	)
	if err != nil {
		return 0, fmt.Errorf("获取最新块失败: %w", err)
	}

	var resp struct {
		BlockHeader struct {
			RawData struct {
				Number int64 `json:"number"`
			} `json:"raw_data"`
		} `json:"block_header"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("解析响应失败: %w", err)
	}
	return uint64(resp.BlockHeader.RawData.Number), nil
}

func (f *TRONFetcher) FetchBlock(ctx context.Context, num uint64) ([]chain.RawEvent, error) {
	reqBody, _ := json.Marshal(map[string]uint64{"num": num})
	_, body, err := f.client.Post(
		f.baseURL+"/walletsolidity/getblockbynum",
		reqBody,
		map[string]string{"Content-Type": "application/json"},
	)
	if err != nil {
		return nil, fmt.Errorf("拉取块 %d 失败: %w", num, err)
	}

	var block struct {
		BlockHeader struct {
			RawData struct {
				Number    int64 `json:"number"`
				Timestamp int64 `json:"timestamp"`
			} `json:"raw_data"`
		} `json:"block_header"`
		Transactions []json.RawMessage `json:"transactions"`
	}
	if err := json.Unmarshal(body, &block); err != nil {
		return nil, fmt.Errorf("解析块 %d 失败: %w", num, err)
	}

	blockTime := block.BlockHeader.RawData.Timestamp / 1000 // 毫秒转秒

	var events []chain.RawEvent
	for _, txRaw := range block.Transactions {
		// 提取 txID
		var txMeta struct {
			TxID string `json:"txID"`
			Ret  []struct {
				ContractRet string `json:"contractRet"`
			} `json:"ret"`
		}
		if err := json.Unmarshal(txRaw, &txMeta); err != nil {
			continue
		}
		// 只处理成功的交易
		if len(txMeta.Ret) == 0 || txMeta.Ret[0].ContractRet != "SUCCESS" {
			continue
		}

		events = append(events, chain.RawEvent{
			Chain:     f.chainName,
			TxHash:    txMeta.TxID,
			BlockNum:  num,
			BlockTime: blockTime,
			Type:      "tx",
			Data:      txRaw,
		})
	}
	return events, nil
}

func (f *TRONFetcher) HealthCheck(ctx context.Context) bool {
	_, err := f.LatestBlockNum(ctx)
	return err == nil
}
