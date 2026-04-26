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
import { useAuthStore } from '@/stores/authStore'
import { fileApi } from '@/api/file'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { SourceSelector } from './SourceSelector'
import { MkdirModal } from './MkdirModal'
import { cn } from '@/utils'

export function FileToolbar() {
  const { currentSource, currentPath, currentPermissions, viewMode, setViewMode, navigateUp, setFiles } = useFileStore()
  const { setUploadModalOpen } = useUIStore()
  const { user } = useAuthStore()
  const queryClient = useQueryClient()
  const [searchQuery, setSearchQuery] = useState('')
  const [showSearch, setShowSearch] = useState(false)
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)

  const canGoUp = currentPath !== '/'
  const canWriteCurrentDirectory =
    user?.role_key === 'super_admin' ||
    user?.role_key === 'admin' ||
    currentPermissions?.write === true

  const { refetch } = useQuery({
    queryKey: ['files-search', currentSource?.id, searchQuery],
    queryFn: () =>
      fileApi.search({
        source_id: currentSource?.id || 0,
        keyword: searchQuery,
        path_prefix: currentPath,
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
    if (!searchQuery.trim() || !currentSource) return
    const res = await refetch()
    if (res.data?.items) {
      setFiles(res.data.items)
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
    if (currentSource) {
      queryClient.invalidateQueries({ queryKey: ['files', currentSource.id, currentPath] })
    }
  }

  return (
    <div className="flex items-center gap-2 px-4 h-14 border-b border-border shrink-0">
      <SourceSelector />

      <div className="w-px h-5 bg-border mx-1" />

      <button
        onClick={navigateUp}
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

      {canWriteCurrentDirectory && (
        <button
          onClick={() => setUploadModalOpen(true)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
        >
          <Upload className="w-4 h-4" />
          <span>上传</span>
        </button>
      )}

      {canWriteCurrentDirectory && (
        <button
          onClick={() => setMkdirOpen(true)}
          disabled={!currentSource}
          className={cn(
            'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors',
            currentSource
              ? 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
              : 'text-muted-foreground/30 cursor-not-allowed'
          )}
        >
          <FolderPlus className="w-4 h-4" />
          <span>新建文件夹</span>
        </button>
      )}

      <button
        onClick={() => {
          if (currentSource) {
            queryClient.invalidateQueries({ queryKey: ['files', currentSource.id, currentPath] })
          }
        }}
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

      {mkdirOpen && currentSource && (
        <MkdirModal
          isOpen={mkdirOpen}
          onClose={() => setMkdirOpen(false)}
          sourceId={currentSource.id}
          parentPath={currentPath}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['files', currentSource.id, currentPath] })
          }}
        />
      )}
    </div>
  )
}
