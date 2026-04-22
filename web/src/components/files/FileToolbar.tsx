import { useState } from 'react'
import {
  Upload,
  FolderPlus,
  RefreshCw,
  Grid3X3,
  List,
  ArrowUp,
  Search,
} from 'lucide-react'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { SourceSelector } from './SourceSelector'
import { MkdirModal } from './MkdirModal'
import { cn } from '@/utils'

export function FileToolbar() {
  const { currentSource, currentPath, viewMode, setViewMode, navigateUp } = useFileStore()
  const { setUploadModalOpen } = useUIStore()
  const [searchQuery, setSearchQuery] = useState('')
  const [showSearch, setShowSearch] = useState(false)
  const [mkdirOpen, setMkdirOpen] = useState(false)

  const canGoUp = currentPath !== '/'

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

      <button
        onClick={() => setUploadModalOpen(true)}
        className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
      >
        <Upload className="w-4 h-4" />
        <span>上传</span>
      </button>

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

      <button
        onClick={() => window.location.reload()}
        className="p-2 rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
        title="刷新"
      >
        <RefreshCw className="w-4 h-4" />
      </button>

      <div className="flex-1" />

      {showSearch && (
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="搜索文件..."
            className="w-48 pl-8 pr-3 py-1.5 text-sm rounded-md border border-input bg-background text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            autoFocus
            onBlur={() => !searchQuery && setShowSearch(false)}
          />
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
          onSuccess={() => window.location.reload()}
        />
      )}
    </div>
  )
}
