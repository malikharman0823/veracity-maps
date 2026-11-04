import React from "react"
import { Navigate, useLocation } from "react-router-dom"
import { useAuth } from "../context/AuthContext"

export default function ProtectedRoute({ children }) {
  const { isAuthenticated, loading } = useAuth()
  const location = useLocation()

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[#f5f5f7] text-[#7a7a7a] text-sm">
        <div className="flex flex-col items-center gap-2">
          <div className="size-6 border-2 border-[#0066cc] border-t-transparent rounded-full animate-spin"></div>
          <span>Verifying credentials...</span>
        </div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return children
}
