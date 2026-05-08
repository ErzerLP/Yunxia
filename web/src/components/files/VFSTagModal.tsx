import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus, Tag, X } from 'lucide-react'
import { fileV2Api } from '@/api/fileV2'
import { tagApi } from '@/api/tag'
import { useUIStore } from '@/stores/uiStore'
import { cn, getApiErrorMessage } from '@/utils'
import { getVfsParentPath } from '@/utils/vfs'
import type { VFSItem, VFSTag } from '@/types/api'

interface VFSTagModalProps {
  item: VFSItem
  onClose: () => void
}

const DEFAULT_TAG_COLORS = ['#2563eb', '#16a34a', '#d97706', '#dc2626', '#7c3aed', '#0f766e']

function isTagAttached(tag: VFSTag, attachedTags: VFSTag[]) {
  return attachedTags.some((candidate) => candidate.id === tag.id)
}

export function VFSTagModal({ item, onClose }: VFSTagModalProps) {
  const queryClient = useQueryClient()
  const { addToast } = useUIStore()
  const [newTagName, setNewTagName] = useState('')
  const [newTagColor, setNewTagColor] = useState(DEFAULT_TAG_COLORS[0])
  const [pendingTagId, setPendingTagId] = useState<number | null>(null)
  const [isCreating, setIsCreating] = useState(false)
  const [error, setError] = useState('')

  const allTagsQuery = useQuery({
    queryKey: ['vfs-tags'],
    queryFn: () => tagApi.list(),
  })

  const nodeTagsQuery = useQuery({
    queryKey: ['vfs-node-tags', item.path],
    queryFn: () => fileV2Api.getTags(item.path),
  })

  const allTags = allTagsQuery.data?.items ?? []
  const attachedTags = nodeTagsQuery.data?.tags ?? []

  const refreshTags = () => {
    void queryClient.invalidateQueries({ queryKey: ['vfs-tags'] })
    void queryClient.invalidateQueries({ queryKey: ['vfs-node-tags', item.path] })
  }

  const handleCreateTag = async (event: React.FormEvent) => {
    event.preventDefault()
    const trimmed = newTagName.trim()
    if (!trimmed) {
      setError('请填写标签名称')
      return
    }

    setIsCreating(true)
    setError('')
    try {
      const res = await tagApi.create({ name: trimmed, color: newTagColor })
      await fileV2Api.list({ path: getVfsParentPath(item.path), page: 1, page_size: 1 })
      await fileV2Api.attachTag({ path: item.path, tag_id: res.tag.id })
      setNewTagName('')
      addToast('标签已创建并绑定', 'success')
      refreshTags()
    } catch (err: unknown) {
      const message = getApiErrorMessage(err, '创建标签失败')
      setError(message)
      addToast(message, 'error')
    } finally {
      setIsCreating(false)
    }
  }

  const handleToggleTag = async (tag: VFSTag) => {
    const attached = isTagAttached(tag, attachedTags)
    setPendingTagId(tag.id)
    setError('')
    try {
      if (attached) {
        await fileV2Api.detachTag({ path: item.path, tag_id: tag.id })
        addToast('标签已解绑', 'success')
      } else {
        await fileV2Api.list({ path: getVfsParentPath(item.path), page: 1, page_size: 1 })
        await fileV2Api.attachTag({ path: item.path, tag_id: tag.id })
        addToast('标签已绑定', 'success')
      }
      refreshTags()
    } catch (err: unknown) {
      const message = getApiErrorMessage(err, attached ? '解绑标签失败' : '绑定标签失败')
      setError(message)
      addToast(message, 'error')
    } finally {
      setPendingTagId(null)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-card border border-border rounded-lg shadow-xl">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border">
          <h3 className="font-medium text-foreground flex items-center gap-2">
            <Tag className="w-4 h-4" />
            管理标签
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

        <div className="p-4 space-y-4">
          <div className="rounded-md border border-border bg-muted/20 px-3 py-2">
            <p className="text-sm text-foreground truncate">{item.name}</p>
            <p className="text-xs text-muted-foreground font-mono truncate" title={item.path}>{item.path}</p>
          </div>

          <form onSubmit={handleCreateTag} className="rounded-md border border-border p-3 space-y-3">
            <div className="flex items-end gap-2">
              <div className="min-w-0 flex-1">
                <label htmlFor="vfs-new-tag-name" className="text-sm text-muted-foreground mb-1 block">
                  新标签
                </label>
                <input
                  id="vfs-new-tag-name"
                  name="tag_name"
                  type="text"
                  value={newTagName}
                  onChange={(event) => setNewTagName(event.target.value)}
                  autoComplete="off"
                  placeholder="例如：工作资料"
                  className="w-full px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                />
              </div>
              <div>
                <label htmlFor="vfs-new-tag-color" className="text-sm text-muted-foreground mb-1 block">
                  颜色
                </label>
                <select
                  id="vfs-new-tag-color"
                  name="tag_color"
                  value={newTagColor}
                  onChange={(event) => setNewTagColor(event.target.value)}
                  className="px-3 py-2 rounded-md border border-input bg-background text-foreground text-sm focus:outline-none focus:ring-2 focus:ring-ring"
                >
                  {DEFAULT_TAG_COLORS.map((color) => (
                    <option key={color} value={color}>{color}</option>
                  ))}
                </select>
              </div>
              <button
                type="submit"
                disabled={isCreating || !newTagName.trim()}
                className="inline-flex items-center gap-1.5 px-3 py-2 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isCreating ? <Loader2 className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
                创建并绑定
              </button>
            </div>
          </form>

          {error && (
            <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}

          <div className="space-y-2">
            <p className="text-sm font-medium text-foreground">已有标签</p>
            {allTagsQuery.isLoading || nodeTagsQuery.isLoading ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="w-4 h-4 animate-spin" />
                加载标签中...
              </div>
            ) : allTags.length === 0 ? (
              <p className="text-sm text-muted-foreground">暂无标签，请先创建。</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {allTags.map((tag) => {
                  const attached = isTagAttached(tag, attachedTags)
                  const isPending = pendingTagId === tag.id
                  return (
                    <button
                      key={tag.id}
                      type="button"
                      onClick={() => void handleToggleTag(tag)}
                      disabled={isPending}
                      className={cn(
                        'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors disabled:opacity-50',
                        attached
                          ? 'border-primary/30 bg-primary/10 text-primary'
                          : 'border-border bg-background text-muted-foreground hover:border-primary/30 hover:text-primary'
                      )}
                      title={attached ? '点击解绑标签' : '点击绑定标签'}
                    >
                      {isPending ? (
                        <Loader2 className="w-3.5 h-3.5 animate-spin" />
                      ) : (
                        <span
                          className="h-2.5 w-2.5 rounded-full"
                          style={{ backgroundColor: tag.color || '#64748b' }}
                        />
                      )}
                      {tag.name}
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
