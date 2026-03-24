package dto

import "time"

// ── 请求 ──────────────────────────────────────────────────

type SetWebhookURLReq struct {
	URL string `json:"url" binding:"required,url"`
}

type ListDeliveryReq struct {
	Chain  string `form:"chain"`
	Status string `form:"status"`
	Page   int    `form:"page"`
	Size   int    `form:"size"`
}

// ── 响应 ──────────────────────────────────────────────────

type WebhookURLResp struct {
	URL string `json:"url"`
}

type DeliveryResp struct {
	ID           uint64    `json:"id"`
	EventID      string    `json:"event_id"`
	Chain        string    `json:"chain"`
	TxHash       string    `json:"tx_hash"`
	EventType    string    `json:"event_type"`
	Status       string    `json:"status"`
	RetryCount   int       `json:"retry_count"`
	CallbackURL  string    `json:"callback_url"`
	ResponseCode *int      `json:"response_code"`
	CreatedAt    time.Time `json:"created_at"`
}

type ListDeliveryResp struct {
	List  []*DeliveryResp `json:"list"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
}
