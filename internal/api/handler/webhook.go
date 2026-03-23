package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"address-monitor/internal/mq"
	"address-monitor/internal/store"

	"github.com/go-redis/redis/v8"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	deliveryStore *store.DeliveryStore
	publisher     *mq.Publisher
	rdb           *redis.Client
}

func NewWebhookHandler(
	deliveryStore *store.DeliveryStore,
	publisher *mq.Publisher,
	rdb *redis.Client, // 新增
) *WebhookHandler {
	return &WebhookHandler{
		deliveryStore: deliveryStore,
		publisher:     publisher,
		rdb:           rdb, // 新增
	}
}

func (h *WebhookHandler) List(c *gin.Context) {
	userID := c.GetHeader("X-API-Key")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	chain := strings.ToUpper(c.Query("chain")) // 可选，按链过滤
	status := c.Query("status")                // 可选，按状态过滤

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	logs, total, err := h.deliveryStore.ListByUser(c.Request.Context(), userID, chain, status, page, size)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, gin.H{
		"list": logs,
		"meta": gin.H{
			"total": total,
			"page":  page,
			"size":  size,
		},
	})
}
func (h *WebhookHandler) Resend(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	// 按 ID 查询推送记录
	log, err := h.deliveryStore.GetByID(c.Request.Context(), id)
	if err != nil {
		Fail(c, http.StatusNotFound, "delivery log not found")
		return
	}

	// 重新发布到 dispatch.exchange
	if err := h.publisher.Publish(
		"dispatch.exchange",
		"dispatch",
		[]byte(log.Payload),
		amqp.Table{"x-retry-count": int32(0)},
	); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	zap.L().Info("手动重推推送任务",
		zap.Uint64("delivery_id", id),
		zap.String("chain", log.Chain),
		zap.String("tx_hash", log.TxHash),
	)

	Success(c, gin.H{"message": "resent"})
}

// SetWebhookURL 设置全局 Webhook URL
func (h *WebhookHandler) SetWebhookURL(c *gin.Context) {
	userID := c.GetHeader("X-API-Key")

	var req struct {
		URL string `json:"url" binding:"required,url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	key := fmt.Sprintf("webhook:url:%s", userID)
	if err := h.rdb.Set(c.Request.Context(), key, req.URL, 0).Err(); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	zap.L().Info("设置全局 Webhook URL",
		zap.String("user_id", userID),
		zap.String("url", req.URL),
	)

	Success(c, nil)
}

// GetWebhookURL 查询全局 Webhook URL
func (h *WebhookHandler) GetWebhookURL(c *gin.Context) {
	userID := c.GetHeader("X-API-Key")
	key := fmt.Sprintf("webhook:url:%s", userID)

	url, err := h.rdb.Get(c.Request.Context(), key).Result()
	if err != nil {
		Success(c, gin.H{"url": ""})
		return
	}

	Success(c, gin.H{"url": url})
}
