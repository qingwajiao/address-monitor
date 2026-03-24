package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenExpired = errors.New("token 已过期")
	ErrTokenInvalid = errors.New("token 无效")
)

type Claims struct {
	UserID uint64 `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret          []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewManager(secret string) *Manager {
	return &Manager{
		secret:          []byte(secret),
		accessTokenTTL:  2 * time.Hour,
		refreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// GenerateAccessToken 生成 Access Token（2小时有效）
func (m *Manager) GenerateAccessToken(userID uint64, email string) (string, error) {
	claims := &Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.accessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ParseAccessToken 解析并验证 Access Token
func (m *Manager) ParseAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalid
		}
		return m.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// AccessTokenTTL 返回 Access Token 有效期（用于设置 Cookie 等场景）
func (m *Manager) AccessTokenTTL() time.Duration {
	return m.accessTokenTTL
}

// RefreshTokenTTL 返回 Refresh Token 有效期
func (m *Manager) RefreshTokenTTL() time.Duration {
	return m.refreshTokenTTL
}
