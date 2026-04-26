import { useEffect, useRef } from 'react'
import {
  Eye,
  Download,
  Pencil,
  Copy,
  Trash2,
  FolderInput,
  Link,
} from 'lucide-react'
import { cn } from '@/utils'

interface MenuItem {
  id: string
  label: string
  icon: React.ElementType
  danger?: boolean
  onClick: () => void
}

interface FileContextMenuProps {
  x: number
  y: number
  fileName: string
  isDir: boolean
  onClose: () => void
  onPreview?: () => void
  onDownload?: () => void
  onRename?: () => void
  onCopy?: () => void
  onMove?: () => void
  onShare?: () => void
  onDelete?: () => void
}

export function FileContextMenu({
  x,
  y,
  fileName,
  isDir,
  onClose,
  onPreview,
  onDownload,
  onRename,
  onCopy,
  onMove,
  onShare,
  onDelete,
}: FileContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose()
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [onClose])

  const items: MenuItem[] = [
    ...(onPreview && !isDir
      ? [{ id: 'preview', label: '预览', icon: Eye, onClick: onPreview }]
      : []),
    ...(onDownload && !isDir
      ? [{ id: 'download', label: '下载', icon: Download, onClick: onDownload }]
      : []),
    ...(onPreview || onDownload ? [{ id: 'sep1', label: '', icon: () => null, onClick: () => {} }] : []),
    ...(onRename
      ? [{ id: 'rename', label: '重命名', icon: Pencil, onClick: onRename }]
      : []),
    ...(onCopy
      ? [{ id: 'copy', label: '复制到', icon: Copy, onClick: onCopy }]
      : []),
    ...(onMove
      ? [{ id: 'move', label: '移动到', icon: FolderInput, onClick: onMove }]
      : []),
    ...(onShare
      ? [{ id: 'share', label: '分享并复制链接', icon: Link, onClick: onShare }]
      : []),
    ...(onCopy || onMove || onShare ? [{ id: 'sep2', label: '', icon: () => null, onClick: () => {} }] : []),
    ...(onDelete
      ? [{
          id: 'delete',
          label: '删除',
          icon: Trash2,
          danger: true,
          onClick: onDelete,
        }]
      : []),
  ]

  // Adjust position to keep menu in viewport
  const menuWidth = 180
  const menuHeight = items.length * 36 + 8
  const adjustedX = Math.min(x, window.innerWidth - menuWidth - 8)
  const adjustedY = Math.min(y, window.innerHeight - menuHeight - 8)

  return (
    <div
      ref={ref}
      className="fixed z-50 w-44 bg-card border border-border rounded-lg shadow-lg py-1"
      style={{ left: adjustedX, top: adjustedY }}
    >
      <div className="px-3 py-1.5 text-xs text-muted-foreground truncate border-b border-border mb-1">
        {fileName}
      </div>
      {items.map((item) =>
        item.id.startsWith('sep') ? (
          <div key={item.id} className="my-1 border-t border-border" />
        ) : (
          <button
            key={item.id}
            onClick={() => {
              item.onClick()
              onClose()
            }}
            className={cn(
              'w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors',
              item.danger
                ? 'text-destructive hover:bg-destructive/10'
                : 'text-foreground hover:bg-accent'
            )}
          >
            <item.icon className="w-4 h-4 shrink-0" />
            <span>{item.label}</span>
          </button>
        )
      )}
    </div>
  )
}
