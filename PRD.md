# 产品需求文档 (PRD) —— 云匣 (Yunxia)

> **版本**: v1.0  
> **日期**: 2026-04-20  
> **产品名**: 云匣 (Yunxia)  
> **定位**: 面向个人技术用户的轻量、开源、自托管文件管理平台  
> **核心 motto**: 一次部署，管理所有存储  
> **架构范式**: DDD (Domain-Driven Design) 分层架构（逻辑四层，工程习惯简称“三层”）  
> **文档职责**: 产品需求、范围边界、验收标准、优先级  
> **下游文档**: `TAD.md` / `DESIGN.md` / `INTERFACE-ARCHITECTURE.md` / `FRONTEND-DESIGN.md`  
> **开源协议**: MIT  

---

## 目录

1. [修订记录](#1-修订记录)
2. [产品概述](#2-产品概述)
3. [目标用户与使用场景](#3-目标用户与使用场景)
4. [功能需求](#4-功能需求)
5. [非功能需求](#5-非功能需求)
6. [技术架构概览](#6-技术架构概览)
7. [数据库设计](#7-数据库设计)
8. [关键业务流程](#8-关键业务流程)
9. [配置体系](#9-配置体系)
10. [部署方案](#10-部署方案)
11. [风险与规避](#11-风险与规避)
12. [开发排期](#12-开发排期)

---

## 1. 修订记录

| 版本 | 日期 | 修订内容 | 作者 |
|------|------|---------|------|
| v1.0 | 2026-04-20 | 整合版：产品需求 + 断点续传 + 离线下载 + 目录级ACL + 1-10万文件量 + DDD分层架构 | - |

---

## 2. 产品概述

### 2.1 一句话定义

**云匣**是一个为个人技术用户打造的轻量、开源、自托管文件管理平台。支持本地磁盘、S3 兼容对象存储、OneDrive 等标准协议存储的统一管理，通过 WebDAV 服务端与 Jellyfin/Emby 等第三方生态无缝集成，内置离线下载能力，采用严格的目录级权限控制保障数据安全。

### 2.2 产品定位

| 维度 | 定位 |
|------|------|
| **核心场景** | 个人/小型团队的文件管理、存储聚合、离线下载、WebDAV 生态集成 |
| **非目标场景** | 企业级协作办公、实时文件同步、媒体转码服务器 |
| **差异化** | 轻量单容器部署 + 标准协议优先 + WebDAV 生态集成 + 断点续传 + 离线下载 + 目录级ACL + 完全开源免费 |
| **文件规模** | 支持 1-10 万文件量级 |
| **对标参考** | FileBrowser（极简）< **云匣** < Cloudreve（功能更丰富）< Nextcloud（企业级） |

### 2.3 核心价值主张

1. **开箱即用**：单容器 Docker 部署，内置 SQLite，5 分钟内运行
2. **存储一体**：一个界面管理本地/S3/OneDrive，不再分散
3. **WebDAV 就绪**：选择性暴露存储源，Jellyfin/Emby 直接挂载
4. **断点续传**：大文件上传支持断点续传、秒传，下载支持 Range 断点续传
5. **离线下载**：集成 Aria2，支持 HTTP/BT/Magnet 离线下载到指定存储
6. **细粒度权限**：目录级 ACL 控制，支持 allow/deny 规则和继承
7. **透明可信**：完全开源免费，MIT 许可证，多维护者治理

### 2.4 需求背景

基于《2025-2026 开源网盘市场调研报告》的核心洞察：

- **中间层空白**：面向 5-50 人团队的轻量网盘产品稀缺
- **信任危机**：AList 收购事件后，用户将"安全/信任"作为首要筛选条件
- **一体化机会**：当前用户需部署 AList+Jellyfin+CloudDrive2+Nextcloud 四个独立系统
- **标准协议优先**：国内网盘聚合存在法律风险，应优先对接标准协议（S3/WebDAV/OneDrive）

---

## 3. 目标用户与使用场景

### 3.1 用户画像

| 用户类型 | 描述 | 核心诉求 |
|---------|------|---------|
| **个人 VPS 玩家** | 拥有 1-2 台云服务器，熟悉 Docker/Linux | 轻量、低资源占用、简单维护、断点续传 |
| **NAS 用户** | 使用绿联/飞牛/极空间等国产 NAS | 应用中心一键安装、中文原生、稳定 |
| **媒体库搭建者** | 已部署/计划部署 Jellyfin/Emby | WebDAV 存储后端、Range 断点续传播放 |
| **下载爱好者** | 需要离线下载 BT/磁力资源 | Aria2 集成、下载完成后自动归档 |
| **小众技术爱好者** | 关注数据主权，不信任商业网盘 | 自托管、开源可审计、目录级权限控制 |

### 3.2 典型使用场景

#### 场景 A：个人文件管理中心
> 小张有一台 2C2G 的 VPS，他用 Docker Compose 一键部署了云匣 + Aria2。他把 VPS 本地磁盘作为主力存储，挂载了一个阿里云 OSS Bucket 作为冷备份，同时接入了 OneDrive。所有文件在一个界面中管理，上传大视频时断点续传不担心网络中断，下载走直传链接不占用 VPS 带宽。

#### 场景 B：Jellyfin 的存储后端
> 小李家里有台 NAS，部署了 Jellyfin 做影视库。他在云匣中挂载了 OneDrive（存放影视资源），然后通过 WebDAV 把 OneDrive 暴露给 Jellyfin。Jellyfin 通过 WebDAV 直接读取云端文件生成海报墙，拖动播放进度时 Range 断点续传确保流畅体验，无需在 NAS 本地存放大体积视频。

#### 场景 C：离线下载归档
> 小王经常下载 BT 资源。他在云匣中提交了一个 Magnet 链接，Aria2 在后台下载完成后，文件自动移动到"/下载/电影/"目录。云匣为这个目录建立了索引，小王可以直接在云匣中浏览、播放或分享下载完成的文件。

#### 场景 D：团队共享（多用户）
> 小赵是一个 5 人设计工作室的管理员。他在云匣中开启了多用户模式，为每个成员分配了"本地存储"的目录级权限：
> - `/项目/` — 所有人可读写
> - `/素材/` — 所有人只读
> - `/财务/` — 仅管理员可访问
> - `/私人/` — 各自独立空间

---

## 4. 功能需求

### 4.1 P0 — MVP 核心功能（必须有）

#### 4.1.1 本地文件管理

| 功能点 | 需求描述 | 验收标准 |
|--------|---------|---------|
| **文件 CRUD** | 创建文件夹、上传文件、下载文件、重命名、移动、复制、删除 | 支持单文件和批量操作 |
| **文件预览** | 图片/视频/音频/文本/PDF 在线预览 | 图片支持缩略图；视频用原生 HTML5 播放器，支持拖动进度（Range 请求）；文本支持代码高亮 |
| **搜索** | 按文件名模糊搜索当前存储源 | 支持拼音搜索（中文优化）；MVP 阶段仅做文件名搜索，全文搜索能力见 P1 本地索引 |
| **回收站** | 删除文件进入回收站，支持恢复和彻底删除 | 回收站按存储源隔离 |
| **分片上传** | 拖拽上传、分片上传（5MB/片）、上传进度显示、断点续传 | 大文件自动分片；网络中断后可恢复；支持秒传（MD5 检查） |
| **下载** | 普通下载 + 断点续传下载（HTTP Range） | 本地文件支持 Range；S3 文件通过预签名 URL 直传，天然支持 Range |
| **路径导航** | 面包屑导航、路径快捷跳转 | 支持点击面包屑任意层级跳转 |
| **视图切换** | 列表视图 / 网格视图 | 记住用户偏好 |
| **排序筛选** | 按名称/大小/修改时间排序 | 默认按修改时间倒序；支持分页（200/页） |
| **虚拟滚动** | 大目录（>1000 文件）流畅滚动 | 使用虚拟滚动技术，DOM 节点不超过 50 个 |

**非需求**：
- 不做服务端转码（视频直接源流播放 + Range 请求）
- 不做实时协作编辑

#### 4.1.2 多存储后端（Driver 架构）

| 后端 | 协议 | 断点续传上传 | 断点续传下载 | 说明 |
|------|------|------------|------------|------|
| **本地磁盘** | 本地文件系统 | ✅ 分片合并 | ✅ Range | 默认存储，数据持久化到宿主机目录 |
| **S3 兼容** | AWS S3 API | ✅ Multipart Upload | ✅ Range | AWS/阿里云 OSS/腾讯云 COS/MinIO |
| **OneDrive** | Microsoft Graph API | ✅ 分片上传 | ✅ Range | 个人版和商业版 |

**架构预留**：
- Driver 注册采用 `init()` 自注册模式
- 新增驱动无需修改核心代码，只需导入驱动包
- 配置文件或 Web 界面中动态启用/禁用驱动

#### 4.1.3 WebDAV 服务端

| 功能点 | 需求描述 | 验收标准 |
|--------|---------|---------|
| **RFC 4918 基础实现** | 支持 PROPFIND/GET/PUT/DELETE/MKCOL/MOVE/COPY | 可用 macOS Finder/Windows 资源管理器挂载 |
| **Range 请求** | 支持 HTTP Range（断点续传下载） | 视频拖动进度流畅；大文件下载可 resume |
| **选择性暴露** | 用户在设置中选择哪些存储源通过 WebDAV 暴露 | 默认关闭，手动开启 |
| **路径映射** | 每个暴露的存储源映射到 WebDAV 根目录下的子路径 | 如 `/dav/local/` → 本地磁盘，`/dav/s3/` → S3 Bucket |
| **Basic Auth** | 仅支持 Basic Auth 认证 | 用户名密码与系统账号一致；**强制 HTTPS** |
| **只读开关** | 每个暴露源可配置只读或读写 | 防止媒体服务器误删文件 |
| **PROPFIND 缓存** | 目录列表缓存 30 秒 | 防止 Jellyfin 扫库时重复请求存储源 API |

#### 4.1.4 断点续传上传

| 功能点 | 需求描述 | 验收标准 |
|--------|---------|---------|
| **分片规格** | 固定 5MB/片 | 1GB 文件 = 200 片 |
| **并发上传** | 前端同时并发 3 个 chunk | 多个文件排队传输 |
| **秒传** | 基于 MD5 哈希检查 | 已存在的文件无需上传，直接秒传成功 |
| **上传会话** | 上传状态持久化到数据库（7 天有效） | 刷新页面后可恢复上传；可手动取消 |
| **本地磁盘上传** | 分片写入临时目录，完成后合并 | 临时文件存在 `./data/temp/{upload_id}/` |
| **S3 上传** | 服务端返回预签名 URL，前端直传 S3 | 无本地临时文件；使用 S3 Multipart Upload |
| **完成合并** | 所有 chunk 完成后合并为最终文件 | 写入文件元数据；清理临时文件 |

#### 4.1.5 离线下载（Aria2 集成）

| 功能点 | 需求描述 | 验收标准 |
|--------|---------|---------|
| **集成模式** | Docker Compose 组合部署为主，支持外部 Aria2 | 默认与 Aria2 同机部署；高级用户可配置外部地址 |
| **提交下载** | 支持 HTTP/HTTPS/BT/Magnet 链接 | 提交到任务队列，Aria2 执行下载 |
| **任务管理** | 查看下载进度、暂停、恢复、删除 | 实时显示速度、已下载/总大小、ETA |
| **下载完成** | 自动移动文件到指定存储源目录 | 支持自定义保存路径；完成后写入文件元数据 |
| **任务队列** | 后台 worker 处理，支持重试（最多 3 次） | 失败任务自动重试；可配置 worker 数量（默认 3） |
| **BT 下载** | 支持 .torrent 文件和 magnet 链接 | 自动更新 tracker；可配置下载目录 |

**部署方式**：
- 推荐：Docker Compose 同时启动 `yunxia` + `aria2` 两个容器
- 备选：用户自行部署 Aria2，配置 JSON-RPC 地址和密钥

#### 4.1.6 用户认证与权限体系

| 功能点 | 需求描述 | 验收标准 |
|--------|---------|---------|
| **单用户默认** | 首次启动创建单管理员账号，部署即用 | 初始化向导引导设置 |
| **可选多用户** | 设置中开启"多用户模式"后支持注册新用户 | 开启后显示用户管理面板 |
| **JWT 双 Token** | Access Token（15 分钟）+ Refresh Token（7 天） | Access 短期有效降低盗用风险；Refresh 可撤销 |
| **Token 撤销** | 通过 Token 版本号机制撤销所有登录 | 无需维护黑名单；改数据库一行即可踢掉所有设备 |
| **角色权限** | 管理员 / 普通用户 / 访客 | 管理员可管理存储源和用户；普通用户只能访问被授权的空间 |
| **目录级 ACL** | 支持存储源级别的细粒度目录权限控制 | allow/deny 规则；优先级排序；子目录继承 |
| **密码安全** | bcrypt 哈希（cost=12），最小 8 位 | 登录失败 5 次锁定 15 分钟 |

#### 4.1.7 Docker 部署

| 功能点 | 需求描述 | 验收标准 |
|--------|---------|---------|
| **单容器运行** | 一个 Docker 容器包含完整服务 | 不需要额外数据库/Redis 容器 |
| **Docker Compose** | 一键启动云匣 + Aria2 | `docker-compose up -d` 完成部署 |
| **环境变量配置** | 关键配置通过环境变量传入 | 如 `JWT_SECRET`, `PORT`, `DATA_DIR` |
| **持久化卷** | 数据（SQLite + 本地文件）挂载到宿主机 | 容器重建不丢失数据 |
| **健康检查** | 内置 `/api/v1/health` 接口 | Docker 可配置 healthcheck |
| **ARM 支持** | Docker 镜像同时支持 amd64 和 arm64 | 适配树莓派和 ARM NAS |

**目标资源占用**：
- Docker 镜像体积 < 100MB
- 空载内存 < 200MB（含 Aria2 约 300MB）
- 最低运行配置：1 核 CPU / 512MB 内存

---

### 4.2 P1 — 增强竞争力（MVP 验证后跟进）

| 功能模块 | 说明 |
|---------|------|
| **本地磁盘索引** | 后台建立文件索引（inotify + 定期扫描），支持全文搜索 |
| **分享功能** | 生成分享链接，支持密码保护、有效期、下载次数限制 |
| **暗黑模式** | 系统级暗色主题适配 |
| **PWA 支持** | 可安装为桌面/移动端 Web 应用，离线浏览已缓存目录 |
| **WebDAV 客户端** | 挂载外部 WebDAV 源到本系统（反向 WebDAV） |
| **Google Drive 驱动** | Google Drive 存储后端 |
| **Dropbox 驱动** | Dropbox 存储后端 |
| **存储配额** | 按用户/按存储源设置容量上限 |
| **操作日志 Web 界面** | 管理员查看系统操作记录 |
| **审计日志** | 完整的审计日志查询和导出 |
| **自动备份** | 定时将元数据/配置备份到指定存储后端 |
| **PostgreSQL 迁移** | 一键迁移工具：SQLite → PostgreSQL |

### 4.3 P2 — 生态扩展（成熟期）

| 功能模块 | 说明 |
|---------|------|
| **插件系统** | 标准插件 API，第三方开发者可扩展存储驱动和功能模块 |
| **国内网盘驱动** | 阿里云盘/百度网盘/115 网盘/夸克网盘（通过预留 Driver 接口接入） |
| **RSS 订阅下载** | 定时抓取 RSS Feed，自动下载匹配的资源 |
| **桌面客户端** | Electron 或 Tauri，提供拖拽上传和系统托盘 |
| **文件去重** | 基于哈希的重复文件检测 |
| **AI 辅助** | 本地 OCR、图片分类、重复图片检测 |
| **高级协作** | 文件评论、@通知、审批流 |
| **双因素认证** | TOTP（Google Authenticator） |

---

## 5. 非功能需求

### 5.1 性能

| 指标 | 目标 |
|------|------|
| **首屏加载** | < 2 秒（在 100Mbps 网络下） |
| **文件列表加载** | < 500ms（分页 200 条） |
| **大文件上传** | 走预签名 URL 直传，服务端不瓶颈；断点续传恢复 < 1 秒 |
| **视频播放拖动** | Range 请求响应 < 200ms |
| **并发用户** | 支持 10 个并发用户流畅使用（2C2G 配置） |
| **WebDAV 扫描** | PROPFIND 缓存后，Jellyfin 扫描 1 万文件 < 5 分钟 |
| **空载内存** | < 200MB（不含 Aria2） |

### 5.2 安全

| 层面 | 措施 |
|------|------|
| **传输加密** | 强制 HTTPS，HSTS 头，CSP 策略 |
| **密码存储** | bcrypt 哈希，cost factor = 12 |
| **认证安全** | JWT 双 Token，Token 版本号撤销，登录限流 |
| **文件访问** | 严格的 ACL 权限控制，路径净化防遍历，用户空间隔离 |
| **WebDAV 安全** | Basic Auth 强制 HTTPS，只读默认，独立限流 |
| **分享安全** | 随机 Token，可选密码，过期自动失效，下载次数限制 |
| **审计日志** | 所有敏感操作记录，保留 180 天 |
| **依赖安全** | 定期扫描依赖漏洞（Dependabot / Snyk） |
| **日志分级** | 6 类日志（access/app/auth/audit/webdav/error），结构化 JSON |

### 5.3 部署与运维

| 需求 | 说明 |
|------|------|
| **单容器** | 云匣服务一个 Docker 镜像，内置所有依赖 |
| **Compose 组合** | 云匣 + Aria2 一键启动 |
| **零配置启动** | 首次启动自动引导初始化向导 |
| **配置持久化** | 配置文件和数据库存储在挂载卷中 |
| **日志输出** | 结构化日志输出到 stdout，便于 Docker 收集 |
| **自动更新检测** | 后端定期检查 GitHub Release，Web 界面提示新版本 |
| **数据库迁移** | 内置 SQLite → PostgreSQL 一键迁移工具（P1） |
| **日志轮转** | 自动切割压缩，保留策略可配置 |

---

## 6. 技术架构概览

### 6.1 DDD 三层架构

> 注：本文档沿用“三层架构”的工程简称，严格表达为四层：接口适配层 / 应用层 / 领域层 / 基础设施层。

```
┌─────────────────────────────────────────────────────────────┐
│  接口适配层 (Interface Adapters)                              │
│  REST API Handler / WebDAV Handler / Middleware               │
├─────────────────────────────────────────────────────────────┤
│  应用层 (Application Layer)                                   │
│  Application Services / DTOs / Use Cases                      │
├─────────────────────────────────────────────────────────────┤
│  领域层 (Domain Layer)                                        │
│  Entities / Value Objects / Domain Services / Repository IFs  │
├─────────────────────────────────────────────────────────────┤
│  基础设施层 (Infrastructure Layer)                            │
│  GORM Repo / Storage Drivers / Cache / Aria2 Client / MQ     │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 技术栈

| 层级 | 选型 | 理由 |
|------|------|------|
| **后端语言** | Go 1.24+ | 基础设施层统治语言，goroutine 适合 I/O 密集文件操作，单二进制部署 |
| **Web 框架** | Gin | 成熟选择，性能优秀，中间件生态丰富 |
| **ORM** | GORM | 中文社区巨大，AutoMigrate 适合快速迭代 |
| **前端框架** | React 18 + TypeScript | 组件生态丰富，适合复杂文件管理交互 |
| **UI 组件** | shadcn/ui + Tailwind CSS | 轻量现代，完全可控，避免企业级厚重感 |
| **数据库** | SQLite（默认）/ PostgreSQL（生产）| SQLite 零配置适合个人；PostgreSQL 适合高并发/大文件量 |
| **缓存** | bigcache / ristretto | 高性能内存缓存，接口抽象可切换 Redis |
| **任务队列** | Go channel + SQLite 持久化 | MVP 简单可靠，接口预留可切换 Redis/RabbitMQ |
| **日志** | zap + lumberjack | 高性能结构化日志，自动切割压缩 |
| **前端构建** | Vite | 快速冷启动，现代化 HMR |
| **状态管理** | Zustand | 轻量，比 Redux 简单 |
| **请求库** | Axios + TanStack Query | 缓存、重试、去重开箱即用 |

### 6.3 系统架构图

```
客户端 (Browser/PWA/WebDAV Client)
    │
    ├──▶ REST API (/api/v1/*) ──▶ HTTP Handler ──▶ Application Service ──▶ Domain Service
    │                                                                             │
    ├──▶ WebDAV (/dav/*) ───────▶ WebDAV Handler ──▶ DAV FileSystem ───────▶ Driver
    │                                                                             │
    └──▶ 静态文件 (/web/*) ─────▶ Static File Server

Domain Layer
    ├── Entity: User, FileMetadata, StorageSource, UploadSession, Task, ACLEntry, Share
    ├── Value Object: FileInfo, Pagination, Permission
    ├── Domain Service: ACLService (权限计算), AuthDomainService (认证逻辑)
    └── Repository Interface: UserRepo, FileRepo, SourceRepo, UploadRepo, TaskRepo, ACLRepo, ShareRepo

Infrastructure Layer
    ├── Persistence: GORM Repository Implementation (SQLite/PostgreSQL)
    ├── Storage Driver: LocalDriver / S3Driver / OneDriveDriver
    ├── Cache: bigcache / ristretto (接口抽象)
    ├── Task Queue: SQLite-based Queue (接口抽象)
    ├── Downloader: Aria2 JSON-RPC Client
    └── Config: YAML + 环境变量
```

---

## 7. 数据库设计

### 7.1 数据库选型策略

| 场景 | 推荐数据库 | 说明 |
|------|-----------|------|
| **个人用户，<5 万文件** | SQLite（WAL 模式） | 零配置，单文件，资源占用低 |
| **重度用户，≥5 万文件** | PostgreSQL | 更好的并发性能，全文搜索更强 |
| **团队多用户** | PostgreSQL | 推荐，连接池支持更高并发 |

### 7.2 表结构

#### users（用户表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | PK | 用户 ID |
| username | VARCHAR(64) UNIQUE | 用户名 |
| password_hash | VARCHAR(255) | bcrypt 哈希 |
| email | VARCHAR(255) | 邮箱 |
| role | VARCHAR(20) | admin / user / guest |
| status | VARCHAR(20) | active / disabled |
| storage_quota | BIGINT | 存储配额（0=无限制） |
| token_version | INT | Token 撤销版本号 |
| created_at / updated_at | DATETIME | 时间戳 |

#### storage_sources（存储源表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | PK | 存储源 ID |
| name | VARCHAR(128) | 显示名称 |
| driver_type | VARCHAR(32) | local / s3 / onedrive |
| config | TEXT(JSON) | 驱动配置 |
| root_path | VARCHAR(512) | 根路径 |
| is_enabled | BOOLEAN | 是否启用 |
| is_webdav_exposed | BOOLEAN | 是否通过 WebDAV 暴露 |
| webdav_readonly | BOOLEAN | WebDAV 是否只读 |
| sort_order | INT | 显示排序 |

#### file_metadata（文件元数据缓存表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | PK | 元数据 ID |
| source_id | FK | 所属存储源 |
| path | VARCHAR(4096) | 完整路径 |
| name | VARCHAR(255) | 文件名 |
| parent_path | VARCHAR(4096) | 父目录路径 |
| is_dir | BOOLEAN | 是否目录 |
| size | BIGINT | 文件大小 |
| mime_type | VARCHAR(128) | MIME 类型 |
| checksum | VARCHAR(64) | 文件哈希 |
| modified_at | DATETIME | 修改时间 |
| cached_at | DATETIME | 缓存时间 |
| extra | TEXT(JSON) | 额外元数据 |

**索引**: `idx_file_parent(source_id, parent_path)`, `idx_file_name(name)`

#### local_file_index_fts（本地文件全文搜索索引，P1 预留）

SQLite FTS5 虚拟表，仅针对本地磁盘存储源建立全文索引。

#### upload_sessions（上传会话表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | VARCHAR(64) PK | UUID |
| user_id | FK | 用户 ID |
| source_id | FK | 目标存储源 |
| target_path | VARCHAR(4096) | 目标路径 |
| filename | VARCHAR(255) | 文件名 |
| file_size | BIGINT | 文件大小 |
| file_hash | VARCHAR(64) | MD5 哈希 |
| status | VARCHAR(20) | pending / uploading / completed / failed / cancelled |
| chunk_size | INT | 分片大小（默认 5MB） |
| total_chunks | INT | 总分片数 |
| uploaded_chunks | INT | 已上传片数 |
| completed_chunks | TEXT(JSON) | 已完成的分片索引列表 |
| storage_data | TEXT(JSON) | 存储后端特定数据（S3 upload_id 等） |
| expires_at | DATETIME | 过期时间（7 天） |

#### acl_entries（ACL 权限表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | PK | ACL ID |
| user_id | FK | 用户 ID |
| source_id | FK | 存储源 ID |
| path | VARCHAR(4096) | 路径（/ 表示根） |
| perm_read / perm_write / perm_delete / perm_share | BOOLEAN | 权限位 |
| rule_type | VARCHAR(20) | allow / deny |
| inherit | BOOLEAN | 子目录是否继承 |
| priority | INT | 优先级（数字越大越优先） |

#### tasks（任务队列表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | VARCHAR(64) PK | 任务 ID |
| task_type | VARCHAR(32) | download / index_scan / cleanup |
| status | VARCHAR(20) | pending / running / completed / failed / cancelled |
| payload | TEXT(JSON) | 任务参数 |
| result | TEXT | 执行结果 |
| error_msg | TEXT | 错误信息 |
| priority | INT | 优先级 |
| scheduled_at | DATETIME | 计划执行时间 |
| started_at / completed_at | DATETIME | 开始/完成时间 |
| retry_count / max_retries | INT | 重试次数 |

#### shares（分享表，P1 预留）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | PK | 分享 ID |
| user_id | FK | 创建者 |
| source_id | FK | 存储源 |
| path | VARCHAR(4096) | 分享路径 |
| token | VARCHAR(64) UNIQUE | 随机 Token |
| password_hash | VARCHAR(255) | 访问密码（bcrypt） |
| expires_at | DATETIME | 过期时间 |
| max_downloads | INT | 最大下载次数（0=无限制） |
| download_count | INT | 已下载次数 |
| is_enabled | BOOLEAN | 是否启用 |

#### operation_logs（操作日志表，P1 预留查询/导出能力）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | PK | 日志 ID |
| user_id | FK | 用户 ID |
| action | VARCHAR(32) | 操作类型 |
| source_id | FK | 存储源 |
| path | VARCHAR(4096) | 文件路径 |
| ip_address | VARCHAR(45) | IP 地址 |
| user_agent | VARCHAR(512) | UA |
| status | VARCHAR(20) | success / failed |
| error_msg | TEXT | 错误信息 |
| created_at | DATETIME | 时间戳 |

---

## 8. 关键业务流程

### 8.1 断点续传上传流程

```
用户选择文件
    │
[前端] 计算 MD5 + 文件大小
    │
POST /api/v1/upload/init
{filename, size, hash, source_id, path}
    │
[后端] 秒传检查（hash + size 匹配？）
    ├── YES ──▶ 秒传成功，返回 is_fast_upload=true
    └── NO ──▶ 创建 UploadSession
                返回 {upload_id, chunk_size=5MB, total_chunks, uploaded_chunks=[]}
    │
[前端] 并发上传 chunks（3 并发，多文件排队）
    │
PUT /api/v1/upload/chunk (本地磁盘)
或 PUT presigned_url (S3 直传)
    │
[后端] 接收 chunk → 更新 completed_chunks
    │
全部 chunks 完成后
    │
POST /api/v1/upload/finish
    │
[后端] 合并临时文件 / S3 CompleteMultipart
    ├── 写入 file_metadata
    ├── 清理临时文件
    └── 删除 upload_session
```

### 8.2 离线下载流程

```
用户提交下载链接（HTTP / BT / Magnet）
    │
POST /api/v1/tasks
{type: "download", payload: {url, save_path, source_id}}
    │
[后端] 创建 Task → 提交到任务队列
    │
[Worker] 获取 pending 任务
    │
调用 Aria2 JSON-RPC (aria2.addUri)
    │
[Aria2] 下载中...
    │
[Worker] 定期轮询状态 (aria2.tellStatus)
    │
下载完成
    │
[Worker] 移动文件到目标目录
    ├── 写入 file_metadata
    └── 标记 Task 为 completed
```

### 8.3 权限检查流程

```
用户请求 /api/v1/files?source_id=1&path=/工作/机密/
    │
[JWT 中间件] 认证通过
    │
[FileAppSvc] ListFiles(userID=2, sourceID=1, path="/工作/机密/")
    │
[ACLService] CheckPermission(userID=2, sourceID=1, "/工作/机密/", {read: true})
    │
生成路径层级：["/工作/机密/", "/工作/", "/"]
    │
查询 ACL 表，按优先级排序
    │
最具体匹配：
    /工作/机密/ → deny ──▶ 拒绝访问
    /工作/      → allow + read
    /           → allow + read+write
    │
返回 403 Forbidden
```

### 8.4 WebDAV 请求处理流程

```
Jellyfin 发送 PROPFIND /dav/local/电影/
    │
[WebDAV Handler]
    ├── Basic Auth 认证（强制 HTTPS）
    ├── 解析路径 → source=local, path=/电影/
    │
[WebDAV AppSvc]
    ├── ACL 检查
    ├── PROPFIND 缓存检查（30s TTL）
    │       ├── 命中 ──▶ 返回缓存
    │       └── 未命中
    │
[LocalDriver] List("/电影/")
    │
返回文件列表
    │
ACL 过滤子项权限
    │
写入缓存
    │
返回 WebDAV XML 响应
```

---

## 9. 配置体系

### 9.1 配置文件

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

database:
  type: "sqlite"
  dsn: "/data/database.db"
  max_open_conns: 10
  max_idle_conns: 5
  debug: false

jwt:
  secret: "change-me-in-production"
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
  max_size: 100
  max_age: 30
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

### 9.2 配置优先级

```
环境变量（最高） > 配置文件 > 默认值（最低）

例：YUNXIA_SERVER_PORT=9090 覆盖 config.yaml 中的 port: 8080
```

### 9.3 关键环境变量

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

## 10. 部署方案

### 10.1 Docker Compose（推荐）

```yaml
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
      - YUNXIA_JWT_SECRET=${JWT_SECRET:-change-me}
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
      - RPC_SECRET=${ARIA2_RPC_SECRET:-}
      - RPC_PORT=6800
      - TZ=Asia/Shanghai
    volumes:
      - ./downloads:/downloads
    ports:
      - "6800:6800"
      - "6888:6888"
      - "6888:6888/udp"
```

### 10.2 硬件要求

| 场景 | CPU | 内存 | 存储 | 网络 |
|------|-----|------|------|------|
| **最低配置** | 1 核 | 512MB | 10GB | 1Mbps |
| **推荐配置** | 2 核 | 1GB | 50GB+ | 5Mbps |
| **离线下载** | 2 核 | 2GB | 200GB+ | 10Mbps+ |

### 10.3 部署模式

| 模式 | 适用场景 | 说明 |
|------|---------|------|
| **Docker Compose** | 推荐，大多数用户 | 一键启动云匣 + Aria2 |
| **纯二进制** | 高级用户，已有 Aria2 | 单独运行 yunxia-server，配置外部 Aria2 |
| **NAS 应用中心** | 国产 NAS 用户 | 未来适配绿联/飞牛/极空间应用中心 |

---

## 11. 风险与规避

| 风险 | 等级 | 规避策略 |
|------|------|---------|
| **开发周期膨胀** | 高 | 严格坚守 P0 边界；离线下载直接调用 Aria2，不自研下载引擎 |
| **SQLite 并发瓶颈（≥5万文件）** | 中 | 双轨数据库策略；文档明确建议大文件量用户切 PostgreSQL |
| **WebDAV 兼容性** | 中 | 测试 macOS Finder / Windows / Jellyfin 三种客户端；PROPFIND 缓存 |
| **OneDrive API 限制** | 中 | 做好速率限制和令牌刷新；文档说明限制 |
| **Aria2 集成复杂度** | 中 | Docker Compose 一键部署；提供外部 Aria2 备选方案 |
| **前端包体积** | 中 | Vite 按需加载 + 代码分割；虚拟滚动减少 DOM |
| **ACL 性能（大目录）** | 低 | ACL 结果内存缓存；目录级快速拒绝优化 |
| **社区贡献不足** | 低 | Day 1 多维护者机制；文档完善；积极回应 Issue |

---

## 12. 开发排期

### Phase 1: 脚手架 + 基础骨架（2-3 周）

- [ ] 项目脚手架（Go DDD 目录结构 + React + Vite + Tailwind + shadcn/ui）
- [ ] 数据库模型 + GORM + 迁移（SQLite WAL 模式）
- [ ] zap 日志框架 + 分级日志
- [ ] 配置管理（YAML + 环境变量）
- [ ] JWT 双 Token 认证（Access/Refresh）
- [ ] 用户认证 API + 初始化向导
- [ ] 第一个接口 `GET /api/v1/health`
- [ ] Docker 构建 + CI/CD（GitHub Actions）

### Phase 2: 文件管理 + 存储驱动（3-4 周）

- [ ] 本地存储 Driver
- [ ] S3 Driver（含预签名 URL）
- [ ] OneDrive Driver
- [ ] 基础文件管理 API（List/Get/Delete/Rename/MakeDir）
- [ ] 前端文件浏览器（列表/网格、面包屑、上传按钮）
- [ ] 文件预览（图片/文本/PDF）
- [ ] 分页 + 虚拟滚动
- [ ] 按需缓存 + 内存缓存（bigcache）

### Phase 3: 断点续传 + WebDAV（3-4 周）

- [ ] 断点续传上传（分片 + 秒传 + 会话管理）
- [ ] 下载断点续传（Range 请求）
- [ ] WebDAV 服务端（PROPFIND/GET/PUT/DELETE/MOVE/COPY）
- [ ] WebDAV 选择性暴露 + Basic Auth + 只读开关
- [ ] WebDAV PROPFIND 缓存 + 限流
- [ ] 视频在线播放（HTML5 + 字幕 + Range）
- [ ] 回收站

### Phase 4: 权限 + 离线下载（3-4 周）

- [ ] 目录级 ACL（CRUD + 权限计算 + 继承）
- [ ] ACL 与文件管理/WebDAV 集成
- [ ] Aria2 客户端（JSON-RPC）
- [ ] 任务队列框架（SQLite 持久化 + worker pool）
- [ ] 离线下载（提交/进度/暂停/恢复/删除）
- [ ] 下载完成自动归档
- [ ] Docker Compose（云匣 + Aria2）

### Phase 5: 体验优化（2-3 周）

- [ ] 暗黑模式
- [ ] PWA 支持
- [ ] 搜索功能（本地磁盘 FTS5）
- [ ] 分享功能
- [ ] 操作日志 Web 界面
- [ ] 中文原生优化
- [ ] 文档（README + 部署指南 + API 文档）

**MVP 总周期预估：13-18 周（约 3-4.5 个月）**

---

## 附录 A：相关文档

| 文档 | 职责 | 说明 |
|------|------|------|
| `DOCS-INDEX.md` (v1.0) | **文档总索引** | 阅读顺序、术语约定、跨文档真相源 |
| `PRD.md` (v1.0) | **产品需求** | 本文档：目标用户、功能范围、验收标准、优先级 |
| `TAD.md` (v1.0) | **技术架构** | 架构决策、约束取舍、接口真值表、部署策略 |
| `INTERFACE-ARCHITECTURE.md` (v1.0) | **接口抽象** | 共享抽象、依赖注入、可替换性规范 |
| `DESIGN.md` (v1.0) | **实现设计** | 后端代码级模块、数据结构、详细实现 |
| `FRONTEND-DESIGN.md` (v1.0) | **前端设计** | 页面布局、交互逻辑、动效设计 |

> 文档职责边界：
> - DOCS-INDEX 回答"先看什么、以谁为准"
> - PRD 回答"做什么"
> - TAD 回答"怎么架构"
> - INTERFACE-ARCHITECTURE 回答"共享抽象如何定义"
> - DESIGN 回答"后端怎么实现"
> - FRONTEND-DESIGN 回答"前端怎么呈现"

---

*本文档基于《2025-2026 开源网盘市场调研报告》分析结论编写，所有技术决策经过市场竞争格局、风险评估和 DDD 架构设计验证。*
