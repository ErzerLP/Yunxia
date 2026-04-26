import { create } from 'zustand'
import type { ToastItem, ToastType } from '@/components/ui/Toast'

interface SidebarState {
  isCollapsed: boolean
  activeItem: string
}

interface PreviewState {
  isOpen: boolean
  mode: 'v1' | 'v2'
  filePath: string | null
  sourceId: number | null
  fileName: string | null
  mimeType: string | null
}

interface UIState {
  sidebar: SidebarState
  preview: PreviewState
  theme: 'light' | 'dark' | 'system'
  isUploadModalOpen: boolean
  globalLoading: boolean
  toasts: ToastItem[]
  toggleSidebar: () => void
  setSidebarActive: (item: string) => void
  openPreview: (file: { path: string; source_id?: number | null; name: string; mime_type: string; mode?: 'v1' | 'v2' }) => void
  closePreview: () => void
  setTheme: (theme: 'light' | 'dark' | 'system') => void
  setUploadModalOpen: (open: boolean) => void
  setGlobalLoading: (loading: boolean) => void
  addToast: (message: string, type: ToastType, duration?: number) => void
  removeToast: (id: string) => void
}

function getInitialTheme(): 'light' | 'dark' | 'system' {
  const stored = localStorage.getItem('theme') as 'light' | 'dark' | 'system' | null
  return stored || 'system'
}

export const useUIStore = create<UIState>((set) => ({
  sidebar: {
    isCollapsed: false,
    activeItem: 'files',
  },
  preview: {
    isOpen: false,
    mode: 'v1',
    filePath: null,
    sourceId: null,
    fileName: null,
    mimeType: null,
  },
  theme: getInitialTheme(),
  isUploadModalOpen: false,
  globalLoading: false,
  toasts: [],
  toggleSidebar: () =>
    set((state) => ({
      sidebar: { ...state.sidebar, isCollapsed: !state.sidebar.isCollapsed },
    })),
  setSidebarActive: (item) =>
    set((state) => ({
      sidebar: { ...state.sidebar, activeItem: item },
    })),
  openPreview: (file) =>
    set({
      preview: {
        isOpen: true,
        mode: file.mode || 'v1',
        filePath: file.path,
        sourceId: file.source_id ?? null,
        fileName: file.name,
        mimeType: file.mime_type,
      },
    }),
  closePreview: () =>
    set({
      preview: {
        isOpen: false,
        mode: 'v1',
        filePath: null,
        sourceId: null,
        fileName: null,
        mimeType: null,
      },
    }),
  setTheme: (theme) => {
    localStorage.setItem('theme', theme)
    set({ theme })
    const root = document.documentElement
    if (theme === 'dark') {
      root.classList.add('dark')
    } else if (theme === 'light') {
      root.classList.remove('dark')
    } else {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
      if (prefersDark) {
        root.classList.add('dark')
      } else {
        root.classList.remove('dark')
      }
    }
  },
  setUploadModalOpen: (open) => set({ isUploadModalOpen: open }),
  setGlobalLoading: (loading) => set({ globalLoading: loading }),
  addToast: (message, type, duration) =>
    set((state) => ({
      toasts: [...state.toasts, { id: Math.random().toString(36).slice(2), message, type, duration }],
    })),
  removeToast: (id) =>
    set((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id),
    })),
}))
