import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { useUIStore } from '@/stores/uiStore'

export function useHasCapability(cap: string): boolean {
  const { hasCapability } = useAuthStore()
  return hasCapability(cap)
}

export function useHasAnyCapability(caps: string[]): boolean {
  const { hasCapability } = useAuthStore()
  return caps.some((c) => hasCapability(c))
}

export function useHasAllCapabilities(caps: string[]): boolean {
  const { hasCapability } = useAuthStore()
  return caps.every((c) => hasCapability(c))
}

interface UseCapabilityGuardOptions {
  redirectTo?: string
  showToast?: boolean
  toastMessage?: string
}

export function useCapabilityGuard(
  cap: string,
  options: UseCapabilityGuardOptions = {}
) {
  const { isLoading, isAuthenticated, hasCapability } = useAuthStore()
  const navigate = useNavigate()
  const { addToast } = useUIStore()

  const {
    redirectTo = '/files',
    showToast = true,
    toastMessage = '无权限访问该页面',
  } = options

  const allowed = !isLoading && isAuthenticated && hasCapability(cap)

  useEffect(() => {
    if (isLoading || !isAuthenticated) return
    if (!hasCapability(cap)) {
      if (showToast) {
        addToast(toastMessage, 'error')
      }
      navigate(redirectTo, { replace: true })
    }
  }, [isLoading, isAuthenticated, cap, hasCapability, navigate, redirectTo, showToast, toastMessage, addToast])

  return { allowed, isLoading }
}
