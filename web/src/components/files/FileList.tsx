import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Folder, FileText, Image, Film, Music, File, MoreHorizontal, Trash2 } from 'lucide-react'
import { fileApi } from '@/api/file'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { formatBytes, formatDate, getFileIconClass } from '@/utils'
import { cn } from '@/utils'
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

function FileIcon({ item }: { item: FileItem }) {
  const type = getFileIconClass(item.mime_type, item.is_dir)
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

export function FileList() {
  const { currentSource, currentPath, files, setFiles, setCurrentPermissions, setLoading, toggleSelection, selectedFiles, navigateTo } = useFileStore()
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
      setCurrentPermissions(data.current_permissions ?? null)
    }
  }, [data, setFiles, setCurrentPermissions])

  useEffect(() => {
    setLoading(isLoading)
  }, [isLoading, setLoading])

  const displayedFiles = data?.items ?? files

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

  if (displayedFiles.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-muted-foreground">
        当前目录为空
      </div>
    )
  }

  const hasSelection = selectedFiles.size > 0

  return (
    <>
      {hasSelection && (
        <div className="flex items-center gap-2 px-4 h-10 border-b border-border bg-primary/5 shrink-0">
          <span className="text-sm text-primary font-medium">已选择 {selectedFiles.size} 项</span>
          <div className="flex-1" />
          <button
            onClick={() => {
              if (!currentSource) return
              const paths = Array.from(selectedFiles)
              // Batch delete not supported by API, delete one by one
              Promise.all(
                paths.map((path) =>
                  fileApi.delete({ source_id: currentSource.id, path, delete_mode: 'permanent' })
                )
              ).then(() => {
                refreshFiles()
                useFileStore.getState().clearSelection()
              })
            }}
            className="flex items-center gap-1 px-2.5 py-1 rounded-md text-xs font-medium text-destructive hover:bg-destructive/10 transition-colors"
          >
            <Trash2 className="w-3.5 h-3.5" />
            批量删除
          </button>
          <button
            onClick={() => useFileStore.getState().clearSelection()}
            className="px-2.5 py-1 rounded-md text-xs text-muted-foreground hover:bg-accent transition-colors"
          >
            取消选择
          </button>
        </div>
      )}
      <div className="flex-1 overflow-auto scrollbar-thin">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-background z-10">
            <tr className="border-b border-border text-muted-foreground">
              <th className="w-10 px-4 py-2 text-left">
                <input
                  type="checkbox"
                  className="rounded border-border"
                  checked={selectedFiles.size === displayedFiles.length && displayedFiles.length > 0}
                  onChange={(e) => {
                    if (e.target.checked) {
                      useFileStore.getState().selectAll(displayedFiles.map((f) => f.path))
                    } else {
                      useFileStore.getState().clearSelection()
                    }
                  }}
                />
              </th>
              <th className="px-4 py-2 text-left font-medium">名称</th>
              <th className="px-4 py-2 text-left font-medium w-28">大小</th>
              <th className="px-4 py-2 text-left font-medium w-40">修改时间</th>
              <th className="w-10 px-4 py-2" />
            </tr>
          </thead>
          <tbody>
            {displayedFiles.map((item) => {
              const selected = selectedFiles.has(item.path)
              return (
                <tr
                  key={item.path}
                  className={cn(
                    'border-b border-border/50 transition-colors cursor-pointer',
                    selected ? 'bg-primary/5' : 'hover:bg-accent/50'
                  )}
                  onClick={() => handleClick(item)}
                  onContextMenu={(e) => handleContextMenu(e, item)}
                >
                  <td className="px-4 py-2.5" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      className="rounded border-border"
                      checked={selected}
                      onChange={() => toggleSelection(item.path)}
                    />
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2.5">
                      <FileIcon item={item} />
                      <span className="text-foreground truncate max-w-[300px]">{item.name}</span>
                    </div>
                  </td>
                  <td className="px-4 py-2.5 text-muted-foreground">
                    {item.is_dir ? '-' : formatBytes(item.size)}
                  </td>
                  <td className="px-4 py-2.5 text-muted-foreground">
                    {formatDate(item.modified_at)}
                  </td>
                  <td className="px-4 py-2.5">
                    <button
                      className="p-1 rounded hover:bg-accent text-muted-foreground"
                      onClick={(e) => {
                        e.stopPropagation()
                        handleContextMenu(e, item)
                      }}
                    >
                      <MoreHorizontal className="w-4 h-4" />
                    </button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
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
    </>
  )
}
