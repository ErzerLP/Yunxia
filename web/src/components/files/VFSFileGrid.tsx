import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Folder, FileText, Image, Film, Music, File } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { shareApi } from '@/api/share'
import { sourceApi } from '@/api/source'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { formatBytes, getFileIconClass, cn } from '@/utils'
import { buildVfsShareRequest, toFrontendShareLink } from '@/utils/vfs'
import { FileContextMenu } from './FileContextMenu'
import { VFSRenameModal } from './VFSRenameModal'
import { VFSDeleteConfirmModal } from './VFSDeleteConfirmModal'
import { VFSMoveCopyModal } from './VFSMoveCopyModal'
import type { VFSItem } from '@/types/api'

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

function FileIcon({ item, className }: { item: VFSItem; className?: string }) {
  const type = getFileIconClass(item.mime_type, item.entry_kind === 'directory')
  const Icon = iconMap[type as keyof typeof iconMap] || File
  return (
    <Icon
      className={cn(
        'shrink-0',
        item.entry_kind === 'directory' ? 'text-primary' : 'text-muted-foreground',
        className
      )}
    />
  )
}

export function VFSFileGrid() {
  const { currentVirtualPath, vfsItems, setVfsItems, setCurrentPermissions, setLoading, selectedFiles, navigateVirtualTo } = useFileStore()
  const { openPreview, addToast } = useUIStore()
  const queryClient = useQueryClient()
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; item: VFSItem } | null>(null)
  const [renameTarget, setRenameTarget] = useState<VFSItem | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<VFSItem | null>(null)
  const [moveCopyTarget, setMoveCopyTarget] = useState<{ item: VFSItem; mode: 'move' | 'copy' } | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['vfs', currentVirtualPath],
    queryFn: () =>
      fileV2Api.list({
        path: currentVirtualPath,
        page: 1,
        page_size: 100,
      }),
  })

  const { data: sourcesData } = useQuery({
    queryKey: ['sources-vfs-share'],
    queryFn: () => sourceApi.list({ view: 'navigation' }),
  })

  useEffect(() => {
    if (data) {
      setVfsItems(data.items)
      setCurrentPermissions(data.current_permissions ?? null)
    }
  }, [data, setVfsItems, setCurrentPermissions])

  useEffect(() => {
    setLoading(isLoading)
  }, [isLoading, setLoading])

  const handleClick = (item: VFSItem) => {
    if (item.entry_kind === 'directory') {
      navigateVirtualTo(item.path)
    } else {
      openPreview({
        path: item.path,
        source_id: item.source_id,
        name: item.name,
        mime_type: item.mime_type,
        mode: 'v2',
      })
    }
  }

  const handleContextMenu = (e: React.MouseEvent, item: VFSItem) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, item })
  }

  const handleDownload = async (item: VFSItem) => {
    try {
      const res = await fileV2Api.accessUrl({
        path: item.path,
        purpose: 'download',
        disposition: 'attachment',
      })
      window.open(res.url, '_blank')
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '获取下载链接失败'
      addToast(msg, 'error')
    }
  }

  const handleShare = async (item: VFSItem) => {
    const payload = buildVfsShareRequest(item, sourcesData?.items || [])
    if (!payload) {
      addToast('无法直接分享纯虚拟目录，请进入具体挂载点后再分享', 'error')
      return
    }

    try {
      const res = await shareApi.create(payload)
      const link = toFrontendShareLink(res.share.link)
      await navigator.clipboard.writeText(link)
      addToast('分享链接已创建并复制', 'success')
      queryClient.invalidateQueries({ queryKey: ['shares'] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '创建分享失败'
      addToast(msg, 'error')
    }
  }

  const handleRename = (item: VFSItem) => {
    setRenameTarget(item)
    setContextMenu(null)
  }

  const handleDelete = (item: VFSItem) => {
    setDeleteTarget(item)
    setContextMenu(null)
  }

  const handleMove = (item: VFSItem) => {
    setMoveCopyTarget({ item, mode: 'move' })
    setContextMenu(null)
  }

  const handleCopy = (item: VFSItem) => {
    setMoveCopyTarget({ item, mode: 'copy' })
    setContextMenu(null)
  }

  const refreshFiles = () => {
    queryClient.invalidateQueries({ queryKey: ['vfs', currentVirtualPath] })
  }

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  if (vfsItems.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-muted-foreground">
        当前目录为空
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-auto scrollbar-thin p-4">
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
        {vfsItems.map((item) => {
          const selected = selectedFiles.has(item.path)
          return (
            <div
              key={item.path}
              className={cn(
                'group flex flex-col items-center gap-2 p-3 rounded-lg border transition-all cursor-pointer',
                selected
                  ? 'bg-primary/5 border-primary/30'
                  : 'bg-card border-border hover:border-primary/30 hover:shadow-sm'
              )}
              onClick={() => handleClick(item)}
              onContextMenu={(e) => handleContextMenu(e, item)}
            >
              <div className="relative w-full aspect-square flex items-center justify-center bg-muted/50 rounded-md">
                <FileIcon item={item} className="w-12 h-12" />
                {selected && (
                  <div className="absolute top-1.5 left-1.5 w-4 h-4 rounded-full bg-primary flex items-center justify-center">
                    <svg className="w-2.5 h-2.5 text-primary-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  </div>
                )}
              </div>
              <div className="w-full text-center">
                <p className="text-sm text-foreground truncate">{item.name}</p>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {item.entry_kind === 'directory' ? '文件夹' : formatBytes(item.size)}
                </p>
              </div>
            </div>
          )
        })}
      </div>

      {contextMenu && (
        <FileContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          fileName={contextMenu.item.name}
          isDir={contextMenu.item.entry_kind === 'directory'}
          onClose={() => setContextMenu(null)}
          onPreview={contextMenu.item.entry_kind === 'file' ? () => openPreview({
            path: contextMenu.item.path,
            source_id: contextMenu.item.source_id,
            name: contextMenu.item.name,
            mime_type: contextMenu.item.mime_type,
            mode: 'v2',
          }) : undefined}
          onDownload={contextMenu.item.entry_kind === 'file' ? () => handleDownload(contextMenu.item) : undefined}
          onRename={() => handleRename(contextMenu.item)}
          onCopy={() => handleCopy(contextMenu.item)}
          onMove={() => handleMove(contextMenu.item)}
          onShare={() => handleShare(contextMenu.item)}
          onDelete={() => handleDelete(contextMenu.item)}
        />
      )}

      {renameTarget && (
        <VFSRenameModal
          isOpen={!!renameTarget}
          onClose={() => setRenameTarget(null)}
          path={renameTarget.path}
          currentName={renameTarget.name}
          onSuccess={refreshFiles}
        />
      )}

      {deleteTarget && (
        <VFSDeleteConfirmModal
          isOpen={!!deleteTarget}
          onClose={() => setDeleteTarget(null)}
          path={deleteTarget.path}
          fileName={deleteTarget.name}
          onSuccess={refreshFiles}
        />
      )}

      {moveCopyTarget && (
        <VFSMoveCopyModal
          isOpen={!!moveCopyTarget}
          onClose={() => setMoveCopyTarget(null)}
          mode={moveCopyTarget.mode}
          sourcePath={moveCopyTarget.item.path}
          fileName={moveCopyTarget.item.name}
          onSuccess={refreshFiles}
        />
      )}
    </div>
  )
}
