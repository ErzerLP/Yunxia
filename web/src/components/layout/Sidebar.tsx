import { useNavigate, useLocation } from 'react-router-dom'
import { FolderOpen, HardDrive, Download, Settings, Menu, ChevronLeft } from 'lucide-react'
import { useUIStore } from '@/stores/uiStore'
import { cn } from '@/utils'

const navItems = [
  { id: 'files', label: '文件', icon: FolderOpen, path: '/files' },
  { id: 'sources', label: '存储源', icon: HardDrive, path: '/sources' },
  { id: 'tasks', label: '离线下载', icon: Download, path: '/tasks' },
  { id: 'settings', label: '设置', icon: Settings, path: '/settings' },
]

export function Sidebar() {
  const navigate = useNavigate()
  const location = useLocation()
  const { sidebar, toggleSidebar, setSidebarActive } = useUIStore()
  const { isCollapsed } = sidebar

  const handleNavigate = (item: typeof navItems[0]) => {
    setSidebarActive(item.id)
    navigate(item.path)
  }

  const isActive = (path: string) => location.pathname.startsWith(path)

  return (
    <aside
      className={cn(
        'flex flex-col bg-card border-r border-border transition-all duration-300 ease-in-out',
        isCollapsed ? 'w-16' : 'w-52'
      )}
    >
      <div className="flex items-center h-14 px-3 border-b border-border shrink-0">
        <div className={cn('flex items-center gap-2 overflow-hidden', isCollapsed && 'justify-center w-full')}>
          <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center shrink-0">
            <span className="text-primary-foreground font-bold text-sm">云</span>
          </div>
          {!isCollapsed && <span className="font-semibold text-card-foreground truncate">云匣</span>}
        </div>
      </div>

      <nav className="flex-1 py-2 space-y-1 overflow-y-auto scrollbar-thin">
        {navItems.map((item) => {
          const active = isActive(item.path)
          return (
            <button
              key={item.id}
              onClick={() => handleNavigate(item)}
              className={cn(
                'flex items-center gap-3 px-3 py-2 mx-2 rounded-md text-sm transition-colors',
                active
                  ? 'bg-primary/10 text-primary font-medium'
                  : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                isCollapsed && 'justify-center mx-1'
              )}
              title={item.label}
            >
              <item.icon className="w-5 h-5 shrink-0" />
              {!isCollapsed && <span className="truncate">{item.label}</span>}
            </button>
          )
        })}
      </nav>

      <div className="p-2 border-t border-border shrink-0">
        <button
          onClick={toggleSidebar}
          className={cn(
            'flex items-center gap-2 px-3 py-2 rounded-md text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors',
            isCollapsed && 'justify-center'
          )}
          title={isCollapsed ? '展开' : '收起'}
        >
          {isCollapsed ? <Menu className="w-5 h-5" /> : <><ChevronLeft className="w-4 h-4" /><span>收起</span></>}
        </button>
      </div>
    </aside>
  )
}
