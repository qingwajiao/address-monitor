package dto

import "time"

// ── 请求 ──────────────────────────────────────────────────

type CreateAddressReq struct {
	Chain   string `json:"chain" binding:"required"`
	Address string `json:"address" binding:"required"`
	Label   string `json:"label"`
}

type BatchCreateAddressReq struct {
	Chain     string   `json:"chain" binding:"required"`
	Addresses []string `json:"addresses" binding:"required,min=1,max=10000"`
	Label     string   `json:"label"`
}

type ListAddressReq struct {
	Chain string `form:"chain"`
	Page  int    `form:"page"`
	Size  int    `form:"size"`
}

// ── 响应 ──────────────────────────────────────────────────

type AddressResp struct {
	ID        uint64    `json:"id"`
	Chain     string    `json:"chain"`
	Address   string    `json:"address"`
	Label     string    `json:"label"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type BatchCreateAddressResp struct {
	Total   int           `json:"total"`
	Success []AddressResp `json:"success"`
	Fail    []FailItem    `json:"fail"`
}

type FailItem struct {
	Address string `json:"address"`
	Error   string `json:"error"`
}

type ListAddressResp struct {
	List  []*AddressResp `json:"list"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}
