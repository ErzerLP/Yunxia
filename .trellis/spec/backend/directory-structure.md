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
