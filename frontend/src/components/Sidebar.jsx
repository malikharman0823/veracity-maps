import React from "react"
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
} from "lucide-react"
import EndSessionDialog from "./EndSessionDialog"
import ThemeToggle from "./ThemeToggle"
import { useAuth } from "../context/AuthContext"

export default function Sidebar() {
  const { setOpenMobile } = useSidebar()
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const handleNavClick = () => {
    // Automatically close mobile drawer when link is clicked
    if (setOpenMobile) {
      setOpenMobile(false)
    }
  }

  return (
    <>
      <ShadcnSidebar
        className="border-r border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1c1c1c] text-neutral-800 dark:text-neutral-100"
      >
        {/* Brand Header */}
        <SidebarHeader className="h-16 border-b border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1c1c1c] px-4 flex flex-row items-center justify-between">
          <NavLink
            to="/"
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
        <SidebarContent className="px-2 py-3 bg-white dark:bg-[#1c1c1c]">
          <SidebarGroup>
            <SidebarGroupLabel className="text-[11px] font-semibold uppercase tracking-wider text-neutral-500 dark:text-neutral-400 px-2 mb-1">
              Personal Workspace
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu className="space-y-1">
                <SidebarMenuItem>
                  <NavLink to="/dashboard" onClick={handleNavClick}>
                    {({ isActive }) => (
                      <SidebarMenuButton
                        isActive={isActive}
                        className={`w-full justify-start gap-2.5 rounded-[6px] px-3 py-2 text-xs font-medium transition-all ${
                          isActive
                            ? "bg-[#3ecf8e] text-[#171717] font-semibold hover:bg-[#24b47e] hover:text-[#171717]"
                            : "text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-[#242424] hover:text-neutral-900 dark:hover:text-white"
                        }`}
                      >
                        <FolderKanban className={`size-4 ${isActive ? "text-[#171717]" : "text-neutral-400"}`} />
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
                        className={`w-full justify-start gap-2.5 rounded-[6px] px-3 py-2 text-xs font-medium transition-all ${
                          isActive
                            ? "bg-[#3ecf8e] text-[#171717] font-semibold hover:bg-[#24b47e] hover:text-[#171717]"
                            : "text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-[#242424] hover:text-neutral-900 dark:hover:text-white"
                        }`}
                      >
                        <UploadCloud className={`size-4 ${isActive ? "text-[#171717]" : "text-neutral-400"}`} />
                        <span>Upload Dataset</span>
                      </SidebarMenuButton>
                    )}
                  </NavLink>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        {/* Footer Info */}
        <SidebarFooter className="border-t border-neutral-200 dark:border-[#2e2e2e] p-3 text-[11px] text-neutral-500 dark:text-neutral-400 bg-neutral-50 dark:bg-[#1c1c1c] space-y-2">
          {user && (
            <div className="pb-2 border-b border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
              <div className="flex items-center gap-2 overflow-hidden">
                <div className="size-6 rounded-full bg-[#3ecf8e]/20 text-[#171717] dark:text-[#3ecf8e] font-semibold flex items-center justify-center text-[10px] border border-[#3ecf8e]/30 shrink-0">
                  {user.name?.charAt(0) || "U"}
                </div>
                <div className="truncate">
                  <div className="text-[11px] font-medium text-neutral-900 dark:text-white truncate">{user.name}</div>
                  <div className="text-[10px] text-neutral-500 dark:text-neutral-400 truncate">{user.email}</div>
                </div>
              </div>
              <button
                type="button"
                onClick={logout}
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
          <div className="pt-1.5 border-t border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
            <span className="text-[10px] text-neutral-500 dark:text-neutral-400">Workspace Session</span>
            <EndSessionDialog
              trigger={
                <button
                  type="button"
                  className="text-[10px] font-medium text-neutral-500 hover:text-red-500 transition-colors cursor-pointer"
                  title="End session and clear workspace"
                >
                  Clear
                </button>
              }
            />
          </div>
        </SidebarFooter>
      </ShadcnSidebar>
    </>
  )
}
