# PikPak 接入完整解析文档

> 生成日期：2026-05-04  
> 适用项目：OpenList 当前目录项目  
> 适用驱动：`PikPak`  
> 目标：读者不需要打开 OpenList 源码，也能理解如何在 OpenList 中接入 PikPak、如何调用 OpenList 统一接口、以及 OpenList 底层如何调用 PikPak。

---

## 0. 读者定位与使用边界

本文面向三类读者：

1. **运维 / 使用者**：想在 OpenList 后台把 PikPak 挂载成一个目录，例如 `/pikpak` 或 `/影视/PikPak`。
2. **外部系统接入方**：想通过 OpenList API 或 WebDAV 操作 PikPak，而不是直接调用 PikPak 官方/私有接口。
3. **二次开发者**：想理解 OpenList 的 PikPak 驱动为什么这样设计，未来要改驱动或排查问题时知道完整链路。

重要边界：

- 如果你的目标只是“让 OpenList 接上 PikPak”，**不需要自己实现 PikPak 登录、captcha、OSS 上传、token 刷新**；这些都由 `PikPak` 驱动完成。
- 如果你的目标是“绕过 OpenList 直接调用 PikPak 原始 API”，本文也给出底层接口结构，但更推荐以 OpenList 统一 API 作为稳定接入层。
- OpenList 对外 API 的 HTTP 状态通常是 `200`，真正成功/失败看 JSON 中的 `code`。

OpenList 统一响应包：

```json
{
  "code": 200,
  "message": "success",
  "data": {}
}
```

错误响应示例：

```json
{
  "code": 500,
  "message": "failed init storage but storage is already created: failed init storage: ...",
  "data": {
    "id": 12
  }
}
```

---

## 1. 一句话结论

OpenList 中的 `PikPak` 驱动不是把 PikPak 简单反向代理出来，而是把 PikPak 的账号登录、文件 ID、目录列表、下载直链、OSS 上传、离线下载任务、容量查询等能力，适配成 OpenList 统一的 `storage`。

整体链路如下：

```text
外部调用者
  ├─ OpenList Web UI
  ├─ OpenList HTTP API
  └─ OpenList WebDAV
        │
        ▼
server/handles
  处理认证、权限、请求参数、统一响应
        │
        ▼
internal/fs
  处理虚拟目录、用户路径、任务化上传/移动/复制
        │
        ▼
internal/op
  选择 storage，路径转 actualPath，调用驱动接口，维护缓存
        │
        ▼
drivers/pikpak.PikPak
  登录/刷新 token，刷新 captcha，调用 PikPak API，上传到 OSS
        │
        ▼
PikPak HTTP API + PikPak 返回的 OSS 临时上传参数
```

所以，正常接入时你只需要关心：

- 在 OpenList 创建一个 `driver = "PikPak"` 的 storage；
- 设置 PikPak 账号密码等 `addition` 参数；
- 之后通过 `/api/fs/*`、`/d/*`、`/dav/*` 操作挂载目录。

---

## 2. OpenList 中 PikPak 接入涉及的核心概念

### 2.1 storage

OpenList 把每个后端存储源抽象为一个 `storage`。一个 PikPak 账号挂载一次，就是一个 storage。

storage 的关键字段：

| 字段 | 示例 | 说明 |
| --- | --- | --- |
| `driver` | `PikPak` | 使用哪个驱动。接入 PikPak 必须是 `PikPak`，大小写要一致。 |
| `mount_path` | `/pikpak` | 在 OpenList 虚拟目录树中显示的位置。 |
| `addition` | JSON 字符串 | PikPak 私有参数，例如账号、密码、refresh token。 |
| `cache_expiration` | `30` | 目录缓存时间，单位分钟。 |
| `webdav_policy` | `302_redirect` / `native_proxy` | WebDAV 下载时的处理策略。 |
| `order_by` / `order_direction` | `name` / `asc` | PikPak 驱动启用本地排序，可按这些字段排序。 |

### 2.2 mount path 与 actual path

用户访问的是 OpenList 路径，例如：

```text
/pikpak/movie/a.mp4
```

OpenList 会用最长前缀匹配找到挂载点 `/pikpak`，然后把挂载点后面的路径转换成驱动内部路径：

```text
OpenList 用户路径：/pikpak/movie/a.mp4
命中的 mount_path：/pikpak
传给 PikPak 驱动的 actualPath：/movie/a.mp4
```

PikPak 自身接口主要是 **ID 驱动** 的：目录列表、移动、复制、删除都依赖 file id / folder id。因此 OpenList 会先通过路径找到对象，再用对象的 `id` 调 PikPak API。

### 2.3 `root_folder_id`

`root_folder_id` 是 PikPak 内部目录 ID。

- 留空：挂载 PikPak 账号默认根目录。
- 填某个 PikPak 文件夹 ID：只把该子目录作为 OpenList 挂载根。

示例：

```json
{
  "root_folder_id": "",
  "username": "user@example.com",
  "password": "password",
  "platform": "web",
  "refresh_token": "",
  "captcha_token": "",
  "device_id": "",
  "disable_media_link": true
}
```

OpenList 的根对象会使用 `root_folder_id` 作为根目录对象 ID。之后列根目录时，驱动会把这个 ID 作为 `parent_id` 调 PikPak 的 `/drive/v1/files`。

### 2.4 `addition` 为什么是字符串

OpenList 数据库中所有驱动都共用同一个 `model.Storage` 表结构，不可能为每个网盘单独建字段。因此：

- 通用字段放在 `model.Storage` 顶层，例如 `mount_path`、`driver`、`cache_expiration`。
- 驱动私有字段统一序列化成 JSON 字符串放在 `addition`。

通过 HTTP API 创建 storage 时，`addition` 必须是 **字符串形式的 JSON**，不是嵌套对象：

```json
{
  "mount_path": "/pikpak",
  "driver": "PikPak",
  "addition": "{\"username\":\"user@example.com\",\"password\":\"password\"}"
}
```

如果写成下面这样，后端绑定结构时类型不匹配：

```json
{
  "addition": {
    "username": "user@example.com"
  }
}
```

---

## 3. PikPak 挂载参数完整说明

### 3.1 通用 storage 参数

这些字段和所有 OpenList 存储源通用。

| 字段 | 类型 | 必填 | 推荐值 | 说明 |
| --- | --- | --- | --- | --- |
| `mount_path` | string | 是 | `/pikpak` | OpenList 挂载路径，必须唯一。会被规范化为以 `/` 开头的干净路径。 |
| `driver` | string | 是 | `PikPak` | 驱动名，接入 PikPak 必须固定为 `PikPak`。 |
| `order` | number | 否 | `0` | 多个挂载在同一级目录展示时的排序值。 |
| `remark` | string | 否 | `""` | 备注。部分引用型驱动会用特殊 remark，PikPak 一般不需要。 |
| `cache_expiration` | number | 是 | `30` | 目录缓存过期时间，单位分钟。 |
| `custom_cache_policies` | text | 否 | `""` | 自定义目录缓存规则，格式为每行 `glob:分钟`。 |
| `web_proxy` | bool | 否 | `false` | Web 页面下载是否走 OpenList 代理。 |
| `webdav_policy` | select | 是 | `302_redirect` 或 `native_proxy` | WebDAV 下载策略，见 WebDAV 章节。 |
| `proxy_range` | bool | 否 | `false` | 代理下载时是否由 OpenList 处理 Range。PikPak 通常不需要主动开启。 |
| `down_proxy_url` | text | 否 | `""` | 自定义下载代理地址。 |
| `disable_proxy_sign` | bool | 否 | `false` | 使用 `down_proxy_url` 时是否禁用签名。 |
| `order_by` | select | 否 | `name` | PikPak 驱动启用 `LocalSort`，可选 `name`、`size`、`modified`。 |
| `order_direction` | select | 否 | `asc` | 可选 `asc`、`desc`。 |
| `extract_folder` | select | 否 | `""` | 文件夹前置或后置，可选 `front`、`back`。 |
| `disable_index` | bool | 是 | `false` | 是否禁用搜索索引。 |
| `enable_sign` | bool | 是 | `false` | 是否对下载链接启用签名。 |
| `disabled` | bool | 否 | `false` | 是否禁用该 storage。创建时通常不填。 |

### 3.2 PikPak 私有参数 `addition`

| 字段 | 类型 | 必填 | 默认值 | 详细说明 |
| --- | --- | --- | --- | --- |
| `root_folder_id` | string | 否 | `""` | 挂载为根的 PikPak 文件夹 ID。留空表示账号根目录。 |
| `username` | string | 是 | `""` | PikPak 登录账号。可以是邮箱、手机号或用户名。 |
| `password` | string | 是 | `""` | PikPak 登录密码。用于首次登录或 refresh token 失效时重新登录。 |
| `platform` | string/select | 是 | `web` | 可选 `android`、`web`、`pc`。决定 client id、UA、captcha 签名参数。推荐先用 `web`。 |
| `refresh_token` | string | 表单标记必填 | `""` | 已有 refresh token 时优先用它换 access token；为空时驱动会用用户名密码登录并保存新 refresh token。 |
| `captcha_token` | string | 否 | `""` | PikPak 风控 captcha token。通常留空，由驱动自动获取；遇到人工验证时需要填入验证后的 token。 |
| `device_id` | string | 否 | `""` | 设备 ID。留空时 OpenList 用 `MD5(username + password)` 生成并保存。 |
| `disable_media_link` | bool | 否 | `true` | 是否禁用媒体转码链接。`true` 表示下载优先使用原始文件链接；`false` 时视频可能使用媒体缓存/转码链接。 |

### 3.3 推荐最小配置

最小可用 `addition`：

```json
{
  "root_folder_id": "",
  "username": "<pikpak_account>",
  "password": "<pikpak_password>",
  "platform": "web",
  "refresh_token": "",
  "captcha_token": "",
  "device_id": "",
  "disable_media_link": true
}
```

说明：

- `refresh_token` 可以先留空。初始化成功后，OpenList 会把新获取的 refresh token 写回 storage。
- `device_id` 可以先留空。初始化成功后，OpenList 会写回生成的 device id。
- `captcha_token` 通常留空。若返回 `need verify: <a ...>Click Here</a>`，需要完成验证后再更新。

### 3.4 `platform` 怎么选

| 值 | 场景 | 特点 |
| --- | --- | --- |
| `web` | 推荐默认 | UA 类似桌面 Chrome，接入成本低，适合大多数部署。 |
| `android` | Web 风控不稳定时尝试 | 使用 Android 风格 UA 和签名参数；上传时会修正 OSS endpoint。 |
| `pc` | 兼容 PC 客户端行为 | UA 类似 PikPak Electron 客户端。 |

平台不是展示字段，它会影响：

- 登录请求中的 `client_id`、`client_secret`；
- captcha 刷新时的签名算法；
- 请求 `User-Agent`；
- Android 上传时 OSS endpoint 的兼容处理。

---

## 4. 后台 UI 接入步骤

如果通过 OpenList 后台接入，按下面步骤即可：

1. 登录 OpenList 管理后台。
2. 进入 `存储` / `Storages`。
3. 点击新增存储。
4. 驱动选择 `PikPak`。
5. 填写通用字段：
   - 挂载路径：例如 `/pikpak` 或 `/影视/PikPak`
   - 缓存时间：例如 `30`
   - WebDAV 策略：不确定时可先用默认；WebDAV 客户端兼容性差时用 `native_proxy`
6. 填写 PikPak 私有字段：
   - `username`：PikPak 账号
   - `password`：PikPak 密码
   - `platform`：推荐 `web`
   - `refresh_token`：首次接入可留空；如果前端强制必填，可填已有 token，或改用 API 创建
   - `captcha_token`：通常留空
   - `device_id`：通常留空
   - `disable_media_link`：推荐 `true`
7. 保存。
8. 返回存储列表查看状态：
   - `work`：初始化成功。
   - `failed init storage ...`：初始化失败，但记录可能已经创建，需要编辑该 storage 修正参数。

保存之后 OpenList 内部会做这些事：

```text
用户保存 storage
  -> 写入数据库 model.Storage
  -> 根据 driver="PikPak" 创建 PikPak 驱动实例
  -> 反序列化 addition 到 PikPak Addition
  -> 调用 PikPak.Init
      -> 选择 platform 对应的 client 参数与 UA
      -> 生成或读取 device_id
      -> 有 refresh_token：refreshToken()
      -> 无 refresh_token：login()
      -> 登录后刷新 drive API 的 captcha_token
      -> 保存 refresh_token / device_id / captcha_token
  -> 把驱动实例放入内存 storagesMap
  -> 状态置为 work 或错误信息
```

---

## 5. HTTP API 接入步骤

本节给出从零开始的完整 API 示例。示例中的 `$OPENLIST` 是 OpenList 地址，`$TOKEN` 是 OpenList 登录 token。

### 5.1 登录 OpenList 获取 token

```bash
export OPENLIST="http://127.0.0.1:5244"

TOKEN=$(curl -s -X POST "$OPENLIST/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<openlist_password>"}' \
  | jq -r '.data.token')
```

注意：

- `/api/auth/login` 接收明文密码，后端会进行静态哈希。
- `/api/auth/login/hash` 接收已经静态哈希后的密码。
- 调 OpenList API 时，`Authorization` 头直接放 token，不需要 `Bearer` 前缀。

```http
Authorization: <openlist_token>
```

### 5.2 查看 PikPak 驱动表单元数据

```bash
curl "$OPENLIST/api/admin/driver/info?driver=PikPak" \
  -H "Authorization: $TOKEN"
```

你会看到三类信息：

- `common`：通用 storage 字段。
- `additional`：PikPak 私有字段。
- `config`：驱动配置，例如驱动名、是否本地排序等。

### 5.3 创建 PikPak storage

完整请求：

```bash
curl -X POST "$OPENLIST/api/admin/storage/create" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "mount_path": "/pikpak",
    "driver": "PikPak",
    "order": 0,
    "cache_expiration": 30,
    "custom_cache_policies": "",
    "web_proxy": false,
    "webdav_policy": "native_proxy",
    "proxy_range": false,
    "down_proxy_url": "",
    "disable_proxy_sign": false,
    "order_by": "name",
    "order_direction": "asc",
    "extract_folder": "",
    "disable_index": false,
    "enable_sign": false,
    "addition": "{\"root_folder_id\":\"\",\"username\":\"<pikpak_account>\",\"password\":\"<pikpak_password>\",\"platform\":\"web\",\"refresh_token\":\"\",\"captcha_token\":\"\",\"device_id\":\"\",\"disable_media_link\":true}"
  }'
```

成功响应：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 12
  }
}
```

初始化失败但记录已创建的响应示例：

```json
{
  "code": 500,
  "message": "failed init storage but storage is already created: failed init storage: username or password is empty",
  "data": {
    "id": 12
  }
}
```

这种情况说明数据库中已有该 storage，但驱动状态不是 `work`。应使用返回的 `id` 查询并更新配置。

### 5.4 查询 storage

```bash
curl "$OPENLIST/api/admin/storage/get?id=12" \
  -H "Authorization: $TOKEN"
```

关注：

- `status` 是否为 `work`。
- `addition` 是否被写回了新的 `refresh_token`、`device_id`、`captcha_token`。

### 5.5 更新 storage

更新时必须提交完整 storage 对象，不能只提交局部字段；并且不能修改 `driver`。

典型流程：

```bash
curl "$OPENLIST/api/admin/storage/get?id=12" \
  -H "Authorization: $TOKEN" > storage.json

# 修改 storage.json 中的 addition、mount_path 等字段后提交
curl -X POST "$OPENLIST/api/admin/storage/update" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @storage.json
```

---

## 6. 挂载后的路径解析与虚拟目录行为

### 6.1 普通挂载

挂载：

```text
mount_path = /pikpak
```

访问：

```text
/pikpak
/pikpak/a.txt
/pikpak/movie/b.mp4
```

解析：

| 用户路径 | 命中 storage | actualPath |
| --- | --- | --- |
| `/pikpak` | `/pikpak` | `/` |
| `/pikpak/a.txt` | `/pikpak` | `/a.txt` |
| `/pikpak/movie/b.mp4` | `/pikpak` | `/movie/b.mp4` |

### 6.2 深层挂载与纯虚拟目录

如果配置：

```text
/影视/PikPak
/影视/本地
```

访问：

```text
/影视
```

此时 `/影视` 没有真实 storage 直接挂载，但 OpenList 会根据下面的挂载路径动态生成子目录：

```text
/影视/PikPak
/影视/本地
```

因此 `/影视` 是 **纯虚拟目录**。

纯虚拟目录的特征：

- 可以列目录，显示下面的挂载点。
- 不能直接写入文件。
- 不能直接作为上传目标。
- WebDAV 下对纯虚拟目录 `PROPFIND` 可以成功，但 `PUT /dav/影视/a.txt` 没有明确目标 storage，会失败。

正确写入方式：

```text
上传到 /影视/PikPak/a.txt  -> 写入 PikPak
上传到 /影视/本地/a.txt    -> 写入本地存储
上传到 /影视/a.txt         -> 失败，因为 /影视 是纯虚拟目录，OpenList 无法判断写到哪个 storage
```

### 6.3 同名虚拟目录与真实 storage 的合并

如果同时存在：

```text
/影视
/影视/PikPak
```

那么 `/影视` 本身是一个真实 storage，同时列表里会合并虚拟子目录 `PikPak`。

这种情况下：

- 上传 `/影视/a.txt`：写入 `/影视` 对应的真实 storage。
- 上传 `/影视/PikPak/a.txt`：写入 `/影视/PikPak` 对应的 PikPak storage。

这就是 OpenList 避免歧义的规则：**写操作必须解析到一个明确的真实 storage**。

---

## 7. PikPak 驱动初始化链路

### 7.1 初始化流程图

```text
PikPak.Init
  │
  ├─ 初始化 Common 运行态
  │   ├─ resty HTTP client
  │   ├─ CaptchaToken
  │   ├─ UserID
  │   ├─ DeviceID = MD5(username + password)
  │   └─ RefreshCTokenCk：captcha 更新后保存 storage
  │
  ├─ 根据 platform 选择 client 参数
  │   ├─ android：Android client id / version / package / UA / algorithms
  │   ├─ web：Web client id / version / package / UA / algorithms
  │   └─ pc：PC client id / version / package / UA / algorithms
  │
  ├─ 处理 captcha_token
  │   └─ 如果用户填了 captcha_token 且没有 refresh_token，则先使用它
  │
  ├─ 处理 device_id
  │   ├─ 用户填了 device_id：使用用户值
  │   └─ 用户没填：使用 MD5(username + password)，并写回 storage
  │
  ├─ 获取 access token
  │   ├─ refresh_token 非空：refreshToken(refresh_token)
  │   └─ refresh_token 为空：login(username, password)
  │
  ├─ 登录后刷新 drive API captcha
  │   └─ action = GET:/drive/v1/files
  │
  ├─ Android 平台补充 userID 后重建 UA
  │
  └─ 保存有效 refresh_token，状态变为 work
```

### 7.2 初始化为什么需要 captcha

PikPak 对登录和 Drive API 请求都可能做风控校验。OpenList 驱动把它抽象成两个阶段：

1. **登录前 captcha**：用于 `POST /v1/auth/signin`。
2. **登录后 captcha**：用于 `GET /drive/v1/files` 等 Drive API。

如果 PikPak 返回需要人工验证的 URL，OpenList 会把错误显示成类似：

```text
need verify: <a target="_blank" href="https://...">Click Here</a>
```

处理方式：

1. 打开提示链接完成验证。
2. 获取或等待新的 captcha token。
3. 回到 storage 配置中填写 `captcha_token`。
4. 保存并重新初始化。

### 7.3 token 保存策略

初始化成功后，驱动会把运行中拿到的有效值写回 storage：

```json
{
  "refresh_token": "<new_refresh_token>",
  "captcha_token": "<new_captcha_token>",
  "device_id": "<stable_device_id>"
}
```

这样 OpenList 重启后可以优先用 `refresh_token` 恢复登录态，减少账号密码登录次数。

---

## 8. PikPak 原始接口与 OpenList 驱动封装

本节用于理解底层实现。实际业务接入建议优先调用 OpenList API。

### 8.1 原始接口域名

| 用途 | 域名 |
| --- | --- |
| 登录、刷新 token、captcha | `https://user.mypikpak.net` |
| 文件、目录、离线任务 | `https://api-drive.mypikpak.net` |
| 容量查询 | `https://api-drive.mypikpak.com` |
| 实体上传 | PikPak 返回的 OSS `endpoint`、`bucket`、`key` |

### 8.2 通用请求头

驱动内部所有 PikPak API 请求都会尽量带上：

```http
Authorization: Bearer <access_token>
User-Agent: <platform_user_agent>
X-Device-ID: <device_id>
X-Captcha-Token: <captcha_token>
```

说明：

- `Authorization` 只有在 `access_token` 非空时才加。
- `X-Captcha-Token` 过期时会自动刷新。
- `User-Agent` 由 `platform` 决定。

### 8.3 登录前 captcha 初始化

接口：

```http
POST https://user.mypikpak.net/v1/shield/captcha/init?client_id=<client_id>
```

请求体结构：

```json
{
  "action": "POST:/v1/auth/signin",
  "captcha_token": "",
  "client_id": "<client_id>",
  "device_id": "<device_id>",
  "meta": {
    "email": "user@example.com"
  },
  "redirect_uri": "xlaccsdk01://xbase.cloud/callback?state=harbor"
}
```

`meta` 字段选择规则：

| username 形式 | meta 字段 |
| --- | --- |
| 符合邮箱格式 | `email` |
| 长度 11 到 18 | `phone_number` |
| 其他 | `username` |

成功响应核心字段：

```json
{
  "captcha_token": "<captcha_token>",
  "expires_in": 300,
  "url": ""
}
```

如果 `url` 非空，表示需要人工验证。

### 8.4 用户名密码登录

接口：

```http
POST https://user.mypikpak.net/v1/auth/signin?client_id=<client_id>
```

请求体：

```json
{
  "captcha_token": "<captcha_token>",
  "client_id": "<client_id>",
  "client_secret": "<client_secret>",
  "username": "<pikpak_account>",
  "password": "<pikpak_password>"
}
```

驱动读取响应中的：

| 响应字段 | 用途 |
| --- | --- |
| `access_token` | 后续 Drive API 的 Bearer token。 |
| `refresh_token` | 之后刷新 access token，并写回 `addition.refresh_token`。 |
| `sub` | PikPak 用户 ID，用于登录后 captcha 签名元数据。 |

### 8.5 refresh token 换 access token

接口：

```http
POST https://user.mypikpak.net/v1/auth/token?client_id=<client_id>
```

请求体：

```json
{
  "client_id": "<client_id>",
  "client_secret": "<client_secret>",
  "grant_type": "refresh_token",
  "refresh_token": "<refresh_token>"
}
```

处理规则：

- 成功：更新 `access_token`、`refresh_token`、`user_id`，并写回 storage。
- 返回错误码 `4126`：
  - 如果没有用户名密码：报 `refresh_token invalid, please re-provide refresh_token`。
  - 如果有用户名密码：回退到用户名密码登录。

### 8.6 登录后 captcha 刷新

接口：

```http
POST https://user.mypikpak.net/v1/shield/captcha/init?client_id=<client_id>
```

请求体结构：

```json
{
  "action": "GET:/drive/v1/files",
  "captcha_token": "<old_captcha_token>",
  "client_id": "<client_id>",
  "device_id": "<device_id>",
  "meta": {
    "client_version": "<client_version>",
    "package_name": "<package_name>",
    "user_id": "<pikpak_user_id>",
    "timestamp": "<unix_milliseconds>",
    "captcha_sign": "1.<md5_chain>"
  },
  "redirect_uri": "xlaccsdk01://xbase.cloud/callback?state=harbor"
}
```

`captcha_sign` 的生成逻辑：

```text
timestamp = 当前毫秒时间戳
str = client_id + client_version + package_name + device_id + timestamp
for algorithm in platform_algorithms:
    str = MD5(str + algorithm)
captcha_sign = "1." + str
```

你通过 OpenList 接入时不需要自己实现该签名；驱动会按 `platform` 自动完成。

### 8.7 request 自动重试规则

PikPak 驱动统一请求封装会检查错误响应中的 `error_code`：

| `error_code` | 语义 | OpenList 驱动处理 |
| --- | --- | --- |
| `0` | 成功或无错误 | 返回响应体。 |
| `4122` / `4121` / `16` | access token 过期或无效 | 调 `refreshToken` 后重试原请求。 |
| `9` | captcha token 过期 | 刷新 captcha token 后重试原请求。 |
| `10` | 操作频繁 | 直接返回 `error_description`。 |
| 其他 | 未分类错误 | 返回 `ErrorCode / Error / ErrorDescription`。 |

这也是推荐使用 OpenList 驱动的原因：外部接入方不用重复实现 token/captcha 重试。

---

## 9. OpenList 统一文件 API：PikPak 挂载后的调用方式

下面所有示例默认已经创建：

```text
mount_path = /pikpak
driver = PikPak
```

### 9.1 列目录

请求：

```bash
curl -X POST "$OPENLIST/api/fs/list" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/pikpak",
    "page": 1,
    "per_page": 100,
    "refresh": false
  }'
```

响应示例：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "content": [
      {
        "name": "movie.mp4",
        "size": 104857600,
        "is_dir": false,
        "modified": "2026-05-04T10:00:00Z",
        "created": "2026-05-04T09:00:00Z",
        "sign": "",
        "thumb": "https://...",
        "type": 2,
        "hashinfo": "gcid:...",
        "hash_info": {}
      }
    ],
    "total": 1,
    "readme": "",
    "header": "",
    "write": true,
    "write_content_bypass": false,
    "provider": "unknown"
  }
}
```

底层链路：

```text
POST /api/fs/list
  -> FsList
  -> fs.List("/pikpak")
  -> 获取虚拟目录项
  -> op.GetStorageAndActualPath("/pikpak")
      mount_path = /pikpak
      actualPath = /
  -> op.List(storage, "/")
  -> op.Get(storage, "/") 得到根目录对象，ID = root_folder_id
  -> PikPak.List(rootDir)
  -> PikPak.getFiles(rootDir.ID)
  -> GET https://api-drive.mypikpak.net/drive/v1/files
```

PikPak 原始列目录请求：

```http
GET https://api-drive.mypikpak.net/drive/v1/files?parent_id=<folder_id>&thumbnail_size=SIZE_LARGE&with_audit=true&limit=100&page_token=<token>&filters=...
```

关键 query：

```text
parent_id      = 当前目录 ID
thumbnail_size = SIZE_LARGE
with_audit     = true
limit          = 100
page_token     = 第一页为空，后续使用 next_page_token
filters        = {"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}
```

字段映射：

| PikPak `File` 字段 | OpenList 对象字段 |
| --- | --- |
| `id` | `Object.ID` |
| `name` | `Object.Name` |
| `size` | `Object.Size`，从字符串转 int64 |
| `created_time` | `Object.Ctime` |
| `modified_time` | `Object.Modified` |
| `kind == "drive#folder"` | `Object.IsFolder = true` |
| `hash` | `HashInfo(GCID)` |
| `thumbnail_link` | `Thumbnail` |

### 9.2 获取对象信息

请求：

```bash
curl -X POST "$OPENLIST/api/fs/get" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"/pikpak/movie.mp4"}'
```

响应示例：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "name": "movie.mp4",
    "size": 104857600,
    "is_dir": false,
    "modified": "2026-05-04T10:00:00Z",
    "created": "2026-05-04T09:00:00Z",
    "sign": "",
    "thumb": "https://...",
    "type": 2,
    "hashinfo": "gcid:...",
    "raw_url": "https://...",
    "readme": "",
    "header": "",
    "provider": "PikPak",
    "related": []
  }
}
```

实现要点：

- PikPak 驱动没有单独实现按路径 `Get`。
- OpenList 会通过父目录 `List` 找到 `movie.mp4`。
- 对文件对象，`/api/fs/get` 可能额外调用 `fs.Link` 获取 `raw_url`。

### 9.3 获取下载链接

管理员接口：

```bash
curl -X POST "$OPENLIST/api/fs/link" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"/pikpak/movie.mp4"}'
```

响应示例：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "url": "https://..."
  }
}
```

普通下载入口：

```text
GET /d/pikpak/movie.mp4
GET /p/pikpak/movie.mp4
```

如果启用了签名，路径后需要加：

```text
?sign=<sign>
```

PikPak 原始下载详情接口：

```http
GET https://api-drive.mypikpak.net/drive/v1/files/{file_id}?_magic=2021&usage=FETCH&thumbnail_size=SIZE_LARGE
```

如果 `disable_media_link = false`，驱动会把 `usage` 改为 `CACHE`，并优先尝试媒体链接：

```text
默认下载链接：resp.web_content_link
媒体链接：resp.medias[0].link.url
```

下载链接选择规则：

| 配置 | 行为 |
| --- | --- |
| `disable_media_link = true` | 使用 `web_content_link`，偏向原始文件。 |
| `disable_media_link = false` | 如果存在 `medias[0].link.url`，优先使用媒体缓存/转码链接。 |

### 9.4 新建目录

请求：

```bash
curl -X POST "$OPENLIST/api/fs/mkdir" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path":"/pikpak/new-folder"}'
```

OpenList 行为：

```text
fs.MakeDir("/pikpak/new-folder")
  -> 命中 /pikpak storage
  -> actualPath = /new-folder
  -> op.MakeDir 会递归确保父目录存在
  -> 获取父目录对象 ID
  -> PikPak.MakeDir(parentDir, "new-folder")
```

PikPak 原始请求：

```http
POST https://api-drive.mypikpak.net/drive/v1/files
```

请求体：

```json
{
  "kind": "drive#folder",
  "parent_id": "<parent_folder_id>",
  "name": "new-folder"
}
```

### 9.5 流式上传文件

请求：

```bash
curl -X PUT "$OPENLIST/api/fs/put" \
  -H "Authorization: $TOKEN" \
  -H "File-Path: /pikpak/upload.bin" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @upload.bin
```

常用请求头：

| Header | 示例 | 说明 |
| --- | --- | --- |
| `File-Path` | `/pikpak/upload.bin` | 必填。目标完整路径，需要 URL 解码后是 OpenList 路径。 |
| `Content-Type` | `application/octet-stream` | 可选。不填时 OpenList 根据文件名推断。 |
| `Content-Length` | `123456` | HTTP 自动带。未知大小时可用 `X-File-Size`。 |
| `X-File-Size` | `123456` | 当 `Content-Length` 不可用时提供文件大小。 |
| `X-File-Md5` | `<md5>` | 可选，写入 OpenList hash 信息。 |
| `X-File-Sha1` | `<sha1>` | 可选。 |
| `X-File-Sha256` | `<sha256>` | 可选。 |
| `Last-Modified` | `1770000000000` | 可选，毫秒时间戳。 |
| `Overwrite` | `false` | 可选。等于 `false` 时如果文件存在直接报错；默认覆盖。 |
| `As-Task` | `true` | 可选。等于 `true` 时进入 OpenList 上传任务队列。 |

任务化上传：

```bash
curl -X PUT "$OPENLIST/api/fs/put" \
  -H "Authorization: $TOKEN" \
  -H "File-Path: /pikpak/upload.bin" \
  -H "As-Task: true" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @upload.bin
```

任务化响应：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "task": {
      "id": "<task_id>",
      "name": "upload upload.bin to [/pikpak](/)",
      "state": 0,
      "status": "uploading",
      "progress": 0
    }
  }
}
```

查询上传任务：

```bash
curl "$OPENLIST/api/task/upload/undone" \
  -H "Authorization: $TOKEN"

curl -X POST "$OPENLIST/api/task/upload/info?tid=<task_id>" \
  -H "Authorization: $TOKEN"
```

### 9.6 PikPak 上传底层完整流程

OpenList 到 PikPak 的上传流程：

```text
PUT /api/fs/put
  -> FsStream
  -> 解析 File-Path
  -> 检查用户写权限
  -> 构造 FileStream
  -> fs.PutDirectly 或 fs.PutAsTask
  -> op.Put(storage=PikPak, dstDirActualPath, file)
      ├─ 如果目标文件存在且大小为 0：先删除旧文件
      ├─ 如果目标目录不存在：递归创建目录
      ├─ 获取目标父目录对象 ID
      └─ PikPak.Put(parentDir, stream)
          ├─ 获取或计算 GCID
          ├─ POST /drive/v1/files 创建上传任务
          ├─ 如果 resumable == nil：秒传成功
          ├─ 如果返回 OSS 参数：
          │   ├─ 文件 <= 10MB：OSS PutObject
          │   └─ 文件 > 10MB：OSS Multipart Upload
          └─ 更新 OpenList 缓存和任务进度
```

为什么要先算 GCID：

- PikPak 上传任务需要提交 `hash`。
- OpenList 使用的 hash 类型是 `GCID`。
- 如果上传流里没有现成 GCID，OpenList 会先缓存完整文件并计算 GCID。
- 这意味着大文件上传到 PikPak 前，OpenList 可能先产生本地临时缓存。

PikPak 创建上传任务请求：

```http
POST https://api-drive.mypikpak.net/drive/v1/files
```

请求体：

```json
{
  "kind": "drive#file",
  "name": "upload.bin",
  "size": 123456,
  "hash": "<UPPERCASE_GCID>",
  "upload_type": "UPLOAD_TYPE_RESUMABLE",
  "objProvider": {
    "provider": "UPLOAD_TYPE_UNKNOWN"
  },
  "parent_id": "<dst_folder_id>",
  "folder_type": "NORMAL"
}
```

如果秒传成功，PikPak 响应中 `resumable` 为 `null` 或不存在，OpenList 直接结束。

如果需要上传实体，PikPak 会返回：

```json
{
  "upload_type": "UPLOAD_TYPE_RESUMABLE",
  "resumable": {
    "kind": "...",
    "provider": "...",
    "params": {
      "access_key_id": "<oss_access_key_id>",
      "access_key_secret": "<oss_access_key_secret>",
      "bucket": "<bucket>",
      "endpoint": "<endpoint>",
      "expiration": "2026-05-04T12:00:00Z",
      "key": "<object_key>",
      "security_token": "<security_token>"
    }
  },
  "file": {}
}
```

OSS 上传请求会带：

```http
X-OSS-Security-Token: <security_token>
User-Agent: aliyun-sdk-android/2.9.13(Linux/Android 14/M2004j7ac;UKQ1.231108.001)
```

分片策略：

| 文件大小 | 上传方式 |
| --- | --- |
| `<= 10MB` | 单次 OSS `PutObject`。 |
| `> 10MB` | OSS multipart。默认 10 个 goroutine 上传分片。 |

分片数量规则：

- 小于 `1GB`：尝试分成 `100` 片。
- 小于 `2GB`：尝试分成 `200` 片。
- 以此类推，小于 `9GB`：尝试分成 `900` 片。
- 大于 `9GB`：分成 `1000` 片。
- 如果单片小于 `1MB`，改为按 `1MB` 分片。

### 9.7 表单上传

OpenList 还支持 multipart form 上传：

```bash
curl -X PUT "$OPENLIST/api/fs/form" \
  -H "Authorization: $TOKEN" \
  -H "File-Path: /pikpak/form-upload.txt" \
  -F "file=@hello.txt"
```

底层最终也会走 `fs.PutDirectly` / `PikPak.Put`，区别只是入口读取上传内容的方式不同。

### 9.8 重命名

请求：

```bash
curl -X POST "$OPENLIST/api/fs/rename" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/pikpak/old.txt",
    "name": "new.txt",
    "overwrite": false
  }'
```

PikPak 原始请求：

```http
PATCH https://api-drive.mypikpak.net/drive/v1/files/{file_id}
```

请求体：

```json
{
  "name": "new.txt"
}
```

限制：

- 不能重命名挂载根目录。
- `name` 不能包含 `/`、`\`，不能是空、`.`、`..`。
- 如果 `overwrite=false` 且目标名已存在，OpenList 层会先拒绝。

### 9.9 移动

请求：

```bash
curl -X POST "$OPENLIST/api/fs/move" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "src_dir": "/pikpak/source",
    "dst_dir": "/pikpak/target",
    "names": ["a.txt"],
    "overwrite": false,
    "skip_existing": false
  }'
```

同一个 PikPak storage 内移动时，OpenList 优先调用 PikPak 原生移动。

PikPak 原始请求：

```http
POST https://api-drive.mypikpak.net/drive/v1/files:batchMove
```

请求体：

```json
{
  "ids": ["<src_file_id>"],
  "to": {
    "parent_id": "<target_folder_id>"
  }
}
```

如果源和目标不是同一个 storage，OpenList 会创建移动任务：

```text
获取源文件下载链接
  -> 作为流读出
  -> 调目标 storage 的 Put
  -> 成功后删除源对象
```

### 9.10 复制

请求：

```bash
curl -X POST "$OPENLIST/api/fs/copy" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "src_dir": "/pikpak/source",
    "dst_dir": "/pikpak/target",
    "names": ["a.txt"],
    "overwrite": false,
    "skip_existing": false,
    "merge": false
  }'
```

PikPak 原始请求：

```http
POST https://api-drive.mypikpak.net/drive/v1/files:batchCopy
```

请求体：

```json
{
  "ids": ["<src_file_id>"],
  "to": {
    "parent_id": "<target_folder_id>"
  }
}
```

跨 storage 复制时，OpenList 使用任务化流式复制：

```text
源 storage Link
  -> seekable stream
  -> 目标 storage Put
```

### 9.11 删除

请求：

```bash
curl -X POST "$OPENLIST/api/fs/remove" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "dir": "/pikpak/source",
    "names": ["a.txt"]
  }'
```

PikPak 原始请求：

```http
POST https://api-drive.mypikpak.net/drive/v1/files:batchTrash
```

请求体：

```json
{
  "ids": ["<file_id>"]
}
```

注意：

- 当前驱动调用的是 `batchTrash`，语义是移入回收站。
- 不是永久删除。
- OpenList 删除挂载根 `/pikpak` 会被拒绝。

### 9.12 容量详情

PikPak 驱动实现了容量详情接口。

底层请求：

```http
GET https://api-drive.mypikpak.com/drive/v1/about
```

响应中的：

```text
quota.limit -> TotalSpace
quota.usage -> UsedSpace
```

OpenList 管理页或虚拟目录详情会展示这些数据，前提是没有开启隐藏容量详情设置。

---

## 10. WebDAV 接入 PikPak

### 10.1 WebDAV 地址

如果 PikPak 挂载在：

```text
/pikpak
```

WebDAV 地址：

```text
http://127.0.0.1:5244/dav/pikpak
```

如果挂载在：

```text
/影视/PikPak
```

WebDAV 地址：

```text
http://127.0.0.1:5244/dav/影视/PikPak
```

### 10.2 WebDAV 认证

默认使用 HTTP Basic：

```text
用户名：OpenList 用户名
密码：OpenList 用户密码
```

权限要求：

| 行为 | 权限 |
| --- | --- |
| `PROPFIND` / `GET` | 用户需要 WebDAV 读权限。 |
| `PUT` / `MKCOL` / `MOVE` / `COPY` / `DELETE` / `PROPPATCH` | 用户需要 WebDAV 写/管理权限。 |

OpenList 还支持在 WebDAV 请求中使用：

```http
Authorization: Bearer <openlist_global_token>
```

这种方式会以管理员身份访问，前提是 OpenList 设置中存在对应 token。

### 10.3 WebDAV 方法映射

| WebDAV 方法 | OpenList 层 | PikPak 驱动 |
| --- | --- | --- |
| `PROPFIND` | `fs.Get` / `fs.List` | `List` |
| `GET` / `HEAD` | `fs.Link` + 代理或 302 | `Link` |
| `PUT` | `fs.PutDirectly` | `Put` |
| `MKCOL` | `fs.MakeDir` | `MakeDir` |
| `MOVE` | `fs.Move` / `fs.Rename` | `Move` / `Rename` |
| `COPY` | `fs.Copy` | `Copy` |
| `DELETE` | `fs.Remove` | `Remove` |

### 10.4 `webdav_policy` 对下载的影响

| `webdav_policy` | 下载行为 |
| --- | --- |
| `302_redirect` | WebDAV `GET` 时返回 302，让客户端直接访问 PikPak 下载链接。 |
| `use_proxy_url` | 如果配置了 `down_proxy_url`，重定向到自定义代理地址。 |
| `native_proxy` | OpenList 自己拉取 PikPak 链接并把内容代理给 WebDAV 客户端。 |

写入行为不受 `webdav_policy` 影响；`PUT` 始终会进入 OpenList 的 `fs.PutDirectly`，最终调用 `PikPak.Put`。

### 10.5 向纯虚拟目录写入会怎样

假设挂载：

```text
/影视/PikPak
/影视/本地
```

那么：

```text
PROPFIND /dav/影视
```

可以看到：

```text
PikPak/
本地/
```

但是：

```text
PUT /dav/影视/a.txt
```

会失败，因为 `/影视` 是纯虚拟目录，没有唯一真实 storage 可以承接写入。

正确方式：

```text
PUT /dav/影视/PikPak/a.txt
PUT /dav/影视/本地/a.txt
```

OpenList 这样设计是为了避免歧义：如果虚拟目录下面有多个挂载点，系统不能自动猜测应该写入 PikPak 还是本地。

---

## 11. PikPak 离线下载接入

PikPak 驱动除了普通文件操作，还被 OpenList 离线下载系统复用。

### 11.1 启用 PikPak 离线下载工具

先确保已经有一个状态为 `work` 的 PikPak storage，例如：

```text
/pikpak
```

然后设置 PikPak 离线下载临时目录：

```bash
curl -X POST "$OPENLIST/api/admin/setting/set_pikpak" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"temp_dir":"/pikpak/offline-temp"}'
```

要求：

- `temp_dir` 必须能解析到一个真实 storage。
- 该 storage 必须是 `PikPak` 驱动。
- 该 storage 状态必须是 `work`。

### 11.2 添加离线下载任务

请求：

```bash
curl -X POST "$OPENLIST/api/fs/add_offline_download" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "urls": [
      "https://example.com/file.zip"
    ],
    "path": "/pikpak/downloads",
    "tool": "PikPak",
    "delete_policy": "delete_never"
  }'
```

响应示例：

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "tasks": [
      {
        "id": "<openlist_task_id>",
        "name": "download https://example.com/file.zip to (/pikpak/downloads)",
        "state": 0,
        "status": "",
        "progress": 0
      }
    ]
  }
}
```

### 11.3 离线下载目标目录选择规则

OpenList 添加离线下载时会先判断目标目录是哪种 storage：

| 目标目录 | 行为 |
| --- | --- |
| 目标本身是 PikPak storage | 直接把 PikPak 离线任务创建到目标目录。 |
| 目标不是 PikPak storage | 先把 PikPak 离线任务创建到配置的 `pikpak_temp_dir/<uuid>`，完成后再由 OpenList 转存到目标 storage。 |

示例：

```text
目标 path = /pikpak/downloads
  -> 直接离线下载到 /pikpak/downloads

目标 path = /local/downloads
  -> 先离线下载到 /pikpak/offline-temp/<uuid>
  -> 完成后 OpenList 读取 PikPak 文件链接
  -> 上传到 /local/downloads
```

### 11.4 PikPak 原始离线下载请求

接口：

```http
POST https://api-drive.mypikpak.net/drive/v1/files
```

请求体：

```json
{
  "kind": "drive#file",
  "name": "",
  "upload_type": "UPLOAD_TYPE_URL",
  "url": {
    "url": "https://example.com/file.zip"
  },
  "parent_id": "<target_folder_id>",
  "folder_type": ""
}
```

响应中 OpenList 关注：

```json
{
  "task": {
    "id": "<pikpak_task_id>",
    "phase": "PHASE_TYPE_PENDING",
    "progress": 0,
    "message": ""
  }
}
```

OpenList 会把 PikPak `task.id` 保存为下载任务的 `GID`。

### 11.5 查询离线下载任务

OpenList 任务列表：

```bash
curl "$OPENLIST/api/task/offline_download/undone" \
  -H "Authorization: $TOKEN"

curl "$OPENLIST/api/task/offline_download/done" \
  -H "Authorization: $TOKEN"
```

查询单个任务：

```bash
curl -X POST "$OPENLIST/api/task/offline_download/info?tid=<openlist_task_id>" \
  -H "Authorization: $TOKEN"
```

OpenList 更新任务状态时，底层会调用：

```http
GET https://api-drive.mypikpak.net/drive/v1/tasks
```

关键 query：

```text
type           = offline
thumbnail_size = SIZE_SMALL
limit          = 10000
page_token     = <next_page_token>
with           = reference_resource
filters        = {"phase":{"in":"PHASE_TYPE_RUNNING,PHASE_TYPE_ERROR,PHASE_TYPE_PENDING,PHASE_TYPE_COMPLETE"}}
```

PikPak 状态映射：

| PikPak `phase` | OpenList 处理 |
| --- | --- |
| `PHASE_TYPE_PENDING` | 等待中，任务继续轮询。 |
| `PHASE_TYPE_RUNNING` | 下载中，使用 `progress` 更新进度。 |
| `PHASE_TYPE_COMPLETE` | 下载完成，触发转存或结束。 |
| `PHASE_TYPE_ERROR` | 任务失败，`message` 作为错误信息。 |

任务列表有 10 秒缓存，用于减少频繁查询 PikPak 任务接口。

### 11.6 取消离线任务

OpenList 取消任务时，PikPak 工具会调用：

```http
DELETE https://api-drive.mypikpak.net/drive/v1/tasks?task_ids=<task_id>&delete_files=false
```

`delete_files=false` 表示删除任务记录时不主动删除已产生的文件。

---

## 12. 外部系统接入推荐方案

如果你在开发另一个系统，想把 PikPak 当成文件存储使用，推荐按下面方式接入 OpenList。

### 12.1 推荐架构

```text
你的系统
  -> OpenList HTTP API / WebDAV
      -> OpenList PikPak storage
          -> PikPak API
```

不要让你的系统直接保存 PikPak 账号密码、captcha token、OSS 参数。让 OpenList 作为适配层负责：

- PikPak 登录态；
- refresh token 自动更新；
- captcha token 自动刷新；
- 路径到 file id 的转换；
- 上传前 GCID 计算；
- 秒传和 OSS 分片上传；
- 目录缓存；
- 权限和 WebDAV 兼容。

### 12.2 最小接入流程

1. 管理员创建 PikPak storage。
2. 外部系统登录 OpenList 或使用 WebDAV Basic。
3. 列目录：

```http
POST /api/fs/list
```

4. 上传：

```http
PUT /api/fs/put
File-Path: /pikpak/path/to/file
```

5. 下载：

```http
POST /api/fs/get
```

或：

```http
GET /d/pikpak/path/to/file
```

6. 管理文件：

```http
POST /api/fs/mkdir
POST /api/fs/rename
POST /api/fs/move
POST /api/fs/copy
POST /api/fs/remove
```

### 12.3 路径选择建议

推荐把外部系统专用目录挂载清楚：

```text
/apps/my-system/pikpak
```

或在 PikPak 内新建目录后，把该目录 ID 作为 `root_folder_id`：

```text
mount_path     = /my-system
root_folder_id = <PikPak 中 my-system 文件夹 ID>
```

这样外部系统看到的 `/my-system` 就是 PikPak 子目录根，权限和目录边界更清晰。

---

## 13. 常见错误与排查

### 13.1 `username or password is empty`

原因：

- `addition.username` 或 `addition.password` 为空。
- API 创建时 `addition` JSON 转义错误，导致字段没有正确反序列化。

排查：

```bash
curl "$OPENLIST/api/admin/storage/get?id=<id>" \
  -H "Authorization: $TOKEN"
```

确认 `addition` 是字符串，且里面包含正确字段。

### 13.2 `refresh_token invalid, please re-provide refresh_token`

原因：

- 填入的 `refresh_token` 已失效。
- 且没有提供有效用户名密码供驱动重新登录。

处理：

- 清空 `refresh_token`；
- 填写正确 `username` / `password`；
- 保存 storage 让驱动重新登录。

### 13.3 `need verify: Click Here`

原因：

- PikPak 风控要求人工验证。

处理：

1. 打开返回的验证链接。
2. 完成验证。
3. 将新的 `captcha_token` 填入 storage。
4. 保存并重试。

### 13.4 `ErrorCode: 9`

语义：

- captcha token 过期。

驱动行为：

- 正常情况下会自动刷新并重试。

如果仍失败：

- 手动更新 `captcha_token`；
- 尝试切换 `platform`；
- 检查账号是否被风控。

### 13.5 `ErrorCode: 4122 / 4121 / 16`

语义：

- access token 过期或无效。

驱动行为：

- 自动用 refresh token 换 access token 并重试。

如果仍失败：

- refresh token 也可能失效；
- 清空 refresh token，让驱动用账号密码重新登录。

### 13.6 `ErrorCode: 10`

语义：

- 操作频繁。

处理：

- 降低请求频率；
- 增大 `cache_expiration`；
- 避免频繁 `refresh=true` 列目录；
- 等待一段时间再重试。

### 13.7 上传大文件很慢或先占用本地临时空间

原因：

- PikPak 上传前需要 GCID。
- 如果上传流没有 GCID，OpenList 会缓存完整文件计算 hash。
- 大于 10MB 后使用 OSS multipart，仍可能需要本地可随机读取的临时文件。

处理：

- 确保 OpenList 临时目录空间充足；
- 尽量让客户端提供可确定的 `Content-Length`；
- 避免网络不稳定时上传超大文件。

### 13.8 WebDAV 能看到 `/影视`，但不能上传到 `/影视`

原因：

- `/影视` 是纯虚拟目录，由 `/影视/PikPak`、`/影视/本地` 等挂载点动态生成。

处理：

- 上传到具体真实挂载点：

```text
/影视/PikPak/a.txt
/影视/本地/a.txt
```

### 13.9 删除后 PikPak 里还能找到文件

原因：

- OpenList 当前 PikPak 删除调用的是 `batchTrash`。
- 文件进入 PikPak 回收站，不是永久删除。

### 13.10 为什么 `/api/fs/get` 有时也触发列表请求

原因：

- PikPak 驱动没有实现按路径直接获取对象的 `Getter`。
- OpenList 需要列父目录，再按名称找到目标对象。

优化：

- 利用目录缓存；
- 不必要时不要频繁 `refresh=true`。

---

## 14. 设计原因与好处

### 14.1 统一 storage 抽象

OpenList 不让上层代码直接依赖 PikPak，而是要求所有网盘实现同一组驱动能力：

```text
List
Link
MakeDir
Put
Rename
Move
Copy
Remove
GetDetails
```

好处：

- Web UI、HTTP API、WebDAV 都不用知道 PikPak 细节。
- 同一个 `/api/fs/list` 可以列本地、S3、阿里云、PikPak。
- 跨 storage 复制/移动可以统一任务化处理。

### 14.2 路径与 ID 分离

OpenList 对用户暴露路径：

```text
/pikpak/movie/a.mp4
```

PikPak API 使用 ID：

```text
file_id = "VO..."
parent_id = "VN..."
```

驱动把 PikPak `File.id` 存进 OpenList `Object.ID`，后续移动、复制、删除、下载都使用该 ID。

好处：

- 上层保持类文件系统体验。
- 底层能使用 PikPak 原生高效接口。

### 14.3 自动 token/captcha 维护

如果外部系统直接调用 PikPak，需要自己处理：

- access token 过期；
- refresh token 失效；
- captcha token 过期；
- 人工验证；
- 不同平台签名；
- 重试逻辑。

OpenList 驱动集中处理这些复杂性，上层只看到成功或标准错误。

### 14.4 上传拆成“创建上传任务 + OSS 上传”

PikPak 不直接接收文件流，而是先返回 OSS 临时参数。OpenList 的处理方式：

```text
计算 GCID
  -> 创建 PikPak 上传任务
  -> 秒传成功则结束
  -> 否则按返回参数上传到 OSS
```

好处：

- 支持 PikPak 秒传。
- 大文件可分片并发上传。
- OpenList 上传任务能显示进度。

### 14.5 纯虚拟目录只读

当多个 storage 挂到同一父目录下：

```text
/影视/PikPak
/影视/本地
```

OpenList 自动生成 `/影视`。但是 `/影视` 不代表任何一个真实后端。

如果允许直接向 `/影视` 上传，系统无法判断写入 PikPak 还是本地。因此 OpenList 让纯虚拟目录只作为导航节点，写入必须选择具体挂载点。

好处：

- 行为明确；
- 不会误写到错误网盘；
- WebDAV 客户端行为可预期。

---

## 15. 接口总表

| 功能 | OpenList API / 入口 | PikPak 原始接口 | 说明 |
| --- | --- | --- | --- |
| 登录 OpenList | `POST /api/auth/login` | 无 | 获取 OpenList token。 |
| 查看驱动元数据 | `GET /api/admin/driver/info?driver=PikPak` | 无 | 查看字段定义。 |
| 创建 PikPak 挂载 | `POST /api/admin/storage/create` | 登录、token、captcha | 初始化 storage。 |
| 更新 PikPak 挂载 | `POST /api/admin/storage/update` | 登录、token、captcha | 修改参数后重新初始化。 |
| 列目录 | `POST /api/fs/list` | `GET /drive/v1/files` | 按 `parent_id` 分页列文件。 |
| 获取对象 | `POST /api/fs/get` | 通常回退到父目录 list | PikPak 驱动未实现直接 Getter。 |
| 下载 | `GET /d/*path` / `POST /api/fs/link` | `GET /drive/v1/files/{id}` | 获取 `web_content_link` 或媒体链接。 |
| 新建目录 | `POST /api/fs/mkdir` | `POST /drive/v1/files` | `kind=drive#folder`。 |
| 上传文件 | `PUT /api/fs/put` / `PUT /api/fs/form` | `POST /drive/v1/files` + OSS | 先建上传任务，再秒传或上传 OSS。 |
| 重命名 | `POST /api/fs/rename` | `PATCH /drive/v1/files/{id}` | 修改 `name`。 |
| 移动 | `POST /api/fs/move` | `POST /drive/v1/files:batchMove` | 同 storage 原生移动。 |
| 复制 | `POST /api/fs/copy` | `POST /drive/v1/files:batchCopy` | 同 storage 原生复制。 |
| 删除 | `POST /api/fs/remove` | `POST /drive/v1/files:batchTrash` | 移入回收站。 |
| 容量 | 管理页 / storage details | `GET /drive/v1/about` | 读取 quota。 |
| 设置 PikPak 离线下载 | `POST /api/admin/setting/set_pikpak` | 无 | 配置临时目录。 |
| 添加离线下载 | `POST /api/fs/add_offline_download` | `POST /drive/v1/files` | `upload_type=UPLOAD_TYPE_URL`。 |
| 查询离线任务 | `GET /api/task/offline_download/*` | `GET /drive/v1/tasks` | 轮询 PikPak 任务状态。 |
| 取消离线任务 | `POST /api/task/offline_download/cancel` | `DELETE /drive/v1/tasks` | `delete_files=false`。 |
| WebDAV 读 | `/dav/<mount>` | `List` / `Link` | 由 OpenList WebDAV 层转换。 |
| WebDAV 写 | `/dav/<mount>` | `Put` / `MakeDir` / `Move` / `Copy` / `Remove` | 要求路径落到真实 storage。 |

---

## 16. 最小可用接入 Checklist

### 16.1 管理员侧

- [ ] OpenList 可访问，例如 `http://127.0.0.1:5244`。
- [ ] 已有 OpenList 管理员账号。
- [ ] 已有 PikPak 账号和密码。
- [ ] 创建 storage：
  - [ ] `driver = PikPak`
  - [ ] `mount_path = /pikpak`
  - [ ] `username` 已填
  - [ ] `password` 已填
  - [ ] `platform = web`
  - [ ] `refresh_token` 首次可空
  - [ ] `disable_media_link = true`
- [ ] 保存后状态为 `work`。
- [ ] 调 `/api/fs/list` 能列出 `/pikpak`。
- [ ] 调 `/api/fs/mkdir` 能创建目录。
- [ ] 调 `/api/fs/put` 能上传小文件。
- [ ] 调 `/api/fs/get` 或 `/d/pikpak/...` 能下载文件。

### 16.2 外部系统侧

- [ ] 使用 OpenList token 或 WebDAV Basic，而不是 PikPak 账号密码。
- [ ] 所有写入路径都落到真实挂载点，例如 `/pikpak/...`，不要写纯虚拟目录。
- [ ] 上传时设置正确 `File-Path`。
- [ ] 大文件上传前确认 OpenList 临时目录空间足够。
- [ ] 如果走 WebDAV，用户有 WebDAV 读写权限。

---

## 17. 源码证据索引

本文已经把关键行为解释为自包含文档。若后续需要核对实现，可参考以下文件：

```text
PikPak 驱动注册与参数：
  drivers/pikpak/meta.go

PikPak 驱动核心能力：
  drivers/pikpak/driver.go

登录、refresh token、captcha、请求重试、OSS 上传：
  drivers/pikpak/util.go

PikPak 响应结构与对象映射：
  drivers/pikpak/types.go

storage 创建、初始化、保存、路径匹配、虚拟目录：
  internal/op/storage.go
  internal/op/path.go
  internal/fs/list.go
  internal/fs/get.go

OpenList 文件 API：
  server/router.go
  server/handles/fsread.go
  server/handles/fsmanage.go
  server/handles/fsup.go

OpenList 上传、复制、移动任务：
  internal/fs/put.go
  internal/fs/copy_move.go

WebDAV：
  server/webdav.go
  server/webdav/webdav.go

PikPak 离线下载适配：
  internal/offline_download/pikpak/pikpak.go
  internal/offline_download/pikpak/util.go
  internal/offline_download/tool/*.go
  server/handles/offline_download.go

统一响应：
  server/common/common.go
  server/common/resp.go
```

