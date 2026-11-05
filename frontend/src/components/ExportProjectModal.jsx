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
  const isGeoreferenced =
    primaryDs?.georeference_status === "completed" ||
    primaryDs?.georeference_status === "georeferenced" ||
    Boolean(primaryDs?.extracted_metadata?.bounds && Object.keys(primaryDs.extracted_metadata.bounds).length > 0)

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
        include_base_map_settings: exportType === "base_layer" ? true : exportType === "mbtiles" ? mbtilesIncludeBasemap : inclusion.include_base_map,
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
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-6 bg-black/60 backdrop-blur-xs animate-in fade-in duration-200">
      <div
        className="relative w-full max-w-5xl max-h-[94vh] flex flex-col bg-white dark:bg-[#202020] text-neutral-900 dark:text-white rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] overflow-hidden"
        role="dialog"
        aria-modal="true"
        aria-labelledby="export-modal-title"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717]">
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center size-10 rounded-[8px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/20 shadow-xs">
              <Download className="size-5" />
            </div>
            <div>
              <h2 id="export-modal-title" className="text-lg font-bold text-neutral-900 dark:text-white leading-tight">
                Export Map Project
              </h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                Choose what you want to export: complete Viewscape project, standalone map tiles, or reusable base layer.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={exporting}
            className="p-2 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px] transition-colors cursor-pointer"
            aria-label="Close export modal"
          >
            <X className="size-5" />
          </button>
        </div>

        {/* Scrollable Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6 bg-white dark:bg-[#1c1c1c]">
          {/* =========================================================================
              SECTION 1: CHOOSE EXPORT TYPE
              Heading: What do you want to export?
              3 Cards in exact order: Option A (Project), Option B (MBTiles), Option C (Base Layer)
              ========================================================================= */}
          <div>
            <div className="mb-3">
              <h3 className="text-sm font-bold text-neutral-900 dark:text-white">What do you want to export?</h3>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                Choose whether you need the complete map project, map tiles only, or a reusable base map.
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-3.5">
              {/* Option A — Viewscape-Ready Project (Primary and Default) */}
              <div
                onClick={() => setExportType("project_zip")}
                className={`relative p-4 rounded-[8px] border cursor-pointer transition-all flex flex-col justify-between ${
                  exportType === "project_zip"
                    ? "bg-[#3ecf8e]/10 border-[#3ecf8e] shadow-xs ring-2 ring-[#3ecf8e]/20"
                    : "bg-white dark:bg-[#202020] border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-[#3a3a3a] hover:bg-neutral-50 dark:hover:bg-[#252525]"
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">
                      Primary • Default
                    </span>
                    <span className="text-[11px] font-mono font-semibold text-neutral-500 dark:text-neutral-400">.zip</span>
                  </div>

                  {/* SVG Illustration: Layered Map (Base + Raster + Points + Config) */}
                  <div className="h-20 w-full mb-3 rounded-[6px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center p-2 relative overflow-hidden">
                    <svg viewBox="0 0 160 80" className="w-full h-full text-[#3ecf8e]">
                      {/* Package container frame */}
                      <rect x="15" y="8" width="130" height="64" rx="6" fill="rgba(62, 207, 142, 0.05)" stroke="#3ecf8e" strokeWidth="1.5" strokeDasharray="3 2" />
                      {/* Base map layer */}
                      <polygon points="35,56 80,68 125,56 80,44" fill="#3ecf8e" opacity="0.4" stroke="#24b47e" strokeWidth="1" />
                      {/* Raster floor plan layer */}
                      <polygon points="35,42 80,54 125,42 80,30" fill="#3ecf8e" opacity="0.7" stroke="#24b47e" strokeWidth="1" />
                      {/* Control point crosshair */}
                      <circle cx="80" cy="40" r="4" fill="#EF4444" stroke="#FFFFFF" strokeWidth="1" />
                      {/* Landmark pin icon */}
                      <path d="M105,24 C105,21.8 103.2,20 101,20 C98.8,20 97,21.8 97,24 C97,27 101,31 101,31 C101,31 105,27 105,24 Z" fill="#F59E0B" />
                      {/* Floor configuration label */}
                      <rect x="25" y="14" width="34" height="10" rx="2" fill="rgba(62, 207, 142, 0.2)" />
                      <text x="28" y="21.5" fontSize="6" fill="#3ecf8e" fontWeight="bold">PROJECT</text>
                    </svg>
                  </div>

                  <h4 className="text-sm font-bold text-neutral-900 dark:text-white leading-snug">Viewscape-Ready Project</h4>
                  <p className="text-xs text-neutral-600 dark:text-neutral-400 mt-1 leading-relaxed">
                    Export your complete map project for offline viewing and future work in a compatible Viewscape.
                  </p>

                  <ul className="mt-2.5 space-y-1 text-[11px] text-neutral-600 dark:text-neutral-400">
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Reconstructs original map workspace</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Preserves custom icons, landmarks, control points</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Includes base map for offline use & multifloor</span>
                    </li>
                  </ul>
                </div>

                <div className="mt-3 pt-2.5 border-t border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
                  <span className="text-[11px] font-medium text-[#24b47e] dark:text-[#3ecf8e]">Action:</span>
                  <span className="text-[11px] font-bold text-[#24b47e] dark:text-[#3ecf8e]">Export Viewscape Project</span>
                </div>
              </div>

              {/* Option B — Single MBTiles File (Secondary Option) */}
              <div
                onClick={() => setExportType("mbtiles")}
                className={`relative p-4 rounded-[8px] border cursor-pointer transition-all flex flex-col justify-between ${
                  exportType === "mbtiles"
                    ? "bg-[#3ecf8e]/10 border-[#3ecf8e] shadow-xs ring-2 ring-[#3ecf8e]/20"
                    : "bg-white dark:bg-[#202020] border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-[#3a3a3a] hover:bg-neutral-50 dark:hover:bg-[#252525]"
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-[4px] bg-neutral-200 dark:bg-[#282828] text-neutral-800 dark:text-neutral-200">
                      Secondary
                    </span>
                    <span className="text-[11px] font-mono font-semibold text-neutral-500 dark:text-neutral-400">.mbtiles</span>
                  </div>

                  {/* SVG Illustration: Map tiles grid into database cylinder */}
                  <div className="h-20 w-full mb-3 rounded-[6px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center p-2 relative overflow-hidden">
                    <svg viewBox="0 0 160 80" className="w-full h-full text-neutral-400">
                      {/* Tile Grid */}
                      <rect x="25" y="20" width="16" height="16" rx="2" fill="#3ecf8e" opacity="0.3" stroke="#24b47e" strokeWidth="1" />
                      <rect x="44" y="20" width="16" height="16" rx="2" fill="#3ecf8e" opacity="0.5" stroke="#24b47e" strokeWidth="1" />
                      <rect x="25" y="39" width="16" height="16" rx="2" fill="#3ecf8e" opacity="0.5" stroke="#24b47e" strokeWidth="1" />
                      <rect x="44" y="39" width="16" height="16" rx="2" fill="#3ecf8e" opacity="0.7" stroke="#24b47e" strokeWidth="1" />
                      {/* Arrow into Database Cylinder */}
                      <path d="M68,37 L82,37" stroke="#3ecf8e" strokeWidth="2" strokeDasharray="2 2" />
                      <polygon points="82,34 88,37 82,40" fill="#3ecf8e" />
                      {/* Database Cylinder */}
                      <ellipse cx="112" cy="26" rx="18" ry="7" fill="#3ecf8e" opacity="0.8" />
                      <path d="M94,26 L94,52 C94,56 102,59 112,59 C122,59 130,56 130,52 L130,26" fill="#3ecf8e" opacity="0.5" stroke="#24b47e" strokeWidth="1.2" />
                      <ellipse cx="112" cy="38" rx="18" ry="6" fill="none" stroke="#24b47e" strokeWidth="1" opacity="0.6" />
                    </svg>
                  </div>

                  <h4 className="text-sm font-bold text-neutral-900 dark:text-white leading-snug">Single MBTiles File</h4>
                  <p className="text-xs text-neutral-600 dark:text-neutral-400 mt-1 leading-relaxed">
                    Export map tiles for use in compatible offline mapping and GIS applications.
                  </p>

                  <ul className="mt-2.5 space-y-1 text-[11px] text-neutral-600 dark:text-neutral-400">
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Portable map-tile database</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Standard zoom levels & geographic bounds</span>
                    </li>
                  </ul>

                  {/* Explicit limitation callout */}
                  <div className="mt-2.5 p-2 rounded-[6px] bg-amber-500/10 border border-amber-500/20 text-[10px] text-amber-600 dark:text-amber-400 leading-snug">
                    <strong>Limitation:</strong> MBTiles does not automatically preserve the complete project, editable control points, custom icon assets, or all original styling.
                  </div>
                </div>

                <div className="mt-3 pt-2.5 border-t border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
                  <span className="text-[11px] font-medium text-neutral-500 dark:text-neutral-400">Action:</span>
                  <span className="text-[11px] font-bold text-neutral-900 dark:text-white">Export MBTiles</span>
                </div>
              </div>

              {/* Option C — Base Layer Only (Additional Option) */}
              <div
                onClick={() => setExportType("base_layer")}
                className={`relative p-4 rounded-[8px] border cursor-pointer transition-all flex flex-col justify-between ${
                  exportType === "base_layer"
                    ? "bg-[#3ecf8e]/10 border-[#3ecf8e] shadow-xs ring-2 ring-[#3ecf8e]/20"
                    : "bg-white dark:bg-[#202020] border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-[#3a3a3a] hover:bg-neutral-50 dark:hover:bg-[#252525]"
                }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">
                      Reusable Base
                    </span>
                    <span className="text-[11px] font-mono font-semibold text-neutral-500 dark:text-neutral-400">.mbtiles / .zip</span>
                  </div>

                  {/* SVG Illustration: Base-map layer separated and transferred to another viewer */}
                  <div className="h-20 w-full mb-3 rounded-[6px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center p-2 relative overflow-hidden">
                    <svg viewBox="0 0 160 80" className="w-full h-full text-[#3ecf8e]">
                      {/* Original stack with overlay separated */}
                      <polygon points="25,60 55,68 85,60 55,52" fill="#3ecf8e" stroke="#24b47e" strokeWidth="1" />
                      <polygon points="25,44 55,52 85,44 55,36" fill="#3ecf8e" opacity="0.3" stroke="#24b47e" strokeWidth="1" strokeDasharray="2 2" />
                      {/* Arrow transferring base layer */}
                      <path d="M75,58 C90,58 95,45 105,45" fill="none" stroke="#24b47e" strokeWidth="2" strokeDasharray="3 2" />
                      <polygon points="105,42 111,45 105,48" fill="#24b47e" />
                      {/* Target Viewer Screen */}
                      <rect x="115" y="24" width="34" height="42" rx="3" fill="rgba(62, 207, 142, 0.1)" stroke="#24b47e" strokeWidth="1.5" />
                      <line x1="115" y1="32" x2="149" y2="32" stroke="#24b47e" strokeWidth="1" />
                      <circle cx="120" cy="28" r="1.5" fill="#3ecf8e" />
                      <polygon points="120,54 132,58 144,54 132,50" fill="#3ecf8e" />
                    </svg>
                  </div>

                  <h4 className="text-sm font-bold text-neutral-900 dark:text-white leading-snug">Base Layer Only</h4>
                  <p className="text-xs text-neutral-600 dark:text-neutral-400 mt-1 leading-relaxed">
                    Export the base map from your project as a reusable offline layer for another compatible Viewscape.
                  </p>

                  <ul className="mt-2.5 space-y-1 text-[11px] text-neutral-600 dark:text-neutral-400">
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Reuse geographic background independently</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Import base map without entire project</span>
                    </li>
                    <li className="flex items-center gap-1.5">
                      <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                      <span>Keeps base map separate from overlays</span>
                    </li>
                  </ul>
                </div>

                <div className="mt-3 pt-2.5 border-t border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between">
                  <span className="text-[11px] font-medium text-[#24b47e] dark:text-[#3ecf8e]">Action:</span>
                  <span className="text-[11px] font-bold text-[#24b47e] dark:text-[#3ecf8e]">Export Base Layer</span>
                </div>
              </div>
            </div>
          </div>

          {/* =========================================================================
              SECTION 2: INTERACTIVE VISUAL INCLUSION INFOGRAPHIC
              Dynamic flowchart reflecting current selection and inclusion state
              ========================================================================= */}
          <div className="p-4 rounded-[8px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs">
            <div className="flex items-center justify-between mb-3">
              <div className="flex items-center gap-2">
                <span className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
                  Visual Inclusion Flow
                </span>
                <span className="text-[10px] px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-[#171717] text-neutral-600 dark:text-neutral-300 border border-neutral-200 dark:border-[#2e2e2e] font-medium">
                  Live Status Diagram
                </span>
              </div>
              <button
                type="button"
                onClick={() => setShowGuidance((prev) => !prev)}
                className="text-xs text-[#24b47e] dark:text-[#3ecf8e] hover:underline flex items-center gap-1 font-medium cursor-pointer"
              >
                <HelpCircle className="size-3.5" />
                <span>{showGuidance ? "Hide Guide" : "What Do I Need?"}</span>
              </button>
            </div>

            {/* Viewscape Project Infographic Flow */}
            {exportType === "project_zip" && (
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2.5">
                {/* 1. Base Map */}
                <div className={`p-2.5 rounded-[6px] border flex flex-col justify-between ${
                  inclusion.include_base_map ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/30 text-neutral-900 dark:text-white" : "bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e] text-neutral-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <Map className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded-[4px] ${
                      inclusion.include_base_map ? "bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]" : "bg-neutral-200 dark:bg-[#282828] text-neutral-600 dark:text-neutral-400"
                    }`}>
                      {inclusion.include_base_map ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Base Map</div>
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400 leading-tight mt-0.5">World tile background</div>
                  </div>
                </div>

                {/* 2. Raster Layers */}
                <div className={`p-2.5 rounded-[6px] border flex flex-col justify-between ${
                  totalSelectedCount > 0 ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/30 text-neutral-900 dark:text-white" : "bg-red-500/10 border-red-500/20 text-red-600 dark:text-red-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <Image className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded-[4px] ${
                      totalSelectedCount > 0 ? "bg-emerald-500/20 text-emerald-600 dark:text-emerald-400" : "bg-red-500/20 text-red-600 dark:text-red-400"
                    }`}>
                      {totalSelectedCount > 0 ? `${totalSelectedCount} Selected` : "Required"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Raster Layers</div>
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400 leading-tight mt-0.5">Floor plans & imagery</div>
                  </div>
                </div>

                {/* 3. Landmark Icons */}
                <div className={`p-2.5 rounded-[6px] border flex flex-col justify-between ${
                  inclusion.include_landmarks ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/30 text-neutral-900 dark:text-white" : "bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e] text-neutral-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <MapPin className="size-4 text-amber-500" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded-[4px] ${
                      inclusion.include_landmarks ? "bg-amber-500/20 text-amber-600 dark:text-amber-400" : "bg-neutral-200 dark:bg-[#282828] text-neutral-600 dark:text-neutral-400"
                    }`}>
                      {inclusion.include_landmarks ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Landmarks</div>
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400 leading-tight mt-0.5">Icons & map points</div>
                  </div>
                </div>

                {/* 4. Control Points */}
                <div className={`p-2.5 rounded-[6px] border flex flex-col justify-between ${
                  inclusion.include_control_points ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/30 text-neutral-900 dark:text-white" : "bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e] text-neutral-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <Crosshair className="size-4 text-rose-500" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded-[4px] ${
                      inclusion.include_control_points ? "bg-rose-500/20 text-rose-600 dark:text-rose-400" : "bg-neutral-200 dark:bg-[#282828] text-neutral-600 dark:text-neutral-400"
                    }`}>
                      {inclusion.include_control_points ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Control Points</div>
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400 leading-tight mt-0.5">Georeferencing coordinates</div>
                  </div>
                </div>

                {/* 5. Project Configuration */}
                <div className={`p-2.5 rounded-[6px] border flex flex-col justify-between ${
                  inclusion.include_building_config ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/30 text-neutral-900 dark:text-white" : "bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e] text-neutral-400"
                }`}>
                  <div className="flex items-center justify-between">
                    <Building2 className="size-4 text-indigo-500" />
                    <span className={`text-[10px] font-bold px-1.5 py-0.2 rounded-[4px] ${
                      inclusion.include_building_config ? "bg-indigo-500/20 text-indigo-600 dark:text-indigo-400" : "bg-neutral-200 dark:bg-[#282828] text-neutral-600 dark:text-neutral-400"
                    }`}>
                      {inclusion.include_building_config ? "Included" : "Not Included"}
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Configuration</div>
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400 leading-tight mt-0.5">Buildings & multifloor</div>
                  </div>
                </div>

                {/* 6. Export Package */}
                <div className="p-2.5 rounded-[6px] border bg-[#3ecf8e]/20 border-[#3ecf8e]/40 text-neutral-900 dark:text-white flex flex-col justify-between">
                  <div className="flex items-center justify-between">
                    <Archive className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className="text-[10px] font-bold px-1.5 py-0.2 rounded-[4px] bg-[#3ecf8e] text-[#171717]">
                      Ready
                    </span>
                  </div>
                  <div className="mt-2">
                    <div className="text-xs font-bold">Export Package</div>
                    <div className="text-[10px] text-[#24b47e] dark:text-[#3ecf8e] leading-tight mt-0.5">Viewscape .zip bundle</div>
                  </div>
                </div>
              </div>
            )}

            {/* MBTiles Infographic Flow */}
            {exportType === "mbtiles" && (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div className="p-3 rounded-[6px] border bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center justify-between">
                    <Image className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-[4px] bg-neutral-200 dark:bg-[#282828] text-neutral-800 dark:text-neutral-200">Source</span>
                  </div>
                  <div className="text-xs font-bold text-neutral-900 dark:text-white mt-2">Selected Tile Data</div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">XYZ raster tile pyramid from {totalSelectedCount} layers</div>
                </div>
                <div className="p-3 rounded-[6px] border bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center justify-between">
                    <Database className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-[4px] bg-neutral-200 dark:bg-[#282828] text-neutral-800 dark:text-neutral-200">Container</span>
                  </div>
                  <div className="text-xs font-bold text-neutral-900 dark:text-white mt-2">MBTiles Database</div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">Standardized tile structure with metadata table</div>
                </div>
                <div className="p-3 rounded-[6px] border bg-[#3ecf8e]/10 border-[#3ecf8e]/30">
                  <div className="flex items-center justify-between">
                    <Eye className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">Target</span>
                  </div>
                  <div className="text-xs font-bold text-neutral-900 dark:text-white mt-2">Compatible Offline Viewer</div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">Loads directly into QGIS, MapLibre GL, and mobile GIS</div>
                </div>
              </div>
            )}

            {/* Base Layer Only Infographic Flow */}
            {exportType === "base_layer" && (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div className="p-3 rounded-[6px] border bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center justify-between">
                    <Map className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-[4px] bg-neutral-200 dark:bg-[#282828] text-neutral-800 dark:text-neutral-200">Background</span>
                  </div>
                  <div className="text-xs font-bold text-neutral-900 dark:text-white mt-2">Base Layer</div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">Clean OpenStreetMap world tile set for offline use</div>
                </div>
                <div className="p-3 rounded-[6px] border bg-neutral-50 dark:bg-[#171717] border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center justify-between">
                    <Database className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-[4px] bg-neutral-200 dark:bg-[#282828] text-neutral-800 dark:text-neutral-200">Format</span>
                  </div>
                  <div className="text-xs font-bold text-neutral-900 dark:text-white mt-2">Standalone MBTiles File</div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">Packaged as veracity-base-map.zip with style.json</div>
                </div>
                <div className="p-3 rounded-[6px] border bg-[#3ecf8e]/10 border-[#3ecf8e]/30">
                  <div className="flex items-center justify-between">
                    <ArrowRight className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">Transfer</span>
                  </div>
                  <div className="text-xs font-bold text-neutral-900 dark:text-white mt-2">Import into Another Viewscape</div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">Reusable background without project overlays or points</div>
                </div>
              </div>
            )}
          </div>

          {/* Optional "What Do I Need?" Guidance Panel */}
          {showGuidance && (
            <div className="p-4 rounded-[8px] bg-neutral-50 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] animate-in fade-in duration-150">
              <div className="flex items-center justify-between mb-3">
                <h4 className="text-xs font-bold uppercase tracking-wider text-neutral-900 dark:text-white">
                  What should your export contain?
                </h4>
                <span className="text-[11px] text-[#24b47e] dark:text-[#3ecf8e]">Resource Guidance</span>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3 text-xs">
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <Map className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span>Base Map</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
                    Include this if you want the geographic background available offline or plan to reuse it in another viewer.
                  </p>
                </div>
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <Image className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span>Raster Imagery</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
                    Include floor plans and processed imagery to reproduce the visible map. Original source files are optional.
                  </p>
                </div>
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <MapPin className="size-3.5 text-amber-500" />
                    <span>Landmarks & Icons</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
                    Include these to preserve cameras, buildings, landmarks, and their custom symbols.
                  </p>
                </div>
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <Crosshair className="size-3.5 text-rose-500" />
                    <span>Control Points</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
                    Include these to preserve point coordinates and the information needed for georeferencing.
                  </p>
                </div>
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <Building2 className="size-3.5 text-indigo-500" />
                    <span>Building Configuration</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
                    Include this for multifloor projects or when floor order and raster associations matter.
                  </p>
                </div>
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <Sliders className="size-3.5 text-purple-500" />
                    <span>Layer Styles</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
                    Include this to retain supported layer visibility, opacity, and appearance settings.
                  </p>
                </div>
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <Archive className="size-3.5 text-neutral-500" />
                    <span>Original Source Files</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
                    Include these when you need to edit or reprocess the original uploaded files. This increases file size.
                  </p>
                </div>
                <div className="p-3 rounded-[6px] bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-1.5 font-bold text-neutral-900 dark:text-white">
                    <FileText className="size-3.5 text-emerald-500" />
                    <span>Project Metadata</span>
                  </div>
                  <p className="text-[11px] text-neutral-600 dark:text-neutral-400 mt-1">
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
                    <div className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
                      Layer Selection & Hierarchy
                    </div>
                    <div className="text-xs text-neutral-500 dark:text-neutral-400 font-medium">
                      {totalSelectedCount} of {totalLayersCount} layers selected
                    </div>
                  </div>

                  {/* Layer hierarchy list */}
                  <div className="bg-white dark:bg-[#202020] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] divide-y divide-neutral-200 dark:divide-[#2e2e2e] shadow-xs max-h-72 overflow-y-auto p-2">
                    {standaloneLayers.map((lyr) => (
                      <div
                        key={lyr.id}
                        onClick={() => setSelectedPreviewLayerId(lyr.id)}
                        className={`flex items-center gap-3 p-2.5 rounded-[6px] cursor-pointer transition-all ${
                          lyr.id === selectedPreviewLayerId ? "bg-[#3ecf8e]/10 border border-[#3ecf8e]/40" : "hover:bg-neutral-50 dark:hover:bg-[#282828]"
                        }`}
                      >
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            toggleLayer(lyr.id)
                          }}
                          className={`size-4 rounded-[4px] flex items-center justify-center border transition-colors cursor-pointer ${
                            isLayerSelected(lyr.id)
                              ? "bg-[#3ecf8e] border-[#3ecf8e] text-[#171717]"
                              : "border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] hover:border-neutral-400"
                          }`}
                        >
                          {isLayerSelected(lyr.id) && <Check className="size-2.5 stroke-[3]" />}
                        </button>

                        <div className="size-7 rounded-[4px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] overflow-hidden shrink-0 flex items-center justify-center">
                          <img
                            src={datasetSourceImageUrl(lyr.id)}
                            alt=""
                            className="size-full object-cover"
                            onError={(e) => { e.target.style.display = "none" }}
                          />
                        </div>

                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-1.5">
                            <span className="text-xs font-semibold text-neutral-900 dark:text-white truncate">{lyr.name}</span>
                            {lyr.isPrimary && (
                              <span className="text-[10px] font-medium px-1.5 py-0.2 rounded-[4px] bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">
                                Primary
                              </span>
                            )}
                          </div>
                          <div className="text-[11px] text-neutral-500 dark:text-neutral-400">Opacity {Math.round(lyr.opacity * 100)}%</div>
                        </div>
                      </div>
                    ))}
                    {buildingList.map((b) => (
                      <div key={b.id} className="p-2 bg-neutral-50 dark:bg-[#171717] rounded-[6px]">
                        <div className="text-xs font-bold text-neutral-800 dark:text-neutral-200 flex items-center gap-1.5 mb-1.5">
                          <Building2 className="size-3.5 text-indigo-500" />
                          <span>{b.name}</span>
                        </div>
                        {b.floors.map((fl) => (
                          <div key={fl.id} className="pl-4 space-y-1">
                            {fl.layers.map((lyr) => (
                              <div
                                key={lyr.id}
                                onClick={() => setSelectedPreviewLayerId(lyr.id)}
                                className={`flex items-center gap-2 p-1.5 rounded-[4px] cursor-pointer ${
                                  lyr.id === selectedPreviewLayerId ? "bg-[#3ecf8e]/10 text-neutral-900 dark:text-white" : "hover:bg-white dark:hover:bg-[#282828]"
                                }`}
                              >
                                <button
                                  type="button"
                                  onClick={(e) => {
                                    e.stopPropagation()
                                    toggleLayer(lyr.id)
                                  }}
                                  className={`size-3.5 rounded-[3px] flex items-center justify-center border cursor-pointer ${
                                    isLayerSelected(lyr.id) ? "bg-[#3ecf8e] border-[#3ecf8e] text-[#171717]" : "border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020]"
                                  }`}
                                >
                                  {isLayerSelected(lyr.id) && <Check className="size-2 stroke-[3]" />}
                                </button>
                                <span className="text-xs text-neutral-800 dark:text-neutral-200 truncate">{fl.name} • {lyr.name}</span>
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
                      className="px-2.5 py-1 text-xs font-medium text-neutral-800 dark:text-neutral-200 bg-white dark:bg-[#202020] border border-neutral-300 dark:border-[#2e2e2e] rounded-[6px] hover:bg-neutral-100 dark:hover:bg-[#282828] cursor-pointer"
                    >
                      Select All
                    </button>
                    <button
                      type="button"
                      onClick={deselectAll}
                      className="px-2.5 py-1 text-xs font-medium text-neutral-800 dark:text-neutral-200 bg-white dark:bg-[#202020] border border-neutral-300 dark:border-[#2e2e2e] rounded-[6px] hover:bg-neutral-100 dark:hover:bg-[#282828] cursor-pointer"
                    >
                      Deselect All
                    </button>
                  </div>

                  {/* Inclusion Options Checkboxes */}
                  <div className="p-4 bg-white dark:bg-[#202020] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs space-y-3">
                    <div className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
                      Inclusion Settings
                    </div>

                    <div className="space-y-2 text-xs">
                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_base_map}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_base_map: e.target.checked }))}
                          className="mt-0.5 size-4 rounded-[4px] text-[#24b47e] focus:ring-[#3ecf8e]"
                        />
                        <div>
                          <span className="font-semibold text-neutral-900 dark:text-white">Include base world map</span>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">Keeps the geographic background available offline when supported.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_raster_imagery}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_raster_imagery: e.target.checked }))}
                          className="mt-0.5 size-4 rounded-[4px] text-[#24b47e] focus:ring-[#3ecf8e]"
                        />
                        <div>
                          <span className="font-semibold text-neutral-900 dark:text-white">Include raster floor plans and processed imagery</span>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">Preserves the raster layers used by your map.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_landmarks}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_landmarks: e.target.checked }))}
                          className="mt-0.5 size-4 rounded-[4px] text-[#24b47e] focus:ring-[#3ecf8e]"
                        />
                        <div>
                          <span className="font-semibold text-neutral-900 dark:text-white">Include landmark icons and map points</span>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">Preserves landmark coordinates, properties, and custom icon assets.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_control_points}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_control_points: e.target.checked }))}
                          className="mt-0.5 size-4 rounded-[4px] text-[#24b47e] focus:ring-[#3ecf8e]"
                        />
                        <div>
                          <span className="font-semibold text-neutral-900 dark:text-white">Include shared control points and georeferencing</span>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">Preserves control-point coordinates and transformation parameters.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_building_config}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_building_config: e.target.checked }))}
                          className="mt-0.5 size-4 rounded-[4px] text-[#24b47e] focus:ring-[#3ecf8e]"
                        />
                        <div>
                          <span className="font-semibold text-neutral-900 dark:text-white">Include building and floor configuration</span>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">Preserves floors, floor order, and multifloor relationships.</p>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={inclusion.include_original_rasters}
                          onChange={(e) => setInclusion((p) => ({ ...p, include_original_rasters: e.target.checked }))}
                          className="mt-0.5 size-4 rounded-[4px] text-[#24b47e] focus:ring-[#3ecf8e]"
                        />
                        <div>
                          <span className="font-semibold text-neutral-900 dark:text-white">Include original uploaded files</span>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">Adds original source files for reprocessing. May increase package size.</p>
                        </div>
                      </label>
                    </div>
                  </div>
                </>
              )}

              {exportType === "mbtiles" && (
                <div className="p-4 bg-white dark:bg-[#202020] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs space-y-4">
                  <div className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
                    MBTiles Settings
                  </div>

                  <div className="space-y-3 text-xs">
                    <div>
                      <span className="font-semibold text-neutral-900 dark:text-white block mb-1">Exported Tile Content</span>
                      <p className="text-neutral-500 dark:text-neutral-400 text-[11px]">
                        Raster map tile pyramid (XYZ convention) generated from your georeferenced floor plans and active overlays.
                      </p>
                    </div>

                    <div className="p-3 bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] space-y-2">
                      <div className="font-semibold text-neutral-900 dark:text-white">Active Layer Coverage</div>
                      <div className="text-[11px] text-neutral-600 dark:text-neutral-300">
                        Primary Dataset: <strong>{primaryDs?.filename || "Current Map"}</strong>
                      </div>
                      <div className="text-[11px] text-neutral-600 dark:text-neutral-300">
                        Georeferenced Status: {isGeoreferenced ? (
                          <span className="text-[#24b47e] dark:text-[#3ecf8e] font-semibold">Georeferenced</span>
                        ) : (
                          <span className="text-amber-500 font-semibold">Coordinate bounds required</span>
                        )}
                      </div>
                      <div className="pt-1.5 border-t border-neutral-200 dark:border-[#2e2e2e]">
                        <label className="flex items-center gap-2 cursor-pointer select-none">
                          <input
                            type="checkbox"
                            checked={mbtilesIncludeBasemap}
                            onChange={(e) => setMbtilesIncludeBasemap(e.target.checked)}
                            className="rounded-[4px] border-neutral-300 dark:border-[#2e2e2e] text-[#24b47e] focus:ring-[#3ecf8e]"
                          />
                          <span className="text-[11px] font-medium text-neutral-800 dark:text-neutral-200">
                            Bundle offline base map into MBTiles (creates a standalone, self-contained map for Viewscape)
                          </span>
                        </label>
                      </div>
                    </div>

                    <div className="p-3 bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] text-[11px] text-neutral-600 dark:text-neutral-400 space-y-1">
                      <div className="font-semibold text-neutral-800 dark:text-neutral-200">Standalone MBTiles 1.3 Specifications:</div>
                      <div>• Output File: <strong>.mbtiles</strong> (Genuine SQLite 3 database with <code>metadata</code> &amp; <code>tiles</code> tables)</div>
                      <div>• Directly reopenable in Veracity Maps, MapLibre tile servers, and QGIS</div>
                    </div>
                  </div>
                </div>
              )}

              {exportType === "base_layer" && (
                <div className="p-4 bg-white dark:bg-[#202020] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs space-y-4">
                  <div className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
                    Base Layer Source Selection
                  </div>

                  <div className="space-y-3 text-xs">
                    <p className="text-neutral-600 dark:text-neutral-400 text-[11px]">
                      Select the base layer candidate to export as an independent, offline-ready mapping layer:
                    </p>

                    <div className="grid grid-cols-1 gap-2.5">
                      <label className="flex items-start gap-3 p-3 rounded-[6px] border border-[#3ecf8e]/40 bg-[#3ecf8e]/10 cursor-pointer">
                        <input
                          type="radio"
                          name="basemap_candidate"
                          checked={selectedBasemapSource === "osm_standard"}
                          onChange={() => setSelectedBasemapSource("osm_standard")}
                          className="mt-0.5 size-4 text-[#24b47e] focus:ring-[#3ecf8e]"
                        />
                        <div className="flex-1">
                          <div className="flex items-center justify-between">
                            <span className="font-bold text-neutral-900 dark:text-white">OpenStreetMap Standard</span>
                            <span className="text-[10px] font-semibold px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">Available Offline</span>
                          </div>
                          <p className="text-[11px] text-neutral-600 dark:text-neutral-300 mt-0.5">
                            Global raster tile base map (zoom 0 to 19). Fully packaged for standalone offline Viewscapes.
                          </p>
                          <div className="text-[10px] text-neutral-500 dark:text-neutral-400 mt-1">
                            Attribution: © OpenStreetMap contributors
                          </div>
                        </div>
                      </label>
                    </div>

                    <div className="p-3 bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] text-[11px] text-neutral-600 dark:text-neutral-400 space-y-1">
                      <div className="font-semibold text-neutral-800 dark:text-neutral-200">Output Specifications:</div>
                      <div>• Output File: <strong>veracity-base-map.zip</strong> (containing offline base map MBTiles &amp; style.json)</div>
                      <div>• Compatible with: MapLibre GL JS, QGIS, and offline Viewscapes</div>
                      <div>• Overlay data (floor plans, icons, control points) is excluded from this export.</div>
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Right Col (5 cols): Zoom Ladder & Live Preview */}
            <div className="lg:col-span-5 flex flex-col gap-4">
              {/* Visual Zoom Configuration (with Illustrated Presets) */}
              <div className="p-4 bg-white dark:bg-[#202020] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
                    Visual Zoom Configuration
                  </span>
                  <button
                    type="button"
                    onClick={handleResetZoom}
                    className="text-xs text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 flex items-center gap-1 font-medium cursor-pointer"
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
                    className={`p-2 rounded-[6px] border text-center transition-all cursor-pointer ${
                      zoomConfig.preset === "auto" ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-bold" : "bg-neutral-100 dark:bg-[#171717] hover:bg-neutral-200 dark:hover:bg-[#282828] border-neutral-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200"
                    }`}
                  >
                    Auto View
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("house")}
                    className={`p-2 rounded-[6px] border text-center transition-all cursor-pointer ${
                      zoomConfig.initial_zoom === 21 ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-bold" : "bg-neutral-100 dark:bg-[#171717] hover:bg-neutral-200 dark:hover:bg-[#282828] border-neutral-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200"
                    }`}
                  >
                    House (z21)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("building")}
                    className={`p-2 rounded-[6px] border text-center transition-all cursor-pointer ${
                      zoomConfig.initial_zoom === 20 ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-bold" : "bg-neutral-100 dark:bg-[#171717] hover:bg-neutral-200 dark:hover:bg-[#282828] border-neutral-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200"
                    }`}
                  >
                    Building (z20)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("street")}
                    className={`p-2 rounded-[6px] border text-center transition-all cursor-pointer ${
                      zoomConfig.initial_zoom === 19 ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-bold" : "bg-neutral-100 dark:bg-[#171717] hover:bg-neutral-200 dark:hover:bg-[#282828] border-neutral-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200"
                    }`}
                  >
                    Street (z19)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("neighborhood")}
                    className={`p-2 rounded-[6px] border text-center transition-all cursor-pointer ${
                      zoomConfig.initial_zoom === 18 ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-bold" : "bg-neutral-100 dark:bg-[#171717] hover:bg-neutral-200 dark:hover:bg-[#282828] border-neutral-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200"
                    }`}
                  >
                    Neighborhood (z18)
                  </button>
                  <button
                    type="button"
                    onClick={() => handlePresetChange("city")}
                    className={`p-2 rounded-[6px] border text-center transition-all cursor-pointer ${
                      zoomConfig.initial_zoom === 15 ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-bold" : "bg-neutral-100 dark:bg-[#171717] hover:bg-neutral-200 dark:hover:bg-[#282828] border-neutral-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200"
                    }`}
                  >
                    City (z15)
                  </button>
                </div>

                {/* Zoom range inputs */}
                <div className="grid grid-cols-3 gap-2 text-xs pt-1">
                  <div>
                    <label className="text-[11px] text-neutral-500 dark:text-neutral-400 font-medium block mb-1">Initial Zoom</label>
                    <input
                      type="number"
                      value={zoomConfig.initial_zoom}
                      onChange={(e) => setZoomConfig((p) => ({ ...p, initial_zoom: parseInt(e.target.value, 10) || 0 }))}
                      className="w-full px-2.5 py-1.5 font-mono text-xs rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] text-neutral-900 dark:text-white"
                    />
                  </div>
                  <div>
                    <label className="text-[11px] text-neutral-500 dark:text-neutral-400 font-medium block mb-1">Min (Out)</label>
                    <input
                      type="number"
                      value={zoomConfig.min_zoom}
                      onChange={(e) => setZoomConfig((p) => ({ ...p, min_zoom: parseInt(e.target.value, 10) || 0 }))}
                      className="w-full px-2.5 py-1.5 font-mono text-xs rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] text-neutral-900 dark:text-white"
                    />
                  </div>
                  <div>
                    <label className="text-[11px] text-neutral-500 dark:text-neutral-400 font-medium block mb-1">Max (In)</label>
                    <input
                      type="number"
                      value={zoomConfig.max_zoom}
                      onChange={(e) => setZoomConfig((p) => ({ ...p, max_zoom: parseInt(e.target.value, 10) || 0 }))}
                      className="w-full px-2.5 py-1.5 font-mono text-xs rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] text-neutral-900 dark:text-white"
                    />
                  </div>
                </div>

                {zoomValidation && (
                  <div className="p-2 rounded-[6px] bg-amber-500/10 border border-amber-500/20 text-[11px] text-amber-600 dark:text-amber-400 flex items-center gap-1.5">
                    <AlertCircle className="size-3.5 shrink-0" />
                    <span>{zoomValidation}</span>
                  </div>
                )}
              </div>

              {/* Layer preview thumbnail */}
              {activePreviewLayer && exportType !== "base_layer" && (
                <div className="p-3 bg-white dark:bg-[#202020] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs flex flex-col gap-2">
                  <div className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
                    Layer Preview
                  </div>
                  <div className="relative aspect-4/3 w-full rounded-[6px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] overflow-hidden flex items-center justify-center p-2">
                    <img
                      src={datasetSourceImageUrl(activePreviewLayer.id)}
                      alt={activePreviewLayer.name}
                      className="max-h-full max-w-full object-contain rounded-[4px]"
                      style={{ opacity: activePreviewLayer.opacity }}
                    />
                  </div>
                  <div className="text-[11px] text-neutral-500 dark:text-neutral-400 flex items-center justify-between">
                    <span className="truncate">{activePreviewLayer.name}</span>
                    <span className="font-semibold text-neutral-800 dark:text-neutral-200">{Math.round(activePreviewLayer.opacity * 100)}% Opacity</span>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* =========================================================================
              SECTION 4: REVIEW EXPORT SUMMARY & INCLUDED VS EXCLUDED
              ========================================================================= */}
          <div className="p-4 rounded-[8px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-xs space-y-4">
            <div className="text-xs font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
              Export Inspection & Comparison
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Included Group */}
              <div className="p-3.5 rounded-[8px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-xs space-y-2">
                <div className="font-bold text-neutral-900 dark:text-white flex items-center gap-1.5">
                  <CheckCircle2 className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                  <span>Included in this export</span>
                </div>
                <ul className="space-y-1.5 text-[11px] text-neutral-700 dark:text-neutral-300">
                  {exportType === "project_zip" && (
                    <>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                        <span>{totalSelectedCount} selected raster overlay{totalSelectedCount !== 1 ? "s" : ""}</span>
                      </li>
                      {inclusion.include_base_map && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                          <span>Offline world base map tile package</span>
                        </li>
                      )}
                      {inclusion.include_landmarks && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                          <span>Landmarks and custom marker icon assets</span>
                        </li>
                      )}
                      {inclusion.include_control_points && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                          <span>Control points and georeferencing metadata</span>
                        </li>
                      )}
                      {inclusion.include_building_config && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                          <span>Building, floor, and layer ordering configurations</span>
                        </li>
                      )}
                    </>
                  )}
                  {exportType === "mbtiles" && (
                    <>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                        <span>Pre-rendered map tile database (.mbtiles)</span>
                      </li>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                        <span>Tile pyramid levels z{zoomConfig.min_zoom} to z{zoomConfig.max_zoom}</span>
                      </li>
                      {mbtilesIncludeBasemap && (
                        <li className="flex items-center gap-1.5">
                          <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                          <span>Base map tile package</span>
                        </li>
                      )}
                    </>
                  )}
                  {exportType === "base_layer" && (
                    <>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                        <span>Standalone OpenStreetMap base layer tiles</span>
                      </li>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                        <span>MapLibre Style Specification v8 (style.json)</span>
                      </li>
                      <li className="flex items-center gap-1.5">
                        <Check className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" />
                        <span>Independent offline reuse manifest</span>
                      </li>
                    </>
                  )}
                </ul>
              </div>

              {/* Not Included Group */}
              <div className="p-3.5 rounded-[8px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-xs space-y-2">
                <div className="font-bold text-neutral-800 dark:text-neutral-200 flex items-center gap-1.5">
                  <XCircle className="size-4 text-neutral-400" />
                  <span>Not included (or functional impact)</span>
                </div>
                <ul className="space-y-1.5 text-[11px] text-neutral-500 dark:text-neutral-400">
                  {exportType === "project_zip" && (
                    <>
                      {!inclusion.include_original_rasters && (
                        <li className="flex items-start gap-1.5">
                          <Minus className="size-3 mt-0.5 text-neutral-400 shrink-0" />
                          <span>Original raw source images (future reprocessing will use processed PNGs)</span>
                        </li>
                      )}
                      {!inclusion.include_base_map && (
                        <li className="flex items-start gap-1.5">
                          <Minus className="size-3 mt-0.5 text-neutral-400 shrink-0" />
                          <span>Base map omitted: Viewscape will render raster layers over dark/blank canvas</span>
                        </li>
                      )}
                      {!inclusion.include_landmarks && (
                        <li className="flex items-start gap-1.5">
                          <Minus className="size-3 mt-0.5 text-neutral-400 shrink-0" />
                          <span>Landmarks excluded: Custom pins and icons will not appear</span>
                        </li>
                      )}
                    </>
                  )}
                  {exportType === "mbtiles" && (
                    <>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-neutral-400 shrink-0" />
                        <span>Editable control points are baked into raster tiles and not individually editable</span>
                      </li>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-neutral-400 shrink-0" />
                        <span>Multifloor layer relationships are flattened into the tile database</span>
                      </li>
                    </>
                  )}
                  {exportType === "base_layer" && (
                    <>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-neutral-400 shrink-0" />
                        <span>Floor plan raster overlays (only the background map is exported)</span>
                      </li>
                      <li className="flex items-start gap-1.5">
                        <Minus className="size-3 mt-0.5 text-neutral-400 shrink-0" />
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
            <div className="p-3 bg-amber-500/10 border border-amber-500/20 rounded-[8px] text-xs text-amber-600 dark:text-amber-400 flex items-start gap-2">
              <ShieldAlert className="size-4 shrink-0 text-amber-500 mt-0.5" />
              <div>
                <div className="font-bold">Georeferencing Warning</div>
                <div className="text-[11px] text-amber-600/90 dark:text-amber-400/90 mt-0.5 leading-relaxed">
                  Your map does not have active ground control points yet. It can still be exported, but geographic tiling and base map overlay alignment may be approximate.
                </div>
              </div>
            </div>
          )}

          {error && (
            <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-[8px] text-xs text-red-600 dark:text-red-400 flex items-start gap-2">
              <AlertCircle className="size-4 shrink-0 text-red-500 mt-0.5" />
              <div>
                <div className="font-semibold">{error.message || "Export error"}</div>
                {error.details && <div className="text-red-500/80 mt-0.5">{error.details}</div>}
              </div>
            </div>
          )}
        </div>

        {/* =========================================================================
            SECTION 5: EXPORT AND DOWNLOAD (FOOTER)
            ========================================================================= */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717]">
          <div className="flex items-center gap-3 text-xs text-neutral-500 dark:text-neutral-400">
            <span>
              Format: <strong className="text-neutral-900 dark:text-white font-mono">{exportType === "project_zip" ? "Viewscape .zip" : exportType === "mbtiles" ? ".mbtiles" : "Base Map .zip"}</strong>
            </span>
            <span>•</span>
            <span>Est. Size: ~{estimatedSizeMb} MB</span>
          </div>

          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={exporting}
              className="px-4 py-2 text-xs font-medium text-neutral-800 dark:text-neutral-200 bg-white dark:bg-[#202020] border border-neutral-300 dark:border-[#2e2e2e] rounded-[6px] hover:bg-neutral-100 dark:hover:bg-[#282828] transition-colors cursor-pointer"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleExportClick}
              disabled={(exportType !== "base_layer" && totalSelectedCount === 0) || exporting || !!zoomValidation}
              className="inline-flex items-center gap-2 px-5 py-2 text-xs font-semibold text-[#171717] bg-[#3ecf8e] hover:bg-[#24b47e] rounded-[6px] transition-all shadow-xs disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
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
