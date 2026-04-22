import { useState } from 'react'
import { X, Trash2, AlertTriangle } from 'lucide-react'
import { fileApi } from '@/api/file'
import { cn } from '@/utils'

interface DeleteConfirmModalProps {
  isOpen: boolean
  onClose: () => void
  sourceId: number
  path: string
  fileName: string
  onSuccess?: () => void
}

export function DeleteConfirmModal({ isOpen, onClose, sourceId, path, fileName, onSuccess }: DeleteConfirmModalProps) {
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleDelete = async () => {
    setIsSubmitting(true)
    try {
      await fileApi.delete({ source_id: sourceId, path, delete_mode: 'permanent' })
      onSuccess?.()
      onClose()
    } catch {
      // ignore
    } finally {
      setIsSubmitting(false)
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-sm bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2 text-destructive">
            <Trash2 className="w-4 h-4" />
            确认删除
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-4">
          <div className="flex items-start gap-3 mb-4">
            <AlertTriangle className="w-5 h-5 text-warning shrink-0 mt-0.5" />
            <div>
              <p className="text-sm text-foreground">
                确定要删除 <span className="font-medium">{fileName}</span> 吗？
              </p>
              <p className="text-xs text-muted-foreground mt-1">此操作不可撤销，文件将被永久删除。</p>
            </div>
          </div>

          <div className="flex justify-end gap-2">
            <button
              onClick={onClose}
              className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
            >
              取消
            </button>
            <button
              onClick={handleDelete}
              disabled={isSubmitting}
              className={cn(
                'px-4 py-1.5 rounded-md bg-destructive text-destructive-foreground text-sm font-medium hover:bg-destructive/90 transition-colors',
                isSubmitting && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '删除中...' : '删除'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
