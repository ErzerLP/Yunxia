# Yunxia Frontend Test Handoff

> 固定文档。前端每次完成页面、交互、API client、权限、VFS/任务流等测试可见更新后，把需要测试/联调负责人重点验证的内容维护在本文。
> 本文面向测试/联调负责人，关注“测什么、怎么测、预期是什么、阻塞在哪里”；后端接口细节仍以 `backend/API_CONTRACT.md` 为准，后端到前端适配队列仍以 `backend/FRONTEND_HANDOFF.md` 为准。

## 使用规则

- 每次前端更新后，如影响用户可见页面、接口联调、权限行为、任务/文件/VFS 流程或重要回归点，必须同步维护：
  1. 顶部 `待测试索引`
  2. 底部 `测试记录 / 交接记录` 详情
- 不为单次前端更新新建零散待测文档；新增测试项优先追加到本文。
- 不删除历史记录；完成测试后只更新状态、checklist 和交接记录。
- 本文只写测试负责人需要的信息：影响范围、关键接口、测试重点、前置条件、步骤、期望结果、阻塞/备注。
- 若接口字段、错误码、权限与本文不一致，以 `backend/API_CONTRACT.md` 为接口真相源；本文更新测试项说明和验证结果。
- 若测试项来自 `backend/FRONTEND_HANDOFF.md`，应保留两个文档的状态语义一致：前端完成但未跑通端到端 smoke 时保持 `待联调`。

## 状态枚举

状态只使用以下固定值，便于搜索和筛选：

```text
待联调
联调中
待回归
阻塞
已通过
暂缓
废弃
```

状态含义：

| 状态 | 含义 | 维护要求 |
|---|---|---|
| 待联调 | 前端静态验证已完成或基本可测，仍需连接后端/下载器/运行环境做端到端验证 | 写清前置环境和 smoke 路径 |
| 联调中 | 测试或联调负责人正在执行，尚未得出结论 | 在详情备注当前进展 |
| 待回归 | 已有能力发生相关改动，需要重点回归确认未退化 | 写清回归范围和基准行为 |
| 阻塞 | 因环境、接口、权限、数据或缺陷无法继续验证 | 写清阻塞原因、责任方向和下一步 |
| 已通过 | 相关测试/联调已完成且结果符合预期 | 记录执行日期、环境和结论 |
| 暂缓 | 当前不进入测试窗口 | 写清暂缓原因 |
| 废弃 | 测试项对应方案/功能已废弃 | 保留历史并说明废弃原因 |

## 检索与维护规则

为避免本文后期变长后难以定位待测内容，维护时遵循以下规则：

- 测试负责人优先看顶部 `待测试索引`，按 `状态`、`模块`、`影响页面`、`优先级`、`关键接口`、`测试重点` 快速筛选。
- 每条索引必须链接到下方稳定锚点；锚点格式固定为 `test-handoff-YYYY-MM-DD-feature`。
- 详情标题固定为 `[优先级][状态][模块] YYYY-MM-DD 标题`，便于全文搜索。
- 同一模块的后续补充优先追加到原详情；只有跨模块或明显独立的新测试项才新增索引行。
- 状态变更时，必须同步更新顶部索引和详情标题。
- 标记 `已通过` 前必须在详情里记录实际测试环境、步骤覆盖情况和结论。
- 当索引行明显过多时，仍保持单文件维护，可在本文内把索引拆成 `待联调/联调中/待回归/已通过/阻塞` 小节；不要新建散落交接文档。

## 待测试索引

| 状态 | 日期 | 模块 | 影响页面 | 优先级 | 关键接口 | 测试重点 | 详情 |
|---|---|---|---|---|---|---|---|
| 阻塞 | 2026-05-03 | RSS/通知增强 | RSS/追番页、任务页、设置页/通知区块 | P1 | `/api/v1/rss/subscriptions/preview`、`/api/v1/rss/items/batch-*`、`/api/v1/rss/subscriptions/:id/clone`、`/api/v1/rss/export`、`/api/v1/notifications/*` | 主流程已联调；阻塞在订阅复制 `is_enabled=false` 未生效、RSS 关联任务取消后 item 未回写 `needs_attention` / 未触发条目待处理通知 | [详情](#test-handoff-2026-05-03-rss-notification-handoff) |
| 已通过 | 2026-04-30 | RSS 无人值守 | RSS/追番页、任务页 | P1 | `/api/v1/rss/sources/refresh-all`、`/api/v1/rss/subscriptions/:id/preview`、`/api/v1/rss/items/:id/reprocess`、`/api/v1/rss/items/:id/retry`、`/api/v1/rss/items?status=needs_attention` | 测试完成反馈确认：刷新全部、规则预览、重试/重处理、`needs_attention`、自动重试/完成回写展示均已覆盖 | [详情](#test-handoff-2026-04-30-rss-unattended) |
| 已通过 | 2026-04-29 | RSS 订阅 | RSS/追番页、任务页、文件页/VFS | P1 | `/api/v1/rss/*`、`/api/v1/tasks`、`/api/v2/fs*` | 测试完成反馈确认：RSS 源/订阅 CRUD、条目、qBittorrent 入队、任务跳转、VFS 目标目录可见均已覆盖 | [详情](#test-handoff-2026-04-29-rss-mvp) |
| 已通过 | 2026-05-01 | 离线下载 / VFS | 离线下载页、任务页、文件页/VFS | P1 | `POST /api/v1/tasks`、`GET /api/v1/tasks/:id`、`GET /api/v2/fs*` | 新建任务使用当前目标虚拟目录，完成后文件出现在该 VFS 目录；旧 `source_id + save_path` 行为不回退 | [详情](#test-handoff-2026-05-01-offline-task-vfs-target) |

---

## 测试记录 / 交接记录

<a id="test-handoff-2026-05-03-rss-notification-handoff"></a>

### [P1][阻塞][RSS/通知增强] 2026-05-03 RSS 模板、批量管理、导入导出与通知告警联调测试

#### 测试目标

确认前端已能消费 2026-05-02 后端 handoff 中的 RSS/通知新增能力：RSS 番剧解析模板、未保存规则预览、条目批量动作、订阅复制/批量启停、RSS 配置导入导出，以及设置页 Webhook 通知通道和通知事件重试。

#### 前置条件

- 使用具备 `rss.read`、`rss.manage`、`notification.read`、`notification.manage`、`task.read_all` 和文件/VFS 查看权限的管理员或等效账号。
- 后端已包含 `backend/FRONTEND_HANDOFF.md` 中 2026-05-02 RSS/通知接口，qBittorrent 与 RSS 测试源可用。
- 至少准备一个可写 VFS 目录，例如 `/local/anime-template-test`。
- 准备可命中的 RSS 条目，标题中最好包含番剧名、季度/集数、字幕组和分辨率；同时准备失败或待处理条目用于批量失败展示。
- 准备一个可接收测试 POST 的 Webhook endpoint；若无法外连，可使用测试环境内 mock endpoint。

#### Checklist

- [x] 订阅创建/编辑表单可填写 `directory_template`、`filename_template`，有占位符和路径安全说明。
- [x] 未保存订阅可执行临时 preview，并展示 `matched/missing/excluded`。
- [x] RSS 条目卡片展示 `parsed.anime_title/season/episode/subtitle_group/resolution`。
- [x] RSS 入队后任务页能展示 `target_filename` 计划命名快照。
- [x] 条目多选批量忽略、批量重试可用；HTTP 200/202 下仍按 `items[].success` 展示部分成功/失败。
- [ ] 订阅复制支持可选新名称和启用状态；订阅批量启用/禁用展示部分成功/失败。
- [x] RSS 配置导出会下载 JSON；导入支持 `dry_run=true` 预检和真实导入，并展示 source/subscription 逐项结果。
- [ ] 设置页通知区块按权限可见；Webhook 通道列表、创建、编辑、删除、测试发送可用。
- [x] 通知事件列表支持状态/事件类型筛选；失败或待重试事件可手动 retry，并以返回的 `event.status` 为准。
- [x] 无 `rss.manage` / `notification.manage` 权限账号不暴露对应管理按钮；仅 `notification.read` 可进入设置页查看通知事件。

#### 测试步骤与期望结果

| 步骤 | 操作 | 期望结果 |
|---|---|---|
| 1 | 打开 RSS/追番页，编辑或新建订阅，填写目录模板和文件名模板 | 表单可保存；安全提示清晰；非法模板失败时有可读提示 |
| 2 | 在未保存/编辑中的规则上点击 preview | 调用临时 preview；展示命中、缺失、排除统计和条目结果 |
| 3 | 刷新 RSS 条目并查看卡片 | parsed 番剧字段可见；字段为空时不出现异常空白布局 |
| 4 | 让命中条目入队并查看任务页 | 任务出现，保存虚拟目录正确；若有文件名模板，任务卡片展示计划命名 |
| 5 | 勾选多个 RSS 条目执行批量忽略/批量重试 | 成功和失败计数准确；单项失败错误码/错误信息可读，列表刷新 |
| 6 | 复制一个订阅并批量禁用/启用多个订阅 | 复制后列表刷新；批量结果按单项 success/failure 展示 |
| 7 | 导出 RSS 配置，再用导入弹窗执行 dry-run 和真实导入 | 下载 JSON 结构可用；预检/导入逐项结果、失败原因和汇总计数可见 |
| 8 | 打开设置页通知区块，新增 Webhook 通道并测试发送 | 通道保存成功；测试发送成功/失败都有明确 toast；失败不显示原始内部错误 |
| 9 | 筛选通知事件并对失败/待重试事件 retry | 筛选参数生效；retry 返回后事件状态按响应刷新 |
| 10 | 使用只读/无权限账号回归 | 入口和按钮按 capability 隐藏或只读；路由 guard 与侧边栏一致 |

#### 期望结果

- RSS 新增管理动作、模板字段和导入导出均可在 UI 中完成闭环。
- 所有批量/导入/通知事件接口都不把 HTTP 成功误判为全部成功，页面能展示逐项失败。
- 通知区块与系统配置区块权限互不误伤：有 `notification.read` 但无 `system.config.read` 的账号仍能查看通知事件。

#### 回归范围

- RSS 既有源/订阅 CRUD、刷新全部、单条 retry/reprocess、任务跳转。
- 任务页状态、保存路径、计划命名和完成后 VFS 可见性。
- 设置页系统配置展示、WebDAV 说明、退出登录等既有功能。
- 侧边栏设置/RSS 入口权限过滤与路由守卫。

#### 阻塞 / 备注

- 若测试环境无法提供 Webhook endpoint，可先记录 mock endpoint 或后端 fixture；未覆盖真实外部投递时需保留待回归说明。
- 若无法稳定制造部分失败样本，可请后端准备失败 item/subscription/import fixture；不能只用全成功样本关闭该项。
- 2026-05-03 实测阻塞：
  - `POST /api/v1/rss/subscriptions/:id/clone` 传入 `{"is_enabled": false}` 后，返回的新订阅仍为 `is_enabled=true`，RSS 页面也显示复制订阅为“启用”。
  - RSS 关联 qBittorrent 任务取消后，任务已为 `canceled`，但关联 RSS item 仍停留在 `enqueued`，未进入 `needs_attention`，也未生成 `rss.item_needs_attention` 通知事件。

#### 交接记录

- 2026-05-03：前端实现和静态验证完成，等待真实后端运行环境联调。已通过：`cd web && npm run lint`、`cd web && npm run build`、`cd web && node scripts/check-vfs-integration.mjs`。
- 2026-05-03：测试负责人在测试机 `test` 清理旧环境后，从 `main@5ada4b2` 拉取仓库并叠加当前本地未提交工作区补丁部署。环境：前端 `http://10.0.0.95:15183`，后端 `http://127.0.0.1:18183`，下载器为内置 Aria2/qBittorrent，Webhook 使用测试环境 mock `http://yunxia-rss-feed:8000/hook` / `/fail`，RSS 数据覆盖本地 fixture 与 `https://mikanani.kas.pub/RSS/Bangumi?bangumiId=3968`。已通过：前端 `npm run lint`、`npm run build`、`node scripts/check-vfs-integration.mjs`；后端 Docker build 与 RSS/Notification 相关 Go 定向测试；本地硬盘挂载原有文件可见；RSS 模板字段、临时 preview、parsed 展示、任务页计划命名、批量忽略/重试部分失败、订阅批量启停部分失败、RSS 导出/导入 dry-run/真实导入、Webhook 成功/失败测试、`rss.source_failure` 通知事件 retry、operator/user 权限 UI 与 API smoke、Mikan 精确命中 `.torrent` 入队。因上方阻塞项，状态调整为 `阻塞`，待后端修复后回归。

---

<a id="test-handoff-2026-04-29-rss-mvp"></a>

### [P1][已通过][RSS] 2026-04-29 RSS 订阅 MVP 联调测试

#### 测试目标

确认 RSS 基础页面已能完成“RSS 源 → 订阅规则 → 条目命中 → qBittorrent 入队 → 离线任务 → VFS 目标目录可见”的最小闭环。

#### 前置条件

- 使用具备 `rss.read`、`rss.manage`、`task.read_all`、文件/VFS 查看权限的管理员或等效账号。
- 后端运行环境可访问，qBittorrent 已启用且健康检查可返回状态。
- 已准备一个可写 VFS 目标目录，例如 `/local/anime-test`。
- 已准备可命中的 RSS 源，条目链接至少包含 `magnet:?` 或 `.torrent` URL。
- 前端已完成静态检查记录：`npm run lint`、`npm run build`、`node scripts/check-vfs-integration.mjs`。

#### Checklist

- [x] RSS/追番入口按权限展示：有 `rss.read` 可见，无权限不可见。
- [x] RSS 页面能展示 qBittorrent 健康状态。
- [x] RSS 源列表、创建、编辑、删除、手动刷新可用。
- [x] RSS 订阅列表、创建、编辑、删除、手动执行可用，且能填写 `target_virtual_parent_path`。
- [x] RSS 条目列表可按源、订阅、状态筛选。
- [x] 条目状态 `new`、`unsupported`、`ignored`、`matched`、`enqueued`、`failed` 文案可理解。
- [x] 命中条目可手动入队；入队后 `task_id` 非空并能跳转任务页。
- [x] 任务页能展示 RSS 创建的任务，`downloader_type=qbittorrent` 时文案正确。
- [x] 下载完成并导入后，文件页/VFS 中目标目录可见新文件。
- [x] 普通 HTTP/HTTPS 离线下载仍按原 Aria2 路径可用。

#### 测试步骤与期望结果

| 步骤 | 操作 | 期望结果 |
|---|---|---|
| 1 | 管理员登录，打开 RSS/追番页 | 页面可进入；无明显接口报错；qBittorrent 健康状态区域可见 |
| 2 | 创建 RSS 源并手动刷新 | 源保存成功；刷新后展示成功/失败提示；条目列表有新数据或明确空态 |
| 3 | 创建订阅，目标目录填写 `/local/anime-test` | 订阅保存成功；详情/列表能看到目标虚拟目录 |
| 4 | 执行订阅或刷新源，让条目命中规则 | 命中条目状态进入 `matched` 或后续可入队状态 |
| 5 | 对命中条目执行手动入队 | 条目状态进入 `enqueued`；`task_id` 出现；重复点击有 loading/disabled 防重复提交表现 |
| 6 | 从条目跳转到任务页 | 能定位或看到对应离线任务；下载器类型为 qBittorrent；进度/错误信息可读 |
| 7 | 等待任务完成并导入 | 任务完成后文件页/VFS 的 `/local/anime-test` 下可见目标文件 |
| 8 | 切换到无 RSS 权限账号 | RSS 入口和管理按钮符合权限预期，不暴露不可用操作 |

#### 期望结果

- RSS MVP 最小闭环端到端跑通。
- 关键失败场景有可读提示，不显示原始数据库或后端内部错误。
- VFS 目标目录与订阅配置一致，任务完成后文件可在文件页验证。

#### 回归范围

- RSS 页面入口、权限守卫、侧边栏展示。
- 任务页任务列表、状态、下载器类型、错误信息展示。
- 文件页/VFS 目录刷新、文件可见性、下载/预览访问链路。
- 普通 HTTP/HTTPS 离线下载不因 RSS/qBittorrent 适配退化。

#### 阻塞 / 备注

- 后续若 qBittorrent 未启用或 Docker/下载器环境不可用，应作为新的阻塞/回归风险记录具体环境原因。
- 若 RSS 源不稳定，可用固定测试 feed 或后端测试数据替代，但必须说明数据来源。

#### 交接记录

- 2026-04-30：前端静态检查、构建和 VFS 静态集成检查已在 `backend/FRONTEND_HANDOFF.md` 中记录通过；尚未完成真实运行环境 smoke，当时状态保持 `待联调`。
- 2026-05-01：测试负责人在测试机 `test` 清理旧环境后，从 `main@8df8468` 重新部署并完成真实运行环境 smoke。环境：前端 `http://10.0.0.95:15181`，后端 `http://127.0.0.1:18181`，RSS fixture 使用本地 feed + `https://mikanani.kas.pub/RSS/Bangumi?bangumiId=3968`。本轮确认 RSS 入口权限、qBittorrent 健康状态、本地 feed 刷新、Mikan `.torrent` 条目解析与精确命中、qBittorrent 入队、任务页展示、无权限账号守卫均符合预期；普通 HTTP 离线下载/VFS 目标目录闭环通过。未等待真实 BT 大文件完成导入，RSS 源/订阅编辑删除也未作为本轮主路径覆盖，当时状态先调整为 `待回归`。
- 2026-05-01（补充）：基于当前测试完成反馈，测试负责人确认 RSS 源/订阅 CRUD、VFS 目标目录可见等前序待回归项已补充覆盖；测试上下文沿用 `test`、`main@8df8468`/当前测试反馈，状态调整为 `已通过`。

---

<a id="test-handoff-2026-04-30-rss-unattended"></a>

### [P1][已通过][RSS] 2026-04-30 RSS 无人值守增强联调测试

#### 测试目标

确认 RSS 无人值守增强在前端可观察、可解释、可人工介入：源健康状态、刷新全部、订阅规则预览、自动重试状态、`needs_attention` 处理入口、手动 retry/reprocess 都能被测试负责人验证。

#### 前置条件

- 满足 RSS MVP 的账号、后端、qBittorrent 和 VFS 前置条件。
- 至少存在一个启用的 RSS 源、一个订阅和若干 RSS 条目。
- 测试数据中尽量包含：可成功入队条目、临时失败条目、确定性失败条目或可模拟的 `needs_attention` 条目。
- 后端支持 RSS 后台刷新、重试和 task 结果回写。

#### Checklist

- [x] RSS 源列表/详情展示 `health_status`、`consecutive_failures`、`last_success_at`、`next_refresh_at`、`last_refresh_status`、`last_refresh_stats`、`last_error`。
- [x] “刷新全部启用源”可调用 `POST /api/v1/rss/sources/refresh-all`，并逐源展示 `success` / `failed` / `skipped`。
- [x] 订阅规则预览可调用 `POST /api/v1/rss/subscriptions/:id/preview`，展示 `matched` / `missing` / `excluded` 解释。
- [x] 条目列表支持 `retry_pending`、`completed`、`needs_attention` 状态筛选和文案。
- [x] 条目卡片/详情展示 `retry_count/max_retry_count`、`last_attempt_at`、`next_retry_at`、`retry_reason`。
- [x] 单条重新处理 `POST /api/v1/rss/items/:id/reprocess` 可用，成功后刷新条目列表。
- [x] 单条手动重试 `POST /api/v1/rss/items/:id/retry` 可用，可按需传 `subscription_id`。
- [x] `needs_attention` 有明显待处理入口，能优先展示权限、路径、只读、unsupported 等确定性错误。
- [x] 已有关联非终态 task 的 item 不会因为重复点击造成重复任务；按钮有 loading/disabled 状态。

#### 测试步骤与期望结果

| 步骤 | 操作 | 期望结果 |
|---|---|---|
| 1 | 打开 RSS 页面，查看源列表 | 每个源健康字段展示完整；`ok`、`degraded`、`circuit_open` 文案可理解 |
| 2 | 点击“刷新全部” | 返回结果按源展示；单个源失败不影响其他源结果展示；`skipped` 解释为已有刷新在进行 |
| 3 | 打开订阅规则预览 | 命中、缺失关键词、排除关键词说明清晰；修改规则后预览结果同步变化 |
| 4 | 筛选 `retry_pending` 条目 | 展示下次重试时间和重试原因；可选择立即重试 |
| 5 | 筛选 `needs_attention` 条目 | 待处理入口明显；错误原因可读；可执行 reprocess 或 retry |
| 6 | 对单条执行 reprocess | 请求成功后条目状态、错误、任务关联刷新；失败时有可读提示 |
| 7 | 对单条执行 retry | 绕过 `next_retry_at` 发起重试；按钮在请求中禁用；结果回写后列表刷新 |
| 8 | 观察 completed 条目 | 已完成条目可追溯到 task 或目标目录，不再重复自动入队 |

#### 期望结果

- 测试负责人能判断 RSS 后台无人值守是否健康运行。
- 临时失败与确定性失败在 UI 上可区分，人工介入路径清晰。
- retry/reprocess 操作不会制造重复任务或隐藏失败。

#### 回归范围

- RSS MVP 基础 CRUD、刷新、手动入队流程。
- 任务页 task 状态回写、错误信息展示和跳转。
- 文件页/VFS 目标目录可见性。
- 权限控制：仅 `rss.manage` 用户可执行刷新、预览、retry/reprocess 等管理操作。

#### 阻塞 / 备注

- 若无法制造失败/重试数据，可请后端提供测试数据或临时测试接口；未覆盖的数据类型必须在本节记录。
- 后续若后台调度未启动或 task 回写不可用，应作为新的阻塞/回归风险记录阻塞范围。

#### 交接记录

- 2026-04-30：前端已完成相关页面/API 接入并通过静态验证；当时仍需真实后端运行环境验证刷新全部、预览、自动重试、task 回写和 `needs_attention` 人工处理闭环。
- 2026-05-01：测试负责人在测试机 `test` 清理旧环境后，从 `main@8df8468` 重新部署并完成主要无人值守 smoke。已覆盖：`refresh-all` 返回成功统计，本地订阅 preview 展示 `matched/missing/excluded`，unsupported 条目 `reprocess` 后进入 `needs_attention`，`needs_attention` 筛选与待处理入口可见，ignored magnet 使用 `retry` + `subscription_id` 可重新生成任务，operator 仅 `rss.read` 时管理接口返回 403 且页面不暴露管理按钮。`retry_pending` / `completed` 筛选接口已验证 200，但本轮未稳定制造自动重试与完成回写样本；当时状态先调整为 `待回归`。
- 2026-05-01（补充）：基于当前测试完成反馈，测试负责人确认健康字段展示、自动重试/完成回写展示等前序待回归项已补充覆盖；测试上下文沿用 `test`、`main@8df8468`/当前测试反馈，状态调整为 `已通过`。

---

<a id="test-handoff-2026-05-01-offline-task-vfs-target"></a>

### [P1][已通过][离线下载/VFS] 2026-05-01 新建任务使用目标虚拟目录回归测试

#### 测试目标

确认离线下载新建任务时前端传递当前目标虚拟目录 `target_virtual_parent_path`，后端按 VFS 目录解析保存位置；任务完成后文件出现在用户选择的 VFS 目录，并且旧任务展示/文件刷新流程不退化。

#### 前置条件

- 使用具备创建离线任务和查看文件/VFS 权限的账号。
- 已准备至少一个可写 VFS 目录，例如 `/local/downloads`。
- 已准备一个可快速完成的 HTTP/HTTPS 测试下载链接。
- 后端 `POST /api/v1/tasks` 支持 `target_virtual_parent_path`。

#### Checklist

- [x] 从离线下载页新建任务时，目标目录字段使用 VFS 虚拟路径，而不是只依赖旧 `source_id + save_path`。
- [x] 创建请求包含 `target_virtual_parent_path=<当前选择目录>`。
- [x] 任务详情/列表展示 `target_virtual_parent_path`、`save_virtual_path` 或等效用户可理解路径。
- [x] 任务完成后文件页/VFS 对应目录刷新并可见新文件。
- [x] 文件下载/预览仍通过 `/api/v2/fs/access-url` / `/api/v2/fs/download` 链路。
- [x] 兼容旧任务数据：旧任务列表、错误任务、取消任务仍能展示。

#### 测试步骤与期望结果

| 步骤 | 操作 | 期望结果 |
|---|---|---|
| 1 | 打开文件页/VFS，确认目标目录 `/local/downloads` 存在且可写 | 目录可进入，权限正常 |
| 2 | 打开离线下载页，新建 HTTP/HTTPS 下载任务，目标目录填写 `/local/downloads` | 前端提交成功；请求体包含 `target_virtual_parent_path` |
| 3 | 查看任务列表/详情 | 新任务保存路径展示为 `/local/downloads` 或等效虚拟路径；状态正常流转 |
| 4 | 等待任务完成 | 任务进入 completed；无异常错误信息 |
| 5 | 回到文件页/VFS 的 `/local/downloads` | 文件可见；列表刷新不需要手动清缓存 |
| 6 | 对文件执行预览或下载 | 走 VFS access-url/download 链路，能正常打开或下载 |
| 7 | 回看旧任务或创建一个兼容旧参数的任务（如环境支持） | 旧数据展示不崩溃，兼容行为不回退 |

#### 期望结果

- 新建离线下载任务默认落到用户指定的 VFS 虚拟目录。
- 任务完成后文件页/VFS 能看到结果，避免“任务完成但文件在 UI 中找不到”。
- 旧任务展示和普通下载能力不受影响。

#### 回归范围

- 离线下载页新建任务表单、目标路径输入、提交 loading/错误提示。
- 任务页列表/详情路径展示、状态展示、完成后刷新。
- 文件页/VFS 查询、下载、预览、目录刷新。
- 上传初始化同样使用 `target_virtual_parent_path` 的既有行为，确认未被离线下载改动影响。

#### 阻塞 / 备注

- 若无法使用真实可下载 URL，可先用后端测试 fixture 或内部测试文件；需记录替代数据来源。
- 若任务完成但 VFS 不可见，优先记录任务详情中的 `target_virtual_parent_path`、`save_virtual_path`、`resolved_source_id`、`resolved_inner_save_path` 以便定位。

#### 交接记录

- 2026-05-01：作为近期离线下载/VFS 修复的重点回归项纳入本文；等待测试负责人在可运行后端环境中执行。
- 2026-05-01：测试负责人在测试机 `test` 清理旧环境后，从 `main@8df8468` 重新部署并完成回归。覆盖数据：本地 host disk 只读挂载预置 `root-preexisting.txt`、`existing-folder/nested-preexisting.txt`、`中文目录/原有文件.txt`、`space-dir/file with space.txt`；HTTP fixture `traffic.bin` 1MiB。结果：挂载本地硬盘后原有文件和中文/空格路径均可见，只读 mkdir 返回 `SOURCE_READ_ONLY`；VFS 本地 mkdir/rename/delete 正常；离线下载弹窗仅保留“下载链接”和“目标虚拟目录”，请求体为 `target_virtual_parent_path=/local/ui-task-34029160`，任务 `id=6` completed，`save_virtual_path=/local/ui-task-34029160`，文件页/VFS 中 `/local/ui-task-34029160/traffic.bin` 可见；旧 `source_id + save_path` 兼容任务也 completed。公开分享 `/s/<token>` smoke 无控制台错误。状态调整为 `已通过`。
