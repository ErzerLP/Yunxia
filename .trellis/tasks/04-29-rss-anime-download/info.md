# RSS 番剧订阅下载 - 技术设计草案

## 总体架构

RSS 模块放在后端现有四层结构中：

- `domain/entity`：RSS 源、订阅、条目实体。
- `domain/repository`：RSS 仓储接口。
- `application/service`：RSS 源管理、刷新、匹配、入队服务。
- `infrastructure`：RSS HTTP fetch/parser、qBittorrent Web API client、GORM repository。
- `interfaces/http`：`/api/v1/rss/*` handler 和 route。

qBittorrent 作为下载器接入现有 TaskService。推荐把现有 `Downloader` 抽象升级为可区分下载器类型的接口，或新增下载器路由器，避免 TaskService 只能持有单个 Aria2 实例。

## 下载器分发

推荐新增 `DownloadRouter`：

- 根据 URL 判断下载器：
  - `magnet:?` 和 `.torrent` → qBittorrent
  - 普通 HTTP/HTTPS → Aria2
- Task 持久化建议新增 `downloader_type` 字段，取值如 `aria2`、`qbittorrent`。
- `Pause`、`Resume`、`Remove`、`TellStatus` 根据 `downloader_type + external_id` 分发。

如果不想一次性改动 TaskService 接口，可临时在 external ID 中编码下载器前缀；但从项目“干净彻底”的长期方向看，不推荐这种隐式编码。

## qBittorrent 外部 ID 策略

qBittorrent `/api/v2/torrents/add` 成功通常只返回文本状态，不直接返回 torrent hash。推荐：

1. 添加任务时生成唯一 tag，例如 `yunxia-task-<uuid>`。
2. 调用 add 时带上 `savepath=<stagingDir>` 和 `tags=<tag>`。
3. 将 tag 或查询到的 hash 作为任务外部标识保存。
4. 状态查询时优先按 tag 查询 torrent，再读取 hash、progress、state、size、downloaded、dlspeed、name。

这样可以稳定地把 qBittorrent torrent 和 Yunxia task 关联起来。

## RSS 入队流程

1. 刷新 RSS 源，解析 item。
2. 对 item 做去重 upsert。
3. 对启用订阅执行匹配。
4. 若匹配且链接类型为 BT/magnet，创建 `download_tasks`。
5. 更新 item 状态为 `enqueued` 并保存 `task_id`。
6. TaskService 后续负责状态同步和完成导入。

## 目标路径校验

订阅创建/更新时：

- 规范化 `target_virtual_parent_path`。
- 通过 VFS resolver 验证路径有 backing storage。
- 校验当前用户对目标目录有写权限。
- 校验底层源不是只读。
- 目标目录不存在时可以尝试创建；失败返回 4xx 业务错误，不应返回 500。

## 测试重点

- RSS item 去重不会重复入队。
- mustContain/mustNotContain/regex/caseSensitive 匹配正确。
- unsupported 链接不会创建任务。
- qBittorrent 客户端登录、添加、查询、暂停、恢复、删除请求格式正确。
- qBittorrent 状态映射正确。
- 目标 VFS 无写权限/只读时创建订阅或入队失败。
- RSS 入队创建的任务完成后能导入本地存储源和非本地导入驱动。
