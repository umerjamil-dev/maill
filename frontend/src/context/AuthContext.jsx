import { createContext, useContext, useEffect, useState, useCallback } from 'react'
import { api, setToken } from '../lib/api'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)

  const loadMe = useCallback(async () => {
    if (!localStorage.getItem('token')) {
      setUser(null)
      setLoading(false)
      return
    }
    try {
      const me = await api.get('/me')
      setUser(me)
    } catch {
      setUser(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadMe()
    const onUnauth = () => setUser(null)
    window.addEventListener('unauthorized', onUnauth)
    return () => window.removeEventListener('unauthorized', onUnauth)
  }, [loadMe])

  async function login(email, password) {
    const res = await api.post('/auth/login', { email, password })
    setToken(res.token)
    await loadMe()
    return res
  }

  async function register(company, email, password, fullName) {
    await api.post('/auth/register', { company, email, password, full_name: fullName })
    return login(email, password)
  }

  function logout() {
    setToken(null)
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, loading, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
