package api

import (
	"net/http"

	"address-monitor/internal/api/handler"
	"address-monitor/internal/api/middleware"
	"address-monitor/internal/config"
	"address-monitor/internal/matcher"
	"address-monitor/internal/mq"
	"address-monitor/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func NewRouter(
	cfg *config.Config,
	subStore *store.SubscriptionStore,
	deliveryStore *store.DeliveryStore,
	m *matcher.Matcher,
	rdb *redis.Client,
	publisher *mq.Publisher,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	// 健康检查（不需要鉴权）
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// v1 路由组，需要鉴权和限流
	v1 := r.Group("/v1",
		middleware.APIKeyAuth(cfg.Server.APIKey),
		middleware.RateLimit(),
	)

	// 订阅管理
	addrHandler := handler.NewAddressHandler(subStore, m, rdb)
	v1.POST("/subscriptions", addrHandler.Create)
	v1.GET("/subscriptions", addrHandler.List)
	v1.GET("/subscriptions/:id", addrHandler.Get)
	v1.DELETE("/subscriptions/:id", addrHandler.Delete)

	webhookHandler := handler.NewWebhookHandler(deliveryStore, publisher, rdb)
	// 推送记录
	v1.GET("/deliveries", webhookHandler.List)
	v1.POST("/deliveries/:id/resend", webhookHandler.Resend)

	// 全局 Webhook URL 管理
	v1.POST("/webhook/url", webhookHandler.SetWebhookURL)
	v1.GET("/webhook/url", webhookHandler.GetWebhookURL)

	return r
}
