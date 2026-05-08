import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { shareApi } from '@/api/share'
import { fileV2Api } from '@/api/fileV2'
import { sourceApi } from '@/api/source'
import { useUIStore } from '@/stores/uiStore'
import {
  Link,
  Plus,
  Trash2,
  Clock,
  Lock,
  Folder,
  FileText,
  Image,
  Film,
  Music,
  File,
  X,
  Copy,
  Pencil,
} from 'lucide-react'
import { formatDate, getApiErrorMessage, getFileIconClass } from '@/utils'
import { cn } from '@/utils'
import { useHasCapability } from '@/hooks/useCapability'
import { buildVfsShareRequest, getVfsParentPath, normalizeVfsPath, toFrontendShareLink } from '@/utils/vfs'
import type { CreateShareRequest, Share } from '@/types/api'

const iconMap = {
  folder: Folder,
  image: Image,
  video: Film,
  audio: Music,
  file: File,
  document: FileText,
  spreadsheet: FileText,
  presentation: FileText,
  code: FileText,
  pdf: FileText,
  archive: File,
}

const fetchShareCreateSources = () =>
  sourceApi.list({ page: 1, page_size: 100, view: 'navigation' })

function ShareIcon({ share }: { share: Share }) {
  const type = getFileIconClass('', share.is_dir)
  const Icon = iconMap[type as keyof typeof iconMap] || File
  return (
    <Icon
      className={cn(
        'w-5 h-5 shrink-0',
        share.is_dir ? 'text-primary' : 'text-muted-foreground'
      )}
    />
  )
}

function EditShareModal({
  onClose,
  onSuccess,
  share,
}: {
  onClose: () => void
  onSuccess: () => void
  share: Share
}) {
  const [expiresIn, setExpiresIn] = useState('')
  const [password, setPassword] = useState('')
  const [removePassword, setRemovePassword] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const { addToast } = useUIStore()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSubmitting(true)
    try {
      const data: { expires_in?: number; password?: string | null } = {}
      const exp = parseInt(expiresIn, 10)
      if (expiresIn && !isNaN(exp) && exp > 0) {
        data.expires_in = exp
      }
      if (removePassword) {
        data.password = null
      } else if (password.trim()) {
        data.password = password.trim()
      }
      await shareApi.update(share.id, data)
      addToast('分享已更新', 'success')
      onSuccess()
      onClose()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '更新失败'
      addToast(msg, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-md bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Pencil className="w-4 h-4" />
            编辑分享
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">名称</label>
            <p className="text-sm text-foreground px-3 py-2 rounded-md border border-border bg-muted/50">
              {share.name}
            </p>
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">新的有效期（秒，可选）</label>
            <input
              type="number"
              value={expiresIn}
              onChange={(e) => setExpiresIn(e.target.value)}
              placeholder="86400"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">新密码（可选）</label>
            <input
              type="text"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="留空表示不修改"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          {share.has_password && (
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={removePassword}
                onChange={(e) => setRemovePassword(e.target.checked)}
                className="rounded border-border"
              />
              <span className="text-sm text-foreground">移除密码</span>
            </label>
          )}
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                isSubmitting && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '保存中...' : '保存'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function CreateShareModal({
  onClose,
  onSuccess,
}: {
  onClose: () => void
  onSuccess: () => void
}) {
  const { addToast } = useUIStore()
  const queryClient = useQueryClient()
  const [vfsPath, setVfsPath] = useState('/')
  const [legacySourceId, setLegacySourceId] = useState('')
  const [legacyPath, setLegacyPath] = useState('/')
  const [expiresIn, setExpiresIn] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const { data: sourcesData } = useQuery({
    queryKey: ['share-create-sources'],
    queryFn: fetchShareCreateSources,
    retry: false,
    staleTime: 30_000,
  })
  const sources = sourcesData?.items ?? []

  const getSourcesForShareFallback = async () => {
    try {
      const result = await queryClient.fetchQuery({
        queryKey: ['share-create-sources'],
        queryFn: fetchShareCreateSources,
        staleTime: 30_000,
      })
      return result.items ?? []
    } catch {
      return sources
    }
  }

  const resolveShareTarget = async (targetPath: string): Promise<CreateShareRequest | null> => {
    const normalizedPath = normalizeVfsPath(targetPath)
    if (normalizedPath !== '/') {
      const parentPath = getVfsParentPath(normalizedPath)
      const list = await fileV2Api.list({ path: parentPath, page: 1, page_size: 200 })
      const target = list.items.find((item) => normalizeVfsPath(item.path) === normalizedPath)
      if (target) {
        const fallbackSources = await getSourcesForShareFallback()
        const nodeFirstPayload = buildVfsShareRequest(target, fallbackSources)
        if (nodeFirstPayload) {
          return nodeFirstPayload
        }
      }
    }

    const sid = parseInt(legacySourceId, 10)
    if (sid && legacyPath.trim()) {
      return {
        source_id: sid,
        path: legacyPath.trim(),
      }
    }
    return null
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!vfsPath.trim()) return

    setIsSubmitting(true)
    setError('')
    try {
      const data = await resolveShareTarget(vfsPath)
      if (!data) {
        throw new Error('未找到可分享的 VFS 节点；请先进入该目录触发索引，或填写兼容 source_id + 源内路径。')
      }
      const exp = parseInt(expiresIn, 10)
      if (expiresIn && !isNaN(exp) && exp > 0) {
        data.expires_in = exp
      }
      if (password.trim()) {
        data.password = password.trim()
      }
      await shareApi.create(data)
      onSuccess()
      onClose()
    } catch (err: unknown) {
      const message = getApiErrorMessage(err, '创建分享失败')
      setError(message)
      addToast(message, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-md bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Plus className="w-4 h-4" />
            创建分享
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          {error && (
            <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}
          <div>
            <label htmlFor="share-create-vfs-path" className="text-sm text-muted-foreground mb-1 block">目标 VFS 路径</label>
            <input
              id="share-create-vfs-path"
              name="vfs_path"
              type="text"
              value={vfsPath}
              onChange={(e) => setVfsPath(e.target.value)}
              placeholder="/local/docs/hello.txt"
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
              required
            />
            <p className="mt-1 text-xs text-muted-foreground">
              前端会优先解析为 metadata VFS 节点，分享会在重命名/移动后跟随同一节点。
            </p>
          </div>
          <div className="rounded-md border border-border bg-muted/20 p-3 space-y-3">
            <p className="text-xs font-medium text-muted-foreground">兼容 fallback（仅旧数据或无法解析节点时填写）</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label htmlFor="share-create-legacy-source-id" className="text-sm text-muted-foreground mb-1 block">存储源 ID</label>
                <input
                  id="share-create-legacy-source-id"
                  name="legacy_source_id"
                  type="number"
                  value={legacySourceId}
                  onChange={(e) => setLegacySourceId(e.target.value)}
                  placeholder="1"
                  className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                />
              </div>
              <div>
                <label htmlFor="share-create-legacy-path" className="text-sm text-muted-foreground mb-1 block">源内路径</label>
                <input
                  id="share-create-legacy-path"
                  name="legacy_path"
                  type="text"
                  value={legacyPath}
                  onChange={(e) => setLegacyPath(e.target.value)}
                  placeholder="/docs/hello.txt"
                  autoComplete="off"
                  className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                />
              </div>
            </div>
          </div>
          <div>
            <label htmlFor="share-create-expires-in" className="text-sm text-muted-foreground mb-1 block">有效期（秒，可选）</label>
            <input
              id="share-create-expires-in"
              name="expires_in"
              type="number"
              value={expiresIn}
              onChange={(e) => setExpiresIn(e.target.value)}
              placeholder="86400"
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div>
            <label htmlFor="share-create-password" className="text-sm text-muted-foreground mb-1 block">密码（可选）</label>
            <input
              id="share-create-password"
              name="password"
              type="text"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder=""
              autoComplete="new-password"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={isSubmitting || !vfsPath.trim()}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || !vfsPath.trim()) && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '创建中...' : '创建'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export function SharesPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const { addToast } = useUIStore()
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Share | null>(null)
  const canManageAll = useHasCapability('share.manage_all')

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data, isLoading } = useQuery({
    queryKey: ['shares'],
    queryFn: () => shareApi.list(),
  })

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除此分享吗？')) return
    try {
      await shareApi.delete(id)
      addToast('分享已删除', 'success')
      queryClient.invalidateQueries({ queryKey: ['shares'] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '删除失败'
      addToast(msg, 'error')
    }
  }

  const handleCopyLink = (link: string) => {
    navigator.clipboard.writeText(toFrontendShareLink(link)).then(() => {
      addToast('链接已复制到剪贴板', 'success')
    })
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const shares = data?.items || []

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">分享管理</h1>
        {canManageAll && (
          <button
            onClick={() => setCreateModalOpen(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            <Plus className="w-4 h-4" />
            <span>创建分享</span>
          </button>
        )}
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {shares.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Link className="w-12 h-12 opacity-30" />
            <p>暂无分享</p>
          </div>
        ) : (
          <div className="space-y-2">
            {shares.map((share) => (
              <div
                key={share.id}
                className="flex items-center gap-3 p-3 rounded-lg border border-border bg-card"
              >
                <ShareIcon share={share} />
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-foreground truncate">{share.name}</p>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                    <span>{share.is_dir ? '文件夹' : '文件'}</span>
                    {share.target_vfs_node_id && (
                      <span className="text-primary">节点 #{share.target_vfs_node_id}</span>
                    )}
                    {share.has_password && (
                      <span className="inline-flex items-center gap-0.5">
                        <Lock className="w-3 h-3" /> 密码保护
                      </span>
                    )}
                    {share.expires_at && (
                      <span className="inline-flex items-center gap-0.5">
                        <Clock className="w-3 h-3" /> {formatDate(share.expires_at)}
                      </span>
                    )}
                  </div>
                  <p
                    className="mt-1 truncate font-mono text-[11px] text-muted-foreground"
                    title={share.target_virtual_path || share.path}
                  >
                    快照路径：{share.target_virtual_path || share.path}
                    {share.resolved_inner_path && ` · 源内 ${share.resolved_inner_path}`}
                  </p>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  <button
                    onClick={() => handleCopyLink(share.link)}
                    className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                    title="复制链接"
                  >
                    <Copy className="w-4 h-4" />
                  </button>
                  {canManageAll && (
                    <button
                      onClick={() => setEditTarget(share)}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      title="编辑"
                    >
                      <Pencil className="w-4 h-4" />
                    </button>
                  )}
                  <button
                    onClick={() => handleDelete(share.id)}
                    className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                    title="删除"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {createModalOpen && (
        <CreateShareModal
          onClose={() => setCreateModalOpen(false)}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['shares'] })
            addToast('分享创建成功', 'success')
          }}
        />
      )}
      {editTarget && (
        <EditShareModal
          key={editTarget.id}
          onClose={() => setEditTarget(null)}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['shares'] })
          }}
          share={editTarget}
        />
      )}
    </div>
  )
}
