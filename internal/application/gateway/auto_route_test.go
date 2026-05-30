package gateway

import (
	"encoding/json"
	"testing"
)

func TestRewriteRequestModelReplacesOnlyModelField(t *testing.T) {
	body := []byte(`{"model":"auto","stream":true,"messages":[{"role":"user","content":"use auto literally"}]}`)
	got, err := rewriteRequestModel(body, "gpt-mini")
	if err != nil {
		t.Fatalf("rewriteRequestModel: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("rewritten JSON invalid: %v", err)
	}
	if out["model"] != "gpt-mini" {
		t.Fatalf("model = %v, want gpt-mini", out["model"])
	}
	messages := out["messages"].([]any)
	content := messages[0].(map[string]any)["content"]
	if content != "use auto literally" {
		t.Fatalf("message content changed: %v", content)
	}
}
