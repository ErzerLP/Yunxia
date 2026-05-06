package service

import (
	"testing"

	"yunxia/internal/infrastructure/persistence/pgtest"

	"gorm.io/gorm"
)

func openTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	return pgtest.OpenIsolatedDB(t)
}
