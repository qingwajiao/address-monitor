package handler

import (
	"net/http"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/api/middleware"
	"address-monitor/internal/api/service"

	"github.com/gin-gonic/gin"
)

type AddressHandler struct {
	svc *service.AddressService
}

func NewAddressHandler(svc *service.AddressService) *AddressHandler {
	return &AddressHandler{svc: svc}
}

func (h *AddressHandler) Create(c *gin.Context) {
	var req dto.CreateAddressReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	appID := middleware.GetAppID(c)
	result, err := h.svc.Create(c.Request.Context(), appID, &req)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	Created(c, result)
}

func (h *AddressHandler) BatchCreate(c *gin.Context) {
	var req dto.BatchCreateAddressReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	appID := middleware.GetAppID(c)
	result, err := h.svc.BatchCreate(c.Request.Context(), appID, &req)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	Success(c, result)
}

func (h *AddressHandler) List(c *gin.Context) {
	var req dto.ListAddressReq
	_ = c.ShouldBindQuery(&req)
	appID := middleware.GetAppID(c)
	result, err := h.svc.List(c.Request.Context(), appID, &req)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, result)
}

func (h *AddressHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	appID := middleware.GetAppID(c)
	result, err := h.svc.GetByID(c.Request.Context(), appID, id)
	if err != nil {
		Fail(c, http.StatusNotFound, err.Error())
		return
	}
	Success(c, result)
}

func (h *AddressHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	appID := middleware.GetAppID(c)
	if err := h.svc.Delete(c.Request.Context(), appID, id); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, nil)
}
