import { useEffect, useState, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { trashApi } from '@/api/trash'
import { sourceApi } from '@/api/source'
import { useUIStore } from '@/stores/uiStore'
import { Trash2, RotateCcw, Folder, FileText, Image, Film, Music, File, HardDrive, ChevronDown, CheckCircle2 } from 'lucide-react'
import { formatBytes, formatDate, getFileIconClass } from '@/utils'
import { cn } from '@/utils'
import type { TrashItem, StorageSource } from '@/types/api'

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

function TrashIcon({ item }: { item: TrashItem }) {
  const type = getFileIconClass(item.is_dir ? '' : '', item.is_dir)
  const Icon = iconMap[type as keyof typeof iconMap] || File
  return (
    <Icon
      className={cn(
        'w-5 h-5 shrink-0',
        item.is_dir ? 'text-primary' : 'text-muted-foreground'
      )}
    />
  )
}

export function TrashPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const { addToast } = useUIStore()
  const [currentSource, setCurrentSource] = useState<StorageSource | null>(null)
  const [sourceOpen, setSourceOpen] = useState(false)
  const sourceRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (sourceRef.current && !sourceRef.current.contains(e.target as Node)) {
        setSourceOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  const { data: sourcesData } = useQuery({
    queryKey: ['sources-trash'],
    queryFn: () => sourceApi.list({ page: 1, page_size: 100, view: 'navigation' }),
  })

  const { data, isLoading, error } = useQuery({
    queryKey: ['trash', currentSource?.id],
    queryFn: () =>
      trashApi.list({
        source_id: currentSource!.id,
        page: 1,
        page_size: 100,
      }),
    enabled: !!currentSource,
  })

  const handleRestore = async (id: number) => {
    try {
      await trashApi.restore(id)
      addToast('文件已恢复', 'success')
      if (currentSource) {
        queryClient.invalidateQueries({ queryKey: ['trash', currentSource.id] })
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '恢复失败'
      addToast(msg, 'error')
    }
  }

  const handleDelete = async (id: number, name: string) => {
    if (!confirm(`确定要永久删除 "${name}" 吗？此操作不可撤销。`)) return
    try {
      await trashApi.delete(id)
      addToast('文件已永久删除', 'success')
      if (currentSource) {
        queryClient.invalidateQueries({ queryKey: ['trash', currentSource.id] })
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '删除失败'
      addToast(msg, 'error')
    }
  }

  const handleClear = async () => {
    if (!currentSource) return
    if (!confirm('确定要清空回收站吗？所有文件将被永久删除，此操作不可撤销。')) return
    try {
      await trashApi.clear(currentSource.id)
      addToast('回收站已清空', 'success')
      queryClient.invalidateQueries({ queryKey: ['trash', currentSource.id] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '清空失败'
      addToast(msg, 'error')
    }
  }

  if (authLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const items = data?.items || []
  const sources = sourcesData?.items || []

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold text-foreground">回收站</h1>
          <div ref={sourceRef} className="relative">
            <button
              onClick={() => setSourceOpen(!sourceOpen)}
              className="flex items-center gap-2 px-3 py-1.5 rounded-md border border-border bg-card text-sm hover:border-primary/30 transition-colors"
            >
              <HardDrive className="w-4 h-4 text-primary" />
              <span className="text-foreground">{currentSource?.name || '选择存储源'}</span>
              <ChevronDown className={cn('w-3.5 h-3.5 text-muted-foreground transition-transform', sourceOpen && 'rotate-180')} />
            </button>
            {sourceOpen && (
              <div className="absolute top-full left-0 mt-1 w-56 bg-card border border-border rounded-lg shadow-lg z-50 py-1">
                {sources.length === 0 ? (
                  <div className="px-3 py-2 text-sm text-muted-foreground">暂无存储源</div>
                ) : (
                  sources.map((source) => (
                    <button
                      key={source.id}
                      onClick={() => {
                        setCurrentSource(source)
                        setSourceOpen(false)
                      }}
                      className={cn(
                        'w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors',
                        currentSource?.id === source.id
                          ? 'bg-primary/5 text-primary'
                          : 'text-foreground hover:bg-accent'
                      )}
                    >
                      <HardDrive className="w-4 h-4 shrink-0" />
                      <span className="flex-1 text-left truncate">{source.name}</span>
                      {currentSource?.id === source.id && <CheckCircle2 className="w-3.5 h-3.5 shrink-0" />}
                    </button>
                  ))
                )}
              </div>
            )}
          </div>
        </div>
        {items.length > 0 && (
          <button
            onClick={handleClear}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-destructive text-sm font-medium hover:bg-destructive/10 transition-colors"
          >
            <Trash2 className="w-4 h-4" />
            <span>清空回收站</span>
          </button>
        )}
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {!currentSource ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <HardDrive className="w-12 h-12 opacity-30" />
            <p>请选择存储源</p>
          </div>
        ) : isLoading ? (
          <div className="flex-1 flex items-center justify-center">
            <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Trash2 className="w-12 h-12 opacity-30" />
            <p className="text-destructive">{(error as Error).message || '加载失败'}</p>
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Trash2 className="w-12 h-12 opacity-30" />
            <p>回收站为空</p>
          </div>
        ) : (
          <div className="space-y-2">
            {items.map((item) => (
              <div
                key={item.id}
                className="flex items-center gap-3 p-3 rounded-lg border border-border bg-card"
              >
                <TrashIcon item={item} />
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-foreground truncate">{item.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {item.is_dir ? '文件夹' : formatBytes(item.size)} · 删除于 {formatDate(item.deleted_at)}
                  </p>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  <button
                    onClick={() => handleRestore(item.id)}
                    className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                    title="恢复"
                  >
                    <RotateCcw className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => handleDelete(item.id, item.name)}
                    className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                    title="永久删除"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
