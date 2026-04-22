import { useQuery } from '@tanstack/react-query'
import { HardDrive, ChevronDown, CheckCircle2 } from 'lucide-react'
import { sourceApi } from '@/api/source'
import { useFileStore } from '@/stores/fileStore'
import { useState, useRef, useEffect } from 'react'
import { cn } from '@/utils'

export function SourceSelector() {
  const { currentSource, setCurrentSource } = useFileStore()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const { data } = useQuery({
    queryKey: ['sources'],
    queryFn: () => sourceApi.list({ page: 1, page_size: 100, view: 'navigation' }),
  })

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  const sources = data?.items || []

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 px-3 py-1.5 rounded-md border border-border bg-card text-sm hover:border-primary/30 transition-colors"
      >
        <HardDrive className="w-4 h-4 text-primary" />
        <span className="text-foreground">{currentSource?.name || '选择存储源'}</span>
        <ChevronDown className={cn('w-3.5 h-3.5 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-1 w-56 bg-card border border-border rounded-lg shadow-lg z-50 py-1">
          {sources.length === 0 ? (
            <div className="px-3 py-2 text-sm text-muted-foreground">暂无存储源</div>
          ) : (
            sources.map((source) => (
              <button
                key={source.id}
                onClick={() => {
                  setCurrentSource(source)
                  setOpen(false)
                }}
                className={cn(
                  'w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors',
                  currentSource?.id === source.id
                    ? 'bg-primary/5 text-primary'
                    : 'text-foreground hover:bg-accent'
                )}
              >
                <HardDrive className="w-4 h-4 shrink-0" />
                <span className="flex-1 text-left truncate">{source.name}</span>
                {currentSource?.id === source.id && <CheckCircle2 className="w-3.5 h-3.5 shrink-0" />}
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}
