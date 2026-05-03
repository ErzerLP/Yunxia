import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'
import { useCapabilityGuard } from '@/hooks/useCapability'

export function CapabilityRoute({ cap, children }: { cap: string; children: React.ReactNode }) {
  const { allowed, isLoading } = useCapabilityGuard(cap)
  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }
  return allowed ? <>{children}</> : null
}

export function CapabilityAnyRoute({ caps, children }: { caps: string[]; children: React.ReactNode }) {
  const navigate = useNavigate()
  const { isLoading, isAuthenticated, hasCapability } = useAuthStore()
  const { addToast } = useUIStore()
  const allowed = !isLoading && isAuthenticated && caps.some((cap) => hasCapability(cap))

  useEffect(() => {
    if (isLoading || !isAuthenticated) return
    if (!caps.some((cap) => hasCapability(cap))) {
      addToast('无权限访问该页面', 'error')
      navigate('/files', { replace: true })
    }
  }, [addToast, caps, hasCapability, isAuthenticated, isLoading, navigate])

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }
  return allowed ? <>{children}</> : null
}
