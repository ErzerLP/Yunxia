import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { VFSFileToolbar } from '@/components/files/VFSFileToolbar'
import { VFSFileBreadcrumb } from '@/components/files/VFSFileBreadcrumb'
import { VFSFileList } from '@/components/files/VFSFileList'
import { VFSFileGrid } from '@/components/files/VFSFileGrid'
import { useFileStore } from '@/stores/fileStore'

export function VFSFileManagerPage() {
  const navigate = useNavigate()
  const { isAuthenticated, isLoading } = useAuthStore()
  const { viewMode } = useFileStore()

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
