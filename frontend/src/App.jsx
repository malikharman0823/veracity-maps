import React, { Suspense, lazy } from "react"
import { BrowserRouter, Navigate, Route, Routes, useParams } from "react-router-dom"

import DashboardLayout from "./components/DashboardLayout"
import ProtectedRoute from "./components/ProtectedRoute"
import { AuthProvider } from "./context/AuthContext"
import { ThemeProvider } from "./context/ThemeContext"
import { TooltipProvider } from "@/components/ui/tooltip"

const HomePage = lazy(() => import("./pages/HomePage"))
const UploadPage = lazy(() => import("./pages/UploadPage"))
const DatasetPage = lazy(() => import("./pages/DatasetPage"))
const GeoreferencePage = lazy(() => import("./pages/GeoreferencePage"))
const WorkspacePage = lazy(() => import("./pages/WorkspacePage"))
const LoginPage = lazy(() => import("./pages/LoginPage"))
const RegisterPage = lazy(() => import("./pages/RegisterPage"))
const DashboardPage = lazy(() => import("./pages/DashboardPage"))
const ProfilePage = lazy(() => import("./pages/ProfilePage"))

function DatasetWorkspaceRedirect() {
  const { id } = useParams()
  return <Navigate to={`/datasets/${id}/workspace`} replace />
}

export default function App() {
  return (
    <ThemeProvider>
      <TooltipProvider>
        <AuthProvider>
          <BrowserRouter>
            <Suspense
              fallback={
                <div className="flex min-h-screen items-center justify-center bg-[#ffffff] dark:bg-[#1c1c1c] text-[#707070] dark:text-[#9a9a9a] text-sm">
                  Loading…
                </div>
              }
            >
              <Routes>
                {/* Public Auth Routes */}
                <Route path="/login" element={<LoginPage />} />
                <Route path="/register" element={<RegisterPage />} />

                {/* All Protected Routes Share the Global Navigation Sidebar */}
                <Route
                  element={
                    <ProtectedRoute>
                      <DashboardLayout />
                    </ProtectedRoute>
                  }
                >
                  <Route path="/dashboard" element={<DashboardPage />} />
                  <Route path="/profile" element={<ProfilePage />} />
                  <Route path="/" element={<HomePage />} />
                  <Route path="/upload" element={<Navigate to="/" replace />} />

                  {/* Canonical Shared Workspace Map Editor Routes */}
                  <Route path="/maps/:mapId/edit" element={<WorkspacePage />} />
                  <Route path="/maps/:mapId" element={<WorkspacePage />} />

                  {/* Project and Mapspace Independent Map Editor Routes */}
                  <Route path="/projects/:projectId/maps/:mapId/edit" element={<WorkspacePage />} />
                  <Route path="/projects/:projectId/maps/:mapId" element={<WorkspacePage />} />
                  <Route path="/mapspaces/:mapspaceId/maps/:mapId/edit" element={<WorkspacePage />} />
                  <Route path="/mapspaces/:mapspaceId/maps/:mapId" element={<WorkspacePage />} />
                  <Route path="/workspace" element={<WorkspacePage />} />

                  {/* Protected Dataset and Georeferencing Routes */}
                  <Route path="/datasets/:id" element={<DatasetWorkspaceRedirect />} />
                  <Route path="/datasets/:id/legacy" element={<DatasetPage />} />
                  <Route path="/datasets/:id/workspace" element={<WorkspacePage />} />
                  <Route path="/datasets/:id/georeference" element={<GeoreferencePage />} />
                </Route>

                {/* Fallback */}
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
          </Suspense>
        </BrowserRouter>
      </AuthProvider>
    </TooltipProvider>
  </ThemeProvider>
  )
}
