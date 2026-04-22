import { useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { setupApi } from '@/api/setup'
import { authApi } from '@/api/auth'
import { UploadModal } from '@/components/files/UploadModal'

export default function App() {
  const navigate = useNavigate()
  const location = useLocation()
  const { setUser, setTokens, logout, setLoading } = useAuthStore()

  useEffect(() => {
    const init = async () => {
      try {
        const status = await setupApi.getStatus()
        if (status.setup_required) {
          if (location.pathname !== '/setup') {
            navigate('/setup', { replace: true })
          }
          setLoading(false)
          return
        }

        const token = localStorage.getItem('access_token')
        if (!token) {
          if (!['/login', '/setup'].includes(location.pathname)) {
            navigate('/login', { replace: true })
          }
          setLoading(false)
          return
        }

        // Token exists, validate by calling /auth/me
        try {
          const user = await authApi.me()
          setUser(user)
          setLoading(false)
        } catch {
          // Token invalid/expired and refresh failed (interceptor redirects)
          logout()
          if (location.pathname !== '/login') {
            navigate('/login', { replace: true })
          }
          setLoading(false)
        }
      } catch {
        setLoading(false)
      }
    }

    init()
  }, [navigate, location.pathname, setLoading, setUser, setTokens, logout])

  return <UploadModal />
}
