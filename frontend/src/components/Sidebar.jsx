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
        className="border-r border-[#e0e0e0] bg-[#ffffff] text-[#1d1d1f]"
      >
        {/* Brand Header */}
        <SidebarHeader className="h-14 border-b border-[#e0e0e0] px-4 flex flex-row items-center justify-between">
          <NavLink
            to="/"
            onClick={handleNavClick}
            className="flex items-center gap-2.5 group"
          >
            <div className="h-7 w-7 rounded-lg bg-[#0066cc] text-white flex items-center justify-center font-bold shadow-xs transition-transform group-hover:scale-105">
              <Compass className="size-4" />
            </div>
            <div className="flex flex-col">
              <span className="font-semibold text-sm tracking-tight text-[#1d1d1f]">
                Veracity Maps
              </span>
            </div>
          </NavLink>
          <span className="font-mono text-[10px] uppercase font-semibold px-2 py-0.5 rounded-full border border-[#e0e0e0] bg-[#f5f5f7] text-[#7a7a7a]">
            Pro MVP
          </span>
        </SidebarHeader>

        {/* Main Navigation */}
        <SidebarContent className="px-2 py-3">
          <SidebarGroup>
            <SidebarGroupLabel className="text-[11px] font-semibold uppercase tracking-wider text-[#7a7a7a] px-2 mb-1">
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
                            ? "bg-[#0066cc] text-white font-medium hover:bg-[#0071e3] hover:text-white"
                            : "text-[#333333] hover:bg-[#f5f5f7] hover:text-[#1d1d1f]"
                        }`}
                      >
                        <FolderKanban className={`size-4 ${isActive ? "text-white" : "text-[#7a7a7a]"}`} />
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
                            ? "bg-[#0066cc] text-white font-medium hover:bg-[#0071e3] hover:text-white"
                            : "text-[#333333] hover:bg-[#f5f5f7] hover:text-[#1d1d1f]"
                        }`}
                      >
                        <UploadCloud className={`size-4 ${isActive ? "text-white" : "text-[#7a7a7a]"}`} />
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
              <SidebarGroupLabel className="text-[11px] font-semibold uppercase tracking-wider text-[#7a7a7a] p-0">
                Organizations
              </SidebarGroupLabel>
              <button
                type="button"
                onClick={() => setShowCreateOrgModal(true)}
                className="text-[#0066cc] hover:text-[#0071e3] p-0.5 rounded cursor-pointer"
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
                              ? "bg-[#0066cc] text-white font-medium hover:bg-[#0071e3] hover:text-white"
                              : "text-[#333333] hover:bg-[#f5f5f7] hover:text-[#1d1d1f]"
                          }`}
                        >
                          <Building2 className={`size-4 ${isActive ? "text-white" : "text-[#7a7a7a]"}`} />
                          <span className="truncate">{org.name}</span>
                        </SidebarMenuButton>
                      )}
                    </NavLink>
                  </SidebarMenuItem>
                ))}
                {organizations.length === 0 && (
                  <div className="px-3 py-2 text-[11px] text-[#7a7a7a] italic">
                    No organizations yet
                  </div>
                )}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        {/* Footer Info */}
        <SidebarFooter className="border-t border-[#e0e0e0] p-3 text-[11px] text-[#7a7a7a] bg-[#fafafc] space-y-2">
          {user && (
            <div className="pb-2 border-b border-[#e0e0e0]/60 flex items-center justify-between">
              <div className="flex items-center gap-2 overflow-hidden">
                <div className="size-6 rounded-full bg-[#0066cc]/10 text-[#0066cc] font-semibold flex items-center justify-center text-[10px] shrink-0">
                  {user.name?.charAt(0) || "U"}
                </div>
                <div className="truncate">
                  <div className="text-[11px] font-medium text-[#1d1d1f] truncate">{user.name}</div>
                  <div className="text-[10px] text-[#7a7a7a] truncate">{user.email}</div>
                </div>
              </div>
              <button
                type="button"
                onClick={logout}
                className="p-1 rounded hover:bg-[#f5f5f7] text-[#7a7a7a] hover:text-[#dc2626] transition-colors"
                title="Sign Out"
              >
                <LogOut className="size-3.5" />
              </button>
            </div>
          )}
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-[#34c759] inline-block animate-pulse"></span>
              Spatial Server
            </span>
            <span className="font-mono text-[10px] text-[#7a7a7a]">Online</span>
          </div>
          <div className="pt-1.5 border-t border-[#e0e0e0]/60 flex items-center justify-between">
            <span className="text-[10px] text-[#7a7a7a]">Workspace Session</span>
            <EndSessionDialog
              trigger={
                <button
                  type="button"
                  className="text-[10px] font-medium text-[#7a7a7a] hover:text-[#dc2626] transition-colors cursor-pointer"
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
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-xs p-4">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl border border-[#e0e0e0]">
            <h3 className="text-base font-bold text-[#1d1d1f] mb-1">Create New Organization</h3>
            <p className="text-xs text-[#7a7a7a] mb-4">
              Set up an isolated team workspace to collaborate with multiple members and manage organizational projects.
            </p>
            <form onSubmit={handleCreateOrg} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Organization Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Acme GeoSolutions"
                  value={newOrgName}
                  onChange={(e) => setNewOrgName(e.target.value)}
                  className="w-full rounded-xl border border-[#e0e0e0] px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold text-[#1d1d1f] mb-1">Description (Optional)</label>
                <textarea
                  rows={3}
                  placeholder="e.g. Primary mapping and surveying workspace"
                  value={newOrgDesc}
                  onChange={(e) => setNewOrgDesc(e.target.value)}
                  className="w-full rounded-xl border border-[#e0e0e0] px-3.5 py-2 text-xs focus:outline-none focus:border-[#0066cc]"
                />
              </div>
              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateOrgModal(false)}
                  className="rounded-xl border border-[#e0e0e0] px-4 py-2 text-xs font-medium text-[#7a7a7a] hover:bg-[#f5f5f7] cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingOrg}
                  className="rounded-xl bg-[#0066cc] px-4 py-2 text-xs font-medium text-white hover:bg-[#0071e3] transition-colors cursor-pointer"
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
