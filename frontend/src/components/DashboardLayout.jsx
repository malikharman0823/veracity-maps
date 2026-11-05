import React from "react"
import { Outlet } from "react-router-dom"
import Sidebar from "./Sidebar"
import { SidebarProvider, SidebarInset } from "@/components/ui/sidebar"

export default function DashboardLayout() {
  return (
    <SidebarProvider defaultOpen={true}>
      <div className="flex min-h-screen w-full bg-white dark:bg-[#1c1c1c] text-neutral-900 dark:text-neutral-100 transition-colors">
        <Sidebar />
        <SidebarInset className="flex flex-1 flex-col min-w-0 bg-white dark:bg-[#1c1c1c] transition-colors">
          <Outlet />
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}
