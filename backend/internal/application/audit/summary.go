package audit

// Summary 返回列表页使用的审计摘要。
func Summary(event Event) string {
	switch {
	case event.ResourceType == "user" && event.Action == "create" && event.Result == ResultSuccess:
		return "创建用户"
	case event.ResourceType == "storage_source" && event.Action == "update" && event.Result == ResultSuccess:
		return "更新存储源配置"
	case event.Result == ResultDenied:
		return "敏感操作被拒绝"
	default:
		return event.ResourceType + "." + event.Action + "." + string(event.Result)
	}
}
