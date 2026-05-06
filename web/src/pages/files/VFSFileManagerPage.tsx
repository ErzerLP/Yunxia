import { useEffect, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { VFSFileToolbar } from '@/components/files/VFSFileToolbar'
import { VFSFileBreadcrumb } from '@/components/files/VFSFileBreadcrumb'
import { VFSFileList } from '@/components/files/VFSFileList'
import { VFSFileGrid } from '@/components/files/VFSFileGrid'
import { useFileStore } from '@/stores/fileStore'
import { normalizeVfsPath } from '@/utils/vfs'

export function VFSFileManagerPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { isAuthenticated, isLoading } = useAuthStore()
  const { viewMode, setMode, currentVirtualPath, setCurrentVirtualPath } = useFileStore()
  const pathParam = searchParams.get('path')
  const lastUrlPathRef = useRef<string | null>(null)
  const pendingUrlPathRef = useRef<string | null>(null)

  useEffect(() => {
    setMode('v2')
  }, [setMode])

  useEffect(() => {
    const pathFromUrl = normalizeVfsPath(pathParam)
    if (lastUrlPathRef.current === pathFromUrl) return

    lastUrlPathRef.current = pathFromUrl
    if (normalizeVfsPath(currentVirtualPath) !== pathFromUrl) {
      pendingUrlPathRef.current = pathFromUrl
      setCurrentVirtualPath(pathFromUrl)
    }
  }, [currentVirtualPath, pathParam, setCurrentVirtualPath])

  useEffect(() => {
    const normalizedCurrentPath = normalizeVfsPath(currentVirtualPath)
    const pathFromUrl = normalizeVfsPath(pathParam)

    if (pendingUrlPathRef.current) {
      if (normalizedCurrentPath === pendingUrlPathRef.current) {
        pendingUrlPathRef.current = null
      }
      return
    }

    if (normalizedCurrentPath === pathFromUrl) {
      lastUrlPathRef.current = pathFromUrl
      return
    }

    const nextSearchParams = new URLSearchParams(searchParams)
    if (normalizedCurrentPath === '/') {
      nextSearchParams.delete('path')
    } else {
      nextSearchParams.set('path', normalizedCurrentPath)
    }
    lastUrlPathRef.current = normalizedCurrentPath
    setSearchParams(nextSearchParams, { replace: true })
  }, [currentVirtualPath, pathParam, searchParams, setSearchParams])

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      navigate('/login', { replace: true })
    }
  }, [isAuthenticated, isLoading, navigate])

  if (isLoading) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <VFSFileToolbar />
      <VFSFileBreadcrumb />
      {viewMode === 'list' ? <VFSFileList /> : <VFSFileGrid />}
    </div>
  )
}
