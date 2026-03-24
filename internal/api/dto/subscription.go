package dto

import "time"

// ── 请求 ──────────────────────────────────────────────────

type CreateSubReq struct {
	Chain       string `json:"chain" binding:"required"`
	Address     string `json:"address" binding:"required"`
	CallbackURL string `json:"callback_url" binding:"omitempty,url"`
	Label       string `json:"label"`
}

type BatchCreateSubReq struct {
	Chain       string   `json:"chain" binding:"required"`
	Addresses   []string `json:"addresses" binding:"required,min=1,max=10000"`
	CallbackURL string   `json:"callback_url" binding:"omitempty,url"`
	Label       string   `json:"label"`
}

type ListSubReq struct {
	Page int `form:"page"`
	Size int `form:"size"`
}

// ── 响应 ──────────────────────────────────────────────────

type SubResp struct {
	ID          uint64    `json:"id"`
	Chain       string    `json:"chain"`
	Address     string    `json:"address"`
	Label       string    `json:"label"`
	CallbackURL string    `json:"callback_url"`
	Secret      string    `json:"secret,omitempty"` // 只在创建时返回
	Status      int       `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type BatchCreateSubResp struct {
	Total   int        `json:"total"`
	Success []SubResp  `json:"success"`
	Fail    []FailItem `json:"fail"`
}

type FailItem struct {
	Address string `json:"address"`
	Error   string `json:"error"`
}

type ListSubResp struct {
	List  []*SubResp `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}
