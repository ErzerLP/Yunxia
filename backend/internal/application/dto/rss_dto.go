package dto

// RSSSourceView 表示 RSS 源视图。
type RSSSourceView struct {
	ID                     uint                 `json:"id"`
	UserID                 uint                 `json:"user_id,omitempty"`
	Name                   string               `json:"name"`
	URL                    string               `json:"url"`
	IsEnabled              bool                 `json:"is_enabled"`
	RefreshIntervalSeconds int                  `json:"refresh_interval_seconds"`
	LastRefreshedAt        *string              `json:"last_refreshed_at"`
	LastError              *string              `json:"last_error"`
	HealthStatus           string               `json:"health_status"`
	ConsecutiveFailures    int                  `json:"consecutive_failures"`
	LastSuccessAt          *string              `json:"last_success_at"`
	NextRefreshAt          *string              `json:"next_refresh_at"`
	LastRefreshStatus      string               `json:"last_refresh_status"`
	LastRefreshStats       *RSSRefreshStatsView `json:"last_refresh_stats"`
	CreatedAt              string               `json:"created_at"`
	UpdatedAt              string               `json:"updated_at"`
}

// RSSSourceListResponse 表示 RSS 源列表响应。
type RSSSourceListResponse struct {
	Items []RSSSourceView `json:"items"`
}

// RSSSourceUpsertRequest 表示新增/更新 RSS 源请求。
type RSSSourceUpsertRequest struct {
	Name                   string `json:"name" binding:"required"`
	URL                    string `json:"url" binding:"required"`
	IsEnabled              *bool  `json:"is_enabled"`
	RefreshIntervalSeconds int    `json:"refresh_interval_seconds"`
}

// RSSExportSource 表示导出文件中的 RSS 源配置，不包含运行时状态。
type RSSExportSource struct {
	Name                   string `json:"name"`
	URL                    string `json:"url"`
	IsEnabled              bool   `json:"is_enabled"`
	RefreshIntervalSeconds int    `json:"refresh_interval_seconds"`
}

// RSSExportSubscription 表示导出文件中的 RSS 订阅配置，不包含条目/任务状态。
type RSSExportSubscription struct {
	SourceURL               string   `json:"source_url"`
	Name                    string   `json:"name"`
	IsEnabled               bool     `json:"is_enabled"`
	MustContain             []string `json:"must_contain"`
	MustNotContain          []string `json:"must_not_contain"`
	UseRegex                bool     `json:"use_regex"`
	CaseSensitive           bool     `json:"case_sensitive"`
	TargetVirtualParentPath string   `json:"target_virtual_parent_path"`
	DirectoryTemplate       string   `json:"directory_template"`
	FilenameTemplate        string   `json:"filename_template"`
}

// RSSExportResponse 表示 RSS 源/订阅导出响应。
type RSSExportResponse struct {
	Version       int                     `json:"version"`
	ExportedAt    string                  `json:"exported_at"`
	Sources       []RSSExportSource       `json:"sources"`
	Subscriptions []RSSExportSubscription `json:"subscriptions"`
}

// RSSImportSource 表示 RSS 导入请求中的源配置。
type RSSImportSource struct {
	Name                   string `json:"name"`
	URL                    string `json:"url"`
	IsEnabled              *bool  `json:"is_enabled"`
	RefreshIntervalSeconds int    `json:"refresh_interval_seconds"`
}

// RSSImportSubscription 表示 RSS 导入请求中的订阅配置。
type RSSImportSubscription struct {
	SourceURL               string   `json:"source_url"`
	SourceRef               string   `json:"source_ref,omitempty"`
	Name                    string   `json:"name"`
	IsEnabled               *bool    `json:"is_enabled"`
	MustContain             []string `json:"must_contain"`
	MustNotContain          []string `json:"must_not_contain"`
	UseRegex                bool     `json:"use_regex"`
	CaseSensitive           bool     `json:"case_sensitive"`
	TargetVirtualParentPath string   `json:"target_virtual_parent_path"`
	DirectoryTemplate       string   `json:"directory_template"`
	FilenameTemplate        string   `json:"filename_template"`
}

// RSSImportRequest 表示 RSS 源/订阅导入请求。
type RSSImportRequest struct {
	DryRun        bool                    `json:"dry_run"`
	Sources       []RSSImportSource       `json:"sources"`
	Subscriptions []RSSImportSubscription `json:"subscriptions"`
}

// RSSImportItemResult 表示导入中单个源或订阅的处理结果。
type RSSImportItemResult struct {
	Index        int     `json:"index"`
	Action       string  `json:"action"`
	Success      bool    `json:"success"`
	ID           uint    `json:"id,omitempty"`
	SourceURL    string  `json:"source_url,omitempty"`
	Name         string  `json:"name,omitempty"`
	ErrorCode    *string `json:"error_code,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

// RSSImportSectionResult 汇总一类导入条目的处理结果。
type RSSImportSectionResult struct {
	Items   []RSSImportItemResult `json:"items"`
	Created int                   `json:"created"`
	Reused  int                   `json:"reused"`
	Skipped int                   `json:"skipped"`
	Failed  int                   `json:"failed"`
}

// RSSImportResponse 表示 RSS 导入响应。
type RSSImportResponse struct {
	DryRun        bool                   `json:"dry_run"`
	Sources       RSSImportSectionResult `json:"sources"`
	Subscriptions RSSImportSectionResult `json:"subscriptions"`
}

// RSSRefreshResponse 表示 RSS 刷新结果。
type RSSRefreshResponse struct {
	SourceID    uint `json:"source_id"`
	Fetched     int  `json:"fetched"`
	Created     int  `json:"created"`
	Updated     int  `json:"updated"`
	Matched     int  `json:"matched"`
	Enqueued    int  `json:"enqueued"`
	Unsupported int  `json:"unsupported"`
	Failed      int  `json:"failed"`
}

// RSSRefreshStatsView 表示一次 RSS 刷新摘要，可持久化给前端解释无人值守期间发生了什么。
type RSSRefreshStatsView struct {
	SourceID    uint `json:"source_id"`
	Fetched     int  `json:"fetched"`
	Created     int  `json:"created"`
	Updated     int  `json:"updated"`
	Matched     int  `json:"matched"`
	Enqueued    int  `json:"enqueued"`
	Unsupported int  `json:"unsupported"`
	Failed      int  `json:"failed"`
}

// RSSRefreshAllItemView 表示 refresh-all 中单个源的处理结果。
type RSSRefreshAllItemView struct {
	SourceID uint                `json:"source_id"`
	Status   string              `json:"status"`
	Error    *string             `json:"error,omitempty"`
	Stats    *RSSRefreshResponse `json:"stats,omitempty"`
}

// RSSRefreshAllResponse 表示批量刷新启用 RSS 源的结果。
type RSSRefreshAllResponse struct {
	Items     []RSSRefreshAllItemView `json:"items"`
	Refreshed int                     `json:"refreshed"`
	Skipped   int                     `json:"skipped"`
	Failed    int                     `json:"failed"`
}

// RSSSubscriptionView 表示 RSS 订阅规则视图。
type RSSSubscriptionView struct {
	ID                      uint     `json:"id"`
	UserID                  uint     `json:"user_id,omitempty"`
	SourceID                uint     `json:"source_id"`
	Name                    string   `json:"name"`
	IsEnabled               bool     `json:"is_enabled"`
	MustContain             []string `json:"must_contain"`
	MustNotContain          []string `json:"must_not_contain"`
	UseRegex                bool     `json:"use_regex"`
	CaseSensitive           bool     `json:"case_sensitive"`
	TargetVirtualParentPath string   `json:"target_virtual_parent_path"`
	DirectoryTemplate       string   `json:"directory_template"`
	FilenameTemplate        string   `json:"filename_template"`
	ResolvedSourceID        uint     `json:"resolved_source_id,omitempty"`
	ResolvedInnerParentPath string   `json:"resolved_inner_parent_path,omitempty"`
	CreatedAt               string   `json:"created_at"`
	UpdatedAt               string   `json:"updated_at"`
}

// RSSSubscriptionListResponse 表示 RSS 订阅列表响应。
type RSSSubscriptionListResponse struct {
	Items []RSSSubscriptionView `json:"items"`
}

// RSSSubscriptionUpsertRequest 表示新增/更新 RSS 订阅请求。
type RSSSubscriptionUpsertRequest struct {
	SourceID                uint     `json:"source_id" binding:"required"`
	Name                    string   `json:"name" binding:"required"`
	IsEnabled               *bool    `json:"is_enabled"`
	MustContain             []string `json:"must_contain"`
	MustNotContain          []string `json:"must_not_contain"`
	UseRegex                bool     `json:"use_regex"`
	CaseSensitive           bool     `json:"case_sensitive"`
	TargetVirtualParentPath string   `json:"target_virtual_parent_path" binding:"required"`
	DirectoryTemplate       string   `json:"directory_template"`
	FilenameTemplate        string   `json:"filename_template"`
}

// RSSSubscriptionCloneRequest 表示复制 RSS 订阅请求。
type RSSSubscriptionCloneRequest struct {
	Name      string `json:"name"`
	IsEnabled *bool  `json:"is_enabled"`
}

// RSSSubscriptionBatchStateRequest 表示批量启用/禁用 RSS 订阅请求。
type RSSSubscriptionBatchStateRequest struct {
	SubscriptionIDs []uint `json:"subscription_ids" binding:"required,min=1"`
	IsEnabled       *bool  `json:"is_enabled" binding:"required"`
}

// RSSSubscriptionPreviewRequest 表示不落库的临时订阅规则 preview 请求。
type RSSSubscriptionPreviewRequest struct {
	SourceID       uint     `json:"source_id" binding:"required"`
	MustContain    []string `json:"must_contain"`
	MustNotContain []string `json:"must_not_contain"`
	UseRegex       bool     `json:"use_regex"`
	CaseSensitive  bool     `json:"case_sensitive"`
}

// RSSAnimeParsedView 表示 RSS 标题解析结果。
type RSSAnimeParsedView struct {
	AnimeTitle    string `json:"anime_title"`
	Season        string `json:"season"`
	Episode       string `json:"episode"`
	SubtitleGroup string `json:"subtitle_group"`
	Resolution    string `json:"resolution"`
}

// RSSItemView 表示 RSS 条目视图。
type RSSItemView struct {
	ID                    uint               `json:"id"`
	UserID                uint               `json:"user_id,omitempty"`
	SourceID              uint               `json:"source_id"`
	Title                 string             `json:"title"`
	Link                  string             `json:"link"`
	PublishedAt           *string            `json:"published_at"`
	GUID                  string             `json:"guid"`
	DownloadURL           string             `json:"download_url"`
	LinkType              string             `json:"link_type"`
	Parsed                RSSAnimeParsedView `json:"parsed"`
	Status                string             `json:"status"`
	MatchedSubscriptionID *uint              `json:"matched_subscription_id"`
	TaskID                *uint              `json:"task_id"`
	ErrorMessage          *string            `json:"error_message"`
	RetryCount            int                `json:"retry_count"`
	MaxRetryCount         int                `json:"max_retry_count"`
	LastAttemptAt         *string            `json:"last_attempt_at"`
	NextRetryAt           *string            `json:"next_retry_at"`
	RetryReason           *string            `json:"retry_reason"`
	CreatedAt             string             `json:"created_at"`
	UpdatedAt             string             `json:"updated_at"`
}

// RSSItemListResponse 表示 RSS 条目列表响应。
type RSSItemListResponse struct {
	Items []RSSItemView `json:"items"`
}

// RSSManualDownloadRequest 表示手动将 RSS 条目入队请求。
type RSSManualDownloadRequest struct {
	SubscriptionID uint `json:"subscription_id"`
}

// RSSItemBatchIgnoreRequest 表示批量忽略 RSS 条目请求。
type RSSItemBatchIgnoreRequest struct {
	ItemIDs []uint `json:"item_ids" binding:"required,min=1"`
}

// RSSItemBatchRetryRequest 表示批量手动重试 RSS 条目请求。
type RSSItemBatchRetryRequest struct {
	ItemIDs        []uint `json:"item_ids" binding:"required,min=1"`
	SubscriptionID uint   `json:"subscription_id"`
}

// RSSItemBatchActionResult 表示批量条目动作中单个 item 的处理结果。
type RSSItemBatchActionResult struct {
	ItemID       uint         `json:"item_id"`
	Success      bool         `json:"success"`
	Item         *RSSItemView `json:"item,omitempty"`
	ErrorCode    *string      `json:"error_code,omitempty"`
	ErrorMessage *string      `json:"error_message,omitempty"`
}

// RSSItemBatchActionResponse 表示批量条目动作响应。
type RSSItemBatchActionResponse struct {
	Items     []RSSItemBatchActionResult `json:"items"`
	Succeeded int                        `json:"succeeded"`
	Failed    int                        `json:"failed"`
}

// RSSSubscriptionBatchStateResult 表示批量订阅状态变更中单个 subscription 的处理结果。
type RSSSubscriptionBatchStateResult struct {
	SubscriptionID uint                 `json:"subscription_id"`
	Success        bool                 `json:"success"`
	Subscription   *RSSSubscriptionView `json:"subscription,omitempty"`
	ErrorCode      *string              `json:"error_code,omitempty"`
	ErrorMessage   *string              `json:"error_message,omitempty"`
}

// RSSSubscriptionBatchStateResponse 表示批量订阅状态变更响应。
type RSSSubscriptionBatchStateResponse struct {
	Items     []RSSSubscriptionBatchStateResult `json:"items"`
	Succeeded int                               `json:"succeeded"`
	Failed    int                               `json:"failed"`
}

// RSSSubscriptionPreviewItem 表示订阅规则对单个已有条目的解释结果。
type RSSSubscriptionPreviewItem struct {
	ItemID        uint     `json:"item_id"`
	Title         string   `json:"title"`
	DownloadURL   string   `json:"download_url"`
	CurrentStatus string   `json:"current_status"`
	Result        string   `json:"result"`
	Matched       []string `json:"matched"`
	Missing       []string `json:"missing"`
	Excluded      []string `json:"excluded"`
}

// RSSSubscriptionPreviewResponse 表示订阅规则 preview 响应。
type RSSSubscriptionPreviewResponse struct {
	SubscriptionID uint                         `json:"subscription_id"`
	SourceID       uint                         `json:"source_id"`
	Items          []RSSSubscriptionPreviewItem `json:"items"`
	Matched        int                          `json:"matched"`
	Missing        int                          `json:"missing"`
	Excluded       int                          `json:"excluded"`
}

// RSSQBitHealthResponse 表示 qBittorrent 健康检查结果。
type RSSQBitHealthResponse struct {
	Enabled bool    `json:"enabled"`
	Status  string  `json:"status"`
	Error   *string `json:"error,omitempty"`
}
