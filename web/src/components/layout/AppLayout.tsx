import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { PreviewDrawer } from './PreviewDrawer'

export function AppLayout() {
  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background">
      <Sidebar />
      <main className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <Outlet />
      </main>
      <PreviewDrawer />
    </div>
  )
}
