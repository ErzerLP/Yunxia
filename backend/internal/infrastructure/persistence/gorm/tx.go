package gorm

import (
	"context"

	gormpkg "gorm.io/gorm"
)

type txContextKey struct{}

// Transactor 使用 GORM Transaction 实现领域事务端口。
type Transactor struct {
	db *gormpkg.DB
}

// NewTransactor 创建 GORM 事务实现。
func NewTransactor(db *gormpkg.DB) *Transactor {
	return &Transactor{db: db}
}

// WithinTx 在事务 context 中执行回调；嵌套调用会复用已有事务。
func (t *Transactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if txFromContext(ctx) != nil {
		return fn(ctx)
	}
	return normalizeGormError(t.db.WithContext(ctx).Transaction(func(tx *gormpkg.DB) error {
		return fn(context.WithValue(ctx, txContextKey{}, tx))
	}))
}

func dbFor(ctx context.Context, db *gormpkg.DB) *gormpkg.DB {
	if tx := txFromContext(ctx); tx != nil {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

func txFromContext(ctx context.Context) *gormpkg.DB {
	tx, _ := ctx.Value(txContextKey{}).(*gormpkg.DB)
	return tx
}
