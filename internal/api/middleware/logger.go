package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger 请求日志中间件
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 请求完成后记录日志
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		userID := c.GetHeader("X-API-Key")

		// 根据状态码选择日志级别
		if statusCode >= 500 {
			zap.L().Error("请求处理失败",
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Int("status", statusCode),
				zap.Duration("latency", latency),
				zap.String("client_ip", clientIP),
				zap.String("api_key", maskAPIKey(userID)),
				zap.String("error", c.Errors.ByType(gin.ErrorTypePrivate).String()),
			)
		} else if statusCode >= 400 {
			zap.L().Warn("请求参数错误",
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Int("status", statusCode),
				zap.Duration("latency", latency),
				zap.String("client_ip", clientIP),
				zap.String("api_key", maskAPIKey(userID)),
			)
		} else {
			zap.L().Info("请求完成",
				zap.String("method", method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Int("status", statusCode),
				zap.Duration("latency", latency),
				zap.String("client_ip", clientIP),
				zap.String("api_key", maskAPIKey(userID)),
			)
		}
	}
}

// maskAPIKey 对 API Key 做脱敏处理，只显示前4位和后4位
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
