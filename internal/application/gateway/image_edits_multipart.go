package gateway

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// extractModelFromMultipart 扫描 multipart body，返回首个表单字段 model 的文本值（用于路由）。
// 仅扫描不修改 body；调用方应另行保留原始 body 字节用于上游透传。
func extractModelFromMultipart(body []byte, contentType string) (string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", errors.New("multipart: missing boundary")
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := p.FormName()
		if name == "model" && p.FileName() == "" {
			bb, rerr := io.ReadAll(p)
			_ = p.Close()
			if rerr != nil {
				return "", rerr
			}
			v := strings.TrimSpace(string(bb))
			if v == "" {
				return "", errors.New("multipart: empty model")
			}
			return v, nil
		}
		_, _ = io.Copy(io.Discard, p)
		_ = p.Close()
	}
	return "", errors.New("multipart: model field not found")
}

// stripMultipartModelFormField 从 multipart 中移除文本字段 model（非文件），并返回新 body 与新的 Content-Type（含新 boundary）。
// 用于 Azure OpenAI deployment URL：模型已由路径指定，部分上游拒绝 form 中再带 model。
func stripMultipartModelFormField(body []byte, contentType string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, "", errors.New("multipart: missing boundary")
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var out bytes.Buffer
	mw := multipart.NewWriter(&out)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
		name := p.FormName()
		fn := p.FileName()
		partHeader := cloneMIMEHeader(p.Header)
		slurp, rerr := io.ReadAll(p)
		_ = p.Close()
		if rerr != nil {
			return nil, "", rerr
		}
		if name == "model" && fn == "" {
			continue
		}
		pw, werr := mw.CreatePart(partHeader)
		if werr != nil {
			return nil, "", werr
		}
		if _, werr = pw.Write(slurp); werr != nil {
			return nil, "", werr
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), mw.FormDataContentType(), nil
}

func cloneMIMEHeader(src textproto.MIMEHeader) textproto.MIMEHeader {
	if len(src) == 0 {
		return textproto.MIMEHeader{}
	}
	dst := make(textproto.MIMEHeader, len(src))
	for k, vv := range src {
		cp := make([]string, len(vv))
		copy(cp, vv)
		dst[k] = cp
	}
	return dst
}
