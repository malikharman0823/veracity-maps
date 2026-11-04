import React from "react"
import { Link } from "react-router-dom"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { Separator } from "@/components/ui/separator"
import { User, ChevronRight } from "lucide-react"
import EndSessionDialog from "./EndSessionDialog"

export default function TopHeader({ breadcrumbs = [], actions }) {
  return (
    <header className="sticky top-0 z-40 h-14 bg-slate-900/80 backdrop-blur-md border-b border-slate-800 px-4 md:px-6 flex items-center justify-between text-slate-100">
      {/* Left: Sidebar trigger + Breadcrumb */}
      <div className="flex items-center gap-3 text-xs">
        <SidebarTrigger className="text-slate-300 hover:bg-slate-800 hover:text-white rounded-lg transition-colors" />
        <Separator orientation="vertical" className="h-4 bg-slate-800" />

        <div className="flex items-center gap-1.5 text-xs">
          <span className="text-slate-400 font-normal">Workspace</span>
          <ChevronRight className="size-3 text-slate-500" />
          {breadcrumbs.map((crumb, i) => (
            <React.Fragment key={i}>
              {crumb.path ? (
                <Link
                  to={crumb.path}
                  className="text-slate-400 hover:text-blue-400 transition-colors"
                >
                  {crumb.label}
                </Link>
              ) : (
                <span
                  className={
                    i === breadcrumbs.length - 1
                      ? "text-white font-semibold"
                      : "text-slate-400"
                  }
                >
                  {crumb.label}
                </span>
              )}
              {i < breadcrumbs.length - 1 && (
                <ChevronRight className="size-3 text-slate-500" />
              )}
            </React.Fragment>
          ))}
        </div>
      </div>

      {/* Right: Actions */}
      <div className="flex items-center gap-2.5">
        {actions}
        <EndSessionDialog />
        <div className="h-4 w-px bg-slate-800 mx-1 hidden sm:block"></div>
        <div className="h-8 w-8 rounded-full bg-slate-800 border border-slate-700 flex items-center justify-center text-slate-300 text-xs font-medium shadow-xs hover:border-blue-500 hover:text-white transition-colors cursor-pointer">
          <User className="size-4" />
        </div>
      </div>
    </header>
  )
}
