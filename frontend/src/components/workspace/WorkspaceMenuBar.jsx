import React, { useEffect, useMemo, useRef, useState } from "react"
import { Link } from "react-router-dom"
import {
  Check,
  Loader2,
  AlertCircle,
  Bookmark,
  Share2,
  LogOut,
  ChevronRight,
  RefreshCw,
} from "lucide-react"
import ThemeToggle from "../ThemeToggle"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { DEFAULT_SHORTCUTS, formatShortcutForOS } from "../../lib/shortcutRegistry"

export default function WorkspaceMenuBar({
  workspaceName = "Workspace",
  revisionNumber,
  lockHeld = false,
  isLockedByOther = false,
  lockHolderName = "",
  saveState = "saved",
  onRetrySave,
  user,
  layout,
  canUndo = false,
  canRedo = false,
  hasSelection = false,
  hasActiveLayer = false,
  snapEnabled = false,
  mapViewPrefs = {},
  shortcuts = DEFAULT_SHORTCUTS,
  // Actions
  onSave,
  onSaveAs,
  onOpenProperties,
  onExport,
  onReopen,
  onShare,
  onCloseWorkspace,
  onUndo,
  onRedo,
  onSelectTool,
  onToggleSnap,
  onDeleteSelected,
  onZoomIn,
  onZoomOut,
  onZoomFullExtent,
  onZoomToActiveLayer,
  onTogglePanel,
  onResetLayout,
  onAddVector,
  onAddRaster,
  onChangeBaseLayer,
  onOpenLayerProperties,
  onOpenAttributeTable,
  onRemoveActiveLayer,
  onRunAlgorithm,
  onOpenProcessingToolbox,
  onSearchProcessing,
  onOpenProcessingHistory,
  onChangeMapViewPrefs,
  onOpenCRS,
  onOpenShortcuts,
  onOpenGuide,
}) {
  const [openMenu, setOpenMenu] = useState(null)
  const barRef = useRef(null)

  const shortcutFor = useMemo(() => {
    const m = new Map()
    ;(shortcuts || DEFAULT_SHORTCUTS).forEach((s) => {
      if (s?.tool_id && s?.shortcut) {
        m.set(s.tool_id, formatShortcutForOS(s.shortcut))
      }
    })
    return (id) => m.get(id) || ""
  }, [shortcuts])

  useEffect(() => {
    if (!openMenu) return undefined
    const handleOutside = (e) => {
      if (barRef.current && !barRef.current.contains(e.target)) {
        setOpenMenu(null)
      }
    }
    const handleKey = (e) => {
      if (e.key === "Escape") setOpenMenu(null)
    }
    window.addEventListener("mousedown", handleOutside)
    window.addEventListener("keydown", handleKey)
    return () => {
      window.removeEventListener("mousedown", handleOutside)
      window.removeEventListener("keydown", handleKey)
    }
  }, [openMenu])

  const run = (fn) => {
    setOpenMenu(null)
    fn?.()
  }

  const menus = [
    {
      id: "project",
      label: "Project",
      items: [
        { label: "Save Workspace", action: () => run(onSave), disabled: isLockedByOther },
        { label: "Save As…", action: () => run(onSaveAs), disabled: isLockedByOther },
        { type: "sep" },
        { label: "Workspace Properties", action: () => run(onOpenProperties) },
        { label: "Export Workspace…", action: () => run(onExport) },
        { label: "Reopen Project Package…", action: () => run(onReopen) },
        { type: "sep" },
        { label: "Close Workspace", action: () => run(onCloseWorkspace) },
      ],
    },
    {
      id: "edit",
      label: "Edit",
      items: [
        { label: "Undo", shortcut: shortcutFor("undo"), action: () => run(onUndo), disabled: !canUndo },
        { label: "Redo", shortcut: shortcutFor("redo"), action: () => run(onRedo), disabled: !canRedo },
        { type: "sep" },
        { label: "Select Features", shortcut: shortcutFor("select"), action: () => run(() => onSelectTool?.("select")) },
        { label: "Identify Feature", shortcut: shortcutFor("identify"), action: () => run(() => onSelectTool?.("identify")) },
        { label: "Edit Feature Vertices", shortcut: shortcutFor("vertex"), action: () => run(() => onSelectTool?.("vertex")) },
        { label: "Move Feature", shortcut: shortcutFor("move"), action: () => run(() => onSelectTool?.("move")) },
        { label: "Toggle Vertex Snapping", shortcut: shortcutFor("snap"), checked: snapEnabled, action: () => run(onToggleSnap) },
        { type: "sep" },
        {
          label: "Delete Selected Features",
          shortcut: shortcutFor("delete_selected"),
          action: () => run(onDeleteSelected),
          disabled: !hasSelection,
          danger: true,
        },
      ],
    },
    {
      id: "view",
      label: "View",
      items: [
        { label: "Zoom In", action: () => run(onZoomIn) },
        { label: "Zoom Out", action: () => run(onZoomOut) },
        { label: "Zoom to Full Extent", shortcut: shortcutFor("zoom_full"), action: () => run(onZoomFullExtent) },
        { label: "Zoom to Active Layer", action: () => run(onZoomToActiveLayer), disabled: !hasActiveLayer },
        { type: "sep" },
        { label: "Layers Panel", shortcut: shortcutFor("layer_manager"), checked: Boolean(layout?.leftOpen && layout?.layersOpen), action: () => run(() => onTogglePanel?.("layers")) },
        { label: "Processing Toolbox", shortcut: shortcutFor("processing_toolbox"), checked: Boolean(layout?.rightOpen), action: () => run(() => onTogglePanel?.("right")) },
        { label: "Workspace Records", shortcut: shortcutFor("attribute_table"), checked: Boolean(layout?.recordsOpen), action: () => run(() => onTogglePanel?.("records")) },
        { type: "sep" },
        { label: "Reset Workspace Layout", action: () => run(onResetLayout) },
      ],
    },
    {
      id: "layer",
      label: "Layer",
      items: [
        { label: "Add Vector Layer…", shortcut: shortcutFor("add_dataset"), action: () => run(onAddVector) },
        { label: "Add Raster Layer…", action: () => run(onAddRaster) },
        { label: "Change Base Layer…", shortcut: shortcutFor("base_layer"), action: () => run(onChangeBaseLayer) },
        { type: "sep" },
        { label: "Layer Properties & Symbology", shortcut: shortcutFor("symbology"), action: () => run(onOpenLayerProperties), disabled: !hasActiveLayer },
        { label: "Open Layer Attribute Table", action: () => run(onOpenAttributeTable), disabled: !hasActiveLayer },
        { type: "sep" },
        { label: "Remove Active Layer", action: () => run(onRemoveActiveLayer), disabled: !hasActiveLayer, danger: true },
      ],
    },
    {
      id: "vector",
      label: "Vector",
      items: [
        { label: "Buffer…", action: () => run(() => onRunAlgorithm?.("buffer")) },
        { label: "Clip…", action: () => run(() => onRunAlgorithm?.("clip")) },
        { label: "Intersection…", action: () => run(() => onRunAlgorithm?.("intersection")) },
        { label: "Union…", action: () => run(() => onRunAlgorithm?.("union")) },
        { label: "Dissolve…", action: () => run(() => onRunAlgorithm?.("dissolve")) },
        { label: "Spatial Join…", action: () => run(() => onRunAlgorithm?.("spatial_join")) },
        { type: "sep" },
        { label: "Geometry Validation (Topology)…", action: () => run(() => onRunAlgorithm?.("topology")) },
      ],
    },
    {
      id: "raster",
      label: "Raster",
      items: [
        { label: "Raster Calculator…", action: () => run(() => onRunAlgorithm?.("raster_calculator")) },
        { label: "Clip Raster…", action: () => run(() => onRunAlgorithm?.("raster_clip")) },
        { label: "Reproject Raster…", action: () => run(() => onRunAlgorithm?.("raster_reproject")) },
        { label: "Resample Raster…", action: () => run(() => onRunAlgorithm?.("raster_resample")) },
        { type: "sep" },
        { label: "Hillshade…", action: () => run(() => onRunAlgorithm?.("raster_hillshade")) },
        { label: "Contours…", action: () => run(() => onRunAlgorithm?.("raster_contours")) },
      ],
    },
    {
      id: "processing",
      label: "Processing",
      items: [
        { label: "Open Processing Toolbox", shortcut: shortcutFor("processing_toolbox"), action: () => run(onOpenProcessingToolbox) },
        { label: "Search Algorithms…", action: () => run(onSearchProcessing) },
        { label: "Recent Operations", action: () => run(onOpenProcessingHistory) },
      ],
    },
    {
      id: "settings",
      label: "Settings",
      items: [
        {
          label: "Show Base Map Labels",
          checked: mapViewPrefs?.show_labels !== false,
          action: () => run(() => onChangeMapViewPrefs?.({ show_labels: mapViewPrefs?.show_labels === false })),
        },
        {
          label: "Show Base Map Roads",
          checked: mapViewPrefs?.show_roads !== false,
          action: () => run(() => onChangeMapViewPrefs?.({ show_roads: mapViewPrefs?.show_roads === false })),
        },
        {
          label: "Show Base Map Places",
          checked: mapViewPrefs?.show_places !== false,
          action: () => run(() => onChangeMapViewPrefs?.({ show_places: mapViewPrefs?.show_places === false })),
        },
        { type: "sep" },
        { label: "CRS Settings…", shortcut: shortcutFor("crs_manager"), action: () => run(onOpenCRS) },
        { label: "Keyboard Shortcuts…", shortcut: shortcutFor("shortcuts_help"), action: () => run(onOpenShortcuts) },
        { label: "Reset Panel Layout", action: () => run(onResetLayout) },
      ],
    },
    {
      id: "help",
      label: "Help",
      items: [
        { label: "Workspace Guide & Tool Reference", action: () => run(onOpenGuide) },
        { label: "Keyboard Shortcuts", shortcut: shortcutFor("shortcuts_help"), action: () => run(onOpenShortcuts) },
      ],
    },
  ]

  return (
    <div className="shrink-0 bg-neutral-100 dark:bg-[#1e1e1e] border-b border-neutral-300 dark:border-[#2f2f2f] select-none z-40">
      {/* Row 1: Compact Desktop Title Strip */}
      <div className="h-9 px-2.5 flex items-center justify-between gap-2 border-b border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1a1a1a]">
        <div className="flex items-center gap-2 min-w-0">
          <SidebarTrigger className="size-7 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] rounded-[4px]" />
          <div className="w-5 h-5 rounded-[4px] bg-[#3ecf8e] text-[#141414] font-extrabold text-[11px] flex items-center justify-center shrink-0">
            V
          </div>
          <span className="text-[11px] font-semibold text-neutral-500 dark:text-neutral-400 hidden md:inline">
            Veracity Maps
          </span>
          <ChevronRight className="size-3 text-neutral-400 hidden md:inline shrink-0" />
          <h1 className="text-xs font-bold text-neutral-900 dark:text-neutral-100 truncate">
            {workspaceName}
          </h1>
          {revisionNumber && (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold bg-[#3ecf8e]/12 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/25 shrink-0">
              v{revisionNumber}
            </span>
          )}
          {lockHeld && (
            <span
              className="hidden sm:inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 shrink-0"
              title="Exclusive Editing Lock Active"
            >
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
              Editing
            </span>
          )}
          {isLockedByOther && (
            <span
              className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/25 shrink-0"
              title={`Locked by ${lockHolderName}`}
            >
              Read-Only ({lockHolderName})
            </span>
          )}
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          <div className="hidden sm:flex items-center gap-1 px-2 py-0.5 rounded text-[11px] text-neutral-500 dark:text-neutral-400">
            {saveState === "saving" ? (
              <Loader2 className="size-3 animate-spin text-[#3ecf8e]" />
            ) : saveState === "failed" ? (
              <AlertCircle className="size-3 text-red-500" />
            ) : (
              <Check className="size-3 text-[#3ecf8e]" />
            )}
            <span>{saveState === "saving" ? "Saving…" : saveState === "failed" ? "Save failed" : "Saved"}</span>
            {saveState === "failed" && (
              <button
                type="button"
                onClick={onRetrySave}
                className="ml-1 inline-flex items-center gap-0.5 text-red-500 hover:underline cursor-pointer"
              >
                <RefreshCw className="size-2.5" /> Retry
              </button>
            )}
          </div>

          <button
            type="button"
            onClick={onSave}
            disabled={isLockedByOther}
            className="h-6 px-2.5 rounded-[4px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] text-[11px] font-bold inline-flex items-center gap-1 disabled:opacity-40 cursor-pointer"
            title="Save Workspace"
          >
            <Bookmark className="size-3" />
            <span>Save</span>
          </button>

          {onShare && (
            <button
              type="button"
              onClick={onShare}
              className="h-6 px-2 rounded-[4px] border border-neutral-200 dark:border-[#323232] bg-neutral-50 dark:bg-[#252525] text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#303030] text-[11px] font-medium inline-flex items-center gap-1 cursor-pointer"
              title="Share Workspace"
            >
              <Share2 className="size-3 text-[#3ecf8e]" />
              <span className="hidden sm:inline">Share</span>
            </button>
          )}

          <ThemeToggle />

          <div className="h-3.5 w-px bg-neutral-200 dark:bg-[#323232] mx-0.5" />

          <div
            className="hidden md:flex items-center gap-1.5 px-1.5 text-[11px] text-neutral-600 dark:text-neutral-300"
            title={user?.email || user?.name || "Signed-in user"}
          >
            <span className="size-5 rounded-full bg-neutral-200 dark:bg-[#303030] text-[10px] font-bold flex items-center justify-center">
              {(user?.name || user?.email || "U")[0].toUpperCase()}
            </span>
            <span className="max-w-[110px] truncate">{user?.name || user?.email}</span>
          </div>

          <Link
            to="/dashboard"
            onClick={(e) => {
              if (onCloseWorkspace) {
                e.preventDefault()
                onCloseWorkspace()
              }
            }}
            className="h-6 px-2 rounded-[4px] border border-neutral-200 dark:border-[#323232] bg-neutral-50 dark:bg-[#252525] text-neutral-600 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#303030] text-[11px] font-medium inline-flex items-center gap-1 cursor-pointer"
            title="Exit Workspace and return to Dashboard"
          >
            <LogOut className="size-3" />
            <span className="hidden sm:inline">Exit</span>
          </Link>
        </div>
      </div>

      {/* Row 2: Desktop GIS Top Menu Bar */}
      <nav
        ref={barRef}
        aria-label="Workspace Menu Bar"
        className="h-8 px-2.5 flex items-center gap-1 text-[12px] bg-neutral-100 dark:bg-[#232323]"
      >
        {menus.map((menu) => {
          const isOpen = openMenu === menu.id
          return (
            <div key={menu.id} className="relative">
              <button
                type="button"
                onClick={() => setOpenMenu(isOpen ? null : menu.id)}
                onMouseEnter={() => {
                  if (openMenu && openMenu !== menu.id) setOpenMenu(menu.id)
                }}
                className={`h-6 px-2.5 rounded-[4px] font-medium transition-colors cursor-pointer ${
                  isOpen
                    ? "bg-[#3ecf8e] text-[#141414]"
                    : "text-neutral-700 dark:text-neutral-200 hover:bg-neutral-200 dark:hover:bg-[#323232]"
                }`}
              >
                <span className="text-[12px]">{menu.label}</span>
              </button>

              {isOpen && (
                <div
                  role="menu"
                  className="absolute left-0 top-full mt-0.5 min-w-[235px] py-1 rounded-[5px] border border-neutral-300 dark:border-[#3a3a3a] bg-white dark:bg-[#242424] shadow-xl z-50"
                >
                  {menu.items.map((item, idx) => {
                    if (item.type === "sep") {
                      return <div key={`sep-${idx}`} className="my-1 border-t border-neutral-200 dark:border-[#343434]" />
                    }
                    return (
                      <button
                        key={item.label}
                        type="button"
                        role="menuitem"
                        disabled={item.disabled}
                        onClick={item.action}
                        className={`w-full px-3 py-1.5 text-left text-[11.5px] flex items-center justify-between gap-4 transition-colors ${
                          item.disabled
                            ? "opacity-40 cursor-not-allowed text-neutral-500"
                            : item.danger
                              ? "text-red-600 dark:text-red-400 hover:bg-red-500/10 cursor-pointer"
                              : "text-neutral-800 dark:text-neutral-100 hover:bg-[#3ecf8e]/15 cursor-pointer"
                        }`}
                      >
                        <span className="flex items-center gap-2 min-w-0">
                          <span className="w-3.5 inline-flex items-center justify-center shrink-0">
                            {item.checked && <Check className="size-3 text-[#3ecf8e]" />}
                          </span>
                          <span className="truncate">{item.label}</span>
                        </span>
                        {item.shortcut && (
                          <kbd className="text-[10px] font-mono text-neutral-400 dark:text-neutral-400 shrink-0">
                            {item.shortcut}
                          </kbd>
                        )}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
      </nav>
    </div>
  )
}
