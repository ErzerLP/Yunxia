import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery } from '@tanstack/react-query'
import { auditApi } from '@/api/audit'
import { useUIStore } from '@/stores/uiStore'
import {
  ScrollText,
  Search,
  X,
  ChevronLeft,
  ChevronRight,
  Eye,
  User,
  Globe,
  FolderOpen,
  Clock,
} from 'lucide-react'
import { cn, formatDate } from '@/utils'
import { useHasCapability } from '@/hooks/useCapability'
import type { AuditLog } from '@/types/api'

function ResultBadge({ result }: { result: string }) {
  const isSuccess = result === 'success' || result === 'allow'
  return (
    <span
      className={cn(
        'inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium',
        isSuccess
          ? 'bg-emerald-500/10 text-emerald-500'
          : 'bg-destructive/10 text-destructive'
      )}
    >
      {result}
    </span>
  )
}

function AuditLogDrawer({
  log,
  onClose,
}: {
  log: AuditLog | null
  onClose: () => void
}) {
  if (!log) return null

  return (
    <>
      <div className="fixed inset-0 bg-black/20 z-40" onClick={onClose} />
      <div className="fixed right-0 top-0 h-full w-[480px] bg-card border-l border-border z-50 shadow-xl flex flex-col animate-slide-in-right">
        <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
          <h3 className="font-medium text-card-foreground flex items-center gap-2">
            <Eye className="w-4 h-4" />
            审计详情
          </h3>
          <button
            onClick={onClose}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground hover:text-accent-foreground transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="flex-1 overflow-auto p-4 space-y-4">
          <section className="space-y-2">
            <h4 className="text-sm font-medium text-foreground flex items-center gap-1.5">
              <Clock className="w-3.5 h-3.5 text-muted-foreground" />
              基本信息
            </h4>
            <div className="bg-muted/50 rounded-lg p-3 space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">ID</span>
                <span className="text-foreground font-mono">{log.id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">时间</span>
                <span className="text-foreground">{formatDate(log.occurred_at)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">资源类型</span>
                <span className="text-foreground">{log.resource_type}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">动作</span>
                <span className="text-foreground">{log.action}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">结果</span>
                <ResultBadge result={log.result} />
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">摘要</span>
                <span className="text-foreground text-right max-w-[250px]">{log.summary}</span>
              </div>
            </div>
          </section>

          <section className="space-y-2">
            <h4 className="text-sm font-medium text-foreground flex items-center gap-1.5">
              <User className="w-3.5 h-3.5 text-muted-foreground" />
              操作者
            </h4>
            <div className="bg-muted/50 rounded-lg p-3 space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">用户 ID</span>
                <span className="text-foreground font-mono">{log.actor.user_id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">用户名</span>
                <span className="text-foreground">{log.actor.username}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">角色</span>
                <span className="text-foreground">{log.actor.role_key}</span>
              </div>
            </div>
          </section>

          <section className="space-y-2">
            <h4 className="text-sm font-medium text-foreground flex items-center gap-1.5">
              <Globe className="w-3.5 h-3.5 text-muted-foreground" />
              请求信息
            </h4>
            <div className="bg-muted/50 rounded-lg p-3 space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">请求 ID</span>
                <span className="text-foreground font-mono text-xs">{log.request.request_id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">入口</span>
                <span className="text-foreground">{log.request.entrypoint}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">客户端 IP</span>
                <span className="text-foreground font-mono">{log.request.client_ip}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">方法</span>
                <span className="text-foreground">{log.request.method}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">路径</span>
                <span className="text-foreground font-mono text-xs text-right max-w-[280px]">{log.request.path}</span>
              </div>
            </div>
          </section>

          <section className="space-y-2">
            <h4 className="text-sm font-medium text-foreground flex items-center gap-1.5">
              <FolderOpen className="w-3.5 h-3.5 text-muted-foreground" />
              目标资源
            </h4>
            <div className="bg-muted/50 rounded-lg p-3 space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">存储源 ID</span>
                <span className="text-foreground font-mono">{log.target.source_id}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">虚拟路径</span>
                <span className="text-foreground font-mono text-xs">{log.target.virtual_path}</span>
              </div>
            </div>
          </section>

          {log.detail && Object.keys(log.detail).length > 0 && (
            <section className="space-y-2">
              <h4 className="text-sm font-medium text-foreground">详细数据</h4>
              <pre className="bg-muted/50 rounded-lg p-3 text-xs text-foreground overflow-auto">
                {JSON.stringify(log.detail, null, 2)}
              </pre>
            </section>
          )}
        </div>
      </div>
    </>
  )
}

export function AuditPage() {
  const navigate = useNavigate()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const { addToast } = useUIStore()
  const [page, setPage] = useState(1)
  const [filters, setFilters] = useState({
    actor_user_id: '',
    resource_type: '',
    action: '',
    result: '',
  })
  const [showFilters, setShowFilters] = useState(false)
  const [detailLog, setDetailLog] = useState<AuditLog | null>(null)

  const canRead = useHasCapability('audit.read')

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  useEffect(() => {
    if (!authLoading && isAuthenticated && !canRead) {
      addToast('无权限访问审计日志', 'error')
      navigate('/files', { replace: true })
    }
  }, [authLoading, isAuthenticated, canRead, navigate, addToast])

  const pageSize = 20

  const { data, isLoading } = useQuery({
    queryKey: ['audit-logs', page, filters],
    queryFn: () =>
      auditApi.list({
        page,
        page_size: pageSize,
        ...(filters.actor_user_id ? { actor_user_id: parseInt(filters.actor_user_id) } : {}),
        ...(filters.resource_type ? { resource_type: filters.resource_type } : {}),
        ...(filters.action ? { action: filters.action } : {}),
        ...(filters.result ? { result: filters.result } : {}),
      }),
    enabled: canRead,
  })

  const handleReset = () => {
    setFilters({ actor_user_id: '', resource_type: '', action: '', result: '' })
    setPage(1)
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const logs = data?.items || []
  const totalPages = data?.total_pages || 1

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">审计日志</h1>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowFilters(!showFilters)}
            className={cn(
              'flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors',
              showFilters
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
            )}
          >
            <Search className="w-4 h-4" />
            <span>筛选</span>
          </button>
          {showFilters && (
            <button
              onClick={handleReset}
              className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
            >
              重置
            </button>
          )}
        </div>
      </div>

      {showFilters && (
        <div className="px-4 py-3 border-b border-border bg-muted/30 space-y-2">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
            <div>
              <label className="text-xs text-muted-foreground mb-1 block">用户 ID</label>
              <input
                type="number"
                value={filters.actor_user_id}
                onChange={(e) => setFilters((f) => ({ ...f, actor_user_id: e.target.value }))}
                placeholder="例如: 1"
                className="w-full px-2.5 py-1.5 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div>
              <label className="text-xs text-muted-foreground mb-1 block">资源类型</label>
              <input
                type="text"
                value={filters.resource_type}
                onChange={(e) => setFilters((f) => ({ ...f, resource_type: e.target.value }))}
                placeholder="例如: file"
                className="w-full px-2.5 py-1.5 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div>
              <label className="text-xs text-muted-foreground mb-1 block">动作</label>
              <input
                type="text"
                value={filters.action}
                onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value }))}
                placeholder="例如: create"
                className="w-full px-2.5 py-1.5 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div>
              <label className="text-xs text-muted-foreground mb-1 block">结果</label>
              <select
                value={filters.result}
                onChange={(e) => setFilters((f) => ({ ...f, result: e.target.value }))}
                className="w-full px-2.5 py-1.5 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="">全部</option>
                <option value="success">成功</option>
                <option value="failure">失败</option>
                <option value="deny">拒绝</option>
              </select>
            </div>
          </div>
        </div>
      )}

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {logs.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <ScrollText className="w-12 h-12 opacity-30" />
            <p>暂无审计日志</p>
          </div>
        ) : (
          <div className="space-y-2">
            {logs.map((log) => (
              <button
                key={log.id}
                onClick={() => setDetailLog(log)}
                className="w-full flex items-center gap-3 p-3 rounded-lg border border-border bg-card hover:border-primary/30 transition-colors text-left"
              >
                <div className="w-10 h-10 rounded-lg bg-primary/10 flex items-center justify-center shrink-0">
                  <ScrollText className="w-5 h-5 text-primary" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-foreground font-medium truncate">{log.summary}</span>
                    <ResultBadge result={log.result} />
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                    <span>{log.actor.username}</span>
                    <span>·</span>
                    <span>{log.action}</span>
                    <span>·</span>
                    <span>{log.resource_type}</span>
                  </div>
                </div>
                <div className="text-xs text-muted-foreground shrink-0">
                  {formatDate(log.occurred_at)}
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between px-4 h-12 border-t border-border shrink-0">
          <span className="text-sm text-muted-foreground">
            共 {data?.total || 0} 条，第 {page} / {totalPages} 页
          </span>
          <div className="flex items-center gap-1">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <button
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
              className="p-1.5 rounded-md hover:bg-accent text-muted-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}

      <AuditLogDrawer log={detailLog} onClose={() => setDetailLog(null)} />
    </div>
  )
}
