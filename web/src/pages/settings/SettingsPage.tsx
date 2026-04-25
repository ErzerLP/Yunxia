import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { systemApi } from '@/api/system'
import {
  Sun, Moon, Monitor, Globe, Server, Info, Shield, LogOut,
  BarChart3, Users, HardDrive, FolderOpen, Download, Link2,
  Pencil, X, Clock,
} from 'lucide-react'
import { cn, formatBytes } from '@/utils'
import { useHasCapability } from '@/hooks/useCapability'
import type { SystemConfigPublic } from '@/types/api'

function StatCard({
  icon: Icon,
  label,
  value,
  color,
}: {
  icon: React.ElementType
  label: string
  value: string | number
  color: string
}) {
  return (
    <div className="bg-card border border-border rounded-lg p-4 flex items-center gap-3">
      <div className={cn('w-10 h-10 rounded-lg flex items-center justify-center', color)}>
        <Icon className="w-5 h-5 text-white" />
      </div>
      <div>
        <p className="text-2xl font-semibold text-foreground">{value}</p>
        <p className="text-xs text-muted-foreground">{label}</p>
      </div>
    </div>
  )
}

function ConfigEditModal({
  isOpen,
  onClose,
  config,
  onSuccess,
}: {
  isOpen: boolean
  onClose: () => void
  config: SystemConfigPublic | null
  onSuccess: () => void
}) {
  const { addToast } = useUIStore()
  const [form, setForm] = useState<SystemConfigPublic>({
    site_name: '云匣',
    multi_user_enabled: false,
    default_source_id: 0,
    max_upload_size: 104857600,
    default_chunk_size: 5242880,
    webdav_enabled: false,
    webdav_prefix: '/webdav',
    theme: 'system',
    language: 'zh-CN',
    time_zone: 'Asia/Shanghai',
  })
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (isOpen && config) {
      setForm(config)
    }
  }, [isOpen, config])

  if (!isOpen || !config) return null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSubmitting(true)
    try {
      await systemApi.updateConfig(form)
      addToast('配置已更新', 'success')
      onSuccess()
      onClose()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '更新失败'
      addToast(msg, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-card border border-border rounded-lg shadow-xl max-h-[90vh] overflow-auto">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border shrink-0">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Pencil className="w-4 h-4" />
            编辑系统配置
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">站点名称</label>
            <input
              type="text"
              value={form.site_name}
              onChange={(e) => setForm((f) => ({ ...f, site_name: e.target.value }))}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">最大上传大小（字节）</label>
              <input
                type="number"
                value={form.max_upload_size}
                onChange={(e) => setForm((f) => ({ ...f, max_upload_size: parseInt(e.target.value) || 0 }))}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">默认分块大小（字节）</label>
              <input
                type="number"
                value={form.default_chunk_size}
                onChange={(e) => setForm((f) => ({ ...f, default_chunk_size: parseInt(e.target.value) || 0 }))}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">语言</label>
              <select
                value={form.language}
                onChange={(e) => setForm((f) => ({ ...f, language: e.target.value }))}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="zh-CN">简体中文</option>
                <option value="en-US">English</option>
              </select>
            </div>
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">时区</label>
              <input
                type="text"
                value={form.time_zone}
                onChange={(e) => setForm((f) => ({ ...f, time_zone: e.target.value }))}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">WebDAV 前缀</label>
              <input
                type="text"
                value={form.webdav_prefix}
                onChange={(e) => setForm((f) => ({ ...f, webdav_prefix: e.target.value }))}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">默认主题</label>
              <select
                value={form.theme}
                onChange={(e) => setForm((f) => ({ ...f, theme: e.target.value as SystemConfigPublic['theme'] }))}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="light">浅色</option>
                <option value="dark">深色</option>
                <option value="system">跟随系统</option>
              </select>
            </div>
          </div>
          <div className="flex gap-4">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={form.multi_user_enabled}
                onChange={(e) => setForm((f) => ({ ...f, multi_user_enabled: e.target.checked }))}
                className="rounded border-border"
              />
              <span className="text-sm text-foreground">多用户模式</span>
            </label>
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={form.webdav_enabled}
                onChange={(e) => setForm((f) => ({ ...f, webdav_enabled: e.target.checked }))}
                className="rounded border-border"
              />
              <span className="text-sm text-foreground">启用 WebDAV</span>
            </label>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                isSubmitting && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '保存中...' : '保存'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export function SettingsPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading, logout } = useAuthStore()
  const { theme, setTheme } = useUIStore()
  const [editModalOpen, setEditModalOpen] = useState(false)

  const canEditConfig = useHasCapability('system.config.write')

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

  const { data: stats } = useQuery({
    queryKey: ['system-stats'],
    queryFn: () => systemApi.getStats(),
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

      <div className="flex-1 overflow-auto scrollbar-thin p-4 max-w-3xl">
        <div className="space-y-6">
          {stats && (
            <section className="space-y-3">
              <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">系统统计</h2>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
                <StatCard icon={Users} label="用户总数" value={stats.total_users} color="bg-blue-500" />
                <StatCard icon={HardDrive} label="存储源" value={stats.total_sources} color="bg-emerald-500" />
                <StatCard icon={FolderOpen} label="文件总数" value={stats.total_files} color="bg-amber-500" />
                <StatCard icon={BarChart3} label="总容量" value={formatBytes(stats.total_bytes)} color="bg-purple-500" />
                <StatCard icon={Download} label="活跃任务" value={stats.active_tasks} color="bg-rose-500" />
                <StatCard icon={Link2} label="分享总数" value={stats.total_shares} color="bg-cyan-500" />
              </div>
            </section>
          )}

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
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">系统配置</h2>
              {canEditConfig && (
                <button
                  onClick={() => setEditModalOpen(true)}
                  className="flex items-center gap-1 px-2.5 py-1 rounded-md text-xs font-medium text-primary hover:bg-primary/5 transition-colors"
                >
                  <Pencil className="w-3 h-3" />
                  编辑
                </button>
              )}
            </div>
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
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Globe className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">语言</p>
                    <p className="text-xs text-muted-foreground">系统默认语言</p>
                  </div>
                </div>
                <span className="text-sm text-muted-foreground">{config?.language || '-'}</span>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Monitor className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">默认主题</p>
                    <p className="text-xs text-muted-foreground">系统默认主题设置</p>
                  </div>
                </div>
                <span className="text-sm text-muted-foreground">{config?.theme || '-'}</span>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Clock className="w-5 h-5 text-muted-foreground" />
                  <div>
                    <p className="font-medium text-foreground">时区</p>
                    <p className="text-xs text-muted-foreground">系统时区设置</p>
                  </div>
                </div>
                <span className="text-sm text-muted-foreground">{config?.time_zone || '-'}</span>
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

      <ConfigEditModal
        isOpen={editModalOpen}
        onClose={() => setEditModalOpen(false)}
        config={config || null}
        onSuccess={() => queryClient.invalidateQueries({ queryKey: ['system-config'] })}
      />
    </div>
  )
}
