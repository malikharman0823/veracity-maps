import React, { createContext, useContext, useState, useEffect } from "react"
import { getAuthToken, getCurrentUser, loginUser, registerUser, logoutUser, clearAuthToken, sendOtp, verifyOtp } from "../api"

const AuthContext = createContext(null)
const CACHED_USER_KEY = "veracity_maps_cached_user"

function getCachedUser() {
  try {
    const raw = localStorage.getItem(CACHED_USER_KEY) || sessionStorage.getItem(CACHED_USER_KEY)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
  }
}

function setCachedUser(u) {
  try {
    if (u) {
      const serialized = JSON.stringify(u)
      localStorage.setItem(CACHED_USER_KEY, serialized)
      sessionStorage.setItem(CACHED_USER_KEY, serialized)
    } else {
      localStorage.removeItem(CACHED_USER_KEY)
      sessionStorage.removeItem(CACHED_USER_KEY)
    }
  } catch {}
}

export function AuthProvider({ children }) {
  const initialToken = getAuthToken()
  const initialUser = initialToken ? getCachedUser() : null
  const [user, setUser] = useState(initialUser)
  const [token, setToken] = useState(initialToken)
  const [loading, setLoading] = useState(Boolean(initialToken && !initialUser))

  useEffect(() => {
    let isMounted = true
    async function loadUser() {
      const activeToken = getAuthToken()
      if (!activeToken) {
        setCachedUser(null)
        if (isMounted) {
          setUser(null)
          setToken(null)
          setLoading(false)
        }
        return
      }

      try {
        const profile = await getCurrentUser()
        if (isMounted) {
          setUser(profile)
          setCachedUser(profile)
          setToken(activeToken)
        }
      } catch (err) {
        if (isMounted) {
          clearAuthToken()
          setCachedUser(null)
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

  const sendOtpCode = async (email, purpose, name = "") => {
    return await sendOtp(email, purpose, name)
  }

  const loginWithOtp = async (email, code) => {
    const data = await verifyOtp(email, code, "login")
    setUser(data.user)
    setCachedUser(data.user)
    setToken(data.token)
    return data
  }

  const registerWithOtp = async (name, email, code) => {
    const data = await verifyOtp(email, code, "register", name)
    setUser(data.user)
    setCachedUser(data.user)
    setToken(data.token)
    return data
  }

  const login = async (email, password) => {
    const data = await loginUser(email, password)
    setUser(data.user)
    setCachedUser(data.user)
    setToken(data.token)
    return data
  }

  const register = async (name, email, password) => {
    const data = await registerUser(name, email, password)
    setUser(data.user)
    setCachedUser(data.user)
    setToken(data.token)
    return data
  }

  const logout = async () => {
    try {
      await logoutUser()
    } finally {
      setCachedUser(null)
      setUser(null)
      setToken(null)
    }
  }

  const value = {
    user,
    token,
    isAuthenticated: !!user && !!token,
    loading,
    sendOtp: sendOtpCode,
    loginWithOtp,
    registerWithOtp,
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
