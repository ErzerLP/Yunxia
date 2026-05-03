import { useState } from 'react'
import { Loader2, Trash2, X } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { useUIStore } from '@/stores/uiStore'
import type { VFSItem } from '@/types/api'

interface VFSSelectionBarProps {
  selectedItems: VFSItem[]
  onClear: () => void
  onRefresh: () => void
}

function getErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : '操作失败'
}

export function VFSSelectionBar({ selectedItems, onClear, onRefresh }: VFSSelectionBarProps) {
  const { addToast } = useUIStore()
  const [isDeleting, setIsDeleting] = useState(false)
  const [error, setError] = useState('')

  const handleBatchDelete = async () => {
    if (selectedItems.length === 0 || isDeleting) return
    const confirmed = confirm(`确定要永久删除已选择的 ${selectedItems.length} 项吗？此操作不可撤销。`)
    if (!confirmed) return

    setIsDeleting(true)
    setError('')
    try {
      const results = await Promise.all(
        selectedItems.map(async (item) => {
          try {
            await fileV2Api.delete({ path: item.path, delete_mode: 'permanent' })
            return { item, success: true, error: '' }
          } catch (err: unknown) {
            return { item, success: false, error: getErrorMessage(err) }
          }
        })
      )

      const succeeded = results.filter((result) => result.success)
      const failed = results.filter((result) => !result.success)
      const summary = `批量删除完成：成功 ${succeeded.length}，失败 ${failed.length}`

      if (failed.length > 0) {
        const detail = failed
          .slice(0, 3)
          .map((result) => `${result.item.name}：${result.error}`)
          .join('；')
        setError(`${summary}${detail ? `。${detail}` : ''}`)
        addToast(summary, 'warning', 7000)
      } else {
        addToast(summary, 'success')
      }

      onRefresh()
      onClear()
    } finally {
      setIsDeleting(false)
    }
  }

  if (selectedItems.length === 0) return null

  return (
    <div className="border-b border-border bg-primary/5 shrink-0">
      <div className="flex items-center gap-2 px-4 h-10">
        <span className="text-sm text-primary font-medium">已选择 {selectedItems.length} 项</span>
        <div className="flex-1" />
        <button
          type="button"
          onClick={() => void handleBatchDelete()}
          disabled={isDeleting}
          className="flex items-center gap-1 px-2.5 py-1 rounded-md text-xs font-medium text-destructive hover:bg-destructive/10 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {isDeleting ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Trash2 className="w-3.5 h-3.5" />}
          批量删除
        </button>
        <button
          type="button"
          onClick={onClear}
          disabled={isDeleting}
          className="flex items-center gap-1 px-2.5 py-1 rounded-md text-xs text-muted-foreground hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          <X className="w-3.5 h-3.5" />
          取消选择
        </button>
      </div>
      {error && (
        <div className="px-4 pb-2 text-xs text-destructive break-all">
          {error}
        </div>
      )}
    </div>
  )
}
