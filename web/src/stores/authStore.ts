import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { UserSummary, AuthTokenPair } from '@/types/api'

interface AuthState {
  user: UserSummary | null
  isAuthenticated: boolean
  isLoading: boolean
  setUser: (user: UserSummary | null) => void
  setTokens: (tokens: AuthTokenPair) => void
  logout: () => void
  setLoading: (loading: boolean) => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      isAuthenticated: false,
      isLoading: true,
      setUser: (user) => set({ user, isAuthenticated: !!user }),
      setTokens: (tokens) => {
        localStorage.setItem('access_token', tokens.access_token)
        localStorage.setItem('refresh_token', tokens.refresh_token)
      },
      logout: () => {
        localStorage.removeItem('access_token')
        localStorage.removeItem('refresh_token')
        set({ user: null, isAuthenticated: false })
      },
      setLoading: (loading) => set({ isLoading: loading }),
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({ user: state.user, isAuthenticated: state.isAuthenticated }),
    }
  )
)
