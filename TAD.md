# 技术架构设计文档 (TAD) —— 云匣 (Yunxia)

> **版本**: v1.0  
> **日期**: 2026-04-20  
> **对应 PRD**: v1.0  
> **架构范式**: DDD (Domain-Driven Design) 分层架构  
> **文档职责**: 架构决策、边界约束、接口真值表、部署策略  
> **依赖抽象参考**: `INTERFACE-ARCHITECTURE.md`  
> **设计目标**: 轻量、可扩展、高安全、支持 1-10 万文件量级

---

## 目录

1. [架构设计原则](#1-架构设计原则)
2. [技术栈选型](#2-技术栈选型)
3. [分层架构总览](#3-分层架构总览)
4. [领域层 (Domain Layer)](#4-领域层-domain-layer)
5. [应用层 (Application Layer)](#5-应用层-application-layer)
6. [基础设施层 (Infrastructure Layer)](#6-基础设施层-infrastructure-layer)
7. [接口适配层 (Interface Adapters)](#7-接口适配层-interface-adapters)
8. [数据架构](#8-数据架构)
9. [安全架构](#9-安全架构)
10. [高并发架构](#10-高并发架构)
11. [关键业务流程](#11-关键业务流程)
12. [配置体系](#12-配置体系)
13. [部署架构](#13-部署架构)
14. [附录](#14-附录)

---

## 1. 架构设计原则

### 1.1 核心原则

| 原则 | 说明 | 实践 |
|------|------|------|
| **依赖倒置 (DIP)** | 高层模块不依赖低层模块，两者依赖抽象 | 领域层定义 Repository 接口，基础设施层实现 |
| **关注点分离 (SoC)** | 业务逻辑与技术细节分离 | 领域层纯 Go 标准库，无框架依赖 |
| **可测试性** | 每层可独立单元测试 | Service 通过 Mock Repository 测试 |
| **可替换性** | 组件可插拔，不绑定实现 | SQLite ↔ PostgreSQL、bigcache ↔ Redis 切换只需替换实现 |
| **无状态设计** | 服务端不保存会话状态 | JWT 认证，任何实例可处理任意请求 |
| **防御性设计** | 安全从架构层开始 | 路径遍历防护、默认拒绝、最小权限 |

### 1.2 依赖方向

```
接口适配层 (HTTP/WebDAV)
    │ 依赖
    ▼
应用层 (Application Services)
    │ 依赖
    ▼
领域层 (Entities / Domain Services / Repository IFs)
    ▲ 依赖（通过接口）
    │
基础设施层 (Persistence / Drivers / Cache / MQ)
```

**关键规则**：
- 领域层不依赖任何外部包（框架、数据库、HTTP）
- 应用层只依赖领域层
- 基础设施层依赖领域层（实现其接口）
- 接口层依赖应用层和领域层

---

## 2. 技术栈选型

### 2.1 后端技术栈

| 组件 | 选型 | 版本 | 选型理由 |
|------|------|------|---------|
| **语言** | Go | 1.24+ | goroutine 适合 I/O 密集文件操作，单二进制部署，跨平台 |
| **Web 框架** | Gin | v1.10+ | 性能优秀，中间件生态成熟，AList/Cloudreve 验证 |
| **ORM** | GORM | v2 | 中文社区巨大，AutoMigrate 适合快速迭代，文档完善 |
| **数据库** | SQLite / PostgreSQL | 3.46+ / 16+ | SQLite 零配置适合个人；PostgreSQL 适合高并发 |
| **缓存** | bigcache / ristretto | v3 / v0.2 | 高性能内存缓存，零 GC 压力，接口可切换 Redis |
| **日志** | zap + lumberjack | v1.27+ | 高性能结构化日志，自动切割压缩 |
| **JWT** | golang-jwt/jwt | v5 | 标准 JWT 库，支持自定义 Claims |
| **密码哈希** | bcrypt (golang.org/x/crypto) | - | 行业标准，Go 标准扩展库 |
| **限流** | golang.org/x/time/rate | - | 官方令牌桶实现 |
| **WebDAV** | golang.org/x/net/webdav | - | 官方标准库，稳定可控 |
| **任务调度** | robfig/cron | v3 | 标准定时任务库 |
| **配置** | viper | v1.19+ | 支持 YAML/JSON/环境变量多源配置 |
| **UUID** | google/uuid | v1.6+ | 标准 UUID 生成 |
| **校验** | go-playground/validator | v10 | 配合 Gin 绑定校验 |

### 2.2 前端技术栈

| 组件 | 选型 | 版本 | 选型理由 |
|------|------|------|---------|
| **框架** | React | 18+ | 生态成熟，组件丰富 |
| **语言** | TypeScript | 5.6+ | 类型安全，IDE 体验好 |
| **构建** | Vite | 6+ | 快速冷启动，现代化 HMR |
| **样式** | Tailwind CSS | 3.4+ | 原子化 CSS，开发效率高，包体积可控 |
| **UI 组件** | shadcn/ui | latest | 轻量现代，源码完全可控，基于 Tailwind |
| **状态管理** | Zustand | 5+ | 轻量，比 Redux 简单，比 Context 性能好 |
| **路由** | React Router | v7+ | 标准选择 |
| **请求** | Axios + TanStack Query | v1+ / v5+ | 缓存、重试、去重开箱即用 |
| **图标** | Lucide React | latest | 轻量、现代、树摇优化 |
| **虚拟滚动** | @tanstack/react-virtual | v3 | 大目录性能优化 |
| **分片上传** | spark-md5 (Web Worker) | v3 | 浏览器 MD5 计算，支持 Web Worker |

### 2.3 基础设施

| 组件 | 选型 | 说明 |
|------|------|------|
| **容器** | Docker | 单容器部署 |
| **编排** | Docker Compose | 云匣 + Aria2 组合部署 |
| **CI/CD** | GitHub Actions | 自动构建、测试、推送镜像 |
| **镜像仓库** | GitHub Container Registry / Docker Hub | 官方镜像分发 |
| **下载器** | Aria2 | JSON-RPC 接口，支持 HTTP/BT/Magnet |

---

## 3. 分层架构总览

> 注：本文档采用 DDD 经典**四层架构**（接口适配层 / 应用层 / 领域层 / 基础设施层），"三层"为习惯性简称。

### 3.1 系统架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              客户端层                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │  Web     │  │ WebDAV   │  │  Mobile  │  │  Desktop │  │  Aria2   │ │
│  │  Browser │  │  Client  │  │  (PWA)   │  │ (Future) │  │  Client  │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘ │
└───────┼─────────────┼─────────────┼─────────────┼─────────────┼─────────┘
        │             │             │             │             │
        └─────────────┴──────┬──────┴─────────────┘             │
                             │                                    │
┌────────────────────────────┼────────────────────────────────────┼──────┐
│                     接口适配层 (Interface Adapters)               │      │
│  ┌─────────────────────────┼────────────────────────────────┐   │      │
│  │    REST API Router      │       WebDAV Handler           │   │      │
│  │    (/api/v1/*)          │       (/dav/*)                 │   │      │
│  │  ┌────────┬────────┐   │   ┌─────────────────────────┐   │   │      │
│  │  │ Auth   │ File   │   │   │  DAV FileSystem Wrapper │   │   │      │
│  │  │Handler │Handler │   │   │  (Driver → webdav.File) │   │   │      │
│  │  ├────────┼────────┤   │   └─────────────────────────┘   │   │      │
│  │  │ Upload │ Source │   │                                   │   │      │
│  │  │Handler │Handler │   │                                   │   │      │
│  │  ├────────┼────────┤   │                                   │   │      │
│  │  │ Task   │ System │   │                                   │   │      │
│  │  │Handler │Handler │   │                                   │   │      │
│  │  └────────┴────────┘   └───────────────────────────────────┘   │      │
│  │                                                                 │      │
│  │  Middleware: JWT Auth / Rate Limit / CORS / Security / Logger   │      │
│  └─────────────────────────────────────────────────────────────────┘      │
├───────────────────────────────────────────────────────────────────────────┤
│                          应用层 (Application Layer)                        │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  AuthAppSvc │ FileAppSvc │ UploadAppSvc │ TaskAppSvc │ ShareAppSvc  │  │
│  │  DTOs (Request/Response)                                           │  │
│  │  Use Cases: Login / ListFiles / UploadChunk / SubmitDownload ...    │  │
│  │  Assembler: DTO ↔ Entity 转换                                      │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
├───────────────────────────────────────────────────────────────────────────┤
│                            领域层 (Domain Layer)                           │
│  ┌──────────────┬──────────────┬───────────────────────────────────────┐  │
│  │   Entities   │  Repository  │      Domain Services                  │  │
│  │  User        │  Interfaces  │  ┌─────────────────────────────────┐  │  │
│  │  FileMetadata│  (UserRepo   │  │ ACLService                      │  │  │
│  │  StorageSrc  │   FileRepo   │  │  - CheckPermission()            │  │  │
│  │  UploadSess  │   SourceRepo │  │  - Path hierarchy resolution    │  │  │
│  │  Task        │   UploadRepo │  │  - Rule priority sorting        │  │  │
│  │  ACLEntry    │   TaskRepo   │  └─────────────────────────────────┘  │  │
│  │  Share       │   ACLRepo    │  ┌─────────────────────────────────┐  │  │
│  │              │   ShareRepo  │  │ AuthDomainService               │  │  │
│  │  Value       │              │  │  - Password hashing             │  │  │
│  │  Objects     │              │  │  - Token version management     │  │  │
│  │  (FileInfo,  │              │  │  - Default ACL generation       │  │  │
│  │   Pagination,│              │  └─────────────────────────────────┘  │  │
│  │   Permission)│              │                                       │  │
│  └──────────────┴──────────────┴───────────────────────────────────────┘  │
├───────────────────────────────────────────────────────────────────────────┤
│                        基础设施层 (Infrastructure Layer)                    │
│  ┌──────────────┬──────────────┬──────────────┬────────────────────────┐  │
│  │ Persistence  │   Storage    │    Cache     │    Task Queue          │  │
│  │ GORM Impl    │   Drivers    │ bigcache /   │    SQLite-based        │  │
│  │ (SQLite/PG)  │ ┌──────────┐ │ ristretto    │    Go channel + worker │  │
│  │              │ │ Local    │ │ (Interface)  │    pool                │  │
│  │ Migration    │ │ S3       │ │              │                        │  │
│  │ FTS5 Index   │ │ OneDrive │ │              │                        │  │
│  └──────────────┤ └──────────┘ └──────────────┴────────────────────────┘  │
│                 │                                                          │
│  ┌──────────────┼───────────────────────────────────────────────────────┐  │
│  │ Downloader   │  Aria2 JSON-RPC Client                                │  │
│  │ (Interface)  │  AddURI / TellStatus / Pause / Resume / Remove        │  │
│  └──────────────┴───────────────────────────────────────────────────────┘  │
│                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Config (Viper) │ Logger (Zap) │ Validator │ Utils │ Security       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 目录结构

```
yunxia/
├── cmd/server/
│   └── main.go                      # 程序入口：依赖注入、服务启动
│
├── internal/
│   ├── domain/                      # ━━ 领域层 ━━
│   │   ├── entity/                  # 实体
│   │   │   ├── user.go
│   │   │   ├── storage_source.go
│   │   │   ├── file_metadata.go
│   │   │   ├── upload_session.go
│   │   │   ├── task.go
│   │   │   ├── acl_entry.go
│   │   │   └── share.go
│   │   ├── valueobject/             # 值对象
│   │   │   ├── file_info.go
│   │   │   ├── pagination.go
│   │   │   └── permission.go
│   │   ├── repository/              # 仓库接口（领域层定义）
│   │   │   ├── user_repo.go
│   │   │   ├── file_repo.go
│   │   │   ├── source_repo.go
│   │   │   ├── upload_repo.go
│   │   │   ├── task_repo.go
│   │   │   ├── acl_repo.go
│   │   │   └── share_repo.go
│   │   └── service/                 # 领域服务
│   │       ├── acl_service.go
│   │       └── auth_domain_service.go
│   │
│   ├── application/                 # ━━ 应用层 ━━
│   │   ├── dto/                     # 数据传输对象
│   │   │   ├── auth_dto.go
│   │   │   ├── file_dto.go
│   │   │   ├── source_dto.go
│   │   │   ├── upload_dto.go
│   │   │   ├── task_dto.go
│   │   │   └── share_dto.go
│   │   ├── service/                 # 应用服务
│   │   │   ├── auth_app_svc.go
│   │   │   ├── file_app_svc.go
│   │   │   ├── upload_app_svc.go
│   │   │   ├── task_app_svc.go
│   │   │   └── share_app_svc.go
│   │   └── assembler/               # DTO ↔ Entity 转换
│   │       └── file_assembler.go
│   │
│   ├── interfaces/                  # ━━ 接口适配层 ━━
│   │   ├── http/
│   │   │   ├── handler/             # REST API Handler
│   │   │   │   ├── auth_handler.go
│   │   │   │   ├── file_handler.go
│   │   │   │   ├── upload_handler.go
│   │   │   │   ├── source_handler.go
│   │   │   │   ├── task_handler.go
│   │   │   │   └── system_handler.go
│   │   │   └── router.go            # 路由注册
│   │   ├── webdav/                  # WebDAV Handler
│   │   │   ├── dav_fs.go
│   │   │   ├── dav_handler.go
│   │   │   └── dav_lock.go
│   │   └── middleware/              # 中间件
│   │       ├── auth_mw.go
│   │       ├── rate_limit.go
│   │       ├── cors.go
│   │       ├── security.go
│   │       └── logger.go
│   │
│   └── infrastructure/              # ━━ 基础设施层 ━━
│       ├── persistence/             # 持久化实现
│       │   ├── gorm/
│       │   │   ├── db.go
│       │   │   ├── user_repo_impl.go
│       │   │   ├── file_repo_impl.go
│       │   │   ├── source_repo_impl.go
│       │   │   ├── upload_repo_impl.go
│       │   │   ├── task_repo_impl.go
│       │   │   ├── acl_repo_impl.go
│       │   │   └── share_repo_impl.go
│       │   └── migration/
│       │       └── migration.go
│       ├── storage/                 # 存储驱动
│       │   ├── driver.go            # Driver 接口
│       │   ├── local/
│       │   ├── s3/
│       │   ├── onedrive/
│       │   └── registry.go
│       ├── cache/                   # 缓存
│       │   ├── cache.go             # 接口
│       │   └── bigcache_impl.go
│       ├── mq/                      # 任务队列
│       │   ├── queue.go             # 接口
│       │   └── sqlite_queue.go
│       ├── downloader/              # 下载器客户端
│       │   ├── downloader.go        # 接口
│       │   └── aria2_client.go
│       ├── config/
│       │   └── config.go
│       └── pkg/
│           ├── logger/
│           ├── validator/
│           └── utils/
│
├── pkg/                             # 可对外暴露
│   └── driver/
│       └── driver.go                # 第三方驱动开发接口
│
├── web/                             # 前端项目
│   ├── src/
│   ├── public/
│   ├── package.json
│   ├── vite.config.ts
│   └── tailwind.config.js
│
├── migrations/
├── scripts/
├── .github/workflows/
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod
└── README.md
```

---

## 4. 领域层 (Domain Layer)

领域层是系统核心，包含业务实体、值对象、领域服务和仓库接口。领域层**不依赖任何外部框架或库**。

### 4.1 实体 (Entities)

#### User

```go
type User struct {
    ID           uint
    Username     string
    PasswordHash string
    Email        string
    Role         Role           // admin / user / guest
    Status       UserStatus     // active / disabled
    StorageQuota int64          // 0 = 无限制
    TokenVersion int            // Token 撤销版本号
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

func (u *User) IsAdmin() bool
func (u *User) CanLogin() bool
func (u *User) UpdateTokenVersion()
```

#### StorageSource

```go
type StorageSource struct {
    ID              uint
    Name            string
    DriverType      DriverType     // local / s3 / onedrive
    Config          map[string]interface{}
    RootPath        string
    IsEnabled       bool
    IsWebDAVExposed bool
    WebDAVReadOnly  bool
    SortOrder       int
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

#### FileMetadata

```go
type FileMetadata struct {
    ID         uint
    SourceID   uint
    Path       string
    Name       string
    ParentPath string
    Size       int64
    IsDir      bool
    MimeType   string
    Checksum   string
    ModifiedAt time.Time
    CreatedAt  time.Time
    CachedAt   time.Time
    Extra      map[string]interface{}
}
```

#### UploadSession

```go
type UploadSession struct {
    ID              string         // UUID
    UserID          uint
    SourceID        uint
    TargetPath      string
    Filename        string
    FileSize        int64
    FileHash        string         // MD5
    Status          UploadStatus   // pending / uploading / completed / failed / cancelled
    ChunkSize       int64          // 默认 5MB
    TotalChunks     int
    UploadedChunks  int
    CompletedChunks []int
    StorageData     string         // JSON
    ExpiresAt       time.Time      // 7 天
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

func (u *UploadSession) IsExpired() bool
func (u *UploadSession) IsChunkUploaded(index int) bool
func (u *UploadSession) MarkChunkUploaded(index int)
func (u *UploadSession) IsComplete() bool
```

#### Task

```go
type Task struct {
    ID          string
    Type        TaskType       // download / index_scan / cleanup
    Status      TaskStatus     // pending / running / completed / failed / cancelled
    Payload     string         // JSON
    Result      string
    ErrorMsg    string
    Priority    int
    ScheduledAt *time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    RetryCount  int
    MaxRetries  int
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

func (t *Task) CanRetry() bool
```

#### ACLEntry

```go
type ACLEntry struct {
    ID       uint
    UserID   uint
    SourceID uint
    Path     string         // / 表示存储源根
    Read     bool
    Write    bool
    Delete   bool
    Share    bool
    RuleType RuleType       // allow / deny
    Inherit  bool           // 子目录继承
    Priority int
    CreatedAt time.Time
    UpdatedAt time.Time
}

func (a *ACLEntry) HasPermission(p Permission) bool
```

#### Share

```go
type Share struct {
    ID             uint
    UserID         uint
    SourceID       uint
    Path           string
    Token          string         // 随机 32 字节
    PasswordHash   string         // bcrypt
    ExpiresAt      *time.Time
    MaxDownloads   int            // 0 = 无限制
    DownloadCount  int
    IsEnabled      bool
    CreatedAt      time.Time
}
```

### 4.2 值对象 (Value Objects)

```go
type FileInfo struct {
    Path       string
    Name       string
    Size       int64
    IsDir      bool
    ModifiedAt time.Time
    CreatedAt  time.Time
    MimeType   string
    Extra      map[string]interface{}
}

type Pagination struct {
    Page       int
    PageSize   int
    Total      int64
    TotalPages int
}

type Permission struct {
    Read   bool
    Write  bool
    Delete bool
    Share  bool
}
```

### 4.3 领域服务 (Domain Services)

#### ACLService — 权限计算核心

```go
type ACLService struct {
    aclRepo repository.ACLRepository
}

// CheckPermission 检查用户对某路径的权限
// 算法：
//   1. 生成路径层级列表（从具体到根）
//   2. 查询所有适用的 ACL 规则
//   3. 按优先级排序
//   4. 取最匹配的规则
//   5. 无匹配则默认拒绝
func (s *ACLService) CheckPermission(
    userID uint, sourceID uint, path string, perm Permission,
) (bool, error)
```

#### AuthDomainService — 认证领域逻辑

```go
type AuthDomainService struct {
    hasher PasswordHasher  // 接口，基础设施实现
}

func (s *AuthDomainService) HashPassword(password string) (string, error)
func (s *AuthDomainService) ValidatePassword(user *User, password string) bool
func (s *AuthDomainService) GenerateDefaultACL(userID, sourceID uint) *ACLEntry
```

### 4.4 仓库接口 (Repository Interfaces)

```go
// UserRepository
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id uint) error
    FindByID(ctx context.Context, id uint) (*User, error)
    FindByUsername(ctx context.Context, username string) (*User, error)
    FindByUsernameWithPassword(ctx context.Context, username string) (*User, error)
    List(ctx context.Context, page, pageSize int) ([]*User, int64, error)
    Exists(ctx context.Context, username string) (bool, error)
    Count(ctx context.Context) (int64, error)
}

// FileRepository
type FileRepository interface {
    Save(ctx context.Context, meta *FileMetadata) error
    Delete(ctx context.Context, sourceID uint, path string) error
    FindByPath(ctx context.Context, sourceID uint, path string) (*FileMetadata, error)
    ListByParent(ctx context.Context, sourceID uint, parentPath string, page, pageSize int) ([]*FileMetadata, int64, error)
    SearchByName(ctx context.Context, sourceID uint, keyword string, page, pageSize int) ([]*FileMetadata, int64, error)
    DeleteBySource(ctx context.Context, sourceID uint) error
}

// StorageSourceRepository
type StorageSourceRepository interface {
    Create(ctx context.Context, source *StorageSource) error
    Update(ctx context.Context, source *StorageSource) error
    Delete(ctx context.Context, id uint) error
    FindByID(ctx context.Context, id uint) (*StorageSource, error)
    ListByUser(ctx context.Context, userID uint) ([]*StorageSource, error)
    ListAll(ctx context.Context) ([]*StorageSource, error)
    ListWebDAVExposed(ctx context.Context) ([]*StorageSource, error)
}

// UploadRepository
type UploadRepository interface {
    Create(ctx context.Context, session *UploadSession) error
    Update(ctx context.Context, session *UploadSession) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*UploadSession, error)
    FindByHash(ctx context.Context, sourceID uint, hash string) (*UploadSession, error)
    ListExpired(ctx context.Context) ([]*UploadSession, error)
}

// TaskRepository
type TaskRepository interface {
    Create(ctx context.Context, task *Task) error
    Update(ctx context.Context, task *Task) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*Task, error)
    FindPending(ctx context.Context, limit int) ([]*Task, error)
    FindByStatus(ctx context.Context, status TaskStatus, page, pageSize int) ([]*Task, int64, error)
    FindScheduledBefore(ctx context.Context, t time.Time, limit int) ([]*Task, error)
}

// ACLRepository
type ACLRepository interface {
    Create(ctx context.Context, entry *ACLEntry) error
    Update(ctx context.Context, entry *ACLEntry) error
    Delete(ctx context.Context, id uint) error
    FindByID(ctx context.Context, id uint) (*ACLEntry, error)
    FindByUserAndSource(ctx context.Context, userID, sourceID uint) ([]*ACLEntry, error)
    FindByUserAndSourceAndPath(ctx context.Context, userID, sourceID uint, path string) ([]*ACLEntry, error)
    DeleteByUserAndSource(ctx context.Context, userID, sourceID uint) error
}

// ShareRepository
type ShareRepository interface {
    Create(ctx context.Context, share *Share) error
    Delete(ctx context.Context, id uint) error
    FindByToken(ctx context.Context, token string) (*Share, error)
    FindByUser(ctx context.Context, userID uint, page, pageSize int) ([]*Share, int64, error)
    IncrementDownloadCount(ctx context.Context, id uint) error
}
```

---

## 5. 应用层 (Application Layer)

应用层负责**编排领域对象**完成具体用例，不包含业务规则。

### 5.1 DTO (Data Transfer Objects)

```go
// Auth DTOs
type LoginRequest struct {
    Username string `json:"username" binding:"required,min=3,max=64"`
    Password string `json:"password" binding:"required,min=8"`
}

type LoginResponse struct {
    AccessToken  string   `json:"access_token"`
    RefreshToken string   `json:"refresh_token"`
    ExpiresIn    int      `json:"expires_in"`
    User         UserInfo `json:"user"`
}

type RefreshTokenRequest struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}

// File DTOs
type ListFilesRequest struct {
    SourceID  uint   `form:"source_id" binding:"required"`
    Path      string `form:"path" binding:"required"`
    Page      int    `form:"page,default=1"`
    PageSize  int    `form:"page_size,default=200"`
    SortBy    string `form:"sort_by,default=modified_at"`
    SortOrder string `form:"sort_order,default=desc"`
}

type ListFilesResponse struct {
    Items      []FileItem `json:"items"`
    Pagination Pagination `json:"pagination"`
}

type FileItem struct {
    Name       string `json:"name"`
    Path       string `json:"path"`
    Size       int64  `json:"size"`
    IsDir      bool   `json:"is_dir"`
    MimeType   string `json:"mime_type"`
    ModifiedAt string `json:"modified_at"`
}

// Upload DTOs
type UploadInitRequest struct {
    SourceID uint   `json:"source_id" binding:"required"`
    Path     string `json:"path" binding:"required"`
    Filename string `json:"filename" binding:"required"`
    FileSize int64  `json:"file_size" binding:"required,min=1"`
    FileHash string `json:"file_hash"` // MD5
}

type UploadInitResponse struct {
    UploadID       string     `json:"upload_id"`
    ChunkSize      int64      `json:"chunk_size"`
    TotalChunks    int        `json:"total_chunks"`
    UploadedChunks []int      `json:"uploaded_chunks"`
    PresignedURLs  []ChunkURL `json:"presigned_urls,omitempty"`
    IsFastUpload   bool       `json:"is_fast_upload"`
}

type ChunkURL struct {
    Index int    `json:"index"`
    URL   string `json:"url"`
}

// Task DTOs
type CreateDownloadTaskRequest struct {
    URL        string `json:"url" binding:"required"`
    SourceID   uint   `json:"source_id" binding:"required"`
    SavePath   string `json:"save_path" binding:"required"`
}
```

### 5.2 应用服务 (Application Services)

```go
// FileApplicationService
type FileApplicationService struct {
    fileRepo    repository.FileRepository
    sourceRepo  repository.StorageSourceRepository
    aclService  *service.ACLService
    driverMgr   *storage.DriverManager
}

func (s *FileApplicationService) ListFiles(ctx context.Context, userID uint, req dto.ListFilesRequest) (*dto.ListFilesResponse, error)
func (s *FileApplicationService) DownloadFile(ctx context.Context, userID uint, sourceID uint, path string) (redirectURL string, err error)

// UploadApplicationService
type UploadApplicationService struct {
    uploadRepo repository.UploadRepository
    sourceRepo repository.StorageSourceRepository
    fileRepo   repository.FileRepository
    driverMgr  *storage.DriverManager
}

func (s *UploadApplicationService) InitUpload(ctx context.Context, userID uint, req dto.UploadInitRequest) (*dto.UploadInitResponse, error)
func (s *UploadApplicationService) UploadChunk(ctx context.Context, uploadID string, index int, reader io.Reader) error
func (s *UploadApplicationService) FinishUpload(ctx context.Context, uploadID string) error

// TaskApplicationService
type TaskApplicationService struct {
    taskRepo   repository.TaskRepository
    queue      mq.TaskQueue
    downloader downloader.Downloader
}

func (s *TaskApplicationService) CreateDownloadTask(ctx context.Context, userID uint, req dto.CreateDownloadTaskRequest) (*entity.Task, error)
func (s *TaskApplicationService) ListTasks(ctx context.Context, status entity.TaskStatus, page, pageSize int) ([]*entity.Task, int64, error)
func (s *TaskApplicationService) CancelTask(ctx context.Context, taskID string) error
```

---

## 6. 基础设施层 (Infrastructure Layer)

### 6.1 持久化实现 (GORM)

#### 数据库连接

```go
func NewDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
    var dialector gorm.Dialector
    switch cfg.Type {
    case "sqlite":
        dialector = sqlite.Open(cfg.DSN + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
    case "postgresql":
        dialector = postgres.Open(cfg.DSN)
    }
    
    db, err := gorm.Open(dialector, &gorm.Config{Logger: gormLogger})
    if err != nil {
        return nil, err
    }
    
    sqlDB, _ := db.DB()
    sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)    // SQLite: 10, PG: 100
    sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)     // SQLite: 5, PG: 10
    sqlDB.SetConnMaxLifetime(time.Hour)
    
    return db, nil
}
```

#### 仓库实现模式

所有仓库实现遵循统一模式：
- 接收 `*gorm.DB` 作为依赖
- 使用 `db.WithContext(ctx)` 传递上下文
- 返回 `gorm.ErrRecordNotFound` 时转换为 `nil` 值

### 6.2 存储驱动 (Storage Drivers)

#### Driver 接口

```go
type Driver interface {
    Init(config map[string]interface{}) error
    Name() string
    List(ctx context.Context, path string) ([]FileInfo, error)
    Get(ctx context.Context, path string) (*FileInfo, error)
    MakeDir(ctx context.Context, path string) error
    Put(ctx context.Context, path string, reader io.Reader, size int64) error
    GetReader(ctx context.Context, path string) (io.ReadCloser, error)
    GetURL(ctx context.Context, path string, expiry time.Duration) (string, error)
    Delete(ctx context.Context, path string) error
    Move(ctx context.Context, src, dst string) error
    Copy(ctx context.Context, src, dst string) error
}
```

#### Driver 注册机制

```go
var registry = make(map[string]func() Driver)

func Register(name string, factory func() Driver) {
    registry[name] = factory
}

// 各驱动包的 init() 中注册
func init() {
    Register("local", func() Driver { return &LocalDriver{} })
    Register("s3", func() Driver { return &S3Driver{} })
    Register("onedrive", func() Driver { return &OneDriveDriver{} })
}
```

#### DriverManager

```go
type DriverManager struct {
    drivers map[string]Driver // 缓存已初始化的驱动
    mu      sync.RWMutex
}

func (m *DriverManager) GetDriver(driverType string, config map[string]interface{}) (Driver, error)
```

### 6.3 缓存 (Cache)

#### 缓存接口

```go
type Cache interface {
    Get(key string) (interface{}, bool)
    Set(key string, value interface{}, ttl time.Duration) bool
    Delete(key string)
    Flush()
}
```

#### 实现

- **MVP**: `bigcache` 或 `ristretto`（进程内存）
- **分布式扩展**: `RedisCache`（通过相同接口替换）

#### 缓存 Key 设计

| Key 模式 | 内容 | TTL |
|---------|------|-----|
| `file:list:{source_id}:{path}` | 目录列表 | 5min |
| `file:info:{source_id}:{path}` | 单文件元数据 | 5min |
| `dav:propfind:{source_id}:{path}` | WebDAV PROPFIND 结果 | 30s |
| `user:{user_id}` | 用户信息 | 10min |
| `source:{source_id}` | 存储源配置 | 10min |
| `acl:{user_id}:{source_id}:{path}` | ACL 权限结果 | 1min |

### 6.4 任务队列 (Task Queue)

#### 队列接口

```go
type TaskQueue interface {
    Submit(ctx context.Context, task *entity.Task) error
    RegisterHandler(taskType string, handler TaskHandler)
    Start(ctx context.Context) error
    Stop()
}

type TaskHandler func(ctx context.Context, task *entity.Task) error
```

#### SQLite 持久化实现

```go
type SQLiteTaskQueue struct {
    db       *gorm.DB
    repo     repository.TaskRepository
    handlers map[string]TaskHandler
    workers  int
    stopCh   chan struct{}
    wg       sync.WaitGroup
}
```

**工作模式**：
- 启动时从数据库恢复未完成的 pending 任务
- 每个 worker 每 5 秒轮询一次数据库
- 任务执行完成后更新状态
- 失败任务自动重试（最多 3 次，间隔递增）

### 6.5 下载器抽象与 Aria2 默认实现

```go
type Downloader interface {
    AddURI(ctx context.Context, uris []string, options map[string]interface{}) (string, error)
    TellStatus(ctx context.Context, gid string) (*DownloadStatus, error)
    Pause(ctx context.Context, gid string) error
    Resume(ctx context.Context, gid string) error
    Remove(ctx context.Context, gid string) error
}

type Aria2Client struct {
    rpcURL string
    secret string
    client *http.Client
}

var _ Downloader = (*Aria2Client)(nil)
```

**默认实现**: Aria2 JSON-RPC over HTTP POST  
**可替换实现**: qBittorrent / Transmission（保持 `Downloader` 接口不变）

### 6.6 日志 (Zap)

```go
type LoggerConfig struct {
    Level      string // debug / info / warn / error
    Format     string // json / text
    Output     string // stdout / file / both
    Dir        string
    MaxSize    int    // MB
    MaxAge     int    // days
    MaxBackups int
}
```

**日志分类**：

| 分类 | 级别 | 文件 | 保留期 |
|------|------|------|--------|
| access | info | app.log | 30 天 |
| app | info | app.log | 30 天 |
| auth | warn | app.log | 90 天 |
| audit | info | audit.log | 180 天 |
| webdav | info | webdav.log | 30 天 |
| error | error | error.log | 90 天 |

---

## 7. 接口适配层 (Interface Adapters)

### 7.1 REST API 路由

```
/api/v1
├── /auth
│   ├── POST /login                    # 登录
│   ├── POST /refresh                  # 刷新 Token
│   └── POST /logout                   # 登出
│
├── /auth/me (需认证)
│   └── GET                            # 获取当前用户
│
├── /files (需认证)
│   ├── GET                            # 列出文件
│   ├── GET /search                    # 搜索
│   ├── POST /mkdir                    # 创建目录
│   ├── POST /move                     # 移动
│   ├── POST /copy                     # 复制
│   ├── DELETE                         # 删除
│   └── GET /download                  # 下载（Range 支持）
│
├── /upload (需认证)
│   ├── POST /init                     # 初始化上传（秒传检查）
│   ├── PUT /chunk                     # 上传分片（本地磁盘）
│   └── POST /finish                   # 完成上传
│
├── /sources (需认证)
│   ├── GET                            # 列出存储源
│   ├── POST                           # 添加存储源
│   ├── GET /:id                       # 获取详情
│   ├── PUT /:id                       # 更新
│   ├── DELETE /:id                    # 删除
│   └── POST /:id/test                 # 测试连接
│
├── /tasks (需认证)
│   ├── GET                            # 列出任务
│   ├── POST                           # 创建任务（离线下载）
│   ├── GET /:id                       # 获取详情
│   └── DELETE /:id                    # 取消任务
│
└── /system (需认证)
    ├── GET /config                    # 获取配置
    ├── PUT /config                    # 更新配置
    ├── GET /stats                     # 系统统计
    └── GET /version                   # 版本信息

/dav/*                                  # WebDAV 服务端（Basic Auth）
/api/v1/health                          # 健康检查（公开）
```

### 7.2 WebDAV 服务端

```
/dav/{source_name}/*

支持方法: PROPFIND / GET / PUT / DELETE / MKCOL / MOVE / COPY
认证: Basic Auth（强制 HTTPS）
缓存: PROPFIND 结果缓存 30s
限流: 独立限流器，每秒 50 请求
```

### 7.3 中间件

| 中间件 | 功能 | 顺序 |
|--------|------|------|
| Recovery | Panic 恢复 | 1 |
| Security Headers | CSP / HSTS / X-Frame-Options | 2 |
| CORS | 跨域配置 | 3 |
| Request Logger | 请求日志（access） | 4 |
| Rate Limiter | 限流（API / 登录 / WebDAV 独立） | 5 |
| JWT Auth | Token 验证（仅需要认证的路由） | 6 |

### 7.4 接口/路由真值表

| 方法 | 路径 | 认证 | 限流 | 说明 |
|------|------|------|------|------|
| GET | `/api/v1/health` | ❌ | ❌ | 健康检查 |
| POST | `/api/v1/auth/login` | ❌ | ✅ 登录限流 | 登录 |
| POST | `/api/v1/auth/refresh` | ❌ | ❌ | 刷新 Token |
| POST | `/api/v1/auth/logout` | ✅ | ❌ | 登出 |
| GET | `/api/v1/auth/me` | ✅ | ❌ | 当前用户信息 |
| GET | `/api/v1/files` | ✅ | ✅ API | 列出文件 |
| GET | `/api/v1/files/search` | ✅ | ✅ API | 搜索文件 |
| POST | `/api/v1/files/mkdir` | ✅ | ✅ API | 创建目录 |
| POST | `/api/v1/files/move` | ✅ | ✅ API | 移动文件 |
| POST | `/api/v1/files/copy` | ✅ | ✅ API | 复制文件 |
| DELETE | `/api/v1/files` | ✅ | ✅ API | 删除文件 |
| GET | `/api/v1/files/download` | ✅ | ✅ API | 下载文件（Range） |
| POST | `/api/v1/upload/init` | ✅ | ✅ API | 初始化上传 |
| PUT | `/api/v1/upload/chunk` | ✅ | ✅ API | 上传分片（本地磁盘） |
| POST | `/api/v1/upload/finish` | ✅ | ✅ API | 完成上传 |
| GET | `/api/v1/sources` | ✅ | ✅ API | 列出存储源 |
| POST | `/api/v1/sources` | ✅ | ✅ API | 添加存储源 |
| GET | `/api/v1/sources/:id` | ✅ | ✅ API | 存储源详情 |
| PUT | `/api/v1/sources/:id` | ✅ | ✅ API | 更新存储源 |
| DELETE | `/api/v1/sources/:id` | ✅ | ✅ API | 删除存储源 |
| POST | `/api/v1/sources/:id/test` | ✅ | ✅ API | 测试连接 |
| GET | `/api/v1/tasks` | ✅ | ✅ API | 列出任务 |
| POST | `/api/v1/tasks` | ✅ | ✅ API | 创建下载任务 |
| GET | `/api/v1/tasks/:id` | ✅ | ✅ API | 任务详情 |
| DELETE | `/api/v1/tasks/:id` | ✅ | ✅ API | 取消任务 |
| GET | `/api/v1/system/config` | ✅ | ✅ API | 获取配置 |
| PUT | `/api/v1/system/config` | ✅ | ✅ API | 更新配置 |
| GET | `/api/v1/system/stats` | ✅ | ✅ API | 系统统计 |
| GET | `/api/v1/system/version` | ✅ | ✅ API | 版本信息 |
| PROPFIND | `/dav/*` | ✅ Basic Auth | ✅ WebDAV | WebDAV 目录列表 |
| GET | `/dav/*` | ✅ Basic Auth | ✅ WebDAV | WebDAV 文件读取（Range） |
| PUT | `/dav/*` | ✅ Basic Auth | ✅ WebDAV | WebDAV 文件写入 |
| DELETE | `/dav/*` | ✅ Basic Auth | ✅ WebDAV | WebDAV 删除 |
| MKCOL | `/dav/*` | ✅ Basic Auth | ✅ WebDAV | WebDAV 创建目录 |
| MOVE | `/dav/*` | ✅ Basic Auth | ✅ WebDAV | WebDAV 移动 |
| COPY | `/dav/*` | ✅ Basic Auth | ✅ WebDAV | WebDAV 复制 |

> 注：所有 WebDAV 路由强制 HTTPS，HTTP 下返回 403。

---

## 8. 数据架构

### 8.1 数据库选型策略

| 场景 | 推荐 | 切换方式 |
|------|------|---------|
| 个人用户，<5 万文件 | SQLite | 默认 |
| 重度用户，≥5 万文件 | PostgreSQL | 一键迁移 |
| 团队多用户 | PostgreSQL | 推荐 |

### 8.2 SQLite 优化配置

```
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;  -- 64MB
PRAGMA temp_store = MEMORY;
```

### 8.3 缓存策略

| 场景 | 策略 | 说明 |
|------|------|------|
| S3/OneDrive 文件列表 | 按需缓存 + 内存缓存 5min | 实时 API 获取，缓存加速 |
| 本地磁盘文件列表 | 按需缓存 + 后台索引 | inotify 监听 + 定期扫描 |
| 用户信息 | 内存缓存 10min | 不常变更 |
| ACL 权限 | 内存缓存 1min | 安全与性能平衡 |
| WebDAV PROPFIND | 内存缓存 30s | 防 Jellyfin 扫库 |

### 8.4 全文搜索

- **本地磁盘 (SQLite)**: SQLite FTS5 虚拟表
- **本地磁盘 (PostgreSQL)**: `to_tsvector` + GIN 索引（见下方）
- **S3/OneDrive**: 仅支持文件名前缀搜索（API 限制）

### 8.5 PostgreSQL 全文搜索方案

当使用 PostgreSQL 时，不采用 SQLite FTS5，而是使用 PostgreSQL 原生全文搜索：

```sql
-- 为 file_metadata 表添加搜索向量列
ALTER TABLE file_metadata ADD COLUMN search_vector tsvector;

-- 创建 GIN 索引（高性能全文搜索）
CREATE INDEX idx_file_search ON file_metadata USING GIN(search_vector);

-- 创建触发器，自动更新搜索向量
CREATE OR REPLACE FUNCTION update_file_search_vector()
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := to_tsvector('simple', NEW.name);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_file_search_update
BEFORE INSERT OR UPDATE ON file_metadata
FOR EACH ROW EXECUTE FUNCTION update_file_search_vector();

-- 中文搜索：使用 pg_jieba 或 zhparser 扩展（可选）
-- 基础安装使用 'simple' 配置，支持中文单字切分
```

**SQLite vs PostgreSQL 搜索能力对比**：

| 能力 | SQLite FTS5 | PostgreSQL tsvector |
|------|-------------|---------------------|
| 中文分词 | 单字切分 | 单字切分（或 pg_jieba） |
| 性能（10万文件） | < 50ms | < 20ms |
|  ranking / 排序 | 有限支持 | 内置 ranking |
|  前缀匹配 | 支持 | 支持 |
|  布尔查询 | 支持 | 支持 |

---

## 9. 安全架构

### 9.1 认证体系

```
登录
  │
  ├──▶ bcrypt 验证密码（cost=12）
  │
  ├──▶ 生成 JWT 双 Token
  │       ├── Access Token: 15 分钟，含 user_id / role / token_version
  │       └── Refresh Token: 7 天，存储在数据库
  │
  └──▶ 返回双 Token

访问受保护资源
  │
  ├──▶ 解析 Authorization: Bearer <access_token>
  │
  ├──▶ 验证 JWT 签名和过期时间
  │
  ├──▶ 验证 token_version == 用户表中的 token_version
  │       └── 不匹配 ──▶ Token 已被撤销
  │
  └──▶ 通过，设置 userID 到 context
```

### 9.2 授权体系 (ACL)

```
请求到达
  │
  ├──▶ 认证通过，获取 userID
  │
  ├──▶ 检查目标路径 ACL
  │       ├── 生成路径层级 ["/a/b/c", "/a/b", "/a", "/"]
  │       ├── 查询所有匹配规则
  │       ├── 按优先级排序
  │       └── 取最具体规则
  │
  ├──▶ deny 规则？──▶ 403 Forbidden
  │
  └──▶ allow 规则但权限不足？──▶ 403 Forbidden
```

### 9.3 WebDAV 安全

- **强制 HTTPS**: HTTP 下拒绝 Basic Auth，返回 403
- **范围控制**: 仅暴露用户有 read 权限的存储源
- **只读默认**: 媒体库场景建议 readonly=true
- **独立限流**: 防止扫库攻击

### 9.4 审计日志

记录所有敏感操作：登录/登出、文件上传/下载/删除、用户管理、配置变更、分享创建/访问。

---

## 10. 高并发架构

### 10.1 并发场景识别

| 场景 | 并发类型 | 压力 |
|------|---------|------|
| Jellyfin 扫描 | WebDAV 大量读取 | 50-200 并发 PROPFIND |
| 多用户同时访问 | API 读取 | 5-20 并发 |
| 大文件分片上传 | Chunk 写入 | 3 并发/用户 |
| 离线下载 | 后台任务 | 3 worker |

### 10.2 优化策略

| 层面 | 策略 | 实现 |
|------|------|------|
| **数据库** | WAL 模式 + 连接池 | SQLite: MaxOpenConns=10; PG: MaxOpenConns=100 |
| **缓存** | 多级缓存 | L1: bigcache (30s-5min); L2: SQLite 缓存表 |
| **WebDAV** | PROPFIND 缓存 + 限流 | 缓存 30s; 限流 50 req/s |
| **上传** | 预签名 URL 直传 | S3 不走服务端代理; 本地磁盘直接写 |
| **下载** | Range 请求 + 直传 | S3 预签名 URL 天然支持 Range |
| **连接复用** | HTTP Keep-Alive | MaxIdleConns=100, MaxIdleConnsPerHost=20 |

### 10.3 限流策略

| 路由 | 限流 | 阈值 |
|------|------|------|
| /api/v1/auth/login | 令牌桶 | 5 次/分钟/IP |
| /api/v1/* | 令牌桶 | 100 次/秒 |
| /dav/* | 令牌桶 | 50 次/秒 |

---

## 11. 关键业务流程

### 11.1 断点续传上传

```
1. 前端计算文件 MD5
2. POST /upload/init → 秒传检查
3. 返回 upload_id + chunk_info
4. 前端并发上传 chunks（3 并发）
5. PUT /upload/chunk（本地）或 PUT presigned_url（S3）
6. 服务端更新 completed_chunks
7. POST /upload/finish → 合并文件 → 清理临时文件
```

### 11.2 离线下载

```
1. 用户提交下载链接
2. POST /tasks → 创建 download 任务
3. 任务入队 → Worker 获取
4. Worker 调用 Aria2.AddURI()
5. Aria2 下载中 → Worker 轮询状态
6. 下载完成 → 移动文件到目标目录
7. 写入 file_metadata → 标记任务完成
```

### 11.3 WebDAV 访问

```
1. 客户端发送 PROPFIND /dav/local/path
2. 强制 HTTPS 检查
3. Basic Auth 认证
4. 解析路径 → source=local, path=/path
5. ACL 检查
6. PROPFIND 缓存检查（30s）
7. Driver.List() 获取文件列表
8. ACL 过滤子项
9. 写入缓存 → 返回 WebDAV XML
```

---

## 12. 配置体系

### 12.1 配置源优先级

```
环境变量 > 配置文件 (YAML) > 默认值
```

### 12.2 关键配置项

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

database:
  type: "sqlite"
  dsn: "/data/database.db"
  max_open_conns: 10
  max_idle_conns: 5

jwt:
  secret: "change-me"
  access_token_expire: 15m
  refresh_token_expire: 168h

storage:
  data_dir: "/data"
  temp_dir: "/data/temp"
  max_upload_size: 10737418240  # 10GB
  default_chunk_size: 5242880   # 5MB

webdav:
  enabled: true
  prefix: "/dav"
  cache_ttl: 30s

log:
  level: "info"
  format: "json"
  output: "stdout"
  dir: "/data/logs"

aria2:
  rpc_url: "http://aria2:6800/jsonrpc"
  rpc_secret: ""

task_queue:
  workers: 3

security:
  login_max_attempts: 5
  login_lock_duration: 15m
  bcrypt_cost: 12
```

---

## 13. 部署架构

### 13.1 Docker Compose（推荐）

```yaml
services:
  yunxia:
    image: yunxia/yunxia:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - YUNXIA_JWT_SECRET=${JWT_SECRET}
      - YUNXIA_ARIA2_RPC_URL=http://aria2:6800/jsonrpc
    depends_on:
      - aria2

  aria2:
    image: p3terx/aria2-pro:latest
    volumes:
      - ./downloads:/downloads
    environment:
      - RPC_SECRET=${ARIA2_RPC_SECRET}
```

### 13.2 资源要求

| 场景 | CPU | 内存 | 存储 |
|------|-----|------|------|
| 最低 | 1 核 | 512MB | 10GB |
| 推荐 | 2 核 | 1GB | 50GB+ |
| 含离线下载 | 2 核 | 2GB | 200GB+ |

### 13.3 无状态设计

- JWT 认证：无 Session，任何实例可处理请求
- 文件存储：映射到外部 volume，实例重启不丢失
- 配置共享：运行时配置存在数据库，所有实例读取
- 上传直传：大文件不走服务端代理，无状态

### 13.4 分布式部署注意事项

**本地上传的临时文件问题**：

本地磁盘上传时，分片临时文件存储在 `./data/temp/{upload_id}/`。多实例部署时，如果用户 A 上传 chunk 到实例 1，完成上传请求打到实例 2，实例 2 无法访问实例 1 的本地临时文件。

**解决方案（按优先级）**：

| 方案 | 说明 | 复杂度 |
|------|------|--------|
| **A. 共享存储卷（推荐）** | 所有实例挂载同一个 NFS/共享卷，`temp_dir` 指向共享路径 | 低 |
| **B. Sticky Session** | 负载均衡器按 upload_id 哈希路由，确保同一上传始终到同一实例 | 中 |
| **C. 分布式临时存储** | 临时文件存入 Redis / MinIO，任意实例可读取 | 高 |
| **D. 仅单实例本地上传** | 文档说明：本地上传仅在单实例模式下完全支持 | 低 |

**推荐：方案 A（共享存储卷）+ 方案 D（文档说明）**

- Docker Swarm/K8s 部署时，使用共享 volume（如 NFS、CephFS）
- 单机 Docker Compose 无需处理
- 文档明确说明：多实例部署本地上传需要共享 `temp_dir`

**S3 上传天然无状态**：
- 分片直接上传到 S3，服务端只记录元数据
- 任意实例可处理 finish 请求
- 推荐重度用户优先使用 S3 存储后端

---

## 14. 附录

### 14.1 环境变量清单

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `YUNXIA_SERVER_HOST` | 监听地址 | `0.0.0.0` |
| `YUNXIA_SERVER_PORT` | 监听端口 | `8080` |
| `YUNXIA_DATABASE_TYPE` | 数据库类型 | `sqlite` |
| `YUNXIA_DATABASE_DSN` | 数据库连接 | `/data/database.db` |
| `YUNXIA_JWT_SECRET` | JWT 密钥 | **必填** |
| `YUNXIA_LOG_LEVEL` | 日志级别 | `info` |
| `YUNXIA_ARIA2_RPC_URL` | Aria2 地址 | `http://aria2:6800/jsonrpc` |
| `YUNXIA_ARIA2_RPC_SECRET` | Aria2 密钥 | `""` |

### 14.2 相关文档

| 文档 | 说明 |
|------|------|
| `DOCS-INDEX.md` | 文档总索引与真相源约定 |
| `PRD.md` | 产品需求文档 |
| `INTERFACE-ARCHITECTURE.md` | 共享抽象与依赖注入规范 |
| `DESIGN.md` | 详细后端实现设计 |
| `FRONTEND-DESIGN.md` | 前端交互与页面设计 |

---

*本文档为云匣 (Yunxia) 的技术架构真相源，采用 DDD 分层架构（逻辑四层，工程习惯简称“三层”），涵盖从领域模型到部署架构的核心技术决策。*
