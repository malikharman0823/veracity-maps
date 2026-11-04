import React, { createContext, useContext, useState, useEffect } from "react"
import { getAuthToken, getCurrentUser, loginUser, registerUser, logoutUser, clearAuthToken } from "../api"

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [token, setToken] = useState(getAuthToken())
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let isMounted = true
    async function loadUser() {
      const activeToken = getAuthToken()
      if (!activeToken) {
        if (isMounted) setLoading(false)
        return
      }

      try {
        const profile = await getCurrentUser()
        if (isMounted) {
          setUser(profile)
          setToken(activeToken)
        }
      } catch (err) {
        if (isMounted) {
          clearAuthToken()
          setUser(null)
          setToken(null)
        }
      } finally {
        if (isMounted) setLoading(false)
      }
    }

    loadUser()
    return () => {
      isMounted = false
    }
  }, [])

  const login = async (email, password) => {
    const data = await loginUser(email, password)
    setUser(data.user)
    setToken(data.token)
    return data
  }

  const register = async (name, email, password) => {
    const data = await registerUser(name, email, password)
    setUser(data.user)
    setToken(data.token)
    return data
  }

  const logout = async () => {
    try {
      await logoutUser()
    } finally {
      setUser(null)
      setToken(null)
    }
  }

  const value = {
    user,
    token,
    isAuthenticated: !!user && !!token,
    loading,
    login,
    register,
    logout,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error("useAuth must be used within an AuthProvider")
  }
  return ctx
}
