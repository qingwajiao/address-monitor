package handler

import (
	"net/http"
	"strconv"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/api/service"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	svc *service.WebhookService
}

func NewWebhookHandler(svc *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

func (h *WebhookHandler) SetWebhookURL(c *gin.Context) {
	var req dto.SetWebhookURLReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := c.GetHeader("X-API-Key")
	if err := h.svc.SetWebhookURL(c.Request.Context(), userID, &req); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, nil)
}

func (h *WebhookHandler) GetWebhookURL(c *gin.Context) {
	userID := c.GetHeader("X-API-Key")
	result, err := h.svc.GetWebhookURL(c.Request.Context(), userID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, result)
}

func (h *WebhookHandler) ListDeliveries(c *gin.Context) {
	var req dto.ListDeliveryReq
	c.ShouldBindQuery(&req)

	userID := c.GetHeader("X-API-Key")
	result, err := h.svc.ListDeliveries(c.Request.Context(), userID, &req)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, result)
}

func (h *WebhookHandler) Resend(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.svc.Resend(c.Request.Context(), id); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, nil)
}
