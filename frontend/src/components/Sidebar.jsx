import React, { useEffect, useState, useCallback } from "react"
import { NavLink, useNavigate, useLocation, Link } from "react-router-dom"
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
  UploadCloud,
  Compass,
  FolderKanban,
  LogOut,
  User,
  Map as MapIcon,
  Users,
  Bookmark,
  Clock,
  Shield,
} from "lucide-react"
import ThemeToggle from "./ThemeToggle"
import { useAuth } from "../context/AuthContext"
import { listMaps } from "../api"
import { resolveSavedMaps } from "../lib/savedMaps"

export default function Sidebar() {
  const { setOpenMobile } = useSidebar()
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const [counts, setCounts] = useState({
    myMaps: 0,
    sharedWithMe: 0,
    savedMaps: 0,
    recentMaps: 0,
  })

  const refreshCounts = useCallback(async (detail) => {
    if (detail && Array.isArray(detail.myMaps) && Array.isArray(detail.sharedWithMe)) {
      setCounts({
        myMaps: detail.myMaps.length,
        sharedWithMe: detail.sharedWithMe.length,
        savedMaps: Array.isArray(detail.savedMaps) ? detail.savedMaps.length : 0,
        recentMaps: detail.myMaps.length + detail.sharedWithMe.length,
      })
      return
    }
    if (!user?.id) return
    try {
      const data = await listMaps()
      const my = data?.my_maps || []
      const shared = data?.shared_with_me || []
      const saved = resolveSavedMaps(data)
      setCounts({
        myMaps: my.length,
        sharedWithMe: shared.length,
        savedMaps: saved.length,
        recentMaps: my.length + shared.length,
      })
    } catch {
      // ignore background count errors
    }
  }, [user?.id])

  useEffect(() => {
    refreshCounts()
  }, [refreshCounts])

  useEffect(() => {
    const onMapsUpdated = (e) => refreshCounts(e?.detail)
    window.addEventListener("veracity:maps-updated", onMapsUpdated)
    return () => window.removeEventListener("veracity:maps-updated", onMapsUpdated)
  }, [refreshCounts])

  const handleNavClick = () => {
    if (setOpenMobile) {
      setOpenMobile(false)
    }
  }

  const searchParams = new URLSearchParams(location.search)
  const currentTab = searchParams.get("tab") || "my-maps"
  const isDashboard = location.pathname === "/dashboard"

  const navButtonClass = (active) =>
    `w-full justify-between gap-2.5 rounded-[6px] px-3 py-2 text-xs font-medium transition-all ${
      active
        ? "bg-[#3ecf8e] text-[#171717] font-semibold hover:bg-[#24b47e] hover:text-[#171717]"
        : "text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-[#242424] hover:text-neutral-900 dark:hover:text-white"
    }`

  const iconClass = (active) => `size-4 shrink-0 ${active ? "text-[#171717]" : "text-neutral-400"}`

  const badgeClass = (active) =>
    `text-[10px] font-semibold px-1.5 py-0.5 rounded-[4px] ${
      active
        ? "bg-[#171717]/15 text-[#171717]"
        : "bg-neutral-100 dark:bg-[#262626] text-neutral-500 dark:text-neutral-400"
    }`

  const dashboardNavItems = [
    {
      label: "My Maps",
      tab: "my-maps",
      to: "/dashboard?tab=my-maps",
      icon: MapIcon,
      badge: `${counts.myMaps}/10`,
      active: isDashboard && (currentTab === "my-maps" || !searchParams.get("tab")),
    },
    {
      label: "Shared With Me",
      tab: "shared-with-me",
      to: "/dashboard?tab=shared-with-me",
      icon: Users,
      badge: `${counts.sharedWithMe}`,
      active: isDashboard && currentTab === "shared-with-me",
    },
    {
      label: "Saved Maps",
      tab: "saved-maps",
      to: "/dashboard?tab=saved-maps",
      icon: Bookmark,
      badge: `${counts.savedMaps}`,
      active: isDashboard && currentTab === "saved-maps",
    },
    {
      label: "Recent Maps",
      tab: "recent",
      to: "/dashboard?tab=recent",
      icon: Clock,
      badge: `${counts.recentMaps}`,
      active: isDashboard && currentTab === "recent",
    },
    {
      label: "Profile & Security",
      tab: "settings",
      to: "/dashboard?tab=settings",
      icon: Shield,
      badge: null,
      active: isDashboard && currentTab === "settings",
    },
  ]

  return (
    <>
      <ShadcnSidebar
        className="border-r border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1c1c1c] text-neutral-800 dark:text-neutral-100"
      >
        {/* Brand Header */}
        <SidebarHeader className="h-16 border-b border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1c1c1c] px-4 flex flex-row items-center justify-between">
          <NavLink
            to="/dashboard"
            onClick={handleNavClick}
            className="flex items-center gap-2.5 group"
          >
            <div className="h-8 w-8 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-[#24b47e] dark:text-[#3ecf8e] flex items-center justify-center font-bold shadow-xs transition-transform group-hover:scale-105">
              <Compass className="size-4" />
            </div>
            <div className="flex flex-col">
              <span className="font-semibold text-sm tracking-tight text-neutral-900 dark:text-white">
                Veracity Maps
              </span>
            </div>
          </NavLink>
          <span className="text-xs px-2 py-0.5 rounded-[4px] bg-neutral-100 dark:bg-[#202020] text-neutral-600 dark:text-neutral-400 border border-neutral-200 dark:border-[#2e2e2e]">
            1.3
          </span>
        </SidebarHeader>

        {/* Main Navigation */}
        <SidebarContent className="px-2 py-3 bg-white dark:bg-[#1c1c1c] space-y-2">
          <SidebarGroup>
            <SidebarGroupLabel className="text-[11px] font-semibold uppercase tracking-wider text-neutral-500 dark:text-neutral-400 px-2 mb-1">
              Personal Workspace
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu className="space-y-1">
                <SidebarMenuItem>
                  <Link to="/dashboard" onClick={handleNavClick}>
                    <SidebarMenuButton
                      isActive={isDashboard && !searchParams.get("tab")}
                      className={navButtonClass(isDashboard && !searchParams.get("tab"))}
                    >
                      <span className="flex items-center gap-2.5 truncate">
                        <FolderKanban className={iconClass(isDashboard && !searchParams.get("tab"))} />
                        <span className="truncate">My Projects</span>
                      </span>
                    </SidebarMenuButton>
                  </Link>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <NavLink to="/profile" onClick={handleNavClick}>
                    {({ isActive }) => (
                      <SidebarMenuButton
                        isActive={isActive}
                        className={navButtonClass(isActive)}
                      >
                        <span className="flex items-center gap-2.5 truncate">
                          <User className={iconClass(isActive)} />
                          <span className="truncate">Profile & Files</span>
                        </span>
                      </SidebarMenuButton>
                    )}
                  </NavLink>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <NavLink to="/" end onClick={handleNavClick}>
                    {({ isActive }) => (
                      <SidebarMenuButton
                        isActive={isActive}
                        className={navButtonClass(isActive)}
                      >
                        <span className="flex items-center gap-2.5 truncate">
                          <UploadCloud className={iconClass(isActive)} />
                          <span className="truncate">Upload Dataset</span>
                        </span>
                      </SidebarMenuButton>
                    )}
                  </NavLink>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>

          <SidebarGroup>
            <SidebarGroupLabel className="text-[11px] font-semibold uppercase tracking-wider text-neutral-500 dark:text-neutral-400 px-2 mb-1">
              Maps & Navigation
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu className="space-y-1">
                {dashboardNavItems.map((item) => {
                  const Icon = item.icon
                  return (
                    <SidebarMenuItem key={item.tab}>
                      <Link to={item.to} onClick={handleNavClick}>
                        <SidebarMenuButton
                          isActive={item.active}
                          className={navButtonClass(item.active)}
                        >
                          <span className="flex items-center gap-2.5 truncate">
                            <Icon className={iconClass(item.active)} />
                            <span className="truncate">{item.label}</span>
                          </span>
                          {item.badge !== null && (
                            <span className={badgeClass(item.active)}>{item.badge}</span>
                          )}
                        </SidebarMenuButton>
                      </Link>
                    </SidebarMenuItem>
                  )
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        {/* Footer Info */}
        <SidebarFooter className="border-t border-neutral-200 dark:border-[#2e2e2e] p-3 text-[11px] text-neutral-500 dark:text-neutral-400 bg-neutral-50 dark:bg-[#1c1c1c] space-y-2">
          {user && (
            <div className="pb-2 border-b border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
              <Link to="/profile" onClick={handleNavClick} className="flex items-center gap-2 overflow-hidden hover:opacity-80 transition-opacity">
                <div className="size-6 rounded-full bg-[#3ecf8e]/20 text-[#171717] dark:text-[#3ecf8e] font-semibold flex items-center justify-center text-[10px] border border-[#3ecf8e]/30 shrink-0">
                  {user.name?.charAt(0)?.toUpperCase() || "U"}
                </div>
                <div className="truncate">
                  <div className="text-[11px] font-medium text-neutral-900 dark:text-white truncate">{user.name}</div>
                  <div className="text-[10px] text-neutral-500 dark:text-neutral-400 truncate">{user.email}</div>
                </div>
              </Link>
              <button
                type="button"
                onClick={() => logout().then(() => navigate("/login"))}
                className="p-1 rounded-[4px] hover:bg-neutral-200 dark:hover:bg-[#282828] text-neutral-500 hover:text-red-500 transition-colors cursor-pointer"
                title="Sign Out"
              >
                <LogOut className="size-3.5" />
              </button>
            </div>
          )}
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-[#3ecf8e] inline-block animate-pulse"></span>
              Spatial Server
            </span>
            <span className="font-mono text-[10px] text-neutral-500 dark:text-neutral-400">Online</span>
          </div>
          <div className="pt-1.5 border-t border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
            <span className="text-[10px] text-neutral-500 dark:text-neutral-400">Appearance</span>
            <ThemeToggle />
          </div>
        </SidebarFooter>
      </ShadcnSidebar>
    </>
  )
}
