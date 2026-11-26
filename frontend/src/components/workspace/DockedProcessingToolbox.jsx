import React from "react"
import {
  Sparkles,
  SlidersHorizontal,
  Eraser,
  Contrast,
  MousePointer2,
  MapPin,
  Minus,
  Pentagon,
  Edit3,
  Trash2,
  Ruler,
  Square,
  Crosshair,
  Compass,
  Table2,
} from "lucide-react"

/**
 * Contextual 4-Category Processing Toolbox (Raster, Vector, Measurement, Georeferencing).
 * Contour/terrain, 3D, and complex topology/overlay tools are intentionally removed.
 */
export default function DockedProcessingToolbox({
  activeLayer,
  datasets = [],
  activeTool = "select",
  measureMode = null,
  onSelectVectorTool,
  onToggleMeasure,
  onOpenEnhance,
  onOpenGeoreference,
  onOpenAttributeTable,
}) {
  const isRasterSelected = activeLayer?.layer_type === "raster" || (!activeLayer && datasets.length > 0)
  const targetRaster =
    (activeLayer?.layer_type === "raster" ? activeLayer : null) || datasets[0] || null

  const ToolCard = ({ icon: Icon, label, desc, active, onClick, disabled }) => (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`flex items-start gap-2.5 p-2.5 rounded-[6px] border text-left transition-colors cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed ${
        active
          ? "border-[#3ecf8e] bg-[#3ecf8e]/10 text-neutral-900 dark:text-white"
          : "border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1e1e1e] hover:border-[#3ecf8e]/60 text-neutral-800 dark:text-neutral-200"
      }`}
    >
      <Icon className="size-4 text-[#24b47e] dark:text-[#3ecf8e] shrink-0 mt-0.5" />
      <div className="min-w-0">
        <div className="text-xs font-medium leading-tight">{label}</div>
        {desc && <div className="text-[10.5px] text-neutral-500 dark:text-neutral-400 mt-0.5">{desc}</div>}
      </div>
    </button>
  )

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 p-3 overflow-y-auto h-full">
      {/* 1. Raster Processing (4 essential operations only) */}
      <div className="space-y-2">
        <div className="text-[11px] font-semibold text-neutral-600 dark:text-neutral-300">
          Raster
        </div>
        <div className="grid grid-cols-1 gap-1.5">
          <ToolCard
            icon={Sparkles}
            label="AI Upscaling"
            desc="2× / 4× super-resolution"
            disabled={!targetRaster}
            onClick={() => targetRaster && onOpenEnhance?.(targetRaster, "upscale")}
          />
          <ToolCard
            icon={Contrast}
            label="Histogram Equalization"
            desc="Adaptive contrast (CLAHE)"
            disabled={!targetRaster}
            onClick={() => targetRaster && onOpenEnhance?.(targetRaster, "clahe")}
          />
          <ToolCard
            icon={SlidersHorizontal}
            label="Brightness & Contrast"
            desc="Quick tonal adjustment"
            disabled={!targetRaster}
            onClick={() => targetRaster && onOpenEnhance?.(targetRaster, "contrast")}
          />
          <ToolCard
            icon={Eraser}
            label="Background Removal"
            desc="Make background transparent"
            disabled={!targetRaster}
            onClick={() => targetRaster && onOpenEnhance?.(targetRaster, "background")}
          />
        </div>
      </div>

      {/* 2. Vector Editing */}
      <div className="space-y-2">
        <div className="text-[11px] font-semibold text-neutral-600 dark:text-neutral-300">
          Vector
        </div>
        <div className="grid grid-cols-2 gap-1.5">
          <ToolCard
            icon={MousePointer2}
            label="Select"
            active={activeTool === "select"}
            onClick={() => onSelectVectorTool?.("select")}
          />
          <ToolCard
            icon={MapPin}
            label="Add Point"
            active={activeTool === "point"}
            onClick={() => onSelectVectorTool?.("point")}
          />
          <ToolCard
            icon={Minus}
            label="Draw Line"
            active={activeTool === "line"}
            onClick={() => onSelectVectorTool?.("line")}
          />
          <ToolCard
            icon={Pentagon}
            label="Draw Polygon"
            active={activeTool === "polygon"}
            onClick={() => onSelectVectorTool?.("polygon")}
          />
          <ToolCard
            icon={Edit3}
            label="Edit Vertices"
            active={activeTool === "vertex"}
            onClick={() => onSelectVectorTool?.("vertex")}
          />
          <ToolCard
            icon={Trash2}
            label="Delete"
            active={activeTool === "delete"}
            onClick={() => onSelectVectorTool?.("delete")}
          />
        </div>
        {activeLayer && (!activeLayer.layer_type || activeLayer.layer_type === "vector") && (
          <button
            type="button"
            onClick={() => onOpenAttributeTable?.(activeLayer)}
            className="w-full flex items-center justify-center gap-1.5 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#303030] hover:border-[#3ecf8e] text-xs font-medium text-neutral-700 dark:text-neutral-200 cursor-pointer"
          >
            <Table2 className="size-3.5 text-[#3ecf8e]" /> Open Attribute Table
          </button>
        )}
      </div>

      {/* 3. Measurement */}
      <div className="space-y-2">
        <div className="text-[11px] font-semibold text-neutral-600 dark:text-neutral-300">
          Measurement
        </div>
        <div className="grid grid-cols-1 gap-1.5">
          <ToolCard
            icon={Ruler}
            label="Distance"
            desc="Measure geodesic line distance"
            active={measureMode === "distance"}
            onClick={() => onToggleMeasure?.("distance")}
          />
          <ToolCard
            icon={Square}
            label="Area"
            desc="Measure polygon area"
            active={measureMode === "area"}
            onClick={() => onToggleMeasure?.("area")}
          />
          <ToolCard
            icon={Crosshair}
            label="Coordinates"
            desc="Inspect lat/lng coordinates"
            active={measureMode === "coordinates"}
            onClick={() => onToggleMeasure?.("coordinates")}
          />
        </div>
      </div>

      {/* 4. Georeferencing */}
      <div className="space-y-2">
        <div className="text-[11px] font-semibold text-neutral-600 dark:text-neutral-300">
          Georeferencing
        </div>
        <div className="grid grid-cols-1 gap-1.5">
          <ToolCard
            icon={Compass}
            label="Georeference Raster"
            desc="Place up to 20 control points, preview & apply"
            disabled={!targetRaster}
            onClick={() => targetRaster && onOpenGeoreference?.(targetRaster)}
          />
        </div>
        {isRasterSelected && targetRaster && (
          <div className="text-[10.5px] text-neutral-500 dark:text-neutral-400 px-1">
            Active raster: <span className="font-medium text-neutral-800 dark:text-neutral-200">{targetRaster.name || targetRaster.layer_name || targetRaster.dataset_id}</span>
          </div>
        )}
      </div>
    </div>
  )
}
