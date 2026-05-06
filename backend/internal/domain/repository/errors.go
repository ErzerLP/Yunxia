package repository

import "errors"

// ErrNotFound 表示资源不存在。
var ErrNotFound = errors.New("resource not found")

// ErrConflict 表示唯一约束或业务唯一性冲突。
var ErrConflict = errors.New("resource conflict")

// ErrConstraintViolation 表示数据库约束校验失败。
var ErrConstraintViolation = errors.New("resource constraint violation")
