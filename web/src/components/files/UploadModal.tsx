import { useState, useRef, useCallback } from 'react'
import { X, Upload, File as FileIcon, Loader2, CheckCircle2, AlertCircle } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useUIStore } from '@/stores/uiStore'
import { useFileStore } from '@/stores/fileStore'
import { uploadApi } from '@/api/upload'
import { computeFileHash, formatBytes } from '@/utils'
import { cn } from '@/utils'

interface UploadFile {
  id: string
  file: File
  name: string
  size: number
  progress: number
  status: 'pending' | 'hashing' | 'uploading' | 'success' | 'error'
  error?: string
  speed: number
}

export function UploadModal() {
  const { isUploadModalOpen, setUploadModalOpen } = useUIStore()
  const { currentSource, currentPath } = useFileStore()
  const queryClient = useQueryClient()
  const [files, setFiles] = useState<UploadFile[]>([])
  const [isDragging, setIsDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const addFiles = useCallback((newFiles: FileList | null) => {
    if (!newFiles) return
    const items: UploadFile[] = Array.from(newFiles).map((file) => ({
      id: Math.random().toString(36).slice(2),
      file,
      name: file.name,
      size: file.size,
      progress: 0,
      status: 'pending',
      speed: 0,
    }))
    setFiles((prev) => [...prev, ...items])
    // Auto-start uploads
    items.forEach((item) => startUpload(item))
  }, [])

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      setIsDragging(false)
      addFiles(e.dataTransfer.files)
    },
    [addFiles]
  )

  const startUpload = async (item: UploadFile) => {
    if (!currentSource) return

    setFiles((prev) =>
      prev.map((f) => (f.id === item.id ? { ...f, status: 'hashing' as const } : f))
    )

    let hash: string
    try {
      hash = await computeFileHash(item.file)
    } catch {
      setFiles((prev) =>
        prev.map((f) => (f.id === item.id ? { ...f, status: 'error' as const, error: '计算文件哈希失败' } : f))
      )
      return
    }

    setFiles((prev) =>
      prev.map((f) => (f.id === item.id ? { ...f, status: 'uploading' as const } : f))
    )

    try {
      const initRes = await uploadApi.init({
        source_id: currentSource.id,
        path: currentPath,
        filename: item.name,
        file_size: item.size,
        file_hash: hash,
      })

      if (initRes.is_fast_upload && initRes.file) {
        setFiles((prev) =>
          prev.map((f) => (f.id === item.id ? { ...f, status: 'success' as const, progress: 100 } : f))
        )
        return
      }

      const upload = initRes.upload!
      const transport = initRes.transport!
      const chunkSize = upload.chunk_size
      const totalChunks = upload.total_chunks
      const concurrency = transport.concurrency

      const uploadedChunks = new Set(upload.uploaded_chunks)
      const chunkTasks: { index: number; start: number; end: number }[] = []

      for (let i = 0; i < totalChunks; i++) {
        if (!uploadedChunks.has(i)) {
          chunkTasks.push({
            index: i,
            start: i * chunkSize,
            end: Math.min((i + 1) * chunkSize, item.size),
          })
        }
      }

      let completedChunks = uploadedChunks.size

      const runChunk = async (task: (typeof chunkTasks)[0]) => {
        const chunk = item.file.slice(task.start, task.end)
        await uploadApi.uploadChunk(upload.upload_id, task.index, chunk)
        completedChunks++
        const progress = Math.round((completedChunks / totalChunks) * 100)
        setFiles((prev) =>
          prev.map((f) => (f.id === item.id ? { ...f, progress } : f))
        )
      }

      const pool = async () => {
        const executing: Promise<void>[] = []
        for (const task of chunkTasks) {
          const p = runChunk(task)
          executing.push(p)
          if (executing.length >= concurrency) {
            await Promise.race(executing)
            executing.splice(
              0,
              executing.length,
              ...executing.filter((e) => e !== p)
            )
          }
        }
        await Promise.all(executing)
      }

      await pool()

      await uploadApi.finish({
        upload_id: upload.upload_id,
      })

      setFiles((prev) =>
        prev.map((f) => (f.id === item.id ? { ...f, status: 'success' as const, progress: 100 } : f))
      )
      queryClient.invalidateQueries({ queryKey: ['files', currentSource?.id, currentPath] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '上传失败'
      setFiles((prev) =>
        prev.map((f) => (f.id === item.id ? { ...f, status: 'error' as const, error: msg } : f))
      )
    }
  }

  const startAll = () => {
    files.filter((f) => f.status === 'pending').forEach((f) => startUpload(f))
  }

  const removeFile = (id: string) => {
    setFiles((prev) => prev.filter((f) => f.id !== id))
  }

  const clearCompleted = () => {
    setFiles((prev) => prev.filter((f) => f.status !== 'success'))
  }

  if (!isUploadModalOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40" onClick={() => setUploadModalOpen(false)} />
      <div className="relative w-full max-w-lg bg-card border border-border rounded-lg shadow-xl flex flex-col max-h-[80vh]">
        <div className="flex items-center justify-between px-4 h-12 border-b border-border shrink-0">
          <h3 className="font-medium text-foreground">上传文件</h3>
          <button
            onClick={() => setUploadModalOpen(false)}
            className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-4 overflow-auto scrollbar-thin">
          <div
            onDragOver={(e) => { e.preventDefault(); setIsDragging(true) }}
            onDragLeave={() => setIsDragging(false)}
            onDrop={handleDrop}
            onClick={() => inputRef.current?.click()}
            className={cn(
              'border-2 border-dashed rounded-lg p-6 text-center cursor-pointer transition-colors mb-4',
              isDragging
                ? 'border-primary bg-primary/5'
                : 'border-border hover:border-primary/50'
            )}
          >
            <Upload className="w-8 h-8 mx-auto text-muted-foreground mb-2" />
            <p className="text-sm text-muted-foreground">
              拖拽文件到此处，或 <span className="text-primary">点击选择</span>
            </p>
            <input
              ref={inputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => addFiles(e.target.files)}
            />
          </div>

          {files.length > 0 && (
            <div className="space-y-2">
              {files.map((item) => (
                <div
                  key={item.id}
                  className="flex items-center gap-3 p-2.5 rounded-md border border-border bg-background"
                >
                  <FileIcon className="w-5 h-5 text-muted-foreground shrink-0" />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-foreground truncate">{item.name}</p>
                    <p className="text-xs text-muted-foreground">
                      {formatBytes(item.size)}
                      {item.status === 'uploading' && ` · ${item.progress}%`}
                    </p>
                    {item.status === 'uploading' && (
                      <div className="w-full h-1 bg-muted rounded-full mt-1 overflow-hidden">
                        <div
                          className="h-full bg-primary rounded-full transition-all"
                          style={{ width: `${item.progress}%` }}
                        />
                      </div>
                    )}
                    {item.status === 'error' && (
                      <p className="text-xs text-destructive mt-0.5">{item.error}</p>
                    )}
                  </div>
                  <div className="shrink-0 flex items-center gap-1">
                    {item.status === 'pending' && (
                      <button
                        onClick={() => startUpload(item)}
                        className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                      >
                        <Upload className="w-4 h-4" />
                      </button>
                    )}
                    {item.status === 'hashing' && (
                      <Loader2 className="w-4 h-4 text-primary animate-spin" />
                    )}
                    {item.status === 'uploading' && (
                      <Loader2 className="w-4 h-4 text-primary animate-spin" />
                    )}
                    {item.status === 'success' && (
                      <CheckCircle2 className="w-4 h-4 text-emerald-500" />
                    )}
                    {item.status === 'error' && (
                      <AlertCircle className="w-4 h-4 text-destructive" />
                    )}
                    <button
                      onClick={() => removeFile(item.id)}
                      className="p-1.5 rounded-md hover:bg-accent text-muted-foreground"
                    >
                      <X className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="flex items-center justify-between px-4 h-14 border-t border-border shrink-0">
          <span className="text-xs text-muted-foreground">
            {files.filter((f) => f.status === 'success').length} / {files.length} 完成
          </span>
          <div className="flex items-center gap-2">
            {files.some((f) => f.status === 'success') && (
              <button
                onClick={clearCompleted}
                className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:bg-accent transition-colors"
              >
                清除已完成
              </button>
            )}
            <button
              onClick={startAll}
              disabled={!files.some((f) => f.status === 'pending')}
              className="px-4 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              开始上传
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
