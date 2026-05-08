import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Folder, FileText, Image, Film, Music, File, Square } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { shareApi } from '@/api/share'
import { sourceApi } from '@/api/source'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { formatBytes, getApiErrorMessage, getFileIconClass, cn } from '@/utils'
import { buildVfsShareRequest, toFrontendShareLink } from '@/utils/vfs'
import { FileContextMenu } from './FileContextMenu'
import { VFSRenameModal } from './VFSRenameModal'
import { VFSDeleteConfirmModal } from './VFSDeleteConfirmModal'
import { VFSMoveCopyModal } from './VFSMoveCopyModal'
import { VFSSelectionBar } from './VFSSelectionBar'
import { VFSSyncStateBadge } from './VFSSyncStateBadge'
import { VFSTagModal } from './VFSTagModal'
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
  const { currentVirtualPath, currentPermissions, vfsItems, setVfsItems, setCurrentPermissions, setLoading, toggleSelection, clearSelection, selectedFiles, navigateVirtualTo } = useFileStore()
  const { openPreview, addToast } = useUIStore()
  const queryClient = useQueryClient()
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; item: VFSItem } | null>(null)
  const [renameTarget, setRenameTarget] = useState<VFSItem | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<VFSItem | null>(null)
  const [moveCopyTarget, setMoveCopyTarget] = useState<{ item: VFSItem; mode: 'move' | 'copy' } | null>(null)
  const [tagTarget, setTagTarget] = useState<VFSItem | null>(null)

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
  const canWriteCurrentDirectory = currentPermissions === null || currentPermissions?.write === true

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
    if (item.can_download === false) {
      addToast('该条目当前不可下载，请刷新目录后重试', 'error')
      return
    }
    try {
      const res = await fileV2Api.accessUrl({
        path: item.path,
        purpose: 'download',
        disposition: 'attachment',
      })
      window.open(res.url, '_blank')
    } catch (err: unknown) {
      const msg = getApiErrorMessage(err, '获取下载链接失败')
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
      const msg = getApiErrorMessage(err, '创建分享失败')
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

  const handleTags = (item: VFSItem) => {
    setTagTarget(item)
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
      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
        {displayedVfsItems.map((item) => {
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
                <button
                  type="button"
                  onClick={(event) => {
                    event.stopPropagation()
                    toggleSelection(item.path)
                  }}
                  className={cn(
                    'absolute top-1.5 left-1.5 flex h-6 w-6 items-center justify-center rounded-md border bg-background/90 shadow-sm transition-colors',
                    selected
                      ? 'border-primary bg-primary text-primary-foreground'
                      : 'border-border text-muted-foreground hover:border-primary hover:text-primary'
                  )}
                  title={selected ? `取消选择 ${item.name}` : `选择 ${item.name}`}
                  aria-label={selected ? `取消选择 ${item.name}` : `选择 ${item.name}`}
                >
                  {selected ? <Check className="w-3.5 h-3.5" /> : <Square className="w-3.5 h-3.5" />}
                </button>
                <FileIcon item={item} className="w-12 h-12" />
              </div>
              <div className="w-full text-center">
                <p className="text-sm text-foreground truncate">{item.name}</p>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {item.entry_kind === 'directory' ? '文件夹' : formatBytes(item.size)}
                </p>
                <div className="mt-1 flex justify-center">
                  <VFSSyncStateBadge
                    syncState={item.sync_state}
                    canDownload={item.entry_kind === 'file' ? item.can_download : true}
                  />
                </div>
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
          onDownload={contextMenu.item.entry_kind === 'file' && contextMenu.item.can_download !== false ? () => handleDownload(contextMenu.item) : undefined}
          onRename={canWriteCurrentDirectory ? () => handleRename(contextMenu.item) : undefined}
          onCopy={canWriteCurrentDirectory ? () => handleCopy(contextMenu.item) : undefined}
          onMove={canWriteCurrentDirectory ? () => handleMove(contextMenu.item) : undefined}
          onShare={() => handleShare(contextMenu.item)}
          onTags={() => handleTags(contextMenu.item)}
          onDelete={contextMenu.item.can_delete ? () => handleDelete(contextMenu.item) : undefined}
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

      {tagTarget && (
        <VFSTagModal
          item={tagTarget}
          onClose={() => setTagTarget(null)}
        />
      )}
      </div>
    </>
  )
}
