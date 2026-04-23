package security

import "context"

type requestAuthContextKey struct{}

// RequestAuth 表示当前请求的最小认证身份。
type RequestAuth struct {
	UserID       uint
	RoleKey      string
	Status       string
	Capabilities []string
}

// WithRequestAuth 把认证身份写入 context。
func WithRequestAuth(ctx context.Context, auth RequestAuth) context.Context {
	return context.WithValue(ctx, requestAuthContextKey{}, auth)
}

// RequestAuthFromContext 读取请求中的认证身份。
func RequestAuthFromContext(ctx context.Context) (RequestAuth, bool) {
	auth, ok := ctx.Value(requestAuthContextKey{}).(RequestAuth)
	return auth, ok
}
