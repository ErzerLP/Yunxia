package gorm

import (
	"encoding/json"
	"strings"
)

func jsonObject(raw string) string {
	return normalizeJSON(raw, "{}")
}

func jsonArray(raw string) string {
	return normalizeJSON(raw, "[]")
}

func normalizeJSON(raw, fallback string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "null") {
		return fallback
	}
	if json.Valid([]byte(raw)) {
		return raw
	}
	// 保留无效内容，让 PostgreSQL jsonb 约束返回明确的数据库错误。
	return raw
}

func nullableUint(value uint) *uint {
	if value == 0 {
		return nil
	}
	v := value
	return &v
}

func uintValue(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}
