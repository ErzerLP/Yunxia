import { useEffect, useMemo, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  AlertTriangle,
  DownloadCloud,
  ExternalLink,
  Filter,
  Loader2,
  Pencil,
  Play,
  Plus,
  RadioTower,
  RefreshCcw,
  Rss,
  Trash2,
  X,
} from 'lucide-react'
import { rssApi } from '@/api/rss'
import { useHasCapability } from '@/hooks/useCapability'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'
import { cn, formatDate } from '@/utils'
import type {
  RSSItemStatus,
  RSSItemView,
  RSSQBitHealthResponse,
  RSSRefreshAllResponse,
  RSSRefreshResponse,
  RSSRefreshStatsView,
  RSSSourceUpsertRequest,
  RSSSourceView,
  RSSSubscriptionPreviewItem,
  RSSSubscriptionPreviewResponse,
  RSSSubscriptionUpsertRequest,
  RSSSubscriptionView,
} from '@/types/api'

const RSS_STATUS_OPTIONS: { value: RSSItemStatus; label: string }[] = [
  { value: 'new', label: '新条目' },
  { value: 'unsupported', label: '不支持的链接' },
  { value: 'ignored', label: '未匹配' },
  { value: 'matched', label: '已匹配' },
  { value: 'enqueued', label: '已加入下载' },
  { value: 'retry_pending', label: '等待重试' },
  { value: 'completed', label: '已完成' },
  { value: 'needs_attention', label: '需要处理' },
  { value: 'failed', label: '处理失败' },
]

const STATUS_CLASSES: Record<RSSItemStatus, string> = {
  new: 'bg-muted text-muted-foreground',
  unsupported: 'bg-amber-500/10 text-amber-500',
  ignored: 'bg-muted text-muted-foreground',
  matched: 'bg-primary/10 text-primary',
  enqueued: 'bg-emerald-500/10 text-emerald-500',
  retry_pending: 'bg-amber-500/10 text-amber-500',
  completed: 'bg-emerald-500/10 text-emerald-500',
  needs_attention: 'bg-destructive/10 text-destructive ring-1 ring-destructive/20',
  failed: 'bg-destructive/10 text-destructive',
}

type RSSRefreshSummary = Pick<
  RSSRefreshResponse | RSSRefreshStatsView,
  'fetched' | 'created' | 'updated' | 'matched' | 'enqueued' | 'unsupported' | 'failed'
>

function statusLabel(status: RSSItemStatus) {
  return RSS_STATUS_OPTIONS.find((item) => item.value === status)?.label ?? status
}

function linkTypeLabel(type: string) {
  switch (type) {
    case 'magnet':
      return 'Magnet'
    case 'torrent':
      return 'Torrent'
    case 'http':
      return 'HTTP'
    case 'unsupported':
      return '不支持'
    default:
      return type || '-'
  }
}

function healthLabel(health?: RSSQBitHealthResponse) {
  if (!health) return '检查中'
  if (!health.enabled) return '未启用'
  if (health.status === 'ok') return '可用'
  if (health.status === 'unavailable') return '不可用'
  return health.status
}

function formatRefreshSummary(result: RSSRefreshSummary) {
  return `获取 ${result.fetched} 条，新增 ${result.created} 条，更新 ${result.updated} 条，匹配 ${result.matched} 条，入队 ${result.enqueued} 条，失败 ${result.failed} 条`
}

function toOptionalNumber(value: string) {
  if (!value) return undefined
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined
}

function parseListText(value: string) {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function listToText(value: string[]) {
  return value.join('\n')
}

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback
}

function sourceHealthLabel(status?: string) {
  switch (status) {
    case 'ok':
      return '健康'
    case 'degraded':
      return '降级'
    case 'circuit_open':
      return '熔断'
    default:
      return status || '未知'
  }
}

function sourceHealthClass(status?: string) {
  switch (status) {
    case 'ok':
      return 'bg-emerald-500/10 text-emerald-500 border-emerald-500/20'
    case 'degraded':
      return 'bg-amber-500/10 text-amber-500 border-amber-500/20'
    case 'circuit_open':
      return 'bg-destructive/10 text-destructive border-destructive/20'
    default:
      return 'bg-muted text-muted-foreground border-border'
  }
}

function refreshStatusLabel(status?: string) {
  switch (status) {
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    default:
      return status || '未刷新'
  }
}

function refreshAllStatusLabel(status: string) {
  switch (status) {
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    case 'skipped':
      return '跳过'
    default:
      return status || '-'
  }
}

function refreshAllStatusClass(status: string) {
  switch (status) {
    case 'success':
      return 'bg-emerald-500/10 text-emerald-500'
    case 'failed':
      return 'bg-destructive/10 text-destructive'
    case 'skipped':
      return 'bg-amber-500/10 text-amber-500'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

function previewResultLabel(result: string) {
  switch (result) {
    case 'matched':
      return '命中'
    case 'missing':
      return '缺失'
    case 'excluded':
      return '排除'
    default:
      return result || '-'
  }
}

function previewResultClass(result: string) {
  switch (result) {
    case 'matched':
      return 'bg-emerald-500/10 text-emerald-500'
    case 'missing':
      return 'bg-amber-500/10 text-amber-500'
    case 'excluded':
      return 'bg-destructive/10 text-destructive'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

function retryReasonLabel(reason?: string | null) {
  switch (reason) {
    case 'downloader_unavailable':
      return '下载器不可用'
    case 'torrent_fetch_failed':
      return 'Torrent 获取失败'
    case 'task_failed':
      return '下载任务失败'
    case 'stalled':
      return '任务无进展'
    case 'deterministic_error':
      return '确定性错误'
    default:
      return reason || '-'
  }
}

function Panel({
  title,
  icon,
  action,
  children,
}: {
  title: string
  icon: ReactNode
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="rounded-lg border border-border bg-card overflow-hidden">
      <div className="flex items-center justify-between px-4 h-12 border-b border-border">
        <h2 className="font-medium text-foreground flex items-center gap-2">
          {icon}
          {title}
        </h2>
        {action}
      </div>
      <div className="p-4">{children}</div>
    </section>
  )
}

function StatusBadge({ status }: { status: RSSItemStatus }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
        STATUS_CLASSES[status]
      )}
    >
      {statusLabel(status)}
    </span>
  )
}

function LinkTypeBadge({ type }: { type: string }) {
  const isBt = type === 'magnet' || type === 'torrent'
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
        isBt ? 'bg-violet-500/10 text-violet-500' : 'bg-muted text-muted-foreground'
      )}
    >
      {linkTypeLabel(type)}
    </span>
  )
}

function SourceHealthBadge({ status }: { status?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium',
        sourceHealthClass(status)
      )}
    >
      {sourceHealthLabel(status)}
    </span>
  )
}

function RefreshAllResultPanel({
  result,
  sourceNameById,
}: {
  result: RSSRefreshAllResponse
  sourceNameById: Map<number, string>
}) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-3 space-y-2">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>刷新全部结果：</span>
        <span className="text-emerald-500">成功 {result.refreshed}</span>
        <span className="text-amber-500">跳过 {result.skipped}</span>
        <span className="text-destructive">失败 {result.failed}</span>
      </div>
      <div className="space-y-1">
        {result.items.map((item) => (
          <div key={item.source_id} className="rounded-md border border-border bg-card px-2 py-1.5 text-xs">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-foreground">
                {sourceNameById.get(item.source_id) ?? `源 #${item.source_id}`}
              </span>
              <span className={cn('rounded-full px-1.5 py-0.5', refreshAllStatusClass(item.status))}>
                {refreshAllStatusLabel(item.status)}
              </span>
            </div>
            {item.stats && (
              <p className="mt-1 text-muted-foreground">
                {formatRefreshSummary(item.stats)}
              </p>
            )}
            {item.error && (
              <p className="mt-1 text-destructive break-all">
                {item.error}
              </p>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function PreviewKeywords({
  label,
  values,
  className,
}: {
  label: string
  values: string[]
  className: string
}) {
  if (values.length === 0) return null

  return (
    <div className="flex flex-wrap items-center gap-1">
      <span className="text-muted-foreground">{label}：</span>
      {values.map((value) => (
        <span key={value} className={cn('rounded border px-1.5 py-0.5', className)}>
          {value}
        </span>
      ))}
    </div>
  )
}

function SubscriptionPreviewPanel({ preview }: { preview: RSSSubscriptionPreviewResponse }) {
  const previewItems = preview.items.slice(0, 6)

  return (
    <div className="mt-3 rounded-lg border border-border bg-muted/30 p-3 space-y-2">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>预览结果：</span>
        <span className="text-emerald-500">命中 {preview.matched}</span>
        <span className="text-amber-500">缺失 {preview.missing}</span>
        <span className="text-destructive">排除 {preview.excluded}</span>
      </div>
      {preview.items.length === 0 ? (
        <p className="text-xs text-muted-foreground">暂无可预览条目。</p>
      ) : (
        <div className="space-y-1.5">
          {previewItems.map((item: RSSSubscriptionPreviewItem) => (
            <div key={item.item_id} className="rounded-md border border-border bg-card px-2 py-1.5 text-xs">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-foreground break-all">{item.title}</span>
                <span className={cn('rounded-full px-1.5 py-0.5', previewResultClass(item.result))}>
                  {previewResultLabel(item.result)}
                </span>
                <span className="text-muted-foreground">{statusLabel(item.current_status as RSSItemStatus)}</span>
              </div>
              <div className="mt-1 space-y-1">
                <PreviewKeywords
                  label="matched"
                  values={item.matched}
                  className="border-emerald-500/20 bg-emerald-500/10 text-emerald-500"
                />
                <PreviewKeywords
                  label="missing"
                  values={item.missing}
                  className="border-amber-500/20 bg-amber-500/10 text-amber-500"
                />
                <PreviewKeywords
                  label="excluded"
                  values={item.excluded}
                  className="border-destructive/20 bg-destructive/10 text-destructive"
                />
              </div>
            </div>
          ))}
          {preview.items.length > previewItems.length && (
            <p className="text-xs text-muted-foreground">仅展示前 {previewItems.length} 条，完整结果请缩小筛选后重试。</p>
          )}
        </div>
      )}
    </div>
  )
}

function EmptyState({ icon, text }: { icon: ReactNode; text: string }) {
  return (
    <div className="flex flex-col items-center justify-center py-10 text-muted-foreground gap-3">
      <div className="opacity-30">{icon}</div>
      <p className="text-sm">{text}</p>
    </div>
  )
}

function QueryError({ error, fallback }: { error: unknown; fallback: string }) {
  return (
    <div className="rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive">
      {getErrorMessage(error, fallback)}
    </div>
  )
}

function SourceModal({
  source,
  onClose,
  onSubmit,
}: {
  source: RSSSourceView | null
  onClose: () => void
  onSubmit: (data: RSSSourceUpsertRequest) => Promise<void>
}) {
  const suffix = source ? `edit-${source.id}` : 'create'
  const [name, setName] = useState(source?.name ?? '')
  const [url, setUrl] = useState(source?.url ?? '')
  const [isEnabled, setIsEnabled] = useState(source?.is_enabled ?? true)
  const [interval, setInterval] = useState(String(source?.refresh_interval_seconds ?? 1800))
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedName = name.trim()
    const trimmedURL = url.trim()
    const refreshInterval = Number(interval)

    if (!trimmedName || !trimmedURL) {
      setError('请填写 RSS 源名称和 URL')
      return
    }
    if (!Number.isFinite(refreshInterval) || refreshInterval <= 0) {
      setError('刷新间隔必须是大于 0 的秒数')
      return
    }

    setIsSubmitting(true)
    setError('')
    try {
      await onSubmit({
        name: trimmedName,
        url: trimmedURL,
        is_enabled: isEnabled,
        refresh_interval_seconds: refreshInterval,
      })
      onClose()
    } catch (err: unknown) {
      setError(getErrorMessage(err, '保存 RSS 源失败'))
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
            <Rss className="w-4 h-4" />
            {source ? '编辑 RSS 源' : '新增 RSS 源'}
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
            title="关闭"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          {error && <p className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          <div>
            <label htmlFor={`rss-source-name-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              名称
            </label>
            <input
              id={`rss-source-name-${suffix}`}
              name="name"
              type="text"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder="示例 RSS"
            />
          </div>

          <div>
            <label htmlFor={`rss-source-url-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              RSS URL
            </label>
            <input
              id={`rss-source-url-${suffix}`}
              name="url"
              type="url"
              value={url}
              onChange={(event) => setUrl(event.target.value)}
              autoComplete="url"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder="https://example.com/feed.xml"
            />
          </div>

          <div>
            <label htmlFor={`rss-source-interval-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              刷新间隔（秒）
            </label>
            <input
              id={`rss-source-interval-${suffix}`}
              name="refresh_interval_seconds"
              type="number"
              min={1}
              value={interval}
              onChange={(event) => setInterval(event.target.value)}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>

          <div className="flex items-center gap-2">
            <input
              id={`rss-source-enabled-${suffix}`}
              name="is_enabled"
              type="checkbox"
              checked={isEnabled}
              onChange={(event) => setIsEnabled(event.target.checked)}
              className="rounded border-border"
            />
            <label htmlFor={`rss-source-enabled-${suffix}`} className="text-sm text-foreground cursor-pointer">
              启用 RSS 源
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

function SubscriptionModal({
  subscription,
  sources,
  defaultSourceId,
  onClose,
  onSubmit,
}: {
  subscription: RSSSubscriptionView | null
  sources: RSSSourceView[]
  defaultSourceId?: number
  onClose: () => void
  onSubmit: (data: RSSSubscriptionUpsertRequest) => Promise<void>
}) {
  const suffix = subscription ? `edit-${subscription.id}` : 'create'
  const initialSourceId = subscription?.source_id ?? defaultSourceId ?? sources[0]?.id
  const [sourceId, setSourceId] = useState(initialSourceId ? String(initialSourceId) : '')
  const [name, setName] = useState(subscription?.name ?? '')
  const [isEnabled, setIsEnabled] = useState(subscription?.is_enabled ?? true)
  const [mustContain, setMustContain] = useState(listToText(subscription?.must_contain ?? []))
  const [mustNotContain, setMustNotContain] = useState(listToText(subscription?.must_not_contain ?? []))
  const [useRegex, setUseRegex] = useState(subscription?.use_regex ?? false)
  const [caseSensitive, setCaseSensitive] = useState(subscription?.case_sensitive ?? false)
  const [targetPath, setTargetPath] = useState(subscription?.target_virtual_parent_path ?? '/')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    const parsedSourceId = Number(sourceId)
    const trimmedName = name.trim()
    const trimmedTarget = targetPath.trim() || '/'

    if (!parsedSourceId || !trimmedName) {
      setError('请选择 RSS 源并填写订阅名称')
      return
    }
    if (!trimmedTarget.startsWith('/')) {
      setError('目标虚拟目录必须以 / 开头')
      return
    }

    setIsSubmitting(true)
    setError('')
    try {
      await onSubmit({
        source_id: parsedSourceId,
        name: trimmedName,
        is_enabled: isEnabled,
        must_contain: parseListText(mustContain),
        must_not_contain: parseListText(mustNotContain),
        use_regex: useRegex,
        case_sensitive: caseSensitive,
        target_virtual_parent_path: trimmedTarget,
      })
      onClose()
    } catch (err: unknown) {
      setError(getErrorMessage(err, '保存订阅失败'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-xl bg-card border border-border rounded-lg shadow-xl max-h-[90vh] overflow-auto">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border sticky top-0 bg-card">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <RadioTower className="w-4 h-4" />
            {subscription ? '编辑订阅规则' : '新增订阅规则'}
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
            title="关闭"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          {error && <p className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          {sources.length === 0 && (
            <p className="rounded-md bg-amber-500/10 px-3 py-2 text-sm text-amber-500">
              请先创建 RSS 源，再创建订阅规则。
            </p>
          )}

          <div>
            <label htmlFor={`rss-sub-source-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              RSS 源
            </label>
            <select
              id={`rss-sub-source-${suffix}`}
              name="source_id"
              value={sourceId}
              onChange={(event) => setSourceId(event.target.value)}
              disabled={sources.length === 0}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-50"
            >
              {sources.map((source) => (
                <option key={source.id} value={source.id}>
                  {source.name}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label htmlFor={`rss-sub-name-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              订阅名称
            </label>
            <input
              id={`rss-sub-name-${suffix}`}
              name="name"
              type="text"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              placeholder="Frieren 1080p"
            />
          </div>

          <div>
            <label htmlFor={`rss-sub-target-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
              目标虚拟目录
            </label>
            <input
              id={`rss-sub-target-${suffix}`}
              name="target_virtual_parent_path"
              type="text"
              value={targetPath}
              onChange={(event) => setTargetPath(event.target.value)}
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
              placeholder="/local/anime-test"
            />
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <div>
              <label htmlFor={`rss-sub-must-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
                必须包含
              </label>
              <textarea
                id={`rss-sub-must-${suffix}`}
                name="must_contain"
                value={mustContain}
                onChange={(event) => setMustContain(event.target.value)}
                rows={4}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                placeholder="Frieren&#10;1080p"
              />
            </div>
            <div>
              <label htmlFor={`rss-sub-must-not-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
                必须不包含
              </label>
              <textarea
                id={`rss-sub-must-not-${suffix}`}
                name="must_not_contain"
                value={mustNotContain}
                onChange={(event) => setMustNotContain(event.target.value)}
                rows={4}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                placeholder="CHT&#10;720p"
              />
            </div>
          </div>

          <div className="grid gap-2 md:grid-cols-3">
            <div className="flex items-center gap-2">
              <input
                id={`rss-sub-enabled-${suffix}`}
                name="is_enabled"
                type="checkbox"
                checked={isEnabled}
                onChange={(event) => setIsEnabled(event.target.checked)}
                className="rounded border-border"
              />
              <label htmlFor={`rss-sub-enabled-${suffix}`} className="text-sm text-foreground cursor-pointer">
                启用订阅
              </label>
            </div>
            <div className="flex items-center gap-2">
              <input
                id={`rss-sub-regex-${suffix}`}
                name="use_regex"
                type="checkbox"
                checked={useRegex}
                onChange={(event) => setUseRegex(event.target.checked)}
                className="rounded border-border"
              />
              <label htmlFor={`rss-sub-regex-${suffix}`} className="text-sm text-foreground cursor-pointer">
                使用正则
              </label>
            </div>
            <div className="flex items-center gap-2">
              <input
                id={`rss-sub-case-${suffix}`}
                name="case_sensitive"
                type="checkbox"
                checked={caseSensitive}
                onChange={(event) => setCaseSensitive(event.target.checked)}
                className="rounded border-border"
              />
              <label htmlFor={`rss-sub-case-${suffix}`} className="text-sm text-foreground cursor-pointer">
                区分大小写
              </label>
            </div>
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
              disabled={isSubmitting || sources.length === 0}
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

function HealthCard({
  health,
  isLoading,
  onRefresh,
}: {
  health?: RSSQBitHealthResponse
  isLoading: boolean
  onRefresh: () => void
}) {
  const isHealthy = health?.enabled && health.status === 'ok'
  const isUnavailable = health?.enabled && health.status === 'unavailable'

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          <div
            className={cn(
              'w-10 h-10 rounded-lg flex items-center justify-center',
              isHealthy
                ? 'bg-emerald-500/10 text-emerald-500'
                : isUnavailable
                  ? 'bg-destructive/10 text-destructive'
                  : 'bg-muted text-muted-foreground'
            )}
          >
            {isLoading ? <Loader2 className="w-5 h-5 animate-spin" /> : <Activity className="w-5 h-5" />}
          </div>
          <div>
            <p className="text-sm text-muted-foreground">qBittorrent 健康状态</p>
            <p className="font-semibold text-foreground">{healthLabel(health)}</p>
          </div>
        </div>
        <button
          type="button"
          onClick={onRefresh}
          className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
          title="刷新健康状态"
        >
          <RefreshCcw className="w-4 h-4" />
        </button>
      </div>
      {health?.error && (
        <p className="mt-3 text-xs text-destructive break-all">
          {health.error}
        </p>
      )}
    </div>
  )
}

export function RssPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const { addToast } = useUIStore()
  const canRead = useHasCapability('rss.read')
  const canManage = useHasCapability('rss.manage')

  const [sourceModalTarget, setSourceModalTarget] = useState<RSSSourceView | null | undefined>(undefined)
  const [subscriptionModalTarget, setSubscriptionModalTarget] = useState<RSSSubscriptionView | null | undefined>(undefined)
  const [sourceFilter, setSourceFilter] = useState('')
  const [subscriptionFilter, setSubscriptionFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState<RSSItemStatus | ''>('')
  const [refreshAllResult, setRefreshAllResult] = useState<RSSRefreshAllResponse | null>(null)
  const [isRefreshingAll, setIsRefreshingAll] = useState(false)
  const [refreshingSourceId, setRefreshingSourceId] = useState<number | null>(null)
  const [runningSubscriptionId, setRunningSubscriptionId] = useState<number | null>(null)
  const [previewingSubscriptionId, setPreviewingSubscriptionId] = useState<number | null>(null)
  const [subscriptionPreviews, setSubscriptionPreviews] = useState<Record<number, RSSSubscriptionPreviewResponse>>({})
  const [activeItemAction, setActiveItemAction] = useState<{
    id: number
    type: 'download' | 'reprocess' | 'retry'
  } | null>(null)

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [authLoading, isAuthenticated, navigate])

  useEffect(() => {
    if (!authLoading && isAuthenticated && !canRead) {
      addToast('无权限访问 RSS 追番', 'error')
      navigate('/files', { replace: true })
    }
  }, [addToast, authLoading, canRead, isAuthenticated, navigate])

  const sourceIdFilter = toOptionalNumber(sourceFilter)
  const subscriptionIdFilter = toOptionalNumber(subscriptionFilter)

  const healthQuery = useQuery({
    queryKey: ['rss', 'qbit-health'],
    queryFn: rssApi.qbitHealth,
    enabled: canRead,
  })

  const sourcesQuery = useQuery({
    queryKey: ['rss', 'sources'],
    queryFn: rssApi.listSources,
    enabled: canRead,
  })

  const subscriptionsQuery = useQuery({
    queryKey: ['rss', 'subscriptions', sourceIdFilter ?? 'all'],
    queryFn: () => rssApi.listSubscriptions(sourceIdFilter ? { source_id: sourceIdFilter } : undefined),
    enabled: canRead,
  })

  const itemsQuery = useQuery({
    queryKey: ['rss', 'items', sourceIdFilter ?? 'all', subscriptionIdFilter ?? 'all', statusFilter || 'all'],
    queryFn: () =>
      rssApi.listItems({
        source_id: sourceIdFilter,
        subscription_id: subscriptionIdFilter,
        status: statusFilter || undefined,
      }),
    enabled: canRead,
  })

  const needsAttentionQuery = useQuery({
    queryKey: ['rss', 'items', 'needs_attention', 'summary'],
    queryFn: () => rssApi.listItems({ status: 'needs_attention' }),
    enabled: canRead,
  })

  const sources = useMemo(() => sourcesQuery.data?.items ?? [], [sourcesQuery.data?.items])
  const subscriptions = useMemo(() => subscriptionsQuery.data?.items ?? [], [subscriptionsQuery.data?.items])
  const items = useMemo(() => itemsQuery.data?.items ?? [], [itemsQuery.data?.items])
  const needsAttentionItems = useMemo(
    () => needsAttentionQuery.data?.items ?? [],
    [needsAttentionQuery.data?.items]
  )

  const sourceNameById = useMemo(
    () => new Map(sources.map((source) => [source.id, source.name])),
    [sources]
  )
  const subscriptionNameById = useMemo(
    () => new Map(subscriptions.map((subscription) => [subscription.id, subscription.name])),
    [subscriptions]
  )

  const invalidateRSS = () => {
    void queryClient.invalidateQueries({ queryKey: ['rss'] })
  }

  const showNeedsAttentionItems = () => {
    setSourceFilter('')
    setSubscriptionFilter('')
    setStatusFilter('needs_attention')
  }

  const handleSaveSource = async (payload: RSSSourceUpsertRequest) => {
    if (sourceModalTarget) {
      await rssApi.updateSource(sourceModalTarget.id, payload)
      addToast('RSS 源已更新', 'success')
    } else {
      await rssApi.createSource(payload)
      addToast('RSS 源已创建', 'success')
    }
    invalidateRSS()
  }

  const handleRefreshHealth = async () => {
    try {
      const result = await healthQuery.refetch()
      if (result.error) {
        addToast(getErrorMessage(result.error, '刷新 qBittorrent 健康状态失败'), 'error')
      }
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '刷新 qBittorrent 健康状态失败'), 'error')
    }
  }

  const handleDeleteSource = async (source: RSSSourceView) => {
    if (!confirm(`确定要删除 RSS 源「${source.name}」吗？`)) return
    try {
      await rssApi.deleteSource(source.id)
      addToast('RSS 源已删除', 'success')
      invalidateRSS()
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '删除 RSS 源失败'), 'error')
    }
  }

  const handleRefreshSource = async (source: RSSSourceView) => {
    setRefreshingSourceId(source.id)
    try {
      const result = await rssApi.refreshSource(source.id)
      addToast(`刷新完成：${formatRefreshSummary(result)}`, 'success', 6000)
      invalidateRSS()
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '刷新 RSS 源失败'), 'error')
    } finally {
      setRefreshingSourceId(null)
    }
  }

  const handleRefreshAllSources = async () => {
    if (isRefreshingAll) return

    setIsRefreshingAll(true)
    setRefreshAllResult(null)
    try {
      const result = await rssApi.refreshAllSources()
      setRefreshAllResult(result)
      addToast(
        `刷新全部完成：成功 ${result.refreshed}，跳过 ${result.skipped}，失败 ${result.failed}`,
        result.failed > 0 ? 'warning' : 'success',
        7000
      )
      invalidateRSS()
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '刷新全部 RSS 源失败'), 'error')
    } finally {
      setIsRefreshingAll(false)
    }
  }

  const handleSaveSubscription = async (payload: RSSSubscriptionUpsertRequest) => {
    if (subscriptionModalTarget) {
      await rssApi.updateSubscription(subscriptionModalTarget.id, payload)
      addToast('订阅规则已更新', 'success')
    } else {
      await rssApi.createSubscription(payload)
      addToast('订阅规则已创建', 'success')
    }
    invalidateRSS()
  }

  const handleDeleteSubscription = async (subscription: RSSSubscriptionView) => {
    if (!confirm(`确定要删除订阅「${subscription.name}」吗？`)) return
    try {
      await rssApi.deleteSubscription(subscription.id)
      addToast('订阅规则已删除', 'success')
      invalidateRSS()
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '删除订阅规则失败'), 'error')
    }
  }

  const handleRunSubscription = async (subscription: RSSSubscriptionView) => {
    setRunningSubscriptionId(subscription.id)
    try {
      const result = await rssApi.runSubscription(subscription.id)
      addToast(`执行完成：${formatRefreshSummary(result)}`, 'success', 6000)
      invalidateRSS()
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '手动执行订阅失败'), 'error')
    } finally {
      setRunningSubscriptionId(null)
    }
  }

  const handlePreviewSubscription = async (subscription: RSSSubscriptionView) => {
    setPreviewingSubscriptionId(subscription.id)
    try {
      const result = await rssApi.previewSubscription(subscription.id)
      setSubscriptionPreviews((current) => ({
        ...current,
        [subscription.id]: result,
      }))
      addToast(
        `规则预览完成：命中 ${result.matched}，缺失 ${result.missing}，排除 ${result.excluded}`,
        'success',
        6000
      )
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '预览订阅规则失败'), 'error')
    } finally {
      setPreviewingSubscriptionId(null)
    }
  }

  const getDownloadDisabledReason = (item: RSSItemView) => {
    if (!canManage) return '无管理权限'
    if (item.status === 'completed') return '已完成'
    if (item.status === 'enqueued' || item.task_id) return '已加入下载'
    if (item.link_type !== 'magnet' && item.link_type !== 'torrent') return '仅支持 BT/magnet 条目'
    if (!item.matched_subscription_id && !subscriptionIdFilter) return '请选择订阅后手动入队'
    return ''
  }

  const getRetryDisabledReason = (item: RSSItemView) => {
    if (!canManage) return '无管理权限'
    if (item.status === 'completed') return '已完成'
    if (item.status === 'enqueued') return '已有下载任务'
    if (item.link_type !== 'magnet' && item.link_type !== 'torrent') return '仅支持 BT/magnet 条目'
    if (!subscriptionIdFilter && !item.matched_subscription_id) return '请选择订阅后重试'
    return ''
  }

  const handleDownloadItem = async (item: RSSItemView) => {
    const reason = getDownloadDisabledReason(item)
    if (reason) {
      addToast(reason, 'error')
      return
    }
    setActiveItemAction({ id: item.id, type: 'download' })
    try {
      const subscriptionId = item.matched_subscription_id ?? subscriptionIdFilter
      await rssApi.downloadItem(item.id, subscriptionId)
      addToast('RSS 条目已加入下载队列', 'success')
      invalidateRSS()
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '手动入队失败'), 'error')
    } finally {
      setActiveItemAction(null)
    }
  }

  const handleReprocessItem = async (item: RSSItemView) => {
    setActiveItemAction({ id: item.id, type: 'reprocess' })
    try {
      await rssApi.reprocessItem(item.id)
      addToast('RSS 条目已重新处理', 'success')
      invalidateRSS()
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '重新处理条目失败'), 'error')
    } finally {
      setActiveItemAction(null)
    }
  }

  const handleRetryItem = async (item: RSSItemView) => {
    const reason = getRetryDisabledReason(item)
    if (reason) {
      addToast(reason, 'error')
      return
    }

    setActiveItemAction({ id: item.id, type: 'retry' })
    try {
      const subscriptionId = subscriptionIdFilter ?? item.matched_subscription_id ?? undefined
      await rssApi.retryItem(item.id, subscriptionId)
      addToast('RSS 条目已触发手动重试', 'success')
      invalidateRSS()
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '手动重试失败'), 'error')
    } finally {
      setActiveItemAction(null)
    }
  }

  if (authLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <div>
          <h1 className="text-lg font-semibold text-foreground flex items-center gap-2">
            <Rss className="w-5 h-5 text-primary" />
            RSS 追番
          </h1>
          <p className="text-xs text-muted-foreground">管理 RSS 源、订阅规则和 BT/magnet 入队状态</p>
        </div>
        {canManage && (
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => void handleRefreshAllSources()}
              disabled={isRefreshingAll}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border text-sm text-foreground hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isRefreshingAll ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCcw className="w-4 h-4" />}
              <span>刷新全部</span>
            </button>
            <button
              type="button"
              onClick={() => setSubscriptionModalTarget(null)}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border text-sm text-foreground hover:bg-accent transition-colors"
            >
              <RadioTower className="w-4 h-4" />
              <span>新增订阅</span>
            </button>
            <button
              type="button"
              onClick={() => setSourceModalTarget(null)}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
            >
              <Plus className="w-4 h-4" />
              <span>新增 RSS 源</span>
            </button>
          </div>
        )}
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4 space-y-4">
        <div className="grid gap-3 md:grid-cols-5">
          <HealthCard
            health={healthQuery.data}
            isLoading={healthQuery.isLoading}
            onRefresh={() => void handleRefreshHealth()}
          />
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">RSS 源</p>
            <p className="text-2xl font-semibold text-foreground">{sources.length}</p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">订阅规则</p>
            <p className="text-2xl font-semibold text-foreground">{subscriptions.length}</p>
          </div>
          <div className="rounded-lg border border-border bg-card p-4">
            <p className="text-sm text-muted-foreground">当前条目</p>
            <p className="text-2xl font-semibold text-foreground">{items.length}</p>
          </div>
          <button
            type="button"
            onClick={showNeedsAttentionItems}
            className={cn(
              'rounded-lg border bg-card p-4 text-left transition-colors hover:bg-accent',
              needsAttentionItems.length > 0
                ? 'border-destructive/40 bg-destructive/5'
                : 'border-border'
            )}
            aria-pressed={statusFilter === 'needs_attention'}
          >
            <p className="text-sm text-muted-foreground flex items-center gap-1.5">
              <AlertTriangle className="w-4 h-4 text-destructive" />
              待处理
            </p>
            <p className="text-2xl font-semibold text-foreground">
              {needsAttentionQuery.isLoading ? '...' : needsAttentionItems.length}
            </p>
            {needsAttentionQuery.error && (
              <p className="mt-1 text-xs text-destructive">加载待处理条目失败</p>
            )}
          </button>
        </div>

        <div className="grid gap-4 xl:grid-cols-2">
          <Panel
            title="RSS 源"
            icon={<Rss className="w-4 h-4 text-primary" />}
            action={
              canManage && (
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => void handleRefreshAllSources()}
                    disabled={isRefreshingAll}
                    className="text-sm text-primary hover:underline disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {isRefreshingAll ? '刷新中...' : '刷新全部'}
                  </button>
                  <button
                    type="button"
                    onClick={() => setSourceModalTarget(null)}
                    className="text-sm text-primary hover:underline"
                  >
                    新增
                  </button>
                </div>
              )
            }
          >
            {sourcesQuery.error ? (
              <QueryError error={sourcesQuery.error} fallback="加载 RSS 源失败" />
            ) : sourcesQuery.isLoading ? (
              <EmptyState icon={<Loader2 className="w-10 h-10 animate-spin" />} text="正在加载 RSS 源" />
            ) : sources.length === 0 ? (
              <EmptyState icon={<Rss className="w-10 h-10" />} text="暂无 RSS 源" />
            ) : (
              <div className="space-y-2">
                {refreshAllResult && (
                  <RefreshAllResultPanel result={refreshAllResult} sourceNameById={sourceNameById} />
                )}
                {sources.map((source) => (
                  <div
                    key={source.id}
                    className={cn(
                      'rounded-lg border p-3',
                      source.health_status === 'circuit_open'
                        ? 'border-destructive/40 bg-destructive/5'
                        : source.health_status === 'degraded'
                          ? 'border-amber-500/40 bg-amber-500/5'
                          : 'border-border'
                    )}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="font-medium text-foreground truncate">{source.name}</h3>
                          <span
                            className={cn(
                              'rounded-full px-2 py-0.5 text-xs',
                              source.is_enabled
                                ? 'bg-emerald-500/10 text-emerald-500'
                                : 'bg-muted text-muted-foreground'
                            )}
                          >
                            {source.is_enabled ? '启用' : '停用'}
                          </span>
                          <SourceHealthBadge status={source.health_status} />
                        </div>
                        <p className="text-xs text-muted-foreground truncate mt-1">{source.url}</p>
                        <div className="grid gap-x-3 gap-y-1 text-xs text-muted-foreground mt-2 sm:grid-cols-2">
                          <span>最近刷新：{formatDate(source.last_refreshed_at)}</span>
                          <span>最近成功：{formatDate(source.last_success_at)}</span>
                          <span>下次刷新：{formatDate(source.next_refresh_at)}</span>
                          <span>刷新间隔：{source.refresh_interval_seconds}s</span>
                          <span>连续失败：{source.consecutive_failures}</span>
                          <span>最近状态：{refreshStatusLabel(source.last_refresh_status)}</span>
                        </div>
                        {source.last_refresh_stats && (
                          <p className="text-xs text-muted-foreground mt-2">
                            最近统计：{formatRefreshSummary(source.last_refresh_stats)}
                          </p>
                        )}
                        {source.last_error && (
                          <p className="text-xs text-destructive mt-1 break-all">{source.last_error}</p>
                        )}
                      </div>
                      {canManage && (
                        <div className="flex items-center gap-1 shrink-0">
                          <button
                            type="button"
                            onClick={() => handleRefreshSource(source)}
                            disabled={refreshingSourceId === source.id}
                            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-40 disabled:cursor-not-allowed"
                            title="手动刷新"
                          >
                            {refreshingSourceId === source.id ? (
                              <Loader2 className="w-4 h-4 animate-spin" />
                            ) : (
                              <RefreshCcw className="w-4 h-4" />
                            )}
                          </button>
                          <button
                            type="button"
                            onClick={() => setSourceModalTarget(source)}
                            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                            title="编辑"
                          >
                            <Pencil className="w-4 h-4" />
                          </button>
                          <button
                            type="button"
                            onClick={() => handleDeleteSource(source)}
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
          </Panel>

          <Panel
            title="订阅规则"
            icon={<RadioTower className="w-4 h-4 text-primary" />}
            action={
              canManage && (
                <button
                  type="button"
                  onClick={() => setSubscriptionModalTarget(null)}
                  className="text-sm text-primary hover:underline"
                >
                  新增
                </button>
              )
            }
          >
            {subscriptionsQuery.error ? (
              <QueryError error={subscriptionsQuery.error} fallback="加载订阅规则失败" />
            ) : subscriptionsQuery.isLoading ? (
              <EmptyState icon={<Loader2 className="w-10 h-10 animate-spin" />} text="正在加载订阅规则" />
            ) : subscriptions.length === 0 ? (
              <EmptyState icon={<RadioTower className="w-10 h-10" />} text="暂无订阅规则" />
            ) : (
              <div className="space-y-2">
                {subscriptions.map((subscription) => {
                  const preview = subscriptionPreviews[subscription.id]
                  return (
                    <div key={subscription.id} className="rounded-lg border border-border p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <h3 className="font-medium text-foreground truncate">{subscription.name}</h3>
                          <span
                            className={cn(
                              'rounded-full px-2 py-0.5 text-xs',
                              subscription.is_enabled
                                ? 'bg-emerald-500/10 text-emerald-500'
                                : 'bg-muted text-muted-foreground'
                            )}
                          >
                            {subscription.is_enabled ? '启用' : '停用'}
                          </span>
                        </div>
                        <p className="text-xs text-muted-foreground mt-1">
                          RSS 源：{sourceNameById.get(subscription.source_id) ?? subscription.source_id}
                        </p>
                        <p className="text-xs text-muted-foreground mt-1 font-mono">
                          目标：{subscription.target_virtual_parent_path}
                        </p>
                        <div className="flex flex-wrap gap-1 mt-2">
                          {subscription.must_contain.map((item) => (
                            <span key={item} className="rounded border border-primary/20 bg-primary/10 px-1.5 py-0.5 text-xs text-primary">
                              + {item}
                            </span>
                          ))}
                          {subscription.must_not_contain.map((item) => (
                            <span key={item} className="rounded border border-destructive/20 bg-destructive/10 px-1.5 py-0.5 text-xs text-destructive">
                              - {item}
                            </span>
                          ))}
                        </div>
                      </div>
                      {canManage && (
                        <div className="flex items-center gap-1 shrink-0">
                          <button
                            type="button"
                            onClick={() => void handlePreviewSubscription(subscription)}
                            disabled={previewingSubscriptionId === subscription.id}
                            className="px-2 py-1.5 rounded-md hover:bg-accent text-xs text-muted-foreground disabled:opacity-40 disabled:cursor-not-allowed"
                            title="预览规则"
                          >
                            {previewingSubscriptionId === subscription.id ? '预览中' : '预览'}
                          </button>
                          <button
                            type="button"
                            onClick={() => handleRunSubscription(subscription)}
                            disabled={runningSubscriptionId === subscription.id}
                            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-40 disabled:cursor-not-allowed"
                            title="手动执行"
                          >
                            {runningSubscriptionId === subscription.id ? (
                              <Loader2 className="w-4 h-4 animate-spin" />
                            ) : (
                              <Play className="w-4 h-4" />
                            )}
                          </button>
                          <button
                            type="button"
                            onClick={() => setSubscriptionModalTarget(subscription)}
                            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                            title="编辑"
                          >
                            <Pencil className="w-4 h-4" />
                          </button>
                          <button
                            type="button"
                            onClick={() => handleDeleteSubscription(subscription)}
                            className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                            title="删除"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      )}
                    </div>
                    {preview && <SubscriptionPreviewPanel preview={preview} />}
                  </div>
                  )
                })}
              </div>
            )}
          </Panel>
        </div>

        <Panel title="RSS 条目" icon={<Filter className="w-4 h-4 text-primary" />}>
          <div className="grid gap-2 md:grid-cols-3 mb-4">
            <div>
              <label htmlFor="rss-filter-source" className="text-xs text-muted-foreground mb-1 block">
                RSS 源
              </label>
              <select
                id="rss-filter-source"
                name="source_id"
                value={sourceFilter}
                onChange={(event) => {
                  setSourceFilter(event.target.value)
                  setSubscriptionFilter('')
                }}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="">全部 RSS 源</option>
                {sources.map((source) => (
                  <option key={source.id} value={source.id}>
                    {source.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="rss-filter-subscription" className="text-xs text-muted-foreground mb-1 block">
                订阅
              </label>
              <select
                id="rss-filter-subscription"
                name="subscription_id"
                value={subscriptionFilter}
                onChange={(event) => setSubscriptionFilter(event.target.value)}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="">全部订阅</option>
                {subscriptions.map((subscription) => (
                  <option key={subscription.id} value={subscription.id}>
                    {subscription.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="rss-filter-status" className="text-xs text-muted-foreground mb-1 block">
                状态
              </label>
              <select
                id="rss-filter-status"
                name="status"
                value={statusFilter}
                onChange={(event) => setStatusFilter(event.target.value as RSSItemStatus | '')}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="">全部状态</option>
                {RSS_STATUS_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {itemsQuery.error ? (
            <QueryError error={itemsQuery.error} fallback="加载 RSS 条目失败" />
          ) : itemsQuery.isLoading ? (
            <EmptyState icon={<Loader2 className="w-10 h-10 animate-spin" />} text="正在加载 RSS 条目" />
          ) : items.length === 0 ? (
            <EmptyState icon={<Rss className="w-10 h-10" />} text="暂无 RSS 条目" />
          ) : (
            <div className="space-y-2">
              {items.map((item) => {
                const disabledReason = getDownloadDisabledReason(item)
                const retryDisabledReason = getRetryDisabledReason(item)
                const isNeedsAttention = item.status === 'needs_attention'
                const isRetryPending = item.status === 'retry_pending'
                const isDownloading = activeItemAction?.id === item.id && activeItemAction.type === 'download'
                const isReprocessing = activeItemAction?.id === item.id && activeItemAction.type === 'reprocess'
                const isRetrying = activeItemAction?.id === item.id && activeItemAction.type === 'retry'
                return (
                  <div
                    key={item.id}
                    className={cn(
                      'rounded-lg border p-3',
                      isNeedsAttention
                        ? 'border-destructive/40 bg-destructive/5'
                        : isRetryPending
                          ? 'border-amber-500/40 bg-amber-500/5'
                          : 'border-border'
                    )}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        {isNeedsAttention && (
                          <div className="mb-2 flex items-start gap-2 rounded-md border border-destructive/20 bg-destructive/10 px-2 py-1.5 text-xs text-destructive">
                            <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                            <span>
                              需要人工处理：{item.error_message || retryReasonLabel(item.retry_reason)}
                            </span>
                          </div>
                        )}
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="font-medium text-foreground break-all">{item.title}</h3>
                          <StatusBadge status={item.status} />
                          <LinkTypeBadge type={item.link_type} />
                        </div>
                        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground mt-2">
                          <span>RSS 源：{sourceNameById.get(item.source_id) ?? item.source_id}</span>
                          <span>
                            订阅：{item.matched_subscription_id
                              ? subscriptionNameById.get(item.matched_subscription_id) ?? item.matched_subscription_id
                              : '-'}
                          </span>
                          <span>发布时间：{formatDate(item.published_at)}</span>
                        </div>
                        <p className="text-xs text-muted-foreground break-all mt-2">
                          下载链接：{item.download_url || item.link}
                        </p>
                        <div className="grid gap-x-3 gap-y-1 text-xs text-muted-foreground mt-2 sm:grid-cols-2">
                          <span>
                            重试次数：{item.retry_count}/{item.max_retry_count}
                          </span>
                          <span>重试原因：{retryReasonLabel(item.retry_reason)}</span>
                          <span>最近尝试：{formatDate(item.last_attempt_at)}</span>
                          <span>下次重试：{formatDate(item.next_retry_at)}</span>
                        </div>
                        {item.error_message && (
                          <p className="text-xs text-destructive mt-2 break-all">{item.error_message}</p>
                        )}
                      </div>
                      <div className="flex items-center gap-1 shrink-0">
                        {item.task_id && (
                          <button
                            type="button"
                            onClick={() => navigate(`/tasks?task_id=${item.task_id}`)}
                            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                            title={`查看任务 #${item.task_id}`}
                          >
                            <ExternalLink className="w-4 h-4" />
                          </button>
                        )}
                        {canManage && (
                          <>
                            <button
                              type="button"
                              onClick={() => void handleReprocessItem(item)}
                              disabled={isReprocessing}
                              className="px-2 py-1.5 rounded-md hover:bg-accent text-xs text-muted-foreground disabled:opacity-40 disabled:cursor-not-allowed"
                              title="重新处理/重新匹配"
                            >
                              {isReprocessing ? '处理中' : '重处理'}
                            </button>
                            <button
                              type="button"
                              onClick={() => void handleRetryItem(item)}
                              disabled={Boolean(retryDisabledReason) || isRetrying}
                              className="px-2 py-1.5 rounded-md hover:bg-accent text-xs text-muted-foreground disabled:opacity-40 disabled:cursor-not-allowed"
                              title={retryDisabledReason || '立即重试'}
                            >
                              {isRetrying ? '重试中' : '重试'}
                            </button>
                            <button
                              type="button"
                              onClick={() => void handleDownloadItem(item)}
                              disabled={Boolean(disabledReason) || isDownloading}
                              className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-40 disabled:cursor-not-allowed"
                              title={disabledReason || '手动入队'}
                            >
                              {isDownloading ? (
                                <Loader2 className="w-4 h-4 animate-spin" />
                              ) : (
                                <DownloadCloud className="w-4 h-4" />
                              )}
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </Panel>
      </div>

      {sourceModalTarget !== undefined && (
        <SourceModal
          source={sourceModalTarget}
          onClose={() => setSourceModalTarget(undefined)}
          onSubmit={handleSaveSource}
        />
      )}

      {subscriptionModalTarget !== undefined && (
        <SubscriptionModal
          subscription={subscriptionModalTarget}
          sources={sources}
          defaultSourceId={sourceIdFilter}
          onClose={() => setSubscriptionModalTarget(undefined)}
          onSubmit={handleSaveSubscription}
        />
      )}
    </div>
  )
}
