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
  Loader2,
  AlertCircle,
  FileText,
  RotateCcw,
  Map,
  Crosshair,
  Archive,
  Info,
  Sliders,
  ShieldAlert,
  Shapes,
  Sparkles,
} from "lucide-react"
import { buildExportHierarchy } from "../lib/exportHierarchy"
import SpringCheck from "./SpringCheck"
import { getExportProgress, cancelExport } from "../api"

export const EXPORT_STAGES = [
  { index: 1, label: "Inspecting selected layers" },
  { index: 2, label: "Calculating tile coverage" },
  { index: 3, label: "Processing raster layers" },
  { index: 4, label: "Generating vector tiles" },
  { index: 5, label: "Processing GeoJSON metadata" },
  { index: 6, label: "Reusing cached tiles" },
  { index: 7, label: "Packaging ZIP" },
  { index: 8, label: "Validating export" },
]

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
  gisLayers = [],
  primaryDatasetId,
  onExport,
  exporting = false,
  error = null,
}) {
  // 1. Build hierarchy of layers and 6 standard categories
  const {
    buildings: buildingList,
    standaloneLayers,
    allLayerIds,
    categories,
  } = useMemo(() => {
    return buildExportHierarchy({
      members: manifest?.members || [],
      datasets: datasets || [],
      buildings: buildings || [],
      gisLayers: gisLayers || [],
      primaryDatasetId,
    })
  }, [buildings, datasets, gisLayers, manifest?.members, primaryDatasetId])

  // Selection states
  const [selectedIds, setSelectedIds] = useState(new Set())
  const [collapsedCategories, setCollapsedCategories] = useState(new Set())

  // Collapsible sections
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [showDiagnostics, setShowDiagnostics] = useState(false)

  // Zoom configuration state
  const [zoomConfig, setZoomConfig] = useState({
    initial_zoom: 18,
    min_zoom: 12,
    max_zoom: 22,
    preset: "neighborhood",
  })

  // Advanced settings state
  const [advancedSettings, setAdvancedSettings] = useState({
    tileSize: 256,
    rasterFormat: "png",
    imageQuality: 85,
    boundsClipping: "project",
  })

  // Live export execution metrics from backend
  const [liveProgress, setLiveProgress] = useState(null)
  const [isCancelling, setIsCancelling] = useState(false)

  // Poll real backend progress during active export
  useEffect(() => {
    if (!exporting || !primaryDatasetId) {
      setLiveProgress(null)
      setIsCancelling(false)
      return
    }

    let active = true
    const poll = async () => {
      try {
        const prog = await getExportProgress(primaryDatasetId)
        if (active && prog && prog.status === "processing") {
          setLiveProgress(prog)
        }
      } catch {
        // Ignore background polling errors
      }
    }

    poll()
    const timer = setInterval(poll, 350)
    return () => {
      active = false
      clearInterval(timer)
    }
  }, [exporting, primaryDatasetId])

  const handleCancelExport = async () => {
    if (isCancelling || !primaryDatasetId) return
    setIsCancelling(true)
    try {
      await cancelExport(primaryDatasetId)
    } catch {}
  }

  // Initialize selection when modal opens
  useEffect(() => {
    if (open) {
      const init = new Set()

      // Default: include base map
      if (categories?.base_maps) {
        categories.base_maps.forEach((item) => init.add(item.id))
      }

      // Default: select all visible raster layers and dependent items
      if (categories?.raster_layers) {
        categories.raster_layers.forEach((item) => {
          if (item.visible !== false) {
            init.add(item.id)
            init.add(`cp_group_${item.id}`)
            init.add(`ip_group_${item.id}`)
            init.add(`area_group_${item.id}`)
          }
        })
      }

      // Default: select all visible vector layers
      if (categories?.vector_layers) {
        categories.vector_layers.forEach((item) => {
          if (item.visible !== false) {
            init.add(item.id)
          }
        })
      }

      setSelectedIds(init)

      // Adopt dataset metadata for zoom if available
      const primaryDs = datasets.find((d) => d.id === primaryDatasetId)
      const meta = primaryDs?.extracted_metadata
      if (meta && (meta.initial_zoom || meta.minzoom || meta.maxzoom)) {
        setZoomConfig({
          initial_zoom: Number(meta.initial_zoom) || 18,
          min_zoom: Number(meta.minzoom) || 12,
          max_zoom: Number(meta.maxzoom) || 22,
          preset: meta.zoom_preset || "neighborhood",
        })
      }
    }
  }, [open, categories, primaryDatasetId, datasets])

  // Category definitions for rendering
  const categoryDefs = useMemo(() => [
    {
      key: "base_maps",
      title: "Base Maps",
      icon: Map,
      items: categories?.base_maps || [],
      description: "Offline world base map tile sets",
    },
    {
      key: "raster_layers",
      title: "Raster Layers",
      icon: Image,
      items: categories?.raster_layers || [],
      description: "Georeferenced imagery and floor plans",
    },
    {
      key: "vector_layers",
      title: "Vector Layers",
      icon: Database,
      items: categories?.vector_layers || [],
      description: "Vector features (GeoPackage, Shapefile, KML, GeoJSON)",
    },
    {
      key: "icon_points",
      title: "Icon Points and Landmarks",
      icon: MapPin,
      items: categories?.icon_points || [],
      description: "Interactive point markers and custom icon assets",
    },
    {
      key: "control_points",
      title: "Control Points",
      icon: Crosshair,
      items: categories?.control_points || [],
      description: "Ground control points and georeferencing coordinates",
    },
    {
      key: "areas",
      title: "Areas and Drawings",
      icon: Shapes,
      items: categories?.areas || [],
      description: "User-created boundary polygons and annotations",
    },
  ], [categories])

  // Toggle single item with dependency enforcement
  const toggleItem = (itemId, itemCategory, rasterId = null) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      const isCurrentlySelected = next.has(itemId)

      if (isCurrentlySelected) {
        next.delete(itemId)
        // If unchecking a raster layer, also deselect dependent items
        if (itemCategory === "raster_layers") {
          next.delete(`cp_group_${itemId}`)
          next.delete(`ip_group_${itemId}`)
          next.delete(`area_group_${itemId}`)
        }
      } else {
        next.add(itemId)
        // If checking a raster layer, automatically select dependent items
        if (itemCategory === "raster_layers") {
          next.add(`cp_group_${itemId}`)
          next.add(`ip_group_${itemId}`)
          next.add(`area_group_${itemId}`)
        }
      }
      return next
    })
  }

  // Toggle entire category
  const toggleCategory = (catKey, items) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      const allSelected = items.every((it) => next.has(it.id))

      items.forEach((it) => {
        if (allSelected) {
          next.delete(it.id)
          if (catKey === "raster_layers") {
            next.delete(`cp_group_${it.id}`)
            next.delete(`ip_group_${it.id}`)
            next.delete(`area_group_${it.id}`)
          }
        } else {
          next.add(it.id)
          if (catKey === "raster_layers") {
            next.add(`cp_group_${it.id}`)
            next.add(`ip_group_${it.id}`)
            next.add(`area_group_${it.id}`)
          }
        }
      })
      return next
    })
  }

  // Toggle category collapse
  const toggleCollapse = (catKey) => {
    setCollapsedCategories((prev) => {
      const next = new Set(prev)
      if (next.has(catKey)) next.delete(catKey)
      else next.add(catKey)
      return next
    })
  }

  // Select all items
  const handleSelectAll = () => {
    const all = new Set()
    categoryDefs.forEach((c) => {
      c.items.forEach((it) => all.add(it.id))
    })
    setSelectedIds(all)
  }

  // Deselect all items
  const handleDeselectAll = () => {
    setSelectedIds(new Set())
  }

  // Preset zoom change handler
  const handlePresetChange = (presetKey) => {
    const found = SUGGESTED_ZOOM_LEVELS.find((z) => z.preset === presetKey)
    if (found) {
      setZoomConfig((prev) => ({
        ...prev,
        initial_zoom: found.level,
        min_zoom: Math.min(prev.min_zoom, found.level),
        max_zoom: Math.max(prev.max_zoom, found.level),
        preset: found.preset,
      }))
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

  // Counts & Validation
  const selectedRasters = (categories?.raster_layers || []).filter((r) => selectedIds.has(r.id))
  const selectedVectors = (categories?.vector_layers || []).filter((v) => selectedIds.has(v.id))
  const selectedControlPointGroups = (categories?.control_points || []).filter((c) => selectedIds.has(c.id))
  const selectedIconPointGroups = (categories?.icon_points || []).filter((i) => selectedIds.has(i.id))
  const selectedAreaGroups = (categories?.areas || []).filter((a) => selectedIds.has(a.id))
  const isBaseMapSelected = selectedIds.has("base_map_osm")

  const totalControlPointsCount = selectedControlPointGroups.reduce((acc, c) => acc + (c.count || 0), 0)
  const totalIconPointsCount = selectedIconPointGroups.reduce((acc, i) => acc + (i.count || 0), 0)
  const totalAreasCount = selectedAreaGroups.reduce((acc, a) => acc + (a.count || 0), 0)

  // Vector features count estimate
  const totalVectorFeaturesCount = selectedVectors.reduce((acc, v) => {
    const cnt = v.dataset?.extracted_metadata?.feature_count || v.dataset?.extracted_metadata?.record_count || 1
    return acc + cnt
  }, 0)

  // Estimated Size Calculation
  const estimatedSizeMb = useMemo(() => {
    let bytes = 0
    if (isBaseMapSelected) bytes += 3 * 1024 * 1024 // ~3MB base map
    selectedRasters.forEach((r) => {
      const sz = r.dataset?.file_size || 2 * 1024 * 1024
      // JPG is more compressed than PNG
      bytes += advancedSettings.rasterFormat === "jpg" ? Math.round(sz * 0.6) : sz
    })
    selectedVectors.forEach((v) => {
      bytes += v.dataset?.file_size || 1 * 1024 * 1024
    })
    bytes += (totalControlPointsCount + totalIconPointsCount + totalAreasCount) * 1024 // ~1KB each
    return (bytes / (1024 * 1024)).toFixed(1)
  }, [isBaseMapSelected, selectedRasters, selectedVectors, totalControlPointsCount, totalIconPointsCount, totalAreasCount, advancedSettings.rasterFormat])

  // Georeference check
  const primaryDs = datasets.find((d) => d.id === primaryDatasetId)
  const isGeoreferenced =
    primaryDs?.georeference_status === "completed" ||
    primaryDs?.georeference_status === "georeferenced" ||
    Boolean(primaryDs?.extracted_metadata?.bounds && Object.keys(primaryDs.extracted_metadata.bounds).length > 0)

  const hasAnyLayerSelected = selectedRasters.length > 0 || selectedVectors.length > 0 || isBaseMapSelected

  // Handle Export Dispatch
  const handleExportClick = () => {
    if (!hasAnyLayerSelected || exporting) return

    const selectedLayerIds = [
      ...selectedRasters.map((r) => r.id),
      ...selectedVectors.map((v) => v.id),
    ]

    const buildingIds = []
    const floorIds = []
    buildingList.forEach((b) => {
      let bldHasSelected = false
      b.floors.forEach((fl) => {
        const hasSelected = fl.layers.some((l) => selectedIds.has(l.id))
        if (hasSelected) {
          bldHasSelected = true
          floorIds.push(fl.id)
        }
      })
      if (bldHasSelected) buildingIds.push(b.id)
    })

    onExport({
      format: "custom_local_tiles",
      layer_ids: selectedLayerIds,
      building_ids: buildingIds,
      floor_ids: floorIds,
      options: {
        initial_zoom: Number(zoomConfig.initial_zoom),
        min_zoom: Number(zoomConfig.min_zoom),
        max_zoom: Number(zoomConfig.max_zoom),
        preset: zoomConfig.preset,
        tile_size: advancedSettings.tileSize,
        raster_format: advancedSettings.rasterFormat,
        image_quality: advancedSettings.imageQuality,
        bounds_clipping: advancedSettings.boundsClipping,
        include_base_map: isBaseMapSelected,
        include_control_points: selectedControlPointGroups.length > 0,
        include_icons: selectedIconPointGroups.length > 0,
        include_areas: selectedAreaGroups.length > 0,
        include_vector_layers: selectedVectors.length > 0,
        include_layer_styles: true,
        include_building_configuration: true,
        include_project_metadata: true,
      },
    })
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-6 bg-black/60 backdrop-blur-xs animate-in fade-in duration-200">
      <div
        className="relative w-full max-w-4xl max-h-[92vh] flex flex-col bg-white dark:bg-[#1a1a1a] text-neutral-900 dark:text-white rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] overflow-hidden"
        role="dialog"
        aria-modal="true"
        aria-labelledby="export-modal-title"
      >
        {/* =========================================================================
            HEADER
            ========================================================================= */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-200 dark:border-[#2a2a2a] bg-neutral-50 dark:bg-[#151515]">
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center size-10 rounded-[8px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/20 shadow-xs">
              <Download className="size-5" />
            </div>
            <div>
              <h2 id="export-modal-title" className="text-base font-bold text-neutral-900 dark:text-white leading-tight">
                Export Custom Local Tiles
              </h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                Export raster and vector layers together as reusable offline map tiles.
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={exporting}
            className="p-2 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#252525] rounded-[6px] transition-colors cursor-pointer"
            aria-label="Close export modal"
          >
            <X className="size-5" />
          </button>
        </div>

        {/* =========================================================================
            SCROLLABLE BODY
            ========================================================================= */}
        <div className="flex-1 overflow-y-auto p-5 sm:p-6 space-y-5 bg-white dark:bg-[#191919]">
          {/* =========================================================================
              LIVE EXPORT PROGRESS SECTION (Stage 1-8 Backend Reporting)
              ========================================================================= */}
          {exporting && (
            <div className="p-4 rounded-[10px] bg-neutral-900 border border-[#3ecf8e]/40 shadow-lg text-white space-y-4 animate-in fade-in duration-200">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5">
                  <Loader2 className="size-5 text-[#3ecf8e] animate-spin shrink-0" />
                  <div>
                    <div className="text-xs font-bold text-[#3ecf8e] uppercase tracking-wider">
                      Stage {liveProgress?.stage_index || 1} of 8: {liveProgress?.stage || "Processing Custom Local Tiles…"}
                    </div>
                    {liveProgress?.current_layer && (
                      <div className="text-[11px] text-neutral-400 font-mono mt-0.5">
                        Active Layer: <span className="text-white font-semibold">{liveProgress.current_layer}</span>
                      </div>
                    )}
                  </div>
                </div>
                <div className="text-right">
                  <div className="text-sm font-bold font-mono text-[#3ecf8e]">
                    {Math.round(liveProgress?.percent || 5)}%
                  </div>
                  {liveProgress?.estimated_remaining_seconds > 0 && (
                    <div className="text-[10px] text-neutral-400 font-mono">
                      ~{liveProgress.estimated_remaining_seconds}s remaining
                    </div>
                  )}
                </div>
              </div>

              {/* Progress Bar */}
              <div className="w-full bg-neutral-800 rounded-full h-2 overflow-hidden border border-neutral-700">
                <div
                  className="bg-[#3ecf8e] h-full transition-all duration-300 ease-out rounded-full shadow-[0_0_8px_rgba(62,207,142,0.6)]"
                  style={{ width: `${Math.min(100, Math.max(5, liveProgress?.percent || 5))}%` }}
                />
              </div>

              {/* Real-time Metric Badges */}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-center text-xs">
                <div className="p-2 rounded-[6px] bg-neutral-800/80 border border-neutral-700/60">
                  <div className="text-[10px] text-neutral-400">Tiles Processed</div>
                  <div className="text-xs font-bold font-mono text-white mt-0.5">
                    {liveProgress?.completed_tiles || 0} / {liveProgress?.expected_tiles || "?"}
                  </div>
                </div>
                <div className="p-2 rounded-[6px] bg-neutral-800/80 border border-neutral-700/60">
                  <div className="text-[10px] text-neutral-400">Empty Tiles Skipped</div>
                  <div className="text-xs font-bold font-mono text-[#3ecf8e] mt-0.5">
                    {liveProgress?.empty_tiles_skipped || 0}
                  </div>
                </div>
                <div className="p-2 rounded-[6px] bg-neutral-800/80 border border-neutral-700/60">
                  <div className="text-[10px] text-neutral-400">Cached Tiles Reused</div>
                  <div className="text-xs font-bold font-mono text-blue-400 mt-0.5">
                    {liveProgress?.cached_tiles_reused || 0}
                  </div>
                </div>
                <div className="p-2 rounded-[6px] bg-neutral-800/80 border border-neutral-700/60">
                  <div className="text-[10px] text-neutral-400">Elapsed Time</div>
                  <div className="text-xs font-bold font-mono text-neutral-200 mt-0.5">
                    {liveProgress?.duration_seconds ? `${liveProgress.duration_seconds}s` : "Starting…"}
                  </div>
                </div>
              </div>

              {/* 8 Distinct Stages Stepper */}
              <div className="pt-2 border-t border-neutral-800">
                <div className="text-[10px] uppercase tracking-wider font-bold text-neutral-400 mb-2">
                  Pipeline Execution Stages
                </div>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-1.5 text-[11px]">
                  {EXPORT_STAGES.map((st) => {
                    const currentIdx = liveProgress?.stage_index || 1
                    const isDone = st.index < currentIdx
                    const isCurrent = st.index === currentIdx
                    return (
                      <div
                        key={st.index}
                        className={`flex items-center gap-1.5 px-2 py-1 rounded-[4px] border ${
                          isDone
                            ? "bg-[#3ecf8e]/10 border-[#3ecf8e]/30 text-[#3ecf8e]"
                            : isCurrent
                            ? "bg-white/10 border-white/30 text-white font-semibold"
                            : "bg-neutral-800/40 border-neutral-800 text-neutral-500"
                        }`}
                      >
                        {isDone ? (
                          <Check className="size-3 shrink-0 text-[#3ecf8e]" />
                        ) : isCurrent ? (
                          <Loader2 className="size-3 shrink-0 animate-spin text-white" />
                        ) : (
                          <span className="size-3 rounded-full border border-neutral-600 inline-block text-[9px] text-center leading-3">
                            {st.index}
                          </span>
                        )}
                        <span className="truncate">{st.label}</span>
                      </div>
                    )
                  })}
                </div>
              </div>
            </div>
          )}

          {/* Main Card: Custom Local Tiles */}
          <div className="p-4 rounded-[10px] bg-[#3ecf8e]/5 border border-[#3ecf8e]/30 shadow-xs relative overflow-hidden">
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
              <div className="flex-1">
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]">
                    Unified Package • Viewscape Ready
                  </span>
                  <span className="text-[11px] font-mono font-semibold text-neutral-500 dark:text-neutral-400">
                    Custom_Local_Tiles.zip
                  </span>
                </div>
                <h3 className="text-sm font-bold text-neutral-900 dark:text-white">Custom Local Tiles</h3>
                <p className="text-xs text-neutral-600 dark:text-neutral-300 mt-0.5 leading-relaxed">
                  Export selected raster and vector layers into one reusable local tile package compatible with Viewscape.
                </p>

                {/* Feature Pills */}
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 mt-3 text-[11px] text-neutral-700 dark:text-neutral-300">
                  <div className="flex items-center gap-1.5">
                    <Check className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                    <span><strong>Raster tiles:</strong> PNG/JPG</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Check className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                    <span><strong>Vector tiles:</strong> MVT/PBF</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Check className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                    <span><strong>Point metadata:</strong> GeoJSON</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Check className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                    <span><strong>Layer styles:</strong> MapLibre v8</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Check className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                    <span><strong>Multifloor:</strong> Config preserved</span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Check className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                    <span><strong>Offline:</strong> 100% self-contained</span>
                  </div>
                </div>
              </div>

              {/* Graphic Preview Illustration */}
              <div className="hidden sm:flex size-24 shrink-0 rounded-[8px] bg-neutral-100 dark:bg-[#151515] border border-neutral-200 dark:border-[#2e2e2e] items-center justify-center p-1.5 shadow-inner">
                <svg viewBox="0 0 80 80" className="w-full h-full text-[#3ecf8e]">
                  {/* Grid Lines */}
                  <line x1="10" y1="28" x2="70" y2="28" stroke="currentColor" strokeWidth="0.5" opacity="0.2" />
                  <line x1="10" y1="52" x2="70" y2="52" stroke="currentColor" strokeWidth="0.5" opacity="0.2" />
                  <line x1="28" y1="10" x2="28" y2="70" stroke="currentColor" strokeWidth="0.5" opacity="0.2" />
                  <line x1="52" y1="10" x2="52" y2="70" stroke="currentColor" strokeWidth="0.5" opacity="0.2" />
                  {/* Raster Base Tile Stack */}
                  <polygon points="16,56 40,68 64,56 40,44" fill="#3ecf8e" opacity="0.3" stroke="#24b47e" strokeWidth="1" />
                  <polygon points="16,42 40,54 64,42 40,30" fill="#3ecf8e" opacity="0.6" stroke="#24b47e" strokeWidth="1" />
                  {/* Vector Geometry Lines & Polygons */}
                  <polygon points="28,38 42,32 50,39 36,45" fill="none" stroke="#60A5FA" strokeWidth="1.2" strokeDasharray="2 1" />
                  {/* Vector Points */}
                  <circle cx="40" cy="48" r="2.5" fill="#EF4444" stroke="#FFFFFF" strokeWidth="0.8" />
                  <circle cx="48" cy="36" r="2.5" fill="#F59E0B" stroke="#FFFFFF" strokeWidth="0.8" />
                  <circle cx="32" cy="40" r="2" fill="#3B82F6" stroke="#FFFFFF" strokeWidth="0.8" />
                </svg>
              </div>
            </div>
          </div>

          {/* =========================================================================
              LAYER SELECTION HIERARCHY (6 CATEGORIES)
              ========================================================================= */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Layers className="size-4 text-[#3ecf8e]" />
                <span className="text-xs font-bold uppercase tracking-wider text-neutral-700 dark:text-neutral-300">
                  Layer Selection
                </span>
                <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-neutral-100 dark:bg-[#252525] text-neutral-600 dark:text-neutral-400 font-medium">
                  {selectedIds.size} items selected
                </span>
              </div>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={handleSelectAll}
                  className="px-2 py-1 text-xs font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-[#242424] hover:bg-neutral-200 dark:hover:bg-[#2e2e2e] rounded-[5px] transition-colors cursor-pointer"
                >
                  Select All
                </button>
                <button
                  type="button"
                  onClick={handleDeselectAll}
                  className="px-2 py-1 text-xs font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-[#242424] hover:bg-neutral-200 dark:hover:bg-[#2e2e2e] rounded-[5px] transition-colors cursor-pointer"
                >
                  Deselect All
                </button>
              </div>
            </div>

            {/* Category Groups List */}
            <div className="space-y-2">
              {categoryDefs.map((cat) => {
                const isCollapsed = collapsedCategories.has(cat.key)
                const items = cat.items
                const selectedCount = items.filter((it) => selectedIds.has(it.id)).length
                const allSelected = items.length > 0 && selectedCount === items.length
                const someSelected = selectedCount > 0 && selectedCount < items.length
                const CatIcon = cat.icon

                return (
                  <div
                    key={cat.key}
                    className="rounded-[8px] border border-neutral-200 dark:border-[#2a2a2a] bg-neutral-50/60 dark:bg-[#202020] overflow-hidden shadow-2xs"
                  >
                    {/* Category Header */}
                    <div
                      onClick={() => toggleCollapse(cat.key)}
                      className="flex items-center justify-between px-3.5 py-2.5 bg-neutral-100/70 dark:bg-[#232323] cursor-pointer hover:bg-neutral-200/50 dark:hover:bg-[#292929] transition-colors select-none"
                    >
                      <div className="flex items-center gap-2.5">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            toggleCategory(cat.key, items)
                          }}
                          disabled={items.length === 0}
                          className={`size-4 rounded-[4px] flex items-center justify-center border transition-colors cursor-pointer ${
                            allSelected
                              ? "bg-[#3ecf8e] border-[#3ecf8e] text-[#171717]"
                              : someSelected
                              ? "bg-[#3ecf8e]/30 border-[#3ecf8e] text-[#24b47e] dark:text-[#3ecf8e]"
                              : "border-neutral-300 dark:border-[#3a3a3a] bg-white dark:bg-[#1a1a1a]"
                          }`}
                          aria-label={`Toggle all in ${cat.title}`}
                        >
                          {allSelected && <Check className="size-2.5 stroke-[3]" />}
                          {someSelected && <div className="size-1.5 bg-[#3ecf8e] rounded-[1px]" />}
                        </button>

                        <CatIcon className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                        <div>
                          <span className="text-xs font-bold text-neutral-900 dark:text-white">
                            {cat.title}
                          </span>
                          <span className="text-[10px] text-neutral-500 dark:text-neutral-400 ml-1.5">
                            ({selectedCount}/{items.length})
                          </span>
                        </div>
                      </div>

                      <div className="flex items-center gap-2">
                        <span className="hidden sm:inline text-[10px] text-neutral-500 dark:text-neutral-400">
                          {cat.description}
                        </span>
                        {isCollapsed ? (
                          <ChevronRight className="size-4 text-neutral-400" />
                        ) : (
                          <ChevronDown className="size-4 text-neutral-400" />
                        )}
                      </div>
                    </div>

                    {/* Category Items */}
                    {!isCollapsed && (
                      <div className="divide-y divide-neutral-100 dark:divide-[#272727]">
                        {items.length === 0 ? (
                          <div className="px-4 py-3 text-xs text-neutral-400 dark:text-neutral-500 italic">
                            No {cat.title.toLowerCase()} present in this workspace.
                          </div>
                        ) : (
                          items.map((item) => {
                            const isChecked = selectedIds.has(item.id)

                            return (
                              <div
                                key={item.id}
                                onClick={() => toggleItem(item.id, cat.key, item.rasterId)}
                                className={`flex items-center justify-between px-3.5 py-2 cursor-pointer transition-colors ${
                                  isChecked
                                    ? "bg-[#3ecf8e]/5 hover:bg-[#3ecf8e]/10"
                                    : "hover:bg-neutral-100/50 dark:hover:bg-[#252525]"
                                }`}
                              >
                                <div className="flex items-center gap-2.5 min-w-0 flex-1">
                                  <SpringCheck
                                    checked={isChecked}
                                    onChange={() => toggleItem(item.id, cat.key, item.rasterId)}
                                    boxSize={16}
                                    boxRadius={4}
                                    fillColor="#3ecf8e"
                                    checkColor="#141414"
                                    strike="none"
                                    label=""
                                    ariaLabel={`Select ${item.name}`}
                                  />
                                  <span className="text-xs font-medium text-neutral-900 dark:text-white truncate">
                                    {item.name}
                                  </span>
                                  {((item.count !== undefined && item.count > 0) || (item.featureCount !== undefined && item.featureCount > 0)) && (
                                    <span className="text-[10px] font-mono px-1.5 py-0.2 rounded bg-neutral-100 dark:bg-[#2a2a2a] text-neutral-600 dark:text-neutral-300">
                                      {item.count ?? item.featureCount} {item.geometryType ? `${item.geometryType.toLowerCase()}${item.featureCount !== 1 ? "s" : ""}` : "pts"}
                                    </span>
                                  )}
                                </div>

                                <div className="flex items-center gap-2 shrink-0">
                                  {/* Associated Floor / Raster */}
                                  {item.associated && (
                                    <span className="hidden md:inline-block text-[10px] text-neutral-500 dark:text-neutral-400 truncate max-w-[130px]">
                                      {item.associated}
                                    </span>
                                  )}

                                  {/* Original format badge */}
                                  <span className="text-[9px] font-mono font-bold uppercase px-1.5 py-0.5 rounded bg-neutral-200/80 dark:bg-[#2c2c2c] text-neutral-700 dark:text-neutral-300">
                                    {item.format}
                                  </span>

                                  {/* Visibility status */}
                                  <span
                                    className={`text-[9px] font-semibold px-1.5 py-0.5 rounded ${
                                      item.visible !== false
                                        ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
                                        : "bg-neutral-200 dark:bg-[#2e2e2e] text-neutral-500"
                                    }`}
                                  >
                                    {item.visible !== false ? "Visible" : "Hidden"}
                                  </span>
                                </div>
                              </div>
                            )
                          })
                        )}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </div>

          {/* =========================================================================
              COLLAPSIBLE ADVANCED SETTINGS
              ========================================================================= */}
          <div className="rounded-[8px] border border-neutral-200 dark:border-[#2a2a2a] bg-neutral-50/40 dark:bg-[#1f1f1f] overflow-hidden">
            <button
              type="button"
              onClick={() => setShowAdvanced((prev) => !prev)}
              className="w-full flex items-center justify-between px-4 py-2.5 hover:bg-neutral-100 dark:hover:bg-[#262626] transition-colors cursor-pointer select-none"
            >
              <div className="flex items-center gap-2 text-xs font-bold text-neutral-800 dark:text-neutral-200">
                <Sliders className="size-4 text-[#3ecf8e]" />
                <span>Advanced Settings</span>
                <span className="text-[10px] text-neutral-400 font-normal">
                  (Zoom ladder, tile dimensions, format, compression)
                </span>
              </div>
              {showAdvanced ? (
                <ChevronDown className="size-4 text-neutral-400" />
              ) : (
                <ChevronRight className="size-4 text-neutral-400" />
              )}
            </button>

            {showAdvanced && (
              <div className="p-4 border-t border-neutral-200 dark:border-[#2a2a2a] space-y-4 bg-white dark:bg-[#1a1a1a] text-xs">
                {/* Zoom Presets & Range */}
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-semibold text-neutral-900 dark:text-white">
                      Camera &amp; Tile Zoom Range
                    </span>
                    <button
                      type="button"
                      onClick={handleResetZoom}
                      className="text-[11px] text-[#24b47e] dark:text-[#3ecf8e] hover:underline flex items-center gap-1 cursor-pointer"
                    >
                      <RotateCcw className="size-3" />
                      <span>Reset</span>
                    </button>
                  </div>

                  {/* Preset quick buttons */}
                  <div className="grid grid-cols-3 sm:grid-cols-6 gap-1.5 mb-3">
                    {[
                      { key: "house", label: "House (z21)" },
                      { key: "building", label: "Building (z20)" },
                      { key: "street", label: "Street (z19)" },
                      { key: "neighborhood", label: "Neighborhood (z18)" },
                      { key: "city", label: "City (z15)" },
                      { key: "regional", label: "Regional (z13)" },
                    ].map((p) => (
                      <button
                        key={p.key}
                        type="button"
                        onClick={() => handlePresetChange(p.key)}
                        className={`py-1 px-2 rounded-[5px] text-[11px] font-medium border text-center transition-all cursor-pointer ${
                          zoomConfig.preset === p.key
                            ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-bold"
                            : "bg-neutral-100 dark:bg-[#252525] border-neutral-200 dark:border-[#333] text-neutral-800 dark:text-neutral-200 hover:bg-neutral-200"
                        }`}
                      >
                        {p.label}
                      </button>
                    ))}
                  </div>

                  {/* Inputs */}
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                    <div>
                      <label className="text-[11px] text-neutral-500 dark:text-neutral-400 block mb-1">
                        Initial Zoom: <strong>z{zoomConfig.initial_zoom}</strong>
                      </label>
                      <input
                        type="number"
                        min="0"
                        max="24"
                        value={zoomConfig.initial_zoom}
                        onChange={(e) =>
                          setZoomConfig((p) => ({ ...p, initial_zoom: parseInt(e.target.value, 10) || 0 }))
                        }
                        className="w-full px-2.5 py-1.5 font-mono text-xs rounded-[6px] border border-neutral-200 dark:border-[#333] bg-neutral-50 dark:bg-[#222] text-neutral-900 dark:text-white"
                      />
                    </div>
                    <div>
                      <label className="text-[11px] text-neutral-500 dark:text-neutral-400 block mb-1">
                        Min Zoom: <strong>z{zoomConfig.min_zoom}</strong>
                      </label>
                      <input
                        type="number"
                        min="0"
                        max="24"
                        value={zoomConfig.min_zoom}
                        onChange={(e) =>
                          setZoomConfig((p) => ({ ...p, min_zoom: parseInt(e.target.value, 10) || 0 }))
                        }
                        className="w-full px-2.5 py-1.5 font-mono text-xs rounded-[6px] border border-neutral-200 dark:border-[#333] bg-neutral-50 dark:bg-[#222] text-neutral-900 dark:text-white"
                      />
                    </div>
                    <div>
                      <label className="text-[11px] text-neutral-500 dark:text-neutral-400 block mb-1">
                        Max Zoom: <strong>z{zoomConfig.max_zoom}</strong>
                      </label>
                      <input
                        type="number"
                        min="0"
                        max="24"
                        value={zoomConfig.max_zoom}
                        onChange={(e) =>
                          setZoomConfig((p) => ({ ...p, max_zoom: parseInt(e.target.value, 10) || 0 }))
                        }
                        className="w-full px-2.5 py-1.5 font-mono text-xs rounded-[6px] border border-neutral-200 dark:border-[#333] bg-neutral-50 dark:bg-[#222] text-neutral-900 dark:text-white"
                      />
                    </div>
                  </div>
                </div>

                {/* Tile & Compression Options */}
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-2 border-t border-neutral-100 dark:border-[#272727]">
                  {/* Tile Size */}
                  <div>
                    <label className="text-[11px] text-neutral-500 dark:text-neutral-400 block mb-1 font-semibold">
                      Tile Dimensions
                    </label>
                    <select
                      value={advancedSettings.tileSize}
                      onChange={(e) =>
                        setAdvancedSettings((p) => ({ ...p, tileSize: parseInt(e.target.value, 10) || 256 }))
                      }
                      className="w-full px-2.5 py-1.5 text-xs rounded-[6px] border border-neutral-200 dark:border-[#333] bg-neutral-50 dark:bg-[#222] text-neutral-900 dark:text-white cursor-pointer"
                    >
                      <option value="256">256 × 256 (Standard)</option>
                      <option value="512">512 × 512 (High Resolution)</option>
                    </select>
                  </div>

                  {/* Raster Format */}
                  <div>
                    <label className="text-[11px] text-neutral-500 dark:text-neutral-400 block mb-1 font-semibold">
                      Raster Tile Format
                    </label>
                    <select
                      value={advancedSettings.rasterFormat}
                      onChange={(e) =>
                        setAdvancedSettings((p) => ({ ...p, rasterFormat: e.target.value }))
                      }
                      className="w-full px-2.5 py-1.5 text-xs rounded-[6px] border border-neutral-200 dark:border-[#333] bg-neutral-50 dark:bg-[#222] text-neutral-900 dark:text-white cursor-pointer"
                    >
                      <option value="png">PNG (Lossless Transparency)</option>
                      <option value="jpg">JPG (Compact File Size)</option>
                    </select>
                  </div>

                  {/* Image Quality */}
                  <div>
                    <div className="flex items-center justify-between mb-1">
                      <label className="text-[11px] text-neutral-500 dark:text-neutral-400 font-semibold">
                        Image Quality
                      </label>
                      <span className="text-[11px] font-mono text-neutral-700 dark:text-neutral-300">
                        {advancedSettings.imageQuality}%
                      </span>
                    </div>
                    <input
                      type="range"
                      min="50"
                      max="100"
                      value={advancedSettings.imageQuality}
                      onChange={(e) =>
                        setAdvancedSettings((p) => ({
                          ...p,
                          imageQuality: parseInt(e.target.value, 10) || 85,
                        }))
                      }
                      className="w-full accent-[#3ecf8e] cursor-pointer"
                    />
                  </div>
                </div>
              </div>
            )}
          </div>

          {/* =========================================================================
              VALIDATION SUMMARY & DIAGNOSTICS
              ========================================================================= */}
          <div className="p-3.5 rounded-[8px] bg-neutral-50 dark:bg-[#202020] border border-neutral-200 dark:border-[#2a2a2a] space-y-2.5">
            <div className="flex items-center justify-between">
              <span className="text-xs font-bold uppercase tracking-wider text-neutral-600 dark:text-neutral-400">
                Export Package Summary
              </span>
              <button
                type="button"
                onClick={() => setShowDiagnostics((p) => !p)}
                className="text-[11px] text-[#24b47e] dark:text-[#3ecf8e] hover:underline flex items-center gap-1 cursor-pointer"
              >
                <Info className="size-3.5" />
                <span>{showDiagnostics ? "Hide Details" : "Diagnostics & Metadata"}</span>
              </button>
            </div>

            {/* Metrics Chips */}
            <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-2 text-center text-xs">
              <div className="p-2 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c]">
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400">Rasters</div>
                <div className="text-sm font-bold text-neutral-900 dark:text-white mt-0.5">
                  {selectedRasters.length}
                </div>
              </div>
              <div className="p-2 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c]">
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400">Vectors</div>
                <div className="text-sm font-bold text-neutral-900 dark:text-white mt-0.5">
                  {selectedVectors.length}
                </div>
              </div>
              <div className="p-2 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c]">
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400">Features</div>
                <div className="text-sm font-bold text-neutral-900 dark:text-white mt-0.5">
                  {totalVectorFeaturesCount}
                </div>
              </div>
              <div className="p-2 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c]">
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400">Control Pts</div>
                <div className="text-sm font-bold text-neutral-900 dark:text-white mt-0.5">
                  {totalControlPointsCount}
                </div>
              </div>
              <div className="p-2 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c]">
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400">Icon Points</div>
                <div className="text-sm font-bold text-neutral-900 dark:text-white mt-0.5">
                  {totalIconPointsCount}
                </div>
              </div>
              <div className="p-2 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c]">
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400">Zoom Range</div>
                <div className="text-sm font-bold font-mono text-neutral-900 dark:text-white mt-0.5">
                  z{zoomConfig.min_zoom}–z{zoomConfig.max_zoom}
                </div>
              </div>
              <div className="p-2 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c]">
                <div className="text-[10px] text-neutral-500 dark:text-neutral-400">Est. Size</div>
                <div className="text-sm font-bold text-[#24b47e] dark:text-[#3ecf8e] mt-0.5">
                  ~{estimatedSizeMb} MB
                </div>
              </div>
            </div>

            {/* Warnings */}
            {!isGeoreferenced && (
              <div className="p-2.5 rounded-[6px] bg-amber-500/10 border border-amber-500/20 text-xs text-amber-600 dark:text-amber-400 flex items-start gap-2">
                <ShieldAlert className="size-4 shrink-0 text-amber-500 mt-0.5" />
                <div>
                  <span className="font-semibold">Uncalibrated Raster:</span> Ground control points are missing or incomplete. Tiling bounds and geographic alignment may be approximate.
                </div>
              </div>
            )}

            {!hasAnyLayerSelected && (
              <div className="p-2.5 rounded-[6px] bg-red-500/10 border border-red-500/20 text-xs text-red-600 dark:text-red-400 flex items-center gap-2">
                <AlertCircle className="size-4 shrink-0 text-red-500" />
                <span>At least one layer must be selected for export.</span>
              </div>
            )}

            {/* Expandable Diagnostics Details */}
            {showDiagnostics && (
              <div className="p-3 rounded-[6px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2c2c2c] text-[11px] font-mono text-neutral-600 dark:text-neutral-300 space-y-1.5 animate-in fade-in duration-150">
                <div><strong>Project Name:</strong> {manifest?.name || primaryDs?.filename || "Veracity Project"}</div>
                <div><strong>Primary Dataset:</strong> {primaryDs?.id || "None"} ({primaryDs?.filename || "N/A"})</div>
                <div><strong>Tile Dimensions:</strong> {advancedSettings.tileSize}×{advancedSettings.tileSize}px ({advancedSettings.rasterFormat.toUpperCase()})</div>
                <div><strong>Coordinate Reference:</strong> EPSG:3857 (Web Mercator) / EPSG:4326 WGS84</div>
                <div><strong>Package Archive:</strong> Custom_Local_Tiles.zip with dual-root compatibility</div>
              </div>
            )}

            {error && (
              <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-[6px] text-xs text-red-600 dark:text-red-400 flex items-start gap-2">
                <AlertCircle className="size-4 shrink-0 text-red-500 mt-0.5" />
                <div>
                  <div className="font-semibold">{error.message || "Export error occurred"}</div>
                  {error.details && <div className="text-red-500/80 mt-0.5">{error.details}</div>}
                </div>
              </div>
            )}
          </div>
        </div>

        {/* =========================================================================
            FIXED FOOTER
            ========================================================================= */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-neutral-200 dark:border-[#2a2a2a] bg-neutral-50 dark:bg-[#151515]">
          <div className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
            <span>Estimated Package:</span>
            <strong className="text-neutral-900 dark:text-white font-mono">~{estimatedSizeMb} MB</strong>
            <span className="hidden sm:inline text-neutral-400">({selectedIds.size} items)</span>
          </div>

          <div className="flex items-center gap-3">
            {exporting ? (
              <button
                type="button"
                onClick={handleCancelExport}
                disabled={isCancelling}
                className="px-4 py-2 text-xs font-semibold text-red-600 dark:text-red-400 hover:bg-red-500/10 rounded-[6px] border border-red-500/30 transition-colors cursor-pointer disabled:opacity-50"
              >
                {isCancelling ? "Cancelling…" : "Cancel Export"}
              </button>
            ) : (
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 text-xs font-semibold text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#252525] rounded-[6px] transition-colors cursor-pointer"
              >
                Cancel
              </button>
            )}

            <button
              type="button"
              onClick={handleExportClick}
              disabled={!hasAnyLayerSelected || exporting}
              className="flex items-center gap-2 px-5 py-2 text-xs font-bold text-[#141414] bg-[#3ecf8e] hover:bg-[#34b77c] disabled:opacity-50 disabled:hover:bg-[#3ecf8e] rounded-[6px] shadow-sm transition-all cursor-pointer disabled:cursor-not-allowed"
            >
              {exporting ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  <span>
                    {liveProgress?.stage || "Packaging Custom Local Tiles…"} ({Math.round(liveProgress?.percent || 5)}%)
                  </span>
                </>
              ) : (
                <>
                  <Download className="size-4" />
                  <span>Export Custom Local Tiles</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
