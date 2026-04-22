// Package gateway 可观测性基建：Prometheus 指标定义与独立 registry。
// @ai_doc_flow 指标出口: /metrics → Metrics.Handler()
// 本文件仅负责指标**定义与暴露**，不做任何业务埋点（留给 Phase 1/2/3）。
package gateway

import (
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 聚合数据面网关所有 Prometheus collector，使用独立 registry 避免污染全局。
type Metrics struct {
	registry *prometheus.Registry

	// 请求总量（Phase 1 在 settleAndLog 里 Inc）
	RequestsTotal *prometheus.CounterVec
	// 上游耗时（Phase 1 Observe）
	UpstreamLatencySeconds *prometheus.HistogramVec
	// 并发中请求数（Phase 1 Inc/Dec）
	Inflight *prometheus.GaugeVec
	// 熔断状态（Phase 2：0=closed 1=half_open 2=open）
	CircuitState *prometheus.GaugeVec
	// 限流拒绝计数（Phase 3）
	RatelimitRejectTotal *prometheus.CounterVec
	// 重试计数（Phase 2）
	RetryTotal *prometheus.CounterVec
	// 构建信息（本卡唯一会被 set 的指标）
	BuildInfo *prometheus.GaugeVec
}

// NewMetrics 创建独立 registry 并注册所有 collector。
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,

		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "traffic_gateway_requests_total",
			Help: "Total number of gateway requests, labeled by model/account/status/protocol.",
		}, []string{"model", "account", "status", "protocol"}),

		UpstreamLatencySeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "traffic_gateway_upstream_latency_seconds",
			Help:    "Upstream latency in seconds for gateway forwarded requests.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 12),
		}, []string{"model", "account"}),

		Inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "traffic_gateway_inflight",
			Help: "In-flight gateway requests, labeled by model/account.",
		}, []string{"model", "account"}),

		CircuitState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "traffic_gateway_circuit_state",
			Help: "Circuit breaker state per account: 0=closed, 1=half_open, 2=open.",
		}, []string{"account"}),

		RatelimitRejectTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "traffic_gateway_ratelimit_reject_total",
			Help: "Rate limit rejections, labeled by scope(global/user/api_key/model/account) and reason(rpm/tpm/concurrent).",
		}, []string{"scope", "reason"}),

		RetryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "traffic_gateway_retry_total",
			Help: "Upstream retry attempts, labeled by model and reason(timeout/upstream_5xx/dial/tls).",
		}, []string{"model", "reason"}),

		BuildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "traffic_gateway_build_info",
			Help: "Gateway build information, value is always 1.",
		}, []string{"version"}),
	}

	reg.MustRegister(
		m.RequestsTotal,
		m.UpstreamLatencySeconds,
		m.Inflight,
		m.CircuitState,
		m.RatelimitRejectTotal,
		m.RetryTotal,
		m.BuildInfo,
	)

	version := os.Getenv("TRAFFIC_VERSION")
	if version == "" {
		version = "dev"
	}
	m.BuildInfo.WithLabelValues(version).Set(1)

	return m
}

// Handler 返回 /metrics 端点处理器，使用本 registry 而非全局。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry: m.registry,
	})
}

// --- MetricsSink 适配层 ---
// 以下方法让 *Metrics 实现 application/gateway.MetricsSink 接口，
// 使 application 层不反向依赖 interfaces 层，也便于测试替换。
// 所有方法对 nil 接收者安全（Metrics 字段均由 NewMetrics 构造后不会为 nil）。

// IncRequest 在 settleAndLog 最终落地时调用：累计一次请求（含成功与失败）。
func (m *Metrics) IncRequest(model, account, status, protocol string) {
	if m == nil {
		return
	}
	m.RequestsTotal.WithLabelValues(model, account, status, protocol).Inc()
}

// ObserveUpstreamLatency 记录上游耗时（秒）。仅用于成功/错误的最终统计，不包含重试过程。
func (m *Metrics) ObserveUpstreamLatency(model, account string, seconds float64) {
	if m == nil {
		return
	}
	m.UpstreamLatencySeconds.WithLabelValues(model, account).Observe(seconds)
}

// IncInflight 请求进入上游转发循环前 +1，配合 DecInflight 使用。
func (m *Metrics) IncInflight(model, account string) {
	if m == nil {
		return
	}
	m.Inflight.WithLabelValues(model, account).Inc()
}

// DecInflight 请求退出上游循环（无论成功/失败）时 -1。
func (m *Metrics) DecInflight(model, account string) {
	if m == nil {
		return
	}
	m.Inflight.WithLabelValues(model, account).Dec()
}

// IncRatelimitReject 限流器拒绝一次请求时调用；scope ∈ {global/user/api_key/model}，reason ∈ {rpm/tpm/concurrent}。
func (m *Metrics) IncRatelimitReject(scope, reason string) {
	if m == nil {
		return
	}
	m.RatelimitRejectTotal.WithLabelValues(scope, reason).Inc()
}

// IncRetry fallback 循环决定继续重试前累计一次；reason 来自 UpstreamErrorKind。
// 最终失败（exhausted / !Retryable）不在此计数，避免与 requests_total{status="error"} 重复归因。
func (m *Metrics) IncRetry(model, reason string) {
	if m == nil {
		return
	}
	m.RetryTotal.WithLabelValues(model, reason).Inc()
}
