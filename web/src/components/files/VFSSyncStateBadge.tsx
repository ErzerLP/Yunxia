import { cn } from '@/utils'
import type { VFSSyncState } from '@/types/api'

const SYNC_STATE_META: Partial<Record<VFSSyncState, { label: string; className: string; title: string }>> = {
  pending: {
    label: '待索引',
    className: 'border-amber-500/20 bg-amber-500/10 text-amber-500',
    title: '该节点等待后端索引，部分操作可能暂不可用',
  },
  syncing: {
    label: '同步中',
    className: 'border-primary/20 bg-primary/10 text-primary',
    title: '该节点正在同步，请稍后刷新目录',
  },
  stale: {
    label: '待刷新',
    className: 'border-amber-500/20 bg-amber-500/10 text-amber-500',
    title: '目录索引可能不是最新状态，建议手动刷新当前目录',
  },
  missing: {
    label: '已缺失',
    className: 'border-destructive/20 bg-destructive/10 text-destructive',
    title: '底层文件可能已不存在，下载已禁用',
  },
  conflict: {
    label: '冲突',
    className: 'border-destructive/20 bg-destructive/10 text-destructive',
    title: '后端检测到同名或身份冲突，需要刷新或人工处理',
  },
  error: {
    label: '同步异常',
    className: 'border-destructive/20 bg-destructive/10 text-destructive',
    title: '目录同步遇到错误，建议刷新目录或稍后重试',
  },
}

export function VFSSyncStateBadge({ syncState, canDownload }: { syncState?: VFSSyncState; canDownload?: boolean }) {
  const meta = syncState ? SYNC_STATE_META[syncState] : undefined
  if (!meta && canDownload !== false) return null

  if (!meta && canDownload === false) {
    return (
      <span
        className="text-xs px-1.5 py-0.5 rounded border border-amber-500/20 bg-amber-500/10 text-amber-500"
        title="后端返回该条目暂不可下载"
      >
        不可下载
      </span>
    )
  }

  return (
    <span
      className={cn('text-xs px-1.5 py-0.5 rounded border', meta?.className)}
      title={meta?.title}
    >
      {meta?.label}
    </span>
  )
}
