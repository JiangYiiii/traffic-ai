package provider

import "strings"

// ResolveDefinition 按 models.provider 解析目录项：先按商家 ID，再按唯一 ProviderTag。
func ResolveDefinition(providerField string) (Definition, bool) {
	if d, ok := ByID(providerField); ok {
		return d, true
	}
	var match *Definition
	for i := range Catalog {
		if Catalog[i].ProviderTag != providerField {
			continue
		}
		if match != nil {
			return Definition{}, false
		}
		d := Catalog[i]
		match = &d
	}
	if match != nil {
		return *match, true
	}
	return Definition{}, false
}

// StoredChatEndpoint 返回上游表中应存储的 endpoint 值（含版本路径）。
// 调用方拼接时只需追加 "/chat/completions"。
func StoredChatEndpoint(d Definition) string {
	return strings.TrimRight(strings.TrimSpace(d.DefaultBaseURL), "/")
}
