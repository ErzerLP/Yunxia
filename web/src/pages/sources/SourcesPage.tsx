import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { sourceApi } from '@/api/source'
import { HardDrive, Plus, CheckCircle2, XCircle, AlertCircle, Trash2, RefreshCw, X } from 'lucide-react'
import { cn, formatBytes } from '@/utils'
import { useFileStore } from '@/stores/fileStore'
import type { StorageSource } from '@/types/api'

function StatusBadge({ status }: { status: StorageSource['status'] }) {
  const config = {
    online: { icon: CheckCircle2, class: 'text-emerald-500 bg-emerald-500/10', label: '在线' },
    offline: { icon: XCircle, class: 'text-muted-foreground bg-muted', label: '离线' },
    error: { icon: AlertCircle, class: 'text-destructive bg-destructive/10', label: '错误' },
  }
  const { icon: Icon, class: cls, label } = config[status]
  return (
    <span className={cn('inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium', cls)}>
      <Icon className="w-3 h-3" />
      {label}
    </span>
  )
}

function CreateSourceModal({ isOpen, onClose, onSuccess }: { isOpen: boolean; onClose: () => void; onSuccess: () => void }) {
  const [name, setName] = useState('')
  const [driverType, setDriverType] = useState<'local' | 's3'>('local')
  const [rootPath, setRootPath] = useState('/data')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (isOpen) {
      setName('')
      setDriverType('local')
      setRootPath('/data')
    }
  }, [isOpen])

  if (!isOpen) return null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setIsSubmitting(true)
    try {
      await sourceApi.create({
        name: name.trim(),
        driver_type: driverType,
        is_enabled: true,
        is_webdav_exposed: false,
        webdav_read_only: false,
        root_path: rootPath,
        config: {},
      })
      onSuccess()
      onClose()
    } catch {
      // ignore
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
            添加存储源
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">名称</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：本地存储"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">驱动类型</label>
            <div className="flex gap-2">
              {(['local', 's3'] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setDriverType(t)}
                  className={cn(
                    'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                    driverType === t
                      ? 'border-primary bg-primary/5 text-primary'
                      : 'border-border text-muted-foreground hover:border-primary/30'
                  )}
                >
                  {t === 'local' ? '本地' : 'S3'}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">根路径</label>
            <input
              type="text"
              value={rootPath}
              onChange={(e) => setRootPath(e.target.value)}
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
              disabled={isSubmitting || !name.trim()}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || !name.trim()) && 'opacity-50 cursor-not-allowed'
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

export function SourcesPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const { setCurrentSource, currentSource } = useFileStore()
  const [createModalOpen, setCreateModalOpen] = useState(false)

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data, isLoading } = useQuery({
    queryKey: ['sources'],
    queryFn: () => sourceApi.list({ page: 1, page_size: 100, view: 'admin' }),
  })

  const handleTest = async (id: number) => {
    try {
      await sourceApi.testById(id)
      queryClient.invalidateQueries({ queryKey: ['sources'] })
    } catch {
      // ignore
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除此存储源吗？此操作不可撤销。')) return
    try {
      await sourceApi.delete(id)
      if (currentSource?.id === id) {
        setCurrentSource(null)
      }
      queryClient.invalidateQueries({ queryKey: ['sources'] })
    } catch {
      // ignore
    }
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const sources = data?.items || []

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">存储源管理</h1>
        <button
          onClick={() => setCreateModalOpen(true)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
        >
          <Plus className="w-4 h-4" />
          <span>添加存储源</span>
        </button>
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {sources.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <HardDrive className="w-12 h-12 opacity-30" />
            <p>暂无存储源</p>
            <button
              onClick={() => setCreateModalOpen(true)}
              className="px-4 py-2 rounded-md bg-primary text-primary-foreground text-sm hover:bg-primary/90 transition-colors"
            >
              添加存储源
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {sources.map((source) => (
              <div
                key={source.id}
                className={cn(
                  'p-4 rounded-lg border transition-all cursor-pointer',
                  currentSource?.id === source.id
                    ? 'border-primary bg-primary/5'
                    : 'border-border bg-card hover:border-primary/30'
                )}
                onClick={() => setCurrentSource(source)}
              >
                <div className="flex items-start justify-between mb-3">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                      <HardDrive className="w-5 h-5 text-primary" />
                    </div>
                    <div>
                      <h3 className="font-medium text-foreground">{source.name}</h3>
                      <p className="text-xs text-muted-foreground uppercase">{source.driver_type}</p>
                    </div>
                  </div>
                  <StatusBadge status={source.status} />
                </div>

                <div className="space-y-2">
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">路径</span>
                    <span className="text-foreground truncate max-w-[180px]">{source.root_path}</span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">容量</span>
                    <span className="text-foreground">
                      {formatBytes(source.used_bytes)} / {formatBytes(source.total_bytes)}
                    </span>
                  </div>
                  {source.total_bytes && source.used_bytes !== null && (
                    <div className="w-full h-1.5 bg-muted rounded-full overflow-hidden">
                      <div
                        className="h-full bg-primary rounded-full transition-all"
                        style={{ width: `${Math.min((source.used_bytes / source.total_bytes) * 100, 100)}%` }}
                      />
                    </div>
                  )}
                </div>

                <div className="flex items-center gap-2 mt-4 pt-3 border-t border-border">
                  <span
                    className={cn(
                      'text-xs px-2 py-0.5 rounded-full',
                      source.is_enabled
                        ? 'bg-emerald-500/10 text-emerald-500'
                        : 'bg-muted text-muted-foreground'
                    )}
                  >
                    {source.is_enabled ? '已启用' : '已禁用'}
                  </span>
                  {source.is_webdav_exposed && (
                    <span className="text-xs px-2 py-0.5 rounded-full bg-primary/10 text-primary">
                      WebDAV
                    </span>
                  )}
                  <div className="flex-1" />
                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      handleTest(source.id)
                    }}
                    className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                    title="测试连接"
                  >
                    <RefreshCw className="w-3.5 h-3.5" />
                  </button>
                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      handleDelete(source.id)
                    }}
                    className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                    title="删除"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <CreateSourceModal
        isOpen={createModalOpen}
        onClose={() => setCreateModalOpen(false)}
        onSuccess={() => queryClient.invalidateQueries({ queryKey: ['sources'] })}
      />
    </div>
  )
}
