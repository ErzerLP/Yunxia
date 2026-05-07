# Yunxia（云匣）

Yunxia 是一个面向个人与小团队的私有云盘 / 网盘管理系统。项目采用前后端分离架构：后端负责统一存储、权限治理、上传下载、RSS 自动化与 WebDAV 等核心能力，前端提供浏览器端管理界面。

> 当前仓库处于 MVP+ 快速迭代阶段，后端主链路、前端管理界面、Docker 后端编排与本地联调流程已具备；生产化部署仍建议结合反向代理、HTTPS、备份与监控方案完善。

## 核心能力

- **统一文件管理**：基于 VFS 虚拟路径浏览、搜索、创建目录、重命名、移动、复制、删除、下载。
- **多存储源**：已接入 local、S3、PikPak driver；后续可继续扩展第三方网盘存储源。
- **上传链路**：支持分片上传、秒传命中、上传会话恢复；S3 支持 multipart 直传。
- **离线下载**：HTTP/HTTPS 任务走 Aria2，BT / magnet / RSS 下载走 qBittorrent，完成后导入目标 VFS 目录。
- **RSS 自动化**：RSS 源、订阅规则、模板命名、批量操作、失败重试、通知告警。
- **权限治理**：角色 + capability 权限模型、ACL 规则、用户管理、审计日志。
- **分享与 WebDAV**：文件/目录分享、公开访问页、local WebDAV 暴露。
- **运维基础**：PostgreSQL-only 后端、Docker Compose 编排、结构化日志、健康检查与 smoke 验证入口。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.25、Gin、GORM、PostgreSQL、JWT、bcrypt、`log/slog` |
| 存储 / 下载 | local、S3、PikPak、Aria2、qBittorrent |
| 前端 | React 19、TypeScript 6、Vite 8、Tailwind CSS、React Router、TanStack Query、Zustand、Axios |
| 部署 | Docker、Docker Compose、PostgreSQL 16、Debian/Alpine sidecar 镜像 |
| 协作 | API contract、frontend handoff、frontend test handoff、changelog |

## 仓库结构

```text
.
├── backend/                   # Go 后端服务
│   ├── cmd/server/            # 服务启动入口
│   ├── internal/              # domain / application / infrastructure / interfaces 分层
│   ├── docker/                # aria2 / qBittorrent sidecar 镜像与入口脚本
│   ├── API_CONTRACT.md        # 当前 API / DTO / 错误码 / 权限真相源
│   ├── FRONTEND_HANDOFF.md    # 后端到前端联调交接
│   ├── CHANGELOG.md           # 后端里程碑与验证记录
│   └── .env.example           # 后端与 Compose 环境变量模板
├── web/                       # React + TypeScript + Vite 前端
│   ├── src/api/               # API client
│   ├── src/components/        # 通用与业务组件
│   ├── src/pages/             # 页面路由
│   ├── src/stores/            # Zustand 状态
│   ├── FRONTEND_TEST_HANDOFF.md
│   └── README.md              # 前端开发入口说明
├── docker-compose.backend.yml # 后端 + PostgreSQL + Aria2 + qBittorrent 编排
└── README.md                  # 项目入口文档
```

## 环境要求

- Docker 与 Docker Compose：推荐用于启动后端、PostgreSQL 与下载器 sidecar。
- Go：与 `backend/go.mod` 保持一致，当前为 Go 1.25。
- Node.js / npm：用于前端开发、构建和 lint。
- PowerShell、bash 或等价 shell：用于执行本文中的启动命令。

## 快速开始

### 1. 准备后端环境变量

PowerShell：

```powershell
Copy-Item backend/.env.example backend/.env -ErrorAction SilentlyContinue
```

Linux / macOS：

```bash
cp -n backend/.env.example backend/.env
```

首次部署至少检查并修改：

```env
YUNXIA_HTTP_PORT=8080
YUNXIA_JWT_SECRET=change-me-in-production
YUNXIA_SERVER_MODE=release
YUNXIA_QBITTORRENT_ENABLED=true
```

> 请勿在真实部署中继续使用默认 `YUNXIA_JWT_SECRET`。

### 2. 启动后端与依赖服务

```powershell
docker compose --env-file backend/.env -f docker-compose.backend.yml up -d --build
```

检查容器状态：

```powershell
docker compose -f docker-compose.backend.yml ps
```

健康检查：

```powershell
Invoke-RestMethod -UseBasicParsing http://127.0.0.1:8080/api/v1/health
```

Linux / macOS 可使用：

```bash
curl -fsS http://127.0.0.1:8080/api/v1/health
```

### 3. 启动前端开发服务

```powershell
cd web
npm install
npm run dev
```

默认访问：

```text
http://127.0.0.1:5173
```

`web/vite.config.ts` 已配置开发期代理：

- `/api/*` → `http://localhost:8080`
- `/dav/*` → `http://localhost:8080`
- `/__public_share/*` → 后端 `/s/*`

首次打开页面后，按系统初始化引导创建首个 `super_admin` 用户。

## 本地开发

### 后端直接运行

适合只调试 Go 业务逻辑；如不需要下载器，可临时关闭 qBittorrent：

```powershell
cd backend
$env:YUNXIA_SERVER_HOST='127.0.0.1'
$env:YUNXIA_SERVER_PORT='8080'
$env:YUNXIA_SERVER_MODE='debug'
$env:YUNXIA_DATABASE_DSN='postgres://yunxia:yunxia@127.0.0.1:5432/yunxia?sslmode=disable&TimeZone=Asia%2FShanghai'
$env:YUNXIA_DATABASE_AUTO_MIGRATE='true'
$env:YUNXIA_STORAGE_DATA_DIR='./data/storage'
$env:YUNXIA_STORAGE_TEMP_DIR='./data/temp'
$env:YUNXIA_JWT_SECRET='dev-only-change-me'
$env:YUNXIA_QBITTORRENT_ENABLED='false'
go run ./cmd/server
```

> 直接运行后端仍需要可访问的 PostgreSQL；日常联调更推荐使用 Compose 后端 + Vite 前端。

### 前端常用命令

```powershell
cd web
npm run dev      # 开发服务
npm run lint     # ESLint
npm run build    # TypeScript build + Vite build
npm run preview  # 预览静态构建产物
```

注意：`npm run preview` 只预览静态文件，不会自动代理 `/api` 到后端；如给测试人员访问，应使用同源反向代理或继续使用 Vite dev server 联调。

### 后端常用命令

```powershell
cd backend
go test ./...
go run ./cmd/server
```

Compose 常用命令：

```powershell
docker compose -f docker-compose.backend.yml logs --tail=200 backend
docker compose -f docker-compose.backend.yml restart backend
docker compose -f docker-compose.backend.yml down
```

清理测试环境数据会删除数据库与数据卷，仅限可重置环境：

```powershell
docker compose -f docker-compose.backend.yml down -v
```

## 配置说明

后端配置以 `YUNXIA_*` 环境变量为主，模板见 `backend/.env.example`。

常用变量：

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `YUNXIA_HTTP_PORT` | `8080` | 宿主机暴露的后端端口 |
| `YUNXIA_DATABASE_DSN` | Compose 自动派生 | PostgreSQL 连接串 |
| `YUNXIA_DATABASE_AUTO_MIGRATE` | `true` | 启动时执行 GORM AutoMigrate |
| `YUNXIA_STORAGE_DATA_DIR` | `/app/data/storage` | 默认本地存储目录 |
| `YUNXIA_STORAGE_TEMP_DIR` | `/app/data/temp` | 上传 / 导入临时目录 |
| `YUNXIA_JWT_SECRET` | `change-me-in-production` | JWT 签名密钥，部署时必须修改 |
| `YUNXIA_ARIA2_RPC_URL` | `http://aria2:6800/jsonrpc` | 后端访问 Aria2 RPC 的地址 |
| `YUNXIA_QBITTORRENT_ENABLED` | `true` | 是否启用 qBittorrent 集成 |
| `YUNXIA_QBITTORRENT_API_URL` | `http://qbittorrent:8080` | 后端访问 qBittorrent Web API 的地址 |

S3、PikPak 等存储源配置不是全局 `.env`，而是通过管理员创建 / 更新存储源时写入对应 source 的公开配置与 secret 配置。

## 测试与质量

文档中的当前基线以仓库已有测试与 handoff 记录为准。常用检查：

```powershell
cd backend
go test ./...

cd ../web
npm run lint
npm run build
```

前端文件 / VFS / 分享 / 上传 / 任务 / 认证相关改动还应运行：

```powershell
cd web
node scripts/check-vfs-integration.mjs
```

## 部署提示

当前仓库提供后端 Compose 编排：

- `postgres`：PostgreSQL
- `backend`：Yunxia Go API
- `aria2`：HTTP/HTTPS 离线下载执行器
- `qbittorrent`：BT / magnet / RSS 下载执行器

前端生产部署建议：

1. 在 `web/` 执行 `npm run build`，产物位于 `web/dist/`。
2. 使用 Nginx / Caddy / 现有网关托管静态文件。
3. 将以下路径反向代理到后端：
   - `/api/` → `http://<backend-host>:8080/api/`
   - `/dav/` → `http://<backend-host>:8080/dav/`
   - `/s/` → `http://<backend-host>:8080/s/`
4. 生产环境补齐 HTTPS、日志采集、数据库备份、对象存储权限与密钥管理。

## 文档入口

| 文档 | 用途 |
|---|---|
| `backend/API_CONTRACT.md` | 后端 API / DTO / 错误码 / 权限真相源 |
| `backend/FRONTEND_HANDOFF.md` | 后端变更到前端适配的交接记录 |
| `backend/CHANGELOG.md` | 后端能力里程碑与验证记录 |
| `web/FRONTEND_TEST_HANDOFF.md` | 前端给测试 / 联调负责人的验证记录 |
| `web/README.md` | 前端开发入口与常用命令 |

## 当前状态与路线

已落地重点：

- local / S3 / PikPak 存储源能力。
- VFS v2 文件管理接口与前端页面。
- PostgreSQL-only 后端运行时。
- Aria2 + qBittorrent 下载器协作。
- RSS 订阅、规则 preview、模板命名、通知告警与失败重试。
- 用户、角色、capability、ACL、审计、分享与 WebDAV 主链路。

后续重点：

- 完善第三方存储驱动抽象与更清晰的 VFS 元数据模型。
- 补齐前端生产部署示例、端到端 smoke 与更多自动化回归。
- 持续收敛 API contract、frontend handoff 与 tester handoff 的同步流程。

## 维护约定

- 接口、DTO、错误码、权限变化：更新 `backend/API_CONTRACT.md`。
- 需要前端适配：更新 `backend/FRONTEND_HANDOFF.md`。
- 需要测试重点验证：更新 `web/FRONTEND_TEST_HANDOFF.md`。
- 历史设计稿只作背景参考；当前实现以代码、API 契约、运行配置和 changelog 为准。

## License

当前仓库未声明开源许可证。如需对外发布，请先补充明确的 license 文件与使用边界。
