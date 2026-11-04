import React, { useState, useEffect } from "react"
import { NavLink, useNavigate } from "react-router-dom"
import {
  Sidebar as ShadcnSidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  useSidebar,
} from "@/components/ui/sidebar"
import {
  Layers,
  UploadCloud,
  MapPin,
  Compass,
  FolderKanban,
  LogOut,
  User,
  Building2,
  Plus,
} from "lucide-react"
import EndSessionDialog from "./EndSessionDialog"
import { useAuth } from "../context/AuthContext"
import { listOrganizations, createOrganization } from "../api"

export default function Sidebar() {
  const { setOpenMobile } = useSidebar()
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const [organizations, setOrganizations] = useState([])
  const [showCreateOrgModal, setShowCreateOrgModal] = useState(false)
  const [newOrgName, setNewOrgName] = useState("")
  const [newOrgDesc, setNewOrgDesc] = useState("")
  const [creatingOrg, setCreatingOrg] = useState(false)

  const fetchOrgs = async () => {
    if (!user) return
    try {
      const orgs = await listOrganizations()
      setOrganizations(orgs)
    } catch {
      // ignore
    }
  }

  useEffect(() => {
    fetchOrgs()
  }, [user])

  const handleCreateOrg = async (e) => {
    e.preventDefault()
    if (!newOrgName.trim()) return

    setCreatingOrg(true)
    try {
      const created = await createOrganization(newOrgName.trim(), newOrgDesc.trim())
      setShowCreateOrgModal(false)
      setNewOrgName("")
      setNewOrgDesc("")
      await fetchOrgs()
      navigate(`/organizations/${encodeURIComponent(created.id)}`)
    } catch (err) {
      alert("Failed to create organization: " + (err.response?.data?.error?.message || err.message))
    } finally {
      setCreatingOrg(false)
    }
  }

  const handleNavClick = () => {
    // Automatically close mobile drawer when link is clicked
    if (setOpenMobile) {
      setOpenMobile(false)
    }
  }

  return (
    <>
      <ShadcnSidebar
        className="border-r border-slate-800 bg-slate-950 text-slate-100"
      >
        {/* Brand Header */}
        <SidebarHeader className="h-16 border-b border-slate-800 bg-slate-900/50 backdrop-blur-md px-4 flex flex-row items-center justify-between">
          <NavLink
            to="/"
            onClick={handleNavClick}
            className="flex items-center gap-2.5 group"
          >
            <div className="h-8 w-8 rounded-lg bg-blue-600/20 border border-blue-500/30 text-blue-400 flex items-center justify-center font-bold shadow-xs transition-transform group-hover:scale-105">
              <Compass className="size-4" />
            </div>
            <div className="flex flex-col">
              <span className="font-semibold text-sm tracking-tight text-white">
                Veracity Maps
              </span>
            </div>
          </NavLink>
          <span className="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-400 border border-slate-700">
            1.3
          </span>
        </SidebarHeader>

        {/* Main Navigation */}
        <SidebarContent className="px-2 py-3 bg-slate-950">
          <SidebarGroup>
            <SidebarGroupLabel className="text-[11px] font-semibold uppercase tracking-wider text-slate-400 px-2 mb-1">
              Personal Workspace
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu className="space-y-1">
                <SidebarMenuItem>
                  <NavLink to="/dashboard" onClick={handleNavClick}>
                    {({ isActive }) => (
                      <SidebarMenuButton
                        isActive={isActive}
                        className={`w-full justify-start gap-2.5 rounded-xl px-3 py-2 text-xs font-medium transition-all ${
                          isActive
                            ? "bg-blue-600 text-white font-medium hover:bg-blue-500 hover:text-white"
                            : "text-slate-300 hover:bg-slate-900 hover:text-white"
                        }`}
                      >
                        <FolderKanban className={`size-4 ${isActive ? "text-white" : "text-slate-400"}`} />
                        <span>My Projects</span>
                      </SidebarMenuButton>
                    )}
                  </NavLink>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <NavLink to="/" end onClick={handleNavClick}>
                    {({ isActive }) => (
                      <SidebarMenuButton
                        isActive={isActive}
                        className={`w-full justify-start gap-2.5 rounded-xl px-3 py-2 text-xs font-medium transition-all ${
                          isActive
                            ? "bg-blue-600 text-white font-medium hover:bg-blue-500 hover:text-white"
                            : "text-slate-300 hover:bg-slate-900 hover:text-white"
                        }`}
                      >
                        <UploadCloud className={`size-4 ${isActive ? "text-white" : "text-slate-400"}`} />
                        <span>Upload Dataset</span>
                      </SidebarMenuButton>
                    )}
                  </NavLink>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>

          {/* Organizations Group */}
          <SidebarGroup className="mt-4">
            <div className="flex items-center justify-between px-2 mb-1">
              <SidebarGroupLabel className="text-[11px] font-semibold uppercase tracking-wider text-slate-400 p-0">
                Organizations
              </SidebarGroupLabel>
              <button
                type="button"
                onClick={() => setShowCreateOrgModal(true)}
                className="text-blue-400 hover:text-blue-300 p-0.5 rounded cursor-pointer"
                title="Create Organization"
              >
                <Plus className="size-3.5" />
              </button>
            </div>
            <SidebarGroupContent>
              <SidebarMenu className="space-y-1">
                {organizations.map((org) => (
                  <SidebarMenuItem key={org.id}>
                    <NavLink to={`/organizations/${org.id}`} onClick={handleNavClick}>
                      {({ isActive }) => (
                        <SidebarMenuButton
                          isActive={isActive}
                          className={`w-full justify-start gap-2.5 rounded-xl px-3 py-2 text-xs font-medium transition-all ${
                            isActive
                              ? "bg-blue-600 text-white font-medium hover:bg-blue-500 hover:text-white"
                              : "text-slate-300 hover:bg-slate-900 hover:text-white"
                          }`}
                        >
                          <Building2 className={`size-4 ${isActive ? "text-white" : "text-slate-400"}`} />
                          <span className="truncate">{org.name}</span>
                        </SidebarMenuButton>
                      )}
                    </NavLink>
                  </SidebarMenuItem>
                ))}
                {organizations.length === 0 && (
                  <div className="px-3 py-2 text-[11px] text-slate-500 italic">
                    No organizations yet
                  </div>
                )}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        {/* Footer Info */}
        <SidebarFooter className="border-t border-slate-800 p-3 text-[11px] text-slate-400 bg-slate-900/60 space-y-2">
          {user && (
            <div className="pb-2 border-b border-slate-800 flex items-center justify-between">
              <div className="flex items-center gap-2 overflow-hidden">
                <div className="size-6 rounded-full bg-blue-600/30 text-blue-300 font-semibold flex items-center justify-center text-[10px] border border-blue-500/20 shrink-0">
                  {user.name?.charAt(0) || "U"}
                </div>
                <div className="truncate">
                  <div className="text-[11px] font-medium text-white truncate">{user.name}</div>
                  <div className="text-[10px] text-slate-400 truncate">{user.email}</div>
                </div>
              </div>
              <button
                type="button"
                onClick={logout}
                className="p-1 rounded hover:bg-slate-800 text-slate-400 hover:text-red-400 transition-colors cursor-pointer"
                title="Sign Out"
              >
                <LogOut className="size-3.5" />
              </button>
            </div>
          )}
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-emerald-500 inline-block animate-pulse"></span>
              Spatial Server
            </span>
            <span className="font-mono text-[10px] text-slate-400">Online</span>
          </div>
          <div className="pt-1.5 border-t border-slate-800 flex items-center justify-between">
            <span className="text-[10px] text-slate-400">Workspace Session</span>
            <EndSessionDialog
              trigger={
                <button
                  type="button"
                  className="text-[10px] font-medium text-slate-400 hover:text-red-400 transition-colors cursor-pointer"
                  title="End session and clear workspace"
                >
                  Clear
                </button>
              }
            />
          </div>
        </SidebarFooter>
      </ShadcnSidebar>

      {/* CREATE ORGANIZATION MODAL */}
      {showCreateOrgModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl bg-slate-900 border border-slate-800 p-6 shadow-2xl">
            <h3 className="text-base font-bold text-white mb-1">Create New Organization</h3>
            <p className="text-xs text-slate-400 mb-4">
              Set up an isolated team workspace to collaborate with multiple members and manage organizational projects.
            </p>
            <form onSubmit={handleCreateOrg} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Organization Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Acme GeoSolutions"
                  value={newOrgName}
                  onChange={(e) => setNewOrgName(e.target.value)}
                  className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">Description (Optional)</label>
                <textarea
                  rows={3}
                  placeholder="e.g. Primary mapping and surveying workspace"
                  value={newOrgDesc}
                  onChange={(e) => setNewOrgDesc(e.target.value)}
                  className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2 text-xs text-white focus:outline-none focus:border-blue-500 resize-none"
                />
              </div>
              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateOrgModal(false)}
                  className="rounded-lg border border-slate-700 px-4 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingOrg}
                  className="rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 transition-colors cursor-pointer"
                >
                  {creatingOrg ? "Creating..." : "Create Organization"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  )
}
