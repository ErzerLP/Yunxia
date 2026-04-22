# 云匣文档总索引

> **版本**: v1.0  
> **日期**: 2026-04-21  
> **用途**: 统一阅读顺序、术语约定、跨文档真相源  
> **适用范围**: `PRD.md` / `TAD.md` / `INTERFACE-ARCHITECTURE.md` / `DESIGN.md` / `FRONTEND-DESIGN.md`

---

## 1. 阅读顺序

1. `PRD.md` —— 先看产品目标、范围、验收标准
2. `TAD.md` —— 再看架构决策、接口真值表、部署边界
3. `INTERFACE-ARCHITECTURE.md` —— 再看共享抽象、依赖注入与可替换性规范
4. `DESIGN.md` —— 最后看后端实现细节
5. `FRONTEND-DESIGN.md` —— 并行查看前端页面与交互设计

## 2. 文档职责与真相源

| 文档 | 回答的问题 | 真相源范围 |
|------|------------|-----------|
| `PRD.md` | 做什么 | 产品范围、优先级、验收标准 |
| `TAD.md` | 怎么架构 | 分层边界、路由真值表、部署策略 |
| `INTERFACE-ARCHITECTURE.md` | 共享抽象怎么定义 | DB/Cache/Logger/Downloader/DI 规范 |
| `DESIGN.md` | 后端怎么实现 | 模块拆分、实体、应用服务、流程落地 |
| `FRONTEND-DESIGN.md` | 前端怎么呈现 | 布局、组件、交互、状态管理 |

**冲突处理顺序：**
1. `DOCS-INDEX.md` 中的统一约定
2. `PRD.md` 的产品边界
3. `TAD.md` 的架构与路由真值表
4. `INTERFACE-ARCHITECTURE.md` 的共享抽象规范
5. `DESIGN.md` / `FRONTEND-DESIGN.md` 的落地实现细节

## 3. 全局统一约定

### 3.1 架构术语

- 统一术语：**DDD 分层架构**
- 严格表达：**接口适配层 / 应用层 / 领域层 / 基础设施层**（逻辑四层）
- 工程口语里出现的“**三层架构**”均视为上述分层架构的简称

### 3.2 健康检查路由

- 唯一公开健康检查接口：`GET /api/v1/health`
- Docker `healthcheck`、开发排期、路由真值表均以此为准

### 3.3 上传分片路由

- 唯一分片上传路由：`PUT /api/v1/upload/chunk`
- 本地磁盘走该接口
- S3 走预签名 URL 直传

### 3.4 搜索能力边界

- MVP / P0：文件名模糊搜索
- P1：本地索引 + 全文搜索
- PostgreSQL 全文搜索方案以 `TAD.md` 为准

### 3.5 依赖抽象约定

- 应用层优先依赖共享抽象，不直接依赖第三方 SDK 细节
- 下载器统一依赖 `Downloader` 接口，MVP 默认实现为 `Aria2Client`
- 可替换性与 DI 容器规范以 `INTERFACE-ARCHITECTURE.md` 为准

## 4. 维护建议

- 发生路由变更时：至少同时更新 `TAD.md` 真值表、`DESIGN.md` 路由代码片段、相关流程图
- 发生抽象层变更时：先更新 `INTERFACE-ARCHITECTURE.md`，再同步 `TAD.md` / `DESIGN.md`
- 发生产品范围变化时：先更新 `PRD.md`，再回推影响到的技术文档

---

*本文件用于降低多文档并行演进时的漂移风险。*
