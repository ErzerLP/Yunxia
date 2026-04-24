import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Folder, FileText, Image, Film, Music, File } from 'lucide-react'
import { fileApi } from '@/api/file'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { formatBytes, getFileIconClass, cn } from '@/utils'
import { FileContextMenu } from './FileContextMenu'
import { RenameModal } from './RenameModal'
import { DeleteConfirmModal } from './DeleteConfirmModal'
import { MoveCopyModal } from './MoveCopyModal'
import type { FileItem } from '@/types/api'

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

function FileIcon({ item, className }: { item: FileItem; className?: string }) {
  const type = getFileIconClass(item.mime_type, item.is_dir)
  const Icon = iconMap[type as keyof typeof iconMap] || File
  return (
    <Icon
      className={cn(
        'shrink-0',
        item.is_dir ? 'text-primary' : 'text-muted-foreground',
        className
      )}
    />
  )
}

export function FileGrid() {
  const { currentSource, currentPath, files, setFiles, setLoading, selectedFiles, navigateTo } = useFileStore()
  const { openPreview } = useUIStore()
  const queryClient = useQueryClient()
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; item: FileItem } | null>(null)
  const [renameTarget, setRenameTarget] = useState<FileItem | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<FileItem | null>(null)
  const [moveCopyTarget, setMoveCopyTarget] = useState<{ item: FileItem; mode: 'move' | 'copy' } | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['files', currentSource?.id, currentPath],
    queryFn: () =>
      fileApi.list({
        source_id: currentSource?.id || 0,
        path: currentPath,
        page: 1,
        page_size: 100,
      }),
    enabled: !!currentSource,
  })

  useEffect(() => {
    if (data) {
      setFiles(data.items)
    }
  }, [data, setFiles])

  useEffect(() => {
    setLoading(isLoading)
  }, [isLoading, setLoading])

  const handleClick = (item: FileItem) => {
    if (item.is_dir) {
      navigateTo(item.path)
    } else {
      openPreview(item)
    }
  }

  const handleContextMenu = (e: React.MouseEvent, item: FileItem) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, item })
  }

  const handleDownload = async (item: FileItem) => {
    if (!currentSource) return
    try {
      const res = await fileApi.getAccessUrl({
        source_id: currentSource.id,
        path: item.path,
        purpose: 'download',
        disposition: 'attachment',
      })
      window.open(res.url, '_blank')
    } catch {
      // ignore
    }
  }

  const handleRename = (item: FileItem) => {
    setRenameTarget(item)
    setContextMenu(null)
  }

  const handleDelete = (item: FileItem) => {
    setDeleteTarget(item)
    setContextMenu(null)
  }

  const handleMove = (item: FileItem) => {
    setMoveCopyTarget({ item, mode: 'move' })
    setContextMenu(null)
  }

  const handleCopy = (item: FileItem) => {
    setMoveCopyTarget({ item, mode: 'copy' })
    setContextMenu(null)
  }

  const refreshFiles = () => {
    queryClient.invalidateQueries({ queryKey: ['files', currentSource?.id, currentPath] })
  }

  if (!currentSource) {
    return (
      <div className="flex-1 flex items-center justify-center text-muted-foreground">
        请选择存储源
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  if (files.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-muted-foreground">
        当前目录为空
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-auto scrollbar-thin p-4">
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
        {files.map((item) => {
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
                  {item.is_dir ? '文件夹' : formatBytes(item.size)}
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
          isDir={contextMenu.item.is_dir}
          onClose={() => setContextMenu(null)}
          onPreview={!contextMenu.item.is_dir ? () => openPreview(contextMenu.item) : undefined}
          onDownload={!contextMenu.item.is_dir ? () => handleDownload(contextMenu.item) : undefined}
          onRename={() => handleRename(contextMenu.item)}
          onCopy={() => handleCopy(contextMenu.item)}
          onMove={() => handleMove(contextMenu.item)}
          onDelete={() => handleDelete(contextMenu.item)}
        />
      )}

      {renameTarget && currentSource && (
        <RenameModal
          isOpen={!!renameTarget}
          onClose={() => setRenameTarget(null)}
          sourceId={currentSource.id}
          path={renameTarget.path}
          currentName={renameTarget.name}
          onSuccess={refreshFiles}
        />
      )}

      {deleteTarget && currentSource && (
        <DeleteConfirmModal
          isOpen={!!deleteTarget}
          onClose={() => setDeleteTarget(null)}
          sourceId={currentSource.id}
          path={deleteTarget.path}
          fileName={deleteTarget.name}
          onSuccess={refreshFiles}
        />
      )}

      {moveCopyTarget && currentSource && (
        <MoveCopyModal
          isOpen={!!moveCopyTarget}
          onClose={() => setMoveCopyTarget(null)}
          mode={moveCopyTarget.mode}
          sourceId={currentSource.id}
          sourcePath={moveCopyTarget.item.path}
          fileName={moveCopyTarget.item.name}
          onSuccess={refreshFiles}
        />
      )}
    </div>
  )
}
