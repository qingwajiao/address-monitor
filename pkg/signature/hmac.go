package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign 对 payload 用 secret 做 HMAC-SHA256 签名
// 返回格式：sha256=xxxxxxxx
func Sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify 验证签名是否正确
func Verify(payload []byte, secret, sigHeader string) bool {
	expected := Sign(payload, secret)
	return hmac.Equal([]byte(expected), []byte(sigHeader))
}
