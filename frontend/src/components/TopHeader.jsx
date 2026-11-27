import React from "react"
import { Link } from "react-router-dom"
import { SidebarTrigger, SidebarContext } from "@/components/ui/sidebar"
import { Separator } from "@/components/ui/separator"
import { User, ChevronRight } from "lucide-react"
import ThemeToggle from "./ThemeToggle"
import { useAuth } from "../context/AuthContext"

export default function TopHeader({ breadcrumbs = [], actions }) {
  const { user } = useAuth() || {}
  const hasSidebar = React.useContext(SidebarContext)

  return (
    <header className="sticky top-0 z-40 h-14 bg-white/80 dark:bg-[#1c1c1c]/80 backdrop-blur-md border-b border-neutral-200 dark:border-[#2e2e2e] px-4 md:px-6 flex items-center justify-between text-neutral-900 dark:text-neutral-100 transition-colors">
      {/* Left: Sidebar trigger + Breadcrumb */}
      <div className="flex items-center gap-3 text-xs">
        {hasSidebar && (
          <>
            <SidebarTrigger className="text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] hover:text-neutral-900 dark:hover:text-white rounded-[6px] transition-colors" />
            <Separator orientation="vertical" className="h-4 bg-neutral-200 dark:bg-[#2e2e2e]" />
          </>
        )}

        <div className="flex items-center gap-1.5 text-xs">
          <span className="text-neutral-500 dark:text-neutral-400 font-normal">Studio</span>
          <ChevronRight className="size-3 text-neutral-400 dark:text-neutral-600" />
          {breadcrumbs.map((crumb, i) => (
            <React.Fragment key={i}>
              {crumb.path ? (
                <Link
                  to={crumb.path}
                  className="text-neutral-500 dark:text-neutral-400 hover:text-[#24b47e] dark:hover:text-[#3ecf8e] transition-colors"
                >
                  {crumb.label}
                </Link>
              ) : (
                <span
                  className={
                    i === breadcrumbs.length - 1
                      ? "text-neutral-900 dark:text-white font-medium"
                      : "text-neutral-500 dark:text-neutral-400"
                  }
                >
                  {crumb.label}
                </span>
              )}
              {i < breadcrumbs.length - 1 && (
                <ChevronRight className="size-3 text-neutral-400 dark:text-neutral-600" />
              )}
            </React.Fragment>
          ))}
        </div>
      </div>

      {/* Right: Actions */}
      <div className="flex items-center gap-2">
        {actions}
        <ThemeToggle />
        <div className="h-4 w-px bg-neutral-200 dark:bg-[#2e2e2e] mx-1 hidden sm:block"></div>
        <Link
          to="/dashboard"
          className="h-8 w-8 rounded-full bg-neutral-100 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center text-neutral-600 dark:text-neutral-300 text-xs font-semibold shadow-xs hover:border-[#3ecf8e] hover:text-[#3ecf8e] transition-colors cursor-pointer"
          title={user?.name ? `${user.name} - Personal Dashboard` : "Personal Dashboard"}
        >
          {user?.name ? user.name.charAt(0).toUpperCase() : <User className="size-4" />}
        </Link>
      </div>
    </header>
  )
}
