import { createContext, useContext } from 'react'

export const AuthContext = createContext(null)

export function canAdminister(session) {
  return session?.isAdmin === true
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used inside AuthProvider')
  }
  return ctx
}
