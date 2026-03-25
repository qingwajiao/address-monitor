package middleware

import (
	"errors"
	"strings"

	jwtpkg "address-monitor/pkg/jwt"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyUserID = "user_id"
	ContextKeyEmail  = "email"
)

func JWTAuth(jwtManager *jwtpkg.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			abortUnauthorized(c, "缺少 Authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			abortUnauthorized(c, "Authorization 格式错误，应为 Bearer {token}")
			return
		}

		claims, err := jwtManager.ParseAccessToken(parts[1])
		if err != nil {
			msg := "token 无效"
			if errors.Is(err, jwtpkg.ErrTokenExpired) {
				msg = "token 已过期"
			}
			abortUnauthorized(c, msg)
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyEmail, claims.Email)
		c.Next()
	}
}

// GetUserID 从 context 取当前登录用户 ID
func GetUserID(c *gin.Context) uint64 {
	if v, exists := c.Get(ContextKeyUserID); exists {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}
