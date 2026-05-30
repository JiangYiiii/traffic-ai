package config

import "testing"

func TestNormalizePathPrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"traffic", "/traffic"},
		{"/traffic/", "/traffic"},
		{" /traffic/ ", "/traffic"},
	}
	for _, tt := range tests {
		if got := NormalizePathPrefix(tt.in); got != tt.want {
			t.Errorf("NormalizePathPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestServerConfigOAuthCallbackPath(t *testing.T) {
	s := ServerConfig{ControlPathPrefix: "/console"}
	if got := s.OAuthCallbackPath(); got != "/console/admin/oauth/callback" {
		t.Fatalf("OAuthCallbackPath() = %q", got)
	}
}
