package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 响应码定义
const (
	CodeSuccess = 1
	CodeError   = 0
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code: CodeSuccess,
		Msg:  "success",
		Data: data,
	})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code: CodeSuccess,
		Msg:  "success",
		Data: data,
	})
}

func Fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, Response{
		Code: CodeError,
		Msg:  msg,
		Data: nil,
	})
}
