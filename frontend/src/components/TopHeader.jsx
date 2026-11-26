import React from "react"
import { Link } from "react-router-dom"
import { SidebarTrigger, SidebarContext } from "@/components/ui/sidebar"
import { Separator } from "@/components/ui/separator"
import { User, ChevronRight } from "lucide-react"
import { useAuth } from "../context/AuthContext"

export default function TopHeader({ title = "", breadcrumbs = [], actions }) {
  const { user } = useAuth() || {}
  const hasSidebar = React.useContext(SidebarContext)

  return (
    <header className="sticky top-0 z-40 h-14 bg-white/90 dark:bg-[#171717]/90 backdrop-blur-md border-b border-neutral-200 dark:border-[#2a2a2a] px-4 md:px-6 flex items-center justify-between gap-4 text-neutral-900 dark:text-neutral-100">
      {/* Left: Sidebar trigger + Page Title / Breadcrumbs */}
      <div className="flex items-center gap-3 min-w-0">
        {hasSidebar && (
          <>
            <SidebarTrigger className="text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#262626] rounded-[6px]" />
            <Separator orientation="vertical" className="h-4 bg-neutral-200 dark:bg-[#2e2e2e]" />
          </>
        )}

        {breadcrumbs.length > 0 ? (
          <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 text-xs min-w-0 truncate">
            {breadcrumbs.map((crumb, i) => (
              <React.Fragment key={i}>
                {crumb.path ? (
                  <Link
                    to={crumb.path}
                    className="text-neutral-500 dark:text-neutral-400 hover:text-[#24b47e] dark:hover:text-[#3ecf8e] transition-colors truncate"
                  >
                    {crumb.label}
                  </Link>
                ) : (
                  <span
                    className={
                      i === breadcrumbs.length - 1
                        ? "text-neutral-900 dark:text-white font-semibold truncate"
                        : "text-neutral-500 dark:text-neutral-400 truncate"
                    }
                  >
                    {crumb.label}
                  </span>
                )}
                {i < breadcrumbs.length - 1 && (
                  <ChevronRight className="size-3 text-neutral-400 shrink-0" />
                )}
              </React.Fragment>
            ))}
          </nav>
        ) : (
          <h1 className="text-sm font-semibold text-neutral-900 dark:text-white truncate">
            {title || "Veracity Maps"}
          </h1>
        )}
      </div>

      {/* Right: Primary Contextual Action + User Profile Menu */}
      <div className="flex items-center gap-2.5 shrink-0">
        {actions}
        <Link
          to="/profile"
          className="h-8 w-8 rounded-full bg-neutral-100 dark:bg-[#242424] border border-neutral-200 dark:border-[#323232] flex items-center justify-center text-neutral-700 dark:text-neutral-200 text-xs font-semibold hover:border-[#3ecf8e] hover:text-[#24b47e] dark:hover:text-[#3ecf8e] transition-colors"
          title={user?.name ? `${user.name} — Profile` : "Profile"}
          aria-label="User Profile"
        >
          {user?.name ? user.name.charAt(0).toUpperCase() : <User className="size-4" />}
        </Link>
      </div>
    </header>
  )
}
