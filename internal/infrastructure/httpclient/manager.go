// Package httpclient 为数据面网关上游转发提供按账号缓存的 *http.Client。
//
// 默认 http.DefaultTransport 的 MaxIdleConnsPerHost=2 是单账号吞吐瓶颈，
// 同时它只暴露整次 Timeout 没有分项超时，上游 hang 就硬等到整次超时。
// 本 Manager：
//  1. 按 account_id 缓存一个 *http.Client + *http.Transport，实现每账号独立连接池；
//  2. 支持 Dial/TLS/ResponseHeader 分项超时，让"连不上"和"不回响应头"比"整次 120s"更早失败；
//  3. StreamIdleTimeout 提供给 handleStream 做帧间空闲兜底，避免流式请求死挂。
//
// 本卡只做连接池与超时配置，不加 Prometheus 打点（留给后续卡）。
package httpclient

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Config 连接池与超时参数。字段单位为 time.Duration，上层把配置里的 Sec 换算后再传进来。
// 任一字段为 0 时，NewManager 会用内部默认值兜底。
type Config struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	StreamIdleTimeout     time.Duration
}

const (
	defaultMaxIdleConns          = 256
	defaultMaxIdleConnsPerHost   = 64
	defaultMaxConnsPerHost       = 128
	defaultIdleConnTimeout       = 90 * time.Second
	defaultDialTimeout           = 5 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultResponseHeaderTimeout = 30 * time.Second
	defaultStreamIdleTimeout     = 60 * time.Second
	defaultOverallTimeout        = 120 * time.Second
)

// accountClient 保存账号级的 Client 和其底层 Transport。
// 之所以同时保存 transport，是为了后续 metrics / 优雅停机可以直接访问连接池。
type accountClient struct {
	client    *http.Client
	transport *http.Transport
}

// Manager 按 accountID 缓存 accountClient；并发场景下使用 sync.Map 避免写锁。
type Manager struct {
	cfg   Config
	cache sync.Map // key: int64 account_id, value: *accountClient
}

// NewManager 构造一个 Manager；cfg 中任一字段为 0 时用内部默认值。
func NewManager(cfg Config) *Manager {
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = defaultMaxIdleConns
	}
	if cfg.MaxIdleConnsPerHost <= 0 {
		cfg.MaxIdleConnsPerHost = defaultMaxIdleConnsPerHost
	}
	if cfg.MaxConnsPerHost <= 0 {
		cfg.MaxConnsPerHost = defaultMaxConnsPerHost
	}
	if cfg.IdleConnTimeout <= 0 {
		cfg.IdleConnTimeout = defaultIdleConnTimeout
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = defaultDialTimeout
	}
	if cfg.TLSHandshakeTimeout <= 0 {
		cfg.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	}
	if cfg.ResponseHeaderTimeout <= 0 {
		cfg.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	}
	if cfg.StreamIdleTimeout <= 0 {
		cfg.StreamIdleTimeout = defaultStreamIdleTimeout
	}
	return &Manager{cfg: cfg}
}

// For 返回 accountID 对应的 *http.Client；
// timeoutSec<=0 时用 120s 作为整次兜底 Timeout。
// 同一 accountID 多次调用返回同一实例（即使 timeoutSec 不同，以首次为准）。
func (m *Manager) For(accountID int64, timeoutSec int) *http.Client {
	if v, ok := m.cache.Load(accountID); ok {
		return v.(*accountClient).client
	}

	tOverall := time.Duration(timeoutSec) * time.Second
	if tOverall <= 0 {
		tOverall = defaultOverallTimeout
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   m.cfg.DialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          m.cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   m.cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       m.cfg.MaxConnsPerHost,
		IdleConnTimeout:       m.cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   m.cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: m.cfg.ResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	ac := &accountClient{
		transport: transport,
		client: &http.Client{
			Transport: transport,
			Timeout:   tOverall,
		},
	}

	// LoadOrStore 防止并发首次构造重复创建 transport（虽然多构造一次无害，但能少浪费一次分配）。
	actual, loaded := m.cache.LoadOrStore(accountID, ac)
	if loaded {
		// 被其他 goroutine 抢先写入：丢弃本次构造的 transport。
		transport.CloseIdleConnections()
		return actual.(*accountClient).client
	}
	return ac.client
}

// StreamIdleTimeout 返回流式帧间空闲超时，供 handleStream 使用。
func (m *Manager) StreamIdleTimeout() time.Duration {
	return m.cfg.StreamIdleTimeout
}

// Close 遍历所有缓存 transport 释放空闲连接，便于优雅停机。
// 本卡 main.go 暂不调用，只提供接口。
func (m *Manager) Close() {
	m.cache.Range(func(_, v any) bool {
		if ac, ok := v.(*accountClient); ok && ac.transport != nil {
			ac.transport.CloseIdleConnections()
		}
		return true
	})
}
