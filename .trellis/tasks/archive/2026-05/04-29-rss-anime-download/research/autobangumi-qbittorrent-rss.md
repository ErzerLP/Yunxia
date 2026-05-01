# AutoBangumi / qBittorrent RSS 调研摘要

日期：2026-04-29

## AutoBangumi 参考点

来源：https://github.com/EstrellaXD/Auto_Bangumi

AutoBangumi 是基于 RSS 的自动追番整理下载工具，核心能力包括：

- RSS 解析并自动生成下载规则。
- 通过 qBittorrent 下载番剧。
- 下载后按番剧、季度、集数整理目录。
- 自动重命名为媒体库友好的格式，例如 `Title S01E07.ext`。
- 支持多 RSS 站点、聚合 RSS、番剧管理、日历、搜索源等高级功能。

对 Yunxia 的启发：不要一开始复制完整 AutoBangumi，而是优先做“RSS 订阅 + qBittorrent 下载 + 下载完成导入 Yunxia 虚拟目录”的闭环；番剧识别、重命名、补番、媒体库格式可以作为后续增强。

## qBittorrent Web API RSS 能力

来源：https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-%28qBittorrent-5.0%29

qBittorrent Web API v5 文档中 RSS API 位于 `/api/v2/rss/*`，包括：

- `addFolder`：创建 RSS 文件夹。
- `addFeed`：添加 RSS feed。
- `items`：获取 RSS feed 和文章。
- `refreshItem`：刷新 feed。
- `setRule`：设置 RSS 自动下载规则。
- `rules`：获取规则列表。
- `matchingArticles`：获取规则匹配文章。

`setRule` 的规则字段包括 `enabled`、`mustContain`、`mustNotContain`、`useRegex`、`episodeFilter`、`smartFilter`、`affectedFeeds`、`assignedCategory`、`savePath` 等。

## 初步判断

Yunxia 当前已有：

- 离线下载任务模型和 API。
- Aria2 staging + 导入存储源的逻辑。
- 统一 VFS 虚拟目录和 ACL。
- 任务状态、下载完成导入、本地/S3 存储源驱动。

RSS 番剧下载更适合新增独立模块，而不是改造现有 `tasks` 为 RSS：

- `rss_sources`：RSS 源配置。
- `rss_subscriptions`：番剧订阅规则。
- `rss_items`：抓取到的条目和匹配状态。
- `rss_download_jobs` 或复用 `download_tasks`：把命中的 torrent/magnet 转成下载任务。
- `qbittorrent_client`：作为下载器适配器，至少支持登录、添加 torrent/magnet、查询 torrent 状态。

## 关键设计分歧

1. 是否直接使用 qBittorrent 内置 RSS 自动下载规则？
   - 优点：少写 RSS 匹配调度逻辑。
   - 缺点：qBittorrent 只能写宿主/容器路径，无法天然理解 Yunxia VFS、ACL、非本地存储源导入。

2. 是否由 Yunxia 自己抓 RSS、自己匹配规则，然后只把匹配结果交给 qBittorrent 下载？
   - 优点：目标目录、权限、导入存储源、订阅状态都由 Yunxia 控制。
   - 缺点：要实现 RSS 拉取、解析、去重、匹配调度。

3. 是否把 AutoBangumi 作为外部服务集成？
   - 优点：番剧识别和重命名能力强。
   - 缺点：Yunxia 会依赖另一个完整系统，和现有 VFS/ACL/任务模型融合较差。

推荐方案：Yunxia 自建 RSS 订阅模块，qBittorrent 只作为 BT 下载执行器；下载完成后走 Yunxia staging/import 流程进入指定虚拟目录。
