package signature

import "testing"

func TestSignAndVerify(t *testing.T) {
	payload := []byte(`{"event":"test"}`)
	secret := "my-secret-key"

	sig := Sign(payload, secret)
	if sig == "" {
		t.Fatal("签名不能为空")
	}
	t.Logf("签名结果: %s", sig)

	if !Verify(payload, secret, sig) {
		t.Fatal("验签应该通过")
	}

	if Verify(payload, "wrong-secret", sig) {
		t.Fatal("错误 secret 验签应该失败")
	}

	if Verify([]byte("tampered1"), secret, sig) {
		t.Fatal("篡改内容验签应该失败")
	}

	t.Log("所有签名测试通过")
}
