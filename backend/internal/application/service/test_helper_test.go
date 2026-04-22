package service

import (
    "testing"

    gormrepo "yunxia/internal/infrastructure/persistence/gorm"

    "gorm.io/gorm"
)

func openTestDB(t *testing.T) (*gorm.DB, func()) {
    t.Helper()

    db, err := gormrepo.OpenSQLite(t.TempDir() + "/test.db")
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
