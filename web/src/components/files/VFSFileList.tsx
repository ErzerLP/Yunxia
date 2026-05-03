import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Folder, FileText, Image, Film, Music, File, MoreHorizontal } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { shareApi } from '@/api/share'
import { sourceApi } from '@/api/source'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { formatBytes, formatDate, getFileIconClass } from '@/utils'
import { cn } from '@/utils'
import { buildVfsShareRequest, toFrontendShareLink } from '@/utils/vfs'
import { FileContextMenu } from './FileContextMenu'
import { VFSRenameModal } from './VFSRenameModal'
import { VFSDeleteConfirmModal } from './VFSDeleteConfirmModal'
import { VFSMoveCopyModal } from './VFSMoveCopyModal'
import { VFSSelectionBar } from './VFSSelectionBar'
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

function FileIcon({ item }: { item: VFSItem }) {
  const type = getFileIconClass(item.mime_type, item.entry_kind === 'directory')
  const Icon = iconMap[type as keyof typeof iconMap] || File
  return (
    <Icon
      className={cn(
        'w-5 h-5 shrink-0',
        item.entry_kind === 'directory' ? 'text-primary' : 'text-muted-foreground'
      )}
    />
  )
}

export function VFSFileList() {
  const { currentVirtualPath, vfsItems, setVfsItems, setCurrentPermissions, setLoading, toggleSelection, selectAll, clearSelection, selectedFiles, navigateVirtualTo } = useFileStore()
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
    refetchOnMount: 'always',
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

  const displayedVfsItems = data?.items ?? vfsItems
  const selectedVfsItems = displayedVfsItems.filter((item) => selectedFiles.has(item.path))

  const handleClick = (item: VFSItem) => {
    if (selectedFiles.size > 0) {
      toggleSelection(item.path)
      return
    }
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

  if (displayedVfsItems.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-muted-foreground">
        当前目录为空
      </div>
    )
  }

  return (
    <>
      <VFSSelectionBar
        selectedItems={selectedVfsItems}
        onClear={clearSelection}
        onRefresh={refreshFiles}
      />
      <div className="flex-1 overflow-auto scrollbar-thin">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-background z-10">
            <tr className="border-b border-border text-muted-foreground">
              <th className="w-10 px-4 py-2 text-left">
                <input
                  aria-label="选择当前目录全部条目"
                  type="checkbox"
                  name="select_all_vfs_items"
                  className="rounded border-border"
                  checked={selectedVfsItems.length === displayedVfsItems.length && displayedVfsItems.length > 0}
                  onChange={(e) => {
                    if (e.target.checked) {
                      selectAll(displayedVfsItems.map((f) => f.path))
                    } else {
                      clearSelection()
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
            {displayedVfsItems.map((item) => {
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
                      aria-label={`选择 ${item.name}`}
                      type="checkbox"
                      name="selected_vfs_item"
                      className="rounded border-border"
                      checked={selected}
                      onChange={() => toggleSelection(item.path)}
                    />
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2.5">
                      <FileIcon item={item} />
                      <span className="text-foreground truncate max-w-[300px]">{item.name}</span>
                      {item.is_mount_point && (
                        <span className="text-xs px-1.5 py-0.5 rounded bg-primary/10 text-primary">挂载点</span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-2.5 text-muted-foreground">
                    {item.entry_kind === 'directory' ? '-' : formatBytes(item.size)}
                  </td>
                  <td className="px-4 py-2.5 text-muted-foreground">
                    {formatDate(item.modified_at)}
                  </td>
                  <td className="px-4 py-2.5">
                    <button
                      type="button"
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
    </>
  )
}
