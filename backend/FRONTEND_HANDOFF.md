# Yunxia Frontend Handoff

> 固定文档。后端新增或变更需要前端适配的能力时，只在本文末尾追加详情，不新建零散文档。
> API 细节真相源仍是 `backend/API_CONTRACT.md`。

## 使用规则

- 每次后端新增或变更前端可见能力，必须同时维护：
  1. 顶部 `待适配索引`
  2. 末尾 `交接记录` 详情
- 不删除历史记录。
- 不为单次变更新建独立交接文档。
- API 路由、字段、错误码的完整定义以 `backend/API_CONTRACT.md` 为准。
- 本文重点写：前端需要改什么、怎么调、有哪些坑。
- 前端完成适配后，应同步更新索引状态和详情 checklist。

## 状态枚举

状态只使用以下固定值，便于搜索和筛选：

```text
待适配
适配中
待联调
已适配
暂缓
废弃
```

## 检索与维护规则

为避免本文后期变长后难以定位待适配内容，维护时遵循以下规则：

- 前端优先看顶部 `待适配索引`，按 `状态`、`模块`、`影响页面`、`优先级`、`关键接口` 快速筛选。
- 每条索引必须链接到下方稳定锚点；锚点格式固定为 `handoff-YYYY-MM-DD-feature`。
- 详情标题固定为 `[优先级][状态][模块] YYYY-MM-DD 标题`，便于全文搜索。
- 同一模块的后续补充优先追加到原详情；只有跨模块或明显独立的新适配项才新增索引行。
- 前端完成适配后，不删除历史记录，只把索引和详情标题/checklist 状态改为 `已适配`。
- 当索引行明显过多时，仍保持单文件维护，可在本文内把索引拆成 `待适配/适配中/已适配/暂缓` 小节；不要新建散落交接文档。

## 待适配索引

| 状态 | 日期 | 模块 | 影响页面 | 优先级 | 关键接口 | 详情 |
|---|---|---|---|---|---|---|
| 待联调 | 2026-04-29 | RSS 订阅 | RSS/追番页、任务页、文件页 | P1 | `/api/v1/rss/*`、`/api/v1/tasks` | [详情](#handoff-2026-04-29-rss) |

---

## 交接记录

<a id="handoff-2026-04-29-rss"></a>

### [P1][待联调][RSS] 2026-04-29 RSS 番剧订阅下载 MVP

#### 前端适配 checklist

- [x] 新增 RSS/追番页面入口。
- [x] 新增 `web/src/api/rss.ts` 或等价 API client。
- [x] 新增/补齐 RSS 相关 TypeScript 类型。
- [x] 接入 `rss.read` / `rss.manage` 权限控制。
- [x] 接入 qBittorrent 健康状态展示。
- [x] 接入 RSS 源列表、创建、编辑、删除、手动刷新。
- [x] 接入 RSS 订阅列表、创建、编辑、删除、手动执行。
- [x] 接入 RSS 条目列表和状态筛选。
- [x] 接入条目手动入队。
- [x] 接入 `task_id` 到离线任务页跳转。
- [ ] 联调下载完成后 VFS 目标目录可见。

#### 前端验证记录

- 2026-04-30 前端静态验证通过：
  - `cd web && npm run lint`
  - `cd web && npm run build`
  - `cd web && node scripts/check-vfs-integration.mjs`
- 当前状态为 `待联调`：前端适配已完成，仍需在后端运行环境中按 smoke 流程验证 RSS 命中、qBittorrent 入队、任务完成导入后 VFS 目标目录可见。

#### 1. 本次新增能力

后端新增 RSS 番剧订阅下载 MVP：

- Yunxia 管理 RSS 源、订阅规则、RSS 条目、匹配状态和目标 VFS 目录。
- RSS 第一版只自动处理 BT/magnet：
  - `magnet:?` 链接
  - `.torrent` URL
- 普通 HTTP/HTTPS RSS 条目不会自动入队，会标记为 `unsupported`。
- 下载执行使用 qBittorrent，下载完成后仍由 Yunxia 从 staging 导入订阅指定的 VFS 目录。
- 原有普通 HTTP/HTTPS 离线下载继续使用 Aria2。

完整接口契约见：`backend/API_CONTRACT.md` 的 `3.10 rss（/api/v1/rss）` 和 `3.9 tasks（/api/v1）`。

#### 2. 前端需要新增/调整

建议新增一个 RSS/追番管理页面，至少包含：

- RSS 源列表、创建、编辑、删除、手动刷新。
- RSS 订阅列表、创建、编辑、删除、手动执行。
- RSS 条目列表，可按源、订阅、状态筛选。
- qBittorrent 健康状态展示。
- 从 RSS 条目跳转到离线任务详情或任务列表。

建议新增 API client 模块：

```text
web/src/api/rss.ts
```

建议新增类型模块或合并到现有类型文件：

```text
RSSSourceView
RSSSubscriptionView
RSSItemView
RSSRefreshResponse
RSSQBitHealthResponse
```

#### 3. 推荐页面流程

1. 页面加载时调用：
   - `GET /api/v1/rss/qbittorrent/health`
   - `GET /api/v1/rss/sources`
   - `GET /api/v1/rss/subscriptions`
2. 用户创建 RSS 源：
   - `POST /api/v1/rss/sources`
3. 用户创建订阅：
   - `POST /api/v1/rss/subscriptions`
   - 必须填写 `target_virtual_parent_path`，例如 `/anime/frieren`
4. 用户点击刷新 RSS 源：
   - `POST /api/v1/rss/sources/:id/refresh`
5. 前端刷新条目列表：
   - `GET /api/v1/rss/items?source_id=<id>`
6. 如果条目状态为 `enqueued` 且有 `task_id`，可以跳转任务页查看下载进度。
7. 下载完成后，前端可通过 VFS 文件页查看目标目录。

#### 4. API 摘要

| 场景 | 方法 | 路径 | 权限 | 返回 |
|---|---|---|---|---|
| RSS 源列表 | GET | `/api/v1/rss/sources` | `rss.read` | `{items}` |
| 创建 RSS 源 | POST | `/api/v1/rss/sources` | `rss.manage` | `{source}` |
| RSS 源详情 | GET | `/api/v1/rss/sources/:id` | `rss.read` | 直接 `RSSSourceView` |
| 更新 RSS 源 | PATCH | `/api/v1/rss/sources/:id` | `rss.manage` | `{source}` |
| 删除 RSS 源 | DELETE | `/api/v1/rss/sources/:id` | `rss.manage` | `{deleted,id}` |
| 手动刷新源 | POST | `/api/v1/rss/sources/:id/refresh` | `rss.manage` | `RSSRefreshResponse` |
| 订阅列表 | GET | `/api/v1/rss/subscriptions?source_id=` | `rss.read` | `{items}` |
| 创建订阅 | POST | `/api/v1/rss/subscriptions` | `rss.manage` | `{subscription}` |
| 订阅详情 | GET | `/api/v1/rss/subscriptions/:id` | `rss.read` | 直接 `RSSSubscriptionView` |
| 更新订阅 | PATCH | `/api/v1/rss/subscriptions/:id` | `rss.manage` | `{subscription}` |
| 删除订阅 | DELETE | `/api/v1/rss/subscriptions/:id` | `rss.manage` | `{deleted,id}` |
| 手动执行订阅 | POST | `/api/v1/rss/subscriptions/:id/run` | `rss.manage` | `RSSRefreshResponse` |
| 条目列表 | GET | `/api/v1/rss/items?source_id=&subscription_id=&status=` | `rss.read` | `{items}` |
| 手动入队条目 | POST | `/api/v1/rss/items/:id/download` | `rss.manage` | `{item}` |
| qBittorrent 健康 | GET | `/api/v1/rss/qbittorrent/health` | `rss.read` | `{enabled,status,error}` |

#### 5. 创建请求示例

创建 RSS 源：

```json
{
  "name": "示例 RSS",
  "url": "https://example.com/feed.xml",
  "is_enabled": true,
  "refresh_interval_seconds": 1800
}
```

创建订阅：

```json
{
  "source_id": 1,
  "name": "Frieren 1080p",
  "is_enabled": true,
  "must_contain": ["Frieren", "1080p"],
  "must_not_contain": ["CHT"],
  "use_regex": false,
  "case_sensitive": false,
  "target_virtual_parent_path": "/anime/frieren"
}
```

手动入队条目：

```json
{
  "subscription_id": 1
}
```

如果条目已经有 `matched_subscription_id`，`subscription_id` 可以不传。

#### 6. 关键 DTO / 字段注意

RSS 条目状态：

```text
new
unsupported
ignored
matched
enqueued
failed
```

前端建议展示含义：

| status | 建议文案 |
|---|---|
| `new` | 新条目 |
| `unsupported` | 不支持的链接 |
| `ignored` | 未匹配 |
| `matched` | 已匹配 |
| `enqueued` | 已加入下载 |
| `failed` | 处理失败 |

注意：

- `RSSSourceView.last_refreshed_at` 可为 `null`。
- `RSSSourceView.last_error` 可为 `null`。
- `RSSItemView.published_at` 可为 `null`。
- `RSSItemView.matched_subscription_id` 可为 `null`。
- `RSSItemView.task_id` 可为 `null`。
- `RSSItemView.error_message` 可为 `null`。
- 列表接口返回 `{items: []}`。
- 部分详情接口直接返回对象，不包 `{source}` / `{subscription}`。
- 创建/更新接口会包 `{source}` 或 `{subscription}`。

#### 7. 错误码处理建议

| 错误码 | 建议 UI |
|---|---|
| `CAPABILITY_DENIED` / `PERMISSION_DENIED` | 隐藏入口或提示无权限 |
| `RSS_SOURCE_NOT_FOUND` | 提示 RSS 源不存在并刷新列表 |
| `RSS_SUBSCRIPTION_NOT_FOUND` | 提示订阅不存在并刷新列表 |
| `RSS_ITEM_NOT_FOUND` | 提示条目不存在并刷新列表 |
| `RSS_REGEX_INVALID` | 在规则表单中提示正则非法 |
| `DOWNLOAD_LINK_UNSUPPORTED` | 提示该 RSS 条目不是 BT/magnet，暂不支持下载 |
| `DOWNLOADER_UNAVAILABLE` | 提示 qBittorrent 不可用或未启用 |
| `PATH_INVALID` | 提示目标虚拟目录非法 |
| `NO_BACKING_STORAGE` | 提示目标虚拟目录没有挂载存储源 |
| `SOURCE_READ_ONLY` | 提示目标存储源只读 |

#### 8. 权限与可见性

新增 capability：

```text
rss.read
rss.manage
```

建议前端：

- 没有 `rss.read`：隐藏 RSS 页面入口。
- 有 `rss.read` 但没有 `rss.manage`：允许查看，隐藏新增、编辑、删除、刷新、手动入队按钮。
- `super_admin` / `admin` 默认具备 RSS 管理能力。
- `operator` 当前只有 `rss.read`。
- `user` 默认没有 RSS capability。

#### 9. 联调建议

推荐 smoke 流程：

1. 管理员登录。
2. 检查 `GET /api/v1/rss/qbittorrent/health`。
3. 创建或确认一个可写 VFS 目录，例如 `/local/anime-test`。
4. 创建 RSS 源。
5. 创建订阅，`target_virtual_parent_path=/local/anime-test`。
6. 手动刷新 RSS 源。
7. 查看条目列表。
8. 命中 BT/magnet 后检查条目 `status=enqueued`、`task_id` 非空。
9. 跳转任务列表，确认 `downloader_type=qbittorrent`。
10. 下载完成后，到 VFS 目标目录确认文件可见。

#### 10. 已知限制

- 第一版不做番剧名、季度、集数识别。
- 第一版不做自动重命名。
- 第一版不做补番、日历、复杂番剧元数据管理。
- 第一版 RSS 不自动下载普通 HTTP/HTTPS 直链。
- qBittorrent Docker 侧车配置已静态验证，但本机 Docker daemon 未运行，尚未完成本地容器启动验证。

#### 11. 后端验证状态

已通过：

```bash
cd backend
go test -count=1 ./...
```

已通过：

```bash
docker compose -f docker-compose.backend.yml config
```

已通过：

```bash
bash -n backend/docker/aria2.entrypoint.sh backend/docker/qbittorrent.entrypoint.sh
```
