import { useState, useRef, useEffect } from 'react'
import {
  Upload,
  FolderPlus,
  RefreshCw,
  Grid3X3,
  List,
  ArrowUp,
  Search,
  X,
} from 'lucide-react'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { fileV2Api } from '@/api/fileV2'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { VFSMkdirModal } from './VFSMkdirModal'
import { cn } from '@/utils'
import { useHasCapability } from '@/hooks/useCapability'

export function VFSFileToolbar() {
  const { currentVirtualPath, viewMode, setViewMode, navigateVirtualUp, setVfsItems } = useFileStore()
  const { setUploadModalOpen } = useUIStore()
  const queryClient = useQueryClient()
  const canWrite = useHasCapability('file.write')
  const [searchQuery, setSearchQuery] = useState('')
  const [showSearch, setShowSearch] = useState(false)
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)

  const canGoUp = currentVirtualPath !== '/'

  const { refetch } = useQuery({
    queryKey: ['vfs-search', searchQuery],
    queryFn: () =>
      fileV2Api.search({
        keyword: searchQuery,
        path_prefix: currentVirtualPath,
        page: 1,
        page_size: 100,
      }),
    enabled: false,
  })

  useEffect(() => {
    if (showSearch && searchInputRef.current) {
      searchInputRef.current.focus()
    }
  }, [showSearch])

  const handleSearch = async () => {
    if (!searchQuery.trim()) return
    const res = await refetch()
    if (res.data?.items) {
      setVfsItems(res.data.items)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleSearch()
    }
    if (e.key === 'Escape') {
      setShowSearch(false)
      setSearchQuery('')
    }
  }

  const clearSearch = () => {
    setSearchQuery('')
    setShowSearch(false)
    queryClient.invalidateQueries({ queryKey: ['vfs', currentVirtualPath] })
  }

  return (
    <div className="flex items-center gap-2 px-4 h-14 border-b border-border shrink-0">
      <span className="text-sm font-medium text-foreground px-2">虚拟目录</span>

      <div className="w-px h-5 bg-border mx-1" />

      <button
        onClick={navigateVirtualUp}
        disabled={!canGoUp}
        className={cn(
          'p-2 rounded-md transition-colors',
          canGoUp
            ? 'hover:bg-accent text-muted-foreground hover:text-accent-foreground'
            : 'text-muted-foreground/30 cursor-not-allowed'
        )}
        title="上级目录"
      >
        <ArrowUp className="w-4 h-4" />
      </button>

      <div className="w-px h-5 bg-border mx-1" />

      {canWrite && (
        <button
          onClick={() => setUploadModalOpen(true)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
        >
          <Upload className="w-4 h-4" />
          <span>上传</span>
        </button>
      )}

      {canWrite && (
        <button
          onClick={() => setMkdirOpen(true)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
        >
          <FolderPlus className="w-4 h-4" />
          <span>新建文件夹</span>
        </button>
      )}

      <button
        onClick={() => queryClient.invalidateQueries({ queryKey: ['vfs', currentVirtualPath] })}
        className="p-2 rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
        title="刷新"
      >
        <RefreshCw className="w-4 h-4" />
      </button>

      <div className="flex-1" />

      {showSearch && (
        <div className="relative flex items-center gap-1">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input
            ref={searchInputRef}
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="搜索文件..."
            className="w-48 pl-8 pr-7 py-1.5 text-sm rounded-md border border-input bg-background text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
          />
          {searchQuery && (
            <button
              onClick={clearSearch}
              className="absolute right-1.5 top-1/2 -translate-y-1/2 p-0.5 rounded hover:bg-accent text-muted-foreground"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      )}

      {!showSearch && (
        <button
          onClick={() => setShowSearch(true)}
          className="p-2 rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
          title="搜索"
        >
          <Search className="w-4 h-4" />
        </button>
      )}

      <div className="flex items-center rounded-md border border-border overflow-hidden">
        <button
          onClick={() => setViewMode('list')}
          className={cn(
            'p-2 transition-colors',
            viewMode === 'list'
              ? 'bg-accent text-accent-foreground'
              : 'text-muted-foreground hover:text-foreground'
          )}
          title="列表视图"
        >
          <List className="w-4 h-4" />
        </button>
        <button
          onClick={() => setViewMode('grid')}
          className={cn(
            'p-2 transition-colors',
            viewMode === 'grid'
              ? 'bg-accent text-accent-foreground'
              : 'text-muted-foreground hover:text-foreground'
          )}
          title="网格视图"
        >
          <Grid3X3 className="w-4 h-4" />
        </button>
      </div>

      {mkdirOpen && (
        <VFSMkdirModal
          isOpen={mkdirOpen}
          onClose={() => setMkdirOpen(false)}
          parentPath={currentVirtualPath}
          onSuccess={() => queryClient.invalidateQueries({ queryKey: ['vfs', currentVirtualPath] })}
        />
      )}
    </div>
  )
}
