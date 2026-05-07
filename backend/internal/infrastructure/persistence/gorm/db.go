package gorm

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	gormpkg "gorm.io/gorm"
	"gorm.io/gorm/logger"

	appcfg "yunxia/internal/infrastructure/config"
)

// Runtime 封装 PostgreSQL + GORM 的连接、迁移、健康检查和关闭能力。
type Runtime struct {
	DB          *gormpkg.DB
	SQLDB       *sql.DB
	autoMigrate bool
}

// OpenDatabase 打开 PostgreSQL 数据库，并按配置执行连接池初始化、健康检查和自动迁移。
func OpenDatabase(ctx context.Context, cfg appcfg.DatabaseConfig) (*Runtime, error) {
	return OpenPostgres(ctx, cfg)
}

// OpenPostgres 打开 PostgreSQL 数据库，不提供 SQLite fallback。
func OpenPostgres(ctx context.Context, cfg appcfg.DatabaseConfig) (*Runtime, error) {
	if cfg.DSN == "" {
		return nil, errors.New("database dsn is required")
	}

	db, err := gormpkg.Open(postgres.Open(cfg.DSN), &gormpkg.Config{
		Logger:         newGormLogger(cfg.SlowThreshold),
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	applyPoolConfig(sqlDB, cfg)

	runtime := &Runtime{DB: db, SQLDB: sqlDB, autoMigrate: cfg.AutoMigrate}
	if err := runtime.Ping(ctx); err != nil {
		_ = runtime.Close()
		return nil, err
	}
	if cfg.AutoMigrate {
		if err := runtime.Migrate(ctx); err != nil {
			_ = runtime.Close()
			return nil, err
		}
	}

	return runtime, nil
}

// Migrate 执行当前 PostgreSQL schema 的 GORM AutoMigrate。
func (r *Runtime) Migrate(ctx context.Context) error {
	if r == nil || r.DB == nil {
		return errors.New("database runtime is not initialized")
	}
	return normalizeGormError(r.DB.WithContext(ctx).AutoMigrate(allModels()...))
}

// Ping 验证底层 PostgreSQL 连接是否可用。
func (r *Runtime) Ping(ctx context.Context) error {
	if r == nil || r.SQLDB == nil {
		return errors.New("database runtime is not initialized")
	}
	return r.SQLDB.PingContext(ctx)
}

// Close 关闭底层数据库连接池。
func (r *Runtime) Close() error {
	if r == nil || r.SQLDB == nil {
		return nil
	}
	return r.SQLDB.Close()
}

func applyPoolConfig(db *sql.DB, cfg appcfg.DatabaseConfig) {
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}
}

func newGormLogger(slowThreshold time.Duration) logger.Interface {
	if slowThreshold <= 0 {
		slowThreshold = 500 * time.Millisecond
	}
	return logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             slowThreshold,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		},
	)
}

func allModels() []any {
	return []any{
		&UserModel{},
		&SystemConfigModel{},
		&RefreshTokenModel{},
		&StorageSourceModel{},
		&StorageObjectModel{},
		&VFSNodeModel{},
		&VFSMountModel{},
		&VFSTagModel{},
		&VFSNodeTagModel{},
		&UploadSessionModel{},
		&DownloadTaskModel{},
		&RSSSourceModel{},
		&RSSSubscriptionModel{},
		&RSSItemModel{},
		&TrashItemModel{},
		&ACLRuleModel{},
		&ShareLinkModel{},
		&AuditLogModel{},
		&NotificationChannelModel{},
		&NotificationEventModel{},
	}
}
