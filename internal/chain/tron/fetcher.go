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
		var txMeta struct {
			TxID string `json:"txID"`
			Ret  []struct {
				ContractRet string `json:"contractRet"`
			} `json:"ret"`
			RawData struct {
				Contract []struct {
					Type string `json:"type"`
				} `json:"contract"`
			} `json:"raw_data"`
		}
		if err := json.Unmarshal(txRaw, &txMeta); err != nil {
			continue
		}

		if len(txMeta.Ret) == 0 {
			continue
		}
		contractType := ""
		if len(txMeta.RawData.Contract) > 0 {
			contractType = txMeta.RawData.Contract[0].Type
		}

		// TransferContract（原生 TRX）成功时 contractRet 可能为空，不能用 != "SUCCESS" 过滤
		// TriggerSmartContract（TRC20 等）必须是 SUCCESS 才处理
		contractRet := txMeta.Ret[0].ContractRet
		if contractType == "TriggerSmartContract" && contractRet != "SUCCESS" {
			continue
		}
		if contractType != "TriggerSmartContract" && contractType != "TransferContract" {
			continue
		}

		// TriggerSmartContract 的 log 不在 getblockbynum 响应里，需单独获取
		data := txRaw
		if contractType == "TriggerSmartContract" {
			data, err = f.injectTxLogs(ctx, txMeta.TxID, txRaw)
			if err != nil {
				// log 获取失败不阻断流程，用原始数据继续（log 为空会被 parser 跳过）
				data = txRaw
			}
		}

		events = append(events, chain.RawEvent{
			Chain:     f.chainName,
			TxHash:    txMeta.TxID,
			BlockNum:  num,
			BlockTime: blockTime,
			Type:      "tx",
			Data:      data,
		})
	}
	return events, nil
}

// injectTxLogs 调用 gettransactioninfobyid 获取 log，注入到原始 tx JSON 中
func (f *TRONFetcher) injectTxLogs(ctx context.Context, txID string, txRaw json.RawMessage) (json.RawMessage, error) {
	reqBody, _ := json.Marshal(map[string]string{"value": txID})
	_, body, err := f.client.Post(
		f.baseURL+"/walletsolidity/gettransactioninfobyid",
		reqBody,
		map[string]string{"Content-Type": "application/json"},
	)
	if err != nil {
		return txRaw, fmt.Errorf("获取 tx info 失败: %w", err)
	}

	var info struct {
		Log []json.RawMessage `json:"log"`
	}
	if err := json.Unmarshal(body, &info); err != nil || len(info.Log) == 0 {
		return txRaw, nil // 无 log，直接返回原始数据
	}

	// 将 tx JSON 反序列化为 map，注入 log 字段后重新序列化
	var txMap map[string]json.RawMessage
	if err := json.Unmarshal(txRaw, &txMap); err != nil {
		return txRaw, err
	}
	logBytes, err := json.Marshal(info.Log)
	if err != nil {
		return txRaw, err
	}
	txMap["log"] = logBytes

	merged, err := json.Marshal(txMap)
	if err != nil {
		return txRaw, err
	}
	return merged, nil
}

func (f *TRONFetcher) HealthCheck(ctx context.Context) bool {
	_, err := f.LatestBlockNum(ctx)
	return err == nil
}
