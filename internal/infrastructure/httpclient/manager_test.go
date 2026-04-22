package httpclient

import (
	"testing"
	"time"
)

func TestManager_ForReusesClient(t *testing.T) {
	m := NewManager(Config{})
	c1 := m.For(42, 30)
	c2 := m.For(42, 30)
	if c1 != c2 {
		t.Fatalf("expected same *http.Client for same accountID, got different pointers")
	}
}

func TestManager_ForDifferentAccounts(t *testing.T) {
	m := NewManager(Config{})
	c1 := m.For(1, 30)
	c2 := m.For(2, 30)
	if c1 == c2 {
		t.Fatalf("expected different *http.Client for different accountIDs, got the same pointer")
	}
}

func TestManager_DefaultTimeout(t *testing.T) {
	m := NewManager(Config{})
	c := m.For(100, 0)
	if c.Timeout != 120*time.Second {
		t.Fatalf("expected default Timeout=120s when timeoutSec<=0, got %s", c.Timeout)
	}
}
