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
type NativeDownloadDriver interface {
    CreateNativeDownload(ctx context.Context, source *entity.StorageSource, req NativeDownloadRequest) (*NativeDownloadTask, error)
    GetNativeDownloadStatus(ctx context.Context, source *entity.StorageSource, externalID string) (*NativeDownloadStatus, error)
    CancelNativeDownload(ctx context.Context, source *entity.StorageSource, externalID string, deleteFiles bool) error
    PauseNativeDownload(ctx context.Context, source *entity.StorageSource, externalID string) error
    ResumeNativeDownload(ctx context.Context, source *entity.StorageSource, externalID string) error
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
    NativeDownload NativeDownloadDriver
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
- Third-party path/id caches must be source-scoped and root-scoped. Cache keys
  should include `source_id`, provider root id, and normalized inner path. Any
  provider write/import operation must invalidate the affected source/root
  cache at least conservatively; stale provider ids must not survive writes.
- Third-party HTTP clients may retry transient provider failures such as 429
  and 5xx with bounded backoff. Do not retry user-correctable auth, token,
  config, captcha, not-found, or name-conflict errors. Retry logs must use
  sanitized method/host/path/status/cause fields and must not include request
  bodies, passwords, tokens, OSS secrets, or download URLs.
- If a driver delete operation maps to a provider recycle-bin operation (for
  example PikPak `batchTrash`), expose that through capabilities and document
  that `delete_mode=trash` does not create a Yunxia `.trash` item; do not label
  it as permanent delete.
- `ImportDriver` imports a backend-visible local staging file into the target
  source path. It is used by offline downloads and by upload flows for drivers
  that cannot safely expose direct browser-upload instructions.
- Third-party `UploadDriver` direct upload may require provider-specific
  prerequisites, such as PikPak GCID in `MultipartUploadRequest.ContentHash`.
  If the prerequisite is absent and an `ImportDriver` is also registered,
  `UploadService` must fall back to `server_chunk -> ImportFile` instead of
  failing the upload init request.
- `NativeDownloadDriver` is an optimization for source targets that can create
  provider-side offline download tasks directly, e.g. PikPak URL tasks. It must
  be registered through `DriverBundle`, and TaskService must keep the generic
  staging -> `ImportDriver` path as fallback / non-native behavior. Native task
  status must use the same task states (`pending`, `running`, `completed`,
  `failed`, `canceled`) and must not leak provider task payloads or secrets.
- `CapacityDriver` is preferred for source capacity/used bytes. If it returns
  `nil` or `UsedBytes == nil`, system stats may fall back to recursive
  `FileDriver` only when `RecursiveStatsFallback` is explicitly true.
- WebDAV must not be hard-coded to local physical paths. Local sources may use
  `webdav.Dir`, but non-local sources exposed via `is_webdav_exposed=true`
  should route through FileService/UploadService and registered drivers:
  `PROPFIND` uses list/stat, `GET/HEAD` returns provider redirect,
  `MKCOL/DELETE/COPY/MOVE` use file service mutations, and `PUT` imports a
  backend temp file through `ImportDriver`.

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
- Good: a writable-but-not-import-capable third-party phase can expose
  `Mkdir` / `Rename` / `Move` / `Copy` / `Delete` through `FileDriver` and
  `Capabilities` while still omitting `Upload` / `Import`; upload and task
  import entry points return `SOURCE_OPERATION_UNSUPPORTED`.
- Good: an import-capable PikPak phase registers `Import` but not `Upload`;
  upload then uses `server_chunk -> ImportFile`; offline/RSS/BT tasks import
  their backend-visible staging files through the same `ImportDriver`; system
  stats still use quota instead of recursive listing.
- Good: a native-download-capable PikPak phase additionally registers
  `NativeDownload`; TaskService chooses it only when the resolved target source
  itself is PikPak, stores `downloader_type=pikpak_native`, and skips local
  staging/import because the provider writes directly into the target folder.
  Non-PikPak targets and unsupported native links continue through the generic
  downloader staging path.
- Good: a direct-upload-capable PikPak phase registers both `Upload` and
  `Import`; `UploadDriver` returns `direct_parts` only when the frontend
  supplied a valid GCID via `file_hash`, returns a fast-upload entry when the
  provider seconds the file, and returns unsupported for missing/invalid GCID
  so `UploadService` can fall back to stable backend `server_chunk` import.
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
  Upload/Task/Share/System services and omits unsupported capabilities. PikPak
  direct-upload stages must register both `Upload` and `Import` so GCID uploads
  can use `direct_parts` while non-GCID uploads still use `server_chunk`.
- Existing S3 direct-upload behavior still returns `transport.mode=direct_parts`.
- Import-only non-local drivers return `transport.mode=server_chunk`, do not
  return direct part instructions, and call `ImportFile` on finish. Task
  completion must also call `ImportFile` and clean the staging directory.
- Direct-upload-capable non-local drivers return `transport.mode=direct_parts`,
  persist driver state in the upload session, pass `ContentHash` through to the
  driver, call `CompleteMultipartUpload` on finish, and fall back to
  `server_chunk` when the driver returns unsupported but an import driver
  exists. Fast-upload plans return `is_fast_upload=true` without creating an
  upload session.
- WebDAV non-local tests cover `PROPFIND`, `GET/HEAD` redirect, write methods
  delegating to FileService/UploadService, read-only source rejection, and no
  direct dependency on provider-specific clients from the HTTP handler.
- Native-download driver tests cover create/status/cancel mapping, provider
  target folder resolution, no local staging/import for native PikPak targets,
  `downloader_type=pikpak_native`, and unsupported pause/resume returning a
  stable unsupported error.
- Path cache tests cover both repeated reads avoiding redundant provider
  resolution and write/import operations invalidating cached paths.
- Provider retry tests cover transient failures eventually succeeding, request
  bodies being recreated for retries, and auth/token errors not being retried.
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
    Upload:       pikpak,
    Import:       pikpak,
    NativeDownload: pikpak,
    Capacity:     pikpak,
    Capabilities: pikpak,
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

Tasks normally download into a backend-visible staging directory first, then
import into the resolved VFS/storage target. If different downloader backends use
different shared volumes, configure per-downloader staging roots instead of
letting one downloader's path become the global default for every task type.
The exception is a registered `NativeDownloadDriver` whose resolved target
source is the same provider (currently PikPak): provider native tasks may write
directly into the target folder, store `downloader_type=pikpak_native`, and skip
staging/import. Do not use provider native downloads for unrelated targets unless
an explicit temp-provider transfer flow is implemented and tested.

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
