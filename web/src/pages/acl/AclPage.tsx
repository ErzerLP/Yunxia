import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { aclApi } from '@/api/acl'
import { fileV2Api } from '@/api/fileV2'
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
import { cn, getApiErrorMessage } from '@/utils'
import { useHasCapability } from '@/hooks/useCapability'
import { getVfsParentPath, normalizeVfsPath } from '@/utils/vfs'
import type { AclRule, CreateAclRuleRequest, StorageSource } from '@/types/api'

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

function AclRuleModal({
  onClose,
  onSuccess,
  rule,
  sources,
}: {
  onClose: () => void
  onSuccess: () => void
  rule: AclRule | null
  sources: StorageSource[]
}) {
  const { addToast } = useUIStore()
  const [vfsPath, setVfsPath] = useState(rule?.virtual_path || rule?.path || '/')
  const [legacySourceId, setLegacySourceId] = useState(rule?.source_id ? String(rule.source_id) : sources[0]?.id ? String(sources[0].id) : '')
  const [legacyPath, setLegacyPath] = useState(rule?.path ?? '/')
  const [subjectType, setSubjectType] = useState<'user' | 'role'>(rule?.subject_type ?? 'user')
  const [subjectId, setSubjectId] = useState(rule ? String(rule.subject_id) : '')
  const [effect, setEffect] = useState<'allow' | 'deny'>(rule?.effect ?? 'allow')
  const [priority, setPriority] = useState(rule ? String(rule.priority) : '0')
  const [read, setRead] = useState(rule?.permissions.read ?? true)
  const [write, setWrite] = useState(rule?.permissions.write ?? false)
  const [deleteP, setDeleteP] = useState(rule?.permissions.delete ?? false)
  const [share, setShare] = useState(rule?.permissions.share ?? false)
  const [inherit, setInherit] = useState(rule?.inherit_to_children ?? true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState('')

  const resolveAclTarget = async (targetPath: string): Promise<Pick<CreateAclRuleRequest, 'vfs_node_id' | 'source_id' | 'path'> | null> => {
    const normalizedPath = normalizeVfsPath(targetPath)
    if (
      rule?.vfs_node_id
      && normalizeVfsPath(rule.virtual_path || rule.path) === normalizedPath
    ) {
      return { vfs_node_id: rule.vfs_node_id }
    }

    if (normalizedPath !== '/') {
      const parentPath = getVfsParentPath(normalizedPath)
      const list = await fileV2Api.list({ path: parentPath, page: 1, page_size: 200 })
      const target = list.items.find((item) => normalizeVfsPath(item.path) === normalizedPath)
      if (target?.id) {
        return { vfs_node_id: target.id }
      }
    }

    const sid = parseInt(legacySourceId, 10)
    if (sid && legacyPath.trim()) {
      return {
        source_id: sid,
        path: legacyPath.trim(),
      }
    }
    return null
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const subId = parseInt(subjectId, 10)
    const pri = parseInt(priority, 10)
    if (!subId || isNaN(pri)) {
      addToast('请填写完整的规则信息', 'error')
      return
    }

    setIsSubmitting(true)
    setError('')
    try {
      const target = await resolveAclTarget(vfsPath)
      if (!target) {
        throw new Error('未找到可绑定的 VFS 节点；请先进入该目录触发索引，或填写兼容 source_id + 源内路径。')
      }
      const data: CreateAclRuleRequest = {
        ...target,
        subject_type: subjectType,
        subject_id: subId,
        effect,
        priority: pri,
        permissions: { read, write, delete: deleteP, share },
        inherit_to_children: inherit,
      }

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
      const msg = getApiErrorMessage(err, '操作失败')
      setError(msg)
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
            <label htmlFor="acl-rule-vfs-path" className="text-sm text-muted-foreground mb-1 block">目标 VFS 路径</label>
            <input
              id="acl-rule-vfs-path"
              name="vfs_path"
              type="text"
              value={vfsPath}
              onChange={(e) => setVfsPath(e.target.value)}
              placeholder="/local/docs"
              autoComplete="off"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
            />
            <p className="mt-1 text-xs text-muted-foreground">
              优先解析为 VFS node id；node-first 规则在目标重命名/移动后仍跟随同一节点。
            </p>
          </div>
          <div className="rounded-md border border-border bg-muted/20 p-3 space-y-3">
            <p className="text-xs font-medium text-muted-foreground">兼容 fallback（无法解析 VFS 节点时填写旧 source_id + 源内路径）</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label htmlFor="acl-rule-legacy-source-id" className="text-sm text-muted-foreground mb-1 block">存储源</label>
                <select
                  id="acl-rule-legacy-source-id"
                  name="legacy_source_id"
                  value={legacySourceId}
                  onChange={(e) => setLegacySourceId(e.target.value)}
                  className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                >
                  <option value="">不使用 fallback</option>
                  {sources.map((s) => (
                    <option key={s.id} value={s.id}>{s.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label htmlFor="acl-rule-legacy-path" className="text-sm text-muted-foreground mb-1 block">源内路径</label>
                <input
                  id="acl-rule-legacy-path"
                  name="legacy_path"
                  type="text"
                  value={legacyPath}
                  onChange={(e) => setLegacyPath(e.target.value)}
                  placeholder="/docs"
                  autoComplete="off"
                  className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
                />
              </div>
            </div>
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
              <label htmlFor="acl-rule-subject-id" className="text-sm text-muted-foreground mb-1 block">主体 ID</label>
              <input
                id="acl-rule-subject-id"
                name="subject_id"
                type="number"
                value={subjectId}
                onChange={(e) => setSubjectId(e.target.value)}
                placeholder="1"
                autoComplete="off"
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
              <label htmlFor="acl-rule-priority" className="text-sm text-muted-foreground mb-1 block">优先级</label>
              <input
                id="acl-rule-priority"
                name="priority"
                type="number"
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
                autoComplete="off"
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
                <label key={key} htmlFor={`acl-rule-permission-${key}`} className="flex items-center gap-1.5 cursor-pointer">
                  <input
                    id={`acl-rule-permission-${key}`}
                    name={`permission_${key}`}
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
          <label htmlFor="acl-rule-inherit" className="flex items-center gap-2 cursor-pointer">
            <input
              id="acl-rule-inherit"
              name="inherit_to_children"
              type="checkbox"
              checked={inherit}
              onChange={(e) => setInherit(e.target.checked)}
              className="rounded border-border"
            />
            <span className="text-sm text-foreground">继承到子目录</span>
          </label>
          {error && (
            <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
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
              disabled={isSubmitting || !subjectId}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || !subjectId) && 'opacity-50 cursor-not-allowed'
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

  const sources = useMemo(() => sourcesData?.items ?? [], [sourcesData?.items])
  const selectedSourceId = currentSourceId ?? sources[0]?.id ?? null

  const { data, isLoading, error } = useQuery({
    queryKey: ['acl-rules', selectedSourceId],
    queryFn: () =>
      aclApi.list({
        source_id: selectedSourceId!,
        page: 1,
        page_size: 100,
      }),
    enabled: canRead && selectedSourceId !== null,
  })

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除此 ACL 规则吗？')) return
    try {
      await aclApi.delete(id)
      addToast('规则已删除', 'success')
      queryClient.invalidateQueries({ queryKey: ['acl-rules', selectedSourceId] })
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
            <div className="flex items-center gap-2">
              <label htmlFor="acl-current-source" className="text-sm text-muted-foreground">
                存储源
              </label>
              <select
                id="acl-current-source"
                name="source_id"
                value={selectedSourceId ?? ''}
                onChange={(e) => setCurrentSourceId(Number(e.target.value))}
                className="px-3 py-1.5 rounded-md border border-border bg-card text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
              >
                {sources.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            </div>
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
        {selectedSourceId === null ? (
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
                  <th className="px-4 py-2 text-left font-medium">目标</th>
                  <th className="px-4 py-2 text-left font-medium">快照路径</th>
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
                      <div className="space-y-1">
                        {rule.vfs_node_id ? (
                          <span className="inline-flex rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                            VFS 节点 #{rule.vfs_node_id}
                          </span>
                        ) : (
                          <span className="inline-flex rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                            Legacy source/path
                          </span>
                        )}
                        <p className="text-xs text-muted-foreground">
                          存储源：{rule.source_id ? sources.find((s) => s.id === rule.source_id)?.name || rule.source_id : '按节点解析'}
                        </p>
                      </div>
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="space-y-1 font-mono text-xs">
                        <p title={rule.virtual_path || rule.path}>{rule.virtual_path || rule.path}</p>
                        {rule.virtual_path && rule.path && rule.virtual_path !== rule.path && (
                          <p className="text-muted-foreground" title={rule.path}>源内：{rule.path}</p>
                        )}
                      </div>
                    </td>
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
                      <div className="flex items-center gap-1">
                        {[
                          { key: 'read' as const, label: '读' },
                          { key: 'write' as const, label: '写' },
                          { key: 'delete' as const, label: '删' },
                          { key: 'share' as const, label: '分享' },
                        ].filter(({ key }) => rule.permissions?.[key] ?? false)
                          .map(({ key, label }) => (
                            <span
                              key={key}
                              className="text-xs px-1.5 py-0.5 rounded border bg-primary/10 text-primary border-primary/20"
                            >
                              {label}
                            </span>
                          ))}
                        {!rule.permissions?.read &&
                          !rule.permissions?.write &&
                          !rule.permissions?.delete &&
                          !rule.permissions?.share && (
                            <span className="text-xs text-muted-foreground">无</span>
                          )}
                      </div>
                      <span className="text-[10px] text-muted-foreground/60 ml-0 font-mono block mt-1">
                        {JSON.stringify(rule.permissions)}
                      </span>
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

      {modalOpen && (
        <AclRuleModal
          key={editTarget?.id ?? 'create'}
          onClose={() => setModalOpen(false)}
          onSuccess={() => queryClient.invalidateQueries({ queryKey: ['acl-rules', selectedSourceId] })}
          rule={editTarget}
          sources={sources}
        />
      )}
    </div>
  )
}
