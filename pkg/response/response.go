// Package response 统一 HTTP JSON 响应格式，所有控制面 API 共用。
package response

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/trailyai/traffic-ai/pkg/errcode"
)

type Body struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data})
}

func OKMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: msg})
}

func Fail(c *gin.Context, err *errcode.AppError) {
	lang := detectLang(c)
	c.JSON(err.HTTPStatus, Body{Code: err.Code, Message: err.Localized(lang)})
}

func detectLang(c *gin.Context) string {
	al := c.GetHeader("Accept-Language")
	if strings.Contains(al, "zh") {
		return "zh"
	}
	return "en"
}

func FailMsg(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, Body{Code: code, Message: msg})
}

func PageResult(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, Body{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"list":      list,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}
