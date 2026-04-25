import { create } from 'zustand'
import type { FileItem, FileListResult, StorageSource, VFSItem, VFSListResult } from '@/types/api'

export type FileMode = 'v1' | 'v2'

interface FileState {
  mode: FileMode
  currentSource: StorageSource | null
  currentPath: string
  currentVirtualPath: string
  files: FileItem[]
  vfsItems: VFSItem[]
  selectedFiles: Set<string>
  isLoading: boolean
  viewMode: 'list' | 'grid'
  sortBy: 'name' | 'size' | 'modified_at'
  sortOrder: 'asc' | 'desc'
  setMode: (mode: FileMode) => void
  setCurrentSource: (source: StorageSource | null) => void
  setCurrentPath: (path: string) => void
  setCurrentVirtualPath: (path: string) => void
  setFiles: (files: FileItem[]) => void
  setVfsItems: (items: VFSItem[]) => void
  setFileListResult: (result: FileListResult) => void
  setVfsListResult: (result: VFSListResult) => void
  toggleSelection: (path: string) => void
  selectAll: (paths: string[]) => void
  clearSelection: () => void
  setLoading: (loading: boolean) => void
  setViewMode: (mode: 'list' | 'grid') => void
  setSort: (by: 'name' | 'size' | 'modified_at', order: 'asc' | 'desc') => void
  navigateTo: (path: string) => void
  navigateUp: () => void
  navigateVirtualTo: (path: string) => void
  navigateVirtualUp: () => void
}

export const useFileStore = create<FileState>((set, get) => ({
  mode: 'v1',
  currentSource: null,
  currentPath: '/',
  currentVirtualPath: '/',
  files: [],
  vfsItems: [],
  selectedFiles: new Set(),
  isLoading: false,
  viewMode: 'list',
  sortBy: 'name',
  sortOrder: 'asc',
  setMode: (mode) => set({ mode, files: [], vfsItems: [], selectedFiles: new Set() }),
  setCurrentSource: (source) => set({ currentSource: source, currentPath: '/', files: [], selectedFiles: new Set() }),
  setCurrentPath: (path) => set({ currentPath: path, selectedFiles: new Set() }),
  setCurrentVirtualPath: (path) => set({ currentVirtualPath: path, selectedFiles: new Set() }),
  setFiles: (files) => set({ files }),
  setVfsItems: (items) => set({ vfsItems: items }),
  setFileListResult: (result) => set({ currentPath: result.current_path, files: result.items }),
  setVfsListResult: (result) => set({ currentVirtualPath: result.current_path, vfsItems: result.items }),
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
  navigateVirtualTo: (path) => set({ currentVirtualPath: path, selectedFiles: new Set() }),
  navigateVirtualUp: () => {
    const { currentVirtualPath } = get()
    if (currentVirtualPath === '/') return
    const parts = currentVirtualPath.split('/').filter(Boolean)
    parts.pop()
    const parent = parts.length === 0 ? '/' : '/' + parts.join('/') + '/'
    set({ currentVirtualPath: parent, selectedFiles: new Set() })
  },
}))
