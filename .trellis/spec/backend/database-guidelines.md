# Database Guidelines

> Database patterns and conventions for this project.

## Scope / Trigger

Read this when touching entities, repository interfaces, GORM models, DB initialization, migrations, persistence tests, upload/task/share/audit fields, or code that changes stored data.

Current stack:

- PostgreSQL-only via GORM; do not add SQLite fallback or dual-driver branches.
- Open/migrate/runtime entry: `backend/internal/infrastructure/persistence/gorm/db.go`
- Models: `backend/internal/infrastructure/persistence/gorm/models.go`
- Repository interfaces: `backend/internal/domain/repository/*.go`
- Repository implementations: `backend/internal/infrastructure/persistence/gorm/*_repo_impl.go`
- Test DB helper: `backend/internal/infrastructure/persistence/pgtest/pgtest.go`

## Signatures

DB open/migration:

```go
func OpenDatabase(ctx context.Context, cfg config.DatabaseConfig) (*Runtime, error)
func OpenPostgres(ctx context.Context, cfg config.DatabaseConfig) (*Runtime, error)
func (r *Runtime) Migrate(ctx context.Context) error
func (r *Runtime) Ping(ctx context.Context) error
func (r *Runtime) Close() error
```

Transaction port:

```go
type Transactor interface {
    WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
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
- JSON-like payloads remain `string` in domain entities/repository conversion,
  but PostgreSQL columns must be `jsonb`, e.g. `ConfigJSON`, `UploadedChunksJSON`,
  `StorageDataJSON`. Repository conversion helpers must normalize empty object
  payloads to `{}` and empty array payloads to `[]` before persistence.
- Prefer nullable columns (`*uint`, `*time.Time`, `*string`) when the database
  meaning is “not resolved / absent”; avoid new `0` sentinel columns.
- Business defaults are assigned in service/entity/repository construction, not
  by relying on database defaults to rewrite Go zero values.

Example:

```go
type StorageSourceModel struct {
    ID         uint   `gorm:"primaryKey"`
    Name       string `gorm:"uniqueIndex;size:128;not null"`
    WebDAVSlug string `gorm:"uniqueIndex;size:128;not null"`
    MountPath  string `gorm:"uniqueIndex;size:512;not null"`
    ConfigJSON string `gorm:"type:jsonb;not null"`
}
```

### Migration contract

`OpenDatabase` / `OpenPostgres` open PostgreSQL only and optionally run
`Runtime.Migrate` based on `DatabaseConfig.AutoMigrate`. `Runtime.Migrate` runs
`db.AutoMigrate` over all persisted models. If adding a new persisted model/field,
update the model list when needed and add/adjust tests.

Docker Compose and local integration tests must use PostgreSQL. SQLite is no
longer a supported test substitute because it can hide PostgreSQL jsonb,
constraint, default, and transaction behavior differences.

### Repository contract

- Always use `dbFor(ctx, r.db)` for DB operations so repositories participate in
  the GORM transaction attached by `Transactor.WithinTx`.
- Convert domain entities and GORM models in helper functions in the repo implementation file.
- Convert database errors through `normalizeGormError` /
  `normalizeGormNotFound`; do not leak `gorm`, `pgconn`, or SQLSTATE details to
  services/handlers.
- For update/delete, check `RowsAffected == 0` and return `domainrepo.ErrNotFound`.
- Use stable ordering for lists, e.g. `sort_order asc, id asc` or `created_at desc, id desc`.

## Query Patterns

### Create

```go
model := userModelFromEntity(user)
if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
    return normalizeGormError(err)
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
if err := dbFor(ctx, r.db).Create(model).Error; err != nil { return normalizeGormError(err) }
if !requestedEnabled {
    if err := dbFor(ctx, r.db).
        Model(&RSSSubscriptionModel{}).
        Where("id = ?", model.ID).
        UpdateColumn("is_enabled", false).Error; err != nil {
        return normalizeGormError(err)
    }
    model.IsEnabled = false
}
```

### Find

```go
var model UserModel
if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
    return nil, normalizeError(err)
}
return userEntityFromModel(&model), nil
```

### Update

```go
result := dbFor(ctx, r.db).
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
result := dbFor(ctx, r.db).
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
| PostgreSQL unique violation / GORM duplicated key | `domainrepo.ErrConflict` or more specific service sentinel for user-facing cases |
| PostgreSQL FK / NOT NULL / CHECK violation | `domainrepo.ErrConstraintViolation`; handler/service maps to stable 4xx/5xx without SQLSTATE leakage |
| Invalid path/config before persistence | Service sentinel such as `ErrPathInvalid` / `ErrConfigInvalid` |
| Update/delete affects zero rows | `domainrepo.ErrNotFound` |
| `Create` receives explicit `false` for a `default:true` bool | Persist `false` explicitly; do not let DB default rewrite it to `true` |
| State transition clears nullable / zero-value fields | Repository update must use `Select("*")` with safe omits; API-only sanitization is not enough |
| JSON payload field is empty | Persist `{}` for object fields or `[]` for array fields |

Do not compare error strings; use `errors.Is`.

## Tests Required

For DB-related changes, add/update:

- Repository tests in `backend/internal/infrastructure/persistence/gorm/*_test.go`
- Service tests in `backend/internal/application/service/*_test.go`
- HTTP workflow tests in `backend/internal/interfaces/http/*_test.go` when visible through API
- Explicit false regression tests when touching `default:true` bool fields.
- PostgreSQL repository/integration tests should use `YUNXIA_TEST_DATABASE_DSN`
  and isolated schema helpers. If the env var is absent, integration tests may
  skip; do not silently fall back to SQLite.

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

## Scenario: PostgreSQL-only Runtime, Transactions, and Integration Tests

### 1. Scope / Trigger

- Trigger: changing DB initialization, GORM models, repository implementations,
  repository tests, Docker database wiring, or stored JSON/nullable fields.
- Goal: keep PostgreSQL as the single runtime/test database and prevent SQLite
  compatibility branches from drifting into the codebase.

### 2. Signatures

Runtime:

```go
func OpenDatabase(ctx context.Context, cfg config.DatabaseConfig) (*Runtime, error)
func OpenPostgres(ctx context.Context, cfg config.DatabaseConfig) (*Runtime, error)
func (r *Runtime) Migrate(ctx context.Context) error
func (r *Runtime) Ping(ctx context.Context) error
func (r *Runtime) Close() error
```

Transaction port:

```go
type Transactor interface {
    WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

Repository DB handle:

```go
func dbFor(ctx context.Context, db *gorm.DB) *gorm.DB
```

### 3. Contracts

- `YUNXIA_DATABASE_DSN` must be a PostgreSQL DSN.
- `YUNXIA_DATABASE_AUTO_MIGRATE` controls whether startup runs AutoMigrate.
- Connection pool env keys:
  - `YUNXIA_DATABASE_MAX_OPEN_CONNS`
  - `YUNXIA_DATABASE_MAX_IDLE_CONNS`
  - `YUNXIA_DATABASE_CONN_MAX_LIFETIME`
  - `YUNXIA_DATABASE_CONN_MAX_IDLE_TIME`
  - `YUNXIA_DATABASE_SLOW_THRESHOLD`
- Docker Compose must include a `postgres` service and a `postgres-data` volume;
  backend must depend on the PostgreSQL healthcheck before startup.
- Repository/integration tests use `YUNXIA_TEST_DATABASE_DSN` and isolated
  schemas. Missing DSN means skip the PostgreSQL integration test, not fallback
  to SQLite. Schema-isolated tests must keep `MaxOpenConns=1` and avoid
  connection lifetime recycling after `SET search_path`, otherwise later queries
  may run on a connection without the test schema.
- Domain entities and DTOs must not expose GORM/PostgreSQL-specific types.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Empty DB DSN | Startup returns a clear DB initialization error |
| PostgreSQL ping fails | Backend startup fails / health is unavailable |
| `AutoMigrate` fails | Startup fails; do not continue with partial schema |
| Unique violation | Repository returns `domainrepo.ErrConflict` |
| FK / NOT NULL / CHECK violation | Repository returns `domainrepo.ErrConstraintViolation` |
| Record not found | Repository returns `domainrepo.ErrNotFound` |
| Missing `YUNXIA_TEST_DATABASE_DSN` | PostgreSQL integration tests skip explicitly |

### 5. Good/Base/Bad Cases

- Good: repository code calls `dbFor(ctx, r.db)` and therefore participates in
  `Transactor.WithinTx` without importing transaction internals.
- Good: JSON object fields persist `{}` and JSON array fields persist `[]`, with
  PostgreSQL columns declared as `jsonb`.
- Base: a service can continue using repositories without explicit transactions
  when it performs one independent write.
- Bad: adding `database.driver=sqlite|postgres`, importing `gorm.io/driver/sqlite`,
  or making tests silently use SQLite when PostgreSQL is unavailable.
- Bad: handler/application service imports `gorm`, `pgconn`, or PostgreSQL driver
  packages directly.

### 6. Tests Required

When touching this area, add/update tests for:

- PostgreSQL runtime config defaults and env overrides.
- Compose config includes PostgreSQL DSN, healthcheck, and `postgres-data`.
- Repository tests for `ErrNotFound`, unique conflict, JSONB column type, nullable
  source IDs, explicit `false` bool persistence, and transaction rollback.
- Service/HTTP workflow tests affected by DB helper changes.
- A full `go test ./...`; with a real `YUNXIA_TEST_DATABASE_DSN`, repository
  integration tests should run against PostgreSQL isolated schemas.

### 7. Wrong vs Correct

#### Wrong

```go
db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
```

#### Correct

```go
runtime, err := OpenDatabase(ctx, cfg.Database)
if err != nil { return err }
defer runtime.Close()
```

#### Wrong

```go
if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
    return err
}
```

#### Correct

```go
if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
    return normalizeGormError(err)
}
```
