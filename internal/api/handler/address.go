package handler

import (
	"net/http"
	"strconv"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/api/service"

	"github.com/gin-gonic/gin"
)

type AddressHandler struct {
	svc *service.SubscriptionService
}

func NewAddressHandler(svc *service.SubscriptionService) *AddressHandler {
	return &AddressHandler{svc: svc}
}

func (h *AddressHandler) Create(c *gin.Context) {
	var req dto.CreateSubReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := c.GetHeader("X-API-Key")
	result, err := h.svc.Create(c.Request.Context(), userID, &req)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	Created(c, result)
}

func (h *AddressHandler) BatchCreate(c *gin.Context) {
	var req dto.BatchCreateSubReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := c.GetHeader("X-API-Key")
	result, err := h.svc.BatchCreate(c.Request.Context(), userID, &req)
	if err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	Success(c, result)
}

func (h *AddressHandler) List(c *gin.Context) {
	var req dto.ListSubReq
	c.ShouldBindQuery(&req)

	userID := c.GetHeader("X-API-Key")
	result, err := h.svc.List(c.Request.Context(), userID, &req)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, result)
}

func (h *AddressHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		Fail(c, http.StatusNotFound, err.Error())
		return
	}

	Success(c, result)
}

func (h *AddressHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	userID := c.GetHeader("X-API-Key")
	if err := h.svc.Delete(c.Request.Context(), userID, id); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	Success(c, nil)
}
