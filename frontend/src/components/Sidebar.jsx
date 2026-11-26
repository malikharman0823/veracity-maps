import React, { useEffect, useState, useCallback } from "react"
import { NavLink, useNavigate, Link } from "react-router-dom"
import {
  Sidebar as ShadcnSidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  useSidebar,
} from "@/components/ui/sidebar"
import {
  Upload,
  FolderKanban,
  Settings,
  Compass,
  LogOut,
} from "lucide-react"
import { useAuth } from "../context/AuthContext"
import { listMaps } from "../api"

export default function Sidebar() {
  const { setOpenMobile } = useSidebar()
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const [counts, setCounts] = useState({ projects: 0, shared: 0 })

  const refreshCounts = useCallback(
    async (detail) => {
      if (detail && Array.isArray(detail.myMaps) && Array.isArray(detail.sharedWithMe)) {
        setCounts({
          projects: detail.myMaps.length,
          shared: detail.sharedWithMe.length,
        })
        return
      }
      if (!user?.id) return
      try {
        const data = await listMaps()
        setCounts({
          projects: (data?.my_maps || []).length,
          shared: (data?.shared_with_me || []).length,
        })
      } catch {
        // ignore background count errors
      }
    },
    [user?.id]
  )

  useEffect(() => {
    refreshCounts()
  }, [refreshCounts])

  useEffect(() => {
    const onMapsUpdated = (e) => refreshCounts(e?.detail)
    window.addEventListener("veracity:maps-updated", onMapsUpdated)
    return () => window.removeEventListener("veracity:maps-updated", onMapsUpdated)
  }, [refreshCounts])

  const handleNavClick = () => {
    if (setOpenMobile) setOpenMobile(false)
  }

  const navButtonClass = (active) =>
    `w-full justify-between gap-2.5 rounded-[6px] px-3 py-2 text-xs font-medium transition-colors ${
      active
        ? "bg-[#3ecf8e] text-[#171717] font-semibold hover:bg-[#34b77c] hover:text-[#171717]"
        : "text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-[#242424] hover:text-neutral-900 dark:hover:text-white"
    }`

  const iconClass = (active) =>
    `size-4 shrink-0 ${active ? "text-[#171717]" : "text-neutral-400 dark:text-neutral-500"}`

  const badgeClass = (active) =>
    `text-[10px] font-semibold px-1.5 py-0.5 rounded-[4px] ${
      active
        ? "bg-[#171717]/15 text-[#171717]"
        : "bg-neutral-100 dark:bg-[#262626] text-neutral-500 dark:text-neutral-400"
    }`

  const navItems = [
    { label: "Upload Dataset", to: "/upload", icon: Upload, badge: null },
    { label: "My Projects", to: "/projects", icon: FolderKanban, badge: counts.projects > 0 ? String(counts.projects) : null },
    { label: "Settings", to: "/settings", icon: Settings, badge: null },
  ]

  return (
    <ShadcnSidebar className="border-r border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#171717] text-neutral-800 dark:text-neutral-100">
      {/* Brand Header */}
      <SidebarHeader className="h-14 border-b border-neutral-200 dark:border-[#2a2a2a] px-4 flex flex-row items-center justify-between">
        <Link to="/upload" onClick={handleNavClick} className="flex items-center gap-2.5">
          <div className="h-7 w-7 rounded-[6px] bg-[#3ecf8e]/15 border border-[#3ecf8e]/30 text-[#24b47e] dark:text-[#3ecf8e] flex items-center justify-center">
            <Compass className="size-4" />
          </div>
          <span className="font-semibold text-sm tracking-tight text-neutral-900 dark:text-white">
            Veracity Maps
          </span>
        </Link>
      </SidebarHeader>

      {/* Main Navigation — Strictly 4 Primary Destinations */}
      <SidebarContent className="px-2 py-3">
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu className="space-y-1">
              {navItems.map(({ label, to, icon: Icon, badge }) => (
                <SidebarMenuItem key={to}>
                  <NavLink to={to} onClick={handleNavClick}>
                    {({ isActive }) => (
                      <SidebarMenuButton isActive={isActive} className={navButtonClass(isActive)}>
                        <span className="flex items-center gap-2.5 truncate">
                          <Icon className={iconClass(isActive)} />
                          <span className="truncate">{label}</span>
                        </span>
                        {badge !== null && <span className={badgeClass(isActive)}>{badge}</span>}
                      </SidebarMenuButton>
                    )}
                  </NavLink>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      {/* Footer: User Profile & Logout */}
      <SidebarFooter className="border-t border-neutral-200 dark:border-[#2a2a2a] p-3">
        {user && (
          <div className="flex items-center justify-between gap-2">
            <Link
              to="/profile"
              onClick={handleNavClick}
              className="flex items-center gap-2.5 min-w-0 flex-1 rounded-[6px] p-1 hover:bg-neutral-100 dark:hover:bg-[#242424] transition-colors"
              title="Open Profile"
            >
              <div className="size-7 rounded-full bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] font-semibold flex items-center justify-center text-xs border border-[#3ecf8e]/30 shrink-0">
                {user.name?.charAt(0)?.toUpperCase() || "U"}
              </div>
              <div className="min-w-0">
                <div className="text-xs font-medium text-neutral-900 dark:text-white truncate">
                  {user.name}
                </div>
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400 truncate">
                  {user.email}
                </div>
              </div>
            </Link>
            <button
              type="button"
              onClick={() => logout().then(() => navigate("/login"))}
              className="p-1.5 rounded-[6px] hover:bg-red-500/10 text-neutral-500 hover:text-red-500 transition-colors cursor-pointer shrink-0"
              title="Sign Out"
              aria-label="Sign Out"
            >
              <LogOut className="size-4" />
            </button>
          </div>
        )}
      </SidebarFooter>
    </ShadcnSidebar>
  )
}
