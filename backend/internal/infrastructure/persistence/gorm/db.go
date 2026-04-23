package gorm

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// OpenSQLite 打开 SQLite 数据库并自动迁移基础表。
func OpenSQLite(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&UserModel{},
		&SystemConfigModel{},
		&RefreshTokenModel{},
		&StorageSourceModel{},
		&UploadSessionModel{},
		&DownloadTaskModel{},
		&TrashItemModel{},
		&ACLRuleModel{},
		&ShareLinkModel{},
		&AuditLogModel{},
	); err != nil {
		return nil, err
	}

	return db, nil
}
