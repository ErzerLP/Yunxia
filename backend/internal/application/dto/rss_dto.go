package dto

// RSSSourceView 表示 RSS 源视图。
type RSSSourceView struct {
	ID                     uint    `json:"id"`
	UserID                 uint    `json:"user_id,omitempty"`
	Name                   string  `json:"name"`
	URL                    string  `json:"url"`
	IsEnabled              bool    `json:"is_enabled"`
	RefreshIntervalSeconds int     `json:"refresh_interval_seconds"`
	LastRefreshedAt        *string `json:"last_refreshed_at"`
	LastError              *string `json:"last_error"`
	CreatedAt              string  `json:"created_at"`
	UpdatedAt              string  `json:"updated_at"`
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
}

// RSSItemView 表示 RSS 条目视图。
type RSSItemView struct {
	ID                    uint    `json:"id"`
	UserID                uint    `json:"user_id,omitempty"`
	SourceID              uint    `json:"source_id"`
	Title                 string  `json:"title"`
	Link                  string  `json:"link"`
	PublishedAt           *string `json:"published_at"`
	GUID                  string  `json:"guid"`
	DownloadURL           string  `json:"download_url"`
	LinkType              string  `json:"link_type"`
	Status                string  `json:"status"`
	MatchedSubscriptionID *uint   `json:"matched_subscription_id"`
	TaskID                *uint   `json:"task_id"`
	ErrorMessage          *string `json:"error_message"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

// RSSItemListResponse 表示 RSS 条目列表响应。
type RSSItemListResponse struct {
	Items []RSSItemView `json:"items"`
}

// RSSManualDownloadRequest 表示手动将 RSS 条目入队请求。
type RSSManualDownloadRequest struct {
	SubscriptionID uint `json:"subscription_id"`
}

// RSSQBitHealthResponse 表示 qBittorrent 健康检查结果。
type RSSQBitHealthResponse struct {
	Enabled bool    `json:"enabled"`
	Status  string  `json:"status"`
	Error   *string `json:"error,omitempty"`
}
