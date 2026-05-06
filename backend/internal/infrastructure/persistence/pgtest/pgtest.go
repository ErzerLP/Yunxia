package pgtest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	gormpkg "gorm.io/gorm"

	appcfg "yunxia/internal/infrastructure/config"
	gormrepo "yunxia/internal/infrastructure/persistence/gorm"
)

// OpenIsolatedDB 为测试创建独立 PostgreSQL schema；未配置 DSN 时跳过测试。
func OpenIsolatedDB(t testing.TB) (*gormpkg.DB, func()) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("YUNXIA_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("skip PostgreSQL integration test: set YUNXIA_TEST_DATABASE_DSN to run it")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg := appcfg.DatabaseConfig{
		DSN:             dsn,
		AutoMigrate:     false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
		ConnMaxIdleTime: 0,
		SlowThreshold:   500 * time.Millisecond,
	}
	adminRuntime, err := gormrepo.OpenPostgres(ctx, cfg)
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}

	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if err := adminRuntime.DB.WithContext(ctx).Exec("CREATE SCHEMA " + quoteIdentifier(schema)).Error; err != nil {
		_ = adminRuntime.Close()
		t.Fatalf("create test schema %s: %v", schema, err)
	}

	runtime, err := gormrepo.OpenPostgres(ctx, cfg)
	if err != nil {
		_ = adminRuntime.DB.WithContext(ctx).Exec("DROP SCHEMA IF EXISTS " + quoteIdentifier(schema) + " CASCADE").Error
		_ = adminRuntime.Close()
		t.Fatalf("open isolated PostgreSQL test database: %v", err)
	}
	if err := runtime.DB.WithContext(ctx).Exec("SET search_path TO " + quoteIdentifier(schema)).Error; err != nil {
		_ = runtime.Close()
		_ = adminRuntime.DB.WithContext(ctx).Exec("DROP SCHEMA IF EXISTS " + quoteIdentifier(schema) + " CASCADE").Error
		_ = adminRuntime.Close()
		t.Fatalf("set test search_path %s: %v", schema, err)
	}
	if err := runtime.Migrate(ctx); err != nil {
		_ = runtime.Close()
		_ = adminRuntime.DB.WithContext(ctx).Exec("DROP SCHEMA IF EXISTS " + quoteIdentifier(schema) + " CASCADE").Error
		_ = adminRuntime.Close()
		t.Fatalf("migrate test schema %s: %v", schema, err)
	}

	return runtime.DB, func() {
		_ = runtime.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		_ = adminRuntime.DB.WithContext(cleanupCtx).Exec("DROP SCHEMA IF EXISTS " + quoteIdentifier(schema) + " CASCADE").Error
		_ = adminRuntime.Close()
	}
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
