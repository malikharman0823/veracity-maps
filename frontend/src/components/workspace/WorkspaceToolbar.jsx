import React, { useState, useRef, useEffect } from "react"
import { Link } from "react-router-dom"
import {
  ChevronDown,
  Save,
  FolderOpen,
  Plus,
  Compass,
  Edit3,
  Ruler,
  Download,
  Wrench,
  Layers,
  Table2,
  SlidersHorizontal,
  Cpu,
  MousePointer2,
  MapPin,
  Minus,
  Pentagon,
  Trash2,
  Undo2,
  Redo2,
  Square,
  Crosshair,
  ArrowLeft,
  Loader2,
} from "lucide-react"
import { SidebarTrigger, SidebarContext } from "@/components/ui/sidebar"

/**
 * Unified, minimal Workspace Top Toolbar (replaces WorkspaceMenuBar + legacy multi-group toolbar).
 * Primary actions only: Project menu, Add Data, Georeference, Edit, Measure, Export + compact Tools menu.
 */
export default function WorkspaceToolbar({
  projectName = "Workspace",
  saving = false,
  activeTool = "select",
  measureMode = null,
  canUndo = false,
  canRedo = false,
  layersOpen = true,
  bottomTab = null, // null | "properties" | "attributes" | "processing"
  onSave,
  onOpenReopen,
  onAddData,
  onGeoreference,
  onSelectTool,
  onUndo,
  onRedo,
  onToggleMeasure,
  onOpenExport,
  onToggleLayers,
  onOpenBottomTab,
}) {
  const hasSidebar = React.useContext(SidebarContext)
  const [openMenu, setOpenMenu] = useState(null) // "project" | "edit" | "measure" | "tools" | null
  const barRef = useRef(null)

  useEffect(() => {
    if (!openMenu) return
    const onDocClick = (e) => {
      if (barRef.current && !barRef.current.contains(e.target)) {
        setOpenMenu(null)
      }
    }
    document.addEventListener("mousedown", onDocClick)
    return () => document.removeEventListener("mousedown", onDocClick)
  }, [openMenu])

  const btnBase =
    "inline-flex items-center gap-1.5 h-8 px-2.5 rounded-[6px] text-xs font-medium transition-colors cursor-pointer whitespace-nowrap"
  const btnGhost = `${btnBase} text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#262626]`
  const btnActive = `${btnBase} bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/35`

  const isEditingTool = activeTool && activeTool !== "select"

  return (
    <header
      ref={barRef}
      className="h-12 px-3 border-b border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#171717] flex items-center justify-between gap-2 shrink-0 select-none z-40"
    >
      {/* Left: Sidebar trigger + Project Menu */}
      <div className="flex items-center gap-2 min-w-0">
        {hasSidebar && (
          <SidebarTrigger className="text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#262626] rounded-[6px]" />
        )}

        <div className="relative">
          <button
            type="button"
            onClick={() => setOpenMenu((m) => (m === "project" ? null : "project"))}
            className={`${btnGhost} font-semibold max-w-[200px]`}
            title="Project menu"
          >
            <span className="truncate">{projectName}</span>
            <ChevronDown className="size-3.5 text-neutral-400 shrink-0" />
          </button>

          {openMenu === "project" && (
            <div className="absolute left-0 mt-1 w-48 rounded-[8px] border border-neutral-200 dark:border-[#323232] bg-white dark:bg-[#1f1f1f] shadow-xl py-1 text-xs z-50">
              <button
                type="button"
                disabled={saving}
                onClick={() => {
                  setOpenMenu(null)
                  onSave?.()
                }}
                className="w-full px-3 py-2 text-left hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center gap-2 cursor-pointer"
              >
                {saving ? <Loader2 className="size-3.5 animate-spin text-[#3ecf8e]" /> : <Save className="size-3.5 text-[#3ecf8e]" />}
                <span>Save Project</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  setOpenMenu(null)
                  onOpenReopen?.()
                }}
                className="w-full px-3 py-2 text-left hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center gap-2 cursor-pointer"
              >
                <FolderOpen className="size-3.5 text-neutral-400" />
                <span>Reopen Package (.zip)</span>
              </button>
              <div className="my-1 border-t border-neutral-200 dark:border-[#2e2e2e]" />
              <Link
                to="/projects"
                onClick={() => setOpenMenu(null)}
                className="w-full px-3 py-2 text-left hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center gap-2 text-neutral-600 dark:text-neutral-300"
              >
                <ArrowLeft className="size-3.5 text-neutral-400" />
                <span>All Projects</span>
              </Link>
            </div>
          )}
        </div>
      </div>

      {/* Center: Core 5 Workflow Actions + Contextual Tools */}
      <div className="flex items-center gap-1 overflow-x-auto no-scrollbar">
        {/* 1. Add Data */}
        <button type="button" onClick={onAddData} className={btnGhost} title="Upload or add raster, vector, CAD, or PDF data">
          <Plus className="size-3.5 text-[#3ecf8e]" />
          <span className="hidden sm:inline">Add Data</span>
        </button>

        {/* 2. Georeference */}
        <button type="button" onClick={onGeoreference} className={btnGhost} title="Georeference active raster">
          <Compass className="size-3.5 text-sky-500" />
          <span className="hidden sm:inline">Georeference</span>
        </button>

        {/* 3. Edit dropdown */}
        <div className="relative">
          <button
            type="button"
            onClick={() => setOpenMenu((m) => (m === "edit" ? null : "edit"))}
            className={isEditingTool ? btnActive : btnGhost}
            title="Vector editing tools"
          >
            <Edit3 className="size-3.5" />
            <span className="hidden sm:inline">Edit</span>
            <ChevronDown className="size-3 opacity-60" />
          </button>

          {openMenu === "edit" && (
            <div className="absolute left-0 mt-1 w-44 rounded-[8px] border border-neutral-200 dark:border-[#323232] bg-white dark:bg-[#1f1f1f] shadow-xl py-1 text-xs z-50">
              {[
                { id: "select", label: "Select", icon: MousePointer2 },
                { id: "point", label: "Add Point", icon: MapPin },
                { id: "line", label: "Draw Line", icon: Minus },
                { id: "polygon", label: "Draw Polygon", icon: Pentagon },
                { id: "vertex", label: "Edit Vertices", icon: Edit3 },
                { id: "delete", label: "Delete Features", icon: Trash2 },
              ].map(({ id, label, icon: Icon }) => (
                <button
                  key={id}
                  type="button"
                  onClick={() => {
                    onSelectTool?.(id)
                    setOpenMenu(null)
                  }}
                  className={`w-full px-3 py-1.5 text-left flex items-center gap-2 cursor-pointer ${
                    activeTool === id
                      ? "bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] font-semibold"
                      : "hover:bg-neutral-100 dark:hover:bg-[#2a2a2a]"
                  }`}
                >
                  <Icon className="size-3.5" />
                  <span>{label}</span>
                </button>
              ))}
              {(canUndo || canRedo) && (
                <>
                  <div className="my-1 border-t border-neutral-200 dark:border-[#2e2e2e]" />
                  <div className="px-2 py-1 flex items-center gap-1">
                    <button
                      type="button"
                      disabled={!canUndo}
                      onClick={() => {
                        onUndo?.()
                        setOpenMenu(null)
                      }}
                      className="flex-1 py-1 rounded hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center justify-center gap-1 disabled:opacity-35 cursor-pointer"
                    >
                      <Undo2 className="size-3" /> Undo
                    </button>
                    <button
                      type="button"
                      disabled={!canRedo}
                      onClick={() => {
                        onRedo?.()
                        setOpenMenu(null)
                      }}
                      className="flex-1 py-1 rounded hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center justify-center gap-1 disabled:opacity-35 cursor-pointer"
                    >
                      <Redo2 className="size-3" /> Redo
                    </button>
                  </div>
                </>
              )}
            </div>
          )}
        </div>

        {/* 4. Measure dropdown */}
        <div className="relative">
          <button
            type="button"
            onClick={() => setOpenMenu((m) => (m === "measure" ? null : "measure"))}
            className={measureMode ? btnActive : btnGhost}
            title="Measurement tools"
          >
            <Ruler className="size-3.5" />
            <span className="hidden sm:inline">Measure</span>
            <ChevronDown className="size-3 opacity-60" />
          </button>

          {openMenu === "measure" && (
            <div className="absolute left-0 mt-1 w-40 rounded-[8px] border border-neutral-200 dark:border-[#323232] bg-white dark:bg-[#1f1f1f] shadow-xl py-1 text-xs z-50">
              {[
                { id: "distance", label: "Distance", icon: Ruler },
                { id: "area", label: "Area", icon: Square },
                { id: "coordinates", label: "Coordinates", icon: Crosshair },
              ].map(({ id, label, icon: Icon }) => (
                <button
                  key={id}
                  type="button"
                  onClick={() => {
                    onToggleMeasure?.(id)
                    setOpenMenu(null)
                  }}
                  className={`w-full px-3 py-1.5 text-left flex items-center gap-2 cursor-pointer ${
                    measureMode === id
                      ? "bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] font-semibold"
                      : "hover:bg-neutral-100 dark:hover:bg-[#2a2a2a]"
                  }`}
                >
                  <Icon className="size-3.5" />
                  <span>{label}</span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* 5. Contextual Tools dropdown */}
        <div className="relative">
          <button
            type="button"
            onClick={() => setOpenMenu((m) => (m === "tools" ? null : "tools"))}
            className={bottomTab ? btnActive : btnGhost}
            title="Workspace panels & contextual tools"
          >
            <Wrench className="size-3.5" />
            <span className="hidden md:inline">Tools</span>
            <ChevronDown className="size-3 opacity-60" />
          </button>

          {openMenu === "tools" && (
            <div className="absolute right-0 mt-1 w-48 rounded-[8px] border border-neutral-200 dark:border-[#323232] bg-white dark:bg-[#1f1f1f] shadow-xl py-1 text-xs z-50">
              <button
                type="button"
                onClick={() => {
                  onToggleLayers?.()
                  setOpenMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center justify-between cursor-pointer"
              >
                <span className="flex items-center gap-2">
                  <Layers className="size-3.5 text-neutral-400" /> Layers Panel
                </span>
                <span className="text-[10px] text-neutral-400">{layersOpen ? "On" : "Off"}</span>
              </button>
              <div className="my-1 border-t border-neutral-200 dark:border-[#2e2e2e]" />
              <button
                type="button"
                onClick={() => {
                  onOpenBottomTab?.(bottomTab === "properties" ? null : "properties")
                  setOpenMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center gap-2 cursor-pointer"
              >
                <SlidersHorizontal className="size-3.5 text-neutral-400" /> Layer Properties
              </button>
              <button
                type="button"
                onClick={() => {
                  onOpenBottomTab?.(bottomTab === "attributes" ? null : "attributes")
                  setOpenMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center gap-2 cursor-pointer"
              >
                <Table2 className="size-3.5 text-neutral-400" /> Attribute Table
              </button>
              <button
                type="button"
                onClick={() => {
                  onOpenBottomTab?.(bottomTab === "processing" ? null : "processing")
                  setOpenMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] flex items-center gap-2 cursor-pointer"
              >
                <Cpu className="size-3.5 text-neutral-400" /> Processing Toolbox
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Right: Primary Export Action */}
      <div className="flex items-center gap-1.5 shrink-0">
        <button
          type="button"
          onClick={onOpenExport}
          className="inline-flex items-center gap-1.5 h-8 px-3 rounded-[6px] bg-[#3ecf8e] hover:bg-[#34b77c] text-[#141414] text-xs font-semibold cursor-pointer transition-colors"
          title="Export Custom Web Tile or Custom Local Tiles"
        >
          <Download className="size-3.5" />
          <span>Export</span>
        </button>
      </div>
    </header>
  )
}
