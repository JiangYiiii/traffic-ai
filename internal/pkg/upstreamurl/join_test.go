package upstreamurl

import "testing"

func TestJoinPath(t *testing.T) {
	cases := []struct {
		endpoint string
		suffix   string
		want     string
	}{
		{
			"https://example.com/azure/v1/openai/deployments/m1",
			"/chat/completions",
			"https://example.com/azure/v1/openai/deployments/m1/chat/completions",
		},
		{
			"https://example.com/azure/v1/openai/deployments/m1?api-version=2025-03-01-preview",
			"/chat/completions",
			"https://example.com/azure/v1/openai/deployments/m1/chat/completions?api-version=2025-03-01-preview",
		},
		{
			"https://example.com/base/",
			"chat/completions",
			"https://example.com/base/chat/completions",
		},
		{
			"https://example.com/q?x=1",
			"/embeddings",
			"https://example.com/q/embeddings?x=1",
		},
	}
	for _, tc := range cases {
		if got := JoinPath(tc.endpoint, tc.suffix); got != tc.want {
			t.Fatalf("JoinPath(%q,%q)=%q want %q", tc.endpoint, tc.suffix, got, tc.want)
		}
	}
}
