package repository

import "errors"

// ErrNotFound 表示资源不存在。
var ErrNotFound = errors.New("resource not found")
