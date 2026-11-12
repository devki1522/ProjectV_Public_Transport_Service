import { createContext, useContext } from 'react'

export const REQUIRED_ADMIN_ROLE = 'TransitAdministrator'

export const AuthContext = createContext(null)

export function hasAdminRole(user) {
  return Boolean(user && Array.isArray(user.roles) && user.roles.includes(REQUIRED_ADMIN_ROLE))
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used inside AuthProvider')
  }
  return ctx
}
