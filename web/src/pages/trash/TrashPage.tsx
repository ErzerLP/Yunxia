import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { trashApi } from '@/api/trash'
import { useUIStore } from '@/stores/uiStore'
import { Trash2, RotateCcw, Folder, FileText, Image, Film, Music, File } from 'lucide-react'
import { formatBytes, formatDate, getFileIconClass } from '@/utils'
import { cn } from '@/utils'
import type { TrashItem } from '@/types/api'

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

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data, isLoading } = useQuery({
    queryKey: ['trash'],
    queryFn: () => trashApi.list({ page: 1, page_size: 100 }),
  })

  const handleRestore = async (id: number) => {
    try {
      await trashApi.restore(id)
      addToast('文件已恢复', 'success')
      queryClient.invalidateQueries({ queryKey: ['trash'] })
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
      queryClient.invalidateQueries({ queryKey: ['trash'] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '删除失败'
      addToast(msg, 'error')
    }
  }

  const handleClear = async () => {
    if (!confirm('确定要清空回收站吗？所有文件将被永久删除，此操作不可撤销。')) return
    try {
      await trashApi.clear()
      addToast('回收站已清空', 'success')
      queryClient.invalidateQueries({ queryKey: ['trash'] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '清空失败'
      addToast(msg, 'error')
    }
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const items = data?.items || []

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">回收站</h1>
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
        {items.length === 0 ? (
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
