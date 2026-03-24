package email

import (
	"fmt"

	"go.uber.org/zap"
	"gopkg.in/gomail.v2"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	DevMode  bool // dev 模式下只打日志，不真实发送
}

type Sender struct {
	cfg *Config
}

func NewSender(cfg *Config) *Sender {
	return &Sender{cfg: cfg}
}

// SendVerificationEmail 发送邮箱验证邮件
func (s *Sender) SendVerificationEmail(toEmail, token, baseURL string) error {
	verifyURL := fmt.Sprintf("%s/auth/verify-email?token=%s", baseURL, token)

	subject := "验证您的邮箱 - Address Monitor"
	body := fmt.Sprintf(`
<h2>欢迎注册 Address Monitor</h2>
<p>请点击下方链接验证您的邮箱地址：</p>
<p><a href="%s">%s</a></p>
<p>链接有效期 30 分钟，过期后请重新申请。</p>
<p>如果您没有注册账号，请忽略此邮件。</p>
`, verifyURL, verifyURL)

	return s.send(toEmail, subject, body)
}

func (s *Sender) send(to, subject, htmlBody string) error {
	if s.cfg.DevMode {
		// dev 模式只打日志，不真实发送
		zap.L().Info("【DEV】模拟发送邮件",
			zap.String("to", to),
			zap.String("subject", subject),
			zap.String("body", htmlBody),
		)
		return nil
	}

	m := gomail.NewMessage()
	m.SetHeader("From", s.cfg.From)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)

	d := gomail.NewDialer(s.cfg.Host, s.cfg.Port, s.cfg.Username, s.cfg.Password)
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	return nil
}
