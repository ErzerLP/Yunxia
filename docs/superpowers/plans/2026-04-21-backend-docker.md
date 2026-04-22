# Backend Docker Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Yunxia 后端补齐可运行的 Docker / Compose 配置，并让本地用户能一条命令启动 backend + Aria2。

**Architecture:** 后端使用 Go 多阶段构建；运行时使用精简 Debian 镜像；Compose 编排后端服务与自建 Aria2 侧车容器；SQLite 与存储目录通过命名卷持久化。

**Tech Stack:** Go 1.25、Gin、SQLite、Docker、多阶段构建、Docker Compose、Alpine/aria2。

---

### Task 1: 补齐设计与计划文档

**Files:**
- Create: `docs/superpowers/specs/2026-04-21-backend-docker-design.md`
- Create: `docs/superpowers/plans/2026-04-21-backend-docker.md`

- [ ] **Step 1: 写入 Docker 设计文档**
- [ ] **Step 2: 写入实现计划文档**

### Task 2: 实现后端镜像构建配置

**Files:**
- Create: `backend/Dockerfile`
- Create: `backend/.dockerignore`
- Test: `docker build -f backend/Dockerfile backend`

- [ ] **Step 1: 创建 Go 多阶段 `Dockerfile`**
- [ ] **Step 2: 创建 `backend/.dockerignore` 缩小上下文**
- [ ] **Step 3: 运行 `docker build -f backend/Dockerfile backend` 验证镜像可构建**

### Task 3: 实现 Aria2 侧车容器配置

**Files:**
- Create: `backend/docker/aria2.Dockerfile`
- Create: `backend/docker/aria2.entrypoint.sh`
- Test: `docker build -f backend/docker/aria2.Dockerfile backend`

- [ ] **Step 1: 创建最小 Aria2 Dockerfile**
- [ ] **Step 2: 创建 Aria2 启动脚本并设置 session / RPC 参数**
- [ ] **Step 3: 运行 `docker build -f backend/docker/aria2.Dockerfile backend` 验证镜像可构建**

### Task 4: 实现 Compose 与环境变量模板

**Files:**
- Create: `docker-compose.backend.yml`
- Create: `backend/.env.example`
- Test: `docker compose -f docker-compose.backend.yml config`

- [ ] **Step 1: 创建 Compose 文件，编排 backend + aria2 + 数据卷**
- [ ] **Step 2: 创建后端 `.env.example` 提供运行模板**
- [ ] **Step 3: 运行 `docker compose -f docker-compose.backend.yml config` 验证配置可解析**

### Task 5: 回归验证与变更记录

**Files:**
- Modify: `backend/CHANGELOG.md`
- Test: `cd backend && go test ./...`

- [ ] **Step 1: 在 `backend/CHANGELOG.md` 记录 Docker 能力补充**
- [ ] **Step 2: 运行 `cd backend && go test ./...` 确认后端回归通过**
- [ ] **Step 3: 汇总验证结果与已知限制**
