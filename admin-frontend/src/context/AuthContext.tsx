import React, { createContext, useContext, useState, useCallback } from 'react'

interface AuthContextValue {
  token: string | null
  username: string | null
  login: (token: string, username: string) => void
  logout: () => void
}

const AuthContext = createContext<AuthContextValue>({
  token: null,
  username: null,
  login: () => {},
  logout: () => {},
})

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(() =>
    localStorage.getItem('admin_token')
  )
  const [username, setUsername] = useState<string | null>(() =>
    localStorage.getItem('admin_username')
  )

  const login = useCallback((newToken: string, newUsername: string) => {
    localStorage.setItem('admin_token', newToken)
    localStorage.setItem('admin_username', newUsername)
    setToken(newToken)
    setUsername(newUsername)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem('admin_token')
    localStorage.removeItem('admin_username')
    setToken(null)
    setUsername(null)
  }, [])

  return (
    <AuthContext.Provider value={{ token, username, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  return useContext(AuthContext)
}
