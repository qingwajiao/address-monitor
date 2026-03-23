package middleware

import (
	"github.com/gin-gonic/gin"
)

func APIKeyAuth(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing X-API-Key header"})
			return
		}
		if key != apiKey {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid api key"})
			return
		}
		c.Next()
	}
}
