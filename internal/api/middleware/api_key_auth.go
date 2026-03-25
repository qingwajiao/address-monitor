package middleware

import (
	"net/http"

	"address-monitor/internal/store"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyAppID  = "app_id"
	ContextKeySecret = "app_secret"
)

func APIKeyAuth(appStore *store.AppStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			abortUnauthorized(c, "缺少 X-API-Key header")
			return
		}

		app, err := appStore.GetByAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			abortUnauthorized(c, "无效的 API Key")
			return
		}

		c.Set(ContextKeyAppID, app.ID)
		c.Set(ContextKeySecret, app.Secret)
		c.Next()
	}
}

// GetAppID 从 context 取当前请求的 app ID
func GetAppID(c *gin.Context) uint64 {
	if v, exists := c.Get(ContextKeyAppID); exists {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}

// abortUnauthorized 统一的 401 响应，格式与 handler.Fail 一致
func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code": 0,
		"msg":  msg,
		"data": nil,
	})
}
