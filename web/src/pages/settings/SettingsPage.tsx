import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'
import { useQuery } from '@tanstack/react-query'
import { systemApi } from '@/api/system'
import { Sun, Moon, Monitor, Globe, Server, Info, Shield, LogOut } from 'lucide-react'
import { cn } from '@/utils'

export function SettingsPage() {
  const navigate = useNavigate()
  const { isAuthenticated, isLoading: authLoading, logout } = useAuthStore()
  const { theme, setTheme } = useUIStore()

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data: version } = useQuery({
    queryKey: ['version'],
    queryFn: () => systemApi.getVersion(),
  })

  const { data: config } = useQuery({
    queryKey: ['system-config'],
    queryFn: () => systemApi.getConfig(),
  })

  if (authLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const themes = [
    { id: 'light' as const, label: '浅色', icon: Sun },
    { id: 'dark' as const, label: '深色', icon: Moon },
    { id: 'system' as const, label: '跟随系统', icon: Monitor },
  ]

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">系统设置</h1>
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4 max-w-2xl">
        <div className="space-y-6">
          <section className="space-y-3">
            <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">外观</h2>
            <div className="bg-card border border-border rounded-lg p-4">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-3">
                  <Globe className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">主题</p>
                    <p className="text-xs text-muted-foreground">选择您喜欢的界面主题</p>
                  </div>
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                {themes.map((t) => (
                  <button
                    key={t.id}
                    onClick={() => setTheme(t.id)}
                    className={cn(
                      'flex flex-col items-center gap-2 p-3 rounded-md border transition-all',
                      theme === t.id
                        ? 'border-primary bg-primary/5 text-primary'
                        : 'border-border hover:border-primary/30 text-muted-foreground'
                    )}
                  >
                    <t.icon className="w-5 h-5" />
                    <span className="text-sm">{t.label}</span>
                  </button>
                ))}
              </div>
            </div>
          </section>

          <section className="space-y-3">
            <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">系统信息</h2>
            <div className="bg-card border border-border rounded-lg p-4 space-y-3">
              <div className="flex items-center gap-3">
                <Server className="w-5 h-5 text-muted-foreground" />
                <div className="flex-1">
                  <p className="font-medium text-foreground">服务版本</p>
                  <p className="text-sm text-muted-foreground">{version?.version || '-'}</p>
                </div>
              </div>
              <div className="flex items-center gap-3">
                <Info className="w-5 h-5 text-muted-foreground" />
                <div className="flex-1">
                  <p className="font-medium text-foreground">API 版本</p>
                  <p className="text-sm text-muted-foreground">{version?.api_version || '-'}</p>
                </div>
              </div>
              {version?.go_version && (
                <div className="flex items-center gap-3">
                  <Info className="w-5 h-5 text-muted-foreground" />
                  <div className="flex-1">
                    <p className="font-medium text-foreground">Go 版本</p>
                    <p className="text-sm text-muted-foreground">{version.go_version}</p>
                  </div>
                </div>
              )}
            </div>
          </section>

          <section className="space-y-3">
            <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">系统配置</h2>
            <div className="bg-card border border-border rounded-lg p-4 space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Globe className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">站点名称</p>
                    <p className="text-xs text-muted-foreground">当前系统显示名称</p>
                  </div>
                </div>
                <span className="text-sm text-muted-foreground">{config?.site_name || '云匣'}</span>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Shield className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">多用户模式</p>
                    <p className="text-xs text-muted-foreground">是否允许多用户注册</p>
                  </div>
                </div>
                <span className={cn('text-sm', config?.multi_user_enabled ? 'text-emerald-500' : 'text-muted-foreground')}>
                  {config?.multi_user_enabled ? '已启用' : '已禁用'}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Server className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">WebDAV</p>
                    <p className="text-xs text-muted-foreground">WebDAV 服务状态</p>
                  </div>
                </div>
                <span className={cn('text-sm', config?.webdav_enabled ? 'text-emerald-500' : 'text-muted-foreground')}>
                  {config?.webdav_enabled ? '已启用' : '已禁用'}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Info className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">最大上传大小</p>
                    <p className="text-xs text-muted-foreground">单文件上传限制</p>
                  </div>
                </div>
                <span className="text-sm text-muted-foreground">
                  {config?.max_upload_size ? `${(config.max_upload_size / 1024 / 1024).toFixed(0)} MB` : '-'}
                </span>
              </div>
            </div>
          </section>

          <section className="space-y-3">
            <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">账号</h2>
            <div className="bg-card border border-border rounded-lg p-4">
              <button
                onClick={() => {
                  logout()
                  navigate('/login', { replace: true })
                }}
                className="w-full flex items-center justify-center gap-2 py-2.5 rounded-md bg-destructive text-destructive-foreground font-medium hover:bg-destructive/90 transition-colors"
              >
                <LogOut className="w-4 h-4" />
                退出登录
              </button>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
