import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { UserSummary, AuthTokenPair } from '@/types/api'

interface AuthState {
  user: UserSummary | null
  capabilities: string[]
  isAuthenticated: boolean
  isLoading: boolean
  setUser: (user: UserSummary | null) => void
  setCapabilities: (capabilities: string[]) => void
  setTokens: (tokens: AuthTokenPair) => void
  logout: () => void
  setLoading: (loading: boolean) => void
  hasCapability: (cap: string) => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      capabilities: [],
      isAuthenticated: false,
      isLoading: true,
      setUser: (user) => set({ user, isAuthenticated: !!user }),
      setCapabilities: (capabilities) => set({ capabilities }),
      setTokens: (tokens) => {
        localStorage.setItem('access_token', tokens.access_token)
        localStorage.setItem('refresh_token', tokens.refresh_token)
      },
      logout: () => {
        localStorage.removeItem('access_token')
        localStorage.removeItem('refresh_token')
        set({ user: null, capabilities: [], isAuthenticated: false })
      },
      setLoading: (loading) => set({ isLoading: loading }),
      hasCapability: (cap) => get().capabilities.includes(cap),
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({ user: state.user, capabilities: state.capabilities, isAuthenticated: state.isAuthenticated }),
    }
  )
)
