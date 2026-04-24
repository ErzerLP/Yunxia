import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { userApi } from '@/api/user'
import { useHasCapability } from '@/hooks/useCapability'
import {
  Users,
  Plus,
  Trash2,
  X,
  Pencil,
  Lock,
  Unlock,
  KeyRound,
  Ban,
  Shield,
  User,
  UserCog,
} from 'lucide-react'
import { formatDate } from '@/utils'
import { cn } from '@/utils'
import type { User as UserType } from '@/types/api'

const roleLabels: Record<string, string> = {
  super_admin: '超级管理员',
  admin: '管理员',
  operator: '操作员',
  user: '普通用户',
}

const roleBadgeClass: Record<string, string> = {
  super_admin: 'bg-destructive/10 text-destructive',
  admin: 'bg-primary/10 text-primary',
  operator: 'bg-amber-500/10 text-amber-600',
  user: 'bg-muted text-muted-foreground',
}

function CreateUserModal({
  isOpen,
  onClose,
  onSuccess,
}: {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
}) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [email, setEmail] = useState('')
  const [roleKey, setRoleKey] = useState('user')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (isOpen) {
      setUsername('')
      setPassword('')
      setEmail('')
      setRoleKey('user')
    }
  }, [isOpen])

  if (!isOpen) return null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username.trim() || !password.trim()) return
    setIsSubmitting(true)
    try {
      await userApi.create({
        username: username.trim(),
        password: password.trim(),
        email: email.trim() || undefined,
        role_key: roleKey,
      })
      onSuccess()
      onClose()
    } catch {
      // ignore
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
            <Plus className="w-4 h-4" />
            创建用户
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">用户名</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="username"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              required
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="******"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              required
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">邮箱（可选）</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="user@example.com"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">角色</label>
            <div className="flex gap-2">
              {(['user', 'operator', 'admin', 'super_admin'] as const).map((r) => (
                <button
                  key={r}
                  type="button"
                  onClick={() => setRoleKey(r)}
                  className={cn(
                    'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                    roleKey === r
                      ? 'border-primary bg-primary/5 text-primary'
                      : 'border-border text-muted-foreground hover:border-primary/30'
                  )}
                >
                  {roleLabels[r]}
                </button>
              ))}
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
              disabled={isSubmitting || !username.trim() || !password.trim()}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || !username.trim() || !password.trim()) && 'opacity-50 cursor-not-allowed'
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

function EditUserModal({
  isOpen,
  onClose,
  onSuccess,
  user,
}: {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
  user: UserType | null
}) {
  const [email, setEmail] = useState('')
  const [roleKey, setRoleKey] = useState('user')
  const [status, setStatus] = useState<'active' | 'locked'>('active')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (isOpen && user) {
      setEmail(user.email || '')
      setRoleKey(user.role_key)
      setStatus(user.status)
    }
  }, [isOpen, user])

  if (!isOpen || !user) return null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsSubmitting(true)
    try {
      const data: { email?: string; role_key?: string; status?: 'active' | 'locked' } = {}
      if (email.trim()) data.email = email.trim()
      if (roleKey !== user.role_key) data.role_key = roleKey
      if (status !== user.status) data.status = status
      if (Object.keys(data).length > 0) {
        await userApi.update(user.id, data)
      }
      onSuccess()
      onClose()
    } catch {
      // ignore
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
            <Pencil className="w-4 h-4" />
            编辑用户
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">用户名</label>
            <input
              type="text"
              value={user.username}
              disabled
              className="w-full px-3 py-2 rounded-md border border-input bg-muted text-muted-foreground text-sm"
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">邮箱</label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="user@example.com"
              className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">角色</label>
            <div className="flex gap-2">
              {(['user', 'operator', 'admin', 'super_admin'] as const).map((r) => (
                <button
                  key={r}
                  type="button"
                  onClick={() => setRoleKey(r)}
                  className={cn(
                    'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                    roleKey === r
                      ? 'border-primary bg-primary/5 text-primary'
                      : 'border-border text-muted-foreground hover:border-primary/30'
                  )}
                >
                  {roleLabels[r]}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="text-sm text-muted-foreground mb-1 block">状态</label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setStatus('active')}
                className={cn(
                  'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                  status === 'active'
                    ? 'border-emerald-500 bg-emerald-500/5 text-emerald-600'
                    : 'border-border text-muted-foreground hover:border-emerald-500/30'
                )}
              >
                <Unlock className="w-3.5 h-3.5 inline mr-1" />
                正常
              </button>
              <button
                type="button"
                onClick={() => setStatus('locked')}
                className={cn(
                  'flex-1 px-3 py-2 rounded-md border text-sm transition-colors',
                  status === 'locked'
                    ? 'border-destructive bg-destructive/5 text-destructive'
                    : 'border-border text-muted-foreground hover:border-destructive/30'
                )}
              >
                <Lock className="w-3.5 h-3.5 inline mr-1" />
                锁定
              </button>
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

function ResetPasswordModal({
  isOpen,
  onClose,
  onSuccess,
  user,
}: {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
  user: UserType | null
}) {
  const [password, setPassword] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (isOpen) setPassword('')
  }, [isOpen])

  if (!isOpen || !user) return null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!password.trim()) return
    setIsSubmitting(true)
    try {
      await userApi.resetPassword(user.id, password.trim())
      onSuccess()
      onClose()
    } catch {
      // ignore
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-sm bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <KeyRound className="w-4 h-4" />
            重置密码
          </h3>
          <button onClick={onClose} className="p-1.5 rounded-md hover:bg-accent text-muted-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="p-4 space-y-3">
          <p className="text-sm text-muted-foreground">
            为 <span className="font-medium text-foreground">{user.username}</span> 设置新密码
          </p>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="新密码"
            className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            required
          />
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
              disabled={isSubmitting || !password.trim()}
              className={cn(
                'px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors',
                (isSubmitting || !password.trim()) && 'opacity-50 cursor-not-allowed'
              )}
            >
              {isSubmitting ? '重置中...' : '重置'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export function UsersPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { isAuthenticated, isLoading: authLoading, user: currentUser } = useAuthStore()
  const { addToast } = useUIStore()
  const canCreate = useHasCapability('user.create')
  const canUpdate = useHasCapability('user.update')
  const canDelete = useHasCapability('user.delete')
  const canResetPassword = useHasCapability('user.password.reset')
  const canRevokeTokens = useHasCapability('user.tokens.revoke')

  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<UserType | null>(null)
  const [resetTarget, setResetTarget] = useState<UserType | null>(null)

  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, authLoading, navigate])

  const { data, isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: () => userApi.list({ page: 1, page_size: 100 }),
  })

  const handleDelete = async (id: number) => {
    if (id === currentUser?.id) {
      addToast('不能删除当前登录用户', 'error')
      return
    }
    if (!confirm('确定要删除此用户吗？此操作不可撤销。')) return
    try {
      await userApi.delete(id)
      addToast('用户已删除', 'success')
      queryClient.invalidateQueries({ queryKey: ['users'] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '删除失败'
      addToast(msg, 'error')
    }
  }

  const handleRevokeTokens = async (id: number) => {
    if (!confirm('确定要吊销该用户的所有 Token 吗？用户将被强制登出。')) return
    try {
      await userApi.revokeTokens(id)
      addToast('Token 已吊销', 'success')
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '操作失败'
      addToast(msg, 'error')
    }
  }

  if (authLoading || isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  const users = data?.items || []

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
        <h1 className="text-lg font-semibold text-foreground">用户管理</h1>
        {canCreate && (
          <button
            onClick={() => setCreateModalOpen(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            <Plus className="w-4 h-4" />
            <span>创建用户</span>
          </button>
        )}
      </div>

      <div className="flex-1 overflow-auto scrollbar-thin p-4">
        {users.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-3">
            <Users className="w-12 h-12 opacity-30" />
            <p>暂无用户</p>
          </div>
        ) : (
          <div className="space-y-2">
            {users.map((u) => (
              <div
                key={u.id}
                className="flex items-center gap-3 p-3 rounded-lg border border-border bg-card"
              >
                <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
                  {u.role_key === 'super_admin' ? (
                    <Shield className="w-5 h-5 text-destructive" />
                  ) : u.role_key === 'admin' ? (
                    <UserCog className="w-5 h-5 text-primary" />
                  ) : (
                    <User className="w-5 h-5 text-muted-foreground" />
                  )}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-foreground">{u.username}</span>
                    <span
                      className={cn(
                        'text-xs px-2 py-0.5 rounded-full',
                        roleBadgeClass[u.role_key]
                      )}
                    >
                      {roleLabels[u.role_key]}
                    </span>
                    {u.status === 'locked' && (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-destructive/10 text-destructive">
                        <Ban className="w-3 h-3 inline mr-0.5" />
                        已锁定
                      </span>
                    )}
                    {u.id === currentUser?.id && (
                      <span className="text-xs px-2 py-0.5 rounded-full bg-primary/10 text-primary">当前用户</span>
                    )}
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                    <span>{u.email || '无邮箱'}</span>
                    <span>创建于 {formatDate(u.created_at)}</span>
                  </div>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  {canResetPassword && (
                    <button
                      onClick={() => setResetTarget(u)}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      title="重置密码"
                    >
                      <KeyRound className="w-4 h-4" />
                    </button>
                  )}
                  {canRevokeTokens && u.id !== currentUser?.id && (
                    <button
                      onClick={() => handleRevokeTokens(u.id)}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      title="吊销 Token"
                    >
                      <Ban className="w-4 h-4" />
                    </button>
                  )}
                  {canUpdate && (
                    <button
                      onClick={() => setEditTarget(u)}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      title="编辑"
                    >
                      <Pencil className="w-4 h-4" />
                    </button>
                  )}
                  {canDelete && u.id !== currentUser?.id && (
                    <button
                      onClick={() => handleDelete(u.id)}
                      className="p-1.5 rounded-md hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                      title="删除"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <CreateUserModal
        isOpen={createModalOpen}
        onClose={() => setCreateModalOpen(false)}
        onSuccess={() => {
          queryClient.invalidateQueries({ queryKey: ['users'] })
          addToast('用户创建成功', 'success')
        }}
      />
      <EditUserModal
        isOpen={!!editTarget}
        onClose={() => setEditTarget(null)}
        onSuccess={() => {
          queryClient.invalidateQueries({ queryKey: ['users'] })
          addToast('用户更新成功', 'success')
        }}
        user={editTarget}
      />
      <ResetPasswordModal
        isOpen={!!resetTarget}
        onClose={() => setResetTarget(null)}
        onSuccess={() => addToast('密码重置成功', 'success')}
        user={resetTarget}
      />
    </div>
  )
}
