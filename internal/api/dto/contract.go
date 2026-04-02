package dto

type CreateContractReq struct {
	Chain           string `json:"chain" binding:"required"`
	ContractAddress string `json:"contract_address" binding:"required"`
	Symbol          string `json:"symbol" binding:"required"`
	Decimals        int    `json:"decimals"`
}

type UpdateContractReq struct {
	Symbol   *string `json:"symbol"`
	Decimals *int    `json:"decimals"`
	Enabled  *int    `json:"enabled"`
}

type ContractResp struct {
	ID              uint64 `json:"id"`
	Chain           string `json:"chain"`
	ContractAddress string `json:"contract_address"`
	Symbol          string `json:"symbol"`
	Decimals        int    `json:"decimals"`
	Enabled         int    `json:"enabled"`
}
