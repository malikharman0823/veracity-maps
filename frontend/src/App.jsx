import React, { Suspense, lazy, useEffect } from "react"
import { BrowserRouter, Navigate, Route, Routes, useParams } from "react-router-dom"

import DashboardLayout from "./components/DashboardLayout"
import ProtectedRoute from "./components/ProtectedRoute"
import { AuthProvider } from "./context/AuthContext"
import { ensureActiveSession } from "./api"
import { TooltipProvider } from "@/components/ui/tooltip"

const HomePage = lazy(() => import("./pages/HomePage"))
const UploadPage = lazy(() => import("./pages/UploadPage"))
const DatasetPage = lazy(() => import("./pages/DatasetPage"))
const GeoreferencePage = lazy(() => import("./pages/GeoreferencePage"))
const WorkspacePage = lazy(() => import("./pages/WorkspacePage"))
const LoginPage = lazy(() => import("./pages/LoginPage"))
const RegisterPage = lazy(() => import("./pages/RegisterPage"))
const DashboardPage = lazy(() => import("./pages/DashboardPage"))
const OrgDashboardPage = lazy(() => import("./pages/OrgDashboardPage"))

function DatasetWorkspaceRedirect() {
  const { id } = useParams()
  return <Navigate to={`/datasets/${id}/workspace`} replace />
}

export default function App() {
  useEffect(() => {
    ensureActiveSession().catch((err) => {
      console.warn("Session check failed:", err)
    })
  }, [])

  return (
    <TooltipProvider>
      <AuthProvider>
        <BrowserRouter>
          <Suspense
            fallback={
              <div className="flex min-h-screen items-center justify-center bg-[#f5f5f7] text-[#7a7a7a] text-sm">
                Loading…
              </div>
            }
          >
            <Routes>
              {/* Public Auth Routes */}
              <Route path="/login" element={<LoginPage />} />
              <Route path="/register" element={<RegisterPage />} />

              {/* Protected User Dashboard */}
              <Route
                path="/dashboard"
                element={
                  <ProtectedRoute>
                    <DashboardPage />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/organizations/:id"
                element={
                  <ProtectedRoute>
                    <OrgDashboardPage />
                  </ProtectedRoute>
                }
              />

              {/* Protected Workspace Layout Routes */}
              <Route
                element={
                  <ProtectedRoute>
                    <DashboardLayout />
                  </ProtectedRoute>
                }
              >
                <Route path="/" element={<HomePage />} />
                <Route path="/upload" element={<Navigate to="/" replace />} />
              </Route>

              {/* Protected Dataset and Georeferencing Routes */}
              <Route
                path="/datasets/:id"
                element={
                  <ProtectedRoute>
                    <DatasetWorkspaceRedirect />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/datasets/:id/legacy"
                element={
                  <ProtectedRoute>
                    <DatasetPage />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/datasets/:id/workspace"
                element={
                  <ProtectedRoute>
                    <WorkspacePage />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/datasets/:id/georeference"
                element={
                  <ProtectedRoute>
                    <GeoreferencePage />
                  </ProtectedRoute>
                }
              />

              {/* Fallback */}
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        </BrowserRouter>
      </AuthProvider>
    </TooltipProvider>
  )
}
