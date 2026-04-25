import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aclApi } from '@/api/acl'
import { sourceApi } from '@/api/source'
import { useUIStore } from '@/stores/uiStore'
import {
  Shield,
  Plus,
  Trash2,
  X,
  Pencil,
  Check,
  XCircle,
  User,
  Users,
} from 'lucide-react'
import { cn } from '@/utils'
import { useHasCapability } from '@/hooks/useCapability'
import type { AclRule, StorageSource } from '@/types/api'

function EffectBadge({ effect }: { effect: AclRule['effect'] }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium',
        effect === 'allow'
          ? 'bg-emerald-500/10 text-emerald-500'
          : 'bg-destructive/10 text-destructive'
      )}
    >
      {effect === 'allow' ? <Check className="w-3 h-3" /> : <XCircle className="w-3 h-3" />}
      {effect === 'allow' ? '允许' : '拒绝'}
    </span>
  )
}

function SubjectBadge({ type }: { type: AclRule['subject_type'] }) {
  return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-primary/10 text-primary">
      {type === 'user' ? <User className="w-3 h-3" /> : <Users className="w-3 h-3" />}
      {type === 'user' ? '用户' : '角色'}
    </span>
  )
}

function PermissionsDisplay({ perms }: { perms: AclRule['permissions'] | undefined }) {
  const labels = [
    { key: 'read', label: '读' },
    { key: 'write', label: '写' },
    { key: 'delete', label: '删' },
    { key: 'share', label: '分享' },
  ] as const
  return (
    <div className="flex items-center gap-1">
      {labels.map(({ key, label }) => {
        const enabled = perms?.[key] ?? false
        return (
          <span
            key={key}
            className={cn(
              'text-xs px-1.5 py-0.5 rounded border',
              enabled
                ? 'bg-primary/10 text-primary border-primary/20'
                : 'bg-muted text-muted-foreground/40 border-transparent line-through'
            )}
          >
            {label}
          </span>
        )
      })}
    </div>
  )
}

function AclRuleModal({
  isOpen,
  onClose,
  onSuccess,
  rule,
  sources,
}: {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
  rule: AclRule | null
  sources: StorageSource[]
}) {
  const { addToast } = useUIStore()
  const [sourceId, setSourceId] = useState('')
  const [path, setPath] = useState('/')
  const [subjectType, setSubjectType] = useState<'user' | 'role'>('user')
  const [subjectId, setSubjectId] = useState('')
  const [effect, setEffect] = useState<'allow' | 'deny'>('allow')
  const [priority, setPriority] = useState('0')
  const [read, setRead] = useState(true)
  const [write, setWrite] = useState(false)
  const [deleteP, setDeleteP] = useState(false)
  const [share, setShare] = useState(false)
  const [inherit, setInherit] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (isOpen && rule) {
      setSourceId(String(rule.source_id))
      setPath(rule.path)
      setSubjectType(rule.subject_type)
      setSubjectId(String(rule.subject_id))
      setEffect(rule.effect)
      setPriority(String(rule.priority))
      setRead(rule.permissions.read)
      setWrite(rule.permissions.write)
      setDeleteP(rule.permissions.delete)
      setShare(rule.permissions.share)
      setInherit(rule.inherit_to_children)
    } else if (isOpen) {
      setSourceId(sources[0]?.id ? String(sources[0].id) : '')
      setPath('/')
      setSubjectType('user')
      setSubjectId('')
      setEffect('allow')
      setPriority('0')
      setRead(true)
      setWrite(false)
      setDeleteP(false)
      setShare(false)
      setInherit(true)
    }
  }, [isOpen, rule, sources])

  if (!isOpen) return null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const sid = parseInt(sourceId, 10)
    const subId = parseInt(subjectId, 10)
    const pri = parseInt(priority, 10)
    if (!sid || !subId || isNaN(pri)) {
      addToast('请填写完整的规则信息', 'error')
      return
    }

    const data = {
      source_id: sid,
      path: path.trim() || '/',
      subject_type: subjectType,
      subject_id: subId,
      effect,
      priority: pri,
      permissions: { read, write, delete: deleteP, share },
      inherit_to_children: inherit,
    }

    setIsSubmitting(true)
    try {
      if (rule) {
        await aclApi.update(rule.id, data)
        addToast('规则已更新', 'success')
      } else {
        await aclApi.create(data)
        addToast('规则已创建', 'success')
      }
      onSuccess()
      onClose()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '操作失败'
      addToast(msg, 'error')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-md bg-card border border-border rounded-lg shadow-xl max-h-[90vh] overflow-auto">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border shrink-0">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Shield className="w-4 h-4" />
            {rule ? '编辑 ACL 规则' : '创建 ACL 规则'}
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">存储源</label>
            <select
              value={sourceId}
              onChange={(e) => setSourceId(e.target.value)}
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            >
              {sources.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">路径</label>
            <input
              type="text"
              value={path}
              onChange={(e) => setPath(e.target.value)}
              placeholder="/"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">主体类型</label>
              <div className="flex gap-2">
                {(['user', 'role'] as const).map((t) => (
                  <button
                    key={t}
                    type="button"
                    onClick={() => setSubjectType(t)}
                    className={cn(
                      'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                      subjectType === t
                        ? 'border-primary bg-primary/5 text-primary'
                        : 'border-border text-muted-foreground hover:border-primary/30'
                    )}
                  >
                    {t === 'user' ? '用户' : '角色'}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">主体 ID</label>
              <input
                type="number"
                value={subjectId}
                onChange={(e) => setSubjectId(e.target.value)}
                placeholder="1"
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">效果</label>
              <div className="flex gap-2">
                {(['allow', 'deny'] as const).map((e) => (
                  <button
                    key={e}
                    type="button"
                    onClick={() => setEffect(e)}
                    className={cn(
                      'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                      effect === e
                        ? e === 'allow'
                          ? 'border-emerald-500 bg-emerald-500/5 text-emerald-500'
                          : 'border-destructive bg-destructive/5 text-destructive'
                        : 'border-border text-muted-foreground hover:border-primary/30'
                    )}
                  >
                    {e === 'allow' ? '允许' : '拒绝'}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <label className="text-sm text-muted-foreground mb-1 block">优先级</label>
              <input
                type="number"
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
                className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">权限</label>
            <div className="flex gap-3">
              {[
                { key: 'read', label: '读取', state: read, set: setRead },
                { key: 'write', label: '写入', state: write, set: setWrite },
                { key: 'delete', label: '删除', state: deleteP, set: setDeleteP },
                { key: 'share', label: '分享', state: share, set: setShare },
              ].map(({ key, label, state, set }) => (
                <label key={key} className="flex items-center gap-1.5 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={state}
                    onChange={(e) => set(e.target.checked)}
                    className="rounded border-border"
                  />
                  <span className="text-sm text-foreground">{label}</span>
                </label>
              ))}
            </div>
          </div>
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={inherit}
              onChange={(e) => setInherit(e.target.checked)}
              className="rounded border-border"
            />
            <span className="text-sm text-foreground">继承到子目录</span>
          </label>
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
              disabled={isSubmitting || !sourceId || !subjectId}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || !sourceId || !subjectId) && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '保存中...' : rule ? '更新' : '创建'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export function AclPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading } = useAuthStore()
  const { addToast } = useUIStore()
  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<AclRule | null>(null)
  const [currentSourceId, setCurrentSourceId] = useState<number | null>(null)

  const canRead = useHasCapability('acl.read')
  const canManage = useHasCapability('acl.manage')

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  useEffect(() => {
    if (!authLoading && isAuthenticated && !canRead) {
      addToast('无权限访问 ACL 管理', 'error')
      navigate('/files', { replace: true })
    }
  }, [authLoading, isAuthenticated, canRead, navigate, addToast])

  const { data: sourcesData } = useQuery({
    queryKey: ['sources-acl'],
    queryFn: () => sourceApi.list({ page: 1, page_size: 100, view: 'admin' }),
    enabled: canRead,
  })

  const sources = sourcesData?.items || []

  useEffect(() => {
    if (sources.length > 0 && currentSourceId === null) {
      setCurrentSourceId(sources[0].id)
    }
  }, [sources, currentSourceId])

  const { data, isLoading, error } = useQuery({
    queryKey: ['acl-rules', currentSourceId],
    queryFn: () =>
      aclApi.list({
        source_id: currentSourceId!,
        page: 1,
        page_size: 100,
      }),
    enabled: canRead && currentSourceId !== null,
  })

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除此 ACL 规则吗？')) return
    try {
      await aclApi.delete(id)
      addToast('规则已删除', 'success')
      queryClient.invalidateQueries({ queryKey: ['acl-rules', currentSourceId] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '删除失败'
      addToast(msg, 'error')
    }
  }

  if (authLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const rules = data?.items || []

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold text-foreground">ACL 管理</h1>
          {sources.length > 0 && (
            <select
              value={currentSourceId ?? ''}
              onChange={(e) => setCurrentSourceId(Number(e.target.value))}
              className="px-3 py-1.5 rounded-md border border-border bg-card text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            >
              {sources.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          )}
        </div>
        {canManage && (
          <button
            onClick={() => {
              setEditTarget(null)
              setModalOpen(true)
            }}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            <Plus className="w-4 h-4" />
            <span>创建规则</span>
          </button>
        )}
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {currentSourceId === null ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Shield className="w-12 h-12 opacity-30" />
            <p>请选择存储源</p>
          </div>
        ) : isLoading ? (
          <div className="flex-1 flex items-center justify-center">
            <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Shield className="w-12 h-12 opacity-30" />
            <p className="text-destructive">{(error as Error).message || '加载失败'}</p>
          </div>
        ) : rules.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Shield className="w-12 h-12 opacity-30" />
            <p>暂无 ACL 规则</p>
            {canManage && (
              <button
                onClick={() => {
                  setEditTarget(null)
                  setModalOpen(true)
                }}
                className="px-4 py-2 rounded-md bg-primary text-primary-foreground text-sm hover:bg-primary/90 transition-colors"
              >
                创建规则
              </button>
            )}
          </div>
        ) : (
          <div className="border border-border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-muted/50">
                <tr className="border-b border-border text-muted-foreground">
                  <th className="px-4 py-2 text-left font-medium">存储源</th>
                  <th className="px-4 py-2 text-left font-medium">路径</th>
                  <th className="px-4 py-2 text-left font-medium">主体</th>
                  <th className="px-4 py-2 text-left font-medium">效果</th>
                  <th className="px-4 py-2 text-left font-medium">权限</th>
                  <th className="px-4 py-2 text-left font-medium w-20">优先级</th>
                  <th className="px-4 py-2 text-right font-medium w-24">操作</th>
                </tr>
              </thead>
              <tbody>
                {rules.map((rule) => (
                  <tr key={rule.id} className="border-b border-border/50 hover:bg-accent/30 transition-colors">
                    <td className="px-4 py-2.5">
                      {sources.find((s) => s.id === rule.source_id)?.name || rule.source_id}
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs">{rule.path}</td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center gap-1.5">
                        <SubjectBadge type={rule.subject_type} />
                        <span className="text-xs text-muted-foreground">ID:{rule.subject_id}</span>
                      </div>
                    </td>
                    <td className="px-4 py-2.5">
                      <EffectBadge effect={rule.effect} />
                    </td>
                    <td className="px-4 py-2.5">
                      <PermissionsDisplay perms={rule.permissions} />
                    </td>
                    <td className="px-4 py-2.5 text-muted-foreground">{rule.priority}</td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center justify-end gap-1">
                        {canManage && (
                          <>
                            <button
                              onClick={() => {
                                setEditTarget(rule)
                                setModalOpen(true)
                              }}
                              className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                              title="编辑"
                            >
                              <Pencil className="w-3.5 h-3.5" />
                            </button>
                            <button
                              onClick={() => handleDelete(rule.id)}
                              className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                              title="删除"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <AclRuleModal
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        onSuccess={() => queryClient.invalidateQueries({ queryKey: ['acl-rules', currentSourceId] })}
        rule={editTarget}
        sources={sources}
      />
    </div>
  )
}
