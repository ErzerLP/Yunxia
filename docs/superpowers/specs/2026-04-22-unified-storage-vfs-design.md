# Yunxia 统一存储与虚拟挂载（VFS）改造设计

**日期**：2026-04-22  
**状态**：已完成设计评审，可进入实施计划阶段  
**设计基线**：

- 当前 Yunxia 后端实现（`backend/internal/**`）
- `docs/21-unified-storage-and-virtual-mounts.md`
- OpenList 的“挂载表 + 最长前缀匹配 + 虚拟目录投影 + 真实/虚拟目录合并”思路

---

## 1. 目标

把 Yunxia 从当前的“按存储源切换浏览”的模型：

- `source_id + path`

升级成“统一虚拟目录树”模型：

- `virtual_path`

用户后续面对的是一棵统一目录树，例如：

- `/影视/阿里云`
- `/影视/本地`
- `/文档/团队`
- `/备份/S3`

系统内部再把 `virtual_path` 解析成：

- 命中的挂载源
- 该源内部路径 `inner_path`

---

## 2. 为什么要改

当前模型的问题：

1. 文件浏览依赖 `source_id`
2. ACL、分享、上传、任务、回收站等模块都直接绑定 `source_id`
3. 前端必须先“选存储源”，再进入该源内部路径
4. 无法自然表达“把不同存储源挂在统一目录树中的不同节点”

目标模型的收益：

1. 用户只面向一棵虚拟目录树
2. 所有业务模块统一围绕 `virtual_path`
3. 底层 driver 差异被收口到 VFS 层
4. 后续扩展更多存储源、WebDAV 统一出口、统一搜索都会更顺

---

## 3. 方案对比

### 方案 A：继续保留 `source_id + path`，只在浏览层加外壳

优点：

- 改动最小

缺点：

- 只是 UI 层“套皮”
- 业务层仍然耦合 `source_id`
- 后续仍需要二次重构

**结论**：不采用。

### 方案 B：北向统一 `virtual_path`，内部保留解析结果（推荐）

做法：

- 所有新业务接口统一使用 `virtual_path`
- VFS 负责解析 `virtual_path -> source + inner_path`
- 长生命周期记录同时保存虚拟路径与解析快照

优点：

- 架构清晰
- 与 OpenList 的基础 VFS 模型一致
- 风险和收益平衡最好

缺点：

- 改动范围较大，需要系统性迁移

**结论**：采用此方案。

### 方案 C：一步到位彻底移除旧模型

优点：

- 最干净

缺点：

- 风险过高
- 迁移窗口过大

**结论**：不作为实施路径。

---

## 4. 最终采用方案

采用 **OpenList 基础版 VFS 路线**：

1. 对外统一使用 `virtual_path`
2. 对内新增 `MountRegistry + PathResolver + VFSService`
3. 每个存储源同时具备：
   - `mount_path`：挂载到虚拟目录树哪里
   - `root_path`：该源内部从哪里开始暴露
4. 支持：
   - 最长前缀匹配
   - 虚拟目录投影
   - 真实目录与虚拟挂载目录合并显示
5. 第一阶段不支持：
   - 多挂载同路径
   - `.balance`
   - Alias 式 union 聚合

---

## 5. 核心概念

### 5.1 `mount_path`

表示存储源在虚拟目录树中的挂载位置。

例如：

- `/影视/阿里云`
- `/文档/团队`
- `/备份/S3`

要求：

- 必须是绝对路径
- 统一规范化
- 全局唯一

### 5.2 `root_path`

表示存储源内部实际暴露的起始路径。

例如：

- local：`base_path=/data`，`root_path=/movies`
- s3：`bucket=abc`，`root_path=/cold`

最终用户访问：

- `/影视/阿里云/电影/a.mp4`

VFS 解析为：

- 命中挂载：`/影视/阿里云`
- `source_id = 7`
- `inner_path = /电影/a.mp4`

底层再由 local / s3 driver 将 `inner_path` 映射到真实存储。

### 5.3 虚拟目录

虚拟目录分两类：

1. **挂载目录**：某个 `mount_path` 自身对应的目录
2. **纯虚拟目录**：仅为承载更深层挂载而存在，本身没有真实后端

例如存在挂载：

- `/docs/team`
- `/docs/personal`

则 `/docs` 是纯虚拟目录。

---

## 6. 分层设计

### 6.1 存储驱动层（保留现有）

继续保留当前：

- local driver
- s3 driver

它们仍然只关心：

- 源内部路径
- 自己的存储语义

不关心整棵虚拟目录树。

### 6.2 新增挂载注册层：`MountRegistry`

职责：

- 加载所有启用的 source
- 维护运行时挂载表
- 提供枚举、前缀扫描和挂载冲突检查

建议运行时结构：

```text
mount_path -> source runtime entry
```

运行时 entry 至少包含：

- source 元信息
- 挂载路径
- 驱动类型
- 驱动实例 / driver provider

### 6.3 新增路径解析层：`PathResolver`

职责：

- 规范化路径
- 基于最长前缀匹配定位挂载源
- 计算 `inner_path`

输入：

- `virtual_path`

输出：

- `matched_mount_path`
- `source`
- `inner_path`
- 是否命中真实挂载

### 6.4 新增虚拟目录投影层：`VirtualDirProjector`

职责：

- 对当前目录 `prefix` 扫描所有更深层挂载
- 生成该目录下应显示的虚拟子目录

示例：

挂载：

- `/movies`
- `/docs/team`
- `/docs/personal`

访问 `/` 时投影出：

- `movies/`
- `docs/`

访问 `/docs` 时投影出：

- `team/`
- `personal/`

### 6.5 新增业务编排层：`VFSService`

职责：

- 提供统一的 `list/get/search/download/mkdir/rename/move/copy/delete/upload-init`
- 合并真实目录结果和虚拟目录结果
- 对外屏蔽 source 差异

---

## 7. 目录与路径解析规则

### 7.1 路径规范化

所有进入 VFS 的路径统一：

- 必须以 `/` 开头
- 清理 `.` 和 `..`
- 统一斜杠
- 空路径归一为 `/`

### 7.2 最长前缀匹配

若存在挂载：

- `/docs`
- `/docs/team`
- `/docs/team/archive`

访问：

- `/docs/team/archive/2024/a.zip`

命中：

- `/docs/team/archive`

### 7.3 纯虚拟目录判定

如果某路径本身没有对应真实挂载，但它是若干挂载的父路径，则该路径视为纯虚拟目录。

例如：

- 挂载：`/影视/阿里云`、`/影视/本地`
- 访问：`/影视`

则 `/影视` 是纯虚拟目录。

### 7.4 写操作落点判定

本设计中的“虚拟路径可写”与“纯虚拟目录可写”不是同一个概念，规则必须区分清楚：

1. **若目标 `virtual_path` 能通过最长前缀匹配唯一命中某个真实挂载源，则允许写操作**
   - 实际写入位置为该挂载源对应的 `inner_path`
   - 这条规则适用于：
     - 上传
     - 新建目录
     - 重命名
     - 移动
     - 复制
     - 删除
2. **若目标 `virtual_path` 只命中中间层纯虚拟目录，没有任何真实挂载源承接该路径，则拒绝写操作**
   - 返回 `NO_BACKING_STORAGE`
   - 不允许系统“猜测”写入到哪个子挂载
3. **若同时存在父挂载和更深层子挂载，则始终以最长前缀命中的子挂载为准**
   - 不允许父挂载越权接管子挂载路径

示例：

- 挂载：
  - `/docs` -> local A
  - `/docs/team` -> s3 B
- 写入：
  - `/docs/readme.md` -> 命中 `/docs`，写入 local A
  - `/docs/team/a.txt` -> 命中 `/docs/team`，写入 s3 B

再例如：

- 挂载：
  - `/影视/阿里云`
  - `/影视/本地`
- 且 `/影视` 本身没有真实挂载
- 那么：
  - `/影视/阿里云/a.mp4` -> 可写
  - `/影视/a.txt` -> 不可写，返回 `NO_BACKING_STORAGE`

---

## 8. 列目录语义

列目录时统一遵循下面的流程：

### 8.1 收集虚拟挂载子目录

先根据所有更深层挂载，为当前目录生成虚拟子目录。

### 8.2 如果当前目录命中真实挂载，则读取真实目录

例如：

- `/docs` 正好也是一个 source 的 `mount_path`

则需要读取该 source 在当前 `inner_path` 下的真实内容。

### 8.3 合并真实内容与虚拟挂载目录

最终返回：

- 当前真实目录内容
- 系统投影出来的虚拟挂载子目录

### 8.4 冲突规则

如果真实目录中存在与挂载子目录同名的项，例如：

- 真实内容：`team/`
- 挂载：`/docs/team`

则规则为：

> **挂载节点优先，真实同名项隐藏。**

原因：

- 点击 `/docs/team` 必须稳定命中挂载源
- 不能让路径语义出现歧义

---

## 9. 模块改造方案

### 9.1 文件浏览与文件操作

这是第一优先级。

新语义：

- `list(path=/docs)`
- `download(path=/影视/阿里云/a.mp4)`
- `mkdir(path=/文档, name=新目录)` 或 `mkdir(parent_path=/文档, name=新目录)`
- `rename(path=/文档/a.txt, new_name=b.txt)`
- `move(path=/文档/a.txt, target_path=/备份)`
- `copy(path=/文档/a.txt, target_path=/影视)`
- `delete(path=/文档/a.txt)`

旧的 `source_id` 不再对前端暴露。

### 9.2 搜索

搜索入口统一改为：

- 指定 `virtual_path` 前缀搜索
- 或从 `/` 开始全局搜索

第一阶段策略：

- 对命中的真实挂载逐个 fan-out 搜索
- 结果统一 rebasing 成 `virtual_path`
- 先不引入全局文件元数据索引

### 9.3 ACL

ACL 改为绑定 `virtual_path`。

例如：

- allow user U read `/docs/team`
- deny user U write `/备份`

不再让权限模型直接绑定 `source_id + path`。

### 9.4 分享

分享对象改成：

- `target_virtual_path`

同时保存解析快照：

- `resolved_source_id`
- `resolved_inner_path`

第一阶段仅允许分享：

- 文件
- 命中真实挂载的目录

不支持分享“纯虚拟目录”。

### 9.5 上传

上传目标改成：

- `target_virtual_parent_path`

创建上传会话时保存：

- 原始虚拟路径
- 解析快照

上传判定规则明确如下：

1. 若 `target_virtual_parent_path` 能唯一解析到真实挂载源，则允许创建上传会话，并将文件写入该源对应的 `inner_path`
2. 若 `target_virtual_parent_path` 只是纯虚拟目录、没有真实挂载承接，则上传失败，返回 `NO_BACKING_STORAGE`
3. 若目标路径命中父挂载与更深层子挂载，则以最长前缀命中的子挂载为最终落点

换言之：

- “上传到虚拟路径”是允许的
- 但前提是该虚拟路径最终必须能映射到确定的真实存储源

不允许的只有一种情况：

- 上传目标停留在没有 backing storage 的中间虚拟目录上

### 9.6 下载任务

任务创建时传：

- `save_virtual_path`

创建时立即解析并固化：

- `resolved_source_id`
- `resolved_inner_save_path`

### 9.7 回收站

回收站对外显示：

- `original_virtual_path`

内部仍保存实际 source 与 inner path 以便恢复。

### 9.8 WebDAV

第一阶段不重写当前基于 `slug` 的 WebDAV。

原因：

- 当前实现仍基于 `local + slug + root_path`
- 若直接切统一树，需要自定义 VFS-backed WebDAV FS adapter

因此 WebDAV 统一树改造放在第二阶段。

---

## 10. API 设计策略

### 10.1 Source 管理 API

Source 管理仍然存在，但新增公开字段：

- `mount_path`
- `root_path`

### 10.2 新增 V2 文件系统接口

建议新增 `/api/v2/fs/*`，避免污染现有 v1 契约。

建议接口：

- `GET /api/v2/fs/list?path=/docs`
- `GET /api/v2/fs/search?path=/docs&keyword=report`
- `POST /api/v2/fs/mkdir`
- `POST /api/v2/fs/rename`
- `POST /api/v2/fs/move`
- `POST /api/v2/fs/copy`
- `DELETE /api/v2/fs`
- `GET /api/v2/fs/download?path=...`
- `POST /api/v2/fs/access-url`

### 10.3 上传、分享、ACL、任务接口

同步新增 V2 版本，统一使用 `virtual_path` 语义。

---

## 11. DTO 与响应设计

### 11.1 文件项响应

不再把 `source_id` 作为前端主定位键。

建议文件项结构至少包含：

- `name`
- `path`（virtual path）
- `parent_path`
- `is_dir`
- `entry_kind`
  - `file`
  - `dir`
  - `mount_dir`
  - `virtual_dir`
- `is_virtual`
- `is_mount_point`
- `size`
- `mime_type`
- `modified_at`
- `can_preview`
- `can_download`
- `can_delete`

可选补充：

- `mounted_source`
- `driver_type`

仅用于展示，不作为主业务键。

### 11.2 错误码

新增或明确以下 VFS 错误：

- `MOUNT_PATH_CONFLICT`
- `NO_BACKING_STORAGE`
- `VIRTUAL_DIR_READONLY`
- `VFS_PATH_INVALID`
- `RESOLVE_TARGET_NOT_FOUND`

---

## 12. 数据迁移策略

### 12.1 `storage_sources` 增加 `mount_path`

回填规则：

- 默认回填为 `"/" + webdav_slug"`

例如：

- `webdav_slug = local` -> `mount_path = /local`
- `webdav_slug = archive` -> `mount_path = /archive`

这样能避免多个 source 默认都挂到 `/` 导致冲突。

### 12.2 旧业务表迁移

第一阶段不强制一次性删掉旧字段，而是按“双写/快照”思路过渡：

- ACL：新增 `virtual_path`
- Share：新增 `target_virtual_path`
- UploadSession：新增 `target_virtual_parent_path`
- DownloadTask：新增 `save_virtual_path`
- TrashItem：新增 `original_virtual_path`

在迁移完成并切到新接口后，再逐步清理旧字段。

---

## 13. 实施顺序

### Phase 0：模型铺底

1. `storage_sources` 增加 `mount_path`
2. 回填所有已存在 source 的 `mount_path`
3. Source API / DTO / 文档补齐 `mount_path`

### Phase 1：VFS Core

1. `MountRegistry`
2. `PathResolver`
3. `VirtualDirProjector`
4. `VFSService` 中的 `list/resolve/stat/download`

交付标准：

- 能列 `/`
- 能列纯虚拟目录
- 能列真实+虚拟混合目录
- 能按 `virtual_path` 下载

### Phase 2：V2 文件接口

迁移：

- list
- search
- mkdir
- rename
- move
- copy
- delete
- access-url
- upload

前端文件管理切换到 `virtual_path`。

### Phase 3：业务模块迁移

迁移：

- ACL
- Share
- Task
- Trash

### Phase 4：WebDAV 统一树

新增统一 `/dav/*` VFS 入口，旧 `slug` WebDAV 标记废弃。

### Phase 5：清理旧模型

移除：

- 北向暴露的 `source_id`
- 旧 v1 文件接口
- source-selector 语义

---

## 14. 测试设计

### 14.1 单元测试

覆盖：

- `mount_path` 规范化
- 最长前缀匹配
- 纯虚拟目录判定
- 虚拟目录子节点投影
- 真实目录与虚拟目录合并
- 重名挂载节点覆盖规则

### 14.2 集成测试

至少覆盖：

1. 多 source 嵌套挂载
2. 列根目录 `/`
3. 列纯虚拟目录 `/docs`
4. 列真实挂载目录 `/docs/team`
5. `virtual_path` 下载 local
6. `virtual_path` 下载 s3（302 或 access-url）
7. `virtual_path` 上传到真实挂载
8. 纯虚拟目录上传失败
9. ACL 对 `virtual_path` 生效
10. 分享路径保持为 `virtual_path`

### 14.3 回归重点

尤其关注：

- 当前 local / s3 driver 不被破坏
- 旧 v1 接口在迁移窗口内仍可工作
- 任务、上传、分享在挂载变更后的行为有一致定义

---

## 15. 风险与边界

### 15.1 真实目录与挂载目录同名

策略：

- 挂载优先
- 后台创建/更新挂载时给出预警

### 15.2 纯虚拟目录上的写操作

策略：

- **并非所有虚拟路径都禁止写**
- 只禁止“没有 backing storage 的纯虚拟目录”上的写操作
- 若目标 `virtual_path` 能唯一解析到真实挂载源，则允许写，并写入该源对应的 `inner_path`
- 若目标 `virtual_path` 仅落在中间层纯虚拟目录上，则拒绝写，返回 `NO_BACKING_STORAGE`
- `VIRTUAL_DIR_READONLY` 仅保留给显式只读的虚拟节点场景；第一阶段主错误码统一使用 `NO_BACKING_STORAGE`

### 15.3 跨挂载 move / copy

策略：

- 同挂载：使用原生能力
- 跨挂载：退化为 `copy + delete`
- 明确标记为非原子操作

### 15.4 长生命周期对象的解析漂移

策略：

- 上传会话、分享、任务都保存解析快照
- 展示仍以 `virtual_path` 为主

### 15.5 搜索性能

策略：

- 第一阶段接受 fan-out
- 第二阶段再考虑异步索引

---

## 16. 非目标

本轮明确不做：

1. `.balance` 式同路径轮询
2. Alias 式真正多源 union 聚合
3. 全局异步文件索引中心
4. 自动健康路由
5. 复杂配额平衡与写冲突策略

这些能力如果未来需要，可在 VFS 基础稳定后单独设计。

---

## 17. 最终结论

Yunxia 应当正式从“按存储源组织文件”升级为“按虚拟目录树组织文件”。

本次设计的最终口径是：

1. **`virtual_path` 是唯一北向文件命名空间**
2. **`source_id` 退到内部执行层**
3. **每个 source 具备 `mount_path + root_path` 双路径模型**
4. **支持嵌套挂载、虚拟目录投影、真实/虚拟目录合并**
5. **第一期先做 OpenList 基础版，不做多源 union 与 balance**

这是对当前 Yunxia 最稳妥、也最具长期价值的架构升级路径。
