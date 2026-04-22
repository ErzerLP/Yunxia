import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/stores/authStore'
import { FileToolbar } from '@/components/files/FileToolbar'
import { FileBreadcrumb } from '@/components/files/FileBreadcrumb'
import { FileList } from '@/components/files/FileList'
import { FileGrid } from '@/components/files/FileGrid'
import { useFileStore } from '@/stores/fileStore'

export function FileManagerPage() {
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
      <FileToolbar />
      <FileBreadcrumb />
      {viewMode === 'list' ? <FileList /> : <FileGrid />}
    </div>
  )
}
