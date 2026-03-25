package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/matcher"
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

var (
	ErrAddressNotFound  = errors.New("地址不存在")
	ErrAddressForbidden = errors.New("无权操作此地址")
)

type AddressService struct {
	addrStore *store.WatchedAddressStore
	rdb       *redis.Client
}

func NewAddressService(
	addrStore *store.WatchedAddressStore,
	rdb *redis.Client,
) *AddressService {
	return &AddressService{
		addrStore: addrStore,
		rdb:       rdb,
	}
}

func (s *AddressService) Create(ctx context.Context, appID uint64, req *dto.CreateAddressReq) (*dto.AddressResp, error) {
	wa := &store.WatchedAddress{
		AppID:   appID,
		Chain:   strings.ToUpper(req.Chain),
		Address: strings.ToLower(req.Address),
		Label:   req.Label,
		Status:  1,
	}

	if err := s.addrStore.Create(ctx, wa); err != nil {
		return nil, err
	}

	s.publishAddEvent(ctx, wa.Chain, wa.Address)

	zap.L().Info("新增监控地址",
		zap.String("chain", wa.Chain),
		zap.String("address", wa.Address),
		zap.Uint64("app_id", appID),
	)

	return toAddressResp(wa), nil
}

func (s *AddressService) BatchCreate(ctx context.Context, appID uint64, req *dto.BatchCreateAddressReq) (*dto.BatchCreateAddressResp, error) {
	chain := strings.ToUpper(req.Chain)

	// 去重
	seen := make(map[string]struct{})
	var validAddrs []string
	for _, addr := range req.Addresses {
		addr = strings.TrimSpace(strings.ToLower(addr))
		if addr == "" {
			continue
		}
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		validAddrs = append(validAddrs, addr)
	}

	if len(validAddrs) == 0 {
		return nil, fmt.Errorf("没有有效地址")
	}

	var successList []dto.AddressResp
	var failList []dto.FailItem
	const batchSize = 500

	for i := 0; i < len(validAddrs); i += batchSize {
		end := i + batchSize
		if end > len(validAddrs) {
			end = len(validAddrs)
		}
		batch := validAddrs[i:end]

		was := make([]*store.WatchedAddress, 0, len(batch))
		for _, addr := range batch {
			was = append(was, &store.WatchedAddress{
				AppID:   appID,
				Chain:   chain,
				Address: addr,
				Label:   req.Label,
				Status:  1,
			})
		}

		if err := s.addrStore.BatchCreate(ctx, was); err != nil {
			for _, addr := range batch {
				failList = append(failList, dto.FailItem{
					Address: addr,
					Error:   err.Error(),
				})
			}
			continue
		}

		for _, wa := range was {
			successList = append(successList, *toAddressResp(wa))
		}
	}

	// Redis Pipeline 批量操作（只推送入库成功的地址）
	if len(successList) > 0 {
		hotKey := fmt.Sprintf("watch:hot:%s", chain)
		incrKey := fmt.Sprintf("bf:incremental:%s", chain)

		members := make([]interface{}, len(successList))
		for i, item := range successList {
			members[i] = item.Address
		}

		pipe := s.rdb.Pipeline()
		pipe.SAdd(ctx, hotKey, members...)
		pipe.LPush(ctx, incrKey, members...)
		pipe.LTrim(ctx, incrKey, 0, matcher.IncrementalMaxLen-1)
		pipe.Publish(ctx, matcher.AddressEventChannel, (&matcher.AddressEvent{
			Type:  matcher.EventTypeBatchAdd,
			Chain: chain,
			Count: len(successList),
		}).Encode())
		if _, err := pipe.Exec(ctx); err != nil {
			zap.L().Warn("Redis Pipeline 执行失败", zap.Error(err))
		}
	}

	zap.L().Info("批量添加监控地址完成",
		zap.String("chain", chain),
		zap.Uint64("app_id", appID),
		zap.Int("success", len(successList)),
		zap.Int("fail", len(failList)),
	)

	return &dto.BatchCreateAddressResp{
		Total:   len(validAddrs),
		Success: successList,
		Fail:    failList,
	}, nil
}

func (s *AddressService) Delete(ctx context.Context, appID, id uint64) error {
	wa, err := s.addrStore.GetByID(ctx, id)
	if err != nil {
		return ErrAddressNotFound
	}
	if wa.AppID != appID {
		return ErrAddressForbidden
	}

	if err := s.addrStore.Delete(ctx, id); err != nil {
		return err
	}

	pipe := s.rdb.Pipeline()
	pipe.SRem(ctx, fmt.Sprintf("watch:hot:%s", wa.Chain), wa.Address)
	pipe.Del(ctx, fmt.Sprintf("sub_cache:%s:%s", wa.Chain, wa.Address))
	pipe.Publish(ctx, matcher.AddressEventChannel, (&matcher.AddressEvent{
		Type:    matcher.EventTypeRemove,
		Chain:   wa.Chain,
		Address: wa.Address,
	}).Encode())
	if _, err := pipe.Exec(ctx); err != nil {
		zap.L().Warn("Redis 清理失败（不影响主流程）",
			zap.Uint64("id", id),
			zap.String("chain", wa.Chain),
			zap.String("address", wa.Address),
			zap.Error(err),
		)
	}

	zap.L().Info("删除监控地址",
		zap.Uint64("id", id),
		zap.String("chain", wa.Chain),
		zap.String("address", wa.Address),
	)
	return nil
}

func (s *AddressService) GetByID(ctx context.Context, appID, id uint64) (*dto.AddressResp, error) {
	wa, err := s.addrStore.GetByID(ctx, id)
	if err != nil {
		return nil, ErrAddressNotFound
	}
	if wa.AppID != appID {
		return nil, ErrAddressForbidden
	}
	return toAddressResp(wa), nil
}

func (s *AddressService) List(ctx context.Context, appID uint64, req *dto.ListAddressReq) (*dto.ListAddressResp, error) {
	page, size := normalizePage(req.Page, req.Size)
	was, total, err := s.addrStore.ListByApp(ctx, appID, req.Chain, page, size)
	if err != nil {
		return nil, err
	}

	list := make([]*dto.AddressResp, 0, len(was))
	for _, wa := range was {
		list = append(list, toAddressResp(wa))
	}

	return &dto.ListAddressResp{
		List:  list,
		Total: total,
		Page:  page,
		Size:  size,
	}, nil
}

func (s *AddressService) publishAddEvent(ctx context.Context, chain, address string) {
	incrKey := fmt.Sprintf("bf:incremental:%s", chain)
	hotKey := fmt.Sprintf("watch:hot:%s", chain)
	event := (&matcher.AddressEvent{
		Type:    matcher.EventTypeAdd,
		Chain:   chain,
		Address: address,
	}).Encode()

	pipe := s.rdb.Pipeline()
	pipe.SAdd(ctx, hotKey, address)
	pipe.LPush(ctx, incrKey, address)
	pipe.LTrim(ctx, incrKey, 0, matcher.IncrementalMaxLen-1)
	pipe.Publish(ctx, matcher.AddressEventChannel, event)
	if _, err := pipe.Exec(ctx); err != nil {
		zap.L().Warn("Redis 写入失败（不影响主流程）",
			zap.String("chain", chain),
			zap.String("address", address),
			zap.Error(err),
		)
	}
}

func toAddressResp(wa *store.WatchedAddress) *dto.AddressResp {
	return &dto.AddressResp{
		ID:        wa.ID,
		Chain:     wa.Chain,
		Address:   wa.Address,
		Label:     wa.Label,
		Status:    wa.Status,
		CreatedAt: wa.CreatedAt,
	}
}
