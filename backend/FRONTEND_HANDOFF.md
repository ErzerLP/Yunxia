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
| 待适配 | 2026-05-04 | 存储源/PikPak | 设置/存储源页、文件/VFS 页 | P1 | `/api/v1/sources*`、`/api/v1/files*`、`/api/v2/fs*` | [详情](#handoff-2026-05-04-pikpak-source-readonly) |
| 待联调 | 2026-05-02 | 通知告警 | 设置/通知页、RSS 待处理入口 | P1 | `/api/v1/notifications/channels`、`/api/v1/notifications/events` | [详情](#handoff-2026-05-02-notifications) |
| 待联调 | 2026-05-02 | RSS 导入导出 | RSS/追番页、设置/备份页 | P1 | `/api/v1/rss/export`、`/api/v1/rss/import` | [详情](#handoff-2026-05-02-rss-import-export) |
| 待联调 | 2026-05-02 | RSS 订阅批量控制 | RSS/追番页、订阅列表 | P1 | `/api/v1/rss/subscriptions/:id/clone`、`/api/v1/rss/subscriptions/batch-state` | [详情](#handoff-2026-05-02-rss-subscription-bulk-controls) |
| 待联调 | 2026-05-02 | RSS 管理动作 | RSS/追番页、订阅编辑页、条目列表 | P1 | `/api/v1/rss/subscriptions/preview`、`/api/v1/rss/items/batch-ignore`、`/api/v1/rss/items/batch-retry` | [详情](#handoff-2026-05-02-rss-management-actions) |
| 待联调 | 2026-05-02 | RSS 番剧模板 | RSS/追番页、任务页 | P1 | `/api/v1/rss/subscriptions`、`/api/v1/rss/items`、`/api/v1/tasks` | [详情](#handoff-2026-05-02-rss-anime-templates) |
| 已适配 | 2026-04-30 | RSS 无人值守 | RSS/追番页、任务页 | P1 | `/api/v1/rss/sources/refresh-all`、`/api/v1/rss/subscriptions/:id/preview`、`/api/v1/rss/items/:id/reprocess`、`/api/v1/rss/items/:id/retry` | [详情](#handoff-2026-04-30-rss-unattended) |
| 已适配 | 2026-04-29 | RSS 订阅 | RSS/追番页、任务页、文件页 | P1 | `/api/v1/rss/*`、`/api/v1/tasks` | [详情](#handoff-2026-04-29-rss) |

---

## 交接记录

<a id="handoff-2026-04-29-rss"></a>

### [P1][已适配][RSS] 2026-04-29 RSS 番剧订阅下载 MVP

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
- [x] 联调下载完成后 VFS 目标目录可见。

#### 前端验证记录

- 2026-04-30 前端静态验证通过：
  - `cd web && npm run lint`
  - `cd web && npm run build`
  - `cd web && node scripts/check-vfs-integration.mjs`
- 当前状态为 `已适配`：2026-05-01 基于测试负责人完成反馈，端到端联调与回归已在测试机 `test`、`main@8df8468`/当前测试上下文完成，覆盖 RSS 命中、qBittorrent 入队、任务页展示、补充 RSS 源/订阅 CRUD 与 VFS 目标目录可见验证；前端适配关闭。

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

---

<a id="handoff-2026-04-30-rss-unattended"></a>

### [P1][已适配][RSS] 2026-04-30 RSS 无人值守可用性增强

#### 前端适配 checklist

- [x] RSS 源列表/详情展示健康状态：`health_status`、`consecutive_failures`、`last_success_at`、`next_refresh_at`、`last_refresh_status`、`last_refresh_stats`、`last_error`。
- [x] 接入“刷新全部启用源”：`POST /api/v1/rss/sources/refresh-all`，展示每个源 `success/failed/skipped` 结果；该接口会强制刷新启用源，`skipped` 仅表示该源已有刷新在进行。
- [x] 订阅详情或编辑页接入规则预览：`POST /api/v1/rss/subscriptions/:id/preview`，展示 `matched/missing/excluded` 解释。
- [x] 条目列表支持新状态筛选：`retry_pending`、`completed`、`needs_attention`。
- [x] 条目卡片/详情展示重试字段：`retry_count/max_retry_count`、`last_attempt_at`、`next_retry_at`、`retry_reason`。
- [x] 接入单条重新处理：`POST /api/v1/rss/items/:id/reprocess`。
- [x] 接入单条手动重试：`POST /api/v1/rss/items/:id/retry`，可选传 `subscription_id`。
- [x] 对 `needs_attention` 做待处理入口，优先展示确定性错误（权限、路径、只读、unsupported）。

#### 背景 / 变更摘要

RSS 后端从“手动跑通”增强为“可无人值守运行”：后台会按 `next_refresh_at` 自动刷新源，源连续失败会退避并进入 `degraded` / `circuit_open`；RSS item 会按 5m / 30m / 2h 做有限自动重试，最多 3 次；task 完成/失败/取消会回写 RSS item，其中取消会进入 `needs_attention` 等待人工确认。

#### 推荐页面流程

1. 进入 RSS 页面：
   - `GET /api/v1/rss/sources`
   - `GET /api/v1/rss/items?status=needs_attention`
2. 源列表突出显示：
   - `health_status=ok`：正常。
   - `health_status=degraded`：连续失败但仍会自动探测。
   - `health_status=circuit_open`：熔断低频探测，提示用户检查源 URL/网络。
3. 用户点击“刷新全部”：
   - `POST /api/v1/rss/sources/refresh-all`
   - 对 `failed` item 展示 `error`，不要因为单源失败隐藏其他成功源。
4. 用户编辑规则前/后点击“预览”：
   - `POST /api/v1/rss/subscriptions/:id/preview`
   - `result=matched` 展示命中；`missing` 展示缺失关键词；`excluded` 展示被排除关键词。
5. 条目列表：
   - `retry_pending`：展示下次自动重试时间，可提供“立即重试”。
   - `needs_attention`：展示错误原因和“重新处理/重试”按钮。
   - `completed`：展示已完成，可跳转 task 或目标目录。

#### 新增/更新接口摘要

| 场景 | 方法 | 路径 | 权限 | 返回 |
|---|---|---|---|---|
| 刷新全部启用源 | POST | `/api/v1/rss/sources/refresh-all` | `rss.manage` | `RSSRefreshAllResponse` |
| 规则预览 | POST | `/api/v1/rss/subscriptions/:id/preview` | `rss.manage` | `RSSSubscriptionPreviewResponse` |
| 单条重新处理 | POST | `/api/v1/rss/items/:id/reprocess` | `rss.manage` | `{item}` |
| 单条手动重试 | POST | `/api/v1/rss/items/:id/retry` | `rss.manage` | `{item}` |
| 待处理条目 | GET | `/api/v1/rss/items?status=needs_attention` | `rss.read` | `{items}` |

#### 新增字段 / 状态

`RSSSourceView` 新增：

```text
health_status: ok | degraded | circuit_open
consecutive_failures
last_success_at
next_refresh_at
last_refresh_status: success | failed | ""
last_refresh_stats
```

`RSSItemView.status` 新增：

```text
retry_pending
completed
needs_attention
```

`RSSItemView` 新增重试字段：

```text
retry_count
max_retry_count
last_attempt_at
next_retry_at
retry_reason
```

常见 `retry_reason`：

```text
downloader_unavailable
torrent_fetch_failed
task_failed
stalled
deterministic_error
```

#### 注意事项

- 自动重试只处理临时错误；权限、路径、只读、unsupported 等确定性错误会进入 `needs_attention`。
- 手动 retry 会绕过 `next_retry_at`，但仍增加 `retry_count`。
- 已有关联非终态 task 的 item 不会重复入队；自动源刷新也不会绕过 `retry_pending` / `needs_attention` / `completed` 状态重复入队；前端不需要额外防抖来避免重复 task，但按钮仍建议 loading/disabled。
- 详细字段以 `backend/API_CONTRACT.md` 的 `3.10 rss` 为准。

#### 前端验证记录

状态：`已适配`。2026-04-30 复核时前端静态检查、构建和 VFS 静态集成检查通过；2026-05-01 基于测试负责人完成反馈，端到端联调与回归已在测试机 `test`、`main@8df8468`/当前测试上下文完成，覆盖刷新全部、规则预览、retry/reprocess、`needs_attention`、健康/重试字段展示与自动重试/完成回写展示；前端适配关闭。

```bash
cd web
npm run lint # pass
npm run build # pass
node scripts/check-vfs-integration.mjs # pass
```

2026-05-01 测试完成说明：运行环境 smoke 与前序待回归项已由测试负责人补充完成，当前无前端适配阻塞项。

---

<a id="handoff-2026-05-02-rss-anime-templates"></a>

### [P1][待联调][RSS] 2026-05-02 RSS 番剧解析与目录模板

#### 前端适配 checklist

- [x] `RSSSubscriptionView` / 创建更新表单接入 `directory_template`。
- [x] `RSSSubscriptionView` / 创建更新表单接入 `filename_template`，并说明它会在单文件 RSS 下载导入完成时实际重命名。
- [x] `RSSItemView` 展示 `parsed.anime_title`、`parsed.season`、`parsed.episode`、`parsed.subtitle_group`、`parsed.resolution`。
- [x] 订阅编辑页给出模板占位符提示：`{anime_title}`、`{season}`、`{episode}`、`{subtitle_group}`、`{resolution}`、`{title}`。
- [x] 模板输入提示路径安全限制：目录模板必须是相对路径；不要输入 `..`、绝对路径或反斜杠。
- [x] 条目/任务联动页说明：`directory_template` 会影响后续 RSS 入队任务的 `target_virtual_parent_path`；`filename_template` 会写入任务 `target_filename` 快照。

#### 背景 / 变更摘要

后端已新增 AutoBangumi-lite 标题解析：RSS 条目刷新/创建时会从常见 Mikan/字幕组标题提取番剧名、季度、集数、字幕组和分辨率，并持久化到 item。订阅新增 `directory_template` 与 `filename_template` 字段；目录模板会在入队前基于条目解析结果渲染为 `target_virtual_parent_path` 下的安全子目录；文件名模板会渲染为离线任务的 `target_filename` 快照，并在任务完成后的后端导入阶段对单文件产物重命名。

#### 接口与字段

完整契约见 `backend/API_CONTRACT.md` 的 `3.10 rss`。前端重点变更：

```ts
type RSSAnimeParsedView = {
  anime_title: string
  season: string
  episode: string
  subtitle_group: string
  resolution: string
}

type RSSItemView = {
  parsed: RSSAnimeParsedView
}

type RSSSubscriptionView = {
  directory_template: string
  filename_template: string
}

type DownloadTaskView = {
  target_filename?: string
}
```

订阅创建/更新请求可传：

```json
{
  "target_virtual_parent_path": "/anime",
  "directory_template": "{anime_title}/{season}",
  "filename_template": "{anime_title} - {episode} [{resolution}]"
}
```

#### 注意事项

- `directory_template=""`：旧行为不变，入队目标仍为订阅的 `target_virtual_parent_path`。
- `directory_template` 非空：后端会将渲染结果作为安全相对子目录拼到 `target_virtual_parent_path` 下，例如 `/anime/Example Show/S01`。
- 未知占位符或非法模板返回 `PATH_INVALID`；绝对路径、Windows drive-style 前缀（如 `C:/anime` / `C:anime`）、`..` 和反斜杠都属于非法模板；条目解析字段缺失时对应占位会渲染为空或安全 fallback；后端会清洗路径片段。
- `filename_template=""`：旧行为不变，导入时保持下载器产物原文件名。
- `filename_template` 非空：RSS 入队时后端渲染为任务 `target_filename`；下载完成后，如果 staging 中只有一个有效文件，导入到目标目录时会使用该文件名。模板结果不含明确扩展名（如 `.mkv` / `.mp4`；`S01.05` 这类集数后缀不算扩展名）时后端保留原文件扩展名，例如模板渲染为 `Example Show - S01E05 [1080p]`、原文件为 `.mkv` 时，最终文件为 `Example Show - S01E05 [1080p].mkv`。
- 多文件 torrent / 多文件 staging 不应用 `filename_template`，仍保留原相对路径，避免破坏 torrent 目录结构。
- `DownloadTaskView.target_filename` 是任务创建时的快照，便于任务页展示“计划命名”；最终文件扩展名可能由导入阶段按原文件补齐。

#### 前端验证记录

- 2026-05-03：前端已完成页面/API client/DTO/权限入口适配，静态验证通过；尚未在真实后端运行环境完成端到端 smoke，因此状态为 `待联调`。
  - `cd web && npm run lint` # pass
  - `cd web && npm run build` # pass
  - `cd web && node scripts/check-vfs-integration.mjs` # pass

<a id="handoff-2026-05-02-rss-management-actions"></a>

### [P1][待联调][RSS] 2026-05-02 RSS 管理动作：临时预览、批量忽略、批量重试

#### 前端适配 checklist

- [x] 订阅创建/编辑表单接入临时规则预览：`POST /api/v1/rss/subscriptions/preview`，未保存订阅也可按 `source_id` + 规则查看 `matched/missing/excluded`。
- [x] API client/types 新增 `RSSSubscriptionPreviewRequest`、`RSSItemBatchActionResponse`。
- [x] 条目列表支持多选后调用 `POST /api/v1/rss/items/batch-ignore`。
- [x] 条目列表支持多选后调用 `POST /api/v1/rss/items/batch-retry`，保留可选 `subscription_id`。
- [x] 批量动作结果按 `items[].success` 展示部分成功/失败，不要只看 HTTP 成功状态。
- [x] 对 `TASK_INVALID_STATE`、`DOWNLOAD_LINK_UNSUPPORTED`、`RSS_REGEX_INVALID` 给出用户可理解提示。

#### 背景 / 变更摘要

后端补齐三项 RSS 高频管理动作：规则可以在保存前临时 preview；误命中的 RSS item 可以批量忽略；失败或待处理 item 可以批量手动 retry。所有接口仍要求 `rss.manage`，并按 item/source owner 做后端授权。

#### 接口与字段

完整契约见 `backend/API_CONTRACT.md` 的 `3.10 rss`。新增接口：

| 动作 | 方法 | 路径 | 权限 | 成功响应 |
|---|---|---|---|---|
| 临时规则 preview | POST | `/api/v1/rss/subscriptions/preview` | `rss.manage` | `RSSSubscriptionPreviewResponse`，`subscription_id=0` |
| 批量忽略 item | POST | `/api/v1/rss/items/batch-ignore` | `rss.manage` | `RSSItemBatchActionResponse` |
| 批量重试 item | POST | `/api/v1/rss/items/batch-retry` | `rss.manage` | `RSSItemBatchActionResponse` |

临时 preview 请求：

```json
{
  "source_id": 1,
  "must_contain": ["Frieren", "1080p"],
  "must_not_contain": ["CHT"],
  "use_regex": false,
  "case_sensitive": false
}
```

批量忽略：

```json
{
  "item_ids": [10, 11, 12]
}
```

批量重试：

```json
{
  "item_ids": [10, 11, 12],
  "subscription_id": 3
}
```

批量响应：

```ts
type RSSItemBatchActionResponse = {
  items: Array<{
    item_id: number
    success: boolean
    item?: RSSItemView
    error_code?: string
    error_message?: string
  }>
  succeeded: number
  failed: number
}
```

#### 注意事项

- 临时 preview 不要求 `target_virtual_parent_path`，也不会创建/更新 subscription；返回结构沿用已保存订阅 preview，差异是 `subscription_id=0`。
- 批量忽略成功项会变成 `ignored`，并清空 `error_message`、`retry_reason`、`next_retry_at`；已完成或已有活跃 task 的 item 会单项失败 `TASK_INVALID_STATE`。
- 批量 retry 复用单条 retry 语义；已有关联非终态 task 的 item 不会重复入队，失败项通过 `error_code/error_message` 返回。
- 批量接口的 HTTP 200/202 只表示请求被处理；前端必须读取 `succeeded/failed` 和每项结果。

#### 前端验证记录

- 2026-05-03：前端已完成页面/API client/DTO/权限入口适配，静态验证通过；尚未在真实后端运行环境完成端到端 smoke，因此状态为 `待联调`。
  - `cd web && npm run lint` # pass
  - `cd web && npm run build` # pass
  - `cd web && node scripts/check-vfs-integration.mjs` # pass

<a id="handoff-2026-05-02-rss-subscription-bulk-controls"></a>

### [P1][待联调][RSS] 2026-05-02 RSS 订阅复制与批量启停

#### 前端适配 checklist

- [x] 订阅列表新增“复制”动作：调用 `POST /api/v1/rss/subscriptions/:id/clone`，成功后插入或刷新订阅列表。
- [x] 复制弹窗/快捷动作支持可选 `name` 与 `is_enabled`；不传 `name` 时后端生成 `原名 Copy`，不传 `is_enabled` 时保持原状态。
- [x] 订阅列表支持多选后调用 `POST /api/v1/rss/subscriptions/batch-state` 实现批量启用/禁用。
- [x] API client/types 新增 `RSSSubscriptionCloneRequest`、`RSSSubscriptionBatchStateRequest`、`RSSSubscriptionBatchStateResponse`。
- [x] 批量启停结果按 `items[].success` 展示部分成功/失败，不要只看 HTTP 200。
- [x] 对 `RSS_SUBSCRIPTION_NOT_FOUND`、`PERMISSION_DENIED`、`CONFIG_INVALID` 给出用户可理解提示，并在失败后刷新列表。

#### 背景 / 变更摘要

后端补齐 RSS 订阅管理列表的两个高频操作：复制已有订阅规则，以及批量启用/禁用订阅。所有接口仍要求 `rss.manage`；复制和批量状态更新都会按原订阅 owner 做后端授权。

#### 接口与字段

完整契约见 `backend/API_CONTRACT.md` 的 `3.10 rss`。新增接口：

| 动作 | 方法 | 路径 | 权限 | 成功响应 |
|---|---|---|---|---|
| 复制订阅 | POST | `/api/v1/rss/subscriptions/:id/clone` | `rss.manage` | 201，`{subscription}` |
| 批量启停订阅 | POST | `/api/v1/rss/subscriptions/batch-state` | `rss.manage` | 200，`RSSSubscriptionBatchStateResponse` |

复制订阅请求体可为空：

```json
{
  "name": "Frieren 1080p Copy",
  "is_enabled": false
}
```

批量启停请求：

```json
{
  "subscription_ids": [1, 2, 3],
  "is_enabled": false
}
```

批量启停响应：

```ts
type RSSSubscriptionBatchStateResponse = {
  items: Array<{
    subscription_id: number
    success: boolean
    subscription?: RSSSubscriptionView
    error_code?: string
    error_message?: string
  }>
  succeeded: number
  failed: number
}
```

#### 注意事项

- 复制会保留 `source_id`、匹配规则、regex/case 设置、`target_virtual_parent_path`、`directory_template`、`filename_template`、`resolved_source_id`、`resolved_inner_parent_path`，但使用新的 `id/created_at/updated_at`；不会修改原订阅。
- 批量启停成功项会返回更新后的 `RSSSubscriptionView`；失败项只影响对应 `subscription_id`，整体 HTTP 仍为 200。
- `is_enabled=false` 是合法值；前端提交批量启停时必须显式带上 `is_enabled` 字段。

#### 前端验证记录

- 2026-05-03：前端已完成页面/API client/DTO/权限入口适配，静态验证通过；尚未在真实后端运行环境完成端到端 smoke，因此状态为 `待联调`。
  - `cd web && npm run lint` # pass
  - `cd web && npm run build` # pass
  - `cd web && node scripts/check-vfs-integration.mjs` # pass

<a id="handoff-2026-05-02-rss-import-export"></a>

### [P1][待联调][RSS] 2026-05-02 RSS 源/订阅导入导出

#### 前端适配 checklist

- [x] RSS 页面或设置/备份页新增“导出配置”：调用 `GET /api/v1/rss/export` 并下载/复制返回 JSON。
- [x] 新增“导入配置”入口：读取 JSON 后调用 `POST /api/v1/rss/import`。
- [x] 支持 `dry_run=true` 预检，展示每个 source/subscription 的 `action/success/error_code/error_message` 与汇总计数。
- [x] API client/types 新增 `RSSExportResponse`、`RSSImportRequest`、`RSSImportResponse`。
- [x] 导入部分失败时不要把 HTTP 200 当作全成功，必须读取 `sources.failed`、`subscriptions.failed` 和 `items[]`。
- [x] 对 `CONFIG_INVALID`、`PATH_INVALID`、`NO_BACKING_STORAGE`、`SOURCE_READ_ONLY`、`PERMISSION_DENIED`、`RSS_SOURCE_NOT_FOUND` 给出逐项提示。

#### 背景 / 变更摘要

后端新增 RSS 配置迁移能力：导出只包含 RSS 源与订阅规则的可迁移配置，不包含 RSS item、下载任务、刷新健康、retry 等运行时状态；导入按当前用户 owner + URL 精确复用已有 source，不覆盖已有 source，缺失 source 才创建。订阅导入会重新校验目标 VFS 路径可写，单项失败不会中断整个导入。

#### 接口与字段

完整契约见 `backend/API_CONTRACT.md` 的 `3.10 rss`。新增接口：

| 动作 | 方法 | 路径 | 权限 | 成功响应 |
|---|---|---|---|---|
| 导出 RSS 配置 | GET | `/api/v1/rss/export` | `rss.manage` | 200，`RSSExportResponse` |
| 导入 RSS 配置 | POST | `/api/v1/rss/import` | `rss.manage` | 200，`RSSImportResponse` |

导出响应核心字段：

```ts
type RSSExportResponse = {
  version: number
  exported_at: string
  sources: Array<{
    name: string
    url: string
    is_enabled: boolean
    refresh_interval_seconds: number
  }>
  subscriptions: Array<{
    source_url: string
    name: string
    is_enabled: boolean
    must_contain: string[]
    must_not_contain: string[]
    use_regex: boolean
    case_sensitive: boolean
    target_virtual_parent_path: string
    directory_template: string
    filename_template: string
  }>
}
```

导入请求：

```ts
type RSSImportRequest = {
  dry_run: boolean
  sources: RSSExportResponse["sources"]
  subscriptions: Array<RSSExportResponse["subscriptions"][number] & {
    source_ref?: string
  }>
}
```

导入响应：

```ts
type RSSImportResponse = {
  dry_run: boolean
  sources: RSSImportSectionResult
  subscriptions: RSSImportSectionResult
}

type RSSImportSectionResult = {
  items: Array<{
    index: number
    action: "create" | "reuse" | "skip" | "failed"
    success: boolean
    id?: number
    source_url?: string
    name?: string
    error_code?: string
    error_message?: string
  }>
  created: number
  reused: number
  skipped: number
  failed: number
}
```

#### 注意事项

- `dry_run=true` 会完整执行字段与目标路径校验，但不会创建 source/subscription；`created` 表示“将创建”的数量。
- 已存在 source 按当前导入 owner + URL 精确复用，不会覆盖名称、启用状态或刷新间隔。
- subscription 推荐使用 `source_url` 关联 source；`source_ref` 只是兼容别名。
- 导出范围沿用后端现有 RSS service auth 语义；具备 `rss.manage` 的管理身份会按 `IncludeAll` 导出可管理范围。

#### 前端验证记录

- 2026-05-03：前端已完成页面/API client/DTO/权限入口适配，静态验证通过；尚未在真实后端运行环境完成端到端 smoke，因此状态为 `待联调`。
  - `cd web && npm run lint` # pass
  - `cd web && npm run build` # pass
  - `cd web && node scripts/check-vfs-integration.mjs` # pass

<a id="handoff-2026-05-02-notifications"></a>

### [P1][待联调][通知告警] 2026-05-02 Webhook 通知与 RSS 告警事件

#### 前端适配 checklist

- [x] 新增设置/通知页或管理区块，接入通知通道列表：`GET /api/v1/notifications/channels`。
- [x] 支持创建 / 更新 / 删除 Webhook 通道：`POST|PUT|DELETE /api/v1/notifications/channels`。
- [x] 支持测试发送：`POST /api/v1/notifications/channels/:id/test`，失败时展示 `NOTIFICATION_DELIVERY_FAILED`。
- [x] 新增通知事件列表或 RSS 待处理入口：`GET /api/v1/notifications/events?status=retry_pending|failed&event_type=...`。
- [x] 支持对失败 / 待重试事件手动 retry：`POST /api/v1/notifications/events/:id/retry`。
- [x] API client/types 新增 `NotificationChannelView`、`NotificationEventView`、`NotificationChannelUpsertRequest`。
- [x] UI 权限控制：通道管理看 `notification.manage`，事件查看看 `notification.read`。

#### 背景 / 变更摘要

后端新增通知模块，当前通道类型只支持 `webhook`，用于把 RSS 无人值守过程中的关键状态推送到外部系统，并为前端提供可查询的待处理事件列表。事件先入库再投递；Webhook 失败会进入 `retry_pending` / `failed`，不会丢失事件。

#### 接口与字段

完整契约见 `backend/API_CONTRACT.md` 的 `3.11 notifications`。核心接口：

| 动作 | 方法 | 路径 | 权限 | 成功响应 |
|---|---|---|---|---|
| 列出通道 | GET | `/api/v1/notifications/channels` | `notification.read` | 200，`{items[]}` |
| 创建通道 | POST | `/api/v1/notifications/channels` | `notification.manage` | 201，`{channel}` |
| 更新通道 | PUT | `/api/v1/notifications/channels/:id` | `notification.manage` | 200，`{channel}` |
| 删除通道 | DELETE | `/api/v1/notifications/channels/:id` | `notification.manage` | 200，`{deleted,id}` |
| 测试通道 | POST | `/api/v1/notifications/channels/:id/test` | `notification.manage` | 200，`{ok:true}` |
| 列出事件 | GET | `/api/v1/notifications/events` | `notification.read` | 200，`{items[]}` |
| 重试事件 | POST | `/api/v1/notifications/events/:id/retry` | `notification.manage` | 202，`{event}` |

通道请求：

```ts
type NotificationChannelUpsertRequest = {
  name: string
  type: "webhook"
  is_enabled?: boolean
  event_types?: Array<"rss.source_failure" | "rss.item_needs_attention" | "rss.download_completed">
  config: {
    url: string
    secret?: string
  }
}
```

通道响应不会返回 secret 明文：

```ts
type NotificationChannelView = {
  id: number
  name: string
  type: "webhook"
  is_enabled: boolean
  event_types: string[]
  config: { url: string; secret_configured: boolean }
  created_at: string
  updated_at: string
}
```

事件状态：

```ts
type NotificationEventStatus = "pending" | "delivered" | "retry_pending" | "failed" | "skipped"
```

#### 注意事项

- `event_types=[]` 或省略表示该通道接收全部支持事件。
- `is_enabled` 创建时省略默认为 `true`；更新时省略会保留原启用状态。
- `config.secret` 仅提交；后端响应只给 `secret_configured`。更新时不传 `secret` 会保留旧值，传空字符串会清空。
- Webhook 签名头：`X-Yunxia-Timestamp` 与 `X-Yunxia-Signature: sha256=...`。如果前端只是配置，不需要计算签名。
- `POST /notifications/events/:id/retry` 返回 202 只代表本次重试已处理；最终状态以返回的 `event.status` 为准。
- `skipped` 表示事件产生时没有匹配的启用通道，通常只做历史记录展示。

#### 前端验证记录

- 2026-05-03：前端已完成页面/API client/DTO/权限入口适配，静态验证通过；尚未在真实后端运行环境完成端到端 smoke，因此状态为 `待联调`。
  - `cd web && npm run lint` # pass
  - `cd web && npm run build` # pass
  - `cd web && node scripts/check-vfs-integration.mjs` # pass

<a id="handoff-2026-05-04-pikpak-source-readonly"></a>

### [P1][待适配][存储源/PikPak] 2026-05-04 PikPak 只读 source 与 VFS 浏览

#### 前端适配 checklist

- [ ] 存储源创建/编辑表单新增 `driver_type="pikpak"` 选项。
- [ ] 表单支持 PikPak public config：`root_folder_id`、`platform`、`disable_media_link`、`cache_ttl_seconds`、`download_strategy`。
- [ ] 表单支持 secret patch：`username`、`password`、`refresh_token`、`captcha_token`、`device_id`；编辑时省略未改 secret，清空时传 `null`。
- [ ] 详情页展示 `secret_fields` 掩码；仅在具备 `source.secret.read` 时展示明文 secret。
- [ ] 文件/VFS 页面把 PikPak 阶段 B 当作只读源：隐藏或禁用 mkdir/rename/move/copy/delete/upload/离线下载目标写入入口。
- [ ] 错误提示新增 `SOURCE_OPERATION_UNSUPPORTED`、`CLOUD_AUTH_FAILED`、`CLOUD_TOKEN_INVALID`、`CLOUD_CAPTCHA_REQUIRED`、`CLOUD_RATE_LIMITED`、`CLOUD_PROVIDER_UNAVAILABLE`。
- [ ] 下载沿用现有 access-url/download 流程；PikPak 会在后端鉴权后 302 到 provider 临时链接。

#### 背景 / 变更摘要

后端新增 `driver_type="pikpak"` 的基础 source 管理与只读 FileDriver。阶段 B 支持 source test/create/update/detail/delete、VFS/list/search/stat/access-url/download；写操作、上传导入、离线下载导入暂不支持。

#### 接口与字段

完整契约见 `backend/API_CONTRACT.md` 的 `3.5 sources`、`3.6 files`、`3.7 upload`、`3.9 tasks`、`4. vfs` 和错误码列表。

创建请求核心形态：

```ts
type PikPakSourceConfig = {
  root_folder_id?: string
  platform?: "web" | "android" | "pc"
  disable_media_link?: boolean
  cache_ttl_seconds?: number
  download_strategy?: "redirect"
}

type PikPakSecretPatch = {
  username?: string | null
  password?: string | null
  refresh_token?: string | null
  captcha_token?: string | null
  device_id?: string | null
}
```

#### 注意事项

- `root_path` 必须传 `/`；远端子目录挂载用 `config.root_folder_id`。
- 阶段 B 的 PikPak 源是只读源；写入口如果仍调用，会返回 `422 SOURCE_OPERATION_UNSUPPORTED`。
- `CLOUD_CAPTCHA_REQUIRED` 表示需要管理员完成 PikPak 人工验证后回填 `captcha_token`。
- 更新 source 时不传某个 secret 字段表示保留旧值；传 `null` 表示清空。
