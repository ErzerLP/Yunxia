import { useEffect } from 'react'
import { X, CheckCircle2, AlertCircle, AlertTriangle, Info } from 'lucide-react'
import { cn } from '@/utils'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface ToastItem {
  id: string
  message: string
  type: ToastType
  duration?: number
}

interface ToastProps {
  toast: ToastItem
  onRemove: (id: string) => void
}

const config: Record<ToastType, { icon: typeof Info; className: string; bg: string }> = {
  success: { icon: CheckCircle2, className: 'text-emerald-500', bg: 'bg-emerald-500/10 border-emerald-500/20' },
  error: { icon: AlertCircle, className: 'text-destructive', bg: 'bg-destructive/10 border-destructive/20' },
  warning: { icon: AlertTriangle, className: 'text-warning', bg: 'bg-warning/10 border-warning/20' },
  info: { icon: Info, className: 'text-primary', bg: 'bg-primary/10 border-primary/20' },
}

export function Toast({ toast, onRemove }: ToastProps) {
  const { icon: Icon, className, bg } = config[toast.type]

  useEffect(() => {
    const timer = setTimeout(() => {
      onRemove(toast.id)
    }, toast.duration || 3000)
    return () => clearTimeout(timer)
  }, [toast.id, toast.duration, onRemove])

  return (
    <div
      className={cn(
        'flex items-center gap-2 px-3 py-2.5 rounded-lg border shadow-lg min-w-[200px] max-w-[400px] animate-slide-in-right',
        bg
      )}
    >
      <Icon className={cn('w-4 h-4 shrink-0', className)} />
      <span className="text-sm text-foreground flex-1">{toast.message}</span>
      <button
        onClick={() => onRemove(toast.id)}
        className="p-0.5 rounded hover:bg-black/5 text-muted-foreground"
      >
        <X className="w-3.5 h-3.5" />
      </button>
    </div>
  )
}

export function ToastContainer({
  toasts,
  onRemove,
}: {
  toasts: ToastItem[]
  onRemove: (id: string) => void
}) {
  if (toasts.length === 0) return null

  return (
    <div className="fixed top-4 right-4 z-[100] flex flex-col gap-2">
      {toasts.map((toast) => (
        <Toast key={toast.id} toast={toast} onRemove={onRemove} />
      ))}
    </div>
  )
}
