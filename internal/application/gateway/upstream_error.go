// Package gateway —— 上游错误分类。
//
// 把 client.Do 返回的 error、resp 非 2xx 的状态码、ctx cancel/deadline
// 归一成有限枚举，供 fallback 决策（是否换账号重试）和熔断决策
// （是否计入失败率）使用。
//
// 分类策略刻意保守：
//   - 明确可重试的错误（Dial/TLS/Timeout/5xx/429）才放进可重试集合；
//   - 明确属于客户端侧的错误（ClientCancel/4xx）不计入熔断，避免误封账号；
//   - 未识别错误统一归为 Unknown，允许重试但不计入熔断，防止串联误判。
package gateway

import (
	"context"
	"errors"
	"net"
	"strings"
)

// UpstreamErrorKind 上游错误分类。空串代表成功（无错误）。
type UpstreamErrorKind string

const (
	UpstreamKindNone         UpstreamErrorKind = ""                // 成功（2xx）
	UpstreamKindDial         UpstreamErrorKind = "dial"            // 连接建立失败（含 DNS）
	UpstreamKindTLS          UpstreamErrorKind = "tls"             // TLS 握手失败
	UpstreamKindTimeout      UpstreamErrorKind = "timeout"         // ctx deadline / client Timeout
	UpstreamKindResponseHdr  UpstreamErrorKind = "response_header" // ResponseHeaderTimeout
	UpstreamKindUpstream5xx  UpstreamErrorKind = "upstream_5xx"    // resp 5xx
	UpstreamKindUpstream429  UpstreamErrorKind = "upstream_429"    // resp 429
	UpstreamKindUpstream4xx  UpstreamErrorKind = "upstream_4xx"    // resp 其他 4xx（不计熔断）
	UpstreamKindClientCancel UpstreamErrorKind = "client_cancel"   // 客户端主动取消
	UpstreamKindStreamIdle   UpstreamErrorKind = "stream_idle"     // 流式空闲超时
	UpstreamKindUnknown      UpstreamErrorKind = "unknown"         // 兜底
)

// classifyTransportError 把 client.Do 返回的 error 归类。
// ctx 用来区分客户端主动 Cancel vs 网关侧 DeadlineExceeded。
func classifyTransportError(ctx context.Context, err error) UpstreamErrorKind {
	if err == nil {
		return UpstreamKindNone
	}

	// 1. ctx 被客户端主动取消
	if ctx != nil && ctx.Err() == context.Canceled {
		return UpstreamKindClientCancel
	}

	// 2. ctx/err 中出现 DeadlineExceeded，统一视为 Timeout
	if errors.Is(err, context.DeadlineExceeded) ||
		(ctx != nil && ctx.Err() == context.DeadlineExceeded) {
		return UpstreamKindTimeout
	}

	// 3. net.OpError：区分 dial / tls
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return UpstreamKindDial
		}
		// TLS 握手失败在 net.OpError.Err.Error() 里带 "tls:" 字样
		if opErr.Err != nil && strings.Contains(opErr.Err.Error(), "tls:") {
			return UpstreamKindTLS
		}
	}

	// 4. 字符串兜底：Go 标准库的 timeout / tls 错误类型多样，
	//    先用字符串匹配覆盖常见情况；TODO：后续可迁移到强类型断言。
	msg := err.Error()
	switch {
	case strings.Contains(msg, "tls:"):
		return UpstreamKindTLS
	case strings.Contains(msg, "response header"):
		return UpstreamKindResponseHdr
	case strings.Contains(msg, "Client.Timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "timeout"):
		return UpstreamKindTimeout
	}

	return UpstreamKindUnknown
}

// classifyStatusCode 把上游 resp.StatusCode 归类。
// 2xx → None（成功）；其余映射到对应错误类型。
func classifyStatusCode(status int) UpstreamErrorKind {
	switch {
	case status >= 200 && status < 300:
		return UpstreamKindNone
	case status == 429:
		return UpstreamKindUpstream429
	case status >= 500 && status < 600:
		return UpstreamKindUpstream5xx
	case status >= 400 && status < 500:
		return UpstreamKindUpstream4xx
	default:
		return UpstreamKindUnknown
	}
}

// Retryable 是否允许换账号重试。
//
// 可重试：Dial / TLS / Timeout / ResponseHdr / Upstream5xx / Upstream429 / Unknown。
// 不可重试：
//   - None       已经成功没必要重试；
//   - ClientCancel 客户端都不要结果了，重试没意义；
//   - Upstream4xx  请求体/鉴权问题换账号也救不了；
//   - StreamIdle   已经在流式中，body 已被 flush，重试会二次写响应。
func (k UpstreamErrorKind) Retryable() bool {
	switch k {
	case UpstreamKindDial,
		UpstreamKindTLS,
		UpstreamKindTimeout,
		UpstreamKindResponseHdr,
		UpstreamKindUpstream5xx,
		UpstreamKindUpstream429,
		UpstreamKindUnknown:
		return true
	default:
		return false
	}
}

// CountsTowardsCircuit 是否计入账号熔断失败率。
//
// 计入：Dial / TLS / Timeout / ResponseHdr / Upstream5xx / Upstream429
//   —— 这些是"账号/上游通道"的问题，累积到阈值即隔离。
//
// 不计入：
//   - Upstream4xx  客户端自己请求不合法；
//   - ClientCancel 客户端主动断开；
//   - StreamIdle   流式空闲，多半是用户侧网络问题（待卡 #5 再细分）；
//   - None         成功不算失败；
//   - Unknown      保守不计入，避免因未识别错误误封账号。
func (k UpstreamErrorKind) CountsTowardsCircuit() bool {
	switch k {
	case UpstreamKindDial,
		UpstreamKindTLS,
		UpstreamKindTimeout,
		UpstreamKindResponseHdr,
		UpstreamKindUpstream5xx,
		UpstreamKindUpstream429:
		return true
	default:
		return false
	}
}
