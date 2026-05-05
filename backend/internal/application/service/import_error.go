package service

import (
	"errors"
	"io/fs"
	"os"
)

func normalizeImportDriverError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrExist):
		return ErrFileAlreadyExists
	case errors.Is(err, os.ErrNotExist):
		return ErrFileNotFound
	case errors.Is(err, os.ErrInvalid):
		return ErrPathInvalid
	default:
		return err
	}
}
