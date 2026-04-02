package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"address-monitor/internal/mq"
	"address-monitor/internal/parser"
	"address-monitor/internal/store"
	"address-monitor/pkg/addrcodec"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type DispatchTask struct {
	WebhookLogID uint64                 `json:"webhook_log_id"`
	AppID        uint64                 `json:"app_id"`
	AddressID    uint64                 `json:"address_id"`
	CallbackURL  string                 `json:"callback_url"`
	Secret       string                 `json:"secret"`
	Event        parser.NormalizedEvent `json:"event"`
}

type Expander struct {
	publisher    *mq.Publisher
	addrStore    *store.WatchedAddressStore
	webhookStore *store.WebhookLogStore
	appStore     *store.AppStore
	rdb          *redis.Client
}

func NewExpander(
	publisher *mq.Publisher,
	addrStore *store.WatchedAddressStore,
	webhookStore *store.WebhookLogStore,
	appStore *store.AppStore,
	rdb *redis.Client,
) *Expander {
	return &Expander{
		publisher:    publisher,
		addrStore:    addrStore,
		webhookStore: webhookStore,
		appStore:     appStore,
		rdb:          rdb,
	}
}

func (e *Expander) Handle(msg amqp.Delivery) {
	var event parser.NormalizedEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		zap.L().Error("解析 matched.events 消息失败", zap.Error(err))
		msg.Ack(false)
		return
	}

	ctx := context.Background()

	zap.L().Debug("收到 matched.events 消息",
		zap.String("event_id", event.EventID),
		zap.String("chain", event.Chain),
		zap.String("type", string(event.EventType)),
		zap.String("address", event.WatchedAddress),
	)

	was := e.getWatchedAddresses(ctx, event.Chain, event.WatchedAddress)

	for _, wa := range was {
		// 幂等检查
		exists, err := e.webhookStore.ExistsByEventAndAddress(ctx, event.EventID, wa.ID)
		if err != nil {
			zap.L().Error("幂等检查失败", zap.Error(err))
			continue
		}
		if exists {
			continue
		}

		// 从 apps 表获取回调地址、签名密钥和合约白名单
		info, err := e.getAppInfo(ctx, wa.AppID)
		if err != nil {
			zap.L().Error("获取 App 信息失败",
				zap.Uint64("app_id", wa.AppID),
				zap.Error(err),
			)
			continue
		}

		if info.callbackURL == "" {
			zap.L().Warn("App 未配置回调地址，跳过推送",
				zap.Uint64("app_id", wa.AppID),
			)
			continue
		}

		// App 级合约白名单过滤
		contractAddr := ""
		if event.Asset != nil {
			contractAddr = event.Asset.ContractAddress
		}
		if !isContractAllowed(info.allowedContracts, event.Chain, contractAddr) {
			zap.L().Debug("App 合约白名单过滤",
				zap.Uint64("app_id", wa.AppID),
				zap.String("chain", event.Chain),
				zap.String("contract", contractAddr),
			)
			continue
		}

		task := DispatchTask{
			AppID:       wa.AppID,
			AddressID:   wa.ID,
			CallbackURL: info.callbackURL,
			Secret:      info.secret,
			Event:       event,
		}

		payload, _ := json.Marshal(task)

		// 先写 webhook_log(pending)
		wl := &store.WebhookLog{
			EventID:     event.EventID,
			AppID:       wa.AppID,
			AddressID:   wa.ID,
			Chain:       event.Chain,
			TxHash:      event.TxHash,
			EventType:   string(event.EventType),
			Payload:     string(payload),
			Status:      "pending",
			CallbackURL: info.callbackURL,
		}
		if err := e.webhookStore.Create(ctx, wl); err != nil {
			zap.L().Error("写入 webhook_log 失败", zap.Error(err))
			continue
		}

		// 补充 WebhookLogID 后重新序列化
		task.WebhookLogID = wl.ID
		payload, _ = json.Marshal(task)

		// 再发 dispatch.tasks
		if err := e.publisher.Publish(
			"dispatch.exchange",
			"dispatch",
			payload,
			amqp.Table{"x-retry-count": int32(0)},
		); err != nil {
			zap.L().Error("发布 dispatch.tasks 失败",
				zap.String("event_id", event.EventID),
				zap.Error(err),
			)
			continue
		}

		zap.L().Info("推送任务已创建",
			zap.String("event_id", event.EventID),
			zap.Uint64("app_id", wa.AppID),
			zap.Uint64("address_id", wa.ID),
			zap.String("callback", info.callbackURL),
		)
	}

	msg.Ack(false)
}

func (e *Expander) getWatchedAddresses(ctx context.Context, chain, address string) []*store.WatchedAddress {
	addr := strings.ToLower(address)
	cacheKey := fmt.Sprintf("sub_cache:%s:%s", chain, addr)

	if cached, err := e.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
		var was []*store.WatchedAddress
		if json.Unmarshal(cached, &was) == nil {
			return was
		}
	}

	was, err := e.addrStore.ListByChainAddress(ctx, chain, addr)
	if err != nil || len(was) == 0 {
		return nil
	}

	if data, err := json.Marshal(was); err == nil {
		e.rdb.Set(ctx, cacheKey, data, 5*time.Minute)
	}
	return was
}

type appInfo struct {
	callbackURL      string
	secret           string
	allowedContracts map[string]map[string]struct{} // chain -> set of contract_address
}

func (e *Expander) getAppInfo(ctx context.Context, appID uint64) (*appInfo, error) {
	cacheKey := fmt.Sprintf("app_info:%d", appID)

	vals, err := e.rdb.HMGet(ctx, cacheKey, "callback_url", "secret", "allowed_contracts").Result()
	if err == nil && len(vals) == 3 && vals[0] != nil && vals[1] != nil {
		info := &appInfo{
			callbackURL: vals[0].(string),
			secret:      vals[1].(string),
		}
		if vals[2] != nil {
			info.allowedContracts = parseAllowedContracts(vals[2].(string))
		}
		return info, nil
	}

	app, err := e.appStore.GetByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	e.rdb.HSet(ctx, cacheKey,
		"callback_url", app.CallbackURL,
		"secret", app.Secret,
		"allowed_contracts", app.AllowedContracts,
	)
	e.rdb.Expire(ctx, cacheKey, 10*time.Minute)

	return &appInfo{
		callbackURL:      app.CallbackURL,
		secret:           app.Secret,
		allowedContracts: parseAllowedContracts(app.AllowedContracts),
	}, nil
}

// parseAllowedContracts 将 JSON 字符串解析为 chain -> set 结构
// DB 中地址已是链原生格式，直接使用
func parseAllowedContracts(raw string) map[string]map[string]struct{} {
	if raw == "" {
		return nil
	}
	var m map[string][]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	result := make(map[string]map[string]struct{}, len(m))
	for c, addrs := range m {
		set := make(map[string]struct{}, len(addrs))
		for _, a := range addrs {
			set[a] = struct{}{}
		}
		result[strings.ToUpper(c)] = set
	}
	return result
}

// isContractAllowed 检查事件合约地址是否在该 App 的白名单内
// 原生转账（空合约地址）或 App 未配置白名单时始终放行
func isContractAllowed(allowed map[string]map[string]struct{}, chainName, contractAddress string) bool {
	if contractAddress == "" || len(allowed) == 0 {
		return true
	}
	contracts, ok := allowed[strings.ToUpper(chainName)]
	if !ok || len(contracts) == 0 {
		return true
	}
	// event.Asset.ContractAddress 已是 Parser 输出的链原生格式
	codec := addrcodec.Get(chainName)
	_, exists := contracts[codec.Normalize(contractAddress)]
	return exists
}

// 确保 uuid 包被使用
var _ = uuid.New
