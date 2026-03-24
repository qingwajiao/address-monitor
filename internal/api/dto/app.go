package dto

import "time"

// ── 请求 ──────────────────────────────────────────────────

type CreateAppReq struct {
	Name        string `json:"name" binding:"required,min=1,max=64"`
	CallbackURL string `json:"callback_url" binding:"omitempty,url"`
}

type UpdateAppReq struct {
	Name        string `json:"name" binding:"omitempty,min=1,max=64"`
	CallbackURL string `json:"callback_url" binding:"omitempty,url"`
}

// ── 响应 ──────────────────────────────────────────────────

type AppResp struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	APIKey      string    `json:"api_key"`
	Secret      string    `json:"secret,omitempty"` // 只在创建和重置时返回
	CallbackURL string    `json:"callback_url"`
	Status      int       `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}
