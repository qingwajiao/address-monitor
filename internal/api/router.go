package api

import (
	"net/http"

	"address-monitor/internal/api/handler"
	"address-monitor/internal/api/middleware"
	"address-monitor/internal/api/service"
	"address-monitor/internal/store"
	jwtpkg "address-monitor/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func NewRouter(
	appStore *store.AppStore,
	appSvc *service.AppService,
	addrSvc *service.AddressService,
	webhookSvc *service.WebhookService,
	authSvc *service.AuthService,
	contractSvc *service.ContractService,
	jwtManager *jwtpkg.Manager,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	authHandler := handler.NewAuthHandler(authSvc)
	appHandler := handler.NewAppHandler(appSvc)
	addrHandler := handler.NewAddressHandler(addrSvc)
	webhookHandler := handler.NewWebhookHandler(webhookSvc)
	contractHandler := handler.NewContractHandler(contractSvc)

	// 认证路由（无需鉴权）
	auth := r.Group("/auth")
	{
		auth.POST("/register", authHandler.Register)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)
		auth.GET("/verify-email", authHandler.VerifyEmail)
		auth.POST("/resend-verify", authHandler.ResendVerify)
	}

	// JWT 鉴权路由（管理接口）
	v1jwt := r.Group("/v1", middleware.JWTAuth(jwtManager))
	{
		v1jwt.POST("/apps", appHandler.Create)
		v1jwt.GET("/apps", appHandler.List)
		v1jwt.GET("/apps/:id", appHandler.Get)
		v1jwt.PUT("/apps/:id", appHandler.Update)
		v1jwt.DELETE("/apps/:id", appHandler.Delete)
		v1jwt.POST("/apps/:id/reset-key", appHandler.ResetAPIKey)
		v1jwt.POST("/apps/:id/reset-secret", appHandler.ResetSecret)
		v1jwt.PUT("/apps/:id/allowed-contracts", appHandler.UpdateAllowedContracts)
	}

	// Admin 路由（JWT + admin role）
	admin := r.Group("/v1/admin", middleware.JWTAuth(jwtManager), middleware.AdminOnly())
	{
		admin.GET("/contracts", contractHandler.List)
		admin.POST("/contracts", contractHandler.Create)
		admin.PUT("/contracts/:id", contractHandler.Update)
		admin.DELETE("/contracts/:id", contractHandler.Delete)
	}

	// API Key 鉴权路由（数据接口）
	v1api := r.Group("/v1",
		middleware.APIKeyAuth(appStore),
		//middleware.RateLimit(),
	)
	{
		v1api.POST("/addresses", addrHandler.Create)
		v1api.POST("/addresses/batch", addrHandler.BatchCreate)
		v1api.GET("/addresses", addrHandler.List)
		v1api.GET("/addresses/:id", addrHandler.Get)
		v1api.DELETE("/addresses/:id", addrHandler.Delete)

		v1api.GET("/webhook/logs", webhookHandler.ListLogs)
		v1api.POST("/webhook/logs/:id/resend", webhookHandler.Resend)
		v1api.GET("/webhook/url", webhookHandler.GetWebhookURL)
		v1api.POST("/webhook/url", webhookHandler.SetWebhookURL)
	}

	return r
}
