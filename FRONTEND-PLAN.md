# 云匣 (Yunxia) 前端开发计划

> **版本**: v1.0
> **日期**: 2026-04-21
> **技术栈**: React 18 + TypeScript + Vite + Tailwind CSS + shadcn/ui + Zustand + TanStack Query
> **上游文档**: `PRD.md` / `TAD.md` / `FRONTEND-DESIGN.md` / `INTERFACE-ARCHITECTURE.md` / `API契约文档`

---

## 1. 项目概述

### 1.1 目标

为云匣自托管文件管理平台构建现代、高效、中文原生的 Web 前端。

### 1.2 核心原则

- **效率优先**: 减少点击，常用操作一步可达
- **上下文感知**: 右键菜单、悬浮工具栏，操作不离手
- **反馈即时**: 按钮 hover、加载骨架屏、操作 toast
- **一致性**: 共享设计语言、间距、圆角、动效
- **容错性**: 危险操作二次确认、撤销提示、优雅降级

### 1.3 技术栈确认

| 层级 | 选型 | 版本 |
|------|------|------|
| 框架 | React | 18+ |
| 语言 | TypeScript | 5.6+ |
| 构建 | Vite | 6+ |
| 样式 | Tailwind CSS | 3.4+ |
| UI 组件 | shadcn/ui | latest |
| 状态管理 | Zustand | 5+ |
| 服务端状态 | TanStack Query | 5+ |
| 路由 | React Router | v7+ |
| 请求库 | Axios | v1+ |
| 图标 | Lucide React | latest |
| 虚拟滚动 | @tanstack/react-virtual | v3 |
| 分片上传 MD5 | spark-md5 | v3 |
| 日期处理 | date-fns | v3+ |

---

## 2. 目录结构

```
web/
├── public/                          # 静态资源
│   ├── favicon.ico
│   └── logo.svg
│
├── src/
│   ├── main.tsx                     # 应用入口
│   ├── App.tsx                      # 根组件（路由 + 全局Provider）
│   ├── index.css                    # 全局样式 + Tailwind指令
│   │
│   ├── api/                         # API 客户端与请求函数
│   │   ├── client.ts                # Axios 实例（拦截器、baseURL）
│   │   ├── auth.ts                  # 认证相关 API
│   │   ├── files.ts                 # 文件管理 API
│   │   ├── upload.ts                # 上传相关 API
│   │   ├── sources.ts               # 存储源 API
│   │   ├── tasks.ts                 # 离线下载任务 API
│   │   ├── system.ts                # 系统配置 API
│   │   └── setup.ts                 # 初始化向导 API
│   │
│   ├── components/                  # 共享组件
│   │   ├── ui/                      # shadcn/ui 组件（自动生成的原始组件）
│   │   ├── layout/                  # 布局组件
│   │   │   ├── Sidebar.tsx          # 侧边栏
│   │   │   ├── TopBar.tsx           # 顶部工具栏
│   │   │   ├── MainLayout.tsx       # 主布局（Sidebar + Content）
│   │   │   └── MobileNav.tsx        # 移动端底部导航
│   │   ├── file/                    # 文件相关组件
│   │   │   ├── FileList.tsx         # 文件列表视图
│   │   │   ├── FileGrid.tsx         # 文件网格视图
│   │   │   ├── FileIcon.tsx         # 文件类型图标
│   │   │   ├── FileItemRow.tsx      # 列表行项
│   │   │   ├── FileItemCard.tsx     # 网格卡片项
│   │   │   ├── Breadcrumb.tsx       # 面包屑导航
│   │   │   ├── EmptyState.tsx       # 空状态
│   │   │   ├── ContextMenu.tsx      # 右键上下文菜单
│   │   │   └── BulkActionBar.tsx    # 批量操作栏
│   │   ├── preview/                 # 预览组件
│   │   │   ├── PreviewDrawer.tsx    # 右侧预览抽屉
│   │   │   ├── ImagePreview.tsx     # 图片预览
│   │   │   ├── VideoPreview.tsx     # 视频预览
│   │   │   ├── AudioPreview.tsx     # 音频预览
│   │   │   ├── PDFPreview.tsx       # PDF 预览
│   │   │   ├── TextPreview.tsx      # 文本/代码预览
│   │   │   └── OfficePlaceholder.tsx# Office 文件占位
│   │   ├── upload/                  # 上传组件
│   │   │   ├── UploadPanel.tsx      # 底部上传面板
│   │   │   ├── UploadTaskItem.tsx   # 上传任务项
│   │   │   └── UploadDropZone.tsx   # 拖拽上传遮罩
│   │   ├── common/                  # 通用组件
│   │   │   ├── SkeletonList.tsx     # 列表骨架屏
│   │   │   ├── SearchInput.tsx      # 搜索输入框
│   │   │   ├── ViewToggle.tsx       # 视图切换（列表/网格）
│   │   │   ├── SortDropdown.tsx     # 排序下拉
│   │   │   ├── Pagination.tsx       # 分页组件
│   │   │   ├── ConfirmDialog.tsx    # 确认对话框
│   │   │   ├── RenameDialog.tsx     # 重命名对话框
│   │   │   ├── MoveDialog.tsx       # 移动对话框
│   │   │   ├── CreateFolderDialog.tsx # 新建文件夹对话框
│   │   │   ├── SourceSelector.tsx   # 存储源选择器
│   │   │   └── UserMenu.tsx         # 用户头像下拉菜单
│   │   └── task/                    # 任务组件
│   │       └── TaskItem.tsx         # 下载任务项
│   │
│   ├── hooks/                       # 自定义 Hooks
│   │   ├── useAuth.ts               # 认证逻辑
│   │   ├── useFiles.ts              # 文件列表查询
│   │   ├── useUpload.ts             # 上传逻辑
│   │   ├── useSelection.ts          # 多选逻辑（Ctrl/Shift/单击）
│   │   ├── useContextMenu.ts        # 右键菜单逻辑
│   │   ├── useDragUpload.ts         # 拖拽上传逻辑
│   │   ├── useKeyboardShortcuts.ts  # 键盘快捷键
│   │   ├── useMediaQuery.ts         # 响应式断点
│   │   ├── useToast.ts              # Toast 通知
│   │   └── useTheme.ts              # 主题切换
│   │
│   ├── stores/                      # Zustand 状态管理
│   │   ├── authStore.ts             # 认证状态
│   │   ├── fileStore.ts             # 文件浏览状态
│   │   ├── uploadStore.ts           # 上传任务状态
│   │   ├── uiStore.ts               # UI 状态（主题、边栏、抽屉）
│   │   └── taskStore.ts             # 下载任务状态
│   │
│   ├── types/                       # TypeScript 类型定义
│   │   ├── api.ts                   # API 请求/响应类型
│   │   ├── models.ts                # 业务模型类型
│   │   └── enums.ts                 # 枚举类型
│   │
│   ├── utils/                       # 工具函数
│   │   ├── format.ts                # 格式化（文件大小、日期）
│   │   ├── file.ts                  # 文件相关工具（mime 判断、扩展名）
│   │   ├── path.ts                  # 路径处理
│   │   ├── hash.ts                  # MD5 计算（Web Worker 封装）
│   │   ├── request.ts               # 请求辅助（分页、排序参数构建）
│   │   └── validators.ts            # 表单校验
│   │
│   ├── workers/                     # Web Workers
│   │   └── md5.worker.ts            # 文件 MD5 计算 Worker
│   │
│   ├── pages/                       # 页面组件
│   │   ├── LoginPage.tsx            # 登录页
│   │   ├── SetupPage.tsx            # 初始化向导
│   │   ├── FilesPage.tsx            # 文件管理（核心页面）
│   │   ├── DownloadsPage.tsx        # 离线下载
│   │   ├── SourcesPage.tsx          # 存储源管理
│   │   ├── SharesPage.tsx           # 分享管理（P1 壳子）
│   │   └── SettingsPage.tsx         # 系统设置
│   │
│   ├── router/                      # 路由配置
│   │   ├── index.tsx                # 路由定义
│   │   ├── guards.tsx               # 路由守卫（认证、初始化检查）
│   │   └── lazyComponents.ts        # 懒加载组件映射
│   │
│   └── config/                      # 前端配置
│       └── constants.ts             # 常量（分页大小、动画时长等）
│
├── index.html
├── package.json
├── vite.config.ts
├── tsconfig.json
├── tsconfig.app.json
├── tailwind.config.js
├── components.json                  # shadcn/ui 配置
├── .env.development
├── .env.production
└── README.md
```

---

## 3. 开发阶段划分

### Phase 1: 脚手架与基础架构（第 1 周）

**目标**: 搭建可运行的前端项目骨架，完成基础配置和布局框架。

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 1.1 项目初始化 | Vite + React + TS 脚手架，配置路径别名 | `vite.config.ts`, `tsconfig.json` |
| 1.2 样式系统 | Tailwind CSS 配置，色彩 Token，深色模式 | `tailwind.config.js`, `index.css` |
| 1.3 shadcn/ui 集成 | 初始化 shadcn，安装基础组件 | `components.json`, `src/components/ui/` |
| 1.4 路由框架 | React Router 配置，路由守卫骨架 | `src/router/` |
| 1.5 状态管理骨架 | Zustand store 文件创建，Provider 包装 | `src/stores/` |
| 1.6 API 客户端 | Axios 实例，请求/响应拦截器，错误处理 | `src/api/client.ts` |
| 1.7 类型定义 | 核心模型类型、API 类型、枚举 | `src/types/` |
| 1.8 全局布局 | Sidebar + TopBar + MainLayout 骨架 | `src/components/layout/` |
| 1.9 工具函数 | 格式化、文件判断、路径处理 | `src/utils/` |

**关键决策**:
- 使用 `zustand` 的 `persist` 中间件持久化用户偏好（视图模式、主题、边栏状态）到 `localStorage`
- Axios 拦截器统一处理：`401` → 尝试 refresh → 失败则跳转登录
- 路由懒加载按页面拆分代码块

---

### Phase 2: 认证与初始化（第 1-2 周）

**目标**: 完成启动流程（初始化向导 → 登录 → 进入主界面）。

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 2.1 启动路由分流 | 应用启动时调用 `GET /api/v1/setup/status`，根据状态跳转 | `App.tsx` 启动逻辑 |
| 2.2 初始化向导页 | 三步向导：欢迎 → 创建管理员 → 完成 | `SetupPage.tsx` |
| 2.3 登录页 | 表单、验证、错误提示、加载状态 | `LoginPage.tsx` |
| 2.4 认证 API 封装 | login / refresh / logout / me | `src/api/auth.ts` |
| 2.5 Auth Store | 用户状态、Token 管理、登录/登出方法 | `authStore.ts` |
| 2.6 路由守卫 | 认证检查、初始化状态检查 | `guards.tsx` |
| 2.7 Token 刷新机制 | 定时刷新 + 请求拦截器自动刷新 | `client.ts` 拦截器 |

**API 对接**:
- `GET /api/v1/setup/status`
- `POST /api/v1/setup/init`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`

---

### Phase 3: 文件管理核心页面（第 2-4 周）

**目标**: 完成文件浏览、操作、预览等核心功能，这是最重要的页面。

#### 3.1 文件列表与浏览

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 3.1.1 文件 API 封装 | list / search / mkdir / rename / move / copy / delete / download / access-url | `src/api/files.ts` |
| 3.1.2 文件列表视图 | 表头、行项、排序、分页、加载骨架 | `FileList.tsx`, `FileItemRow.tsx` |
| 3.1.3 文件网格视图 | 卡片布局、响应式列数、缩略图 | `FileGrid.tsx`, `FileItemCard.tsx` |
| 3.1.4 视图切换 | 列表/网格切换，记住偏好 | `ViewToggle.tsx`, `uiStore` |
| 3.1.5 面包屑导航 | 存储源下拉 + 路径层级，过长折叠 | `Breadcrumb.tsx` |
| 3.1.6 搜索框 | 聚焦展开、实时搜索（防抖 300ms） | `SearchInput.tsx` |
| 3.1.7 空状态 | 插画 + 提示 + 上传按钮 | `EmptyState.tsx` |
| 3.1.8 虚拟滚动 | 大目录（>1000 文件）流畅滚动 | `@tanstack/react-virtual` |

#### 3.2 文件操作与交互

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 3.2.1 单击打开 | 文件夹进入、文件打开预览抽屉 | `FilesPage.tsx` 事件处理 |
| 3.2.2 多选逻辑 | Ctrl/Cmd + 单击多选、Shift 范围选择、全选 | `useSelection.ts` |
| 3.2.3 右键菜单 | 智能定位、操作项、多选时批量操作 | `ContextMenu.tsx` |
| 3.2.4 批量操作栏 | 底部弹出，显示选中数量 + 操作按钮 | `BulkActionBar.tsx` |
| 3.2.5 新建文件夹 | 对话框、表单验证 | `CreateFolderDialog.tsx` |
| 3.2.6 重命名 | 行内编辑或对话框 | `RenameDialog.tsx` |
| 3.2.7 移动/复制 | 目标目录选择对话框 | `MoveDialog.tsx` |
| 3.2.8 删除确认 | 二次确认对话框，显示删除模式 | `ConfirmDialog.tsx` |
| 3.2.9 拖拽上传 | 全局拖拽监听、遮罩提示 | `useDragUpload.ts`, `UploadDropZone.tsx` |
| 3.2.10 键盘快捷键 | Delete 删除、F2 重命名、Ctrl+A 全选、Ctrl+F 搜索、ESC 取消 | `useKeyboardShortcuts.ts` |

#### 3.3 文件预览抽屉

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 3.3.1 抽屉组件 | 右侧滑出、360px 宽、关闭按钮 | `PreviewDrawer.tsx` |
| 3.3.2 图片预览 | 原图展示、缩放、旋转 | `ImagePreview.tsx` |
| 3.3.3 视频预览 | HTML5 播放器、字幕、倍速、全屏 | `VideoPreview.tsx` |
| 3.3.4 音频预览 | 音频播放器 | `AudioPreview.tsx` |
| 3.3.5 PDF 预览 | PDF.js 渲染、翻页、缩放 | `PDFPreview.tsx` |
| 3.3.6 文本/代码预览 | 代码高亮 | `TextPreview.tsx` |
| 3.3.7 文件详情 | 元数据展示、操作按钮（下载/分享/重命名/删除） | `PreviewDrawer.tsx` 详情区 |
| 3.3.8 预览 URL 获取 | 调用 `POST /api/v1/files/access-url` | `files.ts` |

**API 对接**:
- `GET /api/v1/files`
- `GET /api/v1/files/search`
- `POST /api/v1/files/mkdir`
- `POST /api/v1/files/rename`
- `POST /api/v1/files/move`
- `POST /api/v1/files/copy`
- `DELETE /api/v1/files`
- `GET /api/v1/files/download`
- `POST /api/v1/files/access-url`

---

### Phase 4: 上传系统（第 3-4 周）

**目标**: 实现完整的分片上传、秒传、断点续传、上传面板。

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 4.1 上传 API 封装 | init / chunk / finish / sessions / cancel | `src/api/upload.ts` |
| 4.2 MD5 计算 Worker | Web Worker 计算文件 MD5，不阻塞主线程 | `md5.worker.ts`, `hash.ts` |
| 4.3 上传初始化 | 选择文件 → 计算 MD5 → 调用 init → 秒传检查 | `useUpload.ts` |
| 4.4 分片上传调度 | 并发 3 个 chunk、进度跟踪、错误重试 | `useUpload.ts` |
| 4.5 上传面板 UI | 底部浮动面板、展开/收起、任务列表 | `UploadPanel.tsx` |
| 4.6 上传任务项 | 文件名、进度条、速度、暂停/恢复、取消 | `UploadTaskItem.tsx` |
| 4.7 上传状态管理 | 任务列表、进度、面板开关 | `uploadStore.ts` |
| 4.8 会话恢复 | 页面刷新后获取活动会话，提示用户重新选择文件 | `uploadStore.ts` |
| 4.9 直传模式支持 | S3/OneDrive 的 `direct_parts` 模式，PUT 预签名 URL | `useUpload.ts` |

**上传流程**:
```
选择文件
  → 计算 MD5 (Web Worker)
  → POST /upload/init
    → 秒传成功 → 直接完成
    → 普通上传 → 获取 upload_id + chunk_info
      → server_chunk 模式 → PUT /upload/chunk (并发3)
      → direct_parts 模式 → PUT presigned_url (并发3)
  → 全部 chunk 完成
  → POST /upload/finish
  → 刷新文件列表
```

**API 对接**:
- `POST /api/v1/upload/init`
- `PUT /api/v1/upload/chunk`
- `POST /api/v1/upload/finish`
- `GET /api/v1/upload/sessions`
- `DELETE /api/v1/upload/sessions/:upload_id`

---

### Phase 5: 离线下载页面（第 4-5 周）

**目标**: 实现离线下载任务的管理页面。

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 5.1 任务 API 封装 | list / create / detail / cancel | `src/api/tasks.ts` |
| 5.2 任务列表 | 状态筛选（全部/进行中/已完成）、进度展示 | `DownloadsPage.tsx` |
| 5.3 任务项组件 | 名称、进度条、速度、ETA、操作按钮 | `TaskItem.tsx` |
| 5.4 新建任务对话框 | URL 输入、存储源选择、保存路径 | `DownloadsPage.tsx` |
| 5.5 状态轮询 | 每 5 秒轮询任务列表更新进度 | TanStack Query `refetchInterval` |
| 5.6 任务状态管理 | 任务列表、筛选条件 | `taskStore.ts` |

**API 对接**:
- `GET /api/v1/tasks`
- `POST /api/v1/tasks`
- `GET /api/v1/tasks/:id`
- `DELETE /api/v1/tasks/:id`

---

### Phase 6: 存储源管理（第 4-5 周）

**目标**: 实现存储源的查看、添加、编辑、测试。

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 6.1 存储源 API 封装 | list / get / create / update / delete / test | `src/api/sources.ts` |
| 6.2 存储源列表 | 卡片列表，显示图标、名称、驱动类型、状态 | `SourcesPage.tsx` |
| 6.3 添加/编辑表单 | 动态表单（根据 driver_type 变化字段） | `SourcesPage.tsx` |
| 6.4 连接测试 | 测试按钮、加载状态、结果反馈 | `sources.ts` |
| 6.5 侧边栏存储源 | Sidebar 中显示存储源列表，点击切换 | `Sidebar.tsx` |

**API 对接**:
- `GET /api/v1/sources`
- `GET /api/v1/sources/:id`
- `POST /api/v1/sources`
- `PUT /api/v1/sources/:id`
- `DELETE /api/v1/sources/:id`
- `POST /api/v1/sources/test`
- `POST /api/v1/sources/:id/test`

---

### Phase 7: 系统设置（第 5 周）

**目标**: 实现设置页面各 Tab。

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 7.1 系统 API 封装 | get config / update config / version | `src/api/system.ts` |
| 7.2 设置页框架 | Tab 分组布局 | `SettingsPage.tsx` |
| 7.3 通用设置 | 语言、主题、时区 | 通用 Tab |
| 7.4 存储设置 | 默认存储源、上传设置 | 存储 Tab |
| 7.5 WebDAV 设置 | 开关、暴露源列表 | WebDAV Tab |
| 7.6 用户管理 | 用户列表、创建、权限（仅管理员） | 用户 Tab |
| 7.7 安全设置 | 修改密码、Token 撤销 | 安全 Tab |
| 7.8 关于页面 | 版本信息、开源协议 | 关于 Tab |

**API 对接**:
- `GET /api/v1/system/config`
- `PUT /api/v1/system/config`
- `GET /api/v1/system/version`

---

### Phase 8: 体验优化与 Polish（第 5-6 周）

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 8.1 动效实现 | 页面加载、抽屉、弹窗、列表项、Toast 等动效 | CSS transition + Framer Motion |
| 8.2 深色模式 | 系统级暗色主题适配 | Tailwind dark mode |
| 8.3 响应式适配 | 移动端布局、底部导航、FAB、全屏预览 | 媒体查询 + 条件渲染 |
| 8.4 错误边界 | React Error Boundary，优雅降级 | `ErrorBoundary.tsx` |
| 8.5 加载状态优化 | 骨架屏、Suspense fallback | 各页面 |
| 8.6 Toast 通知系统 | 操作成功/失败/信息提示 | `useToast.ts`, `uiStore` |
| 8.7 性能优化 | 虚拟滚动、图片懒加载、代码分割 | 各组件 |
| 8.8 PWA 基础 | manifest、service worker 壳子（P1 完整实现）| `vite-plugin-pwa` |

---

### Phase 9: P1 功能壳子（第 6 周，可选）

| 任务 | 说明 | 交付物 |
|------|------|--------|
| 9.1 分享管理页壳子 | 页面框架、表格占位 | `SharesPage.tsx` |
| 9.2 用户管理壳子 | 在设置页中预留 | 设置页用户 Tab |
| 9.3 ACL 管理壳子 | 页面框架 | 预留路由 |
| 9.4 回收站壳子 | 页面框架 | 预留路由 |

---

## 4. 状态管理详细设计

### 4.1 Zustand Store 结构

```typescript
// stores/authStore.ts
interface AuthState {
  user: UserSummary | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshToken: () => Promise<void>;
  fetchMe: () => Promise<void>;
}

// stores/fileStore.ts
interface FileState {
  currentSourceId: number | null;
  currentPath: string;
  viewMode: 'list' | 'grid';
  sortBy: 'name' | 'size' | 'modified_at';
  sortOrder: 'asc' | 'desc';
  selectedFiles: Set<string>;        // 以 path 为 key
  lastSelectedIndex: number | null;  // 用于 Shift 范围选择
  setCurrentPath: (path: string) => void;
  setViewMode: (mode: 'list' | 'grid') => void;
  setSort: (by: string, order: string) => void;
  selectFile: (path: string, index: number, isMulti: boolean, isRange: boolean) => void;
  clearSelection: () => void;
}

// stores/uploadStore.ts
interface UploadState {
  tasks: UploadTask[];
  isPanelOpen: boolean;
  isPanelExpanded: boolean;
  addTask: (task: UploadTask) => void;
  updateTask: (id: string, updates: Partial<UploadTask>) => void;
  removeTask: (id: string) => void;
  togglePanel: () => void;
  toggleExpanded: () => void;
}

// stores/uiStore.ts
interface UIState {
  theme: 'light' | 'dark' | 'system';
  sidebarExpanded: boolean;
  previewDrawerOpen: boolean;
  previewFile: FileItem | null;
  toasts: Toast[];
  setTheme: (theme: 'light' | 'dark' | 'system') => void;
  toggleSidebar: () => void;
  openPreview: (file: FileItem) => void;
  closePreview: () => void;
  addToast: (toast: Omit<Toast, 'id'>) => void;
  removeToast: (id: string) => void;
}
```

### 4.2 TanStack Query 缓存策略

| 查询 | Query Key | 缓存时间 | 刷新策略 |
|------|-----------|---------|---------|
| 文件列表 | `['files', sourceId, path, page, sortBy, sortOrder]` | 5min | 操作后手动 invalidate |
| 搜索结果 | `['files', 'search', sourceId, keyword]` | 2min | 关键词变化自动刷新 |
| 存储源列表 | `['sources']` | 10min | 操作后手动 invalidate |
| 任务列表 | `['tasks', status]` | 5s (轮询) | `refetchInterval: 5000` |
| 当前用户 | `['auth', 'me']` | 10min | 登录/登出时更新 |
| 系统配置 | `['system', 'config']` | 10min | 更新后手动 invalidate |

---

## 5. 关键交互实现方案

### 5.1 文件多选逻辑

```typescript
// hooks/useSelection.ts 核心逻辑
function useSelection(files: FileItem[]) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [lastIndex, setLastIndex] = useState<number | null>(null);

  const handleSelect = (path: string, index: number, event: MouseEvent) => {
    const isCtrl = event.ctrlKey || event.metaKey;
    const isShift = event.shiftKey;

    if (isShift && lastIndex !== null) {
      // 范围选择
      const start = Math.min(lastIndex, index);
      const end = Math.max(lastIndex, index);
      const rangePaths = files.slice(start, end + 1).map(f => f.path);
      setSelected(prev => new Set([...prev, ...rangePaths]));
    } else if (isCtrl) {
      // 多选切换
      setSelected(prev => {
        const next = new Set(prev);
        if (next.has(path)) next.delete(path);
        else next.add(path);
        return next;
      });
      setLastIndex(index);
    } else {
      // 单选
      setSelected(new Set([path]));
      setLastIndex(index);
    }
  };

  return { selected, handleSelect, clearSelection };
}
```

### 5.2 分片上传调度

```typescript
// hooks/useUpload.ts 核心逻辑
async function uploadFile(file: File, sourceId: number, targetPath: string) {
  // 1. 计算 MD5
  const hash = await calculateMD5(file);

  // 2. 初始化上传
  const initRes = await uploadApi.init({
    source_id: sourceId,
    path: targetPath,
    filename: file.name,
    file_size: file.size,
    file_hash: hash,
  });

  if (initRes.is_fast_upload) {
    // 秒传成功
    addToast({ type: 'success', message: '秒传成功' });
    return;
  }

  // 3. 分片上传
  const { upload, transport, part_instructions } = initRes;
  const chunkSize = upload.chunk_size;
  const totalChunks = upload.total_chunks;
  const concurrency = transport.concurrency || 3;

  // 使用 p-limit 或自定义调度器控制并发
  const queue = new PQueue({ concurrency });

  for (let i = 0; i < totalChunks; i++) {
    if (upload.uploaded_chunks.includes(i)) continue;

    queue.add(async () => {
      const start = i * chunkSize;
      const end = Math.min(start + chunkSize, file.size);
      const chunk = file.slice(start, end);

      if (transport.mode === 'direct_parts') {
        const instruction = part_instructions.find(p => p.index === i);
        await axios.put(instruction.url, chunk, { headers: instruction.headers });
      } else {
        await uploadApi.chunk(upload.upload_id, i, chunk);
      }

      updateTask(upload.upload_id, { uploadedChunks: i + 1 });
    });
  }

  await queue.onIdle();

  // 4. 完成上传
  await uploadApi.finish(upload.upload_id, /* parts for direct mode */);
}
```

### 5.3 右键菜单定位

```typescript
// hooks/useContextMenu.ts
function useContextMenu() {
  const [menuState, setMenuState] = useState<{
    visible: boolean;
    x: number;
    y: number;
    file: FileItem | null;
  }>({ visible: false, x: 0, y: 0, file: null });

  const showMenu = (event: React.MouseEvent, file: FileItem) => {
    event.preventDefault();
    // 智能调整位置，避免超出视口
    const x = Math.min(event.clientX, window.innerWidth - 200);
    const y = Math.min(event.clientY, window.innerHeight - 300);
    setMenuState({ visible: true, x, y, file });
  };

  const hideMenu = () => setMenuState(prev => ({ ...prev, visible: false }));

  return { menuState, showMenu, hideMenu };
}
```

---

## 6. 路由设计

```typescript
// router/index.tsx
const routes = [
  // 公开路由
  { path: '/login', element: <LoginPage />, public: true },
  { path: '/setup', element: <SetupPage />, public: true },

  // 受保护路由（需认证）
  {
    path: '/',
    element: <MainLayout />,
    children: [
      { path: 'files', element: <FilesPage /> },
      { path: 'files/*', element: <FilesPage /> },  // 支持路径参数
      { path: 'downloads', element: <DownloadsPage /> },
      { path: 'sources', element: <SourcesPage /> },
      { path: 'shares', element: <SharesPage /> },
      { path: 'settings', element: <SettingsPage /> },
      { path: '', element: <Navigate to="/files" replace /> },
    ],
  },

  // 404
  { path: '*', element: <Navigate to="/files" replace /> },
];
```

---

## 7. 组件清单（shadcn/ui + 自定义）

### 7.1 shadcn/ui 基础组件（需安装）

| 组件 | 用途 |
|------|------|
| Button | 各种按钮 |
| Input | 输入框 |
| Label | 表单标签 |
| Dialog | 确认/表单对话框 |
| DropdownMenu | 下拉菜单、操作菜单 |
| ContextMenu | 右键菜单 |
| Tabs | 设置页分组 |
| Select | 下拉选择 |
| Checkbox | 复选框 |
| Progress | 进度条 |
| Tooltip | 提示 |
| Badge | 状态标识 |
| Separator | 分隔线 |
| ScrollArea | 滚动区域 |
| Sheet | 移动端抽屉/侧边栏 |
| Toast / Sonner | 通知提示 |
| Skeleton | 加载骨架 |
| Avatar | 用户头像 |
| Card | 卡片容器 |
| Form | 表单管理（配合 react-hook-form）|
| Table | 列表视图表格 |
| Switch | 开关 |
| Slider | 滑块 |
| Textarea | 多行文本 |
| Breadcrumb | 面包屑（如 shadcn 提供）|

### 7.2 自定义组件开发清单

见目录结构中的 `src/components/` 各子目录。

---

## 8. 样式与主题

### 8.1 Tailwind 配置要点

```javascript
// tailwind.config.js
module.exports = {
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        success: {
          DEFAULT: '#22c55e',
        },
        warning: {
          DEFAULT: '#f59e0b',
        },
      },
      borderRadius: {
        lg: '8px',
        md: '6px',
        sm: '4px',
      },
      fontFamily: {
        sans: ['Inter', 'PingFang SC', 'Microsoft YaHei', 'sans-serif'],
      },
    },
  },
};
```

### 8.2 CSS 变量（浅色/深色）

```css
/* index.css */
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    --background: 0 0% 100%;
    --foreground: 0 0% 9%;
    --muted: 0 0% 96%;
    --muted-foreground: 0 0% 45%;
    --border: 0 0% 90%;
    --primary: 217 91% 60%;
    --primary-foreground: 0 0% 100%;
    --accent: 0 0% 96%;
    --destructive: 0 84% 60%;
  }

  .dark {
    --background: 0 0% 4%;
    --foreground: 0 0% 98%;
    --muted: 0 0% 9%;
    --muted-foreground: 0 0% 64%;
    --border: 0 0% 15%;
    --primary: 217 91% 60%;
    --primary-foreground: 0 0% 100%;
    --accent: 0 0% 15%;
    --destructive: 0 62% 55%;
  }
}
```

---

## 9. 开发规范

### 9.1 代码规范

- **命名**: 组件 PascalCase，文件 kebab-case，函数 camelCase，常量 UPPER_SNAKE_CASE
- **类型**: 优先使用 interface 定义对象类型，type 定义联合/交叉类型
- **导入顺序**: React → 第三方库 → 内部模块 → 类型导入 → 样式
- **错误处理**: API 错误统一在拦截器处理，组件内只处理业务逻辑错误
- **注释**: 只写 WHY，不写 WHAT

### 9.2 文件组织原则

- 一个组件一个文件
- 组件相关的 hooks、types、utils 就近放置，必要时用 `ComponentName/` 目录
- 共享逻辑提取到 `hooks/` 或 `utils/`
- API 调用集中到 `api/` 目录，不在组件内直接写 axios 调用

### 9.3 性能规范

- 列表使用虚拟滚动（>100 项）
- 图片懒加载
- 路由级别代码分割
- 避免不必要的重渲染（React.memo 按需使用）
- 大计算使用 Web Worker

---

## 10. Mock 与联调策略

### 10.1 开发阶段 Mock

使用 MSW (Mock Service Worker) 或简单的 axios mock adapter：

```typescript
// 开发环境启用 Mock
if (import.meta.env.DEV && import.meta.env.VITE_ENABLE_MOCK === 'true') {
  import('./mocks/browser').then(({ worker }) => worker.start());
}
```

### 10.2 Mock 优先级

按 API 契约文档的 7.1 节优先级提供 Mock：
1. 第一优先级：认证 + 文件管理 + 上传 + 存储源 + 任务 + 系统配置
2. 第二优先级：用户管理 + ACL + 回收站 + 任务暂停恢复
3. 第三优先级：分享 + 全文搜索 + 审计日志（仅占位）

### 10.3 联调检查清单

- [ ] 启动流程：setup status → setup/init → login → files
- [ ] 文件 CRUD：list → mkdir → rename → move → copy → delete
- [ ] 上传流程：选择文件 → init → chunk → finish → 列表刷新
- [ ] 下载流程：access-url → 浏览器下载/预览
- [ ] 任务流程：create → list(轮询) → cancel
- [ ] 错误处理：401 refresh → 403 无权限 → 429 限流 → 500 服务端错误

---

## 11. 排期汇总

| 阶段 | 内容 | 工期 | 周次 |
|------|------|------|------|
| Phase 1 | 脚手架与基础架构 | 1 周 | 第 1 周 |
| Phase 2 | 认证与初始化 | 1 周 | 第 1-2 周 |
| Phase 3 | 文件管理核心 | 2-3 周 | 第 2-4 周 |
| Phase 4 | 上传系统 | 1-2 周 | 第 3-4 周 |
| Phase 5 | 离线下载 | 1 周 | 第 4-5 周 |
| Phase 6 | 存储源管理 | 1 周 | 第 4-5 周 |
| Phase 7 | 系统设置 | 1 周 | 第 5 周 |
| Phase 8 | 体验优化与 Polish | 1 周 | 第 5-6 周 |
| Phase 9 | P1 功能壳子 | 0.5 周 | 第 6 周 |

**总工期预估**: 6 周（约 1.5 个月），与后端 Phase 1-4 并行推进。

---

## 12. 风险与应对

| 风险 | 等级 | 应对策略 |
|------|------|---------|
| 后端 API 延迟交付 | 高 | 优先完成 Mock，确保前端可独立开发页面 |
| 大文件上传性能 | 中 | Web Worker 计算 MD5，分片并发控制，进度反馈 |
| 虚拟滚动复杂度 | 中 | 使用 `@tanstack/react-virtual`，先实现基础版本再优化 |
| 移动端适配 | 中 | 采用响应式断点，移动端单独测试，必要时简化交互 |
| shadcn/ui 组件不足 | 低 | 自定义组件补充，或引入 Radix UI  primitives 自行封装 |

---

*本计划基于 `FRONTEND-DESIGN.md` 和 API 契约文档制定，开发过程中如有需求变更，需同步更新本文档。*
