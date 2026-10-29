import React, { useMemo, useState, useEffect } from "react"
import {
  Download,
  X,
  Building2,
  ChevronDown,
  ChevronRight,
  Layers,
  Image,
  Database,
  MapPin,
  Check,
  Minus,
  Loader2,
  AlertCircle,
  FileText,
  RotateCw,
  Sparkles,
  Sliders,
  ZoomIn,
  RotateCcw,
  Map,
  Crosshair,
  Archive,
  Info,
  CheckCircle2,
  XCircle,
  ArrowRight,
  HelpCircle,
  Eye,
  ShieldAlert,
} from "lucide-react"
import { datasetSourceImageUrl } from "../api"
import { buildExportHierarchy } from "../lib/exportHierarchy"

export const SUGGESTED_ZOOM_LEVELS = [
  { level: 21, label: "House level", preset: "house" },
  { level: 20, label: "Building level", preset: "building" },
  { level: 19, label: "Street level", preset: "street" },
  { level: 18, label: "Neighborhood level", preset: "neighborhood" },
  { level: 17, label: "Local area", preset: "local" },
  { level: 16, label: "District level", preset: "district" },
  { level: 15, label: "City level", preset: "city" },
  { level: 14, label: "Large city area", preset: "large_city" },
  { level: 13, label: "Regional level", preset: "regional" },
  { level: 12, label: "Wide regional view", preset: "wide_regional" },
  { level: 11, label: "State-level overview", preset: "state" },
  { level: 10, label: "Large regional overview", preset: "large_regional" },
  { level: 5, label: "Country-level overview", preset: "country" },
  { level: 1, label: "World overview", preset: "world" },
]

export default function ExportProjectModal({
  open,
  onClose,
  manifest,
  buildings = [],
  datasets = [],
  primaryDatasetId,
  onExport,
  exporting = false,
  error = null,
}) {
  // 1. Compile hierarchy of buildings, floors, and standalone layers dynamically
  const {
    buildings: buildingList,
    standaloneLayers,
    allLayerIds,
  } = useMemo(() => {
    return buildExportHierarchy({
      members: manifest?.members || [],
      datasets: datasets || [],
      buildings: buildings || [],
      primaryDatasetId,
    })
  }, [buildings, datasets, manifest?.members, primaryDatasetId])

  // Top-level Export Type Selection:
  // Option 1: "project_zip" (Viewscape-Ready Project — primary and default)
  // Option 2: "mbtiles" (Single MBTiles File — secondary option)
  // Option 3: "base_layer" (Base Layer Only — reusable offline base map)
  const [exportType, setExportType] = useState("project_zip")

  // Selection state
  const [selectedLayerIds, setSelectedLayerIds] = useState(new Set())
  const [selectedPreviewLayerId, setSelectedPreviewLayerId] = useState(null)
  const [collapsedFloors, setCollapsedFloors] = useState(new Set())

  // Viewscape-Ready Project Inclusion settings
  const [inclusion, setInclusion] = useState({
    include_base_map: true,
    include_raster_imagery: true,
    include_original_rasters: false,
    include_landmarks: true,
    include_control_points: true,
    include_building_config: true,
    include_layer_styles: true,
    include_metadata: true,
  })

  // MBTiles-specific settings
  const [mbtilesIncludeBasemap, setMbtilesIncludeBasemap] = useState(false)

  // Base layer specific settings
  const [selectedBasemapSource, setSelectedBasemapSource] = useState("osm_standard")

  // Viewscape Zoom Configuration state
  const [zoomConfig, setZoomConfig] = useState({
    initial_zoom: 18,
    min_zoom: 12,
    max_zoom: 22,
    preset: "neighborhood",
  })
  const [showZoomGuide, setShowZoomGuide] = useState(false)
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false)
  const [showGuidance, setShowGuidance] = useState(false)

  // Initialize selection and zoom settings when opening
  useEffect(() => {
    if (open) {
      const init = new Set(allLayerIds)
      setSelectedLayerIds(init)
      if (allLayerIds.length > 0) {
        setSelectedPreviewLayerId(primaryDatasetId && allLayerIds.includes(primaryDatasetId) ? primaryDatasetId : allLayerIds[0])
      }

      const primaryDs = datasets.find((d) => d.id === primaryDatasetId)
      const meta = primaryDs?.extracted_metadata
      if (meta && (meta.initial_zoom || meta.minzoom || meta.maxzoom)) {
        setZoomConfig({
          initial_zoom: Number(meta.initial_zoom) || 18,
          min_zoom: Number(meta.minzoom) || 12,
          max_zoom: Number(meta.maxzoom) || 22,
          preset: meta.zoom_preset || "custom",
        })
      }
    }
  }, [open, allLayerIds, primaryDatasetId, datasets])

  const handlePresetChange = (presetKey) => {
    if (presetKey === "auto") {
      setZoomConfig((prev) => ({
        ...prev,
        initial_zoom: 18,
        min_zoom: 12,
        max_zoom: 22,
        preset: "auto",
      }))
      return
    }
    const found = SUGGESTED_ZOOM_LEVELS.find((z) => z.preset === presetKey)
    if (found) {
      setZoomConfig((prev) => {
        const nextInit = found.level
        const nextMin = Math.min(prev.min_zoom, nextInit)
        const nextMax = Math.max(prev.max_zoom, nextInit)
        return {
          ...prev,
          initial_zoom: nextInit,
          min_zoom: nextMin,
          max_zoom: nextMax,
          preset: found.preset,
        }
      })
    } else {
      setZoomConfig((prev) => ({ ...prev, preset: "custom" }))
    }
  }

  const handleResetZoom = () => {
    setZoomConfig({
      initial_zoom: 18,
      min_zoom: 12,
      max_zoom: 22,
      preset: "neighborhood",
    })
  }

  const zoomValidation = useMemo(() => {
    const init = Number(zoomConfig.initial_zoom)
    const min = Number(zoomConfig.min_zoom)
    const max = Number(zoomConfig.max_zoom)
    if (isNaN(min) || min < 0 || min > 24) return "Minimum zoom must be between 0 and 24."
    if (isNaN(max) || max < 0 || max > 24) return "Maximum zoom must be between 0 and 24."
    if (min > max) return "Minimum zoom cannot exceed maximum zoom."
    if (isNaN(init) || init < min || init > max) {
      return `Initial camera zoom (${init}) must be between minimum zoom (${min}) and maximum zoom (${max}).`
    }
    return null
  }, [zoomConfig])

  const currentZoomLabel = useMemo(() => {
    const found = SUGGESTED_ZOOM_LEVELS.find((z) => z.level === Number(zoomConfig.initial_zoom))
    return found ? found.label : "Custom view"
  }, [zoomConfig.initial_zoom])

  if (!open) return null

  // Selection helpers
  const isLayerSelected = (id) => selectedLayerIds.has(id)

  const toggleLayer = (id) => {
    const next = new Set(selectedLayerIds)
    if (next.has(id)) {
      next.delete(id)
    } else {
      next.add(id)
    }
    setSelectedLayerIds(next)
    setSelectedPreviewLayerId(id)
  }

  const selectAll = () => setSelectedLayerIds(new Set(allLayerIds))
  const deselectAll = () => setSelectedLayerIds(new Set())

  // Active layer for preview
  const activePreviewLayer = (() => {
    for (const l of standaloneLayers) {
      if (l.id === selectedPreviewLayerId) return { ...l, floor: null, building: null }
    }
    for (const b of buildingList) {
      for (const fl of b.floors) {
        for (const l of fl.layers) {
          if (l.id === selectedPreviewLayerId) return { ...l, floor: fl, building: b }
        }
      }
    }
    return null
  })()

  const totalSelectedCount = selectedLayerIds.size
  const totalLayersCount = allLayerIds.length
  const primaryDs = datasets.find((d) => d.id === primaryDatasetId)
  const isGeoreferenced = primaryDs?.georeference_status === "georeferenced" || (primaryDs?.extracted_metadata?.bounds && Object.keys(primaryDs.extracted_metadata.bounds).length > 0)

  // Estimated Size Calculation
  const estimatedSizeMb = (() => {
    if (exportType === "base_layer") return "4.2"
    let bytes = 0
    selectedLayerIds.forEach((id) => {
      const ds = (datasets || []).find((d) => d.id === id)
      if (ds?.file_size) bytes += ds.file_size
      else bytes += 2 * 1024 * 1024
    })
    if (exportType === "project_zip" && inclusion.include_base_map) {
      bytes += 3 * 1024 * 1024
    }
    return (bytes / (1024 * 1024)).toFixed(1)
  })()

  // Handle Export Dispatch
  const handleExportClick = () => {
    if (exportType !== "base_layer" && totalSelectedCount === 0) return
    if (exporting) return

    const buildingIds = []
    const floorIds = []
    buildingList.forEach((b) => {
      let bldHasSelected = false
      b.floors.forEach((fl) => {
        const hasSelected = fl.layers.some((l) => selectedLayerIds.has(l.id))
        if (hasSelected) {
          bldHasSelected = true
          floorIds.push(fl.id)
        }
      })
      if (bldHasSelected) buildingIds.push(b.id)
    })

    onExport({
      format: exportType,
      building_ids: buildingIds,
      floor_ids: floorIds,
      layer_ids: Array.from(selectedLayerIds),
      options: {
        include_building_configuration: inclusion.include_building_config,
        include_control_points: inclusion.include_control_points,
        include_icons: inclusion.include_landmarks,
        include_layer_styles: inclusion.include_layer_styles,
        include_original_rasters: inclusion.include_original_rasters,
        include_processed_rasters: inclusion.include_raster_imagery,
        include_base_map_settings: exportType === "base_layer" ? true : inclusion.include_base_map,
        include_project_metadata: inclusion.include_metadata,
        zoom_config: {
          initial_zoom: Number(zoomConfig.initial_zoom),
          min_zoom: Number(zoomConfig.min_zoom),
          max_zoom: Number(zoomConfig.max_zoom),
          preset: zoomConfig.preset,
        },
      },
    })
  }

  // Dynamic Button Title based on Option
  const getButtonLabel = () => {
    if (exporting) {
      if (exportType === "project_zip") return "Packaging Viewscape Project…"
      if (exportType === "mbtiles") return "Generating MBTiles Database…"
      return "Packaging Base Layer…"
    }
    if (exportType === "project_zip") return "Export Viewscape Project"
    if (exportType === "mbtiles") return "Export MBTiles"
    return "Export Base Layer"
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-6 bg-slate-900/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div
        className="relative w-full max-w-5xl max-h-[94vh] flex flex-col bg-white rounded-2xl shadow-2xl border border-slate-200 overflow-hidden"
        role="dialog"
        aria-modal="true"
        aria-labelledby="export-modal-title"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-100 bg-white">
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center size-10 rounded-xl bg-blue-50 text-blue-600 border border-blue-100/80 shadow-xs">
              <Download className="size-5" />
            </div>
            <div>
              <h2 id="export-modal-title" className="text-lg font-bold text-slate-900 leading-tight">
                Export Map Project
              </h2>
              <p className="text-xs text-slate-500">
                Choose what you want to export: complete Viewscape project, standalone map tiles, or reusable base layer.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={exporting}
            className="p-2 text-slate-400 hover:text-slate-600 hover:bg-slate-100 rounded-lg transition-colors"
            aria-label="Close export modal"
          >
            <X className="size-5" />
          </button>
        </div>

        {/* Scrollable Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6 bg-slate-50/40">
          {/* =========================================================================
              SECTION 1: CHOOSE EXPORT TYPE
              Heading: What do you want to export?
              3 Cards in exact order: Option A (Project), Option B (MBTiles), Option C (Base Layer)
              ========================================================================= */}
          <div>
            <div className="mb-3">
              <h3 className="text-sm font-bold text-slate-900">What do you want to export?</h3>
              <p className="text-xs text-slate-500">
                Choose whether you need the complete map project, map tiles only, or a reusable base map.
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-3.5">
              {/* Option A — Viewscape-Ready Project (Primary and Default) */}
              <div
                onClick={() => setExportType("project_zip")}
                className={`relative p-4 rounded-xl border cursor-pointer transition-all flex flex-col justify-between ${
                  exportType === "project_zip"
                    ? "bg-blue-50/60 border-blue-500 shadow-sm ring-2 ring-blue-500/20"
                    : "bg-white border-slate-200 hover:border-slate-300 hover:bg-slate-50/50"
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-blue-100 text-blue-700">
                      Primary • Default
                    </span>
                    <span className="text-[11px] font-mono font-semibold text-slate-500">.zip</span>
                  </div>

                  {/* SVG Illustration: Layered Map (Base + Raster + Points + Config) */}
                  <div className="h-20 w-full mb-3 rounded-lg bg-blue-100/40 border border-blue-200/50 flex items-center justify-center p-2 relative overflow-hidden">
                    <svg viewBox="0 0 160 80" className="w-full h-full text-blue-600">
                      {/* Package container frame */}
                      <rect x="15" y="8" width="130" height="64" rx="6" fill="#EFF6FF" stroke="#3B82F6" strokeWidth="1.5" strokeDasharray="3 2" />
                      {/* Base map layer */}
                      <polygon points="35,56 80,68 125,56 80,44" fill="#93C5FD" opacity="0.6" stroke="#2563EB" strokeWidth="1" />
                      {/* Raster floor plan layer */}
                      <polygon points="35,42 80,54 125,42 80,30" fill="#60A5FA" opacity="0.8" stroke="#1D4ED8" strokeWidth="1" />
                      {/* Control point crosshair */}
                      <circle cx="80" cy="40" r="4" fill="#EF4444" stroke="#FFFFFF" strokeWidth="1" />
                      {/* Landmark pin icon */}
                      <path d="M105,24 C105,21.8 103.2,20 101,20 C98.8,20 97,21.8 97,24 C97,27 101,31 101,31 C101,31 105,27 105,24 Z" fill="#F59E0B" />
                      {/* Floor configuration label */}
                      <rect x="25" y="14" width="34" height="10" rx="2" fill="#DBEAFE" />
                      <text x="28" y="21.5" fontSize="6" fill="#1E40AF" fontWeight="bold">PROJECT</text>
                    </svg>
                  </div>

                  <h4 className="text-sm font-bold text-slate-900 leading-snug">Viewscape-Ready Project</h4>
                  <p className="text-xs text-slate-600 mt-1 leading-relaxed">
                    Export your complete map project for offline viewing and future work in a compatible Viewscape.
                  </p>

                  <ul className="mt-2.5 space-y-1 text-[11px] text-slate-600">
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-blue-600 shrink-0" />
                      <span>Reconstructs original map workspace</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-blue-600 shrink-0" />
                      <span>Preserves custom icons, landmarks, control points</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-blue-600 shrink-0" />
                      <span>Includes base map for offline use & multifloor</span>
                    </li>
                  </ul>
                </div>

                <div className="mt-3 pt-2.5 border-t border-slate-200/60 flex items-center justify-between">
                  <span className="text-[11px] font-medium text-blue-600">Action:</span>
                  <span className="text-[11px] font-bold text-blue-700">Export Viewscape Project</span>
                </div>
              </div>

              {/* Option B — Single MBTiles File (Secondary Option) */}
              <div
                onClick={() => setExportType("mbtiles")}
                className={`relative p-4 rounded-xl border cursor-pointer transition-all flex flex-col justify-between ${
                  exportType === "mbtiles"
                    ? "bg-indigo-50/60 border-indigo-500 shadow-sm ring-2 ring-indigo-500/20"
                    : "bg-white border-slate-200 hover:border-slate-300 hover:bg-slate-50/50"
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-indigo-100 text-indigo-700">
                      Secondary
                    </span>
                    <span className="text-[11px] font-mono font-semibold text-slate-500">.mbtiles</span>
                  </div>

                  {/* SVG Illustration: Map tiles grid into database cylinder */}
                  <div className="h-20 w-full mb-3 rounded-lg bg-indigo-100/40 border border-indigo-200/50 flex items-center justify-center p-2 relative overflow-hidden">
                    <svg viewBox="0 0 160 80" className="w-full h-full text-indigo-600">
                      {/* Tile Grid */}
                      <rect x="25" y="20" width="16" height="16" rx="2" fill="#C7D2FE" stroke="#4F46E5" strokeWidth="1" />
                      <rect x="44" y="20" width="16" height="16" rx="2" fill="#A5B4FC" stroke="#4F46E5" strokeWidth="1" />
                      <rect x="25" y="39" width="16" height="16" rx="2" fill="#A5B4FC" stroke="#4F46E5" strokeWidth="1" />
                      <rect x="44" y="39" width="16" height="16" rx="2" fill="#818CF8" stroke="#4F46E5" strokeWidth="1" />
                      {/* Arrow into Database Cylinder */}
                      <path d="M68,37 L82,37" stroke="#4F46E5" strokeWidth="2" strokeDasharray="2 2" />
                      <polygon points="82,34 88,37 82,40" fill="#4F46E5" />
                      {/* Database Cylinder */}
                      <ellipse cx="112" cy="26" rx="18" ry="7" fill="#6366F1" />
                      <path d="M94,26 L94,52 C94,56 102,59 112,59 C122,59 130,56 130,52 L130,26" fill="#818CF8" opacity="0.8" stroke="#4338CA" strokeWidth="1.2" />
                      <ellipse cx="112" cy="38" rx="18" ry="6" fill="none" stroke="#4338CA" strokeWidth="1" opacity="0.6" />
                    </svg>
                  </div>

                  <h4 className="text-sm font-bold text-slate-900 leading-snug">Single MBTiles File</h4>
                  <p className="text-xs text-slate-600 mt-1 leading-relaxed">
                    Export map tiles for use in compatible offline mapping and GIS applications.
                  </p>

                  <ul className="mt-2.5 space-y-1 text-[11px] text-slate-600">
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-indigo-600 shrink-0" />
                      <span>Portable map-tile database</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-indigo-600 shrink-0" />
                      <span>Standard zoom levels & geographic bounds</span>
                    </li>
                  </ul>

                  {/* Explicit limitation callout */}
                  <div className="mt-2.5 p-2 rounded bg-amber-50 border border-amber-200/80 text-[10px] text-amber-800 leading-snug">
                    <strong>Limitation:</strong> MBTiles does not automatically preserve the complete project, editable control points, custom icon assets, or all original styling.
                  </div>
                </div>

                <div className="mt-3 pt-2.5 border-t border-slate-200/60 flex items-center justify-between">
                  <span className="text-[11px] font-medium text-indigo-600">Action:</span>
                  <span className="text-[11px] font-bold text-indigo-700">Export MBTiles</span>
                </div>
              </div>

              {/* Option C — Base Layer Only (Additional Option) */}
              <div
                onClick={() => setExportType("base_layer")}
                className={`relative p-4 rounded-xl border cursor-pointer transition-all flex flex-col justify-between ${
                  exportType === "base_layer"
                    ? "bg-emerald-50/60 border-emerald-500 shadow-sm ring-2 ring-emerald-500/20"
                    : "bg-white border-slate-200 hover:border-slate-300 hover:bg-slate-50/50"
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded bg-emerald-100 text-emerald-700">
                      Reusable Base
                    </span>
                    <span className="text-[11px] font-mono font-semibold text-slate-500">.mbtiles / .zip</span>
                  </div>

                  {/* SVG Illustration: Base-map layer separated and transferred to another viewer */}
                  <div className="h-20 w-full mb-3 rounded-lg bg-emerald-100/40 border border-emerald-200/50 flex items-center justify-center p-2 relative overflow-hidden">
                    <svg viewBox="0 0 160 80" className="w-full h-full text-emerald-600">
                      {/* Original stack with overlay separated */}
                      <polygon points="25,60 55,68 85,60 55,52" fill="#6EE7B7" stroke="#059669" strokeWidth="1" />
                      <polygon points="25,44 55,52 85,44 55,36" fill="#A7F3D0" opacity="0.5" stroke="#059669" strokeWidth="1" strokeDasharray="2 2" />
                      {/* Arrow transferring base layer */}
                      <path d="M75,58 C90,58 95,45 105,45" fill="none" stroke="#059669" strokeWidth="2" strokeDasharray="3 2" />
                      <polygon points="105,42 111,45 105,48" fill="#059669" />
                      {/* Target Viewer Screen */}
                      <rect x="115" y="24" width="34" height="42" rx="3" fill="#ECFDF5" stroke="#059669" strokeWidth="1.5" />
                      <line x1="115" y1="32" x2="149" y2="32" stroke="#A7F3D0" strokeWidth="1" />
                      <circle cx="120" cy="28" r="1.5" fill="#10B981" />
                      <polygon points="120,54 132,58 144,54 132,50" fill="#34D399" />
                    </svg>
                  </div>

                  <h4 className="text-sm font-bold text-slate-900 leading-snug">Base Layer Only</h4>
                  <p className="text-xs text-slate-600 mt-1 leading-relaxed">
                    Export the base map from your project as a reusable offline layer for another compatible Viewscape.
                  </p>

                  <ul className="mt-2.5 space-y-1 text-[11px] text-slate-600">
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-emerald-600 shrink-0" />
                      <span>Reuse geographic background independently</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-emerald-600 shrink-0" />
                      <span>Import base map without entire project</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-emerald-600 shrink-0" />
                      <span>Keeps base map separate from overlays</span>
                    </li>
                  </ul>
                </div>

                <div className="mt-3 pt-2.5 border-t border-slate-200/60 flex items-center justify-between">
                  <span className="text-[11px] font-medium text-emerald-600">Action:</span>
                  <span className="text-[11px] font-bold text-emerald-700">Export Base Layer</span>
                </div>
              </div>
            </div>
          </div>

          {/* =========================================================================
              SECTION 2: INTERACTIVE VISUAL INCLUSION INFOGRAPHIC
              Dynamic flowchart reflecting current selection and inclusion state
              ========================================================================= */}
          <div className="p-4 rounded-xl bg-white border border-slate-200 shadow-xs">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <span className="text-xs font-bold uppercase tracking-wider text-slate-500">
                  Visual Inclusion Flow
                </span>
                <span className="text-[10px] px-2 py-0.5 rounded-full bg-slate-100 text-slate-600 font-medium">
                  Live Status Diagram
                </span>
              </div>
              <button
                type="button"
                onClick={() => setShowGuidance((prev) => !prev)}
                className="text-xs text-blue-600 hover:text-blue-700 flex items-center gap-1 font-medium"
              >
                <HelpCircle className="size-3.5" />
                <span>{showGuidance ? "Hide Guide" : "What Do I Need?"}</span>
              </button>
            </div>

            {/* Viewscape Project Infographic Flow */}
            {exportType === "project_zip" && (
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2.5">
                {/* 1. Base Map */}
                <div className={`p-2.5 rounded-lg border flex flex-col justify-between ${
                  inclusion.include_base_map ? "bg-blue-50/70 border-blue-200 text-blue-900" : "bg-slate-50 border-slate-200 text-slate-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <Map className="size-4 text-blue-600" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded ${
                      inclusion.include_base_map ? "bg-blue-100 text-blue-700" : "bg-slate-200 text-slate-600"
                    }`}>
                      {inclusion.include_base_map ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Base Map</div>
                    <div className="text-[10px] text-slate-500 leading-tight mt-0.5">World tile background</div>
                  </div>
                </div>

                {/* 2. Raster Layers */}
                <div className={`p-2.5 rounded-lg border flex flex-col justify-between ${
                  totalSelectedCount > 0 ? "bg-blue-50/70 border-blue-200 text-blue-900" : "bg-red-50 border-red-200 text-red-700"
                }`}>
                  <div className="flex items-center justify-between">
                    <Image className="size-4 text-blue-600" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded ${
                      totalSelectedCount > 0 ? "bg-emerald-100 text-emerald-700" : "bg-red-100 text-red-700"
                    }`}>
                      {totalSelectedCount > 0 ? `${totalSelectedCount} Selected` : "Required"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Raster Layers</div>
                    <div className="text-[10px] text-slate-500 leading-tight mt-0.5">Floor plans & imagery</div>
                  </div>
                </div>

                {/* 3. Landmark Icons */}
                <div className={`p-2.5 rounded-lg border flex flex-col justify-between ${
                  inclusion.include_landmarks ? "bg-blue-50/70 border-blue-200 text-blue-900" : "bg-slate-50 border-slate-200 text-slate-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <MapPin className="size-4 text-amber-500" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded ${
                      inclusion.include_landmarks ? "bg-amber-100 text-amber-800" : "bg-slate-200 text-slate-600"
                    }`}>
                      {inclusion.include_landmarks ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Landmarks</div>
                    <div className="text-[10px] text-slate-500 leading-tight mt-0.5">Icons & map points</div>
                  </div>
                </div>

                {/* 4. Control Points */}
                <div className={`p-2.5 rounded-lg border flex flex-col justify-between ${
                  inclusion.include_control_points ? "bg-blue-50/70 border-blue-200 text-blue-900" : "bg-slate-50 border-slate-200 text-slate-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <Crosshair className="size-4 text-rose-500" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded ${
                      inclusion.include_control_points ? "bg-rose-100 text-rose-800" : "bg-slate-200 text-slate-600"
                    }`}>
                      {inclusion.include_control_points ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Control Points</div>
                    <div className="text-[10px] text-slate-500 leading-tight mt-0.5">Georeferencing coordinates</div>
                  </div>
                </div>

                {/* 5. Project Configuration */}
                <div className={`p-2.5 rounded-lg border flex flex-col justify-between ${
                  inclusion.include_building_config ? "bg-blue-50/70 border-blue-200 text-blue-900" : "bg-slate-50 border-slate-200 text-slate-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <Building2 className="size-4 text-indigo-500" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded ${
                      inclusion.include_building_config ? "bg-indigo-100 text-indigo-800" : "bg-slate-200 text-slate-600"
                    }`}>
                      {inclusion.include_building_config ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Configuration</div>
                    <div className="text-[10px] text-slate-500 leading-tight mt-0.5">Buildings & multifloor</div>
                  </div>
                </div>

                {/* 6. Export Package */}
                <div className="p-2.5 rounded-lg border bg-emerald-50/80 border-emerald-300 text-emerald-950 flex flex-col justify-between">
                  <div className="flex items-center justify-between">
                    <Archive className="size-4 text-emerald-600" />
                    <span className="text-[10px] font-bold px-1.5 py-0.2 rounded bg-emerald-200 text-emerald-800">
                      Ready
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Export Package</div>
                    <div className="text-[10px] text-emerald-700 leading-tight mt-0.5">Viewscape .zip bundle</div>
                  </div>
                </div>
              </div>
            )}

            {/* MBTiles Infographic Flow */}
            {exportType === "mbtiles" && (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div className="p-3 rounded-lg border bg-indigo-50/60 border-indigo-200">
                  <div className="flex items-center justify-between">
                    <Image className="size-4 text-indigo-600" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-indigo-100 text-indigo-700">Source</span>
                  </div>
                  <div className="text-xs font-bold text-slate-800 mt-2">Selected Tile Data</div>
                  <div className="text-[11px] text-slate-500 mt-0.5">XYZ raster tile pyramid from {totalSelectedCount} layers</div>
                </div>
                <div className="p-3 rounded-lg border bg-indigo-50/60 border-indigo-200">
                  <div className="flex items-center justify-between">
                    <Database className="size-4 text-indigo-600" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-indigo-100 text-indigo-700">Container</span>
                  </div>
                  <div className="text-xs font-bold text-slate-800 mt-2">MBTiles Database</div>
                  <div className="text-[11px] text-slate-500 mt-0.5">Standardized tile structure with metadata table</div>
                </div>
                <div className="p-3 rounded-lg border bg-emerald-50/60 border-emerald-200">
                  <div className="flex items-center justify-between">
                    <Eye className="size-4 text-emerald-600" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-emerald-100 text-emerald-700">Target</span>
                  </div>
                  <div className="text-xs font-bold text-slate-800 mt-2">Compatible Offline Viewer</div>
                  <div className="text-[11px] text-slate-500 mt-0.5">Loads directly into QGIS, MapLibre GL, and mobile GIS</div>
                </div>
              </div>
            )}

            {/* Base Layer Only Infographic Flow */}
            {exportType === "base_layer" && (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div className="p-3 rounded-lg border bg-emerald-50/60 border-emerald-200">
                  <div className="flex items-center justify-between">
                    <Map className="size-4 text-emerald-600" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-emerald-100 text-emerald-700">Background</span>
                  </div>
                  <div className="text-xs font-bold text-slate-800 mt-2">Base Layer</div>
                  <div className="text-[11px] text-slate-500 mt-0.5">Clean OpenStreetMap world tile set for offline use</div>
                </div>
                <div className="p-3 rounded-lg border bg-emerald-50/60 border-emerald-200">
                  <div className="flex items-center justify-between">
                    <Database className="size-4 text-emerald-600" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-emerald-100 text-emerald-700">Format</span>
                  </div>
                  <div className="text-xs font-bold text-slate-800 mt-2">Standalone MBTiles File</div>
                  <div className="text-[11px] text-slate-500 mt-0.5">Packaged as veracity-base-map.zip with style.json</div>
                </div>
                <div className="p-3 rounded-lg border bg-blue-50/60 border-blue-200">
                  <div className="flex items-center justify-between">
                    <ArrowRight className="size-4 text-blue-600" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-blue-100 text-blue-700">Transfer</span>
                  </div>
                  <div className="text-xs font-bold text-slate-800 mt-2">Import into Another Viewscape</div>
                  <div className="text-[11px] text-slate-500 mt-0.5">Reusable background without project overlays or points</div>
                </div>
              </div>
            )}
          </div>

          {/* Optional "What Do I Need?" Guidance Panel */}
          {showGuidance && (
            <div className="p-4 rounded-xl bg-blue-50/50 border border-blue-200/80 animate-in fade-in duration-150">
              <div className="flex items-center justify-between mb-3">
                <h4 className="text-xs font-bold uppercase tracking-wider text-blue-900">
                  What should your export contain?
                </h4>
                <span className="text-[11px] text-blue-700">Resource Guidance</span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3 text-xs">
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <Map className="size-3.5 text-blue-600" />
                    <span>Base Map</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include this if you want the geographic background available offline or plan to reuse it in another viewer.
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <Image className="size-3.5 text-blue-600" />
                    <span>Raster Imagery</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include floor plans and processed imagery to reproduce the visible map. Original source files are optional.
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <MapPin className="size-3.5 text-amber-500" />
                    <span>Landmarks & Icons</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include these to preserve cameras, buildings, landmarks, and their custom symbols.
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <Crosshair className="size-3.5 text-rose-500" />
                    <span>Control Points</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include these to preserve point coordinates and the information needed for georeferencing.
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <Building2 className="size-3.5 text-indigo-500" />
                    <span>Building Configuration</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include this for multifloor projects or when floor order and raster associations matter.
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <Sliders className="size-3.5 text-purple-500" />
                    <span>Layer Styles</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include this to retain supported layer visibility, opacity, and appearance settings.
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <Archive className="size-3.5 text-slate-500" />
                    <span>Original Source Files</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include these when you need to edit or reprocess the original uploaded files. This increases file size.
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-white border border-blue-100">
                  <div className="flex items-center gap-1.5 font-bold text-slate-800">
                    <FileText className="size-3.5 text-emerald-500" />
                    <span>Project Metadata</span>
                  </div>
                  <p className="text-[11px] text-slate-600 mt-1">
                    Include this to retain project identification and relevant descriptive information.
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* =========================================================================
              SECTION 3: CONFIGURE THE SELECTED EXPORT
              Conditional configuration panels based on exportType
              ========================================================================= */}
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
            {/* Left Col (7 cols): Layer/Content Selection */}
            <div className="lg:col-span-7 flex flex-col gap-4">
              {exportType === "project_zip" && (
                <>
                  <div className="flex items-center justify-between">
                    <div className="text-xs font-bold uppercase tracking-wider text-slate-500">
                      Layer Selection & Hierarchy
                    </div>
                    <div className="text-xs text-slate-500 font-medium">
                      {totalSelectedCount} of {totalLayersCount} layers selected
                    </div>
                  </div>

                  {/* Layer hierarchy list */}
                  <div className="bg-white rounded-xl border border-slate-200 divide-y divide-slate-100 shadow-xs max-h-72 overflow-y-auto p-2">
                    {standaloneLayers.map((lyr) => (
                      <div
                        key={lyr.id}
                        onClick={() => setSelectedPreviewLayerId(lyr.id)}
                        className={`flex items-center gap-3 p-2.5 rounded-lg cursor-pointer transition-all ${
                          lyr.id === selectedPreviewLayerId ? "bg-blue-50/80 border border-blue-200" : "hover:bg-slate-50"
                        }`}
                      >
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            toggleLayer(lyr.id)
                          }}
                          className={`size-4 rounded flex items-center justify-center border transition-colors ${
                            isLayerSelected(lyr.id)
                              ? "bg-blue-600 border-blue-600 text-white"
                              : "border-slate-300 bg-white hover:border-slate-400"
                          }`}
                        >
                          {isLayerSelected(lyr.id) && <Check className="size-2.5 stroke-[3]" />}
                        </button>

                        <div className="size-7 rounded bg-slate-100 border border-slate-200 overflow-hidden shrink-0 flex items-center justify-center">
                          <img
                            src={datasetSourceImageUrl(lyr.id)}
                            alt=""
                            className="size-full object-cover"
                            onError={(e) => { e.target.style.display = "none" }}
                          />
                        </div>

                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5">
                            <span className="text-xs font-semibold text-slate-800 truncate">{lyr.name}</span>
                            {lyr.isPrimary && (
                              <span className="text-[10px] font-medium px-1.5 py-0.2 rounded bg-blue-100 text-blue-700">
                                Primary
                              </span>
                            )}
                          </div>
                          <div className="text-[11px] text-slate-400">Opacity {Math.round(lyr.opacity * 100)}%</div>
                        </div>
                      </div>
                    ))}
                    {buildingList.map((b) => (
                      <div key={b.id} className="p-2 bg-slate-50/50 rounded-lg">
                        <div className="text-xs font-bold text-slate-700 flex items-center gap-1.5 mb-1.5">
                          <Building2 className="size-3.5 text-indigo-500" />
                          <span>{b.name}</span>
                        </div>
                        {b.floors.map((fl) => (
                          <div key={fl.id} className="pl-4 space-y-1">
                            {fl.layers.map((lyr) => (
                              <div
                                key={lyr.id}
                                onClick={() => setSelectedPreviewLayerId(lyr.id)}
                                className={`flex items-center gap-2 p-1.5 rounded cursor-pointer ${
                                  lyr.id === selectedPreviewLayerId ? "bg-blue-50 text-blue-900" : "hover:bg-white"
                                }`}
                              >
                                <button
                                  type="button"
                                  onClick={(e) => {
                                    e.stopPropagation()
                                    toggleLayer(lyr.id)
                                  }}
                                  className={`size-3.5 rounded flex items-center justify-center border ${
                                    isLayerSelected(lyr.id) ? "bg-blue-600 border-blue-600 text-white" : "border-slate-300 bg-white"
                                  }`}
                                >
                                  {isLayerSelected(lyr.id) && <Check className="size-2 stroke-[3]" />}
                                </button>
                                <span className="text-xs text-slate-700 truncate">{fl.name} • {lyr.name}</span>
                              </div>
                            ))}
                          </div>
                        ))}
                      </div>
                    ))}
                  </div>

                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={selectAll}
                      className="px-2.5 py-1 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-50"
                    >
                      Select All
                    </button>
                    <button
                      type="button"
                      onClick={deselectAll}
                      className="px-2.5 py-1 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-50"
                    >
                      Deselect All
                    </button>
                  </div>

                  {/* Inclusion Options Checkboxes */}
                  <div className="p-4 bg-white rounded-xl border border-slate-200 shadow-xs space-y-3">
                    <div className="text-xs font-bold uppercase tracking-wider text-slate-500">
                      Inclusion Settings
                    </div>

                    <div className="space-y-2 text-xs">
                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_base_map}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_base_map: e.target.checked }))}
                          className="mt-0.5 size-4 rounded text-blue-600 focus:ring-blue-500"
                        />
                        <div>
                          <span className="font-semibold text-slate-800">Include base world map</span>
                          <p className="text-[11px] text-slate-500">Keeps the geographic background available offline when supported.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_raster_imagery}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_raster_imagery: e.target.checked }))}
                          className="mt-0.5 size-4 rounded text-blue-600 focus:ring-blue-500"
                        />
                        <div>
                          <span className="font-semibold text-slate-800">Include raster floor plans and processed imagery</span>
                          <p className="text-[11px] text-slate-500">Preserves the raster layers used by your map.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_landmarks}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_landmarks: e.target.checked }))}
                          className="mt-0.5 size-4 rounded text-blue-600 focus:ring-blue-500"
                        />
                        <div>
                          <span className="font-semibold text-slate-800">Include landmark icons and map points</span>
                          <p className="text-[11px] text-slate-500">Preserves landmark coordinates, properties, and custom icon assets.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_control_points}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_control_points: e.target.checked }))}
                          className="mt-0.5 size-4 rounded text-blue-600 focus:ring-blue-500"
                        />
                        <div>
                          <span className="font-semibold text-slate-800">Include shared control points and georeferencing</span>
                          <p className="text-[11px] text-slate-500">Preserves control-point coordinates and transformation parameters.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_building_config}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_building_config: e.target.checked }))}
                          className="mt-0.5 size-4 rounded text-blue-600 focus:ring-blue-500"
                        />
                        <div>
                          <span className="font-semibold text-slate-800">Include building and floor configuration</span>
                          <p className="text-[11px] text-slate-500">Preserves floors, floor order, and multifloor relationships.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_original_rasters}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_original_rasters: e.target.checked }))}
                          className="mt-0.5 size-4 rounded text-blue-600 focus:ring-blue-500"
                        />
                        <div>
                          <span className="font-semibold text-slate-800">Include original uploaded files</span>
                          <p className="text-[11px] text-slate-500">Adds original source files for reprocessing. May increase package size.</p>
                        </div>
                      </label>
                    </div>
                  </div>
                </>
              )}

              {exportType === "mbtiles" && (
                <div className="p-4 bg-white rounded-xl border border-slate-200 shadow-xs space-y-4">
                  <div className="text-xs font-bold uppercase tracking-wider text-slate-500">
                    MBTiles Settings
                  </div>

                  <div className="space-y-3 text-xs">
                    <div>
                      <span className="font-semibold text-slate-800 block mb-1">Exported Tile Content</span>
                      <p className="text-slate-500 text-[11px]">
                        Raster map tile pyramid (XYZ convention) generated from your georeferenced floor plans and active overlays.
                      </p>
                    </div>

                    <div className="p-3 bg-indigo-50/60 border border-indigo-100 rounded-lg space-y-1.5">
                      <div className="font-semibold text-indigo-950">Active Layer Coverage</div>
                      <div className="text-[11px] text-indigo-800">
                        Primary Dataset: <strong>{primaryDs?.filename || "Current Map"}</strong>
                      </div>
                      <div className="text-[11px] text-indigo-800">
                        Georeferenced Status: {isGeoreferenced ? (
                          <span className="text-emerald-700 font-semibold">Georeferenced</span>
                        ) : (
                          <span className="text-amber-700 font-semibold">Coordinate bounds required</span>
                        )}
                      </div>
                    </div>

                    <label className="flex items-start gap-2.5 cursor-pointer pt-1">
                      <input
                        type="checkbox"
                        checked={mbtilesIncludeBasemap}
                        onChange={(e) => setMbtilesIncludeBasemap(e.target.checked)}
                        className="mt-0.5 size-4 rounded text-indigo-600 focus:ring-indigo-500"
                      />
                      <div>
                        <span className="font-semibold text-slate-800">Include compatible base map in package</span>
                        <p className="text-[11px] text-slate-500">Packages offline base world map tiles together with overlay tiles.</p>
                      </div>
                    </label>
                  </div>
                </div>
              )}

              {exportType === "base_layer" && (
                <div className="p-4 bg-white rounded-xl border border-slate-200 shadow-xs space-y-4">
                  <div className="text-xs font-bold uppercase tracking-wider text-slate-500">
                    Base Layer Source Selection
                  </div>

                  <div className="space-y-3 text-xs">
                    <p className="text-slate-600 text-[11px]">
                      Select the base layer candidate to export as an independent, offline-ready mapping layer:
                    </p>

                    <div className="grid grid-cols-1 gap-2.5">
                      <label className="flex items-start gap-3 p-3 rounded-xl border border-emerald-300 bg-emerald-50/50 cursor-pointer">
                        <input
                          type="radio"
                          name="basemap_candidate"
                          checked={selectedBasemapSource === "osm_standard"}
                          onChange={() => setSelectedBasemapSource("osm_standard")}
                          className="mt-0.5 size-4 text-emerald-600 focus:ring-emerald-500"
                        />
                        <div className="flex-1">
                          <div className="flex items-center justify-between">
                            <span className="font-bold text-slate-900">OpenStreetMap Standard</span>
                            <span className="text-[10px] font-semibold px-2 py-0.5 rounded bg-emerald-100 text-emerald-800">Available Offline</span>
                          </div>
                          <p className="text-[11px] text-slate-600 mt-0.5">
                            Global raster tile base map (zoom 0 to 19). Fully packaged for standalone offline Viewscapes.
                          </p>
                          <div className="text-[10px] text-slate-400 mt-1">
                            Attribution: © OpenStreetMap contributors
                          </div>
                        </div>
                      </label>
                    </div>

                    <div className="p-3 bg-slate-50 border border-slate-200 rounded-lg text-[11px] text-slate-600 space-y-1">
                      <div className="font-semibold text-slate-700">Output Specifications:</div>
                      <div>• Output File: <strong>veracity-base-map.zip</strong> (containing tiles and style.json)</div>
                      <div>• Compatible with: MapLibre GL, Leaflet, QGIS, and offline Viewscapes</div>
                      <div>• Overlay data (floor plans, icons, control points) is excluded from this export.</div>
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Right Col (5 cols): Zoom Ladder & Live Preview */}
            <div className="lg:col-span-5 flex flex-col gap-4">
              {/* Visual Zoom Configuration (with Illustrated Presets) */}
              <div className="p-4 bg-white rounded-xl border border-slate-200 shadow-xs space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold uppercase tracking-wider text-slate-500">
                    Visual Zoom Configuration
                  </span>
                  <button
                    type="button"
                    onClick={handleResetZoom}
                    className="text-xs text-slate-400 hover:text-slate-600 flex items-center gap-1 font-medium"
                  >
                    <RotateCcw className="size-3" />
                    <span>Reset</span>
                  </button>
                </div>

                {/* Preset Buttons */}
                <div className="grid grid-cols-3 gap-1.5 text-xs">
                  <button
                    type="button"
                    onClick={() => handlePresetChange("auto")}
                    className={`p-2 rounded-lg border text-center transition-all ${
                      zoomConfig.preset === "auto" ? "bg-blue-600 text-white border-blue-600 font-bold" : "bg-slate-50 hover:bg-slate-100 border-slate-200 text-slate-700"
                    }`}
                  >
                    Auto View
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("house")}
                    className={`p-2 rounded-lg border text-center transition-all ${
                      zoomConfig.initial_zoom === 21 ? "bg-blue-600 text-white border-blue-600 font-bold" : "bg-slate-50 hover:bg-slate-100 border-slate-200 text-slate-700"
                    }`}
                  >
                    House (z21)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("building")}
                    className={`p-2 rounded-lg border text-center transition-all ${
                      zoomConfig.initial_zoom === 20 ? "bg-blue-600 text-white border-blue-600 font-bold" : "bg-slate-50 hover:bg-slate-100 border-slate-200 text-slate-700"
                    }`}
                  >
                    Building (z20)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("street")}
                    className={`p-2 rounded-lg border text-center transition-all ${
                      zoomConfig.initial_zoom === 19 ? "bg-blue-600 text-white border-blue-600 font-bold" : "bg-slate-50 hover:bg-slate-100 border-slate-200 text-slate-700"
                    }`}
                  >
                    Street (z19)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("neighborhood")}
                    className={`p-2 rounded-lg border text-center transition-all ${
                      zoomConfig.initial_zoom === 18 ? "bg-blue-600 text-white border-blue-600 font-bold" : "bg-slate-50 hover:bg-slate-100 border-slate-200 text-slate-700"
                    }`}
                  >
                    Neighborhood (z18)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("city")}
                    className={`p-2 rounded-lg border text-center transition-all ${
                      zoomConfig.initial_zoom === 15 ? "bg-blue-600 text-white border-blue-600 font-bold" : "bg-slate-50 hover:bg-slate-100 border-slate-200 text-slate-700"
                    }`}
                  >
                    City (z15)
                  </button>
                </div>

                {/* Zoom range inputs */}
                <div className="grid grid-cols-3 gap-2 text-xs pt-1">
                  <div>
                    <label className="text-[11px] text-slate-500 font-medium block mb-1">Initial Zoom</label>
                    <input
                      type="number"
                      value={zoomConfig.initial_zoom}
                      onChange={(e) => setZoomConfig((p) => ({ ...p, initial_zoom: parseInt(e.target.value, 10) || 0 }))}
                      className="w-full px-2.5 py-1.5 font-mono text-xs rounded-lg border border-slate-200"
                    />
                  </div>
                  <div>
                    <label className="text-[11px] text-slate-500 font-medium block mb-1">Min (Out)</label>
                    <input
                      type="number"
                      value={zoomConfig.min_zoom}
                      onChange={(e) => setZoomConfig((p) => ({ ...p, min_zoom: parseInt(e.target.value, 10) || 0 }))}
                      className="w-full px-2.5 py-1.5 font-mono text-xs rounded-lg border border-slate-200"
                    />
                  </div>
                  <div>
                    <label className="text-[11px] text-slate-500 font-medium block mb-1">Max (In)</label>
                    <input
                      type="number"
                      value={zoomConfig.max_zoom}
                      onChange={(e) => setZoomConfig((p) => ({ ...p, max_zoom: parseInt(e.target.value, 10) || 0 }))}
                      className="w-full px-2.5 py-1.5 font-mono text-xs rounded-lg border border-slate-200"
                    />
                  </div>
                </div>

                {zoomValidation && (
                  <div className="p-2 rounded bg-amber-50 border border-amber-200 text-[11px] text-amber-700 flex items-center gap-1.5">
                    <AlertCircle className="size-3.5 shrink-0" />
                    <span>{zoomValidation}</span>
                  </div>
                )}
              </div>

              {/* Layer preview thumbnail */}
              {activePreviewLayer && exportType !== "base_layer" && (
                <div className="p-3 bg-white rounded-xl border border-slate-200 shadow-xs flex flex-col gap-2">
                  <div className="text-xs font-bold uppercase tracking-wider text-slate-500">
                    Layer Preview
                  </div>
                  <div className="relative aspect-4/3 w-full rounded-lg bg-slate-900/5 border border-slate-200 overflow-hidden flex items-center justify-center p-2">
                    <img
                      src={datasetSourceImageUrl(activePreviewLayer.id)}
                      alt={activePreviewLayer.name}
                      className="max-h-full max-w-full object-contain rounded"
                      style={{ opacity: activePreviewLayer.opacity }}
                    />
                  </div>
                  <div className="text-[11px] text-slate-500 flex items-center justify-between">
                    <span className="truncate">{activePreviewLayer.name}</span>
                    <span className="font-semibold text-slate-700">{Math.round(activePreviewLayer.opacity * 100)}% Opacity</span>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* =========================================================================
              SECTION 4: REVIEW EXPORT SUMMARY & INCLUDED VS EXCLUDED
              ========================================================================= */}
          <div className="p-4 rounded-xl bg-white border border-slate-200 shadow-xs space-y-4">
            <div className="text-xs font-bold uppercase tracking-wider text-slate-500">
              Export Inspection & Comparison
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Included Group */}
              <div className="p-3.5 rounded-xl bg-emerald-50/50 border border-emerald-200 text-xs space-y-2">
                <div className="font-bold text-emerald-900 flex items-center gap-1.5">
                  <CheckCircle2 className="size-4 text-emerald-600" />
                  <span>Included in this export</span>
                </div>
                <ul className="space-y-1.5 text-[11px] text-emerald-800">
                  {exportType === "project_zip" && (
                    <>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-emerald-600" />
                        <span>{totalSelectedCount} selected raster overlay{totalSelectedCount !== 1 ? "s" : ""}</span>
                      </li>
                      {inclusion.include_base_map && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-emerald-600" />
                          <span>Offline world base map tile package</span>
                        </li>
                      )}
                      {inclusion.include_landmarks && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-emerald-600" />
                          <span>Landmarks and custom marker icon assets</span>
                        </li>
                      )}
                      {inclusion.include_control_points && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-emerald-600" />
                          <span>Control points and georeferencing metadata</span>
                        </li>
                      )}
                      {inclusion.include_building_config && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-emerald-600" />
                          <span>Building, floor, and layer ordering configurations</span>
                        </li>
                      )}
                    </>
                  )}
                  {exportType === "mbtiles" && (
                    <>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-emerald-600" />
                        <span>Pre-rendered map tile database (.mbtiles)</span>
                      </li>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-emerald-600" />
                        <span>Tile pyramid levels z{zoomConfig.min_zoom} to z{zoomConfig.max_zoom}</span>
                      </li>
                      {mbtilesIncludeBasemap && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-emerald-600" />
                          <span>Base map tile package</span>
                        </li>
                      )}
                    </>
                  )}
                  {exportType === "base_layer" && (
                    <>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-emerald-600" />
                        <span>Standalone OpenStreetMap base layer tiles</span>
                      </li>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-emerald-600" />
                        <span>MapLibre Style Specification v8 (style.json)</span>
                      </li>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-emerald-600" />
                        <span>Independent offline reuse manifest</span>
                      </li>
                    </>
                  )}
                </ul>
              </div>

              {/* Not Included Group */}
              <div className="p-3.5 rounded-xl bg-slate-50 border border-slate-200 text-xs space-y-2">
                <div className="font-bold text-slate-700 flex items-center gap-1.5">
                  <XCircle className="size-4 text-slate-400" />
                  <span>Not included (or functional impact)</span>
                </div>
                <ul className="space-y-1.5 text-[11px] text-slate-500">
                  {exportType === "project_zip" && (
                    <>
                      {!inclusion.include_original_rasters && (
                        <li className="flex items-start gap-1.5">
                          <Minus className="size-3 mt-0.5 text-slate-400 shrink-0" />
                          <span>Original raw source images (future reprocessing will use processed PNGs)</span>
                        </li>
                      )}
                      {!inclusion.include_base_map && (
                        <li className="flex items-start gap-1.5">
                          <Minus className="size-3 mt-0.5 text-slate-400 shrink-0" />
                          <span>Base map omitted: Viewscape will render raster layers over dark/blank canvas</span>
                        </li>
                      )}
                      {!inclusion.include_landmarks && (
                        <li className="flex items-start gap-1.5">
                          <Minus className="size-3 mt-0.5 text-slate-400 shrink-0" />
                          <span>Landmarks excluded: Custom pins and icons will not appear</span>
                        </li>
                      )}
                    </>
                  )}
                  {exportType === "mbtiles" && (
                    <>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-slate-400 shrink-0" />
                        <span>Editable control points are baked into raster tiles and not individually editable</span>
                      </li>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-slate-400 shrink-0" />
                        <span>Multifloor layer relationships are flattened into the tile database</span>
                      </li>
                    </>
                  )}
                  {exportType === "base_layer" && (
                    <>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-slate-400 shrink-0" />
                        <span>Floor plan raster overlays (only the background map is exported)</span>
                      </li>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-slate-400 shrink-0" />
                        <span>Project-specific control points and landmarks</span>
                      </li>
                    </>
                  )}
                </ul>
              </div>
            </div>
          </div>

          {/* Validation Warnings */}
          {!isGeoreferenced && exportType !== "base_layer" && (
            <div className="p-3 bg-amber-50 border border-amber-200 rounded-xl text-xs text-amber-800 flex items-start gap-2">
              <ShieldAlert className="size-4 shrink-0 text-amber-600 mt-0.5" />
              <div>
                <div className="font-bold">Georeferencing Warning</div>
                <div className="text-[11px] text-amber-700 mt-0.5 leading-relaxed">
                  Your map does not have active ground control points yet. It can still be exported, but geographic tiling and base map overlay alignment may be approximate.
                </div>
              </div>
            </div>
          )}

          {error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-xs text-red-600 flex items-start gap-2">
              <AlertCircle className="size-4 shrink-0 mt-0.5" />
              <div>
                <div className="font-semibold">{error.message || "Export error"}</div>
                {error.details && <div className="text-red-500 mt-0.5">{error.details}</div>}
              </div>
            </div>
          )}
        </div>

        {/* =========================================================================
            SECTION 5: EXPORT AND DOWNLOAD (FOOTER)
            ========================================================================= */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-slate-100 bg-white">
          <div className="flex items-center gap-3 text-xs text-slate-500">
            <span>
              Format: <strong className="text-slate-800 font-mono">{exportType === "project_zip" ? "Viewscape .zip" : exportType === "mbtiles" ? ".mbtiles" : "Base Map .zip"}</strong>
            </span>
            <span>•</span>
            <span>Est. Size: ~{estimatedSizeMb} MB</span>
          </div>

          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={exporting}
              className="px-4 py-2 text-sm font-medium text-slate-700 hover:text-slate-900 bg-white border border-slate-200 rounded-xl hover:bg-slate-50 transition-colors"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleExportClick}
              disabled={(exportType !== "base_layer" && totalSelectedCount === 0) || exporting || !!zoomValidation}
              className={`inline-flex items-center gap-2 px-5 py-2 text-sm font-semibold text-white rounded-xl transition-all shadow-sm hover:shadow disabled:opacity-50 disabled:cursor-not-allowed ${
                exportType === "project_zip"
                  ? "bg-blue-600 hover:bg-blue-700"
                  : exportType === "mbtiles"
                  ? "bg-indigo-600 hover:bg-indigo-700"
                  : "bg-emerald-600 hover:bg-emerald-700"
              }`}
            >
              {exporting ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  <span>{getButtonLabel()}</span>
                </>
              ) : (
                <>
                  <Download className="size-4" />
                  <span>{getButtonLabel()}</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
