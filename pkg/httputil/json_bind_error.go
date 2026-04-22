package httputil

import (
	"encoding/json"
	"errors"
	"strings"
)

// FriendlyJSONBindError 将 Gin/encoding/json 绑定错误转为用户可理解的简短说明（中文）。
func FriendlyJSONBindError(err error) string {
	if err == nil {
		return ""
	}
	var synErr *json.SyntaxError
	if errors.As(err, &synErr) {
		return "JSON 格式不正确，请检查括号、引号与逗号。"
	}
	s := err.Error()
	if strings.Contains(s, "cannot unmarshal") {
		if strings.Contains(s, "int64") || strings.Contains(s, " into Go struct field") && strings.Contains(s, "int") {
			return "数值字段须为整数：价格等使用「微美元」（整型），不能使用小数。例如 1 美元请填 1000000。"
		}
		return "字段类型与接口要求不符，请对照文档检查 JSON 类型。"
	}
	if strings.Contains(s, "unexpected end of JSON input") {
		return "请求体为空或 JSON 不完整。"
	}
	return s
}
