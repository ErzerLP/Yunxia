import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'

export function useAuthGuard(requireAuth = true) {
  const { isAuthenticated, isLoading } = useAuthStore()
  const navigate = useNavigate()

  useEffect(() => {
    if (isLoading) return
    if (requireAuth && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
    if (!requireAuth && isAuthenticated) {
      navigate('/', { replace: true })
    }
  }, [isAuthenticated, isLoading, requireAuth, navigate])

  return { isAuthenticated, isLoading }
}

export function useSetupGuard() {
  const navigate = useNavigate()

  useEffect(() => {
    // Setup check will be handled by App initialization
  }, [navigate])

  return { navigate }
}
