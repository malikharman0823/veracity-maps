import React, { useEffect, useState } from "react"
import {
  Upload,
  Layers,
  Image as ImageIcon,
  Globe,
  Check,
  AlertTriangle,
  CheckCircle2,
  ArrowRight,
  ArrowLeft,
  Trash2,
  Info,
  Loader2,
  FolderOpen,
} from "lucide-react"
import {
  uploadDataset,
  inspectUploadedDataset,
  addWorkspaceDatasetLayers,
  listBaseLayers,
  listProfileFiles,
  getWorkspaceBaseLayer,
  selectWorkspaceBaseLayer,
  registerBaseLayer,
  getDataset,
  datasetSourceImageUrl,
  readableApiError,
} from "../../api"
import BaseLayerCard from "../baselayers/BaseLayerCard"
import BaseLayerPreview from "../baselayers/BaseLayerPreview"

const VECTOR_ACCEPT = ".gpkg,.shp,.geojson,.json,.kml,.osm,.dxf,.zip"
const RASTER_ACCEPT = ".tif,.tiff,.png,.jpg,.jpeg,.pdf,.mbtiles"

function errorText(err, fallback) {
  const res = readableApiError(err, fallback)
  if (typeof res === "string") return res
  if (res && typeof res === "object") {
    return res.details ? `${res.message} ${res.details}` : res.message || fallback
  }
  return fallback
}

/**
 * Reusable Unified Dataset Upload Panel for all three optional map-layer groups:
 *   1. Vector Layers (Top) — GeoPackage multi-layer discovery, Shapefile, GeoJSON, KML, OSM, CAD
 *   2. Raster Layers (Middle) — GeoTIFF, TIFF, PNG/JPG, PDF (unreferenced rasters retained safely)
 *   3. Base Layer (Bottom) — Default World Map preselected, or Choose / Upload Custom Base Layer
 *
 * Used by BOTH the standalone Unified Upload Page (/maps/:mapId/setup) AND the Viewscape Add Dataset modal.
 */
export default function UnifiedDatasetUploadPanel({
  workspaceId,
  workspaceName = "Workspace",
  initialFocusSection = null, // "vector" | "raster" | "base" | null
  primaryActionLabel = "Open Workspace",
  showBackButton = true,
  onBack,
  onComplete,
}) {
  // Existing uploaded datasets for quick selection
  const [existingFiles, setExistingFiles] = useState([])
  const [defaultWorldMap, setDefaultWorldMap] = useState(null)
  const [customBaseLayers, setCustomBaseLayers] = useState([])

  // Card 1 — Vector Layers state: [{ datasetId, filename, driver, layers: [...], selectedNames: Set, error }]
  const [vectorItems, setVectorItems] = useState([])
  const [uploadingVector, setUploadingVector] = useState(false)
  const [vectorError, setVectorError] = useState("")

  // Card 2 — Raster Layers state: [{ datasetId, filename, width, height, crs, isGeoreferenced, bounds, status }]
  const [rasterItems, setRasterItems] = useState([])
  const [uploadingRaster, setUploadingRaster] = useState(false)
  const [rasterError, setRasterError] = useState("")

  // Card 3 — Base Layer state (Default World Map preselected)
  const [baseLayerChoice, setBaseLayerChoice] = useState(null) // BaseLayer object
  const [baseTab, setBaseTab] = useState("world") // "world" | "existing" | "upload"
  const [uploadingBase, setUploadingBase] = useState(false)
  const [baseUploadError, setBaseUploadError] = useState("")
  const [baseUploadName, setBaseUploadName] = useState("")

  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState("")

  useEffect(() => {
    let active = true
    ;(async () => {
      try {
        const [libRes, filesRes, wsBlRes] = await Promise.all([
          listBaseLayers().catch(() => null),
          listProfileFiles().catch(() => null),
          workspaceId ? getWorkspaceBaseLayer(workspaceId).catch(() => null) : Promise.resolve(null),
        ])
        if (!active) return
        const worldMap = libRes?.default_base_layer || {
          id: "world_map",
          name: "World Map",
          description: "Use the default global map.",
          source_type: "world_map",
          source_crs: "EPSG:3857",
          extent: [-180, -85.051129, 180, 85.051129],
          status: "ready",
          is_default: true,
        }
        setDefaultWorldMap(worldMap)
        setCustomBaseLayers(libRes?.base_layers || [])
        setExistingFiles(filesRes?.files || [])

        const saved = wsBlRes?.workspace_base_layer
        if (saved?.has_saved_selection && saved?.base_layer_type === "custom" && saved?.base_layer && !saved.previous_unavailable) {
          setBaseLayerChoice(saved.base_layer)
          setBaseTab("existing")
        } else {
          setBaseLayerChoice(worldMap)
          setBaseTab("world")
        }
      } catch {}
    })()
    return () => {
      active = false
    }
  }, [workspaceId])

  // Inspect and register a vector dataset entry dynamically via GDAL/OGR
  const attachVectorDataset = async (dsId, fallbackName) => {
    try {
      const inspection = await inspectUploadedDataset({
        file_id: dsId,
        workspace_id: workspaceId || "",
      })
      const vectorLayers = (inspection?.layers || []).filter(
        (l) => !String(l.geometry_type || "").toLowerCase().includes("raster")
      )
      const rasterLayers = (inspection?.layers || []).filter((l) =>
        String(l.geometry_type || "").toLowerCase().includes("raster")
      )

      if (vectorLayers.length === 0 && rasterLayers.length > 0) {
        setVectorError(
          `"${inspection.filename || fallbackName}" contains raster content rather than vector layers. Added it to the Raster Layers section instead.`
        )
        await attachRasterDataset(dsId, fallbackName)
        return
      }
      if (vectorLayers.length === 0) {
        setVectorError(`No importable vector layers were detected in "${fallbackName}".`)
        return
      }
      setVectorItems((prev) => {
        if (prev.some((item) => item.datasetId === dsId)) return prev
        return [
          ...prev,
          {
            datasetId: dsId,
            filename: inspection.filename || fallbackName,
            driver: inspection.driver || inspection.format || "VECTOR",
            layers: vectorLayers,
            selectedNames: new Set(vectorLayers.map((l) => l.layer_name)),
          },
        ]
      })
    } catch (err) {
      setVectorError(errorText(err, `Unable to inspect "${fallbackName}" as a vector dataset.`))
    }
  }

  const handleVectorFilesSelected = async (fileList) => {
    const files = Array.from(fileList || [])
    if (files.length === 0) return
    setUploadingVector(true)
    setVectorError("")
    for (const file of files) {
      try {
        const uploaded = await uploadDataset(file)
        const dsId = uploaded?.id || uploaded?.dataset_id
        if (dsId) {
          await attachVectorDataset(dsId, file.name)
        }
      } catch (err) {
        setVectorError(errorText(err, `Failed uploading "${file.name}".`))
      }
    }
    setUploadingVector(false)
  }

  // Inspect and stage a raster dataset (non-georeferenced rasters safely retained awaiting georeferencing)
  const attachRasterDataset = async (dsId, fallbackName) => {
    try {
      const ds = await getDataset(dsId)
      const meta = ds?.extracted_metadata || {}
      const isGeoref = Boolean(
        ds?.georeference_status === "completed" ||
          meta.is_georeferenced ||
          (meta.crs && (ds?.map_layer?.bounds || meta.bounds))
      )
      setRasterItems((prev) => {
        if (prev.some((item) => item.datasetId === dsId)) return prev
        return [
          ...prev,
          {
            datasetId: dsId,
            filename: ds?.filename || fallbackName,
            width: Number(meta.width) || 0,
            height: Number(meta.height) || 0,
            crs: meta.crs || ds?.map_layer?.bounds_crs || "",
            isGeoreferenced: isGeoref,
            status: isGeoref ? "georeferenced" : "awaiting_georeferencing",
          },
        ]
      })
    } catch (err) {
      setRasterError(errorText(err, `Failed inspecting raster "${fallbackName}".`))
    }
  }

  const handleRasterFilesSelected = async (fileList) => {
    const files = Array.from(fileList || [])
    if (files.length === 0) return
    setUploadingRaster(true)
    setRasterError("")
    for (const file of files) {
      try {
        const uploaded = await uploadDataset(file)
        const dsId = uploaded?.id || uploaded?.dataset_id
        if (dsId) {
          await attachRasterDataset(dsId, file.name)
        }
      } catch (err) {
        setRasterError(errorText(err, `Failed uploading "${file.name}".`))
      }
    }
    setUploadingRaster(false)
  }

  // Upload & register a custom base layer (requires valid CRS / georeferencing)
  const handleBaseLayerFileUpload = async (file) => {
    if (!file) return
    setUploadingBase(true)
    setBaseUploadError("")
    try {
      const uploaded = await uploadDataset(file)
      const dsId = uploaded?.id || uploaded?.dataset_id
      if (!dsId) throw new Error("Upload failed to return dataset ID")
      const cleanName = baseUploadName.trim() || file.name.replace(/\.[^/.]+$/, "")
      const res = await registerBaseLayer({
        dataset_id: dsId,
        name: cleanName,
        description: "Custom geographic base layer",
      })
      const created = res?.base_layer
      if (created) {
        setCustomBaseLayers((prev) => [created, ...prev.filter((b) => b.id !== created.id)])
        setBaseLayerChoice(created)
        setBaseTab("existing")
        setBaseUploadName("")
      }
    } catch (err) {
      setBaseUploadError(
        errorText(
          err,
          "Custom base layer must be georeferenced with a valid CRS before it can be used as a geographic reference."
        )
      )
    } finally {
      setUploadingBase(false)
    }
  }

  const toggleVectorLayerName = (datasetId, layerName) => {
    setVectorItems((prev) =>
      prev.map((item) => {
        if (item.datasetId !== datasetId) return item
        const nextSet = new Set(item.selectedNames)
        if (nextSet.has(layerName)) nextSet.delete(layerName)
        else nextSet.add(layerName)
        return { ...item, selectedNames: nextSet }
      })
    )
  }

  const selectAllVectorLayers = (datasetId, selectAll) => {
    setVectorItems((prev) =>
      prev.map((item) => {
        if (item.datasetId !== datasetId) return item
        return {
          ...item,
          selectedNames: selectAll ? new Set(item.layers.map((l) => l.layer_name)) : new Set(),
        }
      })
    )
  }

  const totalSelectedVectorLayers = vectorItems.reduce(
    (sum, item) => sum + item.selectedNames.size,
    0
  )

  const handleOpenOrApply = async () => {
    setSubmitting(true)
    setSubmitError("")
    const errors = []
    const addedVectorLayers = []
    const addedRasterLayers = []

    try {
      // 1. Save Base Layer Selection (defaults cleanly to World Map even when 0 files uploaded)
      const chosenBase = baseLayerChoice || defaultWorldMap
      const isDefault = !chosenBase || Boolean(chosenBase.is_default || chosenBase.id === "world_map")
      if (workspaceId) {
        try {
          await selectWorkspaceBaseLayer(workspaceId, {
            base_layer_type: isDefault ? "world_map" : "custom",
            base_layer_id: isDefault ? "world_map" : chosenBase.id,
            selected_view: isDefault ? "world" : "custom",
          })
        } catch (err) {
          errors.push(errorText(err, "Failed saving base layer preference."))
        }
      }

      // 2. Import selected Vector Layers (if any — completely optional!)
      for (const vItem of vectorItems) {
        const names = Array.from(vItem.selectedNames)
        if (names.length === 0 || !workspaceId) continue
        try {
          const res = await addWorkspaceDatasetLayers(workspaceId, {
            dataset_id: vItem.datasetId,
            workspace_id: workspaceId,
            selected_layer_names: names,
          })
          if (res?.layers) addedVectorLayers.push(...res.layers)
        } catch (err) {
          errors.push(errorText(err, `Failed importing layers from "${vItem.filename}".`))
        }
      }

      // 3. Register Raster Layers (if any — completely optional!)
      for (const rItem of rasterItems) {
        if (!workspaceId) continue
        try {
          const res = await addWorkspaceDatasetLayers(workspaceId, {
            dataset_id: rItem.datasetId,
            workspace_id: workspaceId,
          })
          if (res?.layers) addedRasterLayers.push(...res.layers)
        } catch (err) {
          errors.push(errorText(err, `Failed adding raster "${rItem.filename}".`))
        }
      }

      if (errors.length > 0) {
        setSubmitError(errors.join(" "))
      }

      onComplete?.({
        baseLayer: chosenBase,
        baseLayerType: isDefault ? "world_map" : "custom",
        addedVectorLayers,
        addedRasterLayers,
        rasterItems,
      })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="space-y-5">
      {/* Dynamic 3-Card Layout: Vector (Top) → Raster (Middle) → Base Layer (Bottom) */}
      <div className="space-y-4">
        {/* CARD 1 — VECTOR LAYERS (TOP) */}
        <section
          className={`rounded-[12px] border bg-white dark:bg-[#1c1f24] p-5 transition-all ${
            initialFocusSection === "vector"
              ? "border-[#3ecf8e] ring-1 ring-[#3ecf8e]/40"
              : "border-neutral-200 dark:border-[#2e323a]"
          }`}
        >
          <div className="flex flex-wrap items-start justify-between gap-3 mb-3">
            <div className="flex items-start gap-3">
              <div className="w-9 h-9 rounded-[8px] bg-emerald-500/15 text-emerald-500 flex items-center justify-center shrink-0">
                <Layers className="w-5 h-5" />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
                    1. Vector Layers
                  </h3>
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-neutral-100 dark:bg-[#2a2e37] text-neutral-500 dark:text-neutral-300">
                    Optional • Top Layer Group
                  </span>
                </div>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                  Points, lines, polygons and geographic features (GeoPackage `.gpkg`, Shapefile `.zip`, GeoJSON, KML, OSM, DXF).
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <label className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-bold cursor-pointer shadow-xs">
                {uploadingVector ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Upload className="w-3.5 h-3.5" />}
                <span>{uploadingVector ? "Inspecting…" : "Upload Vector Files"}</span>
                <input
                  type="file"
                  multiple
                  accept={VECTOR_ACCEPT}
                  className="hidden"
                  disabled={uploadingVector}
                  onChange={(e) => {
                    handleVectorFilesSelected(e.target.files)
                    e.target.value = ""
                  }}
                />
              </label>
              {existingFiles.length > 0 && (
                <select
                  value=""
                  onChange={(e) => {
                    const dsId = e.target.value
                    if (!dsId) return
                    const found = existingFiles.find((f) => f.id === dsId)
                    attachVectorDataset(dsId, found?.original_filename || dsId)
                  }}
                  className="px-2.5 py-1.5 rounded-[6px] text-xs bg-neutral-100 dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-neutral-700 dark:text-neutral-200 max-w-[200px]"
                >
                  <option value="">Select Existing Dataset…</option>
                  {existingFiles.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.original_filename}
                    </option>
                  ))}
                </select>
              )}
            </div>
          </div>

          {vectorError && (
            <div className="mb-3 p-2.5 rounded-[7px] bg-amber-500/10 border border-amber-500/30 text-xs text-amber-700 dark:text-amber-300 flex items-center gap-2">
              <AlertTriangle className="w-4 h-4 shrink-0" />
              <span>{vectorError}</span>
            </div>
          )}

          {vectorItems.length === 0 ? (
            <div
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault()
                handleVectorFilesSelected(e.dataTransfer?.files)
              }}
              className="rounded-[9px] border border-dashed border-neutral-300 dark:border-[#343842] bg-neutral-50/70 dark:bg-[#14161a] px-4 py-5 text-center text-xs text-neutral-500 dark:text-neutral-400"
            >
              Drag &amp; drop GeoPackage (`.gpkg`), Shapefile (`.zip`), GeoJSON, KML, OSM, or DXF files here — or skip to start with an empty Vector group.
            </div>
          ) : (
            <div className="space-y-3">
              {vectorItems.map((item) => (
                <div
                  key={item.datasetId}
                  className="rounded-[9px] border border-neutral-200 dark:border-[#2e323a] bg-neutral-50 dark:bg-[#14161a] p-3.5 space-y-2.5"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-bold text-neutral-900 dark:text-white">
                        {item.filename}
                      </span>
                      <span className="px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 text-[10px] font-semibold">
                        {item.driver} • {item.layers.length} {item.layers.length === 1 ? "layer" : "layers"} detected
                      </span>
                    </div>
                    <div className="flex items-center gap-2 text-[11px]">
                      <button
                        type="button"
                        onClick={() => selectAllVectorLayers(item.datasetId, true)}
                        className="text-[#3ecf8e] hover:underline font-semibold cursor-pointer"
                      >
                        Select All
                      </button>
                      <button
                        type="button"
                        onClick={() => selectAllVectorLayers(item.datasetId, false)}
                        className="text-neutral-500 hover:text-neutral-800 dark:hover:text-white cursor-pointer"
                      >
                        Deselect All
                      </button>
                      <button
                        type="button"
                        onClick={() => setVectorItems((prev) => prev.filter((x) => x.datasetId !== item.datasetId))}
                        className="p-1 text-red-500 hover:bg-red-500/10 rounded cursor-pointer"
                        title="Remove file from import list"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  {/* Dynamically discovered layers checklist */}
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
                    {item.layers.map((lyr) => {
                      const checked = item.selectedNames.has(lyr.layer_name)
                      return (
                        <label
                          key={lyr.layer_name}
                          className={`flex items-start gap-2.5 p-2.5 rounded-[7px] border cursor-pointer transition-colors ${
                            checked
                              ? "border-[#3ecf8e]/60 bg-[#3ecf8e]/10"
                              : "border-neutral-200 dark:border-[#2a2d34] bg-white dark:bg-[#1c1f24] opacity-75"
                          }`}
                        >
                          <input
                            type="checkbox"
                            checked={checked}
                            onChange={() => toggleVectorLayerName(item.datasetId, lyr.layer_name)}
                            className="mt-0.5 accent-[#3ecf8e] cursor-pointer"
                          />
                          <div className="min-w-0 flex-1">
                            <div className="text-xs font-semibold text-neutral-900 dark:text-white truncate" title={lyr.layer_name}>
                              {lyr.layer_name}
                            </div>
                            <div className="flex flex-wrap items-center gap-1.5 mt-1 text-[10px] text-neutral-500 dark:text-neutral-400">
                              <span className="px-1.5 py-0.5 rounded bg-neutral-200/70 dark:bg-[#2a2e37] font-medium text-neutral-700 dark:text-neutral-200">
                                {lyr.geometry_type || "Geometry"}
                              </span>
                              <span title="Number of points, lines or polygons in this layer.">
                                {lyr.feature_count ?? 0} features
                              </span>
                              {lyr.crs && (
                                <span
                                  className="font-mono"
                                  title="Coordinate Reference System — defines how locations are positioned on the map."
                                >
                                  {lyr.crs}
                                </span>
                              )}
                            </div>
                          </div>
                        </label>
                      )
                    })}
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        {/* CARD 2 — RASTER LAYERS (MIDDLE) */}
        <section
          className={`rounded-[12px] border bg-white dark:bg-[#1c1f24] p-5 transition-all ${
            initialFocusSection === "raster"
              ? "border-[#3ecf8e] ring-1 ring-[#3ecf8e]/40"
              : "border-neutral-200 dark:border-[#2e323a]"
          }`}
        >
          <div className="flex flex-wrap items-start justify-between gap-3 mb-3">
            <div className="flex items-start gap-3">
              <div className="w-9 h-9 rounded-[8px] bg-sky-500/15 text-sky-400 flex items-center justify-center shrink-0">
                <ImageIcon className="w-5 h-5" />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
                    2. Raster Layers
                  </h3>
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-neutral-100 dark:bg-[#2a2e37] text-neutral-500 dark:text-neutral-300">
                    Optional • Middle Layer Group
                  </span>
                </div>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                  Images, scanned maps and raster datasets (GeoTIFF, TIFF, PNG, JPG, PDF, MBTiles). Unreferenced images remain safely staged for georeferencing.
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <label className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-sky-500 hover:bg-sky-400 text-white text-xs font-bold cursor-pointer shadow-xs">
                {uploadingRaster ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Upload className="w-3.5 h-3.5" />}
                <span>{uploadingRaster ? "Uploading…" : "Upload Raster Files"}</span>
                <input
                  type="file"
                  multiple
                  accept={RASTER_ACCEPT}
                  className="hidden"
                  disabled={uploadingRaster}
                  onChange={(e) => {
                    handleRasterFilesSelected(e.target.files)
                    e.target.value = ""
                  }}
                />
              </label>
              {existingFiles.length > 0 && (
                <select
                  value=""
                  onChange={(e) => {
                    const dsId = e.target.value
                    if (!dsId) return
                    const found = existingFiles.find((f) => f.id === dsId)
                    attachRasterDataset(dsId, found?.original_filename || dsId)
                  }}
                  className="px-2.5 py-1.5 rounded-[6px] text-xs bg-neutral-100 dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-neutral-700 dark:text-neutral-200 max-w-[200px]"
                >
                  <option value="">Select Existing Raster…</option>
                  {existingFiles.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.original_filename}
                    </option>
                  ))}
                </select>
              )}
            </div>
          </div>

          {rasterError && (
            <div className="mb-3 p-2.5 rounded-[7px] bg-red-500/10 border border-red-500/30 text-xs text-red-500">
              {rasterError}
            </div>
          )}

          {rasterItems.length === 0 ? (
            <div
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault()
                handleRasterFilesSelected(e.dataTransfer?.files)
              }}
              className="rounded-[9px] border border-dashed border-neutral-300 dark:border-[#343842] bg-neutral-50/70 dark:bg-[#14161a] px-4 py-5 text-center text-xs text-neutral-500 dark:text-neutral-400"
            >
              Drag &amp; drop GeoTIFF, TIFF, PNG, JPG, or PDF files here — or skip to keep Raster Layers empty.
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2.5">
              {rasterItems.map((r) => (
                <div
                  key={r.datasetId}
                  className="flex items-center gap-3 p-2.5 rounded-[8px] border border-neutral-200 dark:border-[#2e323a] bg-neutral-50 dark:bg-[#14161a]"
                >
                  <div className="w-12 h-12 rounded bg-[#101216] overflow-hidden shrink-0 flex items-center justify-center">
                    <img
                      src={datasetSourceImageUrl(r.datasetId)}
                      alt={r.filename}
                      className="w-full h-full object-cover"
                      onError={(e) => {
                        e.currentTarget.style.display = "none"
                      }}
                    />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-xs font-semibold text-neutral-900 dark:text-white truncate">
                      {r.filename}
                    </div>
                    <div className="text-[10px] text-neutral-500 dark:text-neutral-400">
                      {r.width > 0 && r.height > 0 ? `${r.width}×${r.height}px` : "Raster"} {r.crs ? `• ${r.crs}` : ""}
                    </div>
                    <div className="mt-0.5">
                      {r.isGeoreferenced ? (
                        <span className="inline-flex items-center gap-1 text-[10px] font-semibold text-emerald-500">
                          <CheckCircle2 className="w-3 h-3" /> Georeferenced
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-[10px] font-semibold text-amber-500">
                          <Info className="w-3 h-3" /> Awaiting georeferencing
                        </span>
                      )}
                    </div>
                  </div>
                  <button
                    type="button"
                    onClick={() => setRasterItems((prev) => prev.filter((x) => x.datasetId !== r.datasetId))}
                    className="p-1 text-red-500 hover:bg-red-500/10 rounded cursor-pointer"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>

        {/* CARD 3 — BASE LAYER (BOTTOM) */}
        <section
          className={`rounded-[12px] border bg-white dark:bg-[#1c1f24] p-5 transition-all ${
            initialFocusSection === "base"
              ? "border-[#3ecf8e] ring-1 ring-[#3ecf8e]/40"
              : "border-neutral-200 dark:border-[#2e323a]"
          }`}
        >
          <div className="flex flex-wrap items-start justify-between gap-3 mb-3">
            <div className="flex items-start gap-3">
              <div className="w-9 h-9 rounded-[8px] bg-indigo-500/15 text-indigo-400 flex items-center justify-center shrink-0">
                <Globe className="w-5 h-5" />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
                    3. Base Layer
                  </h3>
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">
                    World Map Preselected • Bottom Layer
                  </span>
                </div>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                  The background map for your workspace. Start immediately with World Map or choose/upload a georeferenced custom base map.
                </p>
              </div>
            </div>

            <div className="inline-flex rounded-[7px] bg-neutral-100 dark:bg-[#121418] p-0.5 border border-neutral-200 dark:border-[#2a2d34]">
              <button
                type="button"
                onClick={() => {
                  setBaseTab("world")
                  if (defaultWorldMap) setBaseLayerChoice(defaultWorldMap)
                }}
                className={`px-2.5 py-1 rounded-[5px] text-xs font-semibold cursor-pointer ${
                  baseTab === "world" ? "bg-[#3ecf8e] text-[#171717]" : "text-neutral-500 hover:text-neutral-900 dark:hover:text-white"
                }`}
              >
                Default World Map
              </button>
              <button
                type="button"
                onClick={() => setBaseTab("existing")}
                className={`px-2.5 py-1 rounded-[5px] text-xs font-semibold cursor-pointer ${
                  baseTab === "existing" ? "bg-[#3ecf8e] text-[#171717]" : "text-neutral-500 hover:text-neutral-900 dark:hover:text-white"
                }`}
              >
                My Base Layers ({customBaseLayers.length})
              </button>
              <button
                type="button"
                onClick={() => setBaseTab("upload")}
                className={`px-2.5 py-1 rounded-[5px] text-xs font-semibold cursor-pointer ${
                  baseTab === "upload" ? "bg-[#3ecf8e] text-[#171717]" : "text-neutral-500 hover:text-neutral-900 dark:hover:text-white"
                }`}
              >
                Upload Custom Base
              </button>
            </div>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            <div className="lg:col-span-2">
              {baseTab === "world" && defaultWorldMap && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <BaseLayerCard
                    layer={defaultWorldMap}
                    selected={!baseLayerChoice || baseLayerChoice.id === "world_map" || baseLayerChoice.is_default}
                    onSelect={(l) => setBaseLayerChoice(l)}
                  />
                </div>
              )}

              {baseTab === "existing" && (
                customBaseLayers.length === 0 ? (
                  <div className="rounded-[9px] border border-dashed border-neutral-300 dark:border-[#343842] p-6 text-center text-xs text-neutral-500 dark:text-neutral-400">
                    No custom base layers registered yet. Switch to <strong>Upload Custom Base</strong> or keep <strong>Default World Map</strong>.
                  </div>
                ) : (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    {customBaseLayers.map((bl) => (
                      <BaseLayerCard
                        key={bl.id}
                        layer={bl}
                        selected={baseLayerChoice?.id === bl.id}
                        onSelect={(l) => setBaseLayerChoice(l)}
                      />
                    ))}
                  </div>
                )
              )}

              {baseTab === "upload" && (
                <div className="rounded-[9px] border border-dashed border-neutral-300 dark:border-[#343842] bg-neutral-50 dark:bg-[#14161a] p-4 space-y-3">
                  <div>
                    <label className="block text-[11px] font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                      Base Layer Name (optional — defaults to file name)
                    </label>
                    <input
                      type="text"
                      value={baseUploadName}
                      onChange={(e) => setBaseUploadName(e.target.value)}
                      placeholder="e.g., City Reference Map"
                      className="w-full px-3 py-1.5 rounded-[6px] text-xs bg-white dark:bg-[#1c1f24] border border-neutral-200 dark:border-[#2e323a] text-neutral-900 dark:text-white"
                    />
                  </div>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="text-[11px] text-neutral-500 dark:text-neutral-400">
                      Requires a georeferenced file (GeoTIFF, MBTiles, GeoPackage, GeoJSON).
                    </span>
                    <label className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-indigo-500 hover:bg-indigo-400 text-white text-xs font-bold cursor-pointer">
                      {uploadingBase ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Upload className="w-3.5 h-3.5" />}
                      <span>{uploadingBase ? "Registering…" : "Choose Georeferenced Base Map"}</span>
                      <input
                        type="file"
                        accept=".tif,.tiff,.mbtiles,.gpkg,.geojson,.json,.zip"
                        className="hidden"
                        disabled={uploadingBase}
                        onChange={(e) => {
                          const f = e.target.files?.[0]
                          if (f) handleBaseLayerFileUpload(f)
                          e.target.value = ""
                        }}
                      />
                    </label>
                  </div>
                  {baseUploadError && (
                    <div className="p-2.5 rounded-[6px] bg-amber-500/10 border border-amber-500/30 text-[11px] text-amber-600 dark:text-amber-300">
                      {baseUploadError}
                    </div>
                  )}
                </div>
              )}
            </div>

            <div>
              <BaseLayerPreview
                layer={baseLayerChoice || defaultWorldMap}
                title="Active Base Layer Preview"
              />
            </div>
          </div>
        </section>
      </div>

      {submitError && (
        <div className="p-3 rounded-[8px] bg-amber-500/10 border border-amber-500/30 text-xs text-amber-700 dark:text-amber-300">
          {submitError}
        </div>
      )}

      {/* Dynamic Summary & Action Footer — Open Workspace is NEVER disabled for 0 uploads! */}
      <div className="rounded-[12px] border border-neutral-200 dark:border-[#2e323a] bg-neutral-50 dark:bg-[#15171b] px-5 py-3.5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
          <span className="text-neutral-600 dark:text-neutral-300">
            Vector Layers: <strong className="text-neutral-900 dark:text-white">{totalSelectedVectorLayers} selected</strong>
          </span>
          <span className="text-neutral-400">•</span>
          <span className="text-neutral-600 dark:text-neutral-300">
            Raster Layers: <strong className="text-neutral-900 dark:text-white">{rasterItems.length} selected</strong>
          </span>
          <span className="text-neutral-400">•</span>
          <span className="text-neutral-600 dark:text-neutral-300">
            Base Layer: <strong className="text-[#24b47e] dark:text-[#3ecf8e]">{(baseLayerChoice || defaultWorldMap)?.name || "World Map"}</strong>
          </span>
        </div>

        <div className="flex items-center gap-2.5 shrink-0">
          {showBackButton && onBack && (
            <button
              type="button"
              onClick={onBack}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-[7px] border border-neutral-300 dark:border-[#343842] text-xs font-semibold text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#24272f] cursor-pointer"
            >
              <ArrowLeft className="w-3.5 h-3.5" />
              Back
            </button>
          )}
          <button
            type="button"
            disabled={submitting || uploadingVector || uploadingRaster || uploadingBase}
            onClick={handleOpenOrApply}
            className="inline-flex items-center gap-2 px-5 py-2 rounded-[7px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-bold shadow-xs disabled:opacity-60 cursor-pointer"
          >
            {submitting ? (
              <>
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
                <span>Preparing Workspace…</span>
              </>
            ) : (
              <>
                <span>{primaryActionLabel}</span>
                <ArrowRight className="w-3.5 h-3.5" />
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  )
}
