package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/store"
	"address-monitor/pkg/email"
	jwtpkg "address-monitor/pkg/jwt"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrEmailExists      = errors.New("邮箱已被注册")
	ErrUserNotFound     = errors.New("用户不存在")
	ErrPasswordWrong    = errors.New("密码错误")
	ErrEmailNotVerified = errors.New("邮箱未验证，请先验证邮箱")
	ErrUserDisabled     = errors.New("账号已被禁用")
	ErrTokenInvalid     = errors.New("验证链接无效或已过期")
)

type AuthService struct {
	userStore         *store.UserStore
	emailVerifyStore  *store.EmailVerificationStore
	refreshTokenStore *store.RefreshTokenStore
	jwtManager        *jwtpkg.Manager
	emailSender       *email.Sender
	baseURL           string
}

func NewAuthService(
	userStore *store.UserStore,
	emailVerifyStore *store.EmailVerificationStore,
	refreshTokenStore *store.RefreshTokenStore,
	jwtManager *jwtpkg.Manager,
	emailSender *email.Sender,
	baseURL string,
) *AuthService {
	return &AuthService{
		userStore:         userStore,
		emailVerifyStore:  emailVerifyStore,
		refreshTokenStore: refreshTokenStore,
		jwtManager:        jwtManager,
		emailSender:       emailSender,
		baseURL:           baseURL,
	}
}

// Register 注册
func (s *AuthService) Register(ctx context.Context, req *dto.RegisterReq) error {
	// 检查邮箱是否已注册
	existing, err := s.userStore.GetByEmail(ctx, req.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if existing != nil {
		return ErrEmailExists
	}

	// 加密密码
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %w", err)
	}

	// 创建用户
	user := &store.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		Status:       0, // 未验证
	}
	if err := s.userStore.Create(ctx, user); err != nil {
		return err
	}

	// 生成验证 token
	token := uuid.New().String()
	verification := &store.EmailVerification{
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := s.emailVerifyStore.Create(ctx, verification); err != nil {
		return err
	}

	// 发送验证邮件
	if err := s.emailSender.SendVerificationEmail(req.Email, token, s.baseURL); err != nil {
		// 邮件发送失败不影响注册，打日志即可
		return fmt.Errorf("注册成功，但验证邮件发送失败，请重新发送: %w", err)
	}

	return nil
}

// VerifyEmail 验证邮箱
func (s *AuthService) VerifyEmail(ctx context.Context, token string) error {
	v, err := s.emailVerifyStore.GetByToken(ctx, token)
	if err != nil || v.Used == 1 || time.Now().After(v.ExpiresAt) {
		return ErrTokenInvalid
	}

	if err := s.userStore.UpdateStatus(ctx, v.UserID, 1); err != nil {
		return err
	}
	return s.emailVerifyStore.MarkUsed(ctx, v.ID)
}

// Login 登录
func (s *AuthService) Login(ctx context.Context, req *dto.LoginReq) (*dto.TokenResp, error) {
	user, err := s.userStore.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, ErrUserNotFound
	}

	switch user.Status {
	case 0:
		return nil, ErrEmailNotVerified
	case 2:
		return nil, ErrUserDisabled
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrPasswordWrong
	}

	return s.generateTokenPair(ctx, user)
}

// Refresh 刷新 Token
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*dto.TokenResp, error) {
	// 查 Refresh Token
	tokenHash := hashToken(refreshToken)
	rt, err := s.refreshTokenStore.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	// 撤销旧 Token（旋转机制）
	if err := s.refreshTokenStore.Revoke(ctx, rt.ID); err != nil {
		return nil, err
	}

	// 查用户
	user, err := s.userStore.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return s.generateTokenPair(ctx, user)
}

// Logout 登出（撤销 Refresh Token）
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := hashToken(refreshToken)
	rt, err := s.refreshTokenStore.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil // Token 不存在，视为已登出
	}
	return s.refreshTokenStore.Revoke(ctx, rt.ID)
}

// ResendVerify 重发验证邮件
func (s *AuthService) ResendVerify(ctx context.Context, req *dto.ResendVerifyReq) error {
	user, err := s.userStore.GetByEmail(ctx, req.Email)
	if err != nil {
		return ErrUserNotFound
	}
	if user.Status == 1 {
		return errors.New("邮箱已验证，无需重复验证")
	}

	token := uuid.New().String()
	verification := &store.EmailVerification{
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := s.emailVerifyStore.Create(ctx, verification); err != nil {
		return err
	}
	return s.emailSender.SendVerificationEmail(req.Email, token, s.baseURL)
}

// generateTokenPair 生成 Access Token + Refresh Token
func (s *AuthService) generateTokenPair(ctx context.Context, user *store.User) (*dto.TokenResp, error) {
	// 生成 Access Token
	accessToken, err := s.jwtManager.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		return nil, fmt.Errorf("生成 Access Token 失败: %w", err)
	}

	// 生成 Refresh Token（随机 UUID，存 hash）
	refreshToken := uuid.New().String()
	rt := &store.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: time.Now().Add(s.jwtManager.RefreshTokenTTL()),
	}
	if err := s.refreshTokenStore.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("存储 Refresh Token 失败: %w", err)
	}

	return &dto.TokenResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.jwtManager.AccessTokenTTL().Seconds()),
	}, nil
}

// hashToken 对 Token 做 SHA256，存数据库的是 hash 不是明文
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
