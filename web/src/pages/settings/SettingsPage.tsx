import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { systemApi } from '@/api/system'
import { notificationApi } from '@/api/notification'
import {
  Sun, Moon, Monitor, Globe, Server, Info, Shield, LogOut,
  BarChart3, Users, HardDrive, FolderOpen, Download, Link2,
  Pencil, X, Clock, Bell, Send, Trash2, RefreshCcw, AlertTriangle,
} from 'lucide-react'
import { cn, formatBytes, formatDate } from '@/utils'
import { buildWebDAVBaseUrl } from '@/utils/webdav'
import { useHasCapability } from '@/hooks/useCapability'
import type {
  NotificationChannelUpsertRequest,
  NotificationChannelView,
  NotificationEventStatus,
  NotificationEventType,
  NotificationEventView,
  SystemConfigPublic,
} from '@/types/api'

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
  onClose,
  config,
  onSuccess,
}: {
  onClose: () => void
  config: SystemConfigPublic
  onSuccess: () => void
}) {
  const { addToast } = useUIStore()
  const [form, setForm] = useState<SystemConfigPublic>(config)
  const [isSubmitting, setIsSubmitting] = useState(false)

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

const NOTIFICATION_EVENT_TYPES: { value: NotificationEventType; label: string }[] = [
  { value: 'rss.source_failure', label: 'RSS 源失败' },
  { value: 'rss.item_needs_attention', label: 'RSS 条目待处理' },
  { value: 'rss.download_completed', label: 'RSS 下载完成' },
]

const NOTIFICATION_EVENT_STATUS_OPTIONS: { value: NotificationEventStatus | ''; label: string }[] = [
  { value: 'retry_pending', label: '等待重试' },
  { value: 'failed', label: '失败' },
  { value: 'pending', label: '待投递' },
  { value: 'delivered', label: '已投递' },
  { value: 'skipped', label: '已跳过' },
  { value: '', label: '全部状态' },
]

function notificationEventTypeLabel(type: string) {
  return NOTIFICATION_EVENT_TYPES.find((item) => item.value === type)?.label ?? type
}

function notificationStatusLabel(status: string) {
  switch (status) {
    case 'pending':
      return '待投递'
    case 'delivered':
      return '已投递'
    case 'retry_pending':
      return '等待重试'
    case 'failed':
      return '失败'
    case 'skipped':
      return '已跳过'
    default:
      return status || '-'
  }
}

function notificationStatusClass(status: string) {
  switch (status) {
    case 'delivered':
      return 'bg-emerald-500/10 text-emerald-500'
    case 'retry_pending':
      return 'bg-amber-500/10 text-amber-500'
    case 'failed':
      return 'bg-destructive/10 text-destructive'
    case 'skipped':
      return 'bg-muted text-muted-foreground'
    default:
      return 'bg-primary/10 text-primary'
  }
}

function NotificationChannelModal({
  channel,
  onClose,
  onSubmit,
}: {
  channel: NotificationChannelView | null
  onClose: () => void
  onSubmit: (payload: NotificationChannelUpsertRequest) => Promise<void>
}) {
  const [name, setName] = useState(channel?.name ?? '')
  const [url, setURL] = useState(channel?.config.url ?? '')
  const [secret, setSecret] = useState('')
  const [clearSecret, setClearSecret] = useState(false)
  const [isEnabled, setIsEnabled] = useState(channel?.is_enabled ?? true)
  const [eventTypes, setEventTypes] = useState<Set<NotificationEventType>>(
    () => new Set(channel?.event_types ?? [])
  )
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const suffix = channel ? `edit-${channel.id}` : 'create'

  const toggleEventType = (type: NotificationEventType) => {
    setEventTypes((current) => {
      const next = new Set(current)
      if (next.has(type)) {
        next.delete(type)
      } else {
        next.add(type)
      }
      return next
    })
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    const trimmedName = name.trim()
    const trimmedURL = url.trim()
    if (!trimmedName || !trimmedURL) {
      setError('请填写通道名称和 Webhook URL')
      return
    }

    setIsSubmitting(true)
    setError('')
    try {
      await onSubmit({
        name: trimmedName,
        type: 'webhook',
        is_enabled: isEnabled,
        event_types: [...eventTypes],
        config: {
          url: trimmedURL,
          ...(clearSecret ? { secret: '' } : secret ? { secret } : {}),
        },
      })
      onClose()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '保存通知通道失败')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Bell className="w-4 h-4" />
            {channel ? '编辑通知通道' : '新增通知通道'}
          </h3>
          <button type="button" onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground" title="关闭">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          {error && <p className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          <div>
            <label htmlFor={`notification-name-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              通道名称
            </label>
            <input
              id={`notification-name-${suffix}`}
              name="name"
              type="text"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder="Ops Webhook"
            />
          </div>
          <div>
            <label htmlFor={`notification-url-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              Webhook URL
            </label>
            <input
              id={`notification-url-${suffix}`}
              name="url"
              type="url"
              value={url}
              onChange={(event) => setURL(event.target.value)}
              autoComplete="url"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder="https://example.com/yunxia-webhook"
            />
          </div>
          <div>
            <label htmlFor={`notification-secret-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              签名密钥
            </label>
            <input
              id={`notification-secret-${suffix}`}
              name="secret"
              type="password"
              value={secret}
              onChange={(event) => setSecret(event.target.value)}
              disabled={clearSecret}
              autoComplete="new-password"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-50"
              placeholder={channel?.config.secret_configured ? '留空保留旧密钥' : '可选'}
            />
            <p className="mt-1 text-xs text-muted-foreground">
              后端响应不会返回明文；配置后会使用 X-Yunxia-Signature 签名。
            </p>
            {channel?.config.secret_configured && (
              <label className="mt-2 flex items-center gap-2 text-xs text-foreground cursor-pointer">
                <input
                  id={`notification-clear-secret-${suffix}`}
                  name="clear_secret"
                  type="checkbox"
                  checked={clearSecret}
                  onChange={(event) => setClearSecret(event.target.checked)}
                  className="rounded border-border"
                />
                清空已配置签名密钥
              </label>
            )}
          </div>
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">订阅事件（不选表示全部）</p>
            <div className="grid gap-2 md:grid-cols-3">
              {NOTIFICATION_EVENT_TYPES.map((item) => (
                <label key={item.value} className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
                  <input
                    name="event_types"
                    type="checkbox"
                    checked={eventTypes.has(item.value)}
                    onChange={() => toggleEventType(item.value)}
                    className="rounded border-border"
                  />
                  {item.label}
                </label>
              ))}
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
            <input
              id={`notification-enabled-${suffix}`}
              name="is_enabled"
              type="checkbox"
              checked={isEnabled}
              onChange={(event) => setIsEnabled(event.target.checked)}
              className="rounded border-border"
            />
            启用通道
          </label>
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors">
              取消
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isSubmitting ? '保存中...' : '保存'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function NotificationEventCard({
  event,
  canManage,
  isRetrying,
  onRetry,
}: {
  event: NotificationEventView
  canManage: boolean
  isRetrying: boolean
  onRetry: (event: NotificationEventView) => void
}) {
  return (
    <div className="rounded-md border border-border bg-card px-3 py-2 text-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-foreground">{event.title}</span>
            <span className={cn('rounded-full px-2 py-0.5 text-xs', notificationStatusClass(event.status))}>
              {notificationStatusLabel(event.status)}
            </span>
            <span className="text-xs text-muted-foreground">{notificationEventTypeLabel(event.event_type)}</span>
          </div>
          <p className="mt-1 text-xs text-muted-foreground break-all">{event.message}</p>
          <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>尝试：{event.attempts}/{event.max_attempts}</span>
            <span>下次：{event.next_attempt_at ? formatDate(event.next_attempt_at) : '-'}</span>
            <span>创建：{formatDate(event.created_at)}</span>
          </div>
          {event.last_error && (
            <p className="mt-1 text-xs text-destructive break-all">{event.last_error}</p>
          )}
        </div>
        {canManage && (event.status === 'retry_pending' || event.status === 'failed') && (
          <button
            type="button"
            onClick={() => onRetry(event)}
            disabled={isRetrying}
            className="shrink-0 rounded-md border border-border px-2 py-1 text-xs text-foreground hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isRetrying ? '重试中' : '重试'}
          </button>
        )}
      </div>
    </div>
  )
}

export function SettingsPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading, logout } = useAuthStore()
  const { theme, setTheme, addToast } = useUIStore()
  const [editModalOpen, setEditModalOpen] = useState(false)
  const [notificationModalTarget, setNotificationModalTarget] = useState<NotificationChannelView | null | undefined>(undefined)
  const [notificationStatusFilter, setNotificationStatusFilter] = useState<NotificationEventStatus | ''>('retry_pending')
  const [notificationEventTypeFilter, setNotificationEventTypeFilter] = useState<NotificationEventType | ''>('')
  const [testingChannelId, setTestingChannelId] = useState<number | null>(null)
  const [retryingEventId, setRetryingEventId] = useState<number | null>(null)

  const canReadConfig = useHasCapability('system.config.read')
  const canEditConfig = useHasCapability('system.config.write')
  const canReadNotifications = useHasCapability('notification.read')
  const canManageNotifications = useHasCapability('notification.manage')

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data: version } = useQuery({
    queryKey: ['version'],
    queryFn: () => systemApi.getVersion(),
    enabled: canReadConfig,
  })

  const { data: config } = useQuery({
    queryKey: ['system-config'],
    queryFn: () => systemApi.getConfig(),
    enabled: canReadConfig,
  })

  const { data: stats } = useQuery({
    queryKey: ['system-stats'],
    queryFn: () => systemApi.getStats(),
    enabled: canReadConfig,
  })

  const notificationChannelsQuery = useQuery({
    queryKey: ['notifications', 'channels'],
    queryFn: notificationApi.listChannels,
    enabled: canReadNotifications,
  })

  const notificationEventsQuery = useQuery({
    queryKey: ['notifications', 'events', notificationStatusFilter || 'all', notificationEventTypeFilter || 'all'],
    queryFn: () => notificationApi.listEvents({
      status: notificationStatusFilter || undefined,
      event_type: notificationEventTypeFilter || undefined,
      limit: 50,
    }),
    enabled: canReadNotifications,
  })

  const invalidateNotifications = () => {
    void queryClient.invalidateQueries({ queryKey: ['notifications'] })
  }

  const handleSaveNotificationChannel = async (payload: NotificationChannelUpsertRequest) => {
    if (notificationModalTarget) {
      await notificationApi.updateChannel(notificationModalTarget.id, payload)
      addToast('通知通道已更新', 'success')
    } else {
      await notificationApi.createChannel(payload)
      addToast('通知通道已创建', 'success')
    }
    invalidateNotifications()
  }

  const handleDeleteNotificationChannel = async (channel: NotificationChannelView) => {
    if (!confirm(`确定要删除通知通道「${channel.name}」吗？`)) return
    try {
      await notificationApi.deleteChannel(channel.id)
      addToast('通知通道已删除', 'success')
      invalidateNotifications()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '删除通知通道失败'
      addToast(msg, 'error')
    }
  }

  const handleTestNotificationChannel = async (channel: NotificationChannelView) => {
    setTestingChannelId(channel.id)
    try {
      await notificationApi.testChannel(channel.id)
      addToast('测试通知发送成功', 'success')
      invalidateNotifications()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '测试通知发送失败'
      addToast(msg, 'error')
    } finally {
      setTestingChannelId(null)
    }
  }

  const handleRetryNotificationEvent = async (event: NotificationEventView) => {
    setRetryingEventId(event.id)
    try {
      const result = await notificationApi.retryEvent(event.id)
      addToast(`通知事件已重试，当前状态：${notificationStatusLabel(result.event.status)}`, 'success')
      invalidateNotifications()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '重试通知事件失败'
      addToast(msg, 'error')
    } finally {
      setRetryingEventId(null)
    }
  }

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
  const webDAVBaseUrl = buildWebDAVBaseUrl(config?.webdav_prefix)

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
                <StatCard icon={Users} label="用户总数" value={stats.users_total} color="bg-blue-500" />
                <StatCard icon={HardDrive} label="存储源" value={stats.sources_total} color="bg-emerald-500" />
                <StatCard icon={FolderOpen} label="文件总数" value={stats.files_total} color="bg-amber-500" />
                <StatCard icon={BarChart3} label="总容量" value={formatBytes(stats.storage_used_bytes)} color="bg-purple-500" />
                <StatCard icon={Download} label="活跃任务" value={stats.downloads_running} color="bg-rose-500" />
                <StatCard icon={Link2} label="已完成任务" value={stats.downloads_completed} color="bg-cyan-500" />
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

          {canReadConfig && (
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
          )}

          {canReadConfig && (
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
                    <p className="mt-1 text-xs text-muted-foreground">
                      Base URL：
                      <code className="ml-1 rounded bg-muted px-1.5 py-0.5 font-mono text-foreground">
                        {webDAVBaseUrl}
                      </code>
                    </p>
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
          )}

          {canReadNotifications && (
            <section className="space-y-3">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">通知告警</h2>
                {canManageNotifications && (
                  <button
                    type="button"
                    onClick={() => setNotificationModalTarget(null)}
                    className="flex items-center gap-1 px-2.5 py-1 rounded-md text-xs font-medium text-primary hover:bg-primary/5 transition-colors"
                  >
                    <Bell className="w-3 h-3" />
                    新增通道
                  </button>
                )}
              </div>

              <div className="bg-card border border-border rounded-lg p-4 space-y-4">
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-3">
                      <Bell className="w-5 h-5 text-muted-foreground" />
                      <div>
                        <p className="font-medium text-foreground">Webhook 通道</p>
                        <p className="text-xs text-muted-foreground">配置 RSS 告警和完成事件的外部推送地址</p>
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={() => notificationChannelsQuery.refetch()}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      title="刷新通道"
                    >
                      <RefreshCcw className="w-4 h-4" />
                    </button>
                  </div>
                  {notificationChannelsQuery.isLoading ? (
                    <p className="text-sm text-muted-foreground">正在加载通知通道...</p>
                  ) : notificationChannelsQuery.error ? (
                    <p className="text-sm text-destructive">加载通知通道失败</p>
                  ) : (notificationChannelsQuery.data?.items.length ?? 0) === 0 ? (
                    <p className="text-sm text-muted-foreground">暂无通知通道。</p>
                  ) : (
                    <div className="space-y-2">
                      {notificationChannelsQuery.data?.items.map((channel) => (
                        <div key={channel.id} className="rounded-md border border-border bg-muted/20 px-3 py-2">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="flex flex-wrap items-center gap-2">
                                <span className="font-medium text-foreground">{channel.name}</span>
                                <span className={cn('rounded-full px-2 py-0.5 text-xs', channel.is_enabled ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground')}>
                                  {channel.is_enabled ? '启用' : '停用'}
                                </span>
                                {channel.config.secret_configured && (
                                  <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs text-primary">已配置签名</span>
                                )}
                              </div>
                              <p className="mt-1 text-xs text-muted-foreground break-all">{channel.config.url}</p>
                              <p className="mt-1 text-xs text-muted-foreground">
                                事件：{channel.event_types.length > 0 ? channel.event_types.map(notificationEventTypeLabel).join('、') : '全部事件'}
                              </p>
                            </div>
                            {canManageNotifications && (
                              <div className="flex items-center gap-1 shrink-0">
                                <button
                                  type="button"
                                  onClick={() => void handleTestNotificationChannel(channel)}
                                  disabled={testingChannelId === channel.id}
                                  className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-50 disabled:cursor-not-allowed"
                                  title="测试发送"
                                >
                                  {testingChannelId === channel.id ? <RefreshCcw className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
                                </button>
                                <button
                                  type="button"
                                  onClick={() => setNotificationModalTarget(channel)}
                                  className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                                  title="编辑"
                                >
                                  <Pencil className="w-4 h-4" />
                                </button>
                                <button
                                  type="button"
                                  onClick={() => void handleDeleteNotificationChannel(channel)}
                                  className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                                  title="删除"
                                >
                                  <Trash2 className="w-4 h-4" />
                                </button>
                              </div>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div className="border-t border-border pt-4">
                  <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
                    <div className="flex items-center gap-3">
                      <AlertTriangle className="w-5 h-5 text-muted-foreground" />
                      <div>
                        <p className="font-medium text-foreground">通知事件</p>
                        <p className="text-xs text-muted-foreground">查看失败 / 待重试事件并手动 retry</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <label htmlFor="notification-event-status-filter" className="sr-only">
                        通知事件状态
                      </label>
                      <select
                        id="notification-event-status-filter"
                        name="notification_event_status"
                        value={notificationStatusFilter}
                        onChange={(event) => setNotificationStatusFilter(event.target.value as NotificationEventStatus | '')}
                        className="px-2 py-1.5 rounded-md border border-input bg-background text-foreground text-xs focus:outline-none focus:ring-2 focus:ring-ring"
                      >
                        {NOTIFICATION_EVENT_STATUS_OPTIONS.map((option) => (
                          <option key={option.value || 'all'} value={option.value}>{option.label}</option>
                        ))}
                      </select>
                      <label htmlFor="notification-event-type-filter" className="sr-only">
                        通知事件类型
                      </label>
                      <select
                        id="notification-event-type-filter"
                        name="notification_event_type"
                        value={notificationEventTypeFilter}
                        onChange={(event) => setNotificationEventTypeFilter(event.target.value as NotificationEventType | '')}
                        className="px-2 py-1.5 rounded-md border border-input bg-background text-foreground text-xs focus:outline-none focus:ring-2 focus:ring-ring"
                      >
                        <option value="">全部类型</option>
                        {NOTIFICATION_EVENT_TYPES.map((option) => (
                          <option key={option.value} value={option.value}>{option.label}</option>
                        ))}
                      </select>
                    </div>
                  </div>
                  {notificationEventsQuery.isLoading ? (
                    <p className="text-sm text-muted-foreground">正在加载通知事件...</p>
                  ) : notificationEventsQuery.error ? (
                    <p className="text-sm text-destructive">加载通知事件失败</p>
                  ) : (notificationEventsQuery.data?.items.length ?? 0) === 0 ? (
                    <p className="text-sm text-muted-foreground">当前筛选下没有通知事件。</p>
                  ) : (
                    <div className="space-y-2">
                      {notificationEventsQuery.data?.items.map((event) => (
                        <NotificationEventCard
                          key={event.id}
                          event={event}
                          canManage={canManageNotifications}
                          isRetrying={retryingEventId === event.id}
                          onRetry={(target) => void handleRetryNotificationEvent(target)}
                        />
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </section>
          )}

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

      {editModalOpen && config && (
        <ConfigEditModal
          onClose={() => setEditModalOpen(false)}
          config={config}
          onSuccess={() => queryClient.invalidateQueries({ queryKey: ['system-config'] })}
        />
      )}
      {notificationModalTarget !== undefined && (
        <NotificationChannelModal
          channel={notificationModalTarget}
          onClose={() => setNotificationModalTarget(undefined)}
          onSubmit={handleSaveNotificationChannel}
        />
      )}
    </div>
  )
}
