package upstreamurl

import "strings"

// JoinPath 将 pathSuffix（如 "/chat/completions"、"/embeddings"）拼到上游 endpoint 后。
// 若 endpoint 含查询串（例如 Azure 的 ?api-version=...），会把 suffix 插在 '?' 之前，
// 避免出现 "...?api-version=.../chat/completions" 这类非法 URL。
func JoinPath(endpoint, pathSuffix string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	pathSuffix = strings.TrimSpace(pathSuffix)
	if pathSuffix == "" {
		return endpoint
	}
	if !strings.HasPrefix(pathSuffix, "/") {
		pathSuffix = "/" + pathSuffix
	}
	q := strings.Index(endpoint, "?")
	if q < 0 {
		return endpoint + pathSuffix
	}
	return endpoint[:q] + pathSuffix + endpoint[q:]
}
