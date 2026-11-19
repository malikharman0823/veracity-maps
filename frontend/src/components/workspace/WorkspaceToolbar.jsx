import React, { useMemo } from "react"
import {
  Save,
  Download,
  Plus,
  MousePointer,
  ZoomIn,
  ZoomOut,
  Maximize2,
  Crosshair,
  BookmarkPlus,
  Info,
  Database,
  Table2,
  MapPin,
  Spline,
  Pentagon,
  Move,
  Edit3,
  Magnet,
  Undo2,
  Redo2,
  Trash2,
  Ruler,
  Scan,
  Compass,
  Globe,
  Cpu,
  PanelLeft,
  PanelRight,
  Keyboard,
} from "lucide-react"
import { DEFAULT_SHORTCUTS, formatShortcutForOS } from "../../lib/shortcutRegistry"

function ToolBtn({
  icon: Icon,
  label,
  shortcut = "",
  active = false,
  disabled = false,
  danger = false,
  onClick,
  compact = false,
}) {
  const titleText = shortcut ? `${label} (${shortcut})` : label
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={titleText}
      aria-label={titleText}
      aria-pressed={active}
      className={`h-7 ${compact ? "px-1.5" : "w-7"} rounded-[4px] flex items-center justify-center gap-1 transition-colors shrink-0 ${
        disabled
          ? "opacity-35 cursor-not-allowed text-neutral-500"
          : active
            ? "bg-[#3ecf8e] text-[#141414] shadow-2xs cursor-pointer"
            : danger
              ? "text-neutral-600 dark:text-neutral-300 hover:bg-red-500/15 hover:text-red-500 cursor-pointer"
              : "text-neutral-700 dark:text-neutral-200 hover:bg-neutral-200 dark:hover:bg-[#333333] cursor-pointer"
      }`}
    >
      <Icon className="size-3.5 shrink-0" />
    </button>
  )
}

function Sep() {
  return <div className="h-4 w-px bg-neutral-300 dark:bg-[#383838] mx-1 shrink-0" />
}

export default function WorkspaceToolbar({
  shortcuts = DEFAULT_SHORTCUTS,
  activeTool = "select",
  measureMode = null, // null | "distance" | "area"
  snapEnabled = false,
  canUndo = false,
  canRedo = false,
  hasSelection = false,
  hasActiveLayer = false,
  hasGeoreferenceableRaster = false,
  isLockedByOther = false,
  layout,
  mapViews = [],
  mapViewPrefs = {},
  activeBaseLayer = null,
  // Actions
  onSave,
  onAddDataset,
  onExport,
  onSelectTool,
  onZoomIn,
  onZoomOut,
  onZoomFullExtent,
  onZoomToActiveLayer,
  onAddBookmark,
  onToggleRecords,
  onOpenAttributeTable,
  onToggleSnap,
  onUndo,
  onRedo,
  onDeleteSelected,
  onToggleMeasure,
  onGeoreferenceActive,
  onSelectMapView,
  onOpenBaseLayerModal,
  onOpenCRSModal,
  onToggleLeftDock,
  onToggleRightDock,
  onOpenShortcuts,
}) {
  const shortcutFor = useMemo(() => {
    const m = new Map()
    ;(shortcuts || DEFAULT_SHORTCUTS).forEach((s) => {
      if (s?.tool_id && s?.shortcut) {
        m.set(s.tool_id, formatShortcutForOS(s.shortcut))
      }
    })
    return (id) => m.get(id) || ""
  }, [shortcuts])

  const currentViewType = mapViewPrefs?.view_type || "world"

  return (
    <div
      role="toolbar"
      aria-label="Workspace GIS Toolbars"
      className="shrink-0 min-h-9 px-2 py-1 bg-neutral-50 dark:bg-[#262626] border-b border-neutral-300 dark:border-[#343434] flex items-center gap-0.5 overflow-x-auto select-none z-30"
    >
      {/* 1. Workspace Operations */}
      <ToolBtn icon={Save} label="Save Workspace" onClick={onSave} disabled={isLockedByOther} />
      <ToolBtn icon={Plus} label="Add Dataset (Vector / Raster / Base)" shortcut={shortcutFor("add_dataset")} onClick={onAddDataset} />
      <ToolBtn icon={Download} label="Export Workspace" onClick={onExport} />

      <Sep />

      {/* 2. Map Navigation */}
      <ToolBtn
        icon={MousePointer}
        label="Select & Pan"
        shortcut={shortcutFor("select")}
        active={!measureMode && activeTool === "select"}
        onClick={() => onSelectTool?.("select")}
      />
      <ToolBtn icon={ZoomIn} label="Zoom In" onClick={onZoomIn} />
      <ToolBtn icon={ZoomOut} label="Zoom Out" onClick={onZoomOut} />
      <ToolBtn icon={Maximize2} label="Zoom to Full Extent" shortcut={shortcutFor("zoom_full")} onClick={onZoomFullExtent} />
      <ToolBtn icon={Crosshair} label="Zoom to Active Layer" onClick={onZoomToActiveLayer} disabled={!hasActiveLayer} />
      <ToolBtn icon={BookmarkPlus} label="Save Spatial Bookmark" onClick={onAddBookmark} />

      <Sep />

      {/* 3. Selection & Inspection */}
      <ToolBtn
        icon={Info}
        label="Identify Feature"
        shortcut={shortcutFor("identify")}
        active={!measureMode && activeTool === "identify"}
        onClick={() => onSelectTool?.("identify")}
      />
      <ToolBtn
        icon={Database}
        label="Workspace Records Table"
        shortcut={shortcutFor("attribute_table")}
        active={Boolean(layout?.recordsOpen)}
        onClick={onToggleRecords}
      />
      <ToolBtn
        icon={Table2}
        label="Active Layer Attribute Table"
        onClick={onOpenAttributeTable}
        disabled={!hasActiveLayer}
      />

      <Sep />

      {/* 4. Vector Editing */}
      <ToolBtn
        icon={MapPin}
        label="Add Point Feature"
        shortcut={shortcutFor("point")}
        active={!measureMode && activeTool === "point"}
        onClick={() => onSelectTool?.("point")}
      />
      <ToolBtn
        icon={Spline}
        label="Draw Line Feature"
        shortcut={shortcutFor("line")}
        active={!measureMode && activeTool === "line"}
        onClick={() => onSelectTool?.("line")}
      />
      <ToolBtn
        icon={Pentagon}
        label="Draw Polygon Feature"
        shortcut={shortcutFor("polygon")}
        active={!measureMode && activeTool === "polygon"}
        onClick={() => onSelectTool?.("polygon")}
      />
      <ToolBtn
        icon={Move}
        label="Move Feature"
        shortcut={shortcutFor("move")}
        active={!measureMode && activeTool === "move"}
        onClick={() => onSelectTool?.("move")}
      />
      <ToolBtn
        icon={Edit3}
        label="Edit Vertices (Shift = Proportional Scale)"
        shortcut={shortcutFor("vertex")}
        active={!measureMode && activeTool === "vertex"}
        onClick={() => onSelectTool?.("vertex")}
      />
      <ToolBtn
        icon={Magnet}
        label="Toggle Vertex Snapping"
        shortcut={shortcutFor("snap")}
        active={snapEnabled}
        onClick={onToggleSnap}
      />
      <ToolBtn icon={Undo2} label="Undo Edit" shortcut={shortcutFor("undo")} onClick={onUndo} disabled={!canUndo} />
      <ToolBtn icon={Redo2} label="Redo Edit" shortcut={shortcutFor("redo")} onClick={onRedo} disabled={!canRedo} />
      <ToolBtn
        icon={Trash2}
        label="Delete Selected Features"
        shortcut={shortcutFor("delete_selected")}
        onClick={onDeleteSelected}
        disabled={!hasSelection}
        danger
      />

      <Sep />

      {/* 5. Measurement */}
      <ToolBtn
        icon={Ruler}
        label="Measure Distance (Geodesic)"
        active={measureMode === "distance"}
        onClick={() => onToggleMeasure?.("distance")}
      />
      <ToolBtn
        icon={Scan}
        label="Measure Area (Geodesic)"
        active={measureMode === "area"}
        onClick={() => onToggleMeasure?.("area")}
      />

      <Sep />

      {/* 6. Georeferencing & CRS */}
      <button
        type="button"
        onClick={onGeoreferenceActive}
        disabled={!hasGeoreferenceableRaster}
        title={
          hasGeoreferenceableRaster
            ? "Open Georeferencer for active raster"
            : "Add or select a raster layer to georeference"
        }
        className="h-7 px-2 rounded-[4px] border border-neutral-300 dark:border-[#3a3a3a] bg-white dark:bg-[#1f1f1f] text-[11px] font-medium text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#323232] disabled:opacity-40 disabled:cursor-not-allowed inline-flex items-center gap-1 shrink-0 cursor-pointer"
      >
        <Crosshair className="size-3 text-[#3ecf8e]" />
        <span className="hidden xl:inline">Georeference</span>
      </button>
      <ToolBtn icon={Globe} label="Coordinate System (CRS)" shortcut={shortcutFor("crs_manager")} onClick={onOpenCRSModal} />

      <Sep />

      {/* 7. Base Layer & Map View Switcher */}
      <div className="flex items-center gap-1 shrink-0">
        <select
          aria-label="Map View Mode"
          title="Switch Base Map View"
          value={currentViewType}
          onChange={(e) => {
            const val = e.target.value
            if (val === "custom" && (!activeBaseLayer || activeBaseLayer.is_default)) {
              onOpenBaseLayerModal?.()
              return
            }
            onSelectMapView?.(val)
          }}
          className="h-7 px-2 rounded-[4px] border border-neutral-300 dark:border-[#3a3a3a] bg-white dark:bg-[#1f1f1f] text-[11px] font-medium text-neutral-800 dark:text-neutral-100 outline-none focus:border-[#3ecf8e] cursor-pointer"
        >
          {(mapViews.length > 0
            ? mapViews
            : [
                { type: "world", name: "World Map" },
                { type: "satellite", name: "Satellite" },
                { type: "streets", name: "Streets" },
                { type: "terrain", name: "Terrain" },
                { type: "light", name: "Light" },
                { type: "dark", name: "Dark" },
                { type: "custom", name: "Custom Base" },
              ]
          ).map((mv, idx) => {
            const vType = mv.type || mv.view_type || `view_${idx}`
            return (
              <option key={vType} value={vType}>
                {vType === "custom" && activeBaseLayer && !activeBaseLayer.is_default
                  ? `Custom: ${activeBaseLayer.name}`
                  : mv.name}
              </option>
            )
          })}
        </select>
        <ToolBtn
          icon={Compass}
          label="Base Layer Selector"
          shortcut={shortcutFor("base_layer")}
          onClick={onOpenBaseLayerModal}
        />
      </div>

      {/* Right-aligned Dock Panel Toggles & Shortcuts */}
      <div className="ml-auto flex items-center gap-0.5 pl-2 shrink-0">
        <ToolBtn
          icon={PanelLeft}
          label="Toggle Left Panels (Browser & Layers)"
          shortcut={shortcutFor("layer_manager")}
          active={Boolean(layout?.leftOpen)}
          onClick={onToggleLeftDock}
        />
        <ToolBtn
          icon={Cpu}
          label="Toggle Processing Toolbox"
          shortcut={shortcutFor("processing_toolbox")}
          active={Boolean(layout?.rightOpen)}
          onClick={onToggleRightDock}
        />
        <ToolBtn
          icon={Keyboard}
          label="Keyboard Shortcuts"
          shortcut={shortcutFor("shortcuts_help")}
          onClick={onOpenShortcuts}
        />
      </div>
    </div>
  )
}
