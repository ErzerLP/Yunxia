import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const root = resolve(__dirname, '..')

function read(rel) {
  return readFileSync(resolve(root, rel), 'utf8')
}

function assertIncludes(rel, needle, message) {
  const content = read(rel)
  if (!content.includes(needle)) {
    throw new Error(`${message}\n  Missing ${JSON.stringify(needle)} in ${rel}`)
  }
}

function assertNotIncludes(rel, needle, message) {
  const content = read(rel)
  if (content.includes(needle)) {
    throw new Error(`${message}\n  Unexpected ${JSON.stringify(needle)} in ${rel}`)
  }
}

function assertRegex(rel, regex, message) {
  const content = read(rel)
  if (!regex.test(content)) {
    throw new Error(`${message}\n  Regex ${regex} did not match ${rel}`)
  }
}

assertIncludes(
  'src/components/layout/PreviewDrawer.tsx',
  'fileV2Api',
  'VFS 预览必须通过 /api/v2/fs/access-url，而不是 V1 files/access-url。',
)
assertRegex(
  'src/components/layout/PreviewDrawer.tsx',
  /mode\s*={2,3}\s*['"]v2['"]|previewMode|fileMode/,
  'PreviewDrawer 需要区分 V1/V2 预览模式。',
)

for (const rel of ['src/components/files/VFSFileList.tsx', 'src/components/files/VFSFileGrid.tsx']) {
  assertIncludes(rel, 'fileV2Api.accessUrl', 'VFS 下载必须先换取带 access_token 的临时 URL。')
  assertNotIncludes(rel, 'fileV2Api.download(item.path)', 'VFS 下载不能直接 window.open 无 token 的下载 URL。')
  assertIncludes(rel, 'shareApi.create', 'VFS 右键菜单需要能创建分享。')
}

assertNotIncludes(
  'src/components/files/VFSFileToolbar.tsx',
  'path_prefix',
  'VFS 搜索接口参数应使用后端契约里的 path，而不是 path_prefix。',
)
assertIncludes(
  'src/components/files/VFSFileToolbar.tsx',
  'path: currentVirtualPath',
  'VFS 搜索需要限定在当前虚拟路径。',
)
assertIncludes(
  'src/components/files/FileContextMenu.tsx',
  'onShare',
  '上下文菜单需要支持分享入口。',
)
assertIncludes(
  'src/pages/files/VFSFileManagerPage.tsx',
  "setMode('v2')",
  '进入 VFS 页面时必须把文件 Store 切换到 v2 模式，上传弹窗才能传虚拟路径。',
)
assertNotIncludes(
  'src/components/layout/Sidebar.tsx',
  "label: '虚拟目录'",
  '侧边栏文件相关入口只保留“文件”标签，不再展示“虚拟目录”标签。',
)
assertNotIncludes(
  'src/components/layout/Sidebar.tsx',
  "id: 'vfs'",
  '侧边栏不应再使用单独的 vfs 导航项。',
)
assertIncludes(
  'src/components/layout/Sidebar.tsx',
  "id: 'files'",
  '侧边栏必须保留唯一的文件入口。',
)
assertIncludes(
  'src/components/layout/Sidebar.tsx',
  "label: '文件'",
  '侧边栏文件入口展示名称必须是“文件”。',
)
assertIncludes(
  'src/router/index.tsx',
  '<Navigate to="/files" replace />',
  '首页默认入口应进入文件页。',
)
assertRegex(
  'src/router/index.tsx',
  /path:\s*['"]files['"],\s*element:\s*<VFSFileManagerPage\s*\/>/,
  '文件页 /files 必须使用虚拟目录页面，而不是传统存储源选择页面。',
)
assertRegex(
  'src/router/index.tsx',
  /path:\s*['"]files\/\*['"],\s*element:\s*<VFSFileManagerPage\s*\/>/,
  '文件页子路径 /files/* 必须使用虚拟目录页面。',
)
assertIncludes(
  'src/pages/files/FileManagerPage.tsx',
  "setMode('v1')",
  '回到传统文件页时必须把文件 Store 切回 v1 模式。',
)
assertIncludes(
  'src/components/files/UploadModal.tsx',
  'target_virtual_parent_path',
  'VFS 上传初始化必须传 target_virtual_parent_path。',
)
assertIncludes(
  'src/components/files/UploadModal.tsx',
  "queryKey: ['vfs'",
  'VFS 上传完成后必须刷新 VFS 查询缓存。',
)
assertIncludes(
  'src/api/sharePublic.ts',
  'getOpenUrl',
  '公开分享中的文件下载/预览必须使用直接打开 URL，不能用 XHR 追 302。',
)
assertIncludes(
  'src/pages/shares/ShareAccessPage.tsx',
  'verifiedPassword',
  '公开分享密码验证后需要保留密码用于目录继续浏览和文件下载。',
)

// Regression checks for reported frontend issues.
assertNotIncludes(
  'vite.config.ts',
  "'/s/'",
  'Vite dev server must not proxy /s/:token, otherwise direct share links render backend JSON instead of React page.',
)
assertIncludes(
  'src/pages/shares/SharesPage.tsx',
  'toFrontendShareLink',
  '分享管理页复制链接必须转换为前端 /s/:token 地址。',
)
assertIncludes(
  'src/utils/vfs.ts',
  'toFrontendShareLink',
  '需要统一 helper 将后端 share.link 转成前端公开分享页面链接。',
)
assertNotIncludes(
  'src/pages/acl/AclPage.tsx',
  'line-through',
  'ACL 权限展示不能把未授予的写/删/分享标签也显示出来。',
)
assertIncludes(
  'src/components/files/FileToolbar.tsx',
  'canWriteCurrentDirectory',
  '传统文件页工具栏必须按当前目录写权限隐藏上传/新建入口。',
)
assertIncludes(
  'src/components/files/VFSFileToolbar.tsx',
  'canWriteCurrentDirectory',
  'VFS 工具栏必须按当前目录写权限隐藏上传/新建入口。',
)
assertIncludes(
  'src/components/files/MkdirModal.tsx',
  'addToast',
  '传统文件页新建文件夹失败必须有可见错误提示。',
)

// WebDAV frontend management checks.
assertIncludes(
  'src/utils/webdav.ts',
  'buildSourceWebDAVUrl',
  '需要统一 helper 生成存储源 WebDAV 访问地址。',
)
assertIncludes(
  'src/utils/webdav.ts',
  'buildWebDAVBaseUrl',
  '系统设置页需要能展示全局 WebDAV Base URL。',
)
assertIncludes(
  'src/utils/webdav.ts',
  'toSecureWebDAVOrigin',
  'WebDAV 后端要求 HTTPS 语义，前端生成/复制地址时不能直接沿用当前 HTTP 页面 origin。',
)
assertIncludes(
  'src/utils/webdav.ts',
  "url.protocol = 'https:'",
  '当前页面为 HTTP 时，WebDAV 地址需要提升为 HTTPS，避免复制出会被后端拒绝的 http://.../dav 地址。',
)
assertIncludes(
  'src/pages/settings/SettingsPage.tsx',
  'buildWebDAVBaseUrl',
  '系统设置页需要展示 WebDAV Base URL，而不仅是开关状态。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  "queryKey: ['system-config']",
  '存储源页需要读取 webdav_prefix 来生成真实 WebDAV 地址。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  "useHasCapability('system.config.read')",
  '存储源页读取全局 WebDAV 配置前必须先判断 system.config.read，operator 只具备 source.read 时不能触发 /system/config 403。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'enabled: isAuthenticated && canReadSystemConfig',
  '存储源页的 system-config query 必须按 system.config.read capability 启用，不能仅按登录态启用。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  '当前账号无系统配置读取权限，无法确认全局 WebDAV 开关',
  '无 system.config.read 的账号查看 WebDAV 源时应展示未知状态说明，不能误提示全局 WebDAV 未启用。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'copyWebDAVUrl',
  '存储源卡片需要提供复制 WebDAV 地址的入口。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'isWebDAVOriginPromotedToHttps',
  '存储源卡片需要识别 HTTP 页面下 WebDAV 地址已提升为 HTTPS，并展示部署提示。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  '后端 WebDAV 要求 HTTPS',
  '存储源卡片不能静默复制 http:// WebDAV 地址；HTTP 部署下必须提示 HTTPS / X-Forwarded-Proto 要求。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'isWebDAVExposed',
  '新增/编辑存储源需要提供 WebDAV 暴露开关。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'webDAVReadOnly',
  '新增/编辑存储源需要提供 WebDAV 只读开关。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  '本地硬盘路径 / base_path',
  '添加本地存储源弹窗必须提供 config.base_path 输入项，不能让用户误把容器目录填到 root_path。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'base_path: basePath.trim()',
  '创建 local 存储源必须把本地硬盘路径提交为 config.base_path。',
)
assertNotIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'config: {},',
  '创建存储源不能无条件提交空 config，否则 local 源缺少 base_path 会失败。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'setCreateError',
  '创建存储源失败必须在弹窗内展示可见错误提示。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  "addToast(message, 'error')",
  '创建存储源失败必须弹出错误 toast，不能静默停留在弹窗。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'getCreateSourceErrorMessage',
  '创建存储源失败提示必须经过前端脱敏/友好化，不能直接展示后端数据库错误。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'getApiErrorMessage(err, fallback)',
  '创建存储源失败需要复用统一错误映射，不能把 source connection failed: cloud captcha required 等后端英文原文直接展示给用户。',
)
assertIncludes(
  'src/utils/apiError.ts',
  'cloud captcha required',
  '统一错误映射需要识别 provider 嵌套英文 cloud captcha required。',
)
assertIncludes(
  'src/utils/apiError.ts',
  'PikPak 需要完成安全验证',
  'PikPak captcha 错误必须展示中文友好提示。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'web_dav_slug',
  '创建第二个本地源遇到 web_dav_slug 唯一约束时，前端必须转换为用户可理解的提示。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'WebDAV 访问标识冲突',
  '创建第二个本地源遇到 WebDAV slug 冲突时不能弹出 UNIQUE constraint 数据库原文。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'autoComplete="off"',
  '存储源新增/编辑名称输入框必须显式声明 autocomplete，避免浏览器表单可访问性提示。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'id="source-create-driver-type-label"',
  '存储源新增弹窗驱动类型分组不能使用未关联的 label，应使用可访问的分组标题。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'role="radiogroup"',
  '存储源新增弹窗驱动类型按钮组需要暴露为可访问的单选分组。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'htmlFor="source-create-webdav-exposed"',
  '存储源新增弹窗 WebDAV 暴露复选框必须关联可见 label。',
)
assertNotIncludes(
  'src/pages/sources/SourcesPage.tsx',
  "const message = err instanceof Error ? err.message : '创建存储源失败'",
  '创建存储源 catch 不能直接把 err.message 暴露到弹窗和 toast。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'SourceConfigRows',
  '存储源卡片需要通过详情接口展示本地源 config.base_path 以及云存储源公开配置。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  "queryKey: ['source-detail'",
  '本地源卡片需要读取 /sources/:id 详情来拿 config.base_path。',
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  'source.mount_path',
  '存储源卡片必须显示实际挂载路径 mount_path，不能只显示 root_path。'
)
assertIncludes(
  'src/pages/sources/SourcesPage.tsx',
  '本地硬盘路径',
  '存储源卡片必须展示本地源的本地硬盘路径 base_path。',
)

// Regression checks for offline task status, public share routing, and transient empty file views.
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  'getTaskProgressPercent',
  '离线下载页需要对 completed/paused/running 等状态统一计算并展示进度。',
)
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  'STATUS_LABELS',
  '离线下载任务卡片需要显示明确状态文本，不能只靠图标。',
)
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  "task.status === 'completed'",
  '完成任务需要显示已完成/100%/下载字节等完成状态。',
)
assertIncludes(
  'src/App.tsx',
  'isPublicShareRoute',
  '前端全局鉴权初始化必须放行 /s/:token 公开分享页面。',
)
assertIncludes(
  'src/components/files/FileList.tsx',
  'displayedFiles',
  '传统文件列表应优先渲染本次 query data，避免 store effect 尚未同步时误显示空目录。',
)
assertIncludes(
  'src/components/files/FileGrid.tsx',
  'displayedFiles',
  '传统文件网格应优先渲染本次 query data，避免 store effect 尚未同步时误显示空目录。',
)
assertIncludes(
  'src/components/files/VFSFileList.tsx',
  'displayedVfsItems',
  'VFS 文件列表应优先渲染本次 query data，避免 store effect 尚未同步时误显示空目录。',
)
assertIncludes(
  'src/components/files/VFSFileGrid.tsx',
  'displayedVfsItems',
  'VFS 文件网格应优先渲染本次 query data，避免 store effect 尚未同步时误显示空目录。',
)
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  'invalidateCompletedTaskFileQueries',
  '离线下载任务完成后必须让文件页/VFS 页缓存失效，切换页面时能看到新文件。',
)
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  'getTaskSavePathLabel',
  '离线下载任务卡片保存路径需要统一使用展示 helper。',
)
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  'save_virtual_path',
  '离线下载任务保存路径展示必须优先使用后端返回的虚拟路径 save_virtual_path。',
)
assertNotIncludes(
  'src/pages/tasks/TasksPage.tsx',
  '保存至: {task.save_path}',
  '离线下载任务卡片不能只显示原始 save_path，否则 VFS 目标会误显示为 /。',
)
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  "queryKey: ['files']",
  '离线下载任务完成后必须失效传统文件列表查询缓存。',
)
assertIncludes(
  'src/pages/tasks/TasksPage.tsx',
  "queryKey: ['vfs']",
  '离线下载任务完成后必须失效 VFS 文件列表查询缓存。',
)
assertIncludes(
  'src/components/files/FileList.tsx',
  "refetchOnMount: 'always'",
  '传统文件列表进入页面时必须强制刷新，避免使用 staleTime 内的旧缓存。',
)
assertIncludes(
  'src/components/files/FileGrid.tsx',
  "refetchOnMount: 'always'",
  '传统文件网格进入页面时必须强制刷新，避免使用 staleTime 内的旧缓存。',
)
assertIncludes(
  'src/components/files/VFSFileList.tsx',
  "refetchOnMount: 'always'",
  'VFS 文件列表进入页面时必须强制刷新，避免使用 staleTime 内的旧缓存。',
)
assertIncludes(
  'src/components/files/VFSFileGrid.tsx',
  "refetchOnMount: 'always'",
  'VFS 文件网格进入页面时必须强制刷新，避免使用 staleTime 内的旧缓存。',
)

// Regression checks for login-related form accessibility.
assertIncludes(
  'src/pages/auth/LoginPage.tsx',
  'htmlFor="login-username"',
  '登录页用户名 label 必须用 htmlFor 关联输入框。',
)
assertIncludes(
  'src/pages/auth/LoginPage.tsx',
  'id="login-username"',
  '登录页用户名输入框必须提供 id。',
)
assertIncludes(
  'src/pages/auth/LoginPage.tsx',
  'name="username"',
  '登录页用户名输入框必须提供 name。',
)
assertIncludes(
  'src/pages/auth/LoginPage.tsx',
  'autoComplete="current-password"',
  '登录页密码框必须声明 current-password autocomplete。',
)
assertIncludes(
  'src/pages/auth/LoginPage.tsx',
  'id="login-password"',
  '登录页密码输入框必须提供 id。',
)
assertIncludes(
  'src/pages/setup/SetupPage.tsx',
  'htmlFor="setup-password"',
  '初始化页密码 label 必须用 htmlFor 关联输入框。',
)
assertIncludes(
  'src/pages/setup/SetupPage.tsx',
  'id="setup-username"',
  '初始化页用户名输入框必须提供 id。',
)
assertIncludes(
  'src/pages/setup/SetupPage.tsx',
  'name="confirmPassword"',
  '初始化页确认密码输入框必须提供 name。',
)
assertIncludes(
  'src/pages/setup/SetupPage.tsx',
  'autoComplete="new-password"',
  '初始化页新密码和确认密码必须声明 new-password autocomplete。',
)
assertIncludes(
  'src/pages/shares/ShareAccessPage.tsx',
  'htmlFor="share-access-password"',
  '公开分享密码表单必须用 label 关联密码框。',
)
assertIncludes(
  'src/pages/shares/ShareAccessPage.tsx',
  'id="share-access-password"',
  '公开分享密码框必须提供 id。',
)
assertIncludes(
  'src/pages/shares/ShareAccessPage.tsx',
  'name="password"',
  '公开分享密码框必须提供 name，避免浏览器表单可访问性提示。',
)

console.log('VFS integration static checks passed')
