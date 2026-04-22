package gateway

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestClassifyStatusCode(t *testing.T) {
	cases := []struct {
		status int
		want   UpstreamErrorKind
	}{
		{200, UpstreamKindNone},
		{201, UpstreamKindNone},
		{299, UpstreamKindNone},
		{429, UpstreamKindUpstream429},
		{500, UpstreamKindUpstream5xx},
		{502, UpstreamKindUpstream5xx},
		{599, UpstreamKindUpstream5xx},
		{400, UpstreamKindUpstream4xx},
		{401, UpstreamKindUpstream4xx},
		{404, UpstreamKindUpstream4xx},
		{418, UpstreamKindUpstream4xx},
		{100, UpstreamKindUnknown},
		{301, UpstreamKindUnknown},
	}
	for _, c := range cases {
		got := classifyStatusCode(c.status)
		if got != c.want {
			t.Errorf("classifyStatusCode(%d) = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestRetryableMatrix(t *testing.T) {
	cases := []struct {
		kind         UpstreamErrorKind
		retryable    bool
		countsCircle bool
	}{
		{UpstreamKindNone, false, false},
		{UpstreamKindDial, true, true},
		{UpstreamKindTLS, true, true},
		{UpstreamKindTimeout, true, true},
		{UpstreamKindResponseHdr, true, true},
		{UpstreamKindUpstream5xx, true, true},
		{UpstreamKindUpstream429, true, true},
		{UpstreamKindUpstream4xx, false, false},
		{UpstreamKindClientCancel, false, false},
		{UpstreamKindStreamIdle, false, false},
		{UpstreamKindUnknown, true, false},
	}
	for _, c := range cases {
		if got := c.kind.Retryable(); got != c.retryable {
			t.Errorf("%q.Retryable() = %v, want %v", c.kind, got, c.retryable)
		}
		if got := c.kind.CountsTowardsCircuit(); got != c.countsCircle {
			t.Errorf("%q.CountsTowardsCircuit() = %v, want %v", c.kind, got, c.countsCircle)
		}
	}
}

func TestClassifyTransportError_Nil(t *testing.T) {
	if got := classifyTransportError(context.Background(), nil); got != UpstreamKindNone {
		t.Errorf("nil err got %q, want None", got)
	}
}

func TestClassifyTransportError_ClientCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := errors.New("some err")
	if got := classifyTransportError(ctx, err); got != UpstreamKindClientCancel {
		t.Errorf("canceled ctx got %q, want ClientCancel", got)
	}
}

func TestClassifyTransportError_DeadlineExceeded(t *testing.T) {
	got := classifyTransportError(context.Background(), context.DeadlineExceeded)
	if got != UpstreamKindTimeout {
		t.Errorf("DeadlineExceeded got %q, want Timeout", got)
	}
}

func TestClassifyTransportError_DialOpError(t *testing.T) {
	err := &net.OpError{Op: "dial", Err: errors.New("connect: connection refused")}
	if got := classifyTransportError(context.Background(), err); got != UpstreamKindDial {
		t.Errorf("dial OpError got %q, want Dial", got)
	}
}

func TestClassifyTransportError_TLS(t *testing.T) {
	err := errors.New("tls: handshake failure")
	if got := classifyTransportError(context.Background(), err); got != UpstreamKindTLS {
		t.Errorf("tls err got %q, want TLS", got)
	}
}

func TestClassifyTransportError_ResponseHeaderTimeout(t *testing.T) {
	err := errors.New("net/http: timeout awaiting response headers")
	// "timeout" 字样先命中 Timeout 兜底；但"response header"在前面判断，所以优先。
	// 我们要求 "response header" 优先于 "timeout"：
	if got := classifyTransportError(context.Background(), err); got != UpstreamKindResponseHdr {
		t.Errorf("response header timeout got %q, want ResponseHdr", got)
	}
}

func TestClassifyTransportError_TimeoutString(t *testing.T) {
	err := errors.New("Client.Timeout exceeded while awaiting")
	if got := classifyTransportError(context.Background(), err); got != UpstreamKindTimeout {
		t.Errorf("client.timeout got %q, want Timeout", got)
	}
}

func TestClassifyTransportError_Unknown(t *testing.T) {
	err := errors.New("something random")
	if got := classifyTransportError(context.Background(), err); got != UpstreamKindUnknown {
		t.Errorf("unknown got %q, want Unknown", got)
	}
}
