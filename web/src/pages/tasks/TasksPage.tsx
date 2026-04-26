import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { taskApi } from '@/api/task'
import { sourceApi } from '@/api/source'
import {
  Play,
  Pause,
  X,
  Download,
  Clock,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Plus,
  Link as LinkIcon,
  HardDrive,
  Server,
} from 'lucide-react'
import { cn, formatBytes, formatDate, formatDuration, formatSpeed } from '@/utils'
import { useFileStore } from '@/stores/fileStore'
import type { DownloadTask } from '@/types/api'

const STATUS_LABELS: Record<DownloadTask['status'], string> = {
  pending: 'pending · 等待中',
  running: 'running · 下载中',
  paused: 'paused · 已暂停',
  completed: 'completed · 已完成',
  failed: 'failed · 失败',
  canceled: 'canceled · 已取消',
}

const STATUS_BADGE_CLASSES: Record<DownloadTask['status'], string> = {
  pending: 'bg-muted text-muted-foreground',
  running: 'bg-primary/10 text-primary',
  paused: 'bg-amber-500/10 text-amber-500',
  completed: 'bg-emerald-500/10 text-emerald-500',
  failed: 'bg-destructive/10 text-destructive',
  canceled: 'bg-muted text-muted-foreground',
}

function getTaskProgressPercent(task: DownloadTask) {
  if (task.status === 'completed') return 100
  if (Number.isFinite(task.progress) && task.progress > 0) {
    return Math.min(Math.max(task.progress, 0), 100)
  }
  if (task.total_bytes && task.downloaded_bytes > 0) {
    return Math.min(Math.max((task.downloaded_bytes / task.total_bytes) * 100, 0), 100)
  }
  return 0
}

function getTaskBytesLabel(task: DownloadTask) {
  const downloaded = task.downloaded_bytes || 0
  if (task.total_bytes) {
    return `${formatBytes(downloaded)} / ${formatBytes(task.total_bytes)}（${downloaded} / ${task.total_bytes} bytes）`
  }
  if (downloaded > 0) {
    return `已下载 ${formatBytes(downloaded)}（${downloaded} bytes）`
  }
  return '等待获取文件大小'
}

function shouldShowTaskProgress(task: DownloadTask) {
  return task.status === 'running'
    || task.status === 'paused'
    || task.status === 'completed'
    || task.status === 'failed'
    || task.downloaded_bytes > 0
}

function getCompletedTaskRefreshKey(task: DownloadTask) {
  return `${task.id}:${task.finished_at ?? task.updated_at}`
}

function invalidateCompletedTaskFileQueries(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['files'] })
  void queryClient.invalidateQueries({ queryKey: ['vfs'] })
}

function StatusIcon({ status }: { status: DownloadTask['status'] }) {
  switch (status) {
    case 'pending':
      return <Clock className="w-4 h-4 text-muted-foreground" />
    case 'running':
      return <Loader2 className="w-4 h-4 text-primary animate-spin" />
    case 'paused':
      return <Pause className="w-4 h-4 text-warning" />
    case 'completed':
      return <CheckCircle2 className="w-4 h-4 text-emerald-500" />
    case 'failed':
      return <AlertCircle className="w-4 h-4 text-destructive" />
    case 'canceled':
      return <X className="w-4 h-4 text-muted-foreground" />
  }
}

function CreateTaskModal({
  onClose,
  onSubmit,
}: {
  onClose: () => void
  onSubmit: (url: string, sourceId: number, savePath: string) => void
}) {
  const { currentSource } = useFileStore()
  const [url, setUrl] = useState('')
  const [sourceId, setSourceId] = useState(currentSource ? String(currentSource.id) : '')
  const [savePath, setSavePath] = useState('/')

  const { data: sourcesData } = useQuery({
    queryKey: ['sources-task-modal'],
    queryFn: () => sourceApi.list({ page: 1, page_size: 100 }),
  })
  const sources = sourcesData?.items || []
  const effectiveSourceId = sourceId || (sources[0]?.id ? String(sources[0].id) : '')

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-md bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Plus className="w-4 h-4" />
            新建下载任务
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className="p-4 space-y-3">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">下载链接</label>
            <div className="relative">
              <LinkIcon className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <input
                type="text"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://example.com/file.zip"
                className="w-full pl-8 pr-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">存储源</label>
            <div className="relative">
              <Server className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <select
                value={effectiveSourceId}
                onChange={(e) => setSourceId(e.target.value)}
                className="w-full pl-8 pr-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring appearance-none"
              >
                {sources.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            </div>
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">保存路径</label>
            <div className="relative">
              <HardDrive className="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <input
                type="text"
                value={savePath}
                onChange={(e) => setSavePath(e.target.value)}
                className="w-full pl-8 pr-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button
              onClick={onClose}
              className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
            >
              取消
            </button>
            <button
              onClick={() => {
                const sid = parseInt(effectiveSourceId, 10)
                if (url.trim() && sid > 0) {
                  onSubmit(url.trim(), sid, savePath)
                }
              }}
              disabled={!url.trim() || !effectiveSourceId}
              className="px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              创建任务
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export function TasksPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const refreshedCompletedTasksRef = useRef<Set<string>>(new Set())

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data, isLoading } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => taskApi.list({ page: 1, page_size: 100 }),
    refetchInterval: 3000,
  })
  const taskItems = useMemo(() => data?.items ?? [], [data?.items])

  useEffect(() => {
    let hasNewCompletedTask = false

    for (const task of taskItems) {
      if (task.status !== 'completed') continue

      const key = getCompletedTaskRefreshKey(task)
      if (refreshedCompletedTasksRef.current.has(key)) continue

      refreshedCompletedTasksRef.current.add(key)
      hasNewCompletedTask = true
    }

    if (hasNewCompletedTask) {
      invalidateCompletedTaskFileQueries(queryClient)
    }
  }, [queryClient, taskItems])

  const handleCreate = async (url: string, sourceId: number, savePath: string) => {
    try {
      await taskApi.create({ type: 'download', url, source_id: sourceId, save_path: savePath })
      queryClient.invalidateQueries({ queryKey: ['tasks'] })
      setCreateModalOpen(false)
    } catch {
      // ignore
    }
  }

  const handlePause = async (id: number) => {
    try {
      await taskApi.pause(id)
      queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch {
      // ignore
    }
  }

  const handleResume = async (id: number) => {
    try {
      await taskApi.resume(id)
      queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch {
      // ignore
    }
  }

  const handleCancel = async (id: number) => {
    try {
      await taskApi.cancel(id)
      queryClient.invalidateQueries({ queryKey: ['tasks'] })
    } catch {
      // ignore
    }
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const tasks = taskItems

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">离线下载</h1>
        <button
          onClick={() => setCreateModalOpen(true)}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
        >
          <Download className="w-4 h-4" />
          <span>新建任务</span>
        </button>
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {tasks.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Download className="w-12 h-12 opacity-30" />
            <p>暂无下载任务</p>
          </div>
        ) : (
          <div className="space-y-3">
            {tasks.map((task) => {
              const progressPercent = getTaskProgressPercent(task)
              const showProgress = shouldShowTaskProgress(task)

              return (
                <div
                  key={task.id}
                  className="p-4 rounded-lg border border-border bg-card"
                >
                <div className="flex items-start justify-between mb-3">
                  <div className="flex items-center gap-3 min-w-0">
                    <StatusIcon status={task.status} />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 min-w-0">
                        <h3 className="font-medium text-foreground truncate">
                          {task.display_name}
                        </h3>
                        <span
                          className={cn(
                            'shrink-0 rounded-full px-2 py-0.5 text-xs font-medium',
                            STATUS_BADGE_CLASSES[task.status]
                          )}
                        >
                          {STATUS_LABELS[task.status]}
                        </span>
                      </div>
                      <p className="text-xs text-muted-foreground truncate mt-0.5">
                        {task.source_url}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    {task.status === 'running' && (
                      <button
                        onClick={() => handlePause(task.id)}
                        className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                        title="暂停"
                      >
                        <Pause className="w-4 h-4" />
                      </button>
                    )}
                    {(task.status === 'paused' || task.status === 'failed') && (
                      <button
                        onClick={() => handleResume(task.id)}
                        className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                        title="继续"
                      >
                        <Play className="w-4 h-4" />
                      </button>
                    )}
                    {task.status !== 'completed' && task.status !== 'canceled' && (
                      <button
                        onClick={() => handleCancel(task.id)}
                        className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                        title="取消"
                      >
                        <X className="w-4 h-4" />
                      </button>
                    )}
                  </div>
                </div>

                {showProgress && (
                  <div className="mb-3">
                    <div className="flex justify-between text-xs text-muted-foreground mb-1">
                      <span>{getTaskBytesLabel(task)}</span>
                      <span>{Math.round(progressPercent)}%</span>
                    </div>
                    <div className="w-full h-1.5 bg-muted rounded-full overflow-hidden">
                      <div
                        className={cn(
                          'h-full rounded-full transition-all',
                          task.status === 'completed' ? 'bg-emerald-500' : 'bg-primary'
                        )}
                        style={{ width: `${progressPercent}%` }}
                      />
                    </div>
                    <div className="flex justify-between text-xs text-muted-foreground mt-1">
                      <span>
                        {task.status === 'completed'
                          ? '下载已完成'
                          : formatSpeed(task.speed_bytes)}
                      </span>
                      <span>
                        {task.status === 'completed'
                          ? '100%'
                          : task.eta_seconds !== null
                            ? `剩余 ${formatDuration(task.eta_seconds)}`
                            : '剩余时间未知'}
                      </span>
                    </div>
                  </div>
                )}

                {task.status === 'failed' && task.error_message && (
                  <p className="text-xs text-destructive mb-2">{task.error_message}</p>
                )}

                <div className="flex items-center gap-4 text-xs text-muted-foreground">
                  <span>保存至: {task.save_path}</span>
                  <span>创建于: {formatDate(task.created_at)}</span>
                  {task.status === 'completed' && task.finished_at && (
                    <span>完成于: {formatDate(task.finished_at)}</span>
                  )}
                </div>
              </div>
              )
            })}
          </div>
        )}
      </div>

      {createModalOpen && (
        <CreateTaskModal
          onClose={() => setCreateModalOpen(false)}
          onSubmit={handleCreate}
        />
      )}
    </div>
  )
}
