# 云匣前端开发进度

> 最后更新：2026-04-25

## 总体完成度：约 85%

---

## 一、基础架构（已完成）

| 模块 | 状态 | 说明 |
|------|------|------|
| 项目初始化 | 完成 | React 19 + TypeScript 6 + Vite 8 + Tailwind CSS 3.4 |
| 路由系统 | 完成 | `react-router-dom` + CapabilityRoute 权限守卫 |
| 状态管理 | 完成 | Zustand 5（authStore, uiStore, fileStore） |
| 服务端状态 | 完成 | TanStack Query 5 |
| HTTP 客户端 | 完成 | Axios + 自动 Token 刷新拦截器 |
| 主题系统 | 完成 | 浅色/深色/跟随系统，持久化到 localStorage |
| 布局组件 | 完成 | AppLayout, Sidebar（支持折叠）, Header |

---

## 二、认证与初始化（已完成）

| 功能 | 状态 | 文件 |
|------|------|------|
| 系统初始化引导 | 完成 | `pages/setup/SetupPage.tsx` |
| 登录页面 | 完成 | `pages/auth/LoginPage.tsx` |
| JWT 双 Token | 完成 | `api/client.ts` |
| 权限能力系统 | 完成 | `hooks/useCapability.ts`, `stores/authStore.ts` |

---

## 三、文件管理 V1（已完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| 文件列表（列表/网格） | 完成 | 支持排序、选择、批量操作 |
| 文件上传 | 完成 | 分片上传、秒传、拖拽上传 |
| 目录创建 | 完成 | `MkdirModal` |
| 重命名 | 完成 | `RenameModal` |
| 移动/复制 | 完成 | `MoveCopyModal` |
| 删除（回收站） | 完成 | `DeleteConfirmModal` |
| 文件预览 | 完成 | 图片、视频、文本等 |
| 下载 | 完成 | 临时 URL |
| 存储源选择器 | 完成 | `SourceSelector` |
| 面包屑导航 | 完成 | `FileBreadcrumb` |

---

## 四、虚拟目录 V2（已完成，待后端接口验证）

| 功能 | 状态 | 说明 |
|------|------|------|
| VFS 文件列表 | 完成 | `components/files/VFSFileList.tsx` |
| VFS 网格视图 | 完成 | `components/files/VFSFileGrid.tsx` |
| VFS 工具栏 | 完成 | `components/files/VFSFileToolbar.tsx`（无 SourceSelector） |
| VFS 面包屑 | 完成 | `components/files/VFSFileBreadcrumb.tsx` |
| VFS 目录创建 | 完成 | `components/files/VFSMkdirModal.tsx` |
| VFS 重命名 | 完成 | `components/files/VFSRenameModal.tsx` |
| VFS 移动/复制 | 完成 | `components/files/VFSMoveCopyModal.tsx` |
| VFS 删除 | 完成 | `components/files/VFSDeleteConfirmModal.tsx` |
| VFS 页面 | 完成 | `pages/files/VFSFileManagerPage.tsx` |
| V2 API 模块 | 完成 | `api/fileV2.ts` |

> **注意**：后端 `/api/v2/fs/*` 接口目前未完整实现。前端代码已就绪，待后端对接。

---

## 五、存储源管理（已完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| 存储源列表 | 完成 | `pages/sources/SourcesPage.tsx` |
| 创建/编辑存储源 | 完成 | 支持 local/s3/onedrive |
| 连通性测试 | 完成 | `TestSourceModal` |
| WebDAV 暴露设置 | 完成 | 开关 + 只读 + slug |

---

## 六、离线下载（已完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| 任务列表 | 完成 | `pages/tasks/TasksPage.tsx` |
| 创建下载任务 | 完成 | 支持选择存储源和保存路径 |
| 任务状态展示 | 完成 | 进度、速度、ETA |
| 暂停/继续/删除 | 完成 | 批量操作支持 |

---

## 七、分享功能（已完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| 分享列表 | 完成 | `pages/shares/SharesPage.tsx` |
| 创建分享 | 完成 | 支持有效期和密码 |
| 编辑分享 | 完成 | 修改有效期和密码 |
| 删除分享 | 完成 | 确认对话框 |
| 复制分享链接 | 完成 | 剪贴板写入 |
| **公开访问页** | **完成** | `pages/shares/ShareAccessPage.tsx`，路径 `/s/:token` |
| 公开密码验证 | 完成 | `api/sharePublic.ts` |
| 公开目录浏览 | 完成 | 子目录进入/返回 |
| 公开文件下载 | 完成 | 临时 URL |
| 公开文件预览 | 完成 | 支持可预览类型 |

---

## 八、回收站（已完成）

| 功能 | 状态 | 说明 |
|------|------|------|
| 回收站列表 | 完成 | `pages/trash/TrashPage.tsx` |
| 还原文件 | 完成 | 单条 + 批量 |
| 彻底删除 | 完成 | 确认对话框 |

---

## 九、系统管理（已完成）

### 9.1 系统设置
| 功能 | 状态 | 说明 |
|------|------|------|
| 系统统计展示 | 完成 | 用户数、存储源、文件数、容量、任务数、分享数 |
| 系统配置查看 | 完成 | 站点名称、多用户、WebDAV、上传限制等 |
| 系统配置编辑 | 完成 | `ConfigEditModal`，需 `system.config.write` |
| 主题切换 | 完成 | 个人偏好，即时生效 |
| 退出登录 | 完成 | 清除 Token 并跳转 |

### 9.2 用户管理
| 功能 | 状态 | 说明 |
|------|------|------|
| 用户列表 | 完成 | `pages/users/UsersPage.tsx` |
| 创建用户 | 完成 | 需 `user.create` |
| 编辑用户 | 完成 | 角色、状态 |
| 删除用户 | 完成 | 确认对话框 |

### 9.3 ACL 管理
| 功能 | 状态 | 说明 |
|------|------|------|
| ACL 规则列表 | 完成 | `pages/acl/AclPage.tsx` |
| 创建/编辑规则 | 完成 | 主体类型、效果、权限、继承 |
| 删除规则 | 完成 | 确认对话框 |
| 权限可视化 | 完成 | `EffectBadge`, `SubjectBadge`, `PermissionsDisplay` |

### 9.4 审计日志
| 功能 | 状态 | 说明 |
|------|------|------|
| 审计日志列表 | 完成 | `pages/audit/AuditPage.tsx` |
| 筛选查询 | 完成 | 用户ID、资源类型、动作、结果 |
| 详情抽屉 | 完成 | `AuditLogDrawer`，完整信息展示 |
| 分页 | 完成 | 20条/页 |

---

## 十、待办事项（剩余工作）

### 10.1 后端依赖
| 事项 | 优先级 | 说明 |
|------|--------|------|
| V2 虚拟目录接口 | 中 | 后端 `/api/v2/fs/*` 未完整实现，前端已就绪 |
| 公开分享 API | 中 | `sharePublicApi` 基于假设的接口设计，需后端确认 |

### 10.2 前端优化
| 事项 | 优先级 | 说明 |
|------|--------|------|
| 移动端适配 | 低 | 当前主要面向桌面端 |
| 文件拖拽排序 | 低 | 虚拟目录内手动排序 |
| 图片懒加载 | 低 | 大目录性能优化 |
| 批量分享创建 | 低 | 多选文件一键分享 |
| 国际化完善 | 低 | 当前仅简体中文 |

### 10.3 类型清理（技术债）
| 事项 | 优先级 | 说明 |
|------|--------|------|
| 虚拟路径字段 | 低 | `types/api.ts` 中部分 `*_virtual_*` 字段为前瞻性占位符，后端尚未实现。当前保留不影响功能，未来后端支持后可无缝启用。 |

---

## 文件变更统计（本次会话）

```
24 files changed, 3503 insertions(+), 12 deletions(-)
```

### 新增文件
- `src/api/acl.ts`
- `src/api/audit.ts`
- `src/api/fileV2.ts`
- `src/api/sharePublic.ts`
- `src/components/files/VFSDeleteConfirmModal.tsx`
- `src/components/files/VFSFileBreadcrumb.tsx`
- `src/components/files/VFSFileGrid.tsx`
- `src/components/files/VFSFileList.tsx`
- `src/components/files/VFSFileToolbar.tsx`
- `src/components/files/VFSMkdirModal.tsx`
- `src/components/files/VFSMoveCopyModal.tsx`
- `src/components/files/VFSRenameModal.tsx`
- `src/pages/acl/AclPage.tsx`
- `src/pages/audit/AuditPage.tsx`
- `src/pages/files/VFSFileManagerPage.tsx`
- `src/pages/shares/ShareAccessPage.tsx`

### 修改文件
- `src/api/system.ts`
- `src/components/layout/Sidebar.tsx`
- `src/pages/settings/SettingsPage.tsx`
- `src/pages/shares/SharesPage.tsx`
- `src/router/index.tsx`
- `src/stores/fileStore.ts`
- `src/types/api.ts`
