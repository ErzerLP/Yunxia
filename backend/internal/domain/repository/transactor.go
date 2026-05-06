package repository

import "context"

// Transactor 定义应用层可依赖的最小事务能力。
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
