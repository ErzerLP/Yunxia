import { create } from 'zustand'
import type { FileItem, FileListResult, StorageSource } from '@/types/api'

interface FileState {
  currentSource: StorageSource | null
  currentPath: string
  files: FileItem[]
  selectedFiles: Set<string>
  isLoading: boolean
  viewMode: 'list' | 'grid'
  sortBy: 'name' | 'size' | 'modified_at'
  sortOrder: 'asc' | 'desc'
  setCurrentSource: (source: StorageSource | null) => void
  setCurrentPath: (path: string) => void
  setFiles: (files: FileItem[]) => void
  setFileListResult: (result: FileListResult) => void
  toggleSelection: (path: string) => void
  selectAll: (paths: string[]) => void
  clearSelection: () => void
  setLoading: (loading: boolean) => void
  setViewMode: (mode: 'list' | 'grid') => void
  setSort: (by: 'name' | 'size' | 'modified_at', order: 'asc' | 'desc') => void
  navigateTo: (path: string) => void
  navigateUp: () => void
}

export const useFileStore = create<FileState>((set, get) => ({
  currentSource: null,
  currentPath: '/',
  files: [],
  selectedFiles: new Set(),
  isLoading: false,
  viewMode: 'list',
  sortBy: 'name',
  sortOrder: 'asc',
  setCurrentSource: (source) => set({ currentSource: source, currentPath: '/', files: [], selectedFiles: new Set() }),
  setCurrentPath: (path) => set({ currentPath: path, selectedFiles: new Set() }),
  setFiles: (files) => set({ files }),
  setFileListResult: (result) => set({ currentPath: result.current_path, files: result.items }),
  toggleSelection: (path) =>
    set((state) => {
      const next = new Set(state.selectedFiles)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
      }
      return { selectedFiles: next }
    }),
  selectAll: (paths) => set({ selectedFiles: new Set(paths) }),
  clearSelection: () => set({ selectedFiles: new Set() }),
  setLoading: (loading) => set({ isLoading: loading }),
  setViewMode: (mode) => set({ viewMode: mode }),
  setSort: (by, order) => set({ sortBy: by, sortOrder: order }),
  navigateTo: (path) => set({ currentPath: path, selectedFiles: new Set() }),
  navigateUp: () => {
    const { currentPath } = get()
    if (currentPath === '/') return
    const parts = currentPath.split('/').filter(Boolean)
    parts.pop()
    const parent = parts.length === 0 ? '/' : '/' + parts.join('/') + '/'
    set({ currentPath: parent, selectedFiles: new Set() })
  },
}))
