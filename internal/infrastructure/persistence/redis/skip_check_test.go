package redis

import "testing"

func TestSkipCheck(t *testing.T) {
	t.Setenv("TRAFFIC_REDIS_SKIP_CHECK", "")
	if SkipCheck() {
		t.Fatal("expected false for empty env")
	}
	t.Setenv("TRAFFIC_REDIS_SKIP_CHECK", "1")
	if !SkipCheck() {
		t.Fatal("expected true for TRAFFIC_REDIS_SKIP_CHECK=1")
	}
}
