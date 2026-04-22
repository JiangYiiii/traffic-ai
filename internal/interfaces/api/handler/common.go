// Package handler 控制台 API 层的跨处理器公共辅助。
package handler

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/trailyai/traffic-ai/pkg/errcode"
	"github.com/trailyai/traffic-ai/pkg/response"
)

// handleError 把处理器层收到的 error 映射为统一的失败 JSON。
//
// 约定：
//   - 业务错误使用 *errcode.AppError 承载，直接透传（含 http status 与 message）；
//   - 其他任意错误（数据库、内部 panic recover、未显式封装等）统一兜底为 ErrInternal，
//     避免把下游实现细节泄漏给前端。
//
// 该函数原定义在 model_handler.go 中，历史上被 model/monitor/provider_account/oauth
// 四个处理器跨文件引用，为避免隐式依赖，统一提升到本公共文件。
func handleError(c *gin.Context, err error) {
	if ae, ok := err.(*errcode.AppError); ok {
		response.Fail(c, ae)
		return
	}
	response.Fail(c, errcode.ErrInternal)
}

// parseOptionalTimeQuery 解析可选的时间查询参数。
//
// 返回值：
//   - 第一个返回值：解析得到的时间指针；缺省或空字符串时返回 nil。
//   - 第二个返回值：false 表示格式非法，调用方应直接 return（本函数已写入 400 响应）。
//
// 接受格式：RFC3339、RFC3339Nano、以及 "2006-01-02T15:04"（HTML5 datetime-local
// 默认格式，按本地时区解析）。
func parseOptionalTimeQuery(c *gin.Context, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, true
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return &t, true
		}
	}
	response.Fail(c, errcode.ErrBadRequest)
	return nil, false
}
