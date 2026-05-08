import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { sourceApi } from '@/api/source'
import { systemApi } from '@/api/system'
import { HardDrive, Plus, CheckCircle2, XCircle, AlertCircle, Trash2, RefreshCw, X, Pencil, Link2, Copy, Lock, Unlock, Eye, EyeOff, ExternalLink } from 'lucide-react'
import { cn, formatBytes, getApiErrorDetailString, getApiErrorMessage } from '@/utils'
import { useFileStore } from '@/stores/fileStore'
import { useUIStore } from '@/stores/uiStore'
import { useHasCapability } from '@/hooks/useCapability'
import type { SourceDetailResponse, StorageSource, UpdateSourceRequest } from '@/types/api'
import { buildSourceWebDAVUrl, isWebDAVOriginPromotedToHttps } from '@/utils/webdav'

type SourceDriverType = 'local' | 's3' | 'pikpak'
type PikPakPlatform = 'web' | 'android' | 'pc'
type SecretField = 'username' | 'password' | 'refresh_token' | 'captcha_token' | 'device_id'

const SOURCE_DRIVER_OPTIONS: Array<{ value: SourceDriverType; label: string }> = [
  { value: 'local', label: '本地' },
  { value: 's3', label: 'S3' },
  { value: 'pikpak', label: 'PikPak' },
]

const PIKPAK_SECRET_FIELDS: Array<{ key: SecretField; label: string; autoComplete: string }> = [
  { key: 'username', label: 'PikPak 账号', autoComplete: 'username' },
  { key: 'password', label: 'PikPak 密码', autoComplete: 'new-password' },
  { key: 'refresh_token', label: 'Refresh Token', autoComplete: 'off' },
  { key: 'captcha_token', label: 'Captcha Token', autoComplete: 'off' },
  { key: 'device_id', label: 'Device ID', autoComplete: 'off' },
]

function StatusBadge({ status }: { status: StorageSource['status'] }) {
  const config = {
    online: { icon: CheckCircle2, class: 'text-emerald-500 bg-emerald-500/10', label: '在线' },
    offline: { icon: XCircle, class: 'text-muted-foreground bg-muted', label: '离线' },
    error: { icon: AlertCircle, class: 'text-destructive bg-destructive/10', label: '错误' },
  }
  const { icon: Icon, class: cls, label } = config[status]
  return (
    <span className={cn('inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium', cls)}>
      <Icon className="w-3 h-3" />
      {label}
    </span>
  )
}

function getCreateSourceErrorMessage(err: unknown) {
  const fallback = '创建存储源失败'
  const rawMessage = err instanceof Error ? err.message : ''

  const message = rawMessage.toLowerCase()
  if (
    message.includes('web_dav_slug') &&
    (message.includes('unique constraint') || message.includes('constraint failed'))
  ) {
    return 'WebDAV 访问标识冲突：请换一个包含英文字母或数字的存储源名称后重试，例如 local-disk-2。'
  }
  if (message.includes('source name conflict') || message.includes('storage_source_models.name')) {
    return '存储源名称已存在，请换一个名称。'
  }
  if (message.includes('source mount path conflict') || message.includes('storage_source_models.mount_path')) {
    return '挂载路径已被其他存储源占用，请换一个挂载路径。'
  }
  if (message.includes('unique constraint') || message.includes('constraint failed')) {
    return '存储源配置存在重复项，请检查名称、挂载路径后重试。'
  }

  return getApiErrorMessage(err, fallback)
}

function getLocalBasePath(config: Record<string, unknown> | undefined) {
  const value = config?.base_path
  return typeof value === 'string' && value.trim() ? value : ''
}

function getConfigString(config: Record<string, unknown> | undefined, key: string, fallback = '') {
  const value = config?.[key]
  return typeof value === 'string' ? value : fallback
}

function getConfigBool(config: Record<string, unknown> | undefined, key: string, fallback: boolean) {
  const value = config?.[key]
  return typeof value === 'boolean' ? value : fallback
}

function getConfigNumber(config: Record<string, unknown> | undefined, key: string, fallback: number) {
  const value = config?.[key]
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return fallback
}

function getDriverLabel(driverType: string) {
  if (driverType === 'pikpak') return 'PikPak'
  if (driverType === 's3') return 'S3'
  if (driverType === 'local') return '本地'
  return driverType
}

function toPositiveInt(value: string, fallback: number) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : fallback
}

function isValidPikPakProxyUrl(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return true
  try {
    const url = new URL(trimmed)
    return (url.protocol === 'http:' || url.protocol === 'https:')
      && !url.username
      && !url.password
      && !url.search
      && !url.hash
  } catch {
    return false
  }
}

async function writeTextToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

function getSecretDisplay(detail: SourceDetailResponse | undefined, field: SecretField, canReadSecrets: boolean) {
  const visibleSecret = canReadSecrets ? getConfigString(detail?.config, field) : ''
  if (visibleSecret) return visibleSecret
  const mask = detail?.secret_fields[field]
  if (!mask?.configured) return '未配置'
  return mask.masked || '已配置'
}

function SourceConfigRows({ source, canReadSecrets }: { source: StorageSource; canReadSecrets: boolean }) {
  const { data, isLoading } = useQuery({
    queryKey: ['source-detail', source.id],
    queryFn: () => sourceApi.get(source.id),
  })

  const row = (label: string, value: string | number | boolean | null | undefined) => (
    <div className="flex justify-between gap-3 text-sm">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right text-foreground" title={typeof value === 'string' ? value : undefined}>
        {value === true ? '是' : value === false ? '否' : value || (isLoading ? '加载中...' : '未配置')}
      </span>
    </div>
  )

  if (source.driver_type === 'local') {
    const basePath = getLocalBasePath(data?.config)
    return <>{row('本地硬盘路径', basePath)}</>
  }

  if (source.driver_type === 'pikpak') {
    return (
      <>
        {row('Root Folder ID', getConfigString(data?.config, 'root_folder_id') || '账号根目录')}
        {row('平台', getConfigString(data?.config, 'platform', 'web'))}
        {row('缓存 TTL', `${getConfigNumber(data?.config, 'cache_ttl_seconds', 300)} 秒`)}
        {row('代理地址', getConfigString(data?.config, 'proxy_url') || '使用后端默认代理')}
        <div className="rounded-md border border-border bg-muted/20 p-2 space-y-1 text-xs text-muted-foreground">
          <div className="font-medium text-foreground flex items-center gap-1.5">
            {canReadSecrets ? <Eye className="w-3.5 h-3.5" /> : <EyeOff className="w-3.5 h-3.5" />}
            PikPak 敏感字段
          </div>
          {PIKPAK_SECRET_FIELDS.map((field) => (
            <div key={field.key} className="flex justify-between gap-2">
              <span>{field.label}</span>
              <span className="font-mono text-foreground truncate" title={getSecretDisplay(data, field.key, canReadSecrets)}>
                {getSecretDisplay(data, field.key, canReadSecrets)}
              </span>
            </div>
          ))}
        </div>
      </>
    )
  }

  return null
}

function EditSourceModal({
  onClose,
  onSuccess,
  source,
}: {
  onClose: () => void
  onSuccess: () => void
  source: StorageSource
}) {
  const { addToast } = useUIStore()
  const canReadSecrets = useHasCapability('source.secret.read')
  const [name, setName] = useState(source.name)
  const [mountPath, setMountPath] = useState(source.mount_path)
  const [rootPath, setRootPath] = useState(source.root_path)
  const [isEnabled, setIsEnabled] = useState(source.is_enabled)
  const [isWebDAVExposed, setIsWebDAVExposed] = useState(source.is_webdav_exposed)
  const [webDAVReadOnly, setWebDAVReadOnly] = useState(source.webdav_read_only)
  const [basePath, setBasePath] = useState('')
  const [basePathTouched, setBasePathTouched] = useState(false)
  const [pikPakRootFolderId, setPikPakRootFolderId] = useState('')
  const [pikPakRootFolderIdTouched, setPikPakRootFolderIdTouched] = useState(false)
  const [pikPakPlatform, setPikPakPlatform] = useState<PikPakPlatform>('web')
  const [pikPakPlatformTouched, setPikPakPlatformTouched] = useState(false)
  const [pikPakDisableMediaLink, setPikPakDisableMediaLink] = useState(true)
  const [pikPakDisableMediaLinkTouched, setPikPakDisableMediaLinkTouched] = useState(false)
  const [pikPakCacheTtlSeconds, setPikPakCacheTtlSeconds] = useState('300')
  const [pikPakCacheTtlSecondsTouched, setPikPakCacheTtlSecondsTouched] = useState(false)
  const [pikPakProxyUrl, setPikPakProxyUrl] = useState('')
  const [pikPakProxyUrlTouched, setPikPakProxyUrlTouched] = useState(false)
  const [secretPatch, setSecretPatch] = useState<Partial<Record<SecretField, string>>>({})
  const [secretClear, setSecretClear] = useState<Partial<Record<SecretField, boolean>>>({})
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState('')

  const { data: detail, isLoading: detailLoading } = useQuery({
    queryKey: ['source-detail', source.id],
    queryFn: () => sourceApi.get(source.id),
  })
  const effectiveBasePath = basePathTouched ? basePath : getLocalBasePath(detail?.config)
  const effectivePikPakRootFolderId = pikPakRootFolderIdTouched
    ? pikPakRootFolderId
    : getConfigString(detail?.config, 'root_folder_id')
  const effectivePikPakPlatform = pikPakPlatformTouched
    ? pikPakPlatform
    : getConfigString(detail?.config, 'platform', 'web') as PikPakPlatform
  const effectivePikPakDisableMediaLink = pikPakDisableMediaLinkTouched
    ? pikPakDisableMediaLink
    : getConfigBool(detail?.config, 'disable_media_link', true)
  const effectivePikPakCacheTtlSeconds = pikPakCacheTtlSecondsTouched
    ? pikPakCacheTtlSeconds
    : String(getConfigNumber(detail?.config, 'cache_ttl_seconds', 300))
  const effectivePikPakProxyUrl = pikPakProxyUrlTouched
    ? pikPakProxyUrl
    : getConfigString(detail?.config, 'proxy_url')

  const updateSecretPatch = (field: SecretField, value: string) => {
    setSecretPatch((current) => ({ ...current, [field]: value }))
    setSecretClear((current) => ({ ...current, [field]: false }))
  }

  const buildPayload = (): UpdateSourceRequest => {
    const payload: UpdateSourceRequest = {
      name: name.trim(),
      mount_path: mountPath.trim(),
      root_path: source.driver_type === 'pikpak' ? '/' : (rootPath.trim() || '/'),
      is_enabled: isEnabled,
      is_webdav_exposed: isWebDAVExposed,
      webdav_read_only: webDAVReadOnly,
    }

    if (source.driver_type === 'local') {
      payload.config = { base_path: effectiveBasePath.trim() }
    }

    if (source.driver_type === 'pikpak') {
      payload.config = {
        root_folder_id: effectivePikPakRootFolderId.trim(),
        platform: effectivePikPakPlatform,
        disable_media_link: effectivePikPakDisableMediaLink,
        cache_ttl_seconds: toPositiveInt(effectivePikPakCacheTtlSeconds, 300),
        download_strategy: 'redirect',
        proxy_url: effectivePikPakProxyUrl.trim(),
      }
      const nextSecretPatch: Record<string, string | null> = {}
      for (const field of PIKPAK_SECRET_FIELDS) {
        if (secretClear[field.key]) {
          nextSecretPatch[field.key] = null
        } else if (secretPatch[field.key] !== undefined) {
          nextSecretPatch[field.key] = secretPatch[field.key]?.trim() ?? ''
        }
      }
      if (Object.keys(nextSecretPatch).length > 0) {
        payload.secret_patch = nextSecretPatch
      }
    }

    return payload
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    if (!mountPath.trim().startsWith('/')) {
      const message = '挂载路径必须以 / 开头'
      setError(message)
      addToast(message, 'error')
      return
    }
    if (source.driver_type !== 'pikpak' && !rootPath.trim().startsWith('/')) {
      const message = '源内根路径必须以 / 开头'
      setError(message)
      addToast(message, 'error')
      return
    }
    if (source.driver_type === 'local' && !effectiveBasePath.trim()) {
      const message = '请填写本地硬盘路径 / base_path'
      setError(message)
      addToast(message, 'error')
      return
    }
    if (source.driver_type === 'pikpak' && !isValidPikPakProxyUrl(effectivePikPakProxyUrl)) {
      const message = 'PikPak 代理地址只支持 http/https URL，且不能包含账号密码、query 或 fragment'
      setError(message)
      addToast(message, 'error')
      return
    }
    setIsSubmitting(true)
    setError('')
    try {
      await sourceApi.update(source.id, buildPayload())
      addToast('存储源已更新', 'success')
      onSuccess()
      onClose()
    } catch (err: unknown) {
      const message = getApiErrorMessage(err, '更新存储源失败')
      setError(message)
      addToast(message, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-xl bg-card border border-border rounded-lg shadow-xl flex flex-col max-h-[88vh]">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Pencil className="w-4 h-4" />
            编辑存储源
          </h3>
          <button type="button" onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground" title="关闭">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3 overflow-auto scrollbar-thin">
          <div className="rounded-md border border-border bg-muted/20 px-3 py-2 text-sm text-muted-foreground">
            驱动类型：<span className="text-foreground font-medium">{getDriverLabel(source.driver_type)}</span>。当前不支持编辑时切换驱动。
          </div>
          <div>
            <label htmlFor="source-edit-name" className="text-sm text-muted-foreground mb-1 block">名称</label>
            <input
              id="source-edit-name"
              name="name"
              type="text"
              autoComplete="off"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div>
            <label htmlFor="source-edit-mount-path" className="text-sm text-muted-foreground mb-1 block">挂载路径</label>
            <input
              id="source-edit-mount-path"
              name="mount_path"
              type="text"
              value={mountPath}
              onChange={(e) => setMountPath(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div>
            <label htmlFor="source-edit-root-path" className="text-sm text-muted-foreground mb-1 block">源内根路径 / root_path</label>
            <input
              id="source-edit-root-path"
              name="root_path"
              type="text"
              value={source.driver_type === 'pikpak' ? '/' : rootPath}
              disabled={source.driver_type === 'pikpak'}
              onChange={(e) => setRootPath(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
            {source.driver_type === 'pikpak' && (
              <p className="mt-1 text-xs text-muted-foreground">PikPak 固定为 /；远端子目录通过 Root Folder ID 配置。</p>
            )}
          </div>
          {source.driver_type === 'local' && (
            <div>
              <label htmlFor="source-edit-base-path" className="text-sm text-muted-foreground mb-1 block">本地硬盘路径 / base_path</label>
              <input
                id="source-edit-base-path"
                name="base_path"
                type="text"
                value={effectiveBasePath}
                disabled={detailLoading}
                onChange={(e) => {
                  setBasePathTouched(true)
                  setBasePath(e.target.value)
                }}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          )}
          {source.driver_type === 'pikpak' && (
            <div className="rounded-md border border-border bg-muted/20 p-3 space-y-3">
              <p className="text-sm font-medium text-foreground">PikPak 配置</p>
              <div>
                <label htmlFor="source-edit-pikpak-root-folder-id" className="text-sm text-muted-foreground mb-1 block">Root Folder ID</label>
                <input
                  id="source-edit-pikpak-root-folder-id"
                  name="root_folder_id"
                  type="text"
                  value={effectivePikPakRootFolderId}
                  disabled={detailLoading}
                  onChange={(e) => {
                    setPikPakRootFolderIdTouched(true)
                    setPikPakRootFolderId(e.target.value)
                  }}
                  placeholder="留空表示账号根目录"
                  className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                />
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label htmlFor="source-edit-pikpak-platform" className="text-sm text-muted-foreground mb-1 block">平台</label>
                  <select
                    id="source-edit-pikpak-platform"
                    name="platform"
                    value={effectivePikPakPlatform}
                    disabled={detailLoading}
                    onChange={(e) => {
                      setPikPakPlatformTouched(true)
                      setPikPakPlatform(e.target.value as PikPakPlatform)
                    }}
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                  >
                    <option value="web">web</option>
                    <option value="android">android</option>
                    <option value="pc">pc</option>
                  </select>
                </div>
                <div>
                  <label htmlFor="source-edit-pikpak-cache-ttl" className="text-sm text-muted-foreground mb-1 block">缓存 TTL（秒）</label>
                  <input
                    id="source-edit-pikpak-cache-ttl"
                    name="cache_ttl_seconds"
                    type="number"
                    min={1}
                    max={86400}
                    value={effectivePikPakCacheTtlSeconds}
                    disabled={detailLoading}
                    onChange={(e) => {
                      setPikPakCacheTtlSecondsTouched(true)
                      setPikPakCacheTtlSeconds(e.target.value)
                    }}
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div>
                  <label htmlFor="source-edit-pikpak-download-strategy" className="text-sm text-muted-foreground mb-1 block">下载策略</label>
                  <input
                    id="source-edit-pikpak-download-strategy"
                    name="download_strategy"
                    type="text"
                    value="redirect"
                    disabled
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-60"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label htmlFor="source-edit-pikpak-proxy-url" className="text-sm text-muted-foreground mb-1 block">代理地址 / proxy_url（可选）</label>
                  <input
                    id="source-edit-pikpak-proxy-url"
                    name="proxy_url"
                    type="url"
                    value={effectivePikPakProxyUrl}
                    disabled={detailLoading}
                    onChange={(e) => {
                      setPikPakProxyUrlTouched(true)
                      setPikPakProxyUrl(e.target.value)
                    }}
                    autoComplete="off"
                    placeholder="http://127.0.0.1:7890"
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    仅支持 http/https 代理地址，不能包含账号密码、query 或 fragment；留空使用后端默认代理。
                  </p>
                </div>
              </div>
              <label htmlFor="source-edit-pikpak-disable-media-link" className="flex items-start gap-2 text-sm text-foreground">
                <input
                  id="source-edit-pikpak-disable-media-link"
                  name="disable_media_link"
                  type="checkbox"
                  checked={effectivePikPakDisableMediaLink}
                  disabled={detailLoading}
                  onChange={(e) => {
                    setPikPakDisableMediaLinkTouched(true)
                    setPikPakDisableMediaLink(e.target.checked)
                  }}
                  className="mt-0.5 rounded border-border"
                />
                <span>
                  禁用媒体转码链接
                  <span className="block text-xs text-muted-foreground">推荐开启：下载优先使用原始文件链接。</span>
                </span>
              </label>
              <div className="space-y-3">
                <div>
                  <p className="text-sm font-medium text-foreground">敏感字段</p>
                  <p className="text-xs text-muted-foreground mt-0.5">留空表示不修改；勾选清空会向后端提交 null。</p>
                </div>
                {PIKPAK_SECRET_FIELDS.map((field) => (
                  <div key={field.key} className="space-y-1.5">
                    <label htmlFor={`source-edit-${field.key}`} className="text-sm text-muted-foreground mb-1 block">{field.label}</label>
                    <input
                      id={`source-edit-${field.key}`}
                      name={field.key}
                      type={field.key === 'password' ? 'password' : 'text'}
                      value={secretPatch[field.key] ?? ''}
                      disabled={Boolean(secretClear[field.key])}
                      autoComplete={field.autoComplete}
                      onChange={(e) => updateSecretPatch(field.key, e.target.value)}
                      placeholder={`当前：${getSecretDisplay(detail, field.key, canReadSecrets)}`}
                      className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-60"
                    />
                    <label htmlFor={`source-edit-clear-${field.key}`} className="inline-flex items-center gap-2 text-xs text-muted-foreground">
                      <input
                        id={`source-edit-clear-${field.key}`}
                        name={`clear_${field.key}`}
                        type="checkbox"
                        checked={Boolean(secretClear[field.key])}
                        onChange={(e) => {
                          setSecretClear((current) => ({ ...current, [field.key]: e.target.checked }))
                          if (e.target.checked) {
                            setSecretPatch((current) => ({ ...current, [field.key]: '' }))
                          }
                        }}
                        className="rounded border-border"
                      />
                      清空该字段
                    </label>
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="source-edit-is-enabled"
              name="is_enabled"
              checked={isEnabled}
              onChange={(e) => setIsEnabled(e.target.checked)}
              className="rounded border-border"
            />
            <label htmlFor="source-edit-is-enabled" className="text-sm text-foreground">启用</label>
          </div>
          <div className="rounded-md border border-border bg-muted/20 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <div>
                <label htmlFor="source-edit-webdav-exposed" className="text-sm font-medium text-foreground">WebDAV 暴露</label>
                <p className="text-xs text-muted-foreground">
                  local/S3/PikPak 均可暴露；实际写入能力由后端驱动能力与只读开关决定。
                </p>
              </div>
              <input
                type="checkbox"
                id="source-edit-webdav-exposed"
                name="is_webdav_exposed"
                checked={isWebDAVExposed}
                onChange={(e) => setIsWebDAVExposed(e.target.checked)}
                className="rounded border-border"
              />
            </div>
            <label
              className={cn(
                'flex items-center gap-2 text-sm',
                isWebDAVExposed
                  ? 'text-foreground cursor-pointer'
                  : 'text-muted-foreground cursor-not-allowed'
              )}
            >
              <input
                type="checkbox"
                id="source-edit-webdav-read-only"
                name="webdav_read_only"
                checked={webDAVReadOnly}
                disabled={!isWebDAVExposed}
                onChange={(e) => setWebDAVReadOnly(e.target.checked)}
                className="rounded border-border"
              />
              只读访问
            </label>
            {source.webdav_slug && (
              <p className="text-xs text-muted-foreground">
                Slug：<code className="font-mono text-foreground">{source.webdav_slug}</code>
              </p>
            )}
          </div>
          {error && (
            <div role="alert" className="flex items-start gap-2 rounded-md border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
              <span>{error}</span>
            </div>
          )}
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
              disabled={isSubmitting || detailLoading || !name.trim()}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || detailLoading || !name.trim()) && 'opacity-50 cursor-not-allowed'
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

function CreateSourceModal({ onClose, onSuccess }: { onClose: () => void; onSuccess: () => void }) {
  const { addToast } = useUIStore()
  const [name, setName] = useState('')
  const [driverType, setDriverType] = useState<SourceDriverType>('local')
  const [basePath, setBasePath] = useState('/data')
  const [rootPath, setRootPath] = useState('/')
  const [mountPath, setMountPath] = useState('/')
  const [isWebDAVExposed, setIsWebDAVExposed] = useState(false)
  const [webDAVReadOnly, setWebDAVReadOnly] = useState(true)
  const [pikPakRootFolderId, setPikPakRootFolderId] = useState('')
  const [pikPakPlatform, setPikPakPlatform] = useState<PikPakPlatform>('web')
  const [pikPakDisableMediaLink, setPikPakDisableMediaLink] = useState(true)
  const [pikPakCacheTtlSeconds, setPikPakCacheTtlSeconds] = useState('300')
  const [pikPakProxyUrl, setPikPakProxyUrl] = useState('')
  const [pikPakSecrets, setPikPakSecrets] = useState<Partial<Record<SecretField, string>>>({})
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [verificationUrl, setVerificationUrl] = useState('')

  const updatePikPakSecret = (field: SecretField, value: string) => {
    setPikPakSecrets((current) => ({ ...current, [field]: value }))
  }

  const copyVerificationUrl = async () => {
    if (!verificationUrl) return
    try {
      await writeTextToClipboard(verificationUrl)
      addToast('验证链接已复制', 'success')
    } catch {
      addToast('复制失败，请手动复制验证链接', 'error')
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setVerificationUrl('')
    if (!name.trim()) return
    if (!mountPath.trim().startsWith('/')) {
      const message = '挂载路径必须以 / 开头'
      setCreateError(message)
      addToast(message, 'error')
      return
    }
    if (driverType !== 'pikpak' && !rootPath.trim().startsWith('/')) {
      const message = '源内根路径必须以 / 开头'
      setCreateError(message)
      addToast(message, 'error')
      return
    }
    if (driverType === 'local' && !basePath.trim()) {
      const message = '请填写本地硬盘路径 / base_path'
      setCreateError(message)
      addToast(message, 'error')
      return
    }
    if (driverType === 'pikpak') {
      const hasRefreshToken = Boolean(pikPakSecrets.refresh_token?.trim())
      const hasUsernamePassword = Boolean(pikPakSecrets.username?.trim() && pikPakSecrets.password?.trim())
      if (!hasRefreshToken && !hasUsernamePassword) {
        const message = '请填写 PikPak 账号密码，或提供 refresh_token'
        setCreateError(message)
        addToast(message, 'error')
        return
      }
      if (!isValidPikPakProxyUrl(pikPakProxyUrl)) {
        const message = 'PikPak 代理地址只支持 http/https URL，且不能包含账号密码、query 或 fragment'
        setCreateError(message)
        addToast(message, 'error')
        return
      }
    }
    setCreateError(null)
    setIsSubmitting(true)
    try {
      await sourceApi.create({
        name: name.trim(),
        driver_type: driverType,
        is_enabled: true,
        is_webdav_exposed: isWebDAVExposed,
        webdav_read_only: webDAVReadOnly,
        mount_path: mountPath.trim(),
        root_path: driverType === 'pikpak' ? '/' : rootPath.trim(),
        config: driverType === 'local'
          ? { base_path: basePath.trim() }
          : driverType === 'pikpak'
            ? {
                root_folder_id: pikPakRootFolderId.trim(),
                platform: pikPakPlatform,
                disable_media_link: pikPakDisableMediaLink,
                cache_ttl_seconds: toPositiveInt(pikPakCacheTtlSeconds, 300),
                download_strategy: 'redirect',
                proxy_url: pikPakProxyUrl.trim(),
              }
            : {},
        secret_patch: driverType === 'pikpak'
          ? {
              username: pikPakSecrets.username?.trim() ?? '',
              password: pikPakSecrets.password?.trim() ?? '',
              refresh_token: pikPakSecrets.refresh_token?.trim() ?? '',
              captcha_token: pikPakSecrets.captcha_token?.trim() ?? '',
              device_id: pikPakSecrets.device_id?.trim() ?? '',
            }
          : {},
      })
      addToast('存储源创建成功', 'success')
      onSuccess()
      onClose()
    } catch (err: unknown) {
      const message = getCreateSourceErrorMessage(err)
      setVerificationUrl(getApiErrorDetailString(err, 'verification_url'))
      setCreateError(message)
      addToast(message, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-xl bg-card border border-border rounded-lg shadow-xl flex flex-col max-h-[88vh]">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Plus className="w-4 h-4" />
            添加存储源
          </h3>
          <button type="button" onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground" title="关闭">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3 overflow-auto scrollbar-thin">
          <div>
            <label htmlFor="source-create-name" className="text-sm text-muted-foreground mb-1 block">名称</label>
            <input
              id="source-create-name"
              name="name"
              type="text"
              autoComplete="off"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={driverType === 'pikpak' ? '例如：PikPak 媒体库' : '例如：本地存储'}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
            <p className="mt-1 text-xs text-muted-foreground">
              建议包含英文字母或数字，便于生成唯一的 WebDAV 访问标识。
            </p>
          </div>
          <div>
            <p id="source-create-driver-type-label" className="text-sm text-muted-foreground mb-1 block">驱动类型</p>
            <div className="flex gap-2" role="radiogroup" aria-labelledby="source-create-driver-type-label">
              {SOURCE_DRIVER_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  role="radio"
                  aria-checked={driverType === option.value}
                  onClick={() => setDriverType(option.value)}
                  className={cn(
                    'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                    driverType === option.value
                      ? 'border-primary bg-primary/5 text-primary'
                      : 'border-border text-muted-foreground hover:border-primary/30'
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label htmlFor="source-create-mount-path" className="text-sm text-muted-foreground mb-1 block">挂载路径</label>
            <input
              id="source-create-mount-path"
              name="mount_path"
              type="text"
              value={mountPath}
              onChange={(e) => setMountPath(e.target.value)}
              placeholder={driverType === 'pikpak' ? '/pikpak' : '/local'}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          {driverType === 'local' && (
            <div>
              <label htmlFor="source-create-base-path" className="text-sm text-muted-foreground mb-1 block">本地硬盘路径 / base_path</label>
              <input
                id="source-create-base-path"
                name="base_path"
                type="text"
                value={basePath}
                onChange={(e) => setBasePath(e.target.value)}
                placeholder="/mnt/e2e-host-disk"
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                填容器内可访问的本地目录，例如 /mnt/e2e-host-disk。
              </p>
            </div>
          )}
          {driverType === 'pikpak' && (
            <div className="rounded-md border border-border bg-muted/20 p-3 space-y-3">
              <p className="text-sm font-medium text-foreground">PikPak 配置</p>
              <div>
                <label htmlFor="source-create-pikpak-root-folder-id" className="text-sm text-muted-foreground mb-1 block">Root Folder ID</label>
                <input
                  id="source-create-pikpak-root-folder-id"
                  name="root_folder_id"
                  type="text"
                  value={pikPakRootFolderId}
                  onChange={(e) => setPikPakRootFolderId(e.target.value)}
                  placeholder="留空表示账号根目录"
                  className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                />
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label htmlFor="source-create-pikpak-platform" className="text-sm text-muted-foreground mb-1 block">平台</label>
                  <select
                    id="source-create-pikpak-platform"
                    name="platform"
                    value={pikPakPlatform}
                    onChange={(e) => setPikPakPlatform(e.target.value as PikPakPlatform)}
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                  >
                    <option value="web">web</option>
                    <option value="android">android</option>
                    <option value="pc">pc</option>
                  </select>
                </div>
                <div>
                  <label htmlFor="source-create-pikpak-cache-ttl" className="text-sm text-muted-foreground mb-1 block">缓存 TTL（秒）</label>
                  <input
                    id="source-create-pikpak-cache-ttl"
                    name="cache_ttl_seconds"
                    type="number"
                    min={1}
                    max={86400}
                    value={pikPakCacheTtlSeconds}
                    onChange={(e) => setPikPakCacheTtlSeconds(e.target.value)}
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
                <div>
                  <label htmlFor="source-create-pikpak-download-strategy" className="text-sm text-muted-foreground mb-1 block">下载策略</label>
                  <input
                    id="source-create-pikpak-download-strategy"
                    name="download_strategy"
                    type="text"
                    value="redirect"
                    disabled
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring disabled:opacity-60"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label htmlFor="source-create-pikpak-proxy-url" className="text-sm text-muted-foreground mb-1 block">代理地址 / proxy_url（可选）</label>
                  <input
                    id="source-create-pikpak-proxy-url"
                    name="proxy_url"
                    type="url"
                    value={pikPakProxyUrl}
                    onChange={(e) => setPikPakProxyUrl(e.target.value)}
                    autoComplete="off"
                    placeholder="http://127.0.0.1:7890"
                    className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    网络出口受限时可指定单个 PikPak 源代理；仅支持 http/https，不允许账号密码、query 或 fragment。
                  </p>
                </div>
              </div>
              <label htmlFor="source-create-pikpak-disable-media-link" className="flex items-start gap-2 text-sm text-foreground">
                <input
                  id="source-create-pikpak-disable-media-link"
                  name="disable_media_link"
                  type="checkbox"
                  checked={pikPakDisableMediaLink}
                  onChange={(e) => setPikPakDisableMediaLink(e.target.checked)}
                  className="mt-0.5 rounded border-border"
                />
                <span>
                  禁用媒体转码链接
                  <span className="block text-xs text-muted-foreground">推荐开启：下载优先使用原始文件链接。</span>
                </span>
              </label>
              <div className="space-y-3">
                <div>
                  <p className="text-sm font-medium text-foreground">敏感字段</p>
                  <p className="text-xs text-muted-foreground mt-0.5">首次创建可填写账号密码；已有 refresh_token 时可只填 token。</p>
                </div>
                {PIKPAK_SECRET_FIELDS.map((field) => (
                  <div key={field.key}>
                    <label htmlFor={`source-create-${field.key}`} className="text-sm text-muted-foreground mb-1 block">{field.label}</label>
                    <input
                      id={`source-create-${field.key}`}
                      name={field.key}
                      type={field.key === 'password' ? 'password' : 'text'}
                      value={pikPakSecrets[field.key] ?? ''}
                      autoComplete={field.autoComplete}
                      onChange={(e) => updatePikPakSecret(field.key, e.target.value)}
                      className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                    />
                  </div>
                ))}
              </div>
            </div>
          )}
          <div>
            <label htmlFor="source-create-root-path" className="text-sm text-muted-foreground mb-1 block">源内根路径 / root_path</label>
            <input
              id="source-create-root-path"
              name="root_path"
              type="text"
              value={driverType === 'pikpak' ? '/' : rootPath}
              disabled={driverType === 'pikpak'}
              onChange={(e) => setRootPath(e.target.value)}
              placeholder="/"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
            <p className="mt-1 text-xs text-muted-foreground">
              {driverType === 'pikpak' ? 'PikPak 固定为 /；远端子目录请填写 Root Folder ID。' : '通常保持为 /；这不是本地硬盘路径。'}
            </p>
          </div>
          <div className="rounded-md border border-border bg-muted/20 p-3 space-y-2">
            <div className="flex items-center justify-between gap-3">
              <div>
                <label htmlFor="source-create-webdav-exposed" className="text-sm font-medium text-foreground">WebDAV 暴露</label>
                <p className="text-xs text-muted-foreground">
                  local/S3/PikPak 均可暴露；实际写入能力由后端驱动能力与只读开关决定。
                </p>
              </div>
              <input
                type="checkbox"
                id="source-create-webdav-exposed"
                name="is_webdav_exposed"
                checked={isWebDAVExposed}
                onChange={(e) => setIsWebDAVExposed(e.target.checked)}
                className="rounded border-border"
              />
            </div>
            <label
              className={cn(
                'flex items-center gap-2 text-sm',
                isWebDAVExposed
                  ? 'text-foreground cursor-pointer'
                  : 'text-muted-foreground cursor-not-allowed'
              )}
            >
              <input
                type="checkbox"
                id="source-create-webdav-read-only"
                name="webdav_read_only"
                checked={webDAVReadOnly}
                disabled={!isWebDAVExposed}
                onChange={(e) => setWebDAVReadOnly(e.target.checked)}
                className="rounded border-border"
              />
              只读访问
            </label>
          </div>
          {createError && (
            <div role="alert" className="flex items-start gap-2 rounded-md border border-destructive/20 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
              <div className="min-w-0 flex-1 space-y-2">
                <p>{createError}</p>
                {verificationUrl && (
                  <div className="space-y-2 rounded-md border border-destructive/20 bg-background/80 p-2 text-muted-foreground">
                    <p className="text-xs">
                      请先打开 PikPak 验证页面完成人工验证；验证完成后，将获取到的 captcha_token 填入上方 Captcha Token 字段再重试。
                    </p>
                    <code className="block break-all rounded bg-muted px-2 py-1 font-mono text-[11px] text-foreground">
                      {verificationUrl}
                    </code>
                    <div className="flex flex-wrap gap-2">
                      <a
                        href={verificationUrl}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-1.5 rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
                      >
                        <ExternalLink className="w-3.5 h-3.5" />
                        打开验证页面
                      </a>
                      <button
                        type="button"
                        onClick={copyVerificationUrl}
                        className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-accent"
                      >
                        <Copy className="w-3.5 h-3.5" />
                        复制验证链接
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}
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
              disabled={isSubmitting || !name.trim() || (driverType === 'local' && !basePath.trim())}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || !name.trim() || (driverType === 'local' && !basePath.trim())) && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '创建中...' : '创建'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export function SourcesPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const { setCurrentSource, currentSource } = useFileStore()
  const { addToast } = useUIStore()
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<StorageSource | null>(null)

  const canCreate = useHasCapability('source.create')
  const canUpdate = useHasCapability('source.update')
  const canDelete = useHasCapability('source.delete')
  const canTest = useHasCapability('source.test')
  const canReadSecrets = useHasCapability('source.secret.read')
  const canReadSystemConfig = useHasCapability('system.config.read')

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data, isLoading } = useQuery({
    queryKey: ['sources'],
    queryFn: () => sourceApi.list({ page: 1, page_size: 100, view: 'admin' }),
  })

  const { data: systemConfig, isLoading: systemConfigLoading } = useQuery({
    queryKey: ['system-config'],
    queryFn: () => systemApi.getConfig(),
    enabled: isAuthenticated && canReadSystemConfig,
    retry: false,
  })
  const webDAVPromotedToHttps = isWebDAVOriginPromotedToHttps()

  const copyWebDAVUrl = async (url: string) => {
    try {
      await writeTextToClipboard(url)
      addToast('WebDAV 地址已复制', 'success')
    } catch {
      addToast('复制失败，请手动复制 WebDAV 地址', 'error')
    }
  }

  const handleTest = async (id: number) => {
    try {
      await sourceApi.testById(id)
      queryClient.invalidateQueries({ queryKey: ['sources'] })
      queryClient.invalidateQueries({ queryKey: ['source-detail', id] })
      addToast('存储源连接测试完成', 'success')
    } catch (err: unknown) {
      addToast(getApiErrorMessage(err, '测试存储源失败'), 'error')
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除此存储源吗？此操作不可撤销。')) return
    try {
      await sourceApi.delete(id)
      if (currentSource?.id === id) {
        setCurrentSource(null)
      }
      queryClient.invalidateQueries({ queryKey: ['sources'] })
      queryClient.invalidateQueries({ queryKey: ['source-detail'] })
      addToast('存储源已删除', 'success')
    } catch (err: unknown) {
      addToast(getApiErrorMessage(err, '删除存储源失败'), 'error')
    }
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const sources = data?.items || []

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">存储源管理</h1>
        {canCreate && (
          <button
            onClick={() => setCreateModalOpen(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            <Plus className="w-4 h-4" />
            <span>添加存储源</span>
          </button>
        )}
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {sources.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <HardDrive className="w-12 h-12 opacity-30" />
            <p>暂无存储源</p>
            {canCreate && (
              <button
                onClick={() => setCreateModalOpen(true)}
                className="px-4 py-2 rounded-md bg-primary text-primary-foreground text-sm hover:bg-primary/90 transition-colors"
              >
                添加存储源
              </button>
            )}
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {sources.map((source) => {
              const webDAVUrl = source.is_webdav_exposed && source.webdav_slug
                ? buildSourceWebDAVUrl(systemConfig?.webdav_prefix, source.webdav_slug)
                : ''
              const hasKnownGlobalWebDAVStatus = canReadSystemConfig && Boolean(systemConfig)
              const isWebDAVUsable = Boolean(
                webDAVUrl && (!hasKnownGlobalWebDAVStatus || systemConfig?.webdav_enabled)
              )

              return (
                <div
                  key={source.id}
                  className={cn(
                    'p-4 rounded-lg border transition-all cursor-pointer',
                    currentSource?.id === source.id
                      ? 'border-primary bg-primary/5'
                      : 'border-border bg-card hover:border-primary/30'
                  )}
                  onClick={() => setCurrentSource(source)}
                >
                <div className="flex items-start justify-between mb-3">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center">
                      <HardDrive className="w-5 h-5 text-primary" />
                    </div>
                    <div>
                      <h3 className="font-medium text-foreground">{source.name}</h3>
                      <p className="text-xs text-muted-foreground uppercase">{source.driver_type}</p>
                    </div>
                  </div>
                  <StatusBadge status={source.status} />
                </div>

                <div className="space-y-2">
                  <div className="flex justify-between gap-3 text-sm">
                    <span className="shrink-0 text-muted-foreground">挂载路径</span>
                    <span className="min-w-0 truncate text-right text-foreground" title={source.mount_path}>
                      {source.mount_path}
                    </span>
                  </div>
                  <div className="flex justify-between gap-3 text-sm">
                    <span className="shrink-0 text-muted-foreground">源内根路径</span>
                    <span className="min-w-0 truncate text-right text-foreground" title={source.root_path}>
                      {source.root_path}
                    </span>
                  </div>
                  <SourceConfigRows source={source} canReadSecrets={canReadSecrets} />
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">容量</span>
                    <span className="text-foreground">
                      {formatBytes(source.used_bytes)} / {formatBytes(source.total_bytes)}
                    </span>
                  </div>
                  {source.total_bytes && source.used_bytes !== null && (
                    <div className="w-full h-1.5 bg-muted rounded-full overflow-hidden">
                      <div
                        className="h-full bg-primary rounded-full transition-all"
                        style={{ width: `${Math.min((source.used_bytes / source.total_bytes) * 100, 100)}%` }}
                      />
                    </div>
                  )}
                  {webDAVUrl && (
                    <div
                      className={cn(
                        'rounded-md border p-2 space-y-1.5',
                        isWebDAVUsable
                          ? 'border-primary/20 bg-primary/5'
                          : 'border-border bg-muted/30'
                      )}
                    >
                      <div className="flex items-center gap-1.5 text-xs">
                        <Link2 className={cn('w-3.5 h-3.5', isWebDAVUsable ? 'text-primary' : 'text-muted-foreground')} />
                        <span className="font-medium text-foreground">WebDAV 地址</span>
                        <span
                          className={cn(
                            'ml-auto inline-flex items-center gap-1 rounded-full px-1.5 py-0.5',
                            source.webdav_read_only
                              ? 'bg-amber-500/10 text-amber-500'
                              : 'bg-emerald-500/10 text-emerald-500'
                          )}
                        >
                          {source.webdav_read_only ? <Lock className="w-3 h-3" /> : <Unlock className="w-3 h-3" />}
                          {source.webdav_read_only ? '只读' : '读写'}
                        </span>
                      </div>
                      <div className="flex items-center gap-2">
                        <code
                          className={cn(
                            'min-w-0 flex-1 truncate rounded bg-background/80 px-2 py-1 font-mono text-[11px]',
                            isWebDAVUsable ? 'text-foreground' : 'text-muted-foreground'
                          )}
                          title={webDAVUrl}
                        >
                          {webDAVUrl}
                        </code>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            void copyWebDAVUrl(webDAVUrl)
                          }}
                          className="shrink-0 p-1.5 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground"
                          title="复制 WebDAV 地址"
                        >
                          <Copy className="w-3.5 h-3.5" />
                        </button>
                      </div>
                      {canReadSystemConfig && systemConfigLoading && !systemConfig && (
                        <p className="text-[11px] text-muted-foreground">正在确认全局 WebDAV 状态...</p>
                      )}
                      {canReadSystemConfig && systemConfig?.webdav_enabled === false && (
                        <p className="text-[11px] text-muted-foreground">全局 WebDAV 当前未启用</p>
                      )}
                      {webDAVPromotedToHttps && (
                        <p className="text-[11px] text-muted-foreground">
                          后端 WebDAV 要求 HTTPS，已按 HTTPS 生成地址；当前 HTTP 部署需通过带 X-Forwarded-Proto: https 的反向代理访问。
                        </p>
                      )}
                      {!canReadSystemConfig && (
                        <p className="text-[11px] text-muted-foreground">
                          当前账号无系统配置读取权限，无法确认全局 WebDAV 开关；地址按默认 /dav 前缀展示。
                        </p>
                      )}
                    </div>
                  )}
                </div>

                <div className="flex items-center gap-2 mt-4 pt-3 border-t border-border">
                  <span
                    className={cn(
                      'text-xs px-2 py-0.5 rounded-full',
                      source.is_enabled
                        ? 'bg-emerald-500/10 text-emerald-500'
                        : 'bg-muted text-muted-foreground'
                    )}
                  >
                    {source.is_enabled ? '已启用' : '已禁用'}
                  </span>
                  {source.is_webdav_exposed && (
                    <span className="text-xs px-2 py-0.5 rounded-full bg-primary/10 text-primary">
                      WebDAV
                    </span>
                  )}
                  <div className="flex-1" />
                  {canTest && (
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        handleTest(source.id)
                      }}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      title="测试连接"
                    >
                      <RefreshCw className="w-3.5 h-3.5" />
                    </button>
                  )}
                  {canUpdate && (
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        setEditTarget(source)
                      }}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      title="编辑"
                    >
                      <Pencil className="w-3.5 h-3.5" />
                    </button>
                  )}
                  {canDelete && (
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        handleDelete(source.id)
                      }}
                      className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                      title="删除"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {createModalOpen && (
        <CreateSourceModal
          onClose={() => setCreateModalOpen(false)}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['sources'] })
            queryClient.invalidateQueries({ queryKey: ['source-detail'] })
          }}
        />
      )}
      {editTarget && (
        <EditSourceModal
          key={editTarget.id}
          onClose={() => setEditTarget(null)}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['sources'] })
            queryClient.invalidateQueries({ queryKey: ['source-detail', editTarget.id] })
          }}
          source={editTarget}
        />
      )}
    </div>
  )
}
