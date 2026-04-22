import { createBrowserRouter, Navigate, Outlet } from 'react-router-dom'
import { AppLayout } from '@/components/layout/AppLayout'
import { SetupPage } from '@/pages/setup/SetupPage'
import { LoginPage } from '@/pages/auth/LoginPage'
import { FileManagerPage } from '@/pages/files/FileManagerPage'
import { SourcesPage } from '@/pages/sources/SourcesPage'
import { TasksPage } from '@/pages/tasks/TasksPage'
import { SettingsPage } from '@/pages/settings/SettingsPage'
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
        path: '/',
        element: <AppLayout />,
        children: [
          { index: true, element: <Navigate to="/files" replace /> },
          { path: 'files', element: <FileManagerPage /> },
          { path: 'files/*', element: <FileManagerPage /> },
          { path: 'sources', element: <SourcesPage /> },
          { path: 'tasks', element: <TasksPage /> },
          { path: 'settings', element: <SettingsPage /> },
        ],
      },
    ],
  },
])
