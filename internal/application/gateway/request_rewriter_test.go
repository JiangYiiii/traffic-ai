package gateway

import (
	"encoding/json"
	"reflect"
	"testing"
)

// 辅助函数：把两段 JSON 解析成 map 再做深度比较，避免字段顺序影响断言。
func jsonEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var gotObj, wantObj any
	if err := json.Unmarshal(got, &gotObj); err != nil {
		t.Fatalf("parse got: %v; raw=%s", err, string(got))
	}
	if err := json.Unmarshal(want, &wantObj); err != nil {
		t.Fatalf("parse want: %v; raw=%s", err, string(want))
	}
	if !reflect.DeepEqual(gotObj, wantObj) {
		t.Fatalf("json mismatch\n got:  %s\n want: %s", string(got), string(want))
	}
}

func TestInjectIncludeUsage_AddsOptionWhenMissing(t *testing.T) {
	in := []byte(`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	out := injectIncludeUsageForOpenAIStream(in)
	want := []byte(`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hi"}],"stream_options":{"include_usage":true}}`)
	jsonEqual(t, out, want)
}

func TestInjectIncludeUsage_MergesIntoExistingStreamOptions(t *testing.T) {
	in := []byte(`{"model":"gpt-4o-mini","stream":true,"stream_options":{"chunk_size":64}}`)
	out := injectIncludeUsageForOpenAIStream(in)
	want := []byte(`{"model":"gpt-4o-mini","stream":true,"stream_options":{"chunk_size":64,"include_usage":true}}`)
	jsonEqual(t, out, want)
}

func TestInjectIncludeUsage_RespectsExplicitTrue(t *testing.T) {
	in := []byte(`{"model":"gpt-4o-mini","stream":true,"stream_options":{"include_usage":true}}`)
	out := injectIncludeUsageForOpenAIStream(in)
	jsonEqual(t, out, in)
}

// 客户端明确不要 usage 的场景也要尊重，不能强行覆盖；
// 否则可能影响极少数依赖"不返回 usage"来规避末帧解析问题的客户端。
func TestInjectIncludeUsage_RespectsExplicitFalse(t *testing.T) {
	in := []byte(`{"model":"gpt-4o-mini","stream":true,"stream_options":{"include_usage":false}}`)
	out := injectIncludeUsageForOpenAIStream(in)
	jsonEqual(t, out, in)
}

// 非法 JSON 不应 panic 或改写内容
func TestInjectIncludeUsage_InvalidJSONPassthrough(t *testing.T) {
	in := []byte(`not a json`)
	out := injectIncludeUsageForOpenAIStream(in)
	if string(out) != string(in) {
		t.Fatalf("expected passthrough, got %q", string(out))
	}
}

// stream_options 是数组等非对象类型时保持原样，避免破坏上游预期
func TestInjectIncludeUsage_NonObjectStreamOptionsPassthrough(t *testing.T) {
	in := []byte(`{"model":"gpt-4o-mini","stream":true,"stream_options":[1,2,3]}`)
	out := injectIncludeUsageForOpenAIStream(in)
	jsonEqual(t, out, in)
}

// 保留未知字段不丢
func TestInjectIncludeUsage_PreservesExtraFields(t *testing.T) {
	in := []byte(`{"model":"gpt-4o-mini","stream":true,"temperature":0.3,"my_custom":{"k":1}}`)
	out := injectIncludeUsageForOpenAIStream(in)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := m["temperature"]; !ok {
		t.Errorf("temperature dropped")
	}
	if _, ok := m["my_custom"]; !ok {
		t.Errorf("my_custom dropped")
	}
	opts, ok := m["stream_options"].(map[string]any)
	if !ok {
		t.Fatalf("stream_options missing or wrong type: %v", m["stream_options"])
	}
	if opts["include_usage"] != true {
		t.Errorf("include_usage not set to true, got %v", opts["include_usage"])
	}
}
