# Yunxia Backend API Contract

> 当前文档聚焦**统一存储 / VFS / 上传迁移 / 业务快照字段**。未列出的旧接口继续沿用现有 v1 语义。

## 1. 存储源双路径模型

### 1.1 字段语义

- `mount_path`：存储源挂载到统一虚拟目录树中的位置
- `root_path`：该存储源内部的起始目录

示例：

- source A：`mount_path=/docs`，`root_path=/team-a`
- 则 `/docs/readme.md` 实际落到该源内 `/team-a/readme.md`

### 1.2 约束

- `mount_path` 必须是绝对路径
- `mount_path` 会做规范化：清理重复 `/`、`.`、`..`
- `mount_path` 全局唯一
- 默认本地源挂载为 `/local`

---

## 2. 统一虚拟目录树语义

### 2.1 基本规则

- v2 文件接口统一只接收 `virtual_path`
- 路径解析采用**最长前缀匹配**
- 允许多存储源挂到同一虚拟树的不同节点
- 允许把一个存储源挂到另一个存储源映射出来的子目录下

### 2.2 虚拟目录

- 若某路径本身没有真实后端，但其下存在子挂载，则该路径是**纯虚拟目录**
- 纯虚拟目录可 `list`
- 纯虚拟目录不可写
- 对纯虚拟目录执行 `mkdir / rename / move / copy / upload init` 等写操作时，返回：
  - HTTP `409`
  - Code `NO_BACKING_STORAGE`

### 2.3 同父目录名称唯一

同一父目录下，以下名称统一参与冲突检查：

- 真实文件
- 真实目录
- 挂载点
- 由挂载投影出来的虚拟目录节点

冲突时返回：

- HTTP `409`
- Code `NAME_CONFLICT`

---

## 3. `/api/v2/fs/*` 契约

## 3.1 `GET /api/v2/fs/list`

### Query

- `path`：可选，默认为 `/`

### Response

```json
{
  "items": [
    {
      "name": "team",
      "path": "/docs/team",
      "parent_path": "/docs",
      "source_id": 2,
      "entry_kind": "directory",
      "is_virtual": true,
      "is_mount_point": true
    }
  ],
  "current_path": "/docs"
}
```

### 说明

- 返回值会合并：
  - 当前路径下的真实条目
  - 当前路径下由挂载投影出的虚拟目录
- 冲突时挂载点优先

## 3.2 `GET /api/v2/fs/search`

### Query

- `path`：可选，默认为 `/`
- `keyword`：必填
- `page`
- `page_size`

### 说明

- 搜索范围是 `path` 对应真实挂载内的目录树
- 返回结果中的 `path` / `parent_path` 已重写为统一虚拟路径

## 3.3 `GET /api/v2/fs/download`

### Query

- `path`：必填
- `disposition`：可选，默认 `attachment`
- `access_token`：当通过 access-url 临时下载时使用

### 说明

- local：直接流式返回文件
- S3：后端鉴权成功后 `302` 到 presigned URL

## 3.4 `POST /api/v2/fs/access-url`

### Request

```json
{
  "path": "/docs/readme.md",
  "purpose": "download",
  "disposition": "inline",
  "expires_in": 300
}
```

### Response

- 返回的 `url` 为统一虚拟路径下载地址：
  - `/api/v2/fs/download?...`

## 3.5 `POST /api/v2/fs/mkdir`

### Request

```json
{
  "parent_path": "/docs",
  "name": "notes"
}
```

## 3.6 `POST /api/v2/fs/rename`

### Request

```json
{
  "path": "/docs/readme.md",
  "new_name": "guide.md"
}
```

## 3.7 `POST /api/v2/fs/move`

### Request

```json
{
  "path": "/docs/readme.md",
  "target_path": "/archive"
}
```

## 3.8 `POST /api/v2/fs/copy`

### Request

```json
{
  "path": "/docs/readme.md",
  "target_path": "/archive"
}
```

## 3.9 `DELETE /api/v2/fs`

### Request

```json
{
  "path": "/docs/readme.md",
  "delete_mode": "trash"
}
```

### 写操作统一规则

- 入参一律使用 `virtual_path`
- 同挂载内：
  - 优先走底层 driver 原生语义
- 跨挂载：
  - `move = copy + delete`
  - 当前已支持 `local -> local`
  - 其他 driver 组合返回 `SOURCE_DRIVER_UNSUPPORTED`

---

## 4. v2 文件接口错误码

| HTTP | Code | 语义 |
|---|---|---|
| 400 | `PATH_INVALID` | 路径非法 |
| 403 | `ACL_DENIED` | ACL 拒绝 |
| 404 | `FILE_NOT_FOUND` | 路径不存在或未命中真实挂载 |
| 409 | `NAME_CONFLICT` | 同父目录下名称冲突 |
| 409 | `NO_BACKING_STORAGE` | 命中纯虚拟目录，无真实存储承接写入 |
| 422 | `SOURCE_DRIVER_UNSUPPORTED` | 当前驱动或跨驱动组合暂不支持 |

---

## 5. 上传初始化迁移

## 5.1 `POST /api/v1/upload/init`

当前兼容两种模式：

### 旧模式

```json
{
  "source_id": 1,
  "path": "/docs",
  "filename": "hello.txt",
  "file_size": 123,
  "file_hash": "..."
}
```

### 新模式

```json
{
  "target_virtual_parent_path": "/docs",
  "filename": "hello.txt",
  "file_size": 123,
  "file_hash": "..."
}
```

### 新模式语义

- 服务端先用 `target_virtual_parent_path + filename` 做 VFS 可写落点解析
- 解析成功后，把快照写入上传会话：
  - `target_virtual_parent_path`
  - `resolved_source_id`
  - `resolved_inner_parent_path`
- 分片上传与完成上传阶段继续复用既有 local / s3 协议

### 响应中的 upload 新增字段

```json
{
  "upload": {
    "upload_id": "upl_xxx",
    "source_id": 2,
    "path": "/",
    "target_virtual_parent_path": "/docs",
    "resolved_source_id": 2,
    "resolved_inner_parent_path": "/"
  }
}
```

### 上传相关错误补充

| HTTP | Code | 语义 |
|---|---|---|
| 400 | `PATH_INVALID` | `source_id/path` 与 `target_virtual_parent_path` 都无法形成合法目标 |
| 409 | `FILE_ALREADY_EXISTS` | 目标已存在 |
| 409 | `NAME_CONFLICT` | 与挂载点 / 虚拟节点冲突 |
| 409 | `NO_BACKING_STORAGE` | 目标父目录是纯虚拟目录 |

---

## 6. 业务模块快照字段

## 6.1 ACL Rule

当前 ACL 规则在保留 `source_id + path` 的同时，新增：

- `virtual_path`

当前行为：

- create / update 时自动按 `mount_path + path` 双写 `virtual_path`
- runtime authorizer 优先按 `virtual_path` 判定
- 历史规则若没有 `virtual_path`，回退到旧 `path`

## 6.2 Share

`ShareView` 新增：

- `target_virtual_path`
- `resolved_source_id`
- `resolved_inner_path`

当前 create 时会自动写入上述字段。

## 6.3 Task

`DownloadTaskView` 新增：

- `save_virtual_path`
- `resolved_source_id`
- `resolved_inner_save_path`

当前 create 时会自动写入上述字段。

## 6.4 Trash

`TrashItemView` 新增：

- `original_virtual_path`

`TrashRestoreResponse` 新增：

- `restored_virtual_path`

删除到回收站时自动写入 `original_virtual_path`。

---

## 7. 当前实现边界

- v2 统一虚拟目录能力已覆盖：
  - list / search / download / access-url
  - mkdir / rename / move / copy / delete
- upload init 已支持 `target_virtual_parent_path`
- ACL / Share / Task / Trash 已补最小虚拟路径快照
- 目前 northbound 业务接口仍保留大量 v1 `source_id + path` 兼容字段
- `.balance`、Alias union、统一 WebDAV 尚未纳入当前阶段
