import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { sharePublicApi } from '@/api/sharePublic'
import type { PublicShareEntry } from '@/api/sharePublic'
import {
  Folder,
  FileText,
  Image,
  Film,
  Music,
  File,
  Lock,
  Download,
  ChevronLeft,
  ArrowLeft,
  Clock,
  Eye,
} from 'lucide-react'
import { cn, formatDate, getFileIconClass } from '@/utils'

const iconMap = {
  folder: Folder,
  image: Image,
  video: Film,
  audio: Music,
  file: File,
  document: FileText,
  spreadsheet: FileText,
  presentation: FileText,
  code: FileText,
  pdf: FileText,
  archive: File,
}

function FileIcon({ item }: { item: PublicShareEntry }) {
  const type = getFileIconClass(item.extension, item.is_dir)
  const Icon = iconMap[type as keyof typeof iconMap] || File
  return (
    <Icon
      className={cn(
        'w-5 h-5 shrink-0',
        item.is_dir ? 'text-primary' : 'text-muted-foreground'
      )}
    />
  )
}

function PasswordForm({
  onSubmit,
  error,
  isSubmitting,
}: {
  onSubmit: (password: string) => void
  error: string | null
  isSubmitting: boolean
}) {
  const [password, setPassword] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!password.trim() || isSubmitting) return
    onSubmit(password.trim())
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm bg-card border border-border rounded-lg shadow-xl p-6 space-y-4">
        <div className="flex flex-col items-center gap-3">
          <div className="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center">
            <Lock className="w-6 h-6 text-primary" />
          </div>
          <h2 className="text-lg font-semibold text-foreground">此分享需要密码</h2>
          <p className="text-sm text-muted-foreground text-center">请输入访问密码以查看分享内容</p>
        </div>
        <form onSubmit={handleSubmit} className="space-y-3">
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="访问密码"
            className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            autoFocus
          />
          {error && <p className="text-sm text-destructive">{error}</p>}
          <button
            type="submit"
            disabled={isSubmitting || !password.trim()}
            className={cn(
              'w-full px-4 py-2 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
              (isSubmitting || !password.trim()) && 'opacity-50 cursor-not-allowed'
            )}
          >
            {isSubmitting ? '验证中...' : '进入分享'}
          </button>
        </form>
      </div>
    </div>
  )
}

export function ShareAccessPage() {
  const { token } = useParams<{ token: string }>()
  const navigate = useNavigate()
  const [shareInfo, setShareInfo] = useState<{ name: string; is_dir: boolean; has_password: boolean; expires_at: string | null } | null>(null)
  const [items, setItems] = useState<PublicShareEntry[]>([])
  const [currentPath, setCurrentPath] = useState('/')
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [passwordError, setPasswordError] = useState<string | null>(null)
  const [isVerifying, setIsVerifying] = useState(false)
  const [needsPassword, setNeedsPassword] = useState(false)

  const loadShare = async (password?: string) => {
    if (!token) return
    setIsLoading(true)
    setError(null)
    try {
      const res = await sharePublicApi.open(token, password, currentPath)
      if (!res || !res.share) {
        setError('分享数据格式错误')
        setIsLoading(false)
        return
      }
      setShareInfo({
        name: res.share.name,
        is_dir: res.share.is_dir,
        has_password: res.share.has_password,
        expires_at: res.share.expires_at,
      })
      setItems(res.items || [])
      setCurrentPath(res.current_path || '/')
      setNeedsPassword(false)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '分享加载失败'
      if (msg.toLowerCase().includes('password') || msg.toLowerCase().includes('unauthorized')) {
        setNeedsPassword(true)
      } else {
        setError(msg)
      }
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    if (token) {
      loadShare()
    }
  }, [token])

  const handleVerifyPassword = (password: string) => {
    if (!token) return
    setIsVerifying(true)
    setPasswordError(null)
    loadShare(password).finally(() => {
      setIsVerifying(false)
    })
  }

  const handleNavigate = async (item: PublicShareEntry) => {
    if (!item.is_dir || !token) return
    setIsLoading(true)
    try {
      const res = await sharePublicApi.open(token, undefined, item.path)
      if (res.share) {
        setShareInfo({
          name: res.share.name,
          is_dir: res.share.is_dir,
          has_password: res.share.has_password,
          expires_at: res.share.expires_at,
        })
      }
      setItems(res.items || [])
      setCurrentPath(res.current_path || '/')
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '加载失败'
      setError(msg)
    } finally {
      setIsLoading(false)
    }
  }

  const handleGoUp = async () => {
    if (!token) return
    const parent = currentPath.split('/').slice(0, -1).join('/') || '/'
    setIsLoading(true)
    try {
      const res = await sharePublicApi.open(token, undefined, parent)
      if (res.share) {
        setShareInfo({
          name: res.share.name,
          is_dir: res.share.is_dir,
          has_password: res.share.has_password,
          expires_at: res.share.expires_at,
        })
      }
      setItems(res.items || [])
      setCurrentPath(res.current_path || '/')
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '加载失败'
      setError(msg)
    } finally {
      setIsLoading(false)
    }
  }

  const handleDownload = async (item: PublicShareEntry) => {
    if (!token) return
    try {
      const res = await sharePublicApi.open(token, undefined, item.path)
      // For file downloads, backend returns redirect URL
      if ((res as unknown as { redirect_url?: string }).redirect_url) {
        window.open((res as unknown as { redirect_url: string }).redirect_url, '_blank')
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '获取下载链接失败'
      setError(msg)
    }
  }

  const handlePreview = async (item: PublicShareEntry) => {
    if (!token || !item.can_preview) return
    try {
      const res = await sharePublicApi.open(token, undefined, item.path)
      if ((res as unknown as { redirect_url?: string }).redirect_url) {
        window.open((res as unknown as { redirect_url: string }).redirect_url, '_blank')
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '获取预览链接失败'
      setError(msg)
    }
  }

  if (isLoading && !shareInfo && !needsPassword) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  if (error && !shareInfo) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background p-4">
        <div className="text-center space-y-3">
          <p className="text-destructive font-medium">{error}</p>
          <button
            onClick={() => navigate('/')}
            className="px-4 py-2 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            返回首页
          </button>
        </div>
      </div>
    )
  }

  if (needsPassword) {
    return (
      <PasswordForm
        onSubmit={handleVerifyPassword}
        error={passwordError}
        isSubmitting={isVerifying}
      />
    )
  }

  if (!shareInfo) return null

  const isRoot = currentPath === '/' || currentPath === ''

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-30 bg-card/80 backdrop-blur border-b border-border">
        <div className="flex items-center gap-3 px-4 h-14">
          <button
            onClick={() => navigate('/')}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
            title="返回首页"
          >
            <ArrowLeft className="w-5 h-5" />
          </button>
          <div className="flex-1 min-w-0">
            <h1 className="text-base font-semibold text-foreground truncate">{shareInfo.name}</h1>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              {shareInfo.is_dir && <span>文件夹</span>}
              {!shareInfo.is_dir && <span>文件</span>}
              {shareInfo.expires_at && (
                <span className="inline-flex items-center gap-0.5">
                  <Clock className="w-3 h-3" />
                  有效期至 {formatDate(shareInfo.expires_at)}
                </span>
              )}
            </div>
          </div>
        </div>
        {shareInfo.is_dir && !isRoot && (
          <div className="flex items-center gap-2 px-4 h-10 border-t border-border bg-muted/30">
            <button
              onClick={handleGoUp}
              className="p-1 rounded-md hover:bg-accent text-muted-foreground"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span className="text-sm text-muted-foreground truncate">{currentPath}</span>
          </div>
        )}
      </header>

      <main className="p-4 max-w-5xl mx-auto">
        {isLoading ? (
          <div className="flex items-center justify-center py-20">
            <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
          </div>
        ) : shareInfo.is_dir ? (
          <div className="space-y-2">
            {items.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-20 text-muted-foreground gap-3">
                <Folder className="w-12 h-12 opacity-30" />
                <p>此文件夹为空</p>
              </div>
            ) : (
              items.map((item) => (
                <div
                  key={item.path}
                  className="flex items-center gap-3 p-3 rounded-lg border border-border bg-card hover:border-primary/30 transition-colors"
                >
                  <FileIcon item={item} />
                  <div className="flex-1 min-w-0">
                    <p
                      className={cn(
                        'text-sm truncate',
                        item.is_dir ? 'text-primary font-medium cursor-pointer' : 'text-foreground'
                      )}
                      onClick={() => handleNavigate(item)}
                    >
                      {item.name}
                    </p>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                      {!item.is_dir && <span>{item.size > 0 ? formatBytes(item.size) : '-'}</span>}
                      <span>{formatDate(item.modified_at)}</span>
                    </div>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    {item.can_preview && !item.is_dir && (
                      <button
                        onClick={() => handlePreview(item)}
                        className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                        title="预览"
                      >
                        <Eye className="w-4 h-4" />
                      </button>
                    )}
                    {!item.is_dir && (
                      <button
                        onClick={() => handleDownload(item)}
                        className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                        title="下载"
                      >
                        <Download className="w-4 h-4" />
                      </button>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-20 gap-4">
            <FileText className="w-16 h-16 text-muted-foreground opacity-30" />
            <div className="text-center">
              <p className="text-lg font-medium text-foreground">{shareInfo.name}</p>
              <p className="text-sm text-muted-foreground mt-1">单个文件分享</p>
            </div>
            <button
              onClick={() => {
                if (items[0]) handleDownload(items[0])
              }}
              className="flex items-center gap-2 px-4 py-2 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
            >
              <Download className="w-4 h-4" />
              下载文件
            </button>
          </div>
        )}
      </main>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}
