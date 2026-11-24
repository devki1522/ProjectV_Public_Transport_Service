import { AuthProvider } from './auth/AuthContext.jsx'
import { ProtectedAdminRoute } from './features/admin/ProtectedAdminRoute.jsx'
import { ResidentPage } from './features/resident/ResidentPage.jsx'
import { useRouter } from './lib/routerState.js'
import { RouterProvider } from './lib/router.jsx'
import { AdminBudgetPage } from './pages/AdminBudgetPage.jsx'
import { AdminDashboardPage } from './pages/AdminDashboardPage.jsx'
import { AdminDisruptionsPage } from './pages/AdminDisruptionsPage.jsx'
import { AdminRoutesPage } from './pages/AdminRoutesPage.jsx'
import { AdminStaffPage } from './pages/AdminStaffPage.jsx'
import { AdminVehiclesPage } from './pages/AdminVehiclesPage.jsx'
import { LoginPage } from './pages/LoginPage.jsx'
import { UnauthorizedPage } from './pages/UnauthorizedPage.jsx'
import './App.css'
import './resident.css'
import './transit-tokens.css'
import './features/live/primary.css'

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
    case '/admin/vehicles':
      return (
        <ProtectedAdminRoute>
          <AdminVehiclesPage />
        </ProtectedAdminRoute>
      )
    case '/admin/disruptions':
      return (
        <ProtectedAdminRoute>
          <AdminDisruptionsPage />
        </ProtectedAdminRoute>
      )
    case '/admin/staff':
      return (
        <ProtectedAdminRoute>
          <AdminStaffPage />
        </ProtectedAdminRoute>
      )
    case '/admin/budget':
      return (
        <ProtectedAdminRoute>
          <AdminBudgetPage />
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
