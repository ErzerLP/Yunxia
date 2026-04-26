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

console.log('VFS integration static checks passed')



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
