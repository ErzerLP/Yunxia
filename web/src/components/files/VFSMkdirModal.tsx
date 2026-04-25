import { useState, useEffect, useRef } from 'react'
import { X, FolderPlus } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { useUIStore } from '@/stores/uiStore'
import { cn } from '@/utils'

interface VFSMkdirModalProps {
  isOpen: boolean
  onClose: () => void
  parentPath: string
  onSuccess?: () => void
}

export function VFSMkdirModal({ isOpen, onClose, parentPath, onSuccess }: VFSMkdirModalProps) {
  const [name, setName] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const { addToast } = useUIStore()

  useEffect(() => {
    if (isOpen) {
      setName('')
      setError(null)
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [isOpen])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) {
      onClose()
      return
    }

    setIsSubmitting(true)
    setError(null)
    try {
      await fileV2Api.mkdir({ parent_path: parentPath, name: trimmed })
      addToast('文件夹创建成功', 'success')
      onSuccess?.()
      onClose()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '创建失败'
      setError(msg)
      addToast(msg, 'error')
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
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <FolderPlus className="w-4 h-4" />
            新建文件夹
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-4">
          <input
            ref={inputRef}
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            placeholder="文件夹名称"
          />
          {error && <p className="text-sm text-destructive mt-2">{error}</p>}
          <div className="flex justify-end gap-2 mt-4">
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
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium transition-colors',
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
