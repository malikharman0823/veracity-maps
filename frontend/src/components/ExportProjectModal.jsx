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
} from "lucide-react"
import { datasetSourceImageUrl } from "../api"
import { buildExportHierarchy } from "../lib/exportHierarchy"

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


  // Selection state
  const [selectedLayerIds, setSelectedLayerIds] = useState(new Set())
  const [selectedPreviewLayerId, setSelectedPreviewLayerId] = useState(null)
  const [collapsedFloors, setCollapsedFloors] = useState(new Set())

  // Export options state
  const [options, setOptions] = useState({
    include_building_configuration: true,
    include_control_points: true,
    include_icons: true,
    include_layer_styles: true,
    include_original_rasters: true,
    include_processed_rasters: true,
    include_base_map_settings: false,
    include_project_metadata: true,
  })

  // Initialize selection when opening
  useEffect(() => {
    if (open) {
      const init = new Set(allLayerIds)
      setSelectedLayerIds(init)
      if (allLayerIds.length > 0) {
        setSelectedPreviewLayerId(primaryDatasetId && allLayerIds.includes(primaryDatasetId) ? primaryDatasetId : allLayerIds[0])
      }
    }
  }, [open, allLayerIds, primaryDatasetId])

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

  const getFloorSelectionState = (floor) => {
    const layerIds = floor.layers.map((l) => l.id)
    if (layerIds.length === 0) return "none"
    const selectedCount = layerIds.filter((id) => selectedLayerIds.has(id)).length
    if (selectedCount === 0) return "none"
    if (selectedCount === layerIds.length) return "all"
    return "some"
  }

  const toggleFloor = (floor) => {
    const state = getFloorSelectionState(floor)
    const next = new Set(selectedLayerIds)
    const layerIds = floor.layers.map((l) => l.id)
    if (state === "all") {
      layerIds.forEach((id) => next.delete(id))
    } else {
      layerIds.forEach((id) => next.add(id))
      if (layerIds[0]) setSelectedPreviewLayerId(layerIds[0])
    }
    setSelectedLayerIds(next)
  }

  const getBuildingSelectionState = (bld) => {
    const layerIds = []
    bld.floors.forEach((fl) => fl.layers.forEach((l) => layerIds.push(l.id)))
    if (layerIds.length === 0) return "none"
    const selectedCount = layerIds.filter((id) => selectedLayerIds.has(id)).length
    if (selectedCount === 0) return "none"
    if (selectedCount === layerIds.length) return "all"
    return "some"
  }

  const toggleBuilding = (bld) => {
    const state = getBuildingSelectionState(bld)
    const next = new Set(selectedLayerIds)
    const layerIds = []
    bld.floors.forEach((fl) => fl.layers.forEach((l) => layerIds.push(l.id)))
    if (state === "all") {
      layerIds.forEach((id) => next.delete(id))
    } else {
      layerIds.forEach((id) => next.add(id))
      if (layerIds[0]) setSelectedPreviewLayerId(layerIds[0])
    }
    setSelectedLayerIds(next)
  }

  const selectAll = () => {
    setSelectedLayerIds(new Set(allLayerIds))
  }

  const deselectAll = () => {
    setSelectedLayerIds(new Set())
  }

  const toggleCollapseFloor = (floorId) => {
    const next = new Set(collapsedFloors)
    if (next.has(floorId)) {
      next.delete(floorId)
    } else {
      next.add(floorId)
    }
    setCollapsedFloors(next)
  }

  // Active layer for preview
  const activePreviewLayer = (() => {
    // Check standalone layers first
    for (const l of standaloneLayers) {
      if (l.id === selectedPreviewLayerId) {
        return { ...l, floor: null, building: null }
      }
    }
    // Check buildings
    for (const b of buildingList) {
      for (const fl of b.floors) {
        for (const l of fl.layers) {
          if (l.id === selectedPreviewLayerId) {
            return { ...l, floor: fl, building: b }
          }
        }
      }
    }
    return null
  })()

  // Calculate total counts
  const totalSelectedCount = selectedLayerIds.size
  const totalFloorsCount = buildingList.reduce((acc, b) => acc + b.floors.length, 0)
  const totalLayersCount = allLayerIds.length

  // Calculate estimated size
  const estimatedSizeMb = (() => {
    let bytes = 0
    selectedLayerIds.forEach((id) => {
      const ds = (datasets || []).find((d) => d.id === id)
      if (ds?.file_size) {
        bytes += ds.file_size
      } else {
        bytes += 2 * 1024 * 1024 // ~2MB default
      }
    })
    return (bytes / (1024 * 1024)).toFixed(1)
  })()

  const handleExportClick = () => {
    if (totalSelectedCount === 0 || exporting) return

    // Collect building IDs and floor IDs associated with selected layers
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
      if (bldHasSelected) {
        buildingIds.push(b.id)
      }
    })

    onExport({
      building_ids: buildingIds,
      floor_ids: floorIds,
      layer_ids: Array.from(selectedLayerIds),
      options: options,
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-6 bg-slate-900/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div
        className="relative w-full max-w-5xl max-h-[92vh] flex flex-col bg-white rounded-2xl shadow-2xl border border-slate-200 overflow-hidden"
        role="dialog"
        aria-modal="true"
        aria-labelledby="export-modal-title"
      >
        {/* Header */}
        <div className="flex items-start justify-between px-6 py-5 border-b border-slate-100 bg-white">
          <div className="flex items-center gap-3.5">
            <div className="flex items-center justify-center size-11 rounded-xl bg-blue-50 text-blue-600 border border-blue-100/80 shadow-xs">
              <Download className="size-5.5" />
            </div>
            <div>
              <h2 id="export-modal-title" className="text-xl font-bold text-slate-900 leading-tight">
                Export Map Project
              </h2>
              <p className="text-xs sm:text-sm text-slate-500 mt-0.5">
                Select the layers and data to include in the exported ZIP file. All transformations, styles, icons, and control points will be saved.
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

        {/* Content Body: Split Left / Right */}
        <div className="flex-1 overflow-y-auto grid grid-cols-1 lg:grid-cols-12 divide-y lg:divide-y-0 lg:divide-x divide-slate-100">
          {/* Left Column: Hierarchical Layer Selection (7 cols) */}
          <div className="lg:col-span-7 p-6 flex flex-col gap-4 bg-slate-50/50">
            <div className="flex items-center justify-between">
              <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                Layer Hierarchy
              </div>
              <div className="text-xs text-slate-500 font-medium">
                {totalSelectedCount} of {totalLayersCount} layers selected
              </div>
            </div>

            {/* Tree Container */}
            <div className="flex flex-col gap-3 min-h-[340px]">
              {/* Standalone / Workspace Raster Layers (e.g. single floorplan.png, processed transparent, etc.) */}
              {standaloneLayers.length > 0 && (
                <div className="bg-white rounded-xl border border-slate-200 shadow-xs overflow-hidden">
                  {buildingList.length > 0 && (
                    <div className="flex items-center gap-2 px-4 py-2.5 bg-slate-100/60 border-b border-slate-100 text-xs font-semibold uppercase tracking-wider text-slate-500">
                      <Layers className="size-4 text-slate-400" />
                      <span>Workspace Layers ({standaloneLayers.length})</span>
                    </div>
                  )}

                  <div className="divide-y divide-slate-100 p-2 flex flex-col gap-1">
                    {standaloneLayers.map((lyr) => {
                      const selected = isLayerSelected(lyr.id)
                      const isActivePreview = lyr.id === selectedPreviewLayerId
                      return (
                        <div
                          key={lyr.id}
                          onClick={() => setSelectedPreviewLayerId(lyr.id)}
                          className={`flex items-center gap-3 p-2.5 rounded-lg cursor-pointer transition-all border ${
                            isActivePreview
                              ? "bg-blue-50/70 border-blue-200 shadow-xs"
                              : "hover:bg-slate-50 border-transparent"
                          }`}
                        >
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation()
                              toggleLayer(lyr.id)
                            }}
                            className={`size-4 rounded flex items-center justify-center border transition-colors ${
                              selected
                                ? "bg-blue-600 border-blue-600 text-white"
                                : "border-slate-300 bg-white hover:border-slate-400"
                            }`}
                          >
                            {selected && <Check className="size-2.5 stroke-[3]" />}
                          </button>

                          <div className="size-8 rounded bg-slate-100 border border-slate-200 overflow-hidden shrink-0 flex items-center justify-center">
                            <img
                              src={datasetSourceImageUrl(lyr.id)}
                              alt=""
                              className="size-full object-cover"
                              onError={(e) => {
                                e.target.style.display = "none"
                              }}
                            />
                          </div>

                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-1.5">
                              <span className="text-xs font-semibold text-slate-800 truncate">
                                {lyr.name}
                              </span>
                              {lyr.isPrimary && (
                                <span className="text-[10px] font-medium px-1.5 py-0.2 rounded bg-blue-100 text-blue-700">
                                  Primary
                                </span>
                              )}
                            </div>
                            <div className="text-[11px] text-slate-400 flex items-center gap-1.5 mt-0.5">
                              <span>Opacity {Math.round(lyr.opacity * 100)}%</span>
                              {lyr.bgRemoved && (
                                <>
                                  <span>•</span>
                                  <span className="text-emerald-600 flex items-center gap-0.5">
                                    <Sparkles className="size-2.5" /> BG Removed
                                  </span>
                                </>
                              )}
                              {lyr.rotated && (
                                <>
                                  <span>•</span>
                                  <span className="text-indigo-600 flex items-center gap-0.5">
                                    <RotateCw className="size-2.5" /> Rotated {Math.round(lyr.rotationAngle)}°
                                  </span>
                                </>
                              )}
                            </div>
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}

              {/* Building Hierarchies (only if real buildings are present in workspace) */}
              {buildingList.map((bld) => {
                const bldState = getBuildingSelectionState(bld)
                return (
                  <div key={bld.id} className="bg-white rounded-xl border border-slate-200 shadow-xs overflow-hidden">
                    {/* Building Header */}
                    <div className="flex items-center gap-3 px-4 py-3 bg-blue-50/40 border-b border-slate-100">
                      <button
                        type="button"
                        onClick={() => toggleBuilding(bld)}
                        className={`size-5 rounded flex items-center justify-center border transition-colors ${
                          bldState === "all"
                            ? "bg-blue-600 border-blue-600 text-white"
                            : bldState === "some"
                            ? "bg-blue-600 border-blue-600 text-white"
                            : "border-slate-300 bg-white hover:border-slate-400"
                        }`}
                      >
                        {bldState === "all" && <Check className="size-3.5 stroke-[3]" />}
                        {bldState === "some" && <Minus className="size-3.5 stroke-[3]" />}
                      </button>

                      <Building2 className="size-5 text-blue-600" />
                      <div className="flex-1 min-w-0">
                        <span className="font-semibold text-slate-900 text-sm">{bld.name}</span>
                        <span className="text-xs text-slate-500 ml-2">
                          {bld.floors.length} floor{bld.floors.length !== 1 ? "s" : ""} • {bld.floors.reduce((acc, f) => acc + f.layers.length, 0)} layer{bld.floors.reduce((acc, f) => acc + f.layers.length, 0) !== 1 ? "s" : ""}
                        </span>
                      </div>
                    </div>

                    {/* Floors List */}
                    <div className="divide-y divide-slate-100">
                      {bld.floors.map((fl) => {
                        const flState = getFloorSelectionState(fl)
                        const isCollapsed = collapsedFloors.has(fl.id)
                        return (
                          <div key={fl.id} className="flex flex-col">
                            {/* Floor Header Item */}
                            <div className="flex items-center gap-3 px-4 py-2.5 hover:bg-slate-50/80 transition-colors">
                              <button
                                type="button"
                                onClick={() => toggleCollapseFloor(fl.id)}
                                className="p-0.5 text-slate-400 hover:text-slate-600"
                              >
                                {isCollapsed ? (
                                  <ChevronRight className="size-4" />
                                ) : (
                                  <ChevronDown className="size-4" />
                                )}
                              </button>

                              <button
                                type="button"
                                onClick={() => toggleFloor(fl)}
                                className={`size-4.5 rounded flex items-center justify-center border transition-colors ${
                                  flState === "all"
                                    ? "bg-blue-600 border-blue-600 text-white"
                                    : flState === "some"
                                    ? "bg-blue-600 border-blue-600 text-white"
                                    : "border-slate-300 bg-white hover:border-slate-400"
                                }`}
                              >
                                {flState === "all" && <Check className="size-3 stroke-[3]" />}
                                {flState === "some" && <Minus className="size-3 stroke-[3]" />}
                              </button>

                              {/* Floor Mini Thumbnail */}
                              <div className="size-7 rounded bg-slate-100 border border-slate-200 overflow-hidden flex items-center justify-center shrink-0">
                                {fl.layers[0] ? (
                                  <img
                                    src={datasetSourceImageUrl(fl.layers[0].id)}
                                    alt=""
                                    className="size-full object-cover"
                                    onError={(e) => {
                                      e.target.style.display = "none"
                                    }}
                                  />
                                ) : (
                                  <Image className="size-3.5 text-slate-400" />
                                )}
                              </div>

                              <div className="flex-1 min-w-0">
                                <span className="text-sm font-medium text-slate-800">{fl.name}</span>
                                <span className="text-xs text-slate-400 ml-2">
                                  ({fl.layers.length} layer{fl.layers.length !== 1 ? "s" : ""})
                                </span>
                              </div>
                            </div>

                            {/* Raster Layers in this Floor */}
                            {!isCollapsed && (
                              <div className="pl-12 pr-4 pb-2 flex flex-col gap-1.5">
                                {fl.layers.map((lyr) => {
                                  const selected = isLayerSelected(lyr.id)
                                  const isActivePreview = lyr.id === selectedPreviewLayerId
                                  return (
                                    <div
                                      key={lyr.id}
                                      onClick={() => setSelectedPreviewLayerId(lyr.id)}
                                      className={`flex items-center gap-3 p-2 rounded-lg cursor-pointer transition-all border ${
                                        isActivePreview
                                          ? "bg-blue-50/70 border-blue-200 shadow-xs"
                                          : "hover:bg-slate-100/70 border-transparent"
                                      }`}
                                    >
                                      <button
                                        type="button"
                                        onClick={(e) => {
                                          e.stopPropagation()
                                          toggleLayer(lyr.id)
                                        }}
                                        className={`size-4 rounded flex items-center justify-center border transition-colors ${
                                          selected
                                            ? "bg-blue-600 border-blue-600 text-white"
                                            : "border-slate-300 bg-white hover:border-slate-400"
                                        }`}
                                      >
                                        {selected && <Check className="size-2.5 stroke-[3]" />}
                                      </button>

                                      <div className="size-7 rounded bg-slate-100 border border-slate-200 overflow-hidden shrink-0 flex items-center justify-center">
                                        <img
                                          src={datasetSourceImageUrl(lyr.id)}
                                          alt=""
                                          className="size-full object-cover"
                                          onError={(e) => {
                                            e.target.style.display = "none"
                                          }}
                                        />
                                      </div>

                                      <div className="flex-1 min-w-0">
                                        <div className="flex items-center gap-1.5">
                                          <span className="text-xs font-semibold text-slate-800 truncate">
                                            {lyr.name}
                                          </span>
                                          {lyr.isPrimary && (
                                            <span className="text-[10px] font-medium px-1.5 py-0.2 rounded bg-blue-100 text-blue-700">
                                              Primary
                                            </span>
                                          )}
                                        </div>
                                        <div className="text-[11px] text-slate-400 flex items-center gap-1.5 mt-0.5">
                                          <span>Opacity {Math.round(lyr.opacity * 100)}%</span>
                                          {lyr.bgRemoved && (
                                            <>
                                              <span>•</span>
                                              <span className="text-emerald-600 flex items-center gap-0.5">
                                                <Sparkles className="size-2.5" /> BG Removed
                                              </span>
                                            </>
                                          )}
                                          {lyr.rotated && (
                                            <>
                                              <span>•</span>
                                              <span className="text-indigo-600 flex items-center gap-0.5">
                                                <RotateCw className="size-2.5" /> Rotated {Math.round(lyr.rotationAngle)}°
                                              </span>
                                            </>
                                          )}
                                        </div>
                                      </div>
                                    </div>
                                  )
                                })}
                              </div>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )
              })}
            </div>


            {/* Bulk Selection Actions */}
            <div className="flex items-center gap-3 pt-2">
              <button
                type="button"
                onClick={selectAll}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-lg hover:bg-slate-50 transition-colors shadow-xs"
              >
                <Check className="size-3 text-blue-600" />
                Select All
              </button>
              <button
                type="button"
                onClick={deselectAll}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-slate-700 bg-white border border-slate-200 rounded-lg hover:bg-slate-50 transition-colors shadow-xs"
              >
                <X className="size-3 text-slate-400" />
                Deselect All
              </button>
            </div>
          </div>

          {/* Right Column: Preview & Export Options (5 cols) */}
          <div className="lg:col-span-5 p-6 flex flex-col gap-6 bg-white overflow-y-auto">
            {/* 1. Preview Pane */}
            <div className="flex flex-col gap-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                  Preview
                </span>
                {activePreviewLayer && (
                  <span className="text-xs font-medium text-blue-600 bg-blue-50 px-2 py-0.5 rounded-full border border-blue-100">
                    {activePreviewLayer.floor?.name || "Selected Layer"}
                  </span>
                )}
              </div>

              <div className="relative aspect-4/3 w-full rounded-xl bg-slate-900/5 border border-slate-200 overflow-hidden flex items-center justify-center p-2 shadow-inner">
                {activePreviewLayer ? (
                  <img
                    src={datasetSourceImageUrl(activePreviewLayer.id)}
                    alt={activePreviewLayer.name}
                    className="max-h-full max-w-full object-contain drop-shadow-md rounded"
                    style={{
                      opacity: activePreviewLayer.opacity,
                    }}
                  />
                ) : (
                  <div className="flex flex-col items-center justify-center text-slate-400 text-xs gap-1.5">
                    <Image className="size-8 stroke-[1.5]" />
                    <span>Select a layer to preview</span>
                  </div>
                )}
              </div>
            </div>

            {/* 2. Layer Details */}
            {activePreviewLayer && (
              <div className="flex flex-col gap-2">
                <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                  Layer Details
                </div>
                <div className="p-3.5 rounded-xl bg-slate-50 border border-slate-200/80 text-xs flex flex-col gap-2">
                  <div className="flex items-center justify-between">
                    <span className="text-slate-500">File Name</span>
                    <span className="font-semibold text-slate-800 truncate max-w-[180px]">
                      {activePreviewLayer.dataset?.filename || activePreviewLayer.name}
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-slate-500">Image Size</span>
                    <span className="font-mono text-slate-700">
                      {activePreviewLayer.dataset?.extracted_metadata?.width && activePreviewLayer.dataset?.extracted_metadata?.height
                        ? `${activePreviewLayer.dataset.extracted_metadata.width} × ${activePreviewLayer.dataset.extracted_metadata.height}`
                        : "Available"}
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-slate-500">Opacity</span>
                    <span className="font-medium text-slate-700">
                      {Math.round(activePreviewLayer.opacity * 100)}%
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-slate-500">Background Removal</span>
                    <span
                      className={`font-semibold ${
                        activePreviewLayer.bgRemoved ? "text-emerald-600" : "text-slate-500"
                      }`}
                    >
                      {activePreviewLayer.bgRemoved ? "Enabled" : "Original"}
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-slate-500">Transform</span>
                    <span className="font-medium text-slate-700">
                      {activePreviewLayer.rotated
                        ? `Rotated ${Math.round(activePreviewLayer.rotationAngle)}°`
                        : "Standard"}
                    </span>
                  </div>
                  {activePreviewLayer.building?.name ? (
                    <div className="flex items-center justify-between">
                      <span className="text-slate-500">Building / Floor</span>
                      <span className="font-medium text-slate-700">
                        {activePreviewLayer.building.name} {activePreviewLayer.floor?.name ? `• ${activePreviewLayer.floor.name}` : ""}
                      </span>
                    </div>
                  ) : (
                    <div className="flex items-center justify-between">
                      <span className="text-slate-500">Layer Type</span>
                      <span className="font-medium text-slate-700">Workspace Raster</span>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* 3. Export Options */}
            <div className="flex flex-col gap-2">
              <div className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                Export Options
              </div>
              <div className="flex flex-col gap-2.5">
                <label className="flex items-center gap-2.5 text-xs text-slate-700 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={options.include_building_configuration}
                    onChange={(e) =>
                      setOptions((prev) => ({
                        ...prev,
                        include_building_configuration: e.target.checked,
                      }))
                    }
                    className="size-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>Include building configuration (floors, order, settings)</span>
                </label>

                <label className="flex items-center gap-2.5 text-xs text-slate-700 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={options.include_control_points}
                    onChange={(e) =>
                      setOptions((prev) => ({
                        ...prev,
                        include_control_points: e.target.checked,
                      }))
                    }
                    className="size-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>Include shared control points & georeferencing</span>
                </label>

                <label className="flex items-center gap-2.5 text-xs text-slate-700 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={options.include_icons}
                    onChange={(e) =>
                      setOptions((prev) => ({
                        ...prev,
                        include_icons: e.target.checked,
                      }))
                    }
                    className="size-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>Include icons and map points</span>
                </label>

                <label className="flex items-center gap-2.5 text-xs text-slate-700 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={options.include_layer_styles}
                    onChange={(e) =>
                      setOptions((prev) => ({
                        ...prev,
                        include_layer_styles: e.target.checked,
                      }))
                    }
                    className="size-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>Include layer styles (opacity, background removal, etc.)</span>
                </label>

                <label className="flex items-center gap-2.5 text-xs text-slate-700 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={options.include_original_rasters && options.include_processed_rasters}
                    onChange={(e) => {
                      const val = e.target.checked
                      setOptions((prev) => ({
                        ...prev,
                        include_original_rasters: val,
                        include_processed_rasters: val,
                      }))
                    }}
                    className="size-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>Include original and processed raster images</span>
                </label>

                <label className="flex items-center gap-2.5 text-xs text-slate-700 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={options.include_base_map_settings}
                    onChange={(e) =>
                      setOptions((prev) => ({
                        ...prev,
                        include_base_map_settings: e.target.checked,
                      }))
                    }
                    className="size-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>Include base map settings</span>
                </label>

                <label className="flex items-center gap-2.5 text-xs text-slate-700 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={options.include_project_metadata}
                    onChange={(e) =>
                      setOptions((prev) => ({
                        ...prev,
                        include_project_metadata: e.target.checked,
                      }))
                    }
                    className="size-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
                  />
                  <span>Include project metadata (name, description, date)</span>
                </label>
              </div>
            </div>

            {/* Error Message if any */}
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
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-slate-100 bg-slate-50/80">
          <div className="flex items-center gap-3 text-xs text-slate-500">
            <span>
              <strong>{totalSelectedCount}</strong> layer{totalSelectedCount !== 1 ? "s" : ""} selected
            </span>
            <span>•</span>
            <span>Est. Size: ~{estimatedSizeMb} MB</span>
          </div>

          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={exporting}
              className="px-4 py-2 text-sm font-medium text-slate-700 hover:text-slate-900 bg-white border border-slate-200 rounded-xl hover:bg-slate-100 transition-colors shadow-xs"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleExportClick}
              disabled={totalSelectedCount === 0 || exporting}
              className="inline-flex items-center gap-2 px-5 py-2 text-sm font-semibold text-white bg-blue-600 hover:bg-blue-700 rounded-xl transition-all shadow-sm hover:shadow disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {exporting ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  <span>Packaging ZIP…</span>
                </>
              ) : (
                <>
                  <Download className="size-4" />
                  <span>Export ZIP</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
