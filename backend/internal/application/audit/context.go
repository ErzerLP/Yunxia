package audit

import (
	"context"

	"yunxia/internal/infrastructure/security"
)

type requestContextKey struct{}

// RequestContext 表示审计所需的请求元数据。
type RequestContext struct {
	RequestID  string
	EntryPoint EntryPoint
	ClientIP   string
	UserAgent  string
	Method     string
	Path       string
}

// Snapshot 表示一条审计记录所需的上下文快照。
type Snapshot struct {
	RequestID     string
	EntryPoint    string
	ActorUserID   *uint
	ActorUsername string
	ActorRoleKey  string
	ClientIP      string
	UserAgent     string
	Method        string
	Path          string
}

// WithRequestContext 把请求元数据写入 context。
func WithRequestContext(ctx context.Context, value RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey{}, value)
}

// RequestContextFromContext 读取请求元数据。
func RequestContextFromContext(ctx context.Context) (RequestContext, bool) {
	value, ok := ctx.Value(requestContextKey{}).(RequestContext)
	return value, ok
}

// SnapshotFromContext 组装审计上下文快照。
func SnapshotFromContext(ctx context.Context) Snapshot {
	snapshot := Snapshot{}
	if requestContext, ok := RequestContextFromContext(ctx); ok {
		snapshot.RequestID = requestContext.RequestID
		snapshot.EntryPoint = string(requestContext.EntryPoint)
		snapshot.ClientIP = requestContext.ClientIP
		snapshot.UserAgent = requestContext.UserAgent
		snapshot.Method = requestContext.Method
		snapshot.Path = requestContext.Path
	}
	if auth, ok := security.RequestAuthFromContext(ctx); ok {
		snapshot.ActorUserID = uintPointerFromValue(auth.UserID)
		snapshot.ActorUsername = auth.Username
		snapshot.ActorRoleKey = auth.RoleKey
	}
	return snapshot
}

func uintPointerFromValue(value uint) *uint {
	if value == 0 {
		return nil
	}
	result := value
	return &result
}
