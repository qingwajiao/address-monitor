package handler

import (
	"errors"
	"net/http"
	"strconv"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/api/middleware"
	"address-monitor/internal/api/service"

	"github.com/gin-gonic/gin"
)

type AppHandler struct {
	svc *service.AppService
}

func NewAppHandler(svc *service.AppService) *AppHandler {
	return &AppHandler{svc: svc}
}

func (h *AppHandler) Create(c *gin.Context) {
	var req dto.CreateAppReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	result, err := h.svc.Create(c.Request.Context(), userID, &req)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Created(c, result)
}

func (h *AppHandler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	result, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, result)
}

func (h *AppHandler) Get(c *gin.Context) {
	appID, err := parseID(c)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	userID := middleware.GetUserID(c)
	result, err := h.svc.Get(c.Request.Context(), userID, appID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrAppNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrAppForbidden) {
			status = http.StatusForbidden
		}
		Fail(c, status, err.Error())
		return
	}
	Success(c, result)
}

func (h *AppHandler) Update(c *gin.Context) {
	appID, err := parseID(c)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req dto.UpdateAppReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	result, err := h.svc.Update(c.Request.Context(), userID, appID, &req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrAppNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrAppForbidden) {
			status = http.StatusForbidden
		}
		Fail(c, status, err.Error())
		return
	}
	Success(c, result)
}

func (h *AppHandler) Delete(c *gin.Context) {
	appID, err := parseID(c)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	userID := middleware.GetUserID(c)
	if err := h.svc.Delete(c.Request.Context(), userID, appID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrAppNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, service.ErrAppForbidden) {
			status = http.StatusForbidden
		}
		Fail(c, status, err.Error())
		return
	}
	Success(c, nil)
}

func (h *AppHandler) ResetAPIKey(c *gin.Context) {
	appID, err := parseID(c)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	userID := middleware.GetUserID(c)
	result, err := h.svc.ResetAPIKey(c.Request.Context(), userID, appID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, result)
}

func (h *AppHandler) ResetSecret(c *gin.Context) {
	appID, err := parseID(c)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	userID := middleware.GetUserID(c)
	result, err := h.svc.ResetSecret(c.Request.Context(), userID, appID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, result)
}

func parseID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}
