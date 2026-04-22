# 云匣 (Yunxia) — 项目设计方案

> **版本**: v1.0  
> **日期**: 2026-04-20  
> **架构范式**: DDD (Domain-Driven Design) 分层架构（逻辑四层，工程习惯简称“三层”）  
> **文档职责**: 后端实现设计、代码级模块、数据结构、关键流程落地  
> **上游约束**: `PRD.md` / `TAD.md` / `INTERFACE-ARCHITECTURE.md`  
> **技术栈**: Go 1.24 + Gin + GORM / React 18 + Vite + shadcn/ui

---

## 目录

1. [架构概述](#1-架构概述)
2. [分层架构映射](#2-分层架构映射)
3. [领域层 (Domain Layer)](#3-领域层-domain-layer)
4. [应用层 (Application Layer)](#4-应用层-application-layer)
5. [基础设施层 (Infrastructure Layer)](#5-基础设施层-infrastructure-layer)
6. [接口适配层 (Interface Layer)](#6-接口适配层-interface-layer)
7. [数据库设计](#7-数据库设计)
8. [关键业务流程](#8-关键业务流程)
9. [配置体系](#9-配置体系)
10. [部署方案](#10-部署方案)

---

## 1. 架构概述

### 1.1 DDD 分层架构

> 注：本文档统一使用“分层架构”表述；若出现“三层架构”字样，均指接口适配层 / 应用层 / 领域层 / 基础设施层这套逻辑四层结构。

严格遵循 DDD 分层架构（四层：接口适配层 / 应用层 / 领域层 / 基础设施层），依赖关系只能**向内**（外层依赖内层，内层不依赖外层）：

```
┌─────────────────────────────────────────────────────────────┐
│  接口适配层 (Interface Adapters)                              │
│  ┌──────────────┬──────────────┬─────────────────────────┐  │
│  │ HTTP Handler │ WebDAV Hdlr  │    Middleware           │  │
│  │ (REST API)   │ (WebDAV)     │ (Auth/RateLimit/Log)    │  │
│  └──────┬───────┴──────┬───────┴──────────┬──────────────┘  │
│         │              │                  │                 │
├─────────┼──────────────┼──────────────────┼─────────────────┤
│         ▼              ▼                  ▼                 │
│  应用层 (Application Layer)                                  │
│  ┌──────────────┬──────────────┬─────────────────────────┐  │
│  │  Auth AppSvc │  File AppSvc │    Task AppSvc          │  │
│  │  DTOs        │  DTOs        │    DTOs                 │  │
│  │  Use Cases   │  Use Cases   │    Use Cases            │  │
│  └──────┬───────┴──────┬───────┴──────────┬──────────────┘  │
│         │              │                  │                 │
├─────────┼──────────────┼──────────────────┼─────────────────┤
│         ▼              ▼                  ▼                 │
│  领域层 (Domain Layer)                                        │
│  ┌──────────────┬──────────────┬─────────────────────────┐  │
│  │   Entities   │  Repository  │    Domain Services      │  │
│  │ (User,File)  │  Interfaces  │    (ACL Checker)        │  │
│  │   VO         │              │                         │  │
│  └──────────────┴──────────────┴─────────────────────────┘  │
│         ▲              ▲                  ▲                 │
├─────────┼──────────────┼──────────────────┼─────────────────┤
│         │              │                  │                 │
│  基础设施层 (Infrastructure Layer)                            │
│  ┌──────────────┬──────────────┬─────────────────────────┐  │
│  │ GORM Repo    │ Storage Drv  │    Cache / MQ           │  │
│  │ Impl         │ (Local/S3)   │    / Aria2 Client       │  │
│  └──────────────┴──────────────┴─────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 依赖规则

```
接口层 ──依赖──▶ 应用层 ──依赖──▶ 领域层 ◀──依赖── 基础设施层
                                                          │
                                                    通过依赖注入
                                                    (Dependency Injection)
```

- **领域层**不依赖任何其他层，只包含纯业务逻辑
- **应用层**只依赖领域层，编排领域对象完成用例
- **接口层**依赖应用层，将外部请求转换为应用服务调用
- **基础设施层**依赖领域层（实现领域层定义的接口），通过依赖注入提供给上层

### 1.3 核心设计原则

| 原则 | 说明 |
|------|------|
| **依赖倒置** | 领域层定义 Repository 接口，基础设施层实现。Service 只依赖接口 |
| **关注点分离** | 业务逻辑集中在领域层，技术细节集中在基础设施层 |
| **可测试性** | 每层可独立单元测试，通过 Mock Repository 测试 Service |
| **可替换性** | SQLite ↔ PostgreSQL、bigcache ↔ Redis 切换只需替换实现 |

---

## 2. 分层架构映射

### 2.1 目录结构（DDD 映射）

```
yunxia/
├── cmd/
│   └── server/
│       └── main.go              # 程序入口：依赖注入、服务启动
│
├── internal/                    # 私有代码（不可被外部导入）
│   ├── domain/                  # ━━ 领域层 ━━
│   │   ├── entity/              # 实体（有唯一标识、生命周期）
│   │   │   ├── user.go
│   │   │   ├── file.go
│   │   │   ├── storage_source.go
│   │   │   ├── upload_session.go
│   │   │   ├── task.go
│   │   │   ├── acl_entry.go
│   │   │   └── share.go
│   │   ├── valueobject/         # 值对象（无标识、不可变）
│   │   │   ├── file_info.go
│   │   │   ├── pagination.go
│   │   │   ├── permission.go    # 权限位枚举
│   │   │   └── storage_config.go
│   │   ├── repository/          # 仓库接口（领域层定义）
│   │   │   ├── user_repo.go
│   │   │   ├── file_repo.go
│   │   │   ├── source_repo.go
│   │   │   ├── upload_repo.go
│   │   │   ├── task_repo.go
│   │   │   ├── acl_repo.go
│   │   │   └── share_repo.go
│   │   └── service/             # 领域服务（跨实体的业务逻辑）
│   │       ├── acl_service.go   # 权限计算核心逻辑
│   │       └── auth_service.go  # 认证领域逻辑
│   │
│   ├── application/             # ━━ 应用层 ━━
│   │   ├── dto/                 # 数据传输对象（Request/Response）
│   │   │   ├── auth_dto.go
│   │   │   ├── file_dto.go
│   │   │   ├── source_dto.go
│   │   │   ├── upload_dto.go
│   │   │   ├── task_dto.go
│   │   │   └── webdav_dto.go
│   │   ├── service/             # 应用服务（编排领域对象完成用例）
│   │   │   ├── auth_app_svc.go
│   │   │   ├── file_app_svc.go
│   │   │   ├── source_app_svc.go
│   │   │   ├── upload_app_svc.go
│   │   │   ├── task_app_svc.go
│   │   │   ├── share_app_svc.go
│   │   │   └── webdav_app_svc.go
│   │   └── assembler/           # DTO ↔ Entity 转换器
│   │       ├── auth_assembler.go
│   │       └── file_assembler.go
│   │
│   ├── interfaces/              # ━━ 接口适配层 ━━
│   │   ├── http/                # REST API Handler
│   │   │   ├── handler/
│   │   │   │   ├── auth_handler.go
│   │   │   │   ├── file_handler.go
│   │   │   │   ├── source_handler.go
│   │   │   │   ├── upload_handler.go
│   │   │   │   ├── task_handler.go
│   │   │   │   └── system_handler.go
│   │   │   └── router.go        # 路由注册
│   │   ├── webdav/              # WebDAV Handler
│   │   │   ├── dav_fs.go
│   │   │   ├── dav_handler.go
│   │   │   └── dav_lock.go
│   │   └── middleware/          # 中间件
│   │       ├── auth_mw.go
│   │       ├── rate_limit.go
│   │       ├── cors.go
│   │       ├── security.go
│   │       └── logger.go
│   │
│   └── infrastructure/          # ━━ 基础设施层 ━━
│       ├── persistence/         # 持久化实现
│       │   ├── gorm/
│       │   │   ├── db.go        # 数据库连接初始化
│       │   │   ├── user_repo_impl.go
│       │   │   ├── file_repo_impl.go
│       │   │   ├── source_repo_impl.go
│       │   │   ├── upload_repo_impl.go
│       │   │   ├── task_repo_impl.go
│       │   │   ├── acl_repo_impl.go
│       │   │   └── share_repo_impl.go
│       │   └── migration/       # 数据库迁移
│       │       └── migration.go
│       ├── storage/             # 存储驱动实现
│       │   ├── driver.go        # Driver 接口定义
│       │   ├── local/
│       │   │   └── local_driver.go
│       │   ├── s3/
│       │   │   └── s3_driver.go
│       │   ├── onedrive/
│       │   │   └── onedrive_driver.go
│       │   └── registry.go      # 驱动注册中心
│       ├── cache/               # 缓存实现
│       │   ├── cache.go         # 缓存接口
│       │   └── bigcache_impl.go
│       ├── mq/                  # 消息队列/任务队列实现
│       │   ├── queue.go         # 队列接口
│       │   └── sqlite_queue.go  # SQLite 持久化队列
│       ├── downloader/          # 下载器客户端
│       │   ├── downloader.go    # 下载引擎接口
│       │   └── aria2_client.go  # Aria2 JSON-RPC 客户端
│       ├── config/              # 配置读取
│       │   └── config.go
│       └── pkg/                 # 基础设施通用包
│           ├── logger/
│           ├── validator/
│           └── utils/
│
├── pkg/                         # 可对外暴露的包
│   └── driver/                  # Driver 接口（供第三方驱动使用）
│       └── driver.go
│
├── web/                         # 前端项目
│   ├── src/
│   ├── public/
│   ├── package.json
│   ├── vite.config.ts
│   └── tailwind.config.js
│
├── migrations/                  # 数据库迁移脚本
├── scripts/                     # 构建/部署脚本
├── .github/workflows/            # CI/CD
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod
└── README.md
```

---

## 3. 领域层 (Domain Layer)

领域层是系统的核心，包含**实体、值对象、领域服务和仓库接口**。领域层不依赖任何外部框架或库。

### 3.1 实体 (Entities)

实体是具有唯一标识、生命周期和业务规则的对象。

#### 3.1.1 User（用户）

```go
package entity

import "time"

type Role string

const (
    RoleAdmin Role = "admin"
    RoleUser  Role = "user"
    RoleGuest Role = "guest"
)

type UserStatus string

const (
    UserStatusActive   UserStatus = "active"
    UserStatusDisabled UserStatus = "disabled"
)

// User 用户实体
type User struct {
    ID           uint       `json:"id"`
    Username     string     `json:"username"`
    PasswordHash string     `json:"-"`          // 不序列化
    Email        string     `json:"email"`
    Role         Role       `json:"role"`
    Status       UserStatus `json:"status"`
    StorageQuota int64      `json:"storage_quota"` // 0 = 无限制
    TokenVersion int        `json:"-"`             // Token 撤销版本号
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}

// IsAdmin 检查是否是管理员
func (u *User) IsAdmin() bool {
    return u.Role == RoleAdmin
}

// CanLogin 检查是否可以登录
func (u *User) CanLogin() bool {
    return u.Status == UserStatusActive
}

// UpdateTokenVersion 更新 Token 版本号（用于撤销所有登录）
func (u *User) UpdateTokenVersion() {
    u.TokenVersion++
}

// ValidatePassword 验证密码（bcrypt）
func (u *User) ValidatePassword(password string, hasher PasswordHasher) bool {
    return hasher.Compare(u.PasswordHash, password)
}
```

#### 3.1.2 StorageSource（存储源）

```go
package entity

import "time"

// DriverType 驱动类型
type DriverType string

const (
    DriverLocal    DriverType = "local"
    DriverS3       DriverType = "s3"
    DriverOneDrive DriverType = "onedrive"
)

// StorageSource 存储源实体
type StorageSource struct {
    ID              uint              `json:"id"`
    Name            string            `json:"name"`              // 显示名称
    DriverType      DriverType        `json:"driver_type"`       // 驱动类型
    Config          map[string]interface{} `json:"config"`       // 驱动配置（JSON）
    RootPath        string            `json:"root_path"`         // 根路径
    IsEnabled       bool              `json:"is_enabled"`
    IsWebDAVExposed bool              `json:"is_webdav_exposed"` // 是否通过 WebDAV 暴露
    WebDAVReadOnly  bool              `json:"webdav_readonly"`   // WebDAV 只读
    SortOrder       int               `json:"sort_order"`
    CreatedAt       time.Time         `json:"created_at"`
    UpdatedAt       time.Time         `json:"updated_at"`
}

// CanExposeWebDAV 检查是否可以暴露 WebDAV
func (s *StorageSource) CanExposeWebDAV() bool {
    return s.IsEnabled && s.IsWebDAVExposed
}
```

#### 3.1.3 FileMetadata（文件元数据）

```go
package entity

import "time"

// FileMetadata 文件/目录元数据实体
type FileMetadata struct {
    ID         uint      `json:"id"`
    SourceID   uint      `json:"source_id"`
    Path       string    `json:"path"`        // 完整路径
    Name       string    `json:"name"`        // 文件名
    ParentPath string    `json:"parent_path"` // 父目录路径
    Size       int64     `json:"size"`
    IsDir      bool      `json:"is_dir"`
    MimeType   string    `json:"mime_type"`
    Checksum   string    `json:"checksum,omitempty"`
    ModifiedAt time.Time `json:"modified_at"`
    CreatedAt  time.Time `json:"created_at"`
    CachedAt   time.Time `json:"cached_at"`   // 缓存时间
    Extra      map[string]interface{} `json:"extra,omitempty"`
}

// IsRoot 检查是否是根目录
func (f *FileMetadata) IsRoot() bool {
    return f.Path == "/" || f.Path == ""
}
```

#### 3.1.4 UploadSession（上传会话）

```go
package entity

import "time"

// UploadStatus 上传状态
type UploadStatus string

const (
    UploadPending    UploadStatus = "pending"
    UploadUploading  UploadStatus = "uploading"
    UploadCompleted  UploadStatus = "completed"
    UploadFailed     UploadStatus = "failed"
    UploadCancelled  UploadStatus = "cancelled"
)

// UploadSession 上传会话实体
type UploadSession struct {
    ID              string       `json:"id"`               // UUID
    UserID          uint         `json:"user_id"`
    SourceID        uint         `json:"source_id"`
    TargetPath      string       `json:"target_path"`
    Filename        string       `json:"filename"`
    FileSize        int64        `json:"file_size"`
    FileHash        string       `json:"file_hash"`        // MD5
    Status          UploadStatus `json:"status"`
    ChunkSize       int64        `json:"chunk_size"`       // 默认 5MB
    TotalChunks     int          `json:"total_chunks"`
    UploadedChunks  int          `json:"uploaded_chunks"`
    CompletedChunks []int        `json:"completed_chunks"` // 已完成的 chunk 索引
    StorageData     string       `json:"storage_data"`     // 存储后端特定数据（JSON）
    ExpiresAt       time.Time    `json:"expires_at"`
    CreatedAt       time.Time    `json:"created_at"`
    UpdatedAt       time.Time    `json:"updated_at"`
}

// IsExpired 检查是否过期
func (u *UploadSession) IsExpired() bool {
    return time.Now().After(u.ExpiresAt)
}

// IsChunkUploaded 检查某个 chunk 是否已上传
func (u *UploadSession) IsChunkUploaded(index int) bool {
    for _, i := range u.CompletedChunks {
        if i == index {
            return true
        }
    }
    return false
}

// MarkChunkUploaded 标记 chunk 已上传
func (u *UploadSession) MarkChunkUploaded(index int) {
    if !u.IsChunkUploaded(index) {
        u.CompletedChunks = append(u.CompletedChunks, index)
        u.UploadedChunks++
    }
}

// IsComplete 检查是否全部完成
func (u *UploadSession) IsComplete() bool {
    return u.UploadedChunks >= u.TotalChunks
}
```

#### 3.1.5 Task（任务）

```go
package entity

import "time"

// TaskType 任务类型
type TaskType string

const (
    TaskTypeDownload   TaskType = "download"    // 离线下载
    TaskTypeIndexScan  TaskType = "index_scan"  // 索引扫描
    TaskTypeCleanup    TaskType = "cleanup"     // 清理临时文件
)

// TaskStatus 任务状态
type TaskStatus string

const (
    TaskPending    TaskStatus = "pending"
    TaskRunning    TaskStatus = "running"
    TaskCompleted  TaskStatus = "completed"
    TaskFailed     TaskStatus = "failed"
    TaskCancelled  TaskStatus = "cancelled"
)

// Task 任务实体
type Task struct {
    ID            string     `json:"id"`
    Type          TaskType   `json:"type"`
    Status        TaskStatus `json:"status"`
    Payload       string     `json:"payload"`        // JSON 任务参数
    Result        string     `json:"result"`         // 执行结果
    ErrorMsg      string     `json:"error_msg"`
    Priority      int        `json:"priority"`
    ScheduledAt   *time.Time `json:"scheduled_at"`   // 计划执行时间
    StartedAt     *time.Time `json:"started_at"`
    CompletedAt   *time.Time `json:"completed_at"`
    RetryCount    int        `json:"retry_count"`
    MaxRetries    int        `json:"max_retries"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
}

// CanRetry 检查是否可以重试
func (t *Task) CanRetry() bool {
    return t.RetryCount < t.MaxRetries && 
           (t.Status == TaskFailed || t.Status == TaskPending)
}
```

#### 3.1.6 ACLEntry（访问控制条目）

```go
package entity

import "time"

// Permission 权限位
type Permission struct {
    Read   bool
    Write  bool
    Delete bool
    Share  bool
}

// RuleType 规则类型
type RuleType string

const (
    RuleAllow RuleType = "allow"
    RuleDeny  RuleType = "deny"
)

// ACLEntry 访问控制实体
type ACLEntry struct {
    ID       uint       `json:"id"`
    UserID   uint       `json:"user_id"`
    SourceID uint       `json:"source_id"`
    Path     string     `json:"path"`       // 路径，/ 表示存储源根
    Read     bool       `json:"read"`
    Write    bool       `json:"write"`
    Delete   bool       `json:"delete"`
    Share    bool       `json:"share"`
    RuleType RuleType   `json:"rule_type"`  // allow / deny
    Inherit  bool       `json:"inherit"`    // 子目录是否继承
    Priority int        `json:"priority"`   // 优先级
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// HasPermission 检查是否有指定权限
func (a *ACLEntry) HasPermission(p Permission) bool {
    if a.RuleType == RuleDeny {
        return false
    }
    return (!p.Read || a.Read) &&
           (!p.Write || a.Write) &&
           (!p.Delete || a.Delete) &&
           (!p.Share || a.Share)
}
```

### 3.2 值对象 (Value Objects)

值对象没有唯一标识，不可变，用于描述特征。

```go
package valueobject

import "time"

// FileInfo 文件信息值对象（来自存储驱动）
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

// Pagination 分页值对象
type Pagination struct {
    Page       int   `json:"page"`
    PageSize   int   `json:"page_size"`
    Total      int64 `json:"total"`
    TotalPages int   `json:"total_pages"`
}

// StorageConfig 存储配置值对象
type StorageConfig struct {
    DriverType DriverType
    Config     map[string]interface{}
}

// Permission 权限值对象
type Permission struct {
    Read   bool
    Write  bool
    Delete bool
    Share  bool
}
```

### 3.3 领域服务 (Domain Services)

领域服务处理**跨实体**的业务逻辑，或**不适合放在单个实体中**的逻辑。

#### 3.3.1 ACLService（权限计算服务）

```go
package service

import (
    "path/filepath"
    "sort"
    "strings"
    
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
    "yunxia/internal/domain/valueobject"
)

// ACLService 权限计算领域服务
type ACLService struct {
    aclRepo repository.ACLRepository
}

func NewACLService(aclRepo repository.ACLRepository) *ACLService {
    return &ACLService{aclRepo: aclRepo}
}

// CheckPermission 检查用户对某路径的权限
// 算法：从具体路径向上查找，取最匹配的规则
func (s *ACLService) CheckPermission(
    userID uint, 
    sourceID uint, 
    path string, 
    perm valueobject.Permission,
) (bool, error) {
    // 1. 生成路径层级列表
    // /a/b/c → ["/a/b/c", "/a/b", "/a", "/"]
    paths := generatePathHierarchy(path)
    
    // 2. 查询所有适用的 ACL 规则
    var rules []*entity.ACLEntry
    for _, p := range paths {
        entries, err := s.aclRepo.FindByUserAndSourceAndPath(userID, sourceID, p)
        if err != nil {
            return false, err
        }
        rules = append(rules, entries...)
    }
    
    // 3. 按优先级排序（高优先在前）
    sort.Slice(rules, func(i, j int) bool {
        return rules[i].Priority > rules[j].Priority
    })
    
    // 4. 找到第一个匹配的规则
    for _, rule := range rules {
        // 检查规则是否适用于此路径
        if rule.Path == path || (rule.Inherit && strings.HasPrefix(path, rule.Path)) {
            if rule.RuleType == entity.RuleDeny {
                return false, nil
            }
            return rule.HasPermission(perm), nil
        }
    }
    
    // 5. 无匹配规则：默认拒绝
    return false, nil
}

// generatePathHierarchy 生成路径层级
func generatePathHierarchy(path string) []string {
    path = filepath.Clean(path)
    var paths []string
    for path != "/" && path != "." {
        paths = append(paths, path)
        path = filepath.Dir(path)
    }
    paths = append(paths, "/")
    return paths
}
```

#### 3.3.2 AuthDomainService（认证领域服务）

```go
package service

import "yunxia/internal/domain/entity"

// PasswordHasher 密码哈希接口（领域层定义，基础设施层实现）
type PasswordHasher interface {
    Hash(password string) (string, error)
    Compare(hash, password string) bool
}

// AuthDomainService 认证领域服务
type AuthDomainService struct {
    hasher PasswordHasher
}

func NewAuthDomainService(hasher PasswordHasher) *AuthDomainService {
    return &AuthDomainService{hasher: hasher}
}

// HashPassword 哈希密码
func (s *AuthDomainService) HashPassword(password string) (string, error) {
    return s.hasher.Hash(password)
}

// ValidatePassword 验证密码
func (s *AuthDomainService) ValidatePassword(user *entity.User, password string) bool {
    return user.ValidatePassword(password, s.hasher)
}

// GenerateDefaultACL 为新用户生成默认 ACL
func (s *AuthDomainService) GenerateDefaultACL(userID uint, sourceID uint) *entity.ACLEntry {
    return &entity.ACLEntry{
        UserID:   userID,
        SourceID: sourceID,
        Path:     "/",
        Read:     true,
        Write:    true,
        Delete:   true,
        Share:    true,
        RuleType: entity.RuleAllow,
        Inherit:  true,
        Priority: 0,
    }
}
```

### 3.4 仓库接口 (Repository Interfaces)

仓库接口在领域层定义，由基础设施层实现。这是**依赖倒置**的核心。

```go
// internal/domain/repository/user_repo.go
package repository

import (
    "context"
    "yunxia/internal/domain/entity"
)

// UserRepository 用户仓库接口
type UserRepository interface {
    Create(ctx context.Context, user *entity.User) error
    Update(ctx context.Context, user *entity.User) error
    Delete(ctx context.Context, id uint) error
    FindByID(ctx context.Context, id uint) (*entity.User, error)
    FindByUsername(ctx context.Context, username string) (*entity.User, error)
    FindByUsernameWithPassword(ctx context.Context, username string) (*entity.User, error)
    List(ctx context.Context, page, pageSize int) ([]*entity.User, int64, error)
    Exists(ctx context.Context, username string) (bool, error)
    Count(ctx context.Context) (int64, error)
}
```

```go
// internal/domain/repository/file_repo.go
package repository

import (
    "context"
    "yunxia/internal/domain/entity"
)

// FileRepository 文件元数据仓库接口
type FileRepository interface {
    Save(ctx context.Context, meta *entity.FileMetadata) error
    Delete(ctx context.Context, sourceID uint, path string) error
    FindByPath(ctx context.Context, sourceID uint, path string) (*entity.FileMetadata, error)
    ListByParent(ctx context.Context, sourceID uint, parentPath string, page, pageSize int) ([]*entity.FileMetadata, int64, error)
    SearchByName(ctx context.Context, sourceID uint, keyword string, page, pageSize int) ([]*entity.FileMetadata, int64, error)
    DeleteBySource(ctx context.Context, sourceID uint) error
}
```

```go
// internal/domain/repository/source_repo.go
package repository

import (
    "context"
    "yunxia/internal/domain/entity"
)

// StorageSourceRepository 存储源仓库接口
type StorageSourceRepository interface {
    Create(ctx context.Context, source *entity.StorageSource) error
    Update(ctx context.Context, source *entity.StorageSource) error
    Delete(ctx context.Context, id uint) error
    FindByID(ctx context.Context, id uint) (*entity.StorageSource, error)
    ListByUser(ctx context.Context, userID uint) ([]*entity.StorageSource, error)
    ListAll(ctx context.Context) ([]*entity.StorageSource, error)
    ListWebDAVExposed(ctx context.Context) ([]*entity.StorageSource, error)
}
```

```go
// internal/domain/repository/upload_repo.go
package repository

import (
    "context"
    "yunxia/internal/domain/entity"
)

// UploadRepository 上传会话仓库接口
type UploadRepository interface {
    Create(ctx context.Context, session *entity.UploadSession) error
    Update(ctx context.Context, session *entity.UploadSession) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*entity.UploadSession, error)
    FindByHash(ctx context.Context, sourceID uint, hash string) (*entity.UploadSession, error)
    ListExpired(ctx context.Context) ([]*entity.UploadSession, error)
}
```

```go
// internal/domain/repository/task_repo.go
package repository

import (
    "context"
    "yunxia/internal/domain/entity"
    "time"
)

// TaskRepository 任务仓库接口
type TaskRepository interface {
    Create(ctx context.Context, task *entity.Task) error
    Update(ctx context.Context, task *entity.Task) error
    Delete(ctx context.Context, id string) error
    FindByID(ctx context.Context, id string) (*entity.Task, error)
    FindPending(ctx context.Context, limit int) ([]*entity.Task, error)
    FindByStatus(ctx context.Context, status entity.TaskStatus, page, pageSize int) ([]*entity.Task, int64, error)
    FindScheduledBefore(ctx context.Context, t time.Time, limit int) ([]*entity.Task, error)
}
```

```go
// internal/domain/repository/acl_repo.go
package repository

import (
    "context"
    "yunxia/internal/domain/entity"
)

// ACLRepository ACL 仓库接口
type ACLRepository interface {
    Create(ctx context.Context, entry *entity.ACLEntry) error
    Update(ctx context.Context, entry *entity.ACLEntry) error
    Delete(ctx context.Context, id uint) error
    FindByID(ctx context.Context, id uint) (*entity.ACLEntry, error)
    FindByUserAndSource(ctx context.Context, userID, sourceID uint) ([]*entity.ACLEntry, error)
    FindByUserAndSourceAndPath(ctx context.Context, userID, sourceID uint, path string) ([]*entity.ACLEntry, error)
    DeleteByUserAndSource(ctx context.Context, userID, sourceID uint) error
}
```

```go
// internal/domain/repository/share_repo.go
package repository

import (
    "context"
    "yunxia/internal/domain/entity"
)

// ShareRepository 分享仓库接口
type ShareRepository interface {
    Create(ctx context.Context, share *entity.Share) error
    Delete(ctx context.Context, id uint) error
    FindByToken(ctx context.Context, token string) (*entity.Share, error)
    FindByUser(ctx context.Context, userID uint, page, pageSize int) ([]*entity.Share, int64, error)
    IncrementDownloadCount(ctx context.Context, id uint) error
}
```

---

## 4. 应用层 (Application Layer)

应用层负责**编排领域对象**完成具体用例，不包含业务规则，只负责流程控制。

### 4.1 DTO (Data Transfer Objects)

```go
// internal/application/dto/auth_dto.go
package dto

// LoginRequest 登录请求
type LoginRequest struct {
    Username string `json:"username" binding:"required,min=3,max=64"`
    Password string `json:"password" binding:"required,min=8"`
}

// LoginResponse 登录响应
type LoginResponse struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int    `json:"expires_in"` // 秒
    User         UserInfo `json:"user"`
}

// UserInfo 用户信息 DTO
type UserInfo struct {
    ID       uint   `json:"id"`
    Username string `json:"username"`
    Email    string `json:"email"`
    Role     string `json:"role"`
}

// RefreshTokenRequest 刷新 Token 请求
type RefreshTokenRequest struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}
```

```go
// internal/application/dto/file_dto.go
package dto

// ListFilesRequest 列出文件请求
type ListFilesRequest struct {
    SourceID uint   `form:"source_id" binding:"required"`
    Path     string `form:"path" binding:"required"`
    Page     int    `form:"page,default=1"`
    PageSize int    `form:"page_size,default=200"`
    SortBy   string `form:"sort_by,default=modified_at"` // name/size/modified_at
    SortOrder string `form:"sort_order,default=desc"`    // asc/desc
}

// ListFilesResponse 列出文件响应
type ListFilesResponse struct {
    Items      []FileItem `json:"items"`
    Pagination Pagination `json:"pagination"`
}

// FileItem 文件项 DTO
type FileItem struct {
    Name       string `json:"name"`
    Path       string `json:"path"`
    Size       int64  `json:"size"`
    IsDir      bool   `json:"is_dir"`
    MimeType   string `json:"mime_type"`
    ModifiedAt string `json:"modified_at"`
}

// UploadInitRequest 上传初始化请求
type UploadInitRequest struct {
    SourceID uint   `json:"source_id" binding:"required"`
    Path     string `json:"path" binding:"required"`
    Filename string `json:"filename" binding:"required"`
    FileSize int64  `json:"file_size" binding:"required,min=1"`
    FileHash string `json:"file_hash"` // MD5，可选，用于秒传
}

// UploadInitResponse 上传初始化响应
type UploadInitResponse struct {
    UploadID      string      `json:"upload_id"`
    ChunkSize     int64       `json:"chunk_size"`
    TotalChunks   int         `json:"total_chunks"`
    UploadedChunks []int      `json:"uploaded_chunks"` // 已上传的块
    PresignedURLs []ChunkURL  `json:"presigned_urls,omitempty"` // S3 直传用
    IsFastUpload  bool        `json:"is_fast_upload"` // 是否秒传成功
}

// ChunkURL 分片 URL
type ChunkURL struct {
    Index int    `json:"index"`
    URL   string `json:"url"`
}

// UploadChunkRequest 上传分片请求（本地磁盘）
type UploadChunkRequest struct {
    UploadID string `form:"upload_id" binding:"required"`
    Index    int    `form:"index" binding:"required,min=0"`
}
```

### 4.2 应用服务 (Application Services)

```go
// internal/application/service/file_app_svc.go
package service

import (
    "context"
    "fmt"
    
    "yunxia/internal/application/dto"
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
    "yunxia/internal/domain/service"
    "yunxia/internal/domain/valueobject"
    "yunxia/internal/infrastructure/storage"
)

// FileApplicationService 文件应用服务
type FileApplicationService struct {
    fileRepo    repository.FileRepository
    sourceRepo  repository.StorageSourceRepository
    aclService  *service.ACLService
    driverMgr   *storage.DriverManager
}

func NewFileApplicationService(
    fileRepo repository.FileRepository,
    sourceRepo repository.StorageSourceRepository,
    aclService *service.ACLService,
    driverMgr *storage.DriverManager,
) *FileApplicationService {
    return &FileApplicationService{
        fileRepo:   fileRepo,
        sourceRepo: sourceRepo,
        aclService: aclService,
        driverMgr:  driverMgr,
    }
}

// ListFiles 列出文件用例
func (s *FileApplicationService) ListFiles(
    ctx context.Context,
    userID uint,
    req dto.ListFilesRequest,
) (*dto.ListFilesResponse, error) {
    // 1. 检查权限
    hasPerm, err := s.aclService.CheckPermission(
        userID, req.SourceID, req.Path,
        valueobject.Permission{Read: true},
    )
    if err != nil {
        return nil, err
    }
    if !hasPerm {
        return nil, fmt.Errorf("access denied")
    }
    
    // 2. 获取存储源
    source, err := s.sourceRepo.FindByID(ctx, req.SourceID)
    if err != nil {
        return nil, err
    }
    
    // 3. 获取驱动
    driver, err := s.driverMgr.GetDriver(source.DriverType, source.Config)
    if err != nil {
        return nil, err
    }
    
    // 4. 调用驱动列出文件
    fileInfos, err := driver.List(ctx, req.Path)
    if err != nil {
        return nil, err
    }
    
    // 5. 过滤无权访问的子项（ACL）
    var filtered []*valueobject.FileInfo
    for _, info := range fileInfos {
        childPath := fmt.Sprintf("%s/%s", req.Path, info.Name)
        hasPerm, _ := s.aclService.CheckPermission(
            userID, req.SourceID, childPath,
            valueobject.Permission{Read: true},
        )
        if hasPerm {
            filtered = append(filtered, info)
        }
    }
    
    // 6. 转换为 DTO
    var items []dto.FileItem
    for _, info := range filtered {
        items = append(items, dto.FileItem{
            Name:       info.Name,
            Path:       info.Path,
            Size:       info.Size,
            IsDir:      info.IsDir,
            MimeType:   info.MimeType,
            ModifiedAt: info.ModifiedAt.Format("2006-01-02T15:04:05"),
        })
    }
    
    return &dto.ListFilesResponse{
        Items: items,
        Pagination: dto.Pagination{
            Page:       req.Page,
            PageSize:   req.PageSize,
            Total:      int64(len(items)),
            TotalPages: 1, // 驱动层返回全部，前端做虚拟滚动
        },
    }, nil
}
```

```go
// internal/application/service/upload_app_svc.go
package service

import (
    "context"
    "fmt"
    "time"
    
    "github.com/google/uuid"
    
    "yunxia/internal/application/dto"
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
    "yunxia/internal/infrastructure/storage"
)

const (
    DefaultChunkSize = 5 * 1024 * 1024 // 5MB
    UploadExpireDays = 7
)

// UploadApplicationService 上传应用服务
type UploadApplicationService struct {
    uploadRepo repository.UploadRepository
    sourceRepo repository.StorageSourceRepository
    fileRepo   repository.FileRepository
    driverMgr  *storage.DriverManager
}

func NewUploadApplicationService(
    uploadRepo repository.UploadRepository,
    sourceRepo repository.StorageSourceRepository,
    fileRepo repository.FileRepository,
    driverMgr *storage.DriverManager,
) *UploadApplicationService {
    return &UploadApplicationService{
        uploadRepo: uploadRepo,
        sourceRepo: sourceRepo,
        fileRepo:   fileRepo,
        driverMgr:  driverMgr,
    }
}

// InitUpload 初始化上传（含秒传检查）
func (s *UploadApplicationService) InitUpload(
    ctx context.Context,
    userID uint,
    req dto.UploadInitRequest,
) (*dto.UploadInitResponse, error) {
    // 1. 秒传检查
    if req.FileHash != "" {
        existing, err := s.fileRepo.FindByPath(ctx, req.SourceID, 
            fmt.Sprintf("%s/%s", req.Path, req.Filename))
        // 简化：实际应通过 hash + size 匹配
        _ = existing
        _ = err
    }
    
    // 2. 计算分片
    totalChunks := int((req.FileSize + DefaultChunkSize - 1) / DefaultChunkSize)
    
    // 3. 创建上传会话
    session := &entity.UploadSession{
        ID:           uuid.New().String(),
        UserID:       userID,
        SourceID:     req.SourceID,
        TargetPath:   req.Path,
        Filename:     req.Filename,
        FileSize:     req.FileSize,
        FileHash:     req.FileHash,
        Status:       entity.UploadPending,
        ChunkSize:    DefaultChunkSize,
        TotalChunks:  totalChunks,
        CompletedChunks: []int{},
        ExpiresAt:    time.Now().Add(UploadExpireDays * 24 * time.Hour),
    }
    
    if err := s.uploadRepo.Create(ctx, session); err != nil {
        return nil, err
    }
    
    return &dto.UploadInitResponse{
        UploadID:      session.ID,
        ChunkSize:     session.ChunkSize,
        TotalChunks:   session.TotalChunks,
        UploadedChunks: []int{},
    }, nil
}
```

### 4.3 Assembler（DTO ↔ Entity 转换）

```go
// internal/application/assembler/file_assembler.go
package assembler

import (
    "yunxia/internal/application/dto"
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/valueobject"
)

// ToFileItemDTO 将 FileInfo 值对象转为 DTO
func ToFileItemDTO(info *valueobject.FileInfo) dto.FileItem {
    return dto.FileItem{
        Name:       info.Name,
        Path:       info.Path,
        Size:       info.Size,
        IsDir:      info.IsDir,
        MimeType:   info.MimeType,
        ModifiedAt: info.ModifiedAt.Format("2006-01-02T15:04:05"),
    }
}

// ToUserInfoDTO 将 User 实体转为 DTO
func ToUserInfoDTO(user *entity.User) dto.UserInfo {
    return dto.UserInfo{
        ID:       user.ID,
        Username: user.Username,
        Email:    user.Email,
        Role:     string(user.Role),
    }
}
```

---

## 5. 基础设施层 (Infrastructure Layer)

基础设施层实现领域层定义的接口，提供具体的技术实现。

### 5.1 持久化实现 (GORM)

```go
// internal/infrastructure/persistence/gorm/db.go
package gorm

import (
    "fmt"
    "time"
    
    "gorm.io/driver/postgres"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
    
    "yunxia/internal/infrastructure/config"
)

// NewDB 创建数据库连接
func NewDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
    var dialector gorm.Dialector
    
    switch cfg.Type {
    case "sqlite":
        dialector = sqlite.Open(cfg.DSN + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
    case "postgresql":
        dialector = postgres.Open(cfg.DSN)
    default:
        return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
    }
    
    gormLogger := logger.Default.LogMode(logger.Silent)
    if cfg.Debug {
        gormLogger = logger.Default.LogMode(logger.Info)
    }
    
    db, err := gorm.Open(dialector, &gorm.Config{
        Logger: gormLogger,
    })
    if err != nil {
        return nil, err
    }
    
    // 连接池配置
    sqlDB, err := db.DB()
    if err != nil {
        return nil, err
    }
    
    sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
    sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
    sqlDB.SetConnMaxLifetime(time.Hour)
    
    return db, nil
}
```

```go
// internal/infrastructure/persistence/gorm/user_repo_impl.go
package gorm

import (
    "context"
    
    "gorm.io/gorm"
    
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
)

// userRepositoryImpl 用户仓库 GORM 实现
type userRepositoryImpl struct {
    db *gorm.DB
}

// 编译时接口检查
var _ repository.UserRepository = (*userRepositoryImpl)(nil)

func NewUserRepository(db *gorm.DB) repository.UserRepository {
    return &userRepositoryImpl{db: db}
}

func (r *userRepositoryImpl) Create(ctx context.Context, user *entity.User) error {
    return r.db.WithContext(ctx).Create(user).Error
}

func (r *userRepositoryImpl) Update(ctx context.Context, user *entity.User) error {
    return r.db.WithContext(ctx).Save(user).Error
}

func (r *userRepositoryImpl) Delete(ctx context.Context, id uint) error {
    return r.db.WithContext(ctx).Delete(&entity.User{}, id).Error
}

func (r *userRepositoryImpl) FindByID(ctx context.Context, id uint) (*entity.User, error) {
    var user entity.User
    err := r.db.WithContext(ctx).First(&user, id).Error
    if err == gorm.ErrRecordNotFound {
        return nil, nil
    }
    return &user, err
}

func (r *userRepositoryImpl) FindByUsername(ctx context.Context, username string) (*entity.User, error) {
    var user entity.User
    err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
    if err == gorm.ErrRecordNotFound {
        return nil, nil
    }
    return &user, err
}

func (r *userRepositoryImpl) FindByUsernameWithPassword(ctx context.Context, username string) (*entity.User, error) {
    var user entity.User
    err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
    if err == gorm.ErrRecordNotFound {
        return nil, nil
    }
    return &user, err
}

func (r *userRepositoryImpl) List(ctx context.Context, page, pageSize int) ([]*entity.User, int64, error) {
    var users []*entity.User
    var total int64
    
    offset := (page - 1) * pageSize
    
    err := r.db.WithContext(ctx).Model(&entity.User{}).Count(&total).Error
    if err != nil {
        return nil, 0, err
    }
    
    err = r.db.WithContext(ctx).Offset(offset).Limit(pageSize).Find(&users).Error
    return users, total, err
}

func (r *userRepositoryImpl) Exists(ctx context.Context, username string) (bool, error) {
    var count int64
    err := r.db.WithContext(ctx).Model(&entity.User{}).Where("username = ?", username).Count(&count).Error
    return count > 0, err
}

func (r *userRepositoryImpl) Count(ctx context.Context) (int64, error) {
    var count int64
    err := r.db.WithContext(ctx).Model(&entity.User{}).Count(&count).Error
    return count, err
}
```

### 5.2 存储驱动实现

```go
// internal/infrastructure/storage/driver.go
package storage

import (
    "context"
    "fmt"
    "io"
    "time"
)

// Driver 存储驱动接口
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

// FileInfo 文件信息
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

// DriverFactory 驱动工厂函数
type DriverFactory func() Driver

// registry 驱动注册表
var registry = make(map[string]DriverFactory)

// Register 注册驱动
func Register(name string, factory DriverFactory) {
    registry[name] = factory
}

// NewDriver 创建驱动实例
func NewDriver(name string) (Driver, error) {
    factory, ok := registry[name]
    if !ok {
        return nil, fmt.Errorf("driver not found: %s", name)
    }
    return factory(), nil
}

// ListDrivers 列出所有已注册驱动
func ListDrivers() []string {
    var names []string
    for name := range registry {
        names = append(names, name)
    }
    return names
}

// DriverManager 驱动管理器
type DriverManager struct {
    drivers map[string]Driver // 缓存已初始化的驱动实例
}

func NewDriverManager() *DriverManager {
    return &DriverManager{
        drivers: make(map[string]Driver),
    }
}

// GetDriver 获取驱动实例（带缓存）
func (m *DriverManager) GetDriver(driverType string, config map[string]interface{}) (Driver, error) {
    cacheKey := fmt.Sprintf("%s:%v", driverType, config)
    
    if driver, ok := m.drivers[cacheKey]; ok {
        return driver, nil
    }
    
    driver, err := NewDriver(driverType)
    if err != nil {
        return nil, err
    }
    
    if err := driver.Init(config); err != nil {
        return nil, err
    }
    
    m.drivers[cacheKey] = driver
    return driver, nil
}
```

### 5.3 下载器抽象与 Aria2 默认实现

```go
// internal/infrastructure/downloader/downloader.go
package downloader

import "context"

// Downloader 下载引擎抽象
// 应用层仅依赖该接口，MVP 默认实现为 Aria2Client
type Downloader interface {
    AddURI(ctx context.Context, uris []string, options map[string]interface{}) (string, error)
    TellStatus(ctx context.Context, gid string) (*DownloadStatus, error)
    Pause(ctx context.Context, gid string) error
    Resume(ctx context.Context, gid string) error
    Remove(ctx context.Context, gid string) error
}
```

```go
// internal/infrastructure/downloader/aria2_client.go
package downloader

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    
    "yunxia/internal/domain/entity"
)

// Aria2Client Aria2 JSON-RPC 客户端
// 默认 Downloader 实现
type Aria2Client struct {
    rpcURL string
    secret string
    client *http.Client
}

var _ Downloader = (*Aria2Client)(nil)

// NewAria2Client 创建 Aria2 客户端
func NewAria2Client(rpcURL, secret string) *Aria2Client {
    return &Aria2Client{
        rpcURL: rpcURL,
        secret: secret,
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

// AddURI 添加下载任务（HTTP/BT/Magnet）
func (c *Aria2Client) AddURI(ctx context.Context, uris []string, options map[string]interface{}) (string, error) {
    req := &RPCRequest{
        JSONRPC: "2.0",
        ID:      "1",
        Method:  "aria2.addUri",
        Params:  c.buildParams(uris, options),
    }
    
    resp, err := c.call(ctx, req)
    if err != nil {
        return "", err
    }
    
    // 返回 GID（任务 ID）
    gid, ok := resp.Result.(string)
    if !ok {
        return "", fmt.Errorf("invalid response")
    }
    return gid, nil
}

// TellStatus 获取任务状态
func (c *Aria2Client) TellStatus(ctx context.Context, gid string) (*DownloadStatus, error) {
    req := &RPCRequest{
        JSONRPC: "2.0",
        ID:      "1",
        Method:  "aria2.tellStatus",
        Params:  []interface{}{c.secretPrefix(), gid},
    }
    
    resp, err := c.call(ctx, req)
    if err != nil {
        return nil, err
    }
    
    // 解析状态...
    return parseStatus(resp.Result)
}

// 内部方法...

func (c *Aria2Client) secretPrefix() string {
    if c.secret != "" {
        return fmt.Sprintf("token:%s", c.secret)
    }
    return ""
}

func (c *Aria2Client) buildParams(uris []string, options map[string]interface{}) []interface{} {
    params := []interface{}{c.secretPrefix(), uris}
    if options != nil {
        params = append(params, options)
    }
    return params
}

// RPCRequest JSON-RPC 请求
type RPCRequest struct {
    JSONRPC string        `json:"jsonrpc"`
    ID      string        `json:"id"`
    Method  string        `json:"method"`
    Params  []interface{} `json:"params"`
}

// RPCResponse JSON-RPC 响应
type RPCResponse struct {
    ID      string      `json:"id"`
    Result  interface{} `json:"result"`
    Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

// DownloadStatus 下载状态
type DownloadStatus struct {
    GID         string
    Status      string // active/waiting/paused/error/complete/removed
    TotalLength int64
    CompletedLength int64
    DownloadSpeed int64
    Dir         string
    Files       []DownloadFile
}

type DownloadFile struct {
    Path string
    URI  string
}
```

### 5.4 任务队列实现

```go
// internal/infrastructure/mq/sqlite_queue.go
package mq

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"
    
    "gorm.io/gorm"
    
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
)

// SQLiteTaskQueue SQLite 持久化任务队列
type SQLiteTaskQueue struct {
    db       *gorm.DB
    repo     repository.TaskRepository
    handlers map[string]TaskHandler
    workers  int
    wg       sync.WaitGroup
    stopCh   chan struct{}
    mu       sync.RWMutex
}

// TaskHandler 任务处理器函数
type TaskHandler func(ctx context.Context, task *entity.Task) error

// NewSQLiteTaskQueue 创建队列
func NewSQLiteTaskQueue(db *gorm.DB, repo repository.TaskRepository, workers int) *TaskQueue {
    if workers <= 0 {
        workers = 3
    }
    return &SQLiteTaskQueue{
        db:       db,
        repo:     repo,
        handlers: make(map[string]TaskHandler),
        workers:  workers,
        stopCh:   make(chan struct{}),
    }
}

// RegisterHandler 注册处理器
func (q *SQLiteTaskQueue) RegisterHandler(taskType string, handler TaskHandler) {
    q.mu.Lock()
    defer q.mu.Unlock()
    q.handlers[taskType] = handler
}

// Submit 提交任务
func (q *SQLiteTaskQueue) Submit(ctx context.Context, task *entity.Task) error {
    if err := q.repo.Create(ctx, task); err != nil {
        return err
    }
    return nil
}

// Start 启动消费
func (q *SQLiteTaskQueue) Start(ctx context.Context) error {
    for i := 0; i < q.workers; i++ {
        q.wg.Add(1)
        go q.worker(ctx, i)
    }
    return nil
}

// Stop 停止
func (q *SQLiteTaskQueue) Stop() {
    close(q.stopCh)
    q.wg.Wait()
}

func (q *SQLiteTaskQueue) worker(ctx context.Context, id int) {
    defer q.wg.Done()
    
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-q.stopCh:
            return
        case <-ticker.C:
            q.processOne(ctx)
        }
    }
}

func (q *SQLiteTaskQueue) processOne(ctx context.Context) {
    // 1. 获取一个待处理任务
    tasks, err := q.repo.FindPending(ctx, 1)
    if err != nil || len(tasks) == 0 {
        return
    }
    
    task := tasks[0]
    
    // 2. 更新为运行中
    task.Status = entity.TaskRunning
    task.StartedAt = &[]time.Time{time.Now()}[0]
    q.repo.Update(ctx, task)
    
    // 3. 查找处理器
    q.mu.RLock()
    handler, ok := q.handlers[task.Type]
    q.mu.RUnlock()
    
    if !ok {
        task.Status = entity.TaskFailed
        task.ErrorMsg = fmt.Sprintf("no handler for type: %s", task.Type)
        q.repo.Update(ctx, task)
        return
    }
    
    // 4. 执行
    if err := handler(ctx, task); err != nil {
        task.Status = entity.TaskFailed
        task.ErrorMsg = err.Error()
        task.RetryCount++
        if task.CanRetry() {
            task.Status = entity.TaskPending
            task.ScheduledAt = &[]time.Time{time.Now().Add(time.Duration(task.RetryCount) * time.Minute)}[0]
        }
    } else {
        task.Status = entity.TaskCompleted
        now := time.Now()
        task.CompletedAt = &now
    }
    
    q.repo.Update(ctx, task)
}
```

---

## 6. 接口适配层 (Interface Layer)

接口层负责接收外部请求，调用应用服务，返回响应。

### 6.1 HTTP Handler

```go
// internal/interfaces/http/handler/auth_handler.go
package handler

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
    
    "yunxia/internal/application/dto"
    "yunxia/internal/application/service"
)

// AuthHandler 认证 Handler
type AuthHandler struct {
    authAppSvc *service.AuthApplicationService
}

func NewAuthHandler(authAppSvc *service.AuthApplicationService) *AuthHandler {
    return &AuthHandler{authAppSvc: authAppSvc}
}

// Login 登录
func (h *AuthHandler) Login(c *gin.Context) {
    var req dto.LoginRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
        return
    }
    
    resp, err := h.authAppSvc.Login(c.Request.Context(), req)
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
}

// RefreshToken 刷新 Token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
    var req dto.RefreshTokenRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
        return
    }
    
    resp, err := h.authAppSvc.RefreshToken(c.Request.Context(), req)
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
}

// Me 获取当前用户
func (h *AuthHandler) Me(c *gin.Context) {
    userID, exists := c.Get("userID")
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
        return
    }
    
    user, err := h.authAppSvc.GetUserByID(c.Request.Context(), userID.(uint))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"code": 0, "data": user})
}
```

### 6.2 路由注册

```go
// internal/interfaces/http/router.go
package http

import (
    "github.com/gin-gonic/gin"
    
    "yunxia/internal/interfaces/http/handler"
    "yunxia/internal/interfaces/middleware"
)

// Router HTTP 路由
type Router struct {
    engine *gin.Engine
}

func NewRouter(
    authHandler *handler.AuthHandler,
    fileHandler *handler.FileHandler,
    uploadHandler *handler.UploadHandler,
    sourceHandler *handler.SourceHandler,
    taskHandler *handler.TaskHandler,
    systemHandler *handler.SystemHandler,
    authMiddleware *middleware.AuthMiddleware,
    rateLimiter *middleware.RateLimiter,
) *Router {
    r := gin.New()
    
    // 全局中间件
    r.Use(gin.Recovery())
    r.Use(middleware.SecurityHeaders())
    r.Use(middleware.RequestLogger())
    r.Use(middleware.CORS())
    
    // API v1
    api := r.Group("/api/v1")
    {
        // 健康检查（公开）
        api.GET("/health", systemHandler.Health) // 最终路由：GET /api/v1/health

        // 认证（公开）
        auth := api.Group("/auth")
        {
            auth.POST("/login", rateLimiter.LoginLimit(), authHandler.Login)
            auth.POST("/refresh", authHandler.RefreshToken)
        }
        
        // 需要认证
        authorized := api.Group("")
        authorized.Use(authMiddleware.RequireAuth())
        {
            authorized.GET("/auth/me", authHandler.Me)
            authorized.POST("/auth/logout", authHandler.Logout)
            
            // 文件管理
            files := authorized.Group("/files")
            {
                files.GET("", fileHandler.List)
                files.GET("/search", fileHandler.Search)
                files.POST("/mkdir", fileHandler.Mkdir)
                files.POST("/move", fileHandler.Move)
                files.POST("/copy", fileHandler.Copy)
                files.DELETE("", fileHandler.Delete)
                files.GET("/download", fileHandler.Download)
            }
            
            // 上传
            upload := authorized.Group("/upload")
            {
                upload.POST("/init", uploadHandler.Init)
                upload.PUT("/chunk", uploadHandler.UploadChunk)
                upload.POST("/finish", uploadHandler.Finish)
            }
            
            // 存储源管理
            sources := authorized.Group("/sources")
            {
                sources.GET("", sourceHandler.List)
                sources.POST("", sourceHandler.Create)
                sources.GET("/:id", sourceHandler.Get)
                sources.PUT("/:id", sourceHandler.Update)
                sources.DELETE("/:id", sourceHandler.Delete)
                sources.POST("/:id/test", sourceHandler.Test)
            }
            
            // 任务
            tasks := authorized.Group("/tasks")
            {
                tasks.GET("", taskHandler.List)
                tasks.POST("", taskHandler.Create)
                tasks.GET("/:id", taskHandler.Get)
                tasks.DELETE("/:id", taskHandler.Cancel)
            }
            
            // 系统
            authorized.GET("/system/config", systemHandler.GetConfig)
            authorized.PUT("/system/config", systemHandler.UpdateConfig)
            authorized.GET("/system/stats", systemHandler.GetStats)
            authorized.GET("/system/version", systemHandler.GetVersion)
        }
    }
    
    // WebDAV（独立认证）
    // dav := r.Group("/dav")
    // dav.Use(webdavAuthMiddleware)
    // ...
    
    // 前端静态文件
    r.Static("/", "./web/dist")
    
    return &Router{engine: r}
}

func (r *Router) Engine() *gin.Engine {
    return r.engine
}
```

### 6.3 中间件

```go
// internal/interfaces/middleware/auth.go
package middleware

import (
    "net/http"
    "strings"
    
    "github.com/gin-gonic/gin"
    
    "yunxia/internal/application/service"
)

// AuthMiddleware JWT 认证中间件
type AuthMiddleware struct {
    authAppSvc *service.AuthApplicationService
}

func NewAuthMiddleware(authAppSvc *service.AuthApplicationService) *AuthMiddleware {
    return &AuthMiddleware{authAppSvc: authAppSvc}
}

func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "missing authorization header"})
            return
        }
        
        parts := strings.SplitN(authHeader, " ", 2)
        if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid authorization header"})
            return
        }
        
        token := parts[1]
        claims, err := m.authAppSvc.ValidateAccessToken(token)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": err.Error()})
            return
        }
        
        c.Set("userID", claims.UserID)
        c.Set("username", claims.Username)
        c.Set("role", claims.Role)
        c.Next()
    }
}
```

```go
// internal/interfaces/middleware/rate_limit.go
package middleware

import (
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    "golang.org/x/time/rate"
)

// RateLimiter 限流中间件
type RateLimiter struct {
    loginLimiter *rate.Limiter
    apiLimiter   *rate.Limiter
    davLimiter   *rate.Limiter
}

func NewRateLimiter() *RateLimiter {
    return &RateLimiter{
        loginLimiter: rate.NewLimiter(rate.Every(time.Minute), 5),  // 登录：每分5次
        apiLimiter:   rate.NewLimiter(rate.Every(time.Second), 100), // API：每秒100次
        davLimiter:   rate.NewLimiter(rate.Every(time.Second), 50),  // WebDAV：每秒50次
    }
}

func (rl *RateLimiter) LoginLimit() gin.HandlerFunc {
    return func(c *gin.Context) {
        if !rl.loginLimiter.Allow() {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "code": 429, 
                "message": "too many login attempts, please try again later",
            })
            return
        }
        c.Next()
    }
}

func (rl *RateLimiter) APILimit() gin.HandlerFunc {
    return func(c *gin.Context) {
        if !rl.apiLimiter.Allow() {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "rate limit exceeded"})
            return
        }
        c.Next()
    }
}

func (rl *RateLimiter) DAVLimit() gin.HandlerFunc {
    return func(c *gin.Context) {
        if !rl.davLimiter.Allow() {
            c.Header("Retry-After", "1")
            c.AbortWithStatus(http.StatusTooManyRequests)
            return
        }
        c.Next()
    }
}
```

---

## 7. 数据库设计

### 7.1 完整表结构

```sql
-- 用户表
CREATE TABLE users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        VARCHAR(64) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    email           VARCHAR(255),
    role            VARCHAR(20) NOT NULL DEFAULT 'user',
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    storage_quota   BIGINT DEFAULT 0,
    token_version   INTEGER DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 存储源表
CREATE TABLE storage_sources (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    name                VARCHAR(128) NOT NULL,
    driver_type         VARCHAR(32) NOT NULL,
    config              TEXT NOT NULL,
    root_path           VARCHAR(512) DEFAULT '',
    is_enabled          BOOLEAN DEFAULT 1,
    is_webdav_exposed   BOOLEAN DEFAULT 0,
    webdav_readonly     BOOLEAN DEFAULT 0,
    sort_order          INTEGER DEFAULT 0,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 文件元数据缓存表
CREATE TABLE file_metadata (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id   INTEGER NOT NULL,
    path        VARCHAR(4096) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    parent_path VARCHAR(4096) NOT NULL,
    is_dir      BOOLEAN NOT NULL DEFAULT 0,
    size        BIGINT DEFAULT 0,
    mime_type   VARCHAR(128),
    checksum    VARCHAR(64),
    modified_at DATETIME,
    created_at  DATETIME,
    cached_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    extra       TEXT,
    UNIQUE(source_id, path)
);
CREATE INDEX idx_file_parent ON file_metadata(source_id, parent_path);
CREATE INDEX idx_file_name ON file_metadata(name);

-- 本地文件索引表（FTS5 全文搜索，P1 预留）
CREATE VIRTUAL TABLE local_file_index_fts USING fts5(
    name, 
    content='file_metadata',
    content_rowid='id'
);

-- 上传会话表
CREATE TABLE upload_sessions (
    id                  VARCHAR(64) PRIMARY KEY,
    user_id             INTEGER NOT NULL,
    source_id           INTEGER NOT NULL,
    target_path         VARCHAR(4096) NOT NULL,
    filename            VARCHAR(255) NOT NULL,
    file_size           BIGINT NOT NULL,
    file_hash           VARCHAR(64),
    status              VARCHAR(20) DEFAULT 'pending',
    chunk_size          INTEGER DEFAULT 5242880,
    total_chunks        INTEGER NOT NULL,
    uploaded_chunks     INTEGER DEFAULT 0,
    completed_chunks    TEXT DEFAULT '[]',
    storage_data        TEXT,
    expires_at          DATETIME NOT NULL,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_upload_user ON upload_sessions(user_id, status);
CREATE INDEX idx_upload_expires ON upload_sessions(expires_at);

-- ACL 权限表
CREATE TABLE acl_entries (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL,
    source_id   INTEGER NOT NULL,
    path        VARCHAR(4096) NOT NULL DEFAULT '/',
    perm_read   BOOLEAN DEFAULT 1,
    perm_write  BOOLEAN DEFAULT 0,
    perm_delete BOOLEAN DEFAULT 0,
    perm_share  BOOLEAN DEFAULT 0,
    rule_type   VARCHAR(20) DEFAULT 'allow',
    inherit     BOOLEAN DEFAULT 1,
    priority    INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, source_id, path)
);
CREATE INDEX idx_acl_user ON acl_entries(user_id, source_id);
CREATE INDEX idx_acl_path ON acl_entries(source_id, path);

-- 分享表（P1 预留）
CREATE TABLE shares (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL,
    source_id       INTEGER NOT NULL,
    path            VARCHAR(4096) NOT NULL,
    token           VARCHAR(64) NOT NULL UNIQUE,
    password_hash   VARCHAR(255),
    expires_at      DATETIME,
    max_downloads   INTEGER DEFAULT 0,
    download_count  INTEGER DEFAULT 0,
    is_enabled      BOOLEAN DEFAULT 1,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_share_token ON shares(token);

-- 任务队列表
CREATE TABLE tasks (
    id              VARCHAR(64) PRIMARY KEY,
    task_type       VARCHAR(32) NOT NULL,
    status          VARCHAR(20) DEFAULT 'pending',
    payload         TEXT NOT NULL,
    result          TEXT,
    error_msg       TEXT,
    priority        INTEGER DEFAULT 0,
    scheduled_at    DATETIME,
    started_at      DATETIME,
    completed_at    DATETIME,
    retry_count     INTEGER DEFAULT 0,
    max_retries     INTEGER DEFAULT 3,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_task_status ON tasks(status, scheduled_at);
CREATE INDEX idx_task_type ON tasks(task_type, status);

-- 操作日志表
-- 操作日志表（P1 预留查询/导出能力）
CREATE TABLE operation_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER,
    action      VARCHAR(32) NOT NULL,
    source_id   INTEGER,
    path        VARCHAR(4096),
    ip_address  VARCHAR(45),
    user_agent  VARCHAR(512),
    status      VARCHAR(20) DEFAULT 'success',
    error_msg   TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_log_user ON operation_logs(user_id, created_at);
CREATE INDEX idx_log_action ON operation_logs(action, created_at);

-- 系统配置表
CREATE TABLE system_configs (
    key         VARCHAR(128) PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 8. 关键业务流程

### 8.1 文件上传（断点续传）流程

```
用户选择文件
    │
    ▼
[前端] 计算 MD5 + 文件大小
    │
    ├──▶ POST /api/v1/upload/init
    │       {filename, size, hash, source_id, path}
    │
    ▼
[应用层] UploadAppSvc.InitUpload()
    │
    ├── 秒传检查（hash + size 匹配？）
    │       ├── YES ──▶ 秒传成功，返回 is_fast_upload=true
    │       └── NO ──▶ 创建 UploadSession
    │
    ▼
返回 {upload_id, chunk_size, total_chunks, uploaded_chunks}
    │
    ▼
[前端] 并发上传 chunks（3 并发）
    │
    ├──▶ PUT /api/v1/upload/chunk (本地磁盘)
    │   或 PUT presigned_url (S3)
    │
    ▼
[应用层] 接收 chunk，写入临时文件 / 直传 S3
    │
    ├── 更新 upload_sessions（completed_chunks）
    │
    ▼
所有 chunks 完成
    │
    ├──▶ POST /api/v1/upload/finish
    │
    ▼
[应用层] 合并临时文件 / S3 CompleteMultipart
    │
    ├── 写入 file_metadata
    ├── 清理临时文件
    └── 删除 upload_session
    │
    ▼
上传完成
```

### 8.2 离线下载流程

```
用户提交下载链接（HTTP/BT/Magnet）
    │
    ├──▶ POST /api/v1/tasks
    │       {type: "download", payload: {url, save_path, source_id}}
    │
    ▼
[应用层] TaskAppSvc.CreateTask()
    │
    ├── 创建 Task 实体
    ├── 提交到任务队列
    │
    ▼
[任务队列] 获取 pending 任务
    │
    ├── 调用 Downloader.AddURI()
    │       └── MVP 默认实现为 Aria2Client，经 JSON-RPC 提交到 Aria2
    │
    ▼
[Aria2] 下载中...
    │
    ├── 定期查询 TellStatus()
    │
    ▼
下载完成
    │
    ├── 回调 / 轮询检测到完成
    ├── 移动文件到目标存储源目录
    ├── 写入 file_metadata
    └── 标记 Task 为 completed
    │
    ▼
通知用户（可选）
```

### 8.3 WebDAV 请求处理流程

```
Jellyfin 发送 PROPFIND /dav/local/电影/
    │
    ▼
[接口层] WebDAV Handler
    │
    ├── Basic Auth 认证（强制 HTTPS）
    ├── 解析路径 → source=local, path=/电影/
    │
    ▼
[应用层] WebDAVAppSvc.Propfind()
    │
    ├── ACL 检查：用户是否有 read 权限？
    │       └── NO ──▶ 403 Forbidden
    │
    ├── 检查缓存（PROPFIND 结果缓存 30s）
    │       ├── 命中 ──▶ 返回缓存
    │       └── 未命中
    │
    ▼
[领域层] 调用 Driver.List("/电影/")
    │
    ├── 本地磁盘：os.ReadDir()
    ├── S3：ListObjectsV2()
    │
    ▼
返回文件列表
    │
    ├── ACL 过滤（逐个检查子项权限）
    ├── 写入缓存
    │
    ▼
返回 WebDAV XML 响应
```

### 8.4 权限检查流程

```
用户请求 /api/v1/files?source_id=1&path=/工作/机密/
    │
    ▼
[中间件] JWT 认证通过
    │
    ▼
[应用层] FileAppSvc.ListFiles(userID=2, sourceID=1, path="/工作/机密/")
    │
    ├── 调用 ACLService.CheckPermission(userID=2, sourceID=1, "/工作/机密/", {read: true})
    │
    ▼
[领域层] ACLService
    │
    ├── 生成路径层级：["/工作/机密/", "/工作/", "/"]
    ├── 查询 ACL 表
    ├── 按优先级排序
    ├── 找到最匹配规则：
    │       /工作/机密/ → deny（无权限）
    │       /工作/      → allow + read（有权限）
    │       /           → allow + read+write（有权限）
    │
    ├── 最具体匹配：/工作/机密/ → deny
    │
    ▼
返回 false（拒绝访问）
    │
    ▼
返回 403 Forbidden
```

---

## 9. 配置体系

### 9.1 配置文件结构

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"  # debug / release

database:
  type: "sqlite"           # sqlite / postgresql
  dsn: "/data/database.db"  # SQLite 文件路径 或 PostgreSQL DSN
  max_open_conns: 10
  max_idle_conns: 5
  debug: false

jwt:
  secret: "your-secret-key-here"  # 最少 32 字节
  access_token_expire: 15m        # Access Token 过期时间
  refresh_token_expire: 168h      # Refresh Token 过期时间（7天）

storage:
  data_dir: "/data"
  temp_dir: "/data/temp"
  max_upload_size: 10737418240    # 10GB
  default_chunk_size: 5242880     # 5MB

webdav:
  enabled: true
  prefix: "/dav"
  cache_ttl: 30s

log:
  level: "info"              # debug / info / warn / error
  format: "json"             # json / text
  output: "stdout"           # stdout / file / both
  dir: "/data/logs"
  max_size: 100              # MB
  max_age: 30                # 天
  max_backups: 10

aria2:
  rpc_url: "http://aria2:6800/jsonrpc"
  rpc_secret: ""
  default_download_dir: "/downloads"

task_queue:
  workers: 3
  poll_interval: 5s

security:
  login_max_attempts: 5
  login_lock_duration: 15m
  bcrypt_cost: 12
```

### 9.2 配置加载优先级

```
环境变量（最高优先级）> 配置文件 > 默认值（最低优先级）

例如：
- 环境变量：YUNXIA_SERVER_PORT=9090
- 配置文件：server.port: 8080
- 默认值：8080

最终 port = 9090
```

---

## 10. 部署方案

### 10.1 Docker Compose（推荐）

```yaml
# docker-compose.yml
version: "3.8"

services:
  yunxia:
    image: yunxia/yunxia:latest
    container_name: yunxia
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - TZ=Asia/Shanghai
      - YUNXIA_DATABASE_TYPE=sqlite
      - YUNXIA_DATABASE_DSN=/data/database.db
      - YUNXIA_JWT_SECRET=${JWT_SECRET:-change-me-in-production}
      - YUNXIA_ARIA2_RPC_URL=http://aria2:6800/jsonrpc
      - YUNXIA_ARIA2_RPC_SECRET=${ARIA2_RPC_SECRET:-}
    depends_on:
      - aria2
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  aria2:
    image: p3terx/aria2-pro:latest
    container_name: yunxia-aria2
    restart: unless-stopped
    environment:
      - PUID=1000
      - PGID=1000
      - UMASK_SET=022
      - RPC_SECRET=${ARIA2_RPC_SECRET:-}
      - RPC_PORT=6800
      - LISTEN_PORT=6888
      - DISK_CACHE=64M
      - IPV6_MODE=false
      - UPDATE_TRACKERS=true
      - CUSTOM_TRACKER_URL=
      - TZ=Asia/Shanghai
    volumes:
      - ./downloads:/downloads
      - ./aria2-config:/config
    ports:
      - "6800:6800"
      - "6888:6888"
      - "6888:6888/udp"
```

### 10.2 纯二进制部署（外部 Aria2）

```bash
# 1. 启动 Aria2（用户自行部署）
aria2c --enable-rpc --rpc-listen-port=6800 --rpc-secret=your-secret

# 2. 启动云匣
export YUNXIA_ARIA2_RPC_URL=http://localhost:6800/jsonrpc
export YUNXIA_ARIA2_RPC_SECRET=your-secret
./yunxia-server
```

### 10.3 环境变量清单

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `YUNXIA_SERVER_HOST` | 监听地址 | `0.0.0.0` |
| `YUNXIA_SERVER_PORT` | 监听端口 | `8080` |
| `YUNXIA_DATABASE_TYPE` | 数据库类型 | `sqlite` |
| `YUNXIA_DATABASE_DSN` | 数据库连接 | `/data/database.db` |
| `YUNXIA_JWT_SECRET` | JWT 密钥 | **必须设置** |
| `YUNXIA_LOG_LEVEL` | 日志级别 | `info` |
| `YUNXIA_ARIA2_RPC_URL` | Aria2 RPC 地址 | `http://aria2:6800/jsonrpc` |
| `YUNXIA_ARIA2_RPC_SECRET` | Aria2 RPC 密钥 | `""` |

---

*本文档为云匣 (Yunxia) 的后端实现设计基线，遵循 DDD 分层架构（逻辑四层，工程习惯简称“三层”）。所有核心接口、实体与关键流程均以可实现为目标进行定义。*
