import { Outlet } from 'react-router-dom'
import { useUIStore } from '@/stores/uiStore'
import { Sidebar } from './Sidebar'
import { PreviewDrawer } from './PreviewDrawer'
import { ToastContainer } from '@/components/ui/Toast'

export function AppLayout() {
  const { toasts, removeToast } = useUIStore()

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background">
      <Sidebar />
      <main className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <Outlet />
      </main>
      <PreviewDrawer />
      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  )
}
