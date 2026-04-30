# Error Handling

> How errors are handled in this project.

## Overview

Backend error handling is layered:

1. Domain/application code returns sentinel errors or wrapped errors.
2. Handlers bind/validate requests and map known errors with `errors.Is`.
3. REST responses use `internal/interfaces/http/response/response.go`.
4. Middleware supplies `request_id`, access logs, and panic recovery.

## API Response Contract

Success envelope:

```json
{
  "success": true,
  "code": "OK",
  "message": "ok",
  "data": {},
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

Error envelope:

```json
{
  "success": false,
  "code": "VALIDATION_ERROR",
  "message": "...",
  "error": { "details": {} },
  "meta": { "request_id": "...", "timestamp": "..." }
}
```

Use helpers:

```go
httpresp.JSON(c, http.StatusOK, "OK", "ok", resp)
httpresp.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
httpresp.Empty(c, http.StatusOK)
```

Exceptions: file streams, redirects/presigned downloads, and WebDAV responses may bypass the JSON envelope.

## Error Types

Repository shared error:

```go
var ErrNotFound = errors.New("resource not found")
```

Service sentinel errors live in files like `backend/internal/application/service/storage_errors.go`, e.g.:

- `ErrSourceDriverUnsupported`, `ErrConfigInvalid`, `ErrSourceNameConflict`, `ErrPathInvalid`
- `ErrFileNotFound`, `ErrFileAlreadyExists`, `ErrUploadSessionNotFound`, `ErrUploadTooLarge`
- `ErrACLDenied`, `ErrPermissionDenied`
- `ErrShareExpired`, `ErrSharePasswordRequired`, `ErrSharePasswordInvalid`
- `ErrTaskInvalidState`, `ErrNoBackingStorage`, `ErrNameConflict`

Handlers must use `errors.Is`, not string matching.

## Handler Mapping Pattern

Prefer a per-handler `writeError` helper when a feature has multiple known errors:

```go
func (h *SourceHandler) writeError(c *gin.Context, err error) {
    switch {
    case errors.Is(err, domainrepo.ErrNotFound):
        httpresp.Error(c, http.StatusNotFound, "SOURCE_NOT_FOUND", err.Error(), nil)
    case errors.Is(err, appsvc.ErrSourceMountPathConflict):
        httpresp.Error(c, http.StatusConflict, "MOUNT_PATH_CONFLICT", err.Error(), nil)
    case errors.Is(err, appsvc.ErrPathInvalid):
        httpresp.Error(c, http.StatusBadRequest, "PATH_INVALID", err.Error(), nil)
    default:
        httpresp.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
    }
}
```

Binding errors from `ShouldBindJSON` / `ShouldBindQuery` currently return `400 VALIDATION_ERROR`.

## Validation & Error Matrix

| Condition | HTTP | Code Pattern |
|---|---:|---|
| Binding/query validation fails | 400 | `VALIDATION_ERROR` |
| JWT missing/invalid | 401 | `AUTH_TOKEN_INVALID` / auth-specific code |
| Invalid credentials | 401 | `AUTH_INVALID_CREDENTIALS` |
| Missing capability | 403 | `CAPABILITY_DENIED` / `PERMISSION_DENIED` |
| Repository not found | 404 | `<RESOURCE>_NOT_FOUND` |
| User-correctable conflict | 409 | `*_CONFLICT` |
| Unsupported driver/invalid config | 422 | `*_UNSUPPORTED` / `CONFIG_INVALID` |
| Unexpected failure | 500 | `INTERNAL_ERROR` |

## Common Mistakes

- Do not expose raw DB messages for common user errors.
- Do not `panic` or `log.Fatal` from request paths.
- Do not forget `return` after writing an error response.
- Do not add frontend-facing error codes without updating `backend/API_CONTRACT.md` and frontend handling/types.

## Tests Required

For new error behavior, assert:

- HTTP status code
- `success: false`
- stable `code`
- no raw low-level sensitive message for common user-triggered errors
- `meta.request_id` present for JSON REST responses

## Wrong vs Correct

### Wrong

```go
if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
}
```

### Correct

```go
if err != nil {
    h.writeError(c, err)
    return
}
```
