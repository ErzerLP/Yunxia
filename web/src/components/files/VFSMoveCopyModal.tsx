import { useState, useEffect } from 'react'
import { X, Folder, ArrowRight, Copy, FolderInput } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { cn } from '@/utils'

interface VFSMoveCopyModalProps {
  isOpen: boolean
  onClose: () => void
  mode: 'move' | 'copy'
  sourcePath: string
  fileName: string
  onSuccess?: () => void
}

export function VFSMoveCopyModal({
  isOpen,
  onClose,
  mode,
  sourcePath,
  fileName,
  onSuccess,
}: VFSMoveCopyModalProps) {
  const [currentPath, setCurrentPath] = useState('/')
  const [folders, setFolders] = useState<{ name: string; path: string }[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (isOpen) {
      setCurrentPath('/')
      loadFolders('/')
    }
  }, [isOpen])

  const loadFolders = async (path: string) => {
    setIsLoading(true)
    try {
      const res = await fileV2Api.list({ path, page: 1, page_size: 100 })
      const dirs = res.items
        .filter((item) => item.entry_kind === 'directory')
        .map((item) => ({ name: item.name, path: item.path }))
      setFolders(dirs)
    } catch {
      setFolders([])
    } finally {
      setIsLoading(false)
    }
  }

  const navigateTo = (path: string) => {
    setCurrentPath(path)
    loadFolders(path)
  }

  const navigateUp = () => {
    if (currentPath === '/') return
    const parts = currentPath.split('/').filter(Boolean)
    parts.pop()
    const parent = parts.length === 0 ? '/' : '/' + parts.join('/') + '/'
    navigateTo(parent)
  }

  const handleSubmit = async () => {
    if (currentPath === sourcePath) {
      onClose()
      return
    }
    setIsSubmitting(true)
    try {
      if (mode === 'move') {
        await fileV2Api.move({
          path: sourcePath,
          target_path: currentPath,
        })
      } else {
        await fileV2Api.copy({
          path: sourcePath,
          target_path: currentPath,
        })
      }
      onSuccess?.()
      onClose()
    } catch {
      // ignore
    } finally {
      setIsSubmitting(false)
    }
  }

  if (!isOpen) return null

  const Icon = mode === 'move' ? FolderInput : Copy
  const title = mode === 'move' ? '移动到' : '复制到'
  const actionLabel = mode === 'move' ? '移动' : '复制'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-sm bg-card border border-border rounded-lg shadow-xl flex flex-col max-h-[60vh]">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border shrink-0">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Icon className="w-4 h-4" />
            {title}
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="px-4 py-2 border-b border-border shrink-0">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span className="truncate max-w-[120px]">{fileName}</span>
            <ArrowRight className="w-3.5 h-3.5 shrink-0" />
            <span className="truncate">{currentPath}</span>
          </div>
        </div>

        <div className="flex-1 overflow-auto scrollbar-thin p-2">
          {currentPath !== '/' && (
            <button
              onClick={navigateUp}
              className="w-full flex items-center gap-2 px-3 py-2 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
            >
              <Folder className="w-4 h-4" />
              <span>..</span>
            </button>
          )}

          {isLoading ? (
            <div className="flex items-center justify-center py-4">
              <div className="w-5 h-5 border-2 border-primary border-t-transparent rounded-full animate-spin" />
            </div>
          ) : folders.length === 0 ? (
            <div className="text-sm text-muted-foreground text-center py-4">当前目录无子文件夹</div>
          ) : (
            folders.map((folder) => (
              <button
                key={folder.path}
                onClick={() => navigateTo(folder.path)}
                className="w-full flex items-center gap-2 px-3 py-2 rounded-md text-sm text-foreground hover:bg-accent transition-colors"
              >
                <Folder className="w-4 h-4 text-primary" />
                <span className="truncate">{folder.name}</span>
              </button>
            ))
          )}
        </div>

        <div className="flex items-center justify-end gap-2 px-4 h-14 border-t border-border shrink-0">
          <button
            onClick={onClose}
            className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
          >
            取消
          </button>
          <button
            onClick={handleSubmit}
            disabled={isSubmitting || currentPath === sourcePath}
            className={cn(
              'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
              (isSubmitting || currentPath === sourcePath) && 'opacity-50 cursor-not-allowed'
            )}
          >
            {isSubmitting ? '处理中...' : actionLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
