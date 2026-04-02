package service

import (
	"context"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/store"
)

type ContractService struct {
	contractStore *store.AllowedContractStore
}

func NewContractService(contractStore *store.AllowedContractStore) *ContractService {
	return &ContractService{contractStore: contractStore}
}

func (s *ContractService) Create(ctx context.Context, req *dto.CreateContractReq) (*dto.ContractResp, error) {
	c := &store.AllowedContract{
		Chain:           req.Chain,
		ContractAddress: req.ContractAddress,
		Symbol:          req.Symbol,
		Decimals:        req.Decimals,
		Enabled:         1,
	}
	if err := s.contractStore.Create(ctx, c); err != nil {
		return nil, err
	}
	return toContractResp(c), nil
}

func (s *ContractService) List(ctx context.Context) ([]*dto.ContractResp, error) {
	contracts, err := s.contractStore.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]*dto.ContractResp, 0, len(contracts))
	for _, c := range contracts {
		list = append(list, toContractResp(c))
	}
	return list, nil
}

func (s *ContractService) Update(ctx context.Context, id uint64, req *dto.UpdateContractReq) error {
	updates := map[string]interface{}{}
	if req.Symbol != nil {
		updates["symbol"] = *req.Symbol
	}
	if req.Decimals != nil {
		updates["decimals"] = *req.Decimals
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		return nil
	}
	return s.contractStore.Update(ctx, id, updates)
}

func (s *ContractService) Delete(ctx context.Context, id uint64) error {
	return s.contractStore.Delete(ctx, id)
}

func toContractResp(c *store.AllowedContract) *dto.ContractResp {
	return &dto.ContractResp{
		ID:              c.ID,
		Chain:           c.Chain,
		ContractAddress: c.ContractAddress,
		Symbol:          c.Symbol,
		Decimals:        c.Decimals,
		Enabled:         c.Enabled,
	}
}
