import { Home, ChevronRight } from 'lucide-react'
import { useFileStore } from '@/stores/fileStore'

export function FileBreadcrumb() {
  const { currentPath, navigateTo } = useFileStore()

  const parts = currentPath.split('/').filter(Boolean)

  return (
    <div className="flex items-center gap-1 px-4 h-10 border-b border-border shrink-0 text-sm overflow-x-auto scrollbar-thin">
      <button
        onClick={() => navigateTo('/')}
        className="flex items-center gap-1 px-1.5 py-0.5 rounded text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shrink-0"
      >
        <Home className="w-3.5 h-3.5" />
        <span>首页</span>
      </button>

      {parts.map((part, index) => {
        const path = '/' + parts.slice(0, index + 1).join('/') + '/'
        const isLast = index === parts.length - 1
        return (
          <div key={path} className="flex items-center gap-1 shrink-0">
            <ChevronRight className="w-3.5 h-3.5 text-muted-foreground/50" />
            <button
              onClick={() => !isLast && navigateTo(path)}
              className={`px-1.5 py-0.5 rounded transition-colors ${
                isLast
                  ? 'text-foreground font-medium cursor-default'
                  : 'text-muted-foreground hover:text-foreground hover:bg-accent'
              }`}
            >
              {part}
            </button>
          </div>
        )
      })}
    </div>
  )
}
