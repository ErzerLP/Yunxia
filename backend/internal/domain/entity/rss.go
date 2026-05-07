package entity

import "time"

// RSSSource 表示 Yunxia 管理的 RSS 源。
type RSSSource struct {
	ID                     uint
	UserID                 uint
	Name                   string
	URL                    string
	IsEnabled              bool
	RefreshIntervalSeconds int
	LastRefreshedAt        *time.Time
	LastError              *string
	HealthStatus           string
	ConsecutiveFailures    int
	LastSuccessAt          *time.Time
	NextRefreshAt          *time.Time
	LastRefreshStatus      string
	LastRefreshStatsJSON   string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// RSSSubscription 表示绑定 RSS 源的番剧订阅规则。
type RSSSubscription struct {
	ID                      uint
	UserID                  uint
	SourceID                uint
	Name                    string
	IsEnabled               bool
	MustContain             []string
	MustNotContain          []string
	UseRegex                bool
	CaseSensitive           bool
	TargetVFSParentNodeID   uint
	TargetVirtualParentPath string
	DirectoryTemplate       string
	FilenameTemplate        string
	ResolvedSourceID        uint
	ResolvedInnerParentPath string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// RSSAnimeParsed 表示从 RSS 标题提取出的轻量番剧元信息。
type RSSAnimeParsed struct {
	AnimeTitle    string
	Season        string
	Episode       string
	SubtitleGroup string
	Resolution    string
}

// RSSItem 表示抓取并去重后的 RSS 条目。
type RSSItem struct {
	ID                    uint
	UserID                uint
	SourceID              uint
	Title                 string
	Link                  string
	PublishedAt           *time.Time
	GUID                  string
	DedupKey              string
	DownloadURL           string
	LinkType              string
	Parsed                RSSAnimeParsed
	Status                string
	MatchedSubscriptionID *uint
	TaskID                *uint
	ResultVFSNodeID       uint
	ErrorMessage          *string
	RetryCount            int
	MaxRetryCount         int
	LastAttemptAt         *time.Time
	NextRetryAt           *time.Time
	RetryReason           *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
