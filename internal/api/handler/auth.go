package handler

import (
	"errors"
	"net/http"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/api/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.svc.Register(c.Request.Context(), &req); err != nil {
		if errors.Is(err, service.ErrEmailExists) {
			Fail(c, http.StatusConflict, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, gin.H{"message": "注册成功，请查收验证邮件"})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		Fail(c, http.StatusBadRequest, "缺少 token 参数")
		return
	}

	if err := h.svc.VerifyEmail(c.Request.Context(), token); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	Success(c, gin.H{"message": "邮箱验证成功，请登录"})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.Login(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) ||
			errors.Is(err, service.ErrPasswordWrong) {
			Fail(c, http.StatusUnauthorized, "邮箱或密码错误")
			return
		}
		if errors.Is(err, service.ErrEmailNotVerified) {
			Fail(c, http.StatusForbidden, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, result)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		Fail(c, http.StatusUnauthorized, err.Error())
		return
	}

	Success(c, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	h.svc.Logout(c.Request.Context(), req.RefreshToken)
	Success(c, gin.H{"message": "已登出"})
}

func (h *AuthHandler) ResendVerify(c *gin.Context) {
	var req dto.ResendVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.svc.ResendVerify(c.Request.Context(), &req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	Success(c, gin.H{"message": "验证邮件已重新发送"})
}
