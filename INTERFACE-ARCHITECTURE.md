# 接口抽象与可替换性架构设计

> **版本**: v1.0  
> **日期**: 2026-04-20  
> **设计目标**: 严格面向接口编程，所有第三方依赖可插拔、可替换、可 Mock  
> **文档职责**: 共享抽象定义、依赖注入规范、可替换性边界  
> **对应文档**: `TAD.md` / `DESIGN.md`  
> **核心原则**: "依赖接口，不依赖实现"

---

## 目录

1. [设计哲学](#1-设计哲学)
2. [接口分层体系](#2-接口分层体系)
3. [核心接口定义](#3-核心接口定义)
4. [依赖注入容器](#4-依赖注入容器)
5. [可替换性实现矩阵](#5-可替换性实现矩阵)
6. [测试策略](#6-测试策略)
7. [新增/替换组件指南](#7-新增替换组件指南)
8. [相关文档](#8-相关文档)

---

## 1. 设计哲学

### 1.1 核心原则

```
┌─────────────────────────────────────────────────────────────┐
│                    面向接口编程宣言                           │
├─────────────────────────────────────────────────────────────┤
│  1. 任何对第三方库的调用，必须通过自定义接口转发                │
│  2. 业务代码只持有接口，不持有具体实现                         │
│  3. 接口定义在领域层或独立 pkg 中，实现放在基础设施层            │
│  4. 替换实现只需修改一行：NewXXX() 的返回值                    │
│  5. 测试时任意接口可用 Mock 实现替换                           │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 接口放置原则

| 接口类型 | 放置位置 | 理由 |
|---------|---------|------|
| **领域模型接口** | `internal/domain/repository/` | 领域层定义，基础设施实现 |
| **通用能力接口** | `pkg/` 或 `internal/infrastructure/*/interface.go` | 跨层复用，第三方可见 |
| **应用服务接口** | 不需要 | 应用服务直接由接口层调用 |

### 1.3 反模式清单

以下写法在代码审查时必须拒绝：

```go
// ❌ 反模式 1：业务代码直接依赖 GORM
func (s *UserService) CreateUser(ctx context.Context, user *User) error {
    return s.db.Create(user).Error  // 直接调用 GORM API
}

// ❌ 反模式 2：业务代码直接依赖 Redis 客户端
func (s *UserService) GetUser(ctx context.Context, id uint) (*User, error) {
    val, err := s.redisClient.Get(ctx, fmt.Sprintf("user:%d", id)).Result()
    // ...
}

// ❌ 反模式 3：业务代码直接依赖 Aria2 RPC 结构
func (s *TaskService) AddDownload(url string) {
    s.aria2Client.Call("aria2.addUri", []interface{}{nil, []string{url}})
}

// ❌ 反模式 4：new 关键字在业务层创建实现
func NewUserService() *UserService {
    return &UserService{
        db: gorm.Open(sqlite.Open("db.sqlite")),  // 硬编码
    }
}
```

---

## 2. 接口分层体系

### 2.1 完整接口拓扑

```
领域层接口（业务语义）
    ├── UserRepository          ← 用户数据访问
    ├── FileRepository          ← 文件元数据访问
    ├── StorageSourceRepository ← 存储源配置访问
    ├── UploadRepository        ← 上传会话访问
    ├── TaskRepository          ← 任务队列访问
    ├── ACLRepository           ← 权限数据访问
    └── ShareRepository         ← 分享数据访问

通用能力接口（技术能力）
    ├── DB              ← 数据库连接池抽象
    ├── Cache           ← 缓存能力抽象
    ├── Locker          ← 分布式锁抽象
    ├── Logger          ← 日志能力抽象
    ├── Config          ← 配置读取抽象
    ├── Hasher          ← 密码哈希抽象
    ├── TokenGenerator  ← Token 生成抽象
    └── Validator       ← 校验能力抽象

存储与外部服务接口（基础设施）
    ├── Driver          ← 存储驱动抽象
    ├── Downloader      ← 下载引擎抽象
    └── TaskQueue       ← 任务队列抽象
```

### 2.2 依赖方向

```
接口层 (HTTP/WebDAV Handler)
    │
    ├──▶ 应用服务 (Application Service)
    │       │
    │       ├──▶ 领域服务 (Domain Service)
    │       │       ├──▶ 领域接口 (Repository IF)
    │       │       │       ▲
    │       │       │       │ 实现
    │       │       │       └── 基础设施 (GORM / Redis / ...)
    │       │       │
    │       │       └──▶ 通用能力接口 (Cache / Logger / ...)
    │       │               ▲
    │       │               │ 实现
    │       │               └── 基础设施 (bigcache / zap / ...)
    │       │
    │       └──▶ 外部服务接口 (Driver / Downloader / ...)
    │               ▲
    │               │ 实现
    │               └── 基础设施 (LocalDriver / Aria2Client / ...)
    │
    └──▶ 通用能力接口 (Logger / Config)
```

---

## 3. 核心接口定义

### 3.1 数据库连接抽象（DB）

```go
// pkg/db/db.go
package db

import (
    "context"
    "database/sql"
)

// DB 数据库连接抽象
// 不暴露 GORM 或 sql.DB 的具体类型，只暴露业务需要的能力
type DB interface {
    // 执行原生 SQL（用于复杂查询）
    Exec(ctx context.Context, sql string, args ...interface{}) (Result, error)
    Query(ctx context.Context, sql string, args ...interface{}) (Rows, error)
    QueryRow(ctx context.Context, sql string, args ...interface{}) Row
    
    // 事务
    Transaction(ctx context.Context, fn func(Tx) error) error
    
    // 健康检查
    Ping(ctx context.Context) error
    
    // 关闭
    Close() error
}

// Result 执行结果
type Result interface {
    LastInsertId() (int64, error)
    RowsAffected() (int64, error)
}

// Rows 查询结果集
type Rows interface {
    Next() bool
    Scan(dest ...interface{}) error
    Close() error
}

// Row 单行查询
type Row interface {
    Scan(dest ...interface{}) error
}

// Tx 事务接口
type Tx interface {
    DB
    Commit() error
    Rollback() error
}
```

**GORM 适配实现**：

```go
// internal/infrastructure/persistence/gorm/db_adapter.go
package gorm

import (
    "context"
    "gorm.io/gorm"
    "yunxia/pkg/db"
    pkglock "yunxia/pkg/lock"
)

type gormDB struct {
    db *gorm.DB
}

// 编译时检查
var _ db.DB = (*gormDB)(nil)

func NewGormDB(dialector gorm.Dialector, cfg *gorm.Config) (db.DB, error) {
    gdb, err := gorm.Open(dialector, cfg)
    if err != nil {
        return nil, err
    }
    return &gormDB{db: gdb}, nil
}

func (d *gormDB) Exec(ctx context.Context, sql string, args ...interface{}) (db.Result, error) {
    result := d.db.WithContext(ctx).Exec(sql, args...)
    return &gormResult{result: result}, result.Error
}

func (d *gormDB) Transaction(ctx context.Context, fn func(db.Tx) error) error {
    return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        return fn(&gormTx{db: tx})
    })
}

// ... 其他方法实现
```

**替换路径**：
- SQLite → PostgreSQL：修改 `NewGormDB()` 的 dialector 参数
- GORM → 原生 database/sql：实现 `db.DB` 接口即可
- GORM → Ent：同样实现 `db.DB` 接口

### 3.2 缓存抽象（Cache）

```go
// pkg/cache/cache.go
package cache

import "time"

// Cache 缓存能力抽象
type Cache interface {
    // Get 获取缓存值
    Get(key string) (interface{}, bool)
    
    // Set 设置缓存值，ttl=0 表示永不过期
    Set(key string, value interface{}, ttl time.Duration) bool
    
    // GetString 获取字符串（便捷方法）
    GetString(key string) (string, bool)
    
    // SetString 设置字符串
    SetString(key string, value string, ttl time.Duration) bool
    
    // GetBytes 获取字节（便捷方法）
    GetBytes(key string) ([]byte, bool)
    
    // SetBytes 设置字节
    SetBytes(key string, value []byte, ttl time.Duration) bool
    
    // Delete 删除缓存
    Delete(key string)
    
    // Flush 清空缓存
    Flush()
    
    // Close 关闭缓存连接
    Close() error
}

// CacheFactory 缓存工厂
type CacheFactory func(config CacheConfig) (Cache, error)

type CacheConfig struct {
    Type       string        // "memory" / "redis"
    MaxSize    int           // 最大条目数（内存缓存）
    MaxMemory  int64         // 最大内存（字节）
    TTL        time.Duration // 默认 TTL
    // Redis 配置
    RedisAddr  string
    RedisPass  string
    RedisDB    int
}
```

**bigcache 实现**：

```go
// internal/infrastructure/cache/bigcache_impl.go
package cache

import (
    "github.com/allegro/bigcache"
    "yunxia/pkg/cache"
)

type bigCacheImpl struct {
    client *bigcache.BigCache
}

var _ cache.Cache = (*bigCacheImpl)(nil)

func NewBigCache(cfg cache.CacheConfig) (cache.Cache, error) {
    client, err := bigcache.New(bigcache.Config{
        Shards:             1024,
        LifeWindow:         cfg.TTL,
        MaxEntriesInWindow: cfg.MaxSize,
        MaxEntrySize:       500,
        HardMaxCacheSize:   int(cfg.MaxMemory / 1024 / 1024), // MB
    })
    if err != nil {
        return nil, err
    }
    return &bigCacheImpl{client: client}, nil
}
```

**Redis 实现**（预留）：

```go
// internal/infrastructure/cache/redis_impl.go
package cache

import (
    "github.com/redis/go-redis/v9"
    "yunxia/pkg/cache"
)

type redisCacheImpl struct {
    client *redis.Client
}

var _ cache.Cache = (*redisCacheImpl)(nil)

func NewRedisCache(cfg cache.CacheConfig) (cache.Cache, error) {
    client := redis.NewClient(&redis.Options{
        Addr:     cfg.RedisAddr,
        Password: cfg.RedisPass,
        DB:       cfg.RedisDB,
    })
    return &redisCacheImpl{client: client}, nil
}
```

**配置切换**：

```yaml
cache:
  type: "memory"      # 切换为 "redis" 即可替换
  max_size: 100000
  max_memory: 134217728  # 128MB
  # redis 配置（type=redis 时生效）
  redis_addr: "localhost:6379"
  redis_pass: ""
  redis_db: 0
```

### 3.3 日志抽象（Logger）

```go
// pkg/logger/logger.go
package logger

import "context"

// Logger 日志能力抽象
// 不暴露 zap/logrus 的具体 API
type Logger interface {
    // 基础日志
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    Fatal(msg string, fields ...Field)
    
    // 带上下文的日志（自动注入 trace_id / user_id）
    DebugContext(ctx context.Context, msg string, fields ...Field)
    InfoContext(ctx context.Context, msg string, fields ...Field)
    WarnContext(ctx context.Context, msg string, fields ...Field)
    ErrorContext(ctx context.Context, msg string, fields ...Field)
    
    // 分类日志（用于审计）
    Audit(ctx context.Context, action string, fields ...Field)
    
    // 创建子 Logger（添加固定字段）
    With(fields ...Field) Logger
    
    // 同步（程序退出前调用）
    Sync() error
}

// Field 日志字段
type Field struct {
    Key   string
    Value interface{}
}

// 便捷构造函数
func String(key, val string) Field { return Field{Key: key, Value: val} }
func Int(key string, val int) Field { return Field{Key: key, Value: val} }
func Error(err error) Field { return Field{Key: "error", Value: err} }
```

**zap 实现**：

```go
// internal/infrastructure/logger/zap_impl.go
package logger

import (
    "go.uber.org/zap"
    "yunxia/pkg/logger"
)

type zapLogger struct {
    logger *zap.Logger
}

var _ logger.Logger = (*zapLogger)(nil)

func NewZapLogger(cfg LogConfig) (logger.Logger, error) {
    // ... 初始化 zap
    return &zapLogger{logger: z}, nil
}

func (l *zapLogger) Info(msg string, fields ...logger.Field) {
    l.logger.Info(msg, convertFields(fields)...)
}

func (l *zapLogger) Audit(ctx context.Context, action string, fields ...logger.Field) {
    // 审计日志自动添加分类标记
    allFields := append([]logger.Field{
        logger.String("category", "audit"),
        logger.String("action", action),
    }, fields...)
    l.InfoContext(ctx, "audit_log", allFields...)
}

// ... 其他方法
```

### 3.4 密码哈希抽象（Hasher）

```go
// pkg/crypto/hasher.go
package crypto

// Hasher 密码哈希抽象
// 替换 bcrypt 成本或算法时，无需修改业务代码
type Hasher interface {
    // Hash 生成密码哈希
    Hash(password string) (string, error)
    
    // Compare 验证密码
    Compare(hash, password string) bool
    
    // Cost 返回当前成本因子
    Cost() int
}

// HasherFactory 哈希工厂
type HasherFactory func(cost int) Hasher
```

**bcrypt 实现**：

```go
// internal/infrastructure/crypto/bcrypt_impl.go
package crypto

import "golang.org/x/crypto/bcrypt"

type bcryptHasher struct {
    cost int
}

var _ crypto.Hasher = (*bcryptHasher)(nil)

func NewBCryptHasher(cost int) crypto.Hasher {
    return &bcryptHasher{cost: cost}
}

func (h *bcryptHasher) Hash(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
    return string(bytes), err
}

func (h *bcryptHasher) Compare(hash, password string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

### 3.5 Token 生成抽象（TokenGenerator）

```go
// pkg/auth/token.go
package auth

import "time"

// TokenGenerator Token 生成器抽象
type TokenGenerator interface {
    // Generate 生成 Token 对
    Generate(subject string, claims map[string]interface{}, ttl time.Duration) (string, error)
    
    // Parse 解析 Token，返回声明
    Parse(token string) (map[string]interface{}, error)
    
    // Validate 验证 Token 是否有效（不解析声明）
    Validate(token string) error
}

// TokenPair Access + Refresh Token
type TokenPair struct {
    AccessToken  string
    RefreshToken string
    ExpiresIn    int // 秒
}
```

**JWT 实现**：

```go
// internal/infrastructure/auth/jwt_impl.go
package auth

import "github.com/golang-jwt/jwt/v5"

type jwtGenerator struct {
    secret []byte
    issuer string
}

var _ auth.TokenGenerator = (*jwtGenerator)(nil)

func NewJWTGenerator(secret, issuer string) auth.TokenGenerator {
    return &jwtGenerator{secret: []byte(secret), issuer: issuer}
}
```

### 3.6 配置抽象（Config）

```go
// pkg/config/config.go
package config

// Config 配置读取抽象
// 不暴露 viper 的具体 API
type Config interface {
    // GetString 获取字符串配置
    GetString(key string) string
    GetStringDefault(key, defaultVal string) string
    
    // GetInt 获取整数配置
    GetInt(key string) int
    GetIntDefault(key string, defaultVal int) int
    
    // GetBool 获取布尔配置
    GetBool(key string) bool
    GetBoolDefault(key string, defaultVal bool) bool
    
    // GetDuration 获取时间配置
    GetDuration(key string) time.Duration
    
    // GetStringMap 获取 Map 配置
    GetStringMap(key string) map[string]interface{}
    
    // UnmarshalKey 将配置反序列化到结构体
    UnmarshalKey(key string, rawVal interface{}) error
    
    // Watch 监听配置变更
    Watch(key string, callback func())
}
```

### 3.7 分布式锁抽象（Locker）

```go
// pkg/lock/lock.go
package lock

import "context"
import "time"

// Locker 分布式锁抽象
type Locker interface {
    // Lock 获取锁
    Lock(ctx context.Context, key string, ttl time.Duration) (LockToken, error)
    
    // Unlock 释放锁
    Unlock(ctx context.Context, token LockToken) error
    
    // Extend 延长锁的过期时间
    Extend(ctx context.Context, token LockToken, ttl time.Duration) error
}

// LockToken 锁令牌
type LockToken struct {
    Key   string
    Value string // 唯一标识，防止误释放他人锁
}
```

**本地互斥锁实现（MVP）**：

```go
// internal/infrastructure/lock/local_lock.go
package lock

import "sync"

type localLocker struct {
    mu    sync.Mutex
    locks map[string]struct{}
}

var _ lock.Locker = (*localLocker)(nil)

func NewLocalLocker() lock.Locker {
    return &localLocker{locks: make(map[string]struct{})}
}
```

**Redis Redlock 实现（分布式扩展）**：

```go
// internal/infrastructure/lock/redis_lock.go（预留）
package lock

type redisLocker struct {
    client *redis.Client
}

var _ lock.Locker = (*redisLocker)(nil)
```

### 3.8 文件系统抽象（FS）

本地磁盘驱动不直接调用 `os` 包，而是通过 FS 接口：

```go
// pkg/fs/fs.go
package fs

import (
    "io"
    "time"
)

// FS 文件系统抽象
// 本地磁盘、内存文件系统、S3 都可以通过此接口统一访问
type FS interface {
    // Open 打开文件读取
    Open(path string) (File, error)
    
    // Create 创建文件
    Create(path string) (FileWriter, error)
    
    // Mkdir 创建目录
    Mkdir(path string) error
    
    // Remove 删除文件/目录
    Remove(path string) error
    
    // Rename 重命名/移动
    Rename(oldPath, newPath string) error
    
    // Stat 获取文件信息
    Stat(path string) (FileInfo, error)
    
    // List 列出目录内容
    List(path string) ([]FileInfo, error)
    
    // Exists 检查文件是否存在
    Exists(path string) bool
}

// File 可读文件
type File interface {
    io.ReadCloser
    Stat() (FileInfo, error)
}

// FileWriter 可写文件
type FileWriter interface {
    io.WriteCloser
}

// FileInfo 文件元数据
type FileInfo struct {
    Name    string
    Size    int64
    Mode    uint32
    ModTime time.Time
    IsDir   bool
}
```

### 3.9 下载器抽象（Downloader）

```go
// internal/infrastructure/downloader/downloader.go
package downloader

import "context"

// Downloader 下载器抽象
// 应用层只依赖该接口，默认实现为 Aria2Client
type Downloader interface {
    AddURI(ctx context.Context, uris []string, options map[string]interface{}) (string, error)
    TellStatus(ctx context.Context, gid string) (*DownloadStatus, error)
    Pause(ctx context.Context, gid string) error
    Resume(ctx context.Context, gid string) error
    Remove(ctx context.Context, gid string) error
}
```

**默认实现**：`Aria2Client`  
**可替换实现**：qBittorrent / Transmission / 自定义 HTTP 下载器

---

## 4. 依赖注入容器

### 4.1 容器设计

> 说明：配置的"可替换性"发生在读取与解析阶段；容器持有的是已完成解析的运行时配置快照 `*config.Config`。

```go
// internal/di/container.go
package di

import (
    "fmt"

    "yunxia/internal/application/service"
    "yunxia/internal/domain/repository"
    "yunxia/internal/domain/service"
    "yunxia/internal/infrastructure/cache"
    "yunxia/internal/infrastructure/config"
    "yunxia/internal/infrastructure/downloader"
    infraLock "yunxia/internal/infrastructure/lock"
    "yunxia/internal/infrastructure/logger"
    "yunxia/internal/infrastructure/mq"
    "yunxia/internal/infrastructure/persistence/gorm"
    "yunxia/internal/infrastructure/storage"
    "yunxia/pkg/auth"
    infraAuth "yunxia/internal/infrastructure/auth"
    pkgcache "yunxia/pkg/cache"
    pkgcrypto "yunxia/pkg/crypto"
    infraCrypto "yunxia/internal/infrastructure/crypto"
    "yunxia/pkg/db"
    pkglock "yunxia/pkg/lock"
    pkglogger "yunxia/pkg/logger"
)

// Container 依赖注入容器
// 所有组件的创建和组装集中在此处
// 业务代码通过 Container 获取依赖，不直接 new
type Container struct {
    // 配置
    Config *config.Config
    
    // 通用能力
    DB             db.DB
    Cache          pkgcache.Cache
    Logger         pkglogger.Logger
    Hasher         pkgcrypto.Hasher
    TokenGenerator auth.TokenGenerator
    Locker         pkglock.Locker
    
    // 基础设施
    DriverManager *storage.DriverManager
    TaskQueue     mq.TaskQueue
    Downloader    downloader.Downloader
    
    // 仓库（接口）
    UserRepo   repository.UserRepository
    FileRepo   repository.FileRepository
    SourceRepo repository.StorageSourceRepository
    UploadRepo repository.UploadRepository
    TaskRepo   repository.TaskRepository
    ACLRepo    repository.ACLRepository
    ShareRepo  repository.ShareRepository
    
    // 领域服务
    ACLDomainService  *service.ACLService
    AuthDomainService *service.AuthDomainService
    
    // 应用服务
    AuthAppSvc   *service.AuthApplicationService
    FileAppSvc   *service.FileApplicationService
    UploadAppSvc *service.UploadApplicationService
    TaskAppSvc   *service.TaskApplicationService
}

// NewContainer 根据配置创建容器
func NewContainer(cfg *config.Config) (*Container, error) {
    c := &Container{Config: cfg}
    
    // 1. 初始化通用能力（按依赖顺序）
    if err := c.initLogger(); err != nil {
        return nil, err
    }
    if err := c.initDB(); err != nil {
        return nil, err
    }
    if err := c.initCache(); err != nil {
        return nil, err
    }
    c.initHasher()
    c.initTokenGenerator()
    if err := c.initLocker(); err != nil {
        return nil, err
    }
    
    // 2. 初始化仓库（依赖 DB + Logger）
    c.initRepositories()
    
    // 3. 初始化领域服务（依赖仓库 + 通用能力）
    c.initDomainServices()
    
    // 4. 初始化外部服务（依赖配置）
    if err := c.initExternalServices(); err != nil {
        return nil, err
    }
    
    // 5. 初始化应用服务（依赖领域服务 + 外部服务）
    c.initApplicationServices()
    
    return c, nil
}

func (c *Container) initLogger() error {
    var err error
    // 根据配置选择实现
    switch c.Config.Log.Type {
    case "zap":
        c.Logger, err = logger.NewZapLogger(c.Config.Log)
    default:
        c.Logger, err = logger.NewZapLogger(c.Config.Log) // 默认
    }
    return err
}

func (c *Container) initDB() error {
    var err error
    switch c.Config.Database.Type {
    case "sqlite":
        c.DB, err = gorm.NewSQLiteDB(c.Config.Database)
    case "postgresql":
        c.DB, err = gorm.NewPostgresDB(c.Config.Database)
    default:
        return fmt.Errorf("unknown database type: %s", c.Config.Database.Type)
    }
    return err
}

func (c *Container) initCache() error {
    var err error
    switch c.Config.Cache.Type {
    case "memory":
        c.Cache, err = cache.NewBigCache(c.Config.Cache)
    case "redis":
        c.Cache, err = cache.NewRedisCache(c.Config.Cache)
    default:
        c.Cache, err = cache.NewBigCache(c.Config.Cache) // 默认
    }
    return err
}

func (c *Container) initHasher() {
    c.Hasher = infraCrypto.NewBCryptHasher(c.Config.Security.BCryptCost)
}

func (c *Container) initTokenGenerator() {
    c.TokenGenerator = infraAuth.NewJWTGenerator(
        c.Config.JWT.Secret,
        c.Config.JWT.Issuer,
    )
}

func (c *Container) initLocker() error {
    var err error
    switch c.Config.Lock.Type {
    case "local":
        c.Locker = infraLock.NewLocalLocker()
    case "redis":
        c.Locker, err = infraLock.NewRedisLocker(c.Config.Lock.Redis)
    default:
        c.Locker = infraLock.NewLocalLocker()
    }
    return err
}

func (c *Container) initRepositories() {
    // 所有仓库只依赖 DB + Logger 接口
    c.UserRepo = gorm.NewUserRepository(c.DB, c.Logger)
    c.FileRepo = gorm.NewFileRepository(c.DB, c.Logger)
    c.SourceRepo = gorm.NewSourceRepository(c.DB, c.Logger)
    c.UploadRepo = gorm.NewUploadRepository(c.DB, c.Logger)
    c.TaskRepo = gorm.NewTaskRepository(c.DB, c.Logger)
    c.ACLRepo = gorm.NewACLRepository(c.DB, c.Logger)
    c.ShareRepo = gorm.NewShareRepository(c.DB, c.Logger)
}

func (c *Container) initDomainServices() {
    c.ACLDomainService = service.NewACLService(c.ACLRepo)
    c.AuthDomainService = service.NewAuthDomainService(c.Hasher)
}

func (c *Container) initExternalServices() error {
    c.DriverManager = storage.NewDriverManager()
    
    var err error
    c.TaskQueue, err = mq.NewSQLiteTaskQueue(c.DB, c.TaskRepo, c.Config.TaskQueue.Workers)
    if err != nil {
        return err
    }
    
    c.Downloader = downloader.NewAria2Client(
        c.Config.Aria2.RPCURL,
        c.Config.Aria2.RPCSecret,
    )
    
    return nil
}

func (c *Container) initApplicationServices() {
    c.AuthAppSvc = service.NewAuthApplicationService(
        c.UserRepo, c.AuthDomainService, c.TokenGenerator, c.Config.JWT,
    )
    c.FileAppSvc = service.NewFileApplicationService(
        c.FileRepo, c.SourceRepo, c.ACLDomainService, c.DriverManager,
    )
    c.UploadAppSvc = service.NewUploadApplicationService(
        c.UploadRepo, c.SourceRepo, c.FileRepo, c.DriverManager,
    )
    c.TaskAppSvc = service.NewTaskApplicationService(
        c.TaskRepo, c.TaskQueue, c.Downloader,
    )
}
```

### 4.2 使用方式

```go
// cmd/server/main.go
func main() {
    // 1. 读取配置
    cfg := config.Load()
    
    // 2. 创建容器（所有依赖在此组装）
    container, err := di.NewContainer(cfg)
    if err != nil {
        log.Fatal(err)
    }
    
    // 3. 创建路由，注入 Handler
    router := interfaces.NewRouter(
        container.AuthAppSvc,
        container.FileAppSvc,
        container.UploadAppSvc,
        container.TaskAppSvc,
        // ...
    )
    
    // 4. 启动服务
    router.Run(":8080")
}
```

---

## 5. 可替换性实现矩阵

| 能力 | 接口位置 | MVP 实现 | 替换实现 | 替换成本 |
|------|---------|---------|---------|---------|
| **数据库** | `pkg/db/db.go` | GORM + SQLite | GORM + PostgreSQL / 原生 sql / Ent | 修改 `NewContainer().initDB()` 一行 |
| **缓存** | `pkg/cache/cache.go` | bigcache | Redis / ristretto | 修改配置 `cache.type` |
| **日志** | `pkg/logger/logger.go` | zap | logrus / zerolog | 修改 `NewContainer().initLogger()` |
| **密码哈希** | `pkg/crypto/hasher.go` | bcrypt | argon2 / scrypt | 修改 `NewContainer().initHasher()` |
| **Token** | `pkg/auth/token.go` | JWT | PASETO / 自定义 | 修改 `NewContainer().initTokenGenerator()` |
| **配置** | `pkg/config/config.go` | viper | etcd / Consul / 环境变量 | 实现 Config 接口 |
| **锁** | `pkg/lock/lock.go` | 本地互斥锁 | Redis Redlock | 修改配置 `lock.type` |
| **存储驱动** | `internal/infrastructure/storage/driver.go` | Local / S3 / OneDrive | Google Drive / Dropbox | 实现 Driver 接口并注册 |
| **下载器** | `internal/infrastructure/downloader/downloader.go` | Aria2 | qBittorrent / Transmission | 实现 Downloader 接口 |
| **任务队列** | `internal/infrastructure/mq/queue.go` | SQLite + channel | Redis / RabbitMQ / NATS | 实现 TaskQueue 接口 |
| **文件系统** | `pkg/fs/fs.go` | OS 文件系统 | 内存 FS / S3 FS | 实现 FS 接口 |

---

## 6. 测试策略

### 6.1 Mock 实现示例

```go
// tests/mocks/user_repo_mock.go
package mocks

import (
    "context"
    "yunxia/internal/domain/entity"
    "yunxia/internal/domain/repository"
)

// MockUserRepository 用户仓库 Mock
type MockUserRepository struct {
    Users map[uint]*entity.User
}

var _ repository.UserRepository = (*MockUserRepository)(nil)

func (m *MockUserRepository) Create(ctx context.Context, user *entity.User) error {
    m.Users[user.ID] = user
    return nil
}

func (m *MockUserRepository) FindByUsername(ctx context.Context, username string) (*entity.User, error) {
    for _, u := range m.Users {
        if u.Username == username {
            return u, nil
        }
    }
    return nil, nil
}

// ... 其他方法
```

### 6.2 服务层单元测试

```go
// internal/application/service/auth_app_svc_test.go
package service

import (
    "testing"
    "yunxia/internal/domain/service"
    "yunxia/pkg/auth"
    infraAuth "yunxia/internal/infrastructure/auth"
    "yunxia/pkg/crypto"
    "yunxia/tests/mocks"
)

func TestAuthApplicationService_Login(t *testing.T) {
    // 1. 创建 Mock 仓库
    userRepo := &mocks.MockUserRepository{
        Users: map[uint]*entity.User{
            1: {ID: 1, Username: "admin", PasswordHash: "$2a$12$..."},
        },
    }
    
    // 2. 创建真实领域服务（轻量，不需要 Mock）
    hasher := infraCrypto.NewBCryptHasher(12)
    authDomainSvc := service.NewAuthDomainService(hasher)
    
    // 3. 创建 Mock Token 生成器
    tokenGen := &mocks.MockTokenGenerator{}
    
    // 4. 组装应用服务（只依赖接口！）
    svc := NewAuthApplicationService(userRepo, authDomainSvc, tokenGen, jwtConfig)
    
    // 5. 测试
    resp, err := svc.Login(context.Background(), dto.LoginRequest{
        Username: "admin",
        Password: "password123",
    })
    
    // 断言...
}
```

### 6.3 测试金字塔

```
        /\
       /  \      E2E 测试（少量）
      /____\     启动完整服务，模拟真实请求
     /      \
    /________\   集成测试（中等）
   /          \   测试多个组件集成（DB + Cache + Service）
  /____________\  单元测试（大量）
 /              \  Mock 外部依赖，只测业务逻辑
/________________\
```

---

## 7. 新增/替换组件指南

### 7.1 新增存储驱动（如 Google Drive）

```
1. 创建目录：internal/infrastructure/storage/googledrive/
2. 实现接口：type GoogleDriveDriver struct{} → 实现 storage.Driver
3. 注册驱动：func init() { storage.Register("googledrive", ...) }
4. 添加配置：config.go 增加 googledrive 配置项
5. 前端表单：添加 Google Drive 配置表单
6. 完成！业务代码零修改
```

### 7.2 替换缓存（bigcache → Redis）

```
1. 确保 Redis 实现已存在：internal/infrastructure/cache/redis_impl.go
2. 修改配置：cache.type = "redis"
3. 配置 Redis 地址：cache.redis_addr = "localhost:6379"
4. 重启服务
5. 完成！零代码修改
```

### 7.3 替换数据库（SQLite → PostgreSQL）

```
1. 准备 PostgreSQL 实例
2. 修改配置：database.type = "postgresql"
3. 修改 DSN：database.dsn = "postgres://user:pass@localhost/db"
4. 运行迁移：自动执行 AutoMigrate
5. （可选）数据迁移：运行 `yunxia migrate --from sqlite --to postgresql`

---

## 8. 相关文档

| 文档 | 作用 |
|------|------|
| `DOCS-INDEX.md` | 文档阅读顺序与跨文档真相源约定 |
| `PRD.md` | 产品范围、验收标准、优先级 |
| `TAD.md` | 架构决策、路由真值表、部署约束 |
| `DESIGN.md` | 后端实现设计与代码级结构 |
| `FRONTEND-DESIGN.md` | 前端页面、交互、状态管理方案 |
6. 重启服务
7. 完成！业务代码零修改
```

---

*本文档定义了云匣 (Yunxia) 的接口抽象规范。所有新增基础设施组件必须遵循"先定义接口，再实现"的原则，确保系统的可扩展性和可测试性。*
