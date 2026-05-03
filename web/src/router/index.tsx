import { createBrowserRouter, Navigate, Outlet } from 'react-router-dom'
import { AppLayout } from '@/components/layout/AppLayout'
import { SetupPage } from '@/pages/setup/SetupPage'
import { LoginPage } from '@/pages/auth/LoginPage'
import { VFSFileManagerPage } from '@/pages/files/VFSFileManagerPage'
import { SourcesPage } from '@/pages/sources/SourcesPage'
import { TasksPage } from '@/pages/tasks/TasksPage'
import { TrashPage } from '@/pages/trash/TrashPage'
import { SharesPage } from '@/pages/shares/SharesPage'
import { SettingsPage } from '@/pages/settings/SettingsPage'
import { UsersPage } from '@/pages/users/UsersPage'
import { AclPage } from '@/pages/acl/AclPage'
import { AuditPage } from '@/pages/audit/AuditPage'
import { RssPage } from '@/pages/rss/RssPage'
import { ShareAccessPage } from '@/pages/shares/ShareAccessPage'
import { CapabilityAnyRoute, CapabilityRoute } from './CapabilityRoute'
import App from '@/App'

export const router = createBrowserRouter([
  {
    element: (
      <>
        <App />
        <Outlet />
      </>
    ),
    children: [
      {
        path: '/setup',
        element: <SetupPage />,
      },
      {
        path: '/login',
        element: <LoginPage />,
      },
      {
        path: '/s/:token',
        element: <ShareAccessPage />,
      },
      {
        path: '/',
        element: <AppLayout />,
        children: [
          { index: true, element: <Navigate to="/files" replace /> },
          { path: 'files', element: <VFSFileManagerPage /> },
          { path: 'files/*', element: <VFSFileManagerPage /> },
          { path: 'vfs', element: <Navigate to="/files" replace /> },
          { path: 'vfs/*', element: <Navigate to="/files" replace /> },
          { path: 'sources', element: <CapabilityRoute cap="source.read"><SourcesPage /></CapabilityRoute> },
          { path: 'tasks', element: <CapabilityRoute cap="task.read_all"><TasksPage /></CapabilityRoute> },
          { path: 'rss', element: <CapabilityRoute cap="rss.read"><RssPage /></CapabilityRoute> },
          { path: 'trash', element: <TrashPage /> },
          { path: 'shares', element: <CapabilityRoute cap="share.read_all"><SharesPage /></CapabilityRoute> },
          { path: 'settings', element: <CapabilityAnyRoute caps={['system.config.read', 'notification.read']}><SettingsPage /></CapabilityAnyRoute> },
          { path: 'users', element: <CapabilityRoute cap="user.read"><UsersPage /></CapabilityRoute> },
          { path: 'acl', element: <CapabilityRoute cap="acl.read"><AclPage /></CapabilityRoute> },
          { path: 'audit', element: <CapabilityRoute cap="audit.read"><AuditPage /></CapabilityRoute> },
        ],
      },
    ],
  },
])
