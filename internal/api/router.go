package api

import (
	"net/http"

	"address-monitor/internal/api/handler"
	"address-monitor/internal/api/middleware"
	"address-monitor/internal/api/service"
	"address-monitor/internal/config"
	"address-monitor/internal/mq"
	"address-monitor/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func NewRouter(
	cfg *config.Config,
	subStore *store.SubscriptionStore,
	deliveryStore *store.DeliveryStore,
	rdb *redis.Client,
	publisher *mq.Publisher,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 初始化 Service
	subSvc := service.NewSubscriptionService(subStore, rdb)
	webhookSvc := service.NewWebhookService(deliveryStore, publisher, rdb)

	// 初始化 Handler
	addrHandler := handler.NewAddressHandler(subSvc)
	webhookHandler := handler.NewWebhookHandler(webhookSvc)

	// 路由注册
	v1 := r.Group("/v1",
		middleware.APIKeyAuth(cfg.Server.APIKey),
		middleware.RateLimit(),
	)
	{
		// 订阅管理
		v1.POST("/subscriptions", addrHandler.Create)
		v1.POST("/subscriptions/batch", addrHandler.BatchCreate)
		v1.GET("/subscriptions", addrHandler.List)
		v1.GET("/subscriptions/:id", addrHandler.Get)
		v1.DELETE("/subscriptions/:id", addrHandler.Delete)

		// 全局 Webhook URL
		v1.POST("/webhook/url", webhookHandler.SetWebhookURL)
		v1.GET("/webhook/url", webhookHandler.GetWebhookURL)

		// 推送记录
		v1.GET("/deliveries", webhookHandler.ListDeliveries)
		v1.POST("/deliveries/:id/resend", webhookHandler.Resend)
	}

	return r
}
