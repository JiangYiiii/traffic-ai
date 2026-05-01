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

func TestAppendRawQuery(t *testing.T) {
	base := JoinPath("https://x/openai/deployments/m1?api-version=2", "/files")
	want := "https://x/openai/deployments/m1/files?api-version=2&limit=5"
	if got := AppendRawQuery(base, "limit=5"); got != want {
		t.Fatalf("AppendRawQuery(%q,%q)=%q want %q", base, "limit=5", got, want)
	}
	if got := AppendRawQuery("https://api.openai.com/v1/files", "purpose=vision"); got != "https://api.openai.com/v1/files?purpose=vision" {
		t.Fatalf("AppendRawQuery plain: %q", got)
	}
	if got := AppendRawQuery(base, ""); got != base {
		t.Fatalf("empty query should noop: %q", got)
	}
}
