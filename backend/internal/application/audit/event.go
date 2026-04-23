package audit

// EntryPoint 表示审计入口类型。
type EntryPoint string

const (
	EntryPointRESTV1 EntryPoint = "rest_v1"
	EntryPointRESTV2 EntryPoint = "rest_v2"
	EntryPointWebDAV EntryPoint = "webdav"
)

// Result 表示审计结果。
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailed  Result = "failed"
	ResultDenied  Result = "denied"
)

// Target 表示审计目标。
type Target struct {
	ResourceID       string
	SourceID         *uint
	VirtualPath      string
	ResolvedSourceID *uint
	ResolvedPath     string
}

// Event 表示待写入的审计事件。
type Event struct {
	ResourceType string
	Action       string
	Result       Result
	ErrorCode    string
	ResourceID   string
	SourceID     *uint
	VirtualPath  string
	Target       Target
	Before       map[string]any
	After        map[string]any
	Detail       map[string]any
}
