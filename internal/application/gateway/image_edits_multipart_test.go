package gateway

import (
	"bytes"
	"mime/multipart"
	"strings"
	"testing"
)

func TestExtractModelFromMultipart(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("prompt", "hello")
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.Close()
	ct := w.FormDataContentType()

	got, err := extractModelFromMultipart(buf.Bytes(), ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gpt-image-2" {
		t.Fatalf("model: want gpt-image-2, got %q", got)
	}
}

func TestExtractModelFromMultipart_ModelAfterFile(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("image", "a.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("fakepng"))
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.WriteField("prompt", "x")
	_ = w.Close()
	ct := w.FormDataContentType()

	got, err := extractModelFromMultipart(buf.Bytes(), ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gpt-image-2" {
		t.Fatalf("model: want gpt-image-2, got %q", got)
	}
}

func TestExtractModelFromMultipart_MissingModel(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("prompt", "only")
	_ = w.Close()
	ct := w.FormDataContentType()

	_, err := extractModelFromMultipart(buf.Bytes(), ct)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStripMultipartModelFormField(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("model", "gpt-image-2")
	_ = w.WriteField("prompt", "edit me")
	fw, err := w.CreateFormFile("image", "one.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("abc"))
	_ = w.Close()
	origCT := w.FormDataContentType()

	out, newCT, err := stripMultipartModelFormField(buf.Bytes(), origCT)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `name="model"`) {
		t.Fatalf("model part should be stripped, body snippet: %q", out[:intMin(200, len(out))])
	}
	if _, err := extractModelFromMultipart(out, newCT); err == nil {
		t.Fatal("expected no model field after strip")
	}
	if !strings.Contains(newCT, "multipart/form-data") {
		t.Fatalf("content-type: %s", newCT)
	}
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
