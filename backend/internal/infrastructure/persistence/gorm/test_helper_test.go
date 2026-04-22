package gorm

import (
	"testing"

	"gorm.io/gorm"
)

func testDB(t *testing.T, dsn string) (*gorm.DB, func()) {
	t.Helper()

	db, err := OpenSQLite(dsn)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}

	return db, func() {
		_ = sqlDB.Close()
	}
}
