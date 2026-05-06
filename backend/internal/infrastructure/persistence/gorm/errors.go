package gorm

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	gormpkg "gorm.io/gorm"

	domainrepo "yunxia/internal/domain/repository"
)

func normalizeGormError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gormpkg.ErrRecordNotFound) {
		return domainrepo.ErrNotFound
	}
	if errors.Is(err, gormpkg.ErrDuplicatedKey) {
		return domainrepo.ErrConflict
	}
	if errors.Is(err, gormpkg.ErrForeignKeyViolated) {
		return domainrepo.ErrConstraintViolation
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domainrepo.ErrConflict
		case "23502", "23503", "23514":
			return domainrepo.ErrConstraintViolation
		}
	}

	return err
}

func normalizeGormNotFound(err error) error {
	if errors.Is(err, gormpkg.ErrRecordNotFound) {
		return domainrepo.ErrNotFound
	}
	return normalizeGormError(err)
}
