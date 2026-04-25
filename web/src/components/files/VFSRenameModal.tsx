import { useState, useEffect, useRef } from 'react'
import { X, Pencil } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { cn } from '@/utils'

interface VFSRenameModalProps {
  isOpen: boolean
  onClose: () => void
  path: string
  currentName: string
  onSuccess?: () => void
}

export function VFSRenameModal({ isOpen, onClose, path, currentName, onSuccess }: VFSRenameModalProps) {
  const [newName, setNewName] = useState(currentName)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (isOpen) {
      setNewName(currentName)
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [isOpen, currentName])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = newName.trim()
    if (!trimmed || trimmed === currentName) {
      onClose()
      return
    }

    setIsSubmitting(true)
    try {
      await fileV2Api.rename({ path, new_name: trimmed })
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
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Pencil className="w-4 h-4" />
            重命名
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-4">
          <input
            ref={inputRef}
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            placeholder="新名称"
          />
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
              disabled={isSubmitting || !newName.trim() || newName.trim() === currentName}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium transition-colors',
                (isSubmitting || !newName.trim() || newName.trim() === currentName) && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '保存中...' : '确定'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
