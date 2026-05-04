# Directory Structure

> How backend code is organized in this project.

## Overview

Yunxia backend follows a DDD-style four-layer architecture, with a few adapter
seams that are present in the current codebase:

```text
interfaces  ->  application  ->  domain
 HTTP/WebDAV     services/DTOs    entities/contracts

infrastructure -> domain contracts and selected application adapter interfaces
 GORM/security/storage/downloader/config/logging
```

Dependency direction is inward for domain code. Domain must not import Gin,
GORM, JWT, S3, aria2, or other infrastructure/framework packages.
Application services mostly depend on domain abstractions and injected
interfaces. Current application code also uses a small set of existing
infrastructure helpers for request auth context, logging context, and S3 config
parsing; keep those seams narrow instead of introducing direct DB/HTTP clients
inside services. Infrastructure implements persistence/security/storage/
downloader details and may import domain contracts or application adapter
interfaces when implementing drivers, e.g. the aria2 downloader adapter.

## Directory Layout

```text
backend/
├── cmd/server/                         # Program entry, dependency wiring, server start
├── docker/                             # Container helpers
└── internal/
    ├── domain/
    │   ├── entity/                     # Business entities only
    │   ├── permission/                 # Roles/capabilities/permission resolution
    │   ├── repository/                 # Repository interfaces + shared repo errors
    │   └── storage/                    # Storage driver-facing domain contracts
    ├── application/
    │   ├── audit/                      # Audit event/context/projector/query logic
    │   ├── dto/                        # API request/response shapes
    │   └── service/                    # Use cases, VFS, ACL, upload, task orchestration
    ├── infrastructure/
    │   ├── config/                     # Viper-backed runtime config
    │   ├── downloader/                 # Aria2 / qBittorrent downloader integrations
    │   ├── observability/logging/       # slog root logger and context helpers
    │   ├── persistence/gorm/            # GORM models, DB open/migration, repos
    │   ├── security/                   # bcrypt, JWT, request auth, file access tokens
    │   └── storage/                    # S3 driver/config/client factory
    └── interfaces/
        ├── http/                       # Router + HTTP workflow tests
        ├── http/handler/               # Gin handlers
        ├── http/response/              # Standard response envelope
        └── middleware/                 # Request ID, access log, auth, recovery, capabilities
```

## Feature Placement

When adding/changing a backend feature:

1. Domain model/repository capability: `internal/domain/**`.
2. API request/response structs: `internal/application/dto/`.
3. Use-case orchestration and validation: `internal/application/service/`.
4. Database/security/storage/downloader implementation: `internal/infrastructure/**`.
5. Gin handlers and routes: `internal/interfaces/http/handler/` and `internal/interfaces/http/router.go`.
6. Dependency wiring: `cmd/server/main.go`.
7. Frontend-visible contract: update `backend/API_CONTRACT.md`.

## Constructor / Injection Pattern

Concrete dependencies are wired in `cmd/server/main.go`. Services use constructor injection and functional options:

```go
vfsSvc := appsvc.NewVFSService(
    sourceRepo,
    appsvc.WithVFSFileDriver("s3", s3Driver),
    appsvc.WithVFSFileOperator(fileSvc),
    appsvc.WithVFSACLAuthorizer(aclAuthorizer),
)
```

Do not instantiate GORM, S3, JWT, or repositories from inside application services.

## Scenario: Storage Driver Registry and Third-Party Source Drivers

### 1. Scope / Trigger

- Trigger: adding or changing a non-local storage backend such as S3, PikPak,
  WebDAV source, OneDrive, or another third-party cloud drive.
- Goal: register provider capabilities once, then let source/file/VFS/upload/
  task/share/trash/system services consume the same capability bundle.
- Avoid: provider-specific switches spread across multiple services or
  `cmd/server/main.go` manual registrations that can drift.

### 2. Signatures

Storage driver contracts live in `backend/internal/domain/storage/driver.go`:

```go
type SourceDriverProbe interface {
    Test(ctx context.Context, source *entity.StorageSource) error
}

type FileDriver interface {
    List(ctx context.Context, source *entity.StorageSource, virtualPath string) ([]StorageEntry, error)
    SearchByName(ctx context.Context, source *entity.StorageSource, pathPrefix, keyword string) ([]StorageEntry, error)
    Stat(ctx context.Context, source *entity.StorageSource, virtualPath string) (*StorageEntry, error)
    Mkdir(ctx context.Context, source *entity.StorageSource, parentPath, name string) (*StorageEntry, error)
    Rename(ctx context.Context, source *entity.StorageSource, virtualPath, newName string) (*StorageEntry, error)
    Move(ctx context.Context, source *entity.StorageSource, virtualPath, targetPath string) error
    Copy(ctx context.Context, source *entity.StorageSource, virtualPath, targetPath string) error
    Delete(ctx context.Context, source *entity.StorageSource, virtualPath string) error
    PresignDownload(ctx context.Context, source *entity.StorageSource, virtualPath, disposition string, ttl time.Duration) (string, time.Time, error)
}

type UploadDriver interface { /* direct upload / multipart */ }
type ImportDriver interface {
    ImportFile(ctx context.Context, source *entity.StorageSource, targetPath string, localPath string) error
}
type CapacityDriver interface {
    Capacity(ctx context.Context, source *entity.StorageSource) (*CapacityInfo, error)
}
```

Application registration lives in `backend/internal/application/service/`:

```go
type DriverBundle struct {
    Type        string
    DisplayName string
    Config      SourceConfigCodec
    Probe       SourceDriverProbe
    File        FileDriver
    Upload      UploadDriver
    Import      ImportDriver
    Capacity    CapacityDriver
    Capabilities CapabilityProvider

    // Enable only for drivers that are safe to recursively list on request.
    RecursiveStatsFallback bool
}
```

### 3. Contracts

- `DriverBundle.Type` is the stable `storage_sources.driver_type`, e.g. `s3`
  or `pikpak`; it must match the frontend/API `driver_type` value.
- `SourceConfigCodec` is responsible for build/update/detail/audit views for
  one driver type. It must preserve existing secret values on update when a
  `secret_patch` field is omitted.
- `config` contains public/non-sensitive source settings. `secret_patch`
  contains passwords, access keys, refresh tokens, captcha tokens, and similar
  values.
- `FileDriver` paths are always source-internal virtual paths beginning with
  `/`; VFS is responsible for mapping mount path `/mount/...` to inner path
  `/...`.
- `ImportDriver` imports a backend-visible local staging file into the target
  source path. It is used by offline downloads and by upload flows for drivers
  that cannot safely expose direct browser-upload instructions.
- `CapacityDriver` is preferred for source capacity/used bytes. If it returns
  `nil` or `UsedBytes == nil`, system stats may fall back to recursive
  `FileDriver` only when `RecursiveStatsFallback` is explicitly true.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| `driver_type` has no registered `SourceConfigCodec` | Return `ErrSourceDriverUnsupported` / API `SOURCE_DRIVER_UNSUPPORTED` |
| Config field has wrong type or required public/secret field missing | Return `ErrConfigInvalid` or a stable validation sentinel; handler maps to 4xx |
| Source update omits a secret field | Keep the previous stored secret value |
| Source update sets a secret field to `null` | Clear that stored secret value when the codec supports clearing |
| Driver lacks an operation capability | Return a stable unsupported error, not 500 |
| Third-party driver has quota but not recursive-safe listing | Register `CapacityDriver`; do not set `RecursiveStatsFallback` |
| `CapacityDriver` cannot provide `UsedBytes` | Continue to explicit recursive fallback if configured; otherwise count as unknown/0 |

### 5. Good/Base/Bad Cases

- Good: a read-only third-party phase registers only the capabilities it
  actually supports, e.g. PikPak phase B registers `Config`, `Probe`, `File`,
  `Capacity`, and `Capabilities`, but omits `Upload` / `Import`; write and
  import entry points return `SOURCE_OPERATION_UNSUPPORTED`.
- Good: a later writable/import-capable PikPak phase can add `Import`; upload
  then uses `server_chunk -> ImportFile`; system stats still use quota instead
  of recursive listing.
- Base: S3 registers `Config`, `Probe`, `File`, `Upload`, `Import`, and sets
  `RecursiveStatsFallback=true` to preserve existing recursive stats behavior.
- Bad: adding `if driverType == "pikpak"` branches in `SourceService`,
  `UploadService`, `TaskService`, and `cmd/server/main.go` instead of adding a
  single driver bundle and codec.

### 6. Tests Required

When adding or changing a storage driver bundle, add/update focused backend
tests that assert:

- Source create/test/detail uses the codec, masks secrets by default, and shows
  secrets only with `source.secret.read`.
- The registry wires every intended capability into Source/File/VFS/Trash/
  Upload/Task/Share/System services and omits unsupported capabilities (for
  example read-only PikPak must not be registered as Upload/Task Import).
- Existing S3 direct-upload behavior still returns `transport.mode=direct_parts`.
- Import-only non-local drivers return `transport.mode=server_chunk` and call
  `ImportFile` on finish.
- System stats use `CapacityDriver` before recursive `FileDriver`, and only use
  recursive fallback when explicitly enabled.

Run from `backend/`:

```bash
go test ./...
```

### 7. Wrong vs Correct

#### Wrong

```go
// Scattered provider registration: easy to forget one service.
sourceSvc := NewSourceService(..., WithSourceDriverProbe("pikpak", pikpak))
fileSvc := NewFileService(..., WithFileDriver("pikpak", pikpak))
// Missing VFS/Share/Task/Upload registration becomes a runtime bug.
```

#### Correct

```go
drivers := NewStorageDriverRegistry(DriverBundle{
    Type:         "pikpak",
    Config:       NewPikPakSourceConfigCodec(),
    Probe:        pikpak,
    File:         pikpak,
    Capacity:     pikpak,
    Capabilities: pikpak, // read-only phase: no Upload/Import registration.
})

sourceOpts := append(baseSourceOpts, drivers.SourceServiceOptions()...)
uploadOpts := append(baseUploadOpts, drivers.UploadServiceOptions()...)
taskOpts := append(baseTaskOpts, drivers.TaskServiceOptions()...)
```

## Current Layering Exceptions

Document and preserve these existing seams when reviewing future changes:

- `backend/internal/application/audit/context.go` reads request auth snapshots
  through `internal/infrastructure/security` context helpers.
- `backend/internal/application/audit/recorder.go` and service audit helpers use
  `internal/infrastructure/observability/logging` to recover request loggers.
- `backend/internal/application/service/source_service.go` parses/masks S3
  config through `internal/infrastructure/storage`.
- `backend/internal/infrastructure/downloader/aria2_client.go` imports
  `internal/application/service` to implement the download adapter interface.

These are current project conventions, not a reason to put GORM queries, Gin
contexts, or new external clients directly into application services.

## Downloader / Staging Placement

Downloader adapters live in `internal/infrastructure/downloader/` and implement
the minimal application-layer `Downloader` interface. When multiple downloaders
coexist, keep routing and task orchestration in `application/service` and keep
protocol-specific HTTP/RPC calls in infrastructure.

Tasks must always download into a backend-visible staging directory first, then
import into the resolved VFS/storage target. If different downloader backends use
different shared volumes, configure per-downloader staging roots instead of
letting one downloader's path become the global default for every task type.

## Naming Conventions

- Package names are short lower-case: `service`, `repository`, `gorm`, `handler`, `middleware`.
- Feature files use snake_case with a suffix: `source_service.go`, `vfs_handler.go`, `acl_rule_repo_impl.go`.
- DTO structs use `json:"snake_case"` and Gin `binding` tags when validation is required.
- Repository interfaces live in `internal/domain/repository/<feature>_repo.go`; GORM implementations live in `internal/infrastructure/persistence/gorm/<feature>_repo_impl.go`.

## Good / Base / Bad Cases

- Good: `VFSHandler` binds and maps errors; `VFSService` resolves virtual paths; GORM repos persist/load models.
- Base: a small read-only endpoint may add a DTO, service method, handler method, route, and tests without new domain entities.
- Bad: putting GORM queries in handlers, importing Gin in services, or adding a route without updating `backend/API_CONTRACT.md`.

## Wrong vs Correct

### Wrong

```go
// service layer directly depends on Gin and GORM
type SourceService struct { db *gorm.DB }
func (s *SourceService) Create(c *gin.Context) error { ... }
```

### Correct

```go
type SourceService struct {
    sourceRepo domainrepo.SourceRepository
}

func (s *SourceService) Create(ctx context.Context, req dto.SourceUpsertRequest) (*dto.StorageSourceView, error) {
    // validate + orchestrate; persistence goes through repository interface
}
```

## Examples to Follow

- Full wiring: `backend/cmd/server/main.go`
- Capability-protected route groups: `backend/internal/interfaces/http/router.go`
- Virtual path helpers: `backend/internal/application/service/vfs_path.go`
- Repository entity/model conversion: `backend/internal/infrastructure/persistence/gorm/user_repo_impl.go`
