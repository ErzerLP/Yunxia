import { useEffect, useMemo, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  AlertTriangle,
  CheckSquare,
  Copy,
  DownloadCloud,
  ExternalLink,
  FileDown,
  Filter,
  Loader2,
  Pencil,
  Play,
  Plus,
  RadioTower,
  RefreshCcw,
  Rss,
  Square,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import { rssApi } from '@/api/rss'
import { useHasCapability } from '@/hooks/useCapability'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'
import { cn, formatDate, getApiErrorMessage } from '@/utils'
import type {
  RSSItemStatus,
  RSSItemBatchActionResponse,
  RSSItemView,
  RSSExportResponse,
  RSSImportRequest,
  RSSImportResponse,
  RSSQBitHealthResponse,
  RSSRefreshAllResponse,
  RSSRefreshResponse,
  RSSRefreshStatsView,
  RSSSourceUpsertRequest,
  RSSSourceView,
  RSSSubscriptionBatchStateResponse,
  RSSSubscriptionCloneRequest,
  RSSSubscriptionPreviewItem,
  RSSSubscriptionPreviewRequest,
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
  return getApiErrorMessage(error, fallback)
}

function isMetadataCommitFailureMessage(message?: string | null) {
  return (message || '').toLowerCase().includes('metadata vfs commit failed')
}

function getRSSItemIssueMessage(item: RSSItemView) {
  if (isMetadataCommitFailureMessage(item.error_message) || isMetadataCommitFailureMessage(item.retry_reason)) {
    return '文件已写入底层存储，但目录索引提交失败。请刷新目标目录或联系管理员处理。'
  }
  return getErrorMessage(item.error_message || retryReasonLabel(item.retry_reason), 'RSS 条目处理失败')
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

function formatBatchSummary(result: { succeeded: number; failed: number }) {
  return `成功 ${result.succeeded}，失败 ${result.failed}`
}

function getBatchErrorLines(
  result: RSSItemBatchActionResponse | RSSSubscriptionBatchStateResponse,
  idLabel: string
) {
  return result.items
    .filter((item) => !item.success)
    .slice(0, 5)
    .map((item) => {
      const id = 'item_id' in item ? item.item_id : item.subscription_id
      return `${idLabel} #${id}：${getErrorMessage(item.error_message || item.error_code || '失败', '失败')}`
    })
}

function hasParsedInfo(item: RSSItemView) {
  return Boolean(
    item.parsed
    && (
      item.parsed.anime_title
      || item.parsed.season
      || item.parsed.episode
      || item.parsed.subtitle_group
      || item.parsed.resolution
    )
  )
}

function matchesSubscriptionRule(text: string, rule: string, useRegex: boolean, caseSensitive: boolean) {
  if (!rule) return false
  if (useRegex) {
    try {
      return new RegExp(rule, caseSensitive ? '' : 'i').test(text)
    } catch {
      return false
    }
  }
  const haystack = caseSensitive ? text : text.toLowerCase()
  const needle = caseSensitive ? rule : rule.toLowerCase()
  return haystack.includes(needle)
}

function getItemMatchExplanation(item: RSSItemView, subscription?: RSSSubscriptionView) {
  if (subscription) {
    const searchableText = [item.title, item.download_url, item.link].filter(Boolean).join('\n')
    const matchedKeywords = subscription.must_contain.filter((rule) =>
      matchesSubscriptionRule(searchableText, rule, subscription.use_regex, subscription.case_sensitive)
    )
    const missingKeywords = subscription.must_contain.filter((rule) =>
      !matchesSubscriptionRule(searchableText, rule, subscription.use_regex, subscription.case_sensitive)
    )
    const excludedKeywords = subscription.must_not_contain.filter((rule) =>
      matchesSubscriptionRule(searchableText, rule, subscription.use_regex, subscription.case_sensitive)
    )
    const positivePart = subscription.must_contain.length === 0
      ? '未设置必须包含关键词'
      : missingKeywords.length > 0
        ? `缺失 ${subscription.use_regex ? '正则/关键词' : '关键词'}：${missingKeywords.join('、')}`
        : `命中 ${subscription.use_regex ? '正则/关键词' : '关键词'}：${matchedKeywords.join('、')}`
    const excludePart = subscription.must_not_contain.length === 0
      ? '无排除关键词'
      : excludedKeywords.length > 0
        ? `命中排除项：${excludedKeywords.join('、')}`
        : `未命中排除项：${subscription.must_not_contain.join('、')}`
    return `${positivePart}；${excludePart}`
  }

  if (item.status === 'unsupported') {
    return '未入队原因：不是 BT/magnet 或 .torrent 链接。'
  }
  if (item.status === 'ignored') {
    return '未匹配任何订阅规则；可调整订阅规则后重新处理或重试。'
  }
  if (item.status === 'new') {
    return '新条目尚未匹配订阅；刷新源或执行订阅后会更新匹配结果。'
  }
  return ''
}

function downloadJSON(filename: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
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
                {getErrorMessage(item.error, '刷新失败')}
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
  onPreview,
}: {
  subscription: RSSSubscriptionView | null
  sources: RSSSourceView[]
  defaultSourceId?: number
  onClose: () => void
  onSubmit: (data: RSSSubscriptionUpsertRequest) => Promise<void>
  onPreview: (data: RSSSubscriptionPreviewRequest) => Promise<RSSSubscriptionPreviewResponse>
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
  const [directoryTemplate, setDirectoryTemplate] = useState(subscription?.directory_template ?? '')
  const [filenameTemplate, setFilenameTemplate] = useState(subscription?.filename_template ?? '')
  const [preview, setPreview] = useState<RSSSubscriptionPreviewResponse | null>(null)
  const [error, setError] = useState('')
  const [isPreviewing, setIsPreviewing] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const buildPreviewPayload = (): RSSSubscriptionPreviewRequest | null => {
    const parsedSourceId = Number(sourceId)
    if (!parsedSourceId) {
      setError('请选择 RSS 源后再预览')
      return null
    }
    return {
      source_id: parsedSourceId,
      must_contain: parseListText(mustContain),
      must_not_contain: parseListText(mustNotContain),
      use_regex: useRegex,
      case_sensitive: caseSensitive,
    }
  }

  const handlePreview = async () => {
    const payload = buildPreviewPayload()
    if (!payload) return

    setIsPreviewing(true)
    setError('')
    try {
      const result = await onPreview(payload)
      setPreview(result)
    } catch (err: unknown) {
      setError(getErrorMessage(err, '预览订阅规则失败'))
    } finally {
      setIsPreviewing(false)
    }
  }

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
        directory_template: directoryTemplate.trim(),
        filename_template: filenameTemplate.trim(),
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
              <label htmlFor={`rss-sub-directory-template-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
                目录模板
              </label>
              <input
                id={`rss-sub-directory-template-${suffix}`}
                name="directory_template"
                type="text"
                value={directoryTemplate}
                onChange={(event) => setDirectoryTemplate(event.target.value)}
                autoComplete="off"
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                placeholder="{anime_title}/{season}"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                留空保持旧行为；只能填写相对路径，不要包含 <code>..</code>、绝对路径或反斜杠。
              </p>
            </div>
            <div>
              <label htmlFor={`rss-sub-filename-template-${suffix}`} className="text-sm text-muted-foreground mb-1 block">
                文件名模板
              </label>
              <input
                id={`rss-sub-filename-template-${suffix}`}
                name="filename_template"
                type="text"
                value={filenameTemplate}
                onChange={(event) => setFilenameTemplate(event.target.value)}
                autoComplete="off"
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                placeholder="{anime_title} - {episode} [{resolution}]"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                单文件 RSS 下载导入完成时会实际重命名；多文件 torrent 保持原目录结构。
              </p>
            </div>
          </div>

          <div className="rounded-md border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
            支持占位符：<code>{'{anime_title}'}</code>、<code>{'{season}'}</code>、<code>{'{episode}'}</code>、<code>{'{subtitle_group}'}</code>、<code>{'{resolution}'}</code>、<code>{'{title}'}</code>。
            目录模板会影响后续 RSS 入队任务的目标子目录；文件名模板会写入任务 <code>target_filename</code> 快照。
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
              onClick={() => void handlePreview()}
              disabled={isPreviewing || sources.length === 0}
              className="mr-auto px-3 py-1.5 rounded-md border border-border text-sm text-foreground hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isPreviewing ? '预览中...' : '预览当前规则'}
            </button>
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
          {preview && <SubscriptionPreviewPanel preview={preview} />}
        </form>
      </div>
    </div>
  )
}

function CloneSubscriptionModal({
  subscription,
  onClose,
  onSubmit,
}: {
  subscription: RSSSubscriptionView
  onClose: () => void
  onSubmit: (data: RSSSubscriptionCloneRequest) => Promise<void>
}) {
  const [name, setName] = useState(`${subscription.name} Copy`)
  const [overrideEnabled, setOverrideEnabled] = useState(false)
  const [isEnabled, setIsEnabled] = useState(subscription.is_enabled)
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setIsSubmitting(true)
    setError('')
    try {
      await onSubmit({
        name: name.trim() || undefined,
        is_enabled: overrideEnabled ? isEnabled : undefined,
      })
      onClose()
    } catch (err: unknown) {
      setError(getErrorMessage(err, '复制订阅失败'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-md bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Copy className="w-4 h-4" />
            复制订阅
          </h3>
          <button type="button" onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground" title="关闭">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          {error && <p className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          <p className="text-sm text-muted-foreground">
            将复制「{subscription.name}」的匹配规则、目标路径和模板，不会修改原订阅。
          </p>
          <div>
            <label htmlFor="rss-clone-name" className="text-sm text-muted-foreground mb-1 block">
              新订阅名称
            </label>
            <input
              id="rss-clone-name"
              name="name"
              type="text"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div className="space-y-2 rounded-md border border-border bg-muted/30 p-3">
            <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
              <input
                id="rss-clone-override-enabled"
                name="override_enabled"
                type="checkbox"
                checked={overrideEnabled}
                onChange={(event) => setOverrideEnabled(event.target.checked)}
                className="rounded border-border"
              />
              覆盖新订阅启用状态
            </label>
            <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
              <input
                id="rss-clone-enabled"
                name="is_enabled"
                type="checkbox"
                checked={isEnabled}
                disabled={!overrideEnabled}
                onChange={(event) => setIsEnabled(event.target.checked)}
                className="rounded border-border disabled:opacity-50"
              />
              新订阅启用
            </label>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors">
              取消
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isSubmitting ? '复制中...' : '复制'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function RSSImportResultPanel({ result }: { result: RSSImportResponse }) {
  const sections = [
    { key: 'sources', label: 'RSS 源', result: result.sources },
    { key: 'subscriptions', label: '订阅', result: result.subscriptions },
  ]

  return (
    <div className="rounded-lg border border-border bg-muted/30 p-3 space-y-3 text-xs">
      <div className="flex flex-wrap items-center gap-2 text-muted-foreground">
        <span>{result.dry_run ? '预检结果' : '导入结果'}</span>
        {sections.map((section) => (
          <span key={section.key}>
            {section.label}：创建 {section.result.created}，复用 {section.result.reused}，跳过 {section.result.skipped}，失败 {section.result.failed}
          </span>
        ))}
      </div>
      {sections.map((section) => (
        <div key={section.key} className="space-y-1">
          <p className="font-medium text-foreground">{section.label}</p>
          {section.result.items.slice(0, 8).map((item) => (
            <div key={`${section.key}-${item.index}`} className="rounded border border-border bg-card px-2 py-1">
              <span className={item.success ? 'text-emerald-500' : 'text-destructive'}>
                #{item.index} {item.action}
              </span>
              <span className="ml-2 text-muted-foreground">{item.name || item.source_url || item.id || '-'}</span>
              {!item.success && (
                <span className="ml-2 text-destructive">
                  {getErrorMessage(item.error_message || item.error_code || '失败', '失败')}
                </span>
              )}
            </div>
          ))}
          {section.result.items.length > 8 && (
            <p className="text-muted-foreground">仅展示前 8 条结果。</p>
          )}
        </div>
      ))}
    </div>
  )
}

function RSSImportModal({
  onClose,
  onDryRun,
  onImport,
}: {
  onClose: () => void
  onDryRun: (data: RSSImportRequest) => Promise<RSSImportResponse>
  onImport: (data: RSSImportRequest) => Promise<RSSImportResponse>
}) {
  const [rawJSON, setRawJSON] = useState('')
  const [result, setResult] = useState<RSSImportResponse | null>(null)
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const parsePayload = (dryRun: boolean): RSSImportRequest | null => {
    try {
      const parsed = JSON.parse(rawJSON) as Partial<RSSImportRequest & RSSExportResponse>
      if (!Array.isArray(parsed.sources) || !Array.isArray(parsed.subscriptions)) {
        setError('JSON 必须包含 sources[] 与 subscriptions[]')
        return null
      }
      return {
        dry_run: dryRun,
        sources: parsed.sources,
        subscriptions: parsed.subscriptions,
      }
    } catch {
      setError('JSON 格式不正确')
      return null
    }
  }

  const handleFile = async (file?: File) => {
    if (!file) return
    setRawJSON(await file.text())
  }

  const submit = async (dryRun: boolean) => {
    const payload = parsePayload(dryRun)
    if (!payload) return
    setIsSubmitting(true)
    setError('')
    try {
      const response = dryRun ? await onDryRun(payload) : await onImport(payload)
      setResult(response)
    } catch (err: unknown) {
      setError(getErrorMessage(err, dryRun ? '导入预检失败' : '导入失败'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-3xl bg-card border border-border rounded-lg shadow-xl max-h-[90vh] overflow-auto">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border sticky top-0 bg-card">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Upload className="w-4 h-4" />
            导入 RSS 配置
          </h3>
          <button type="button" onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground" title="关闭">
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className="p-4 space-y-3">
          {error && <p className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          <div>
            <label htmlFor="rss-import-file" className="text-sm text-muted-foreground mb-1 block">
              选择导出的 JSON 文件
            </label>
            <input
              id="rss-import-file"
              name="rss_import_file"
              type="file"
              accept="application/json,.json"
              onChange={(event) => void handleFile(event.target.files?.[0])}
              className="w-full text-sm text-muted-foreground"
            />
          </div>
          <div>
            <label htmlFor="rss-import-json" className="text-sm text-muted-foreground mb-1 block">
              或粘贴 JSON
            </label>
            <textarea
              id="rss-import-json"
              name="rss_import_json"
              value={rawJSON}
              onChange={(event) => setRawJSON(event.target.value)}
              rows={12}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-xs focus:outline-none focus:ring-2 focus:ring-ring font-mono"
              placeholder='{"version":1,"sources":[],"subscriptions":[]}'
            />
          </div>
          {result && <RSSImportResultPanel result={result} />}
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors">
              关闭
            </button>
            <button
              type="button"
              onClick={() => void submit(true)}
              disabled={!rawJSON.trim() || isSubmitting}
              className="px-3 py-1.5 rounded-md border border-border text-sm text-foreground hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              dry-run 预检
            </button>
            <button
              type="button"
              onClick={() => void submit(false)}
              disabled={!rawJSON.trim() || isSubmitting}
              className="px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isSubmitting ? '处理中...' : '正式导入'}
            </button>
          </div>
        </div>
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
  const [cloneModalTarget, setCloneModalTarget] = useState<RSSSubscriptionView | null>(null)
  const [importModalOpen, setImportModalOpen] = useState(false)
  const [isExportingConfig, setIsExportingConfig] = useState(false)
  const [selectedSubscriptionIds, setSelectedSubscriptionIds] = useState<Set<number>>(() => new Set())
  const [selectedItemIds, setSelectedItemIds] = useState<Set<number>>(() => new Set())
  const [subscriptionBatchResult, setSubscriptionBatchResult] = useState<RSSSubscriptionBatchStateResponse | null>(null)
  const [itemBatchResult, setItemBatchResult] = useState<RSSItemBatchActionResponse | null>(null)
  const [isBatchingSubscriptions, setIsBatchingSubscriptions] = useState(false)
  const [isBatchingItems, setIsBatchingItems] = useState(false)
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
  const subscriptionById = useMemo(
    () => new Map(subscriptions.map((subscription) => [subscription.id, subscription])),
    [subscriptions]
  )
  const subscriptionIds = useMemo(() => subscriptions.map((subscription) => subscription.id), [subscriptions])
  const validSelectedSubscriptionIds = useMemo(() => {
    const validIds = new Set(subscriptionIds)
    return new Set([...selectedSubscriptionIds].filter((id) => validIds.has(id)))
  }, [selectedSubscriptionIds, subscriptionIds])
  const itemIds = useMemo(() => items.map((item) => item.id), [items])
  const validSelectedItemIds = useMemo(() => {
    const validIds = new Set(itemIds)
    return new Set([...selectedItemIds].filter((id) => validIds.has(id)))
  }, [selectedItemIds, itemIds])

  const invalidateRSS = () => {
    void queryClient.invalidateQueries({ queryKey: ['rss'] })
  }

  const showNeedsAttentionItems = () => {
    setSourceFilter('')
    setSubscriptionFilter('')
    setStatusFilter('needs_attention')
  }

  const toggleSubscriptionSelection = (id: number) => {
    setSelectedSubscriptionIds((current) => {
      const next = new Set(current)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleAllSubscriptions = () => {
    setSelectedSubscriptionIds(() => {
      if (validSelectedSubscriptionIds.size === subscriptionIds.length) return new Set()
      return new Set(subscriptionIds)
    })
  }

  const toggleItemSelection = (id: number) => {
    setSelectedItemIds((current) => {
      const next = new Set(current)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleAllItems = () => {
    setSelectedItemIds(() => {
      if (validSelectedItemIds.size === itemIds.length) return new Set()
      return new Set(itemIds)
    })
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

  const handlePreviewSubscriptionDraft = async (payload: RSSSubscriptionPreviewRequest) => {
    const result = await rssApi.previewSubscriptionDraft(payload)
    addToast(
      `规则预览完成：命中 ${result.matched}，缺失 ${result.missing}，排除 ${result.excluded}`,
      'success',
      6000
    )
    return result
  }

  const handleCloneSubscription = async (payload: RSSSubscriptionCloneRequest) => {
    if (!cloneModalTarget) return
    await rssApi.cloneSubscription(cloneModalTarget.id, payload)
    addToast('订阅已复制', 'success')
    setCloneModalTarget(null)
    invalidateRSS()
  }

  const handleBatchSubscriptionState = async (isEnabled: boolean) => {
    const ids = [...validSelectedSubscriptionIds]
    if (ids.length === 0) {
      addToast('请先选择订阅', 'warning')
      return
    }

    setIsBatchingSubscriptions(true)
    setSubscriptionBatchResult(null)
    try {
      const result = await rssApi.batchSubscriptionState({
        subscription_ids: ids,
        is_enabled: isEnabled,
      })
      setSubscriptionBatchResult(result)
      addToast(`批量${isEnabled ? '启用' : '禁用'}完成：${formatBatchSummary(result)}`, result.failed > 0 ? 'warning' : 'success', 7000)
      if (result.failed > 0) {
        getBatchErrorLines(result, '订阅').forEach((line) => addToast(line, 'error', 8000))
      }
      setSelectedSubscriptionIds(new Set())
      invalidateRSS()
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '批量更新订阅状态失败'), 'error')
    } finally {
      setIsBatchingSubscriptions(false)
    }
  }

  const handleExportConfig = async () => {
    setIsExportingConfig(true)
    try {
      const result = await rssApi.exportConfig()
      downloadJSON(`yunxia-rss-export-${new Date().toISOString().slice(0, 10)}.json`, result)
      addToast(`RSS 配置已导出：${result.sources.length} 个源，${result.subscriptions.length} 个订阅`, 'success')
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '导出 RSS 配置失败'), 'error')
    } finally {
      setIsExportingConfig(false)
    }
  }

  const handleImportConfig = async (payload: RSSImportRequest) => {
    const result = await rssApi.importConfig(payload)
    addToast(
      `${payload.dry_run ? '导入预检' : '导入'}完成：RSS 源失败 ${result.sources.failed}，订阅失败 ${result.subscriptions.failed}`,
      result.sources.failed + result.subscriptions.failed > 0 ? 'warning' : 'success',
      7000
    )
    if (!payload.dry_run) {
      invalidateRSS()
    }
    return result
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

  const handleOpenRSSResultDirectory = (item: RSSItemView, subscription?: RSSSubscriptionView) => {
    if (!item.result_vfs_node_id) {
      addToast('该条目没有返回结果节点，暂不能定位结果', 'warning')
      return
    }
    const targetPath = subscription?.target_virtual_parent_path
    if (!targetPath) {
      addToast(`结果节点 #${item.result_vfs_node_id} 已记录，但缺少可打开的订阅目标目录`, 'warning')
      return
    }
    void queryClient.invalidateQueries({ queryKey: ['vfs', targetPath] })
    navigate(`/files?path=${encodeURIComponent(targetPath)}`)
  }

  const handleBatchIgnoreItems = async () => {
    const ids = [...validSelectedItemIds]
    if (ids.length === 0) {
      addToast('请先选择 RSS 条目', 'warning')
      return
    }

    setIsBatchingItems(true)
    setItemBatchResult(null)
    try {
      const result = await rssApi.batchIgnoreItems(ids)
      setItemBatchResult(result)
      addToast(`批量忽略完成：${formatBatchSummary(result)}`, result.failed > 0 ? 'warning' : 'success', 7000)
      if (result.failed > 0) {
        getBatchErrorLines(result, '条目').forEach((line) => addToast(line, 'error', 8000))
      }
      setSelectedItemIds(new Set())
      invalidateRSS()
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '批量忽略失败'), 'error')
    } finally {
      setIsBatchingItems(false)
    }
  }

  const handleBatchRetryItems = async () => {
    const ids = [...validSelectedItemIds]
    if (ids.length === 0) {
      addToast('请先选择 RSS 条目', 'warning')
      return
    }

    setIsBatchingItems(true)
    setItemBatchResult(null)
    try {
      const result = await rssApi.batchRetryItems(ids, subscriptionIdFilter)
      setItemBatchResult(result)
      addToast(`批量重试完成：${formatBatchSummary(result)}`, result.failed > 0 ? 'warning' : 'success', 7000)
      if (result.failed > 0) {
        getBatchErrorLines(result, '条目').forEach((line) => addToast(line, 'error', 8000))
      }
      setSelectedItemIds(new Set())
      invalidateRSS()
      void queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch (err: unknown) {
      addToast(getErrorMessage(err, '批量重试失败'), 'error')
    } finally {
      setIsBatchingItems(false)
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
              onClick={() => void handleExportConfig()}
              disabled={isExportingConfig}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border text-sm text-foreground hover:bg-accent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isExportingConfig ? <Loader2 className="w-4 h-4 animate-spin" /> : <FileDown className="w-4 h-4" />}
              <span>导出配置</span>
            </button>
            <button
              type="button"
              onClick={() => setImportModalOpen(true)}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border text-sm text-foreground hover:bg-accent transition-colors"
            >
              <Upload className="w-4 h-4" />
              <span>导入配置</span>
            </button>
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
                          <p className="text-xs text-destructive mt-1 break-all">
                            {getErrorMessage(source.last_error, 'RSS 源刷新失败')}
                          </p>
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
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => void handleBatchSubscriptionState(true)}
                    disabled={validSelectedSubscriptionIds.size === 0 || isBatchingSubscriptions}
                    className="text-sm text-primary hover:underline disabled:opacity-40 disabled:no-underline"
                  >
                    批量启用
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleBatchSubscriptionState(false)}
                    disabled={validSelectedSubscriptionIds.size === 0 || isBatchingSubscriptions}
                    className="text-sm text-primary hover:underline disabled:opacity-40 disabled:no-underline"
                  >
                    批量禁用
                  </button>
                  <button
                    type="button"
                    onClick={() => setSubscriptionModalTarget(null)}
                    className="text-sm text-primary hover:underline"
                  >
                    新增
                  </button>
                </div>
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
                {canManage && (
                  <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                    <button
                      type="button"
                      onClick={toggleAllSubscriptions}
                      className="flex items-center gap-1 text-foreground hover:text-primary"
                    >
                      {validSelectedSubscriptionIds.size === subscriptions.length ? (
                        <CheckSquare className="w-4 h-4" />
                      ) : (
                        <Square className="w-4 h-4" />
                      )}
                      已选 {validSelectedSubscriptionIds.size} / {subscriptions.length}
                    </button>
                    {subscriptionBatchResult && (
                      <span>
                        最近批量结果：{formatBatchSummary(subscriptionBatchResult)}
                      </span>
                    )}
                  </div>
                )}
                {subscriptions.map((subscription) => {
                  const preview = subscriptionPreviews[subscription.id]
                  return (
                    <div key={subscription.id} className="rounded-lg border border-border p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          {canManage && (
                            <button
                              type="button"
                              onClick={() => toggleSubscriptionSelection(subscription.id)}
                              className="text-muted-foreground hover:text-primary"
                              title={validSelectedSubscriptionIds.has(subscription.id) ? '取消选择' : '选择订阅'}
                            >
                              {validSelectedSubscriptionIds.has(subscription.id) ? (
                                <CheckSquare className="w-4 h-4" />
                              ) : (
                                <Square className="w-4 h-4" />
                              )}
                            </button>
                          )}
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
                        {(subscription.directory_template || subscription.filename_template) && (
                          <div className="mt-1 space-y-0.5 text-xs text-muted-foreground">
                            {subscription.directory_template && (
                              <p className="font-mono">目录模板：{subscription.directory_template}</p>
                            )}
                            {subscription.filename_template && (
                              <p className="font-mono">文件名模板：{subscription.filename_template}</p>
                            )}
                          </div>
                        )}
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
                            onClick={() => setCloneModalTarget(subscription)}
                            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                            title="复制订阅"
                          >
                            <Copy className="w-4 h-4" />
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

        <Panel
          title="RSS 条目"
          icon={<Filter className="w-4 h-4 text-primary" />}
          action={
            canManage && (
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => void handleBatchIgnoreItems()}
                  disabled={validSelectedItemIds.size === 0 || isBatchingItems}
                  className="text-sm text-primary hover:underline disabled:opacity-40 disabled:no-underline"
                >
                  批量忽略
                </button>
                <button
                  type="button"
                  onClick={() => void handleBatchRetryItems()}
                  disabled={validSelectedItemIds.size === 0 || isBatchingItems}
                  className="text-sm text-primary hover:underline disabled:opacity-40 disabled:no-underline"
                >
                  批量重试
                </button>
              </div>
            )
          }
        >
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
              {canManage && (
                <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                  <button
                    type="button"
                    onClick={toggleAllItems}
                    className="flex items-center gap-1 text-foreground hover:text-primary"
                  >
                    {validSelectedItemIds.size === items.length ? (
                      <CheckSquare className="w-4 h-4" />
                    ) : (
                      <Square className="w-4 h-4" />
                    )}
                    已选 {validSelectedItemIds.size} / {items.length}
                  </button>
                  {itemBatchResult && (
                    <span>
                      最近批量结果：{formatBatchSummary(itemBatchResult)}
                    </span>
                  )}
                  {subscriptionIdFilter ? (
                    <span>批量重试会使用当前订阅 #{subscriptionIdFilter}</span>
                  ) : (
                    <span>批量重试未选择订阅时使用条目已匹配订阅。</span>
                  )}
                </div>
              )}
              {items.map((item) => {
                const disabledReason = getDownloadDisabledReason(item)
                const retryDisabledReason = getRetryDisabledReason(item)
                const matchedSubscription = item.matched_subscription_id
                  ? subscriptionById.get(item.matched_subscription_id)
                  : undefined
                const selectedFilterSubscription = subscriptionIdFilter
                  ? subscriptionById.get(subscriptionIdFilter)
                  : undefined
                const matchExplanation = getItemMatchExplanation(
                  item,
                  matchedSubscription ?? selectedFilterSubscription
                )
                const isNeedsAttention = item.status === 'needs_attention'
                const isRetryPending = item.status === 'retry_pending'
                const completedWithResultNode = item.status === 'completed' && Boolean(item.result_vfs_node_id)
                const completedWithoutResultNode = item.status === 'completed' && !item.result_vfs_node_id
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
                              需要人工处理：{getRSSItemIssueMessage(item)}
                            </span>
                          </div>
                        )}
                        <div className="flex flex-wrap items-center gap-2">
                          {canManage && (
                            <button
                              type="button"
                              onClick={() => toggleItemSelection(item.id)}
                              className="text-muted-foreground hover:text-primary"
                              title={validSelectedItemIds.has(item.id) ? '取消选择' : '选择条目'}
                            >
                              {validSelectedItemIds.has(item.id) ? (
                                <CheckSquare className="w-4 h-4" />
                              ) : (
                                <Square className="w-4 h-4" />
                              )}
                            </button>
                          )}
                          <h3 className="font-medium text-foreground break-all">{item.title}</h3>
                          <StatusBadge status={item.status} />
                          <LinkTypeBadge type={item.link_type} />
                        </div>
                        {hasParsedInfo(item) && (
                          <div className="mt-2 flex flex-wrap gap-1 text-xs">
                            {item.parsed.anime_title && (
                              <span className="rounded border border-primary/20 bg-primary/10 px-1.5 py-0.5 text-primary">
                                番剧：{item.parsed.anime_title}
                              </span>
                            )}
                            {item.parsed.season && (
                              <span className="rounded border border-border bg-muted px-1.5 py-0.5 text-muted-foreground">
                                季度：{item.parsed.season}
                              </span>
                            )}
                            {item.parsed.episode && (
                              <span className="rounded border border-border bg-muted px-1.5 py-0.5 text-muted-foreground">
                                集数：{item.parsed.episode}
                              </span>
                            )}
                            {item.parsed.subtitle_group && (
                              <span className="rounded border border-border bg-muted px-1.5 py-0.5 text-muted-foreground">
                                字幕组：{item.parsed.subtitle_group}
                              </span>
                            )}
                            {item.parsed.resolution && (
                              <span className="rounded border border-border bg-muted px-1.5 py-0.5 text-muted-foreground">
                                分辨率：{item.parsed.resolution}
                              </span>
                            )}
                          </div>
                        )}
                        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground mt-2">
                          <span>RSS 源：{sourceNameById.get(item.source_id) ?? item.source_id}</span>
                          <span>
                            订阅：{item.matched_subscription_id
                              ? subscriptionNameById.get(item.matched_subscription_id) ?? item.matched_subscription_id
                              : '-'}
                          </span>
                          <span>发布时间：{formatDate(item.published_at)}</span>
                          {completedWithResultNode && (
                            <span className="text-emerald-500">结果节点 #{item.result_vfs_node_id}</span>
                          )}
                        </div>
                        {completedWithoutResultNode && (
                          <div className="mt-2 flex items-start gap-2 rounded-md border border-amber-500/20 bg-amber-500/10 px-2 py-1.5 text-xs text-amber-600">
                            <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                            <span>该条目已完成但未返回结果节点，请刷新 RSS 或目标目录确认。</span>
                          </div>
                        )}
                        <p className="text-xs text-muted-foreground break-all mt-2">
                          下载链接：{item.download_url || item.link}
                        </p>
                        {matchExplanation && (
                          <div className="mt-2 rounded-md border border-border bg-muted/30 px-2 py-1.5 text-xs text-muted-foreground">
                            <span className="font-medium text-foreground">匹配说明：</span>
                            <span className="break-all">{matchExplanation}</span>
                          </div>
                        )}
                        <div className="grid gap-x-3 gap-y-1 text-xs text-muted-foreground mt-2 sm:grid-cols-2">
                          <span>
                            重试次数：{item.retry_count}/{item.max_retry_count}
                          </span>
                          <span>重试原因：{retryReasonLabel(item.retry_reason)}</span>
                          <span>最近尝试：{formatDate(item.last_attempt_at)}</span>
                          <span>下次重试：{formatDate(item.next_retry_at)}</span>
                        </div>
                        {item.error_message && (
                          <p className="text-xs text-destructive mt-2 break-all">{getRSSItemIssueMessage(item)}</p>
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
                        {completedWithResultNode && (
                          <button
                            type="button"
                            onClick={() => handleOpenRSSResultDirectory(item, matchedSubscription)}
                            className="px-2 py-1.5 rounded-md border border-border hover:bg-accent text-xs text-foreground"
                            title={`打开结果节点 #${item.result_vfs_node_id} 所在订阅目录`}
                          >
                            打开结果目录
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
          onPreview={handlePreviewSubscriptionDraft}
        />
      )}

      {cloneModalTarget && (
        <CloneSubscriptionModal
          subscription={cloneModalTarget}
          onClose={() => setCloneModalTarget(null)}
          onSubmit={handleCloneSubscription}
        />
      )}

      {importModalOpen && (
        <RSSImportModal
          onClose={() => setImportModalOpen(false)}
          onDryRun={(payload) => handleImportConfig({ ...payload, dry_run: true })}
          onImport={(payload) => handleImportConfig({ ...payload, dry_run: false })}
        />
      )}
    </div>
  )
}
