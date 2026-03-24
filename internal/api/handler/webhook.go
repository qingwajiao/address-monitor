package handler

import (
	"net/http"
	"strconv"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/api/middleware"
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
	appID := middleware.GetAppID(c)
	if err := h.svc.SetWebhookURL(c.Request.Context(), appID, &req); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, nil)
}

func (h *WebhookHandler) GetWebhookURL(c *gin.Context) {
	appID := middleware.GetAppID(c)
	result, err := h.svc.GetWebhookURL(c.Request.Context(), appID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, result)
}

func (h *WebhookHandler) ListLogs(c *gin.Context) {
	var req dto.ListWebhookLogReq
	c.ShouldBindQuery(&req)
	appID := middleware.GetAppID(c)
	result, err := h.svc.ListLogs(c.Request.Context(), appID, &req)
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
	appID := middleware.GetAppID(c)
	if err := h.svc.Resend(c.Request.Context(), appID, id); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, nil)
}
