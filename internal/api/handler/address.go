package handler

import (
	"net/http"
	"strconv"
	"strings"

	"address-monitor/internal/matcher"
	"address-monitor/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AddressHandler struct {
	subStore *store.SubscriptionStore
	matcher  *matcher.Matcher
	rdb      *redis.Client
}

func NewAddressHandler(
	subStore *store.SubscriptionStore,
	matcher *matcher.Matcher,
	rdb *redis.Client,
) *AddressHandler {
	return &AddressHandler{
		subStore: subStore,
		matcher:  matcher,
		rdb:      rdb,
	}
}

type createSubRequest struct {
	Chain       string `json:"chain" binding:"required"`
	Address     string `json:"address" binding:"required"`
	CallbackURL string `json:"callback_url" binding:"required,url"`
	Label       string `json:"label"`
}

func (h *AddressHandler) Create(c *gin.Context) {
	var req createSubRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	// 地址统一小写，链名称统一大写
	req.Address = strings.ToLower(req.Address)
	req.Chain = strings.ToUpper(req.Chain)

	// 从 context 取 user_id（由 auth 中间件注入）
	userID := c.GetString("user_id")
	if userID == "" {
		userID = c.GetHeader("X-API-Key")
	}

	sub := &store.Subscription{
		UserID:      userID,
		Chain:       req.Chain,
		Address:     req.Address,
		Label:       req.Label,
		CallbackURL: req.CallbackURL,
		Secret:      uuid.New().String(), // 自动生成签名密钥
		Status:      1,
	}

	if err := h.matcher.Add(c.Request.Context(), sub); err != nil {
		//c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 新增地址同时加入热集合
	h.matcher.AddToHotSet(c.Request.Context(), req.Chain, req.Address)

	zap.L().Info("新增监控地址",
		zap.String("chain", sub.Chain),
		zap.String("address", sub.Address),
		zap.String("user_id", userID),
		zap.Uint64("sub_id", sub.ID),
		zap.String("callback_url", sub.CallbackURL),
	)

	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"id":           sub.ID,
			"chain":        sub.Chain,
			"address":      sub.Address,
			"label":        sub.Label,
			"callback_url": sub.CallbackURL,
			"secret":       sub.Secret, // 仅此一次展示
			"status":       sub.Status,
			"created_at":   sub.CreatedAt,
		},
	})
}

func (h *AddressHandler) List(c *gin.Context) {
	userID := c.GetHeader("X-API-Key")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	subs, total, err := h.subStore.ListByUser(c.Request.Context(), userID, page, size)
	if err != nil {
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": subs,
		"meta": gin.H{
			"total": total,
			"page":  page,
			"size":  size,
		},
	})
}

func (h *AddressHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		//c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	sub, err := h.subStore.GetByID(c.Request.Context(), id)
	if err != nil {
		//c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		Fail(c, http.StatusNotFound, "subscription not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sub})
	Success(c, gin.H{"data": sub})
}

func (h *AddressHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		//c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	// 查询订阅信息（用于清理缓存）
	sub, err := h.subStore.GetByID(c.Request.Context(), id)
	if err != nil {
		//c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		Fail(c, http.StatusNotFound, "subscription not found")
		return
	}

	if err := h.matcher.Remove(c.Request.Context(), id, sub.Chain, sub.Address); err != nil {
		//c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	zap.L().Info("删除监控地址",
		zap.Uint64("sub_id", id),
		zap.String("chain", sub.Chain),
		zap.String("address", sub.Address),
	)

	//c.JSON(http.StatusOK, gin.H{"message": "deleted"})
	Success(c, gin.H{"message": "deleted"})
}
