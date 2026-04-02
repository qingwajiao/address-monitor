package handler

import (
	"net/http"
	"strconv"

	"address-monitor/internal/api/dto"
	"address-monitor/internal/api/service"

	"github.com/gin-gonic/gin"
)

type ContractHandler struct {
	svc *service.ContractService
}

func NewContractHandler(svc *service.ContractService) *ContractHandler {
	return &ContractHandler{svc: svc}
}

func (h *ContractHandler) Create(c *gin.Context) {
	var req dto.CreateContractReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Created(c, resp)
}

func (h *ContractHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.Request.Context())
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, list)
}

func (h *ContractHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, "无效的 id")
		return
	}
	var req dto.UpdateContractReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, &req); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, nil)
}

func (h *ContractHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, "无效的 id")
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	Success(c, nil)
}
