import { AuthProvider } from './auth/AuthContext.jsx'
import { ProtectedAdminRoute } from './features/admin/ProtectedAdminRoute.jsx'
import { ResidentPage } from './features/resident/ResidentPage.jsx'
import { useRouter } from './lib/routerState.js'
import { RouterProvider } from './lib/router.jsx'
import { AdminDashboardPage } from './pages/AdminDashboardPage.jsx'
import { AdminRoutesPage } from './pages/AdminRoutesPage.jsx'
import { LoginPage } from './pages/LoginPage.jsx'
import { UnauthorizedPage } from './pages/UnauthorizedPage.jsx'
import './App.css'

function AppRoutes() {
  const { pathname } = useRouter()

  switch (pathname) {
    case '/login':
      return <LoginPage />
    case '/unauthorized':
      return <UnauthorizedPage />
    case '/admin':
      return (
        <ProtectedAdminRoute>
          <AdminDashboardPage />
        </ProtectedAdminRoute>
      )
    case '/admin/transit':
      return (
        <ProtectedAdminRoute>
          <AdminRoutesPage />
        </ProtectedAdminRoute>
      )
    default:
      return <ResidentPage />
  }
}

export function App({ initialPath } = {}) {
  return (
    <RouterProvider initialPath={initialPath}>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </RouterProvider>
  )
}

export default App
