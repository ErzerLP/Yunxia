# Database Guidelines

> Database patterns and conventions for this project.

## Scope / Trigger

Read this when touching entities, repository interfaces, GORM models, DB initialization, migrations, persistence tests, upload/task/share/audit fields, or code that changes stored data.

Current stack:

- SQLite via GORM
- Open/migrate entry: `backend/internal/infrastructure/persistence/gorm/db.go`
- Models: `backend/internal/infrastructure/persistence/gorm/models.go`
- Repository interfaces: `backend/internal/domain/repository/*.go`
- Repository implementations: `backend/internal/infrastructure/persistence/gorm/*_repo_impl.go`

## Signatures

DB open/migration:

```go
func OpenSQLite(dsn string) (*gorm.DB, error)
```

Repository interface pattern:

```go
type UserRepository interface {
    Create(ctx context.Context, user *entity.User) error
    FindByID(ctx context.Context, id uint) (*entity.User, error)
    List(ctx context.Context, filter UserListFilter) ([]*entity.User, error)
    Update(ctx context.Context, user *entity.User) error
}
```

GORM constructor pattern:

```go
type UserRepository struct { db *gorm.DB }
func NewUserRepository(db *gorm.DB) *UserRepository { return &UserRepository{db: db} }
```

## Contracts

### GORM model contract

- Models are persistence-only structs in `internal/infrastructure/persistence/gorm/models.go`.
- Domain entities stay in `internal/domain/entity/` and must not carry GORM tags.
- Model fields use explicit GORM tags for primary keys, indexes, sizes, defaults, and column overrides.
- JSON-like payloads are stored as `string` with `gorm:"type:text"`, e.g. `ConfigJSON`, `UploadedChunksJSON`, `StorageDataJSON`.

Example:

```go
type StorageSourceModel struct {
    ID         uint   `gorm:"primaryKey"`
    Name       string `gorm:"uniqueIndex;size:128;not null"`
    WebDAVSlug string `gorm:"uniqueIndex;size:128;not null"`
    MountPath  string `gorm:"uniqueIndex;size:512;not null"`
    ConfigJSON string `gorm:"type:text;not null"`
}
```

### Migration contract

`OpenSQLite` runs `db.AutoMigrate` over all persisted models. If adding a new persisted model/field, update the `AutoMigrate` list when needed and add/adjust tests.

### Repository contract

- Always use `r.db.WithContext(ctx)` for DB operations.
- Convert domain entities and GORM models in helper functions in the repo implementation file.
- Convert `gorm.ErrRecordNotFound` to `domainrepo.ErrNotFound`.
- For update/delete, check `RowsAffected == 0` and return `domainrepo.ErrNotFound`.
- Use stable ordering for lists, e.g. `sort_order asc, id asc` or `created_at desc, id desc`.

## Query Patterns

### Create

```go
model := userModelFromEntity(user)
if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
    return err
}
*user = *userEntityFromModel(model)
```

> **Warning**: GORM fields with `gorm:"default:..."` may use the database
> default for Go zero values during `Create`. For `bool` fields whose default is
> `true`, an intentional `false` must be persisted explicitly, then covered by a
> repository/service regression test.

Example:

```go
requestedEnabled := subscription.IsEnabled
if err := r.db.WithContext(ctx).Create(model).Error; err != nil { return err }
if !requestedEnabled {
    if err := r.db.WithContext(ctx).
        Model(&RSSSubscriptionModel{}).
        Where("id = ?", model.ID).
        UpdateColumn("is_enabled", false).Error; err != nil {
        return err
    }
    model.IsEnabled = false
}
```

### Find

```go
var model UserModel
if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
    return nil, normalizeError(err)
}
return userEntityFromModel(&model), nil
```

### Update

```go
result := r.db.WithContext(ctx).
    Model(&UserModel{}).
    Where("id = ?", user.ID).
    Select("*").
    Omit("ID", "Username", "CreatedAt").
    Updates(model)
if result.Error != nil { return result.Error }
if result.RowsAffected == 0 { return domainrepo.ErrNotFound }
```

For state-machine records that intentionally clear fields or write zero values
(for example `DownloadTaskModel` clearing `error_message`, `eta_seconds`, or
`staging_dir` after a terminal transition), use `Select("*")` with explicit
omits so GORM does not silently skip zero values:

```go
result := r.db.WithContext(ctx).
    Model(&DownloadTaskModel{}).
    Where("id = ?", task.ID).
    Select("*").
    Omit("ID", "CreatedAt").
    Updates(model)
```

This is required when a later refresh must observe that a nullable/string field
was actually cleared, not just hidden in the API view.

## Validation & Error Matrix

| Condition | Return / Behavior |
|---|---|
| Record missing | `domainrepo.ErrNotFound` from repository |
| Unique constraint conflict | Prefer service pre-validation + sentinel conflict error for user-facing cases |
| Invalid path/config before persistence | Service sentinel such as `ErrPathInvalid` / `ErrConfigInvalid` |
| Update/delete affects zero rows | `domainrepo.ErrNotFound` |
| `Create` receives explicit `false` for a `default:true` bool | Persist `false` explicitly; do not let DB default rewrite it to `true` |
| State transition clears nullable / zero-value fields | Repository update must use `Select("*")` with safe omits; API-only sanitization is not enough |

Do not compare error strings; use `errors.Is`.

## Tests Required

For DB-related changes, add/update:

- Repository tests in `backend/internal/infrastructure/persistence/gorm/*_test.go`
- Service tests in `backend/internal/application/service/*_test.go`
- HTTP workflow tests in `backend/internal/interfaces/http/*_test.go` when visible through API
- Explicit false regression tests when touching `default:true` bool fields.

Run from `backend/`:

```bash
go test ./...
```

## Wrong vs Correct

### Wrong

```go
if err == gorm.ErrRecordNotFound {
    return nil, err // leaks infrastructure error upward
}
```

### Correct

```go
if errors.Is(err, gorm.ErrRecordNotFound) {
    return nil, domainrepo.ErrNotFound
}
```
