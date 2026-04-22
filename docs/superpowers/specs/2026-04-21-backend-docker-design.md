# Yunxia 后端 Docker 配置设计

**目标**：为 `backend/` 提供可维护的容器化配置，让开发者能通过 Docker / Docker Compose 快速启动后端服务，并可选联动 Aria2。

## 1. 范围

本轮只覆盖后端容器化基础设施：

- `backend/Dockerfile`
- `backend/.dockerignore`
- `docker-compose.backend.yml`
- `backend/.env.example`
- Aria2 侧车容器所需的最小 Docker 资源（放在 `backend/docker/`）

本轮不做：

- 前端容器化
- CI/CD 镜像发布流程
- 多环境（dev/prod）Compose 拆分
- Kubernetes / Helm
- 离线下载路径语义改造

## 2. 现状约束

### 2.1 应用配置

后端当前通过 `YUNXIA_*` 环境变量读取配置，关键默认值：

- `YUNXIA_SERVER_HOST=0.0.0.0`
- `YUNXIA_SERVER_PORT=8080`
- `YUNXIA_DATABASE_DSN=./data/database.db`
- `YUNXIA_STORAGE_DATA_DIR=./data/storage`
- `YUNXIA_STORAGE_TEMP_DIR=./data/temp`
- `YUNXIA_ARIA2_RPC_URL=http://aria2:6800/jsonrpc`

### 2.2 持久化

后端需要持久化：

- SQLite 数据库文件
- 本地存储源数据
- 上传临时目录

因此容器内需要稳定的数据目录，推荐统一放在 `/app/data`。

### 2.3 Aria2 联动边界

当前任务服务向 Aria2 传递的 `dir` 是业务层的 `save_path` 字符串，而不是自动换算后的本地物理路径。

因此本轮 Docker 方案只保证：

- 后端与 Aria2 在同一 Compose 网络中可通信
- 提供一个共享下载挂载点 `/downloads`
- 便于后续通过“自定义 local source 指向 `/downloads`”的方式联动

不在本轮承诺“默认本地源与离线下载目录自动打通”。

## 3. 推荐方案

### 方案 A：最小可运行单体后端 + Compose Aria2（推荐）

- 后端使用 Go 多阶段构建
- 运行镜像使用 `debian:bookworm-slim`
- Compose 启动两个服务：
  - `backend`
  - `aria2`
- 提供两个核心卷：
  - `backend-data` -> `/app/data`
  - `backend-downloads` -> `/downloads`

**优点**：

- 结构简单
- 不依赖外部 Aria2 镜像行为约定太多
- 后续易拆 dev/prod

**缺点**：

- 默认下载目录与默认 local source 仍需人工配置对齐

### 方案 B：只做后端容器，Aria2 外置

- 只提供后端 `Dockerfile`
- Compose 不内置 Aria2

**优点**：更轻

**缺点**：不符合当前产品“一键后端 + Aria2”预期

### 方案 C：一次性拆 dev/prod 双 Compose

**优点**：后续规范更完整

**缺点**：当前性价比低，超出这轮目标

## 4. 最终采用

采用 **方案 A**。

## 5. 文件设计

### 5.1 `backend/Dockerfile`

职责：

- 构建 `yunxia-server`
- 设置容器默认工作目录 `/app`
- 提供基础运行依赖（如 `ca-certificates` / `tzdata` / `curl`）
- 设置默认环境变量
- 暴露 `8080`
- 声明健康检查

### 5.2 `backend/.dockerignore`

职责：

- 缩小构建上下文
- 排除测试缓存、数据库、临时目录、前端依赖等无关内容

### 5.3 `backend/docker/aria2.Dockerfile`

职责：

- 构建最小 Aria2 运行镜像
- 避免依赖第三方社区镜像的不稳定约定

### 5.4 `backend/docker/aria2.entrypoint.sh`

职责：

- 生成/复用 session 文件
- 统一启动参数
- 支持通过环境变量覆盖 RPC secret、端口和下载目录

### 5.5 `docker-compose.backend.yml`

职责：

- 编排后端与 Aria2
- 暴露端口
- 挂载数据卷
- 配置健康检查
- 提供安全但可本地运行的默认值

### 5.6 `backend/.env.example`

职责：

- 给出常用运行参数模板
- 让用户能快速复制为真实 `.env`

## 6. 验证标准

至少完成以下验证：

1. `docker compose -f docker-compose.backend.yml config` 成功
2. `docker build -f backend/Dockerfile backend` 成功
3. `docker build -f backend/docker/aria2.Dockerfile backend` 成功
4. 现有 `go test ./...` 继续通过

## 7. 兼容性与风险

- SQLite 继续作为默认数据库，适合单机部署
- 运行镜像不做 rootless 强制，优先保证卷挂载可用性
- 离线下载保存路径和默认 local source 的自动映射仍是后续增强项

## 8. 输出结果

交付后，用户应可通过以下流程启动：

```bash
docker compose -f docker-compose.backend.yml up -d --build
```

并通过：

- `http://localhost:8080/api/v1/health`
- `http://localhost:6800/jsonrpc`

验证后端与 Aria2 基本可用。
