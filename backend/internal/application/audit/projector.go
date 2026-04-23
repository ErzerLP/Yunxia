package audit

import "encoding/json"

// ProjectStorageSource 返回脱敏后的存储源摘要。
func ProjectStorageSource(raw map[string]any) string {
	safe := map[string]any{
		"name": raw["name"],
	}
	config := map[string]any{}
	if value, ok := raw["config"].(map[string]any); ok {
		config["endpoint"] = value["endpoint"]
		config["region"] = value["region"]
		config["bucket"] = value["bucket"]
		config["base_prefix"] = value["base_prefix"]
		config["force_path_style"] = value["force_path_style"]
		changedFields := make([]string, 0, 2)
		if value["access_key"] != nil {
			changedFields = append(changedFields, "access_key")
		}
		if value["secret_key"] != nil {
			changedFields = append(changedFields, "secret_key")
		}
		if len(changedFields) > 0 {
			config["secret_changed_fields"] = changedFields
		}
	}
	if len(config) > 0 {
		safe["config"] = config
	}
	return mustJSON(safe)
}

func mustJSON(value any) string {
	if value == nil {
		return ""
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
