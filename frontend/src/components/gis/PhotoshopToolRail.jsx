import React, { useMemo } from "react"
import {
  Map as MapIcon,
  Database,
  MousePointer,
  Info,
  MapPin,
  Spline,
  Pentagon,
  Move,
  Edit3,
  Magnet,
  Undo2,
  Redo2,
  Table2,
  Palette,
  Globe,
  Layers,
  Cpu,
  Plus,
  Maximize2,
  PanelRight,
  Compass,
  Keyboard,
} from "lucide-react"
import ToolHoverPreview from "./ToolHoverPreview"
import { DEFAULT_SHORTCUTS, formatShortcutForOS } from "../../lib/shortcutRegistry"

const VECTOR_TOOLS = [
  {
    id: "select",
    shortcutId: "select",
    label: "Select Tool",
    icon: MousePointer,
    previewKey: "select",
    requiresVectorLayer: false,
    description: "Click any point, line, or shape on the map to select it.",
  },
  {
    id: "identify",
    shortcutId: "identify",
    label: "Identify Feature",
    icon: Info,
    previewKey: "select",
    requiresVectorLayer: false,
    description: "Click any visible vector feature on the map to inspect its source layer, geometry type, and attributes.",
  },
  {
    id: "point",
    shortcutId: "point",
    label: "Place Point",
    icon: MapPin,
    previewKey: "point",
    requiresVectorLayer: true,
    description: "Click anywhere on the map to drop a new point marker, such as a building, station, or landmark.",
  },
  {
    id: "line",
    shortcutId: "line",
    label: "Draw Line",
    icon: Spline,
    previewKey: "line",
    requiresVectorLayer: true,
    description: "Click along a path to draw roads, pipes, or boundaries. Press Confirm when you are done.",
  },
  {
    id: "polygon",
    shortcutId: "polygon",
    label: "Draw Polygon",
    icon: Pentagon,
    previewKey: "polygon",
    requiresVectorLayer: true,
    description: "Click corners on the map to outline an area like a ward, building plot, or zone.",
  },
  {
    id: "move",
    shortcutId: "move",
    label: "Move Feature",
    icon: Move,
    previewKey: "move",
    requiresVectorLayer: true,
    description: "Drag a selected shape or point to reposition it accurately on the map.",
  },
  {
    id: "vertex",
    shortcutId: "vertex",
    label: "Edit Vertices",
    icon: Edit3,
    previewKey: "vertex",
    requiresVectorLayer: true,
    description: "Drag vertices to reshape or resize geometries (hold Shift to scale proportionally).",
  },
]

export default function PhotoshopToolRail({
  view,
  onSetView,
  activeGISLayer,
  activeTool = "select",
  onSelectTool,
  snapEnabled = false,
  onToggleSnap,
  canUndo = false,
  canRedo = false,
  onUndo,
  onRedo,
  onOpenAttributeTable,
  onOpenStyleEditor,
  onOpenCRSModal,
  onOpenDatasetBrowser,
  onOpenProcessingToolbox,
  onOpenAddDataset,
  onToggleLayerManager,
  onOpenBaseLayerModal,
  onZoomFullExtent,
  onOpenShortcutsModal,
  shortcuts = DEFAULT_SHORTCUTS,
}) {
  const hasVectorLayer = activeGISLayer?.layer_type === "vector"

  const shortcutFor = useMemo(() => {
    const map = new Map()
    ;(shortcuts || DEFAULT_SHORTCUTS).forEach((entry) => {
      if (entry?.tool_id && entry?.shortcut) {
        map.set(entry.tool_id, formatShortcutForOS(entry.shortcut))
      }
    })
    return (toolId) => map.get(toolId) || ""
  }, [shortcuts])

  return (
    <aside
      aria-label="Photoshop GIS Tool Rail"
      className="w-12 shrink-0 bg-neutral-100 dark:bg-[#232323] border-r border-neutral-200 dark:border-[#323232] flex flex-col items-center py-2 gap-1 z-30 select-none overflow-y-auto"
    >
      {/* View Switcher */}
      <ToolHoverPreview
        title="Map Canvas View"
        shortcut={shortcutFor("view_map")}
        previewKey="map"
        description="View your interactive map with base layers, rasters, and vector GIS layers."
      >
        <button
          type="button"
          onClick={() => onSetView("map")}
          className={`size-8 rounded-[5px] flex items-center justify-center transition-colors cursor-pointer ${
            view === "map"
              ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
              : "text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232]"
          }`}
          aria-label="Map Canvas View"
        >
          <MapIcon className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="Workspace Records"
        shortcut={shortcutFor("view_records")}
        previewKey="database"
        description="Open the centralized Workspace Records table across vectors, rasters, control points, and user icons."
      >
        <button
          type="button"
          onClick={() => onSetView("database")}
          className={`size-8 rounded-[5px] flex items-center justify-center transition-colors cursor-pointer ${
            view === "database"
              ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
              : "text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232]"
          }`}
          aria-label="Workspace Records"
        >
          <Database className="size-4" />
        </button>
      </ToolHoverPreview>

      <div className="w-6 h-[1px] bg-neutral-300 dark:bg-[#363636] my-1 shrink-0" />

      {/* Map Inspection & Vector Editing Tools */}
      {VECTOR_TOOLS.map((tool) => {
        const Icon = tool.icon
        const isActive = view === "map" && activeTool === tool.id
        const dimmed = tool.requiresVectorLayer && !hasVectorLayer
        return (
          <ToolHoverPreview
            key={tool.id}
            title={tool.label}
            shortcut={shortcutFor(tool.shortcutId)}
            previewKey={tool.previewKey}
            description={
              dimmed
                ? `${tool.description} (Create or select a Vector GIS Layer on the right panel first.)`
                : tool.description
            }
          >
            <button
              type="button"
              onClick={() => {
                if (view !== "map") onSetView("map")
                onSelectTool?.(tool.id)
              }}
              className={`size-8 rounded-[5px] flex items-center justify-center transition-colors relative cursor-pointer ${
                isActive
                  ? "bg-[#323232] text-[#3ecf8e] ring-1 ring-[#3ecf8e]/60"
                  : "text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232]"
              } ${dimmed ? "opacity-55" : ""}`}
              aria-label={tool.label}
            >
              <Icon className="size-4" />
              {isActive && (
                <span className="absolute left-0 top-1.5 bottom-1.5 w-[2px] rounded-r bg-[#3ecf8e]" />
              )}
            </button>
          </ToolHoverPreview>
        )
      })}

      {/* Snap, Undo, Redo */}
      <ToolHoverPreview
        title="Snap to Vertices"
        shortcut={shortcutFor("snap")}
        previewKey="snap"
        description="Automatically magnetizes your cursor to nearby corners and edges so shapes connect cleanly with zero gaps."
      >
        <button
          type="button"
          onClick={onToggleSnap}
          className={`size-8 rounded-[5px] flex items-center justify-center transition-colors cursor-pointer ${
            snapEnabled
              ? "bg-sky-500/20 text-sky-400 ring-1 ring-sky-500/50"
              : "text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232]"
          }`}
          aria-label="Toggle Snapping"
        >
          <Magnet className="size-4" />
        </button>
      </ToolHoverPreview>

      <div className="flex items-center gap-0.5">
        <ToolHoverPreview
          title="Undo Last Edit"
          shortcut={shortcutFor("undo")}
          previewKey="undo"
          description="Step backward and undo the last shape you added or moved."
        >
          <button
            type="button"
            disabled={!canUndo}
            onClick={onUndo}
            className="size-7 rounded-[4px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] disabled:opacity-35 cursor-pointer"
            aria-label="Undo"
          >
            <Undo2 className="size-3.5" />
          </button>
        </ToolHoverPreview>
        <ToolHoverPreview
          title="Redo Edit"
          shortcut={shortcutFor("redo")}
          previewKey="redo"
          description="Bring back an edit that you just undid."
        >
          <button
            type="button"
            disabled={!canRedo}
            onClick={onRedo}
            className="size-7 rounded-[4px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] disabled:opacity-35 cursor-pointer"
            aria-label="Redo"
          >
            <Redo2 className="size-3.5" />
          </button>
        </ToolHoverPreview>
      </div>

      <div className="w-6 h-[1px] bg-neutral-300 dark:bg-[#363636] my-1 shrink-0" />

      {/* Navigation, Panels & GIS Studio Tools */}
      <ToolHoverPreview
        title="Zoom to Full Extent"
        shortcut={shortcutFor("zoom_full")}
        previewKey="map"
        description="Fit the map camera to the full extent of all visible vector, raster, and custom base layers."
      >
        <button
          type="button"
          onClick={onZoomFullExtent}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="Zoom to Full Extent"
        >
          <Maximize2 className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="Layer Manager"
        shortcut={shortcutFor("layer_manager")}
        previewKey="datasets"
        description="Focus or toggle the Layers panel to manage Vector, Raster, and Base Layer visibility and order."
      >
        <button
          type="button"
          onClick={onToggleLayerManager}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="Layer Manager"
        >
          <PanelRight className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="Base Layer / Map View"
        shortcut={shortcutFor("base_layer")}
        previewKey="map"
        description="Switch between the Default World Map, Satellite/Terrain/Streets views, or your Custom Base Layers."
      >
        <button
          type="button"
          onClick={onOpenBaseLayerModal}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="Base Layer Selector"
        >
          <Compass className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="Attribute Table"
        shortcut={shortcutFor("attribute_table")}
        previewKey="attributes"
        description="Open a spreadsheet of all features in your active layer to search, filter, edit attributes, or export to CSV."
      >
        <button
          type="button"
          onClick={onOpenAttributeTable}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="Attribute Table"
        >
          <Table2 className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="Symbology & Styling"
        shortcut={shortcutFor("symbology")}
        previewKey="symbology"
        description="Customize fill colors, border thickness, point sizes, and text labels for your map layers."
      >
        <button
          type="button"
          onClick={onOpenStyleEditor}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="Symbology & Styling"
        >
          <Palette className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="Coordinate System (CRS)"
        shortcut={shortcutFor("crs_manager")}
        previewKey="crs"
        description="Check or change the map projection (such as EPSG:4326 or UTM) and reproject datasets accurately."
      >
        <button
          type="button"
          onClick={onOpenCRSModal}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="CRS Manager"
        >
          <Globe className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="Dataset Browser"
        shortcut={shortcutFor("dataset_browser")}
        previewKey="datasets"
        description="Browse all your uploaded maps, GeoTIFFs, Shapefiles, and GeoPackages to preview or add them here."
      >
        <button
          type="button"
          onClick={onOpenDatasetBrowser}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="Dataset Browser"
        >
          <Layers className="size-4" />
        </button>
      </ToolHoverPreview>

      <ToolHoverPreview
        title="GIS Processing Toolbox"
        shortcut={shortcutFor("processing_toolbox")}
        previewKey="processing"
        description="Run spatial tools like Buffer Zones, Clip, Intersection, Union, Spatial Join, Hillshade, and Contours."
      >
        <button
          type="button"
          onClick={onOpenProcessingToolbox}
          className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
          aria-label="Processing Toolbox"
        >
          <Cpu className="size-4" />
        </button>
      </ToolHoverPreview>

      <div className="mt-auto pt-1 flex flex-col items-center gap-1">
        <ToolHoverPreview
          title="Keyboard Shortcuts"
          shortcut={shortcutFor("shortcuts_help")}
          previewKey="default"
          description="View and customize all keyboard shortcuts for sidebar tools and workspace panels."
        >
          <button
            type="button"
            onClick={onOpenShortcutsModal}
            className="size-8 rounded-[5px] flex items-center justify-center text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
            aria-label="Keyboard Shortcuts"
          >
            <Keyboard className="size-4" />
          </button>
        </ToolHoverPreview>

        <ToolHoverPreview
          title="Add Dataset"
          shortcut={shortcutFor("add_dataset")}
          previewKey="add_dataset"
          description="Open the Unified Upload dialog to add Vector Layers (GeoPackage, Shapefile, GeoJSON), Raster Layers, or a Custom Base Layer."
        >
          <button
            type="button"
            onClick={onOpenAddDataset}
            className="size-8 rounded-[5px] flex items-center justify-center bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] hover:bg-[#3ecf8e] hover:text-[#171717] transition-colors cursor-pointer"
            aria-label="Add Dataset"
          >
            <Plus className="size-4" />
          </button>
        </ToolHoverPreview>
      </div>
    </aside>
  )
}
