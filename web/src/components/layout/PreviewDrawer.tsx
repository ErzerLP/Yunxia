import { X, Music, File } from 'lucide-react'
import { useUIStore } from '@/stores/uiStore'
import { fileApi } from '@/api/file'
import { fileV2Api } from '@/api/fileV2'
import { useEffect, useState } from 'react'
import { cn } from '@/utils'

export function PreviewDrawer() {
  const { preview, closePreview } = useUIStore()
  const { isOpen, mode, filePath, sourceId, fileName, mimeType } = preview
  const [url, setUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!isOpen || !filePath || (mode === 'v1' && !sourceId)) {
      setUrl(null)
      return
    }

    let revoked = false
    setLoading(true)
    setUrl(null)

    const request =
      mode === 'v2'
        ? fileV2Api.accessUrl({
            path: filePath,
            purpose: 'preview',
            disposition: 'inline',
          })
        : fileApi.getAccessUrl({
            source_id: sourceId!,
            path: filePath,
            purpose: 'preview',
            disposition: 'inline',
          })

    request
      .then((res) => {
        if (!revoked) {
          setUrl(res.url)
        }
      })
      .catch(() => {
        if (!revoked) {
          setUrl(null)
        }
      })
      .finally(() => setLoading(false))

    return () => {
      revoked = true
      if (url) URL.revokeObjectURL(url)
    }
  }, [isOpen, mode, filePath, sourceId])

  if (!isOpen) return null

  const isImage = mimeType?.startsWith('image/')
  const isVideo = mimeType?.startsWith('video/')
  const isAudio = mimeType?.startsWith('audio/')
  const isText = mimeType?.startsWith('text/') || mimeType?.includes('json') || mimeType?.includes('javascript')

  return (
    <>
      <div
        className="fixed inset-0 bg-black/20 z-40"
        onClick={closePreview}
      />
      <div
        className={cn(
          'fixed right-0 top-0 h-full w-[360px] bg-card border-l border-border z-50 shadow-xl flex flex-col animate-slide-in-right'
        )}
      >
        <div className="flex items-center justify-between px-4 h-14 border-b border-border shrink-0">
          <h3 className="font-medium text-card-foreground truncate pr-2">{fileName}</h3>
          <button
            onClick={closePreview}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground hover:text-accent-foreground transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="flex-1 overflow-auto p-4 flex items-center justify-center">
          {loading && (
            <div className="flex flex-col items-center gap-3 text-muted-foreground">
              <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
              <span className="text-sm">加载中...</span>
            </div>
          )}

          {!loading && isImage && url && (
            <img
              src={url}
              alt={fileName || ''}
              className="max-w-full max-h-full object-contain rounded-md"
            />
          )}

          {!loading && isVideo && url && (
            <video
              src={url}
              controls
              className="max-w-full max-h-full rounded-md"
            />
          )}

          {!loading && isAudio && url && (
            <div className="w-full flex flex-col items-center gap-4">
              <Music className="w-16 h-16 text-primary/60" />
              <audio src={url} controls className="w-full" />
            </div>
          )}

          {!loading && isText && url && (
            <iframe
              src={url}
              className="w-full h-full rounded-md border border-border bg-background"
              title={fileName || 'preview'}
            />
          )}

          {!loading && !isImage && !isVideo && !isAudio && !isText && (
            <div className="flex flex-col items-center gap-3 text-muted-foreground">
              <File className="w-16 h-16 text-muted-foreground/40" />
              <span className="text-sm">该文件类型暂不支持预览</span>
            </div>
          )}
        </div>
      </div>
    </>
  )
}
