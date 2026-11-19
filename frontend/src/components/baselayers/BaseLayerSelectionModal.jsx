import React, { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import {
  X,
  Search,
  Plus,
  Upload,
  AlertTriangle,
  CheckCircle2,
  ArrowRight,
  Globe,
  ExternalLink,
  FolderOpen,
} from "lucide-react"
import {
  listBaseLayers,
  listProfileFiles,
  uploadDataset,
  getDataset,
  registerBaseLayer,
  getWorkspaceBaseLayer,
  selectWorkspaceBaseLayer,
  datasetSourceImageUrl,
  readableApiError,
} from "../../api"
import BaseLayerCard from "./BaseLayerCard"
import BaseLayerPreview from "./BaseLayerPreview"

function errorText(err, fallback) {
  const res = readableApiError(err, fallback)
  if (typeof res === "string") return res
  if (res && typeof res === "object") {
    return res.details ? `${res.message} ${res.details}` : res.message || fallback
  }
  return fallback
}

export default function BaseLayerSelectionModal({
  open,
  initialMode = "select", // "select" | "upload"
  workspaceId,
  workspaceName = "Workspace",
  onCancel,
  onConfirmed,
}) {
  const navigate = useNavigate()
  const [mode, setMode] = useState(initialMode)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState("")
  const [defaultWorldMap, setDefaultWorldMap] = useState(null)
  const [customLayers, setCustomLayers] = useState([])
  const [selectedLayer, setSelectedLayer] = useState(null)
  const [unavailableWarning, setUnavailableWarning] = useState("")
  const [search, setSearch] = useState("")

  // Upload & Register Custom Base Layer State
  const [uploadSourceTab, setUploadSourceTab] = useState("upload") // "upload" | "existing"
  const [draggingOver, setDraggingOver] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [userFiles, setUserFiles] = useState([])
  const [loadingFiles, setLoadingFiles] = useState(false)
  const [selectedDatasetId, setSelectedDatasetId] = useState("")
  const [inspectedDataset, setInspectedDataset] = useState(null)
  const [registerName, setRegisterName] = useState("")
  const [registerDesc, setRegisterDesc] = useState("")
  const [registerError, setRegisterError] = useState("")
  const [registering, setRegistering] = useState(false)

  const loadSelectionData = async () => {
    setLoading(true)
    setError("")
    setUnavailableWarning("")
    try {
      const [libData, wsData] = await Promise.all([
        listBaseLayers(),
        workspaceId ? getWorkspaceBaseLayer(workspaceId).catch(() => null) : Promise.resolve(null),
      ])
      const worldMap = libData?.default_base_layer || {
        id: "world_map",
        name: "World Map",
        description: "Use the default global map.",
        source_type: "world_map",
        source_crs: "EPSG:3857",
        extent: [-180, -85.051129, 180, 85.051129],
        status: "ready",
        is_default: true,
      }
      const list = libData?.base_layers || []
      setDefaultWorldMap(worldMap)
      setCustomLayers(list)

      const saved = wsData?.workspace_base_layer
      if (saved?.previous_unavailable) {
        setUnavailableWarning(
          saved.unavailable_warning ||
            "Your previously saved custom base layer is unavailable. Select another base layer or switch to the Default World Map."
        )
        setSelectedLayer(worldMap)
      } else if (saved?.has_saved_selection && saved?.base_layer_type === "custom" && saved?.base_layer) {
        const match = list.find((item) => item.id === saved.base_layer.id) || saved.base_layer
        setSelectedLayer(match)
      } else {
        setSelectedLayer(worldMap)
      }
    } catch (err) {
      setError(errorText(err, "Unable to load base layers."))
    } finally {
      setLoading(false)
    }
  }

  const loadExistingFiles = async () => {
    setLoadingFiles(true)
    try {
      const res = await listProfileFiles()
      setUserFiles(res?.files || [])
    } catch {
      setUserFiles([])
    } finally {
      setLoadingFiles(false)
    }
  }

  useEffect(() => {
    if (open) {
      setMode(initialMode === "upload" ? "upload" : "select")
      setRegisterError("")
      setSelectedDatasetId("")
      setInspectedDataset(null)
      setRegisterName("")
      setRegisterDesc("")
      setUploadSourceTab("upload")
      loadSelectionData()
    }
  }, [open, workspaceId, initialMode])

  useEffect(() => {
    if (open && mode === "upload") {
      loadExistingFiles()
    }
  }, [open, mode])

  if (!open) return null

  const inspectDatasetById = async (dsId, fallbackFilename = "") => {
    setSelectedDatasetId(dsId)
    setRegisterError("")
    if (!dsId) {
      setInspectedDataset(null)
      return
    }
    try {
      const ds = await getDataset(dsId)
      setInspectedDataset(ds)
      const cleanName = (ds?.filename || fallbackFilename || "").replace(/\.[^/.]+$/, "")
      if (cleanName) {
        setRegisterName((prev) => prev.trim() || cleanName)
      }
    } catch (err) {
      setInspectedDataset(null)
      setRegisterError(errorText(err, "Failed to inspect uploaded map file."))
    }
  }

  const handleUploadFile = async (file) => {
    if (!file) return
    setUploading(true)
    setRegisterError("")
    try {
      const uploaded = await uploadDataset(file)
      const dsId = uploaded?.id || uploaded?.dataset_id
      if (!dsId) {
        setRegisterError("Upload succeeded, but no dataset ID was returned.")
        return
      }
      setRegisterName(file.name.replace(/\.[^/.]+$/, ""))
      await inspectDatasetById(dsId, file.name)
      await loadExistingFiles()
    } catch (err) {
      setRegisterError(errorText(err, "Failed to upload map file."))
    } finally {
      setUploading(false)
    }
  }

  const handleDrop = (e) => {
    e.preventDefault()
    e.stopPropagation()
    setDraggingOver(false)
    const file = e.dataTransfer?.files?.[0]
    if (file) {
      handleUploadFile(file)
    }
  }

  const isInspectedGeoreferenced = Boolean(
    inspectedDataset &&
      (inspectedDataset.georeference_status === "completed" ||
        inspectedDataset.extracted_metadata?.is_georeferenced ||
        (inspectedDataset.extracted_metadata?.crs &&
          (inspectedDataset.map_layer?.bounds || inspectedDataset.extracted_metadata?.bounds)))
  )

  const inspectedCRS =
    inspectedDataset?.extracted_metadata?.crs ||
    inspectedDataset?.map_layer?.bounds_crs ||
    ""

  const handleRegisterAndApply = async (e) => {
    e.preventDefault()
    if (!selectedDatasetId) {
      setRegisterError("Please upload or select a map file first.")
      return
    }
    setRegistering(true)
    setRegisterError("")
    try {
      const regRes = await registerBaseLayer({
        dataset_id: selectedDatasetId,
        name: registerName.trim() || inspectedDataset?.filename || "Custom Base Layer",
        description: registerDesc.trim(),
      })
      const created = regRes?.base_layer
      if (!created) {
        setRegisterError("Base layer registration did not return a valid layer.")
        return
      }

      let savedWs = null
      if (workspaceId) {
        const wsRes = await selectWorkspaceBaseLayer(workspaceId, {
          base_layer_type: "custom",
          base_layer_id: created.id,
          selected_view: "custom",
        })
        savedWs = wsRes?.workspace_base_layer || null
      }

      onConfirmed?.({
        baseLayer: created,
        baseLayerType: "custom",
        workspaceBaseLayer: savedWs,
      })
    } catch (err) {
      setRegisterError(errorText(err, "Failed to register custom base layer."))
    } finally {
      setRegistering(false)
    }
  }

  const filteredCustom = customLayers.filter((bl) => {
    if (!search.trim()) return true
    const q = search.trim().toLowerCase()
    return (
      (bl.name || "").toLowerCase().includes(q) ||
      (bl.description || "").toLowerCase().includes(q) ||
      (bl.source_crs || "").toLowerCase().includes(q) ||
      (bl.dataset_name || "").toLowerCase().includes(q)
    )
  })

  const handleConfirmSelection = async () => {
    if (!selectedLayer) return
    const isDefault = Boolean(selectedLayer.is_default || selectedLayer.id === "world_map")
    setSaving(true)
    setError("")
    try {
      let savedResult = null
      if (workspaceId) {
        const res = await selectWorkspaceBaseLayer(workspaceId, {
          base_layer_type: isDefault ? "world_map" : "custom",
          base_layer_id: isDefault ? "world_map" : selectedLayer.id,
          selected_view: isDefault ? "world" : "custom",
        })
        savedResult = res?.workspace_base_layer || null
      }
      onConfirmed?.({
        baseLayer: selectedLayer,
        baseLayerType: isDefault ? "world_map" : "custom",
        workspaceBaseLayer: savedResult,
      })
    } catch (err) {
      setError(errorText(err, "Failed to save base layer selection."))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-[1050] bg-black/70 backdrop-blur-xs flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="select-base-layer-title"
    >
      <div className="w-full max-w-5xl max-h-[90vh] rounded-[14px] bg-white dark:bg-[#191b1f] border border-neutral-200 dark:border-[#2e323a] shadow-2xl flex flex-col overflow-hidden">
        {/* Header */}
        <div className="px-6 py-4 border-b border-neutral-200 dark:border-[#2a2d34] flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-[8px] bg-[#3ecf8e]/15 text-[#3ecf8e] flex items-center justify-center">
              {mode === "upload" ? <Upload className="w-5 h-5" /> : <Globe className="w-5 h-5" />}
            </div>
            <div>
              <h2
                id="select-base-layer-title"
                className="text-base font-bold text-neutral-900 dark:text-white"
              >
                {mode === "upload" ? "Upload Custom Base Layer" : "Select Your Base Layer"}
              </h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                {mode === "upload"
                  ? `Upload a georeferenced map file (GeoTIFF, MBTiles, GeoPackage, GeoJSON, ZIP, or PNG/JPG) to use as the base layer for ${workspaceName}.`
                  : `Choose the geographic reference map for ${workspaceName}.`}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {/* Mode Switcher */}
            <div className="inline-flex items-center rounded-[7px] bg-neutral-100 dark:bg-[#121418] p-0.5 border border-neutral-200 dark:border-[#2a2d34]">
              <button
                type="button"
                onClick={() => setMode("select")}
                className={`px-3 py-1 rounded-[5px] text-xs font-semibold transition-colors cursor-pointer ${
                  mode === "select"
                    ? "bg-[#3ecf8e] text-[#171717]"
                    : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"
                }`}
              >
                My Base Layers
              </button>
              <button
                type="button"
                onClick={() => setMode("upload")}
                className={`inline-flex items-center gap-1 px-3 py-1 rounded-[5px] text-xs font-semibold transition-colors cursor-pointer ${
                  mode === "upload"
                    ? "bg-[#3ecf8e] text-[#171717]"
                    : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"
                }`}
              >
                <Upload className="w-3 h-3" />
                Upload Map File
              </button>
            </div>

            <button
              type="button"
              onClick={onCancel}
              className="p-1.5 rounded-[6px] text-neutral-400 hover:text-neutral-700 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#2a2d34] cursor-pointer"
              aria-label="Close base layer selector"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-5">
          {unavailableWarning && (
            <div className="flex items-start gap-2.5 p-3.5 rounded-[8px] bg-amber-500/10 border border-amber-500/30 text-xs text-amber-700 dark:text-amber-300">
              <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5 text-amber-500" />
              <div>
                <div className="font-semibold">Previous Base Layer Unavailable</div>
                <p className="mt-0.5 opacity-90">{unavailableWarning}</p>
              </div>
            </div>
          )}

          {error && (
            <div className="p-3 rounded-[8px] bg-red-500/10 border border-red-500/30 text-xs text-red-500">
              {error}
            </div>
          )}

          {mode === "upload" ? (
            <form id="custom-baselayer-upload-form" onSubmit={handleRegisterAndApply} className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2 space-y-4">
                {/* Upload vs Existing toggle */}
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => setUploadSourceTab("upload")}
                    className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] text-xs font-semibold cursor-pointer ${
                      uploadSourceTab === "upload"
                        ? "bg-neutral-900 dark:bg-[#2a2e37] text-white border border-[#3ecf8e]/50"
                        : "bg-neutral-100 dark:bg-[#14161a] text-neutral-600 dark:text-neutral-400 border border-neutral-200 dark:border-[#2a2d34]"
                    }`}
                  >
                    <Upload className="w-3.5 h-3.5 text-[#3ecf8e]" />
                    Upload New Map File
                  </button>
                  <button
                    type="button"
                    onClick={() => setUploadSourceTab("existing")}
                    className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] text-xs font-semibold cursor-pointer ${
                      uploadSourceTab === "existing"
                        ? "bg-neutral-900 dark:bg-[#2a2e37] text-white border border-[#3ecf8e]/50"
                        : "bg-neutral-100 dark:bg-[#14161a] text-neutral-600 dark:text-neutral-400 border border-neutral-200 dark:border-[#2a2d34]"
                    }`}
                  >
                    <FolderOpen className="w-3.5 h-3.5 text-[#3ecf8e]" />
                    Pick Existing Uploaded Dataset ({userFiles.length})
                  </button>
                </div>

                {uploadSourceTab === "upload" ? (
                  <div
                    onDragOver={(e) => {
                      e.preventDefault()
                      setDraggingOver(true)
                    }}
                    onDragLeave={() => setDraggingOver(false)}
                    onDrop={handleDrop}
                    className={`rounded-[12px] border-2 border-dashed p-7 text-center transition-colors ${
                      draggingOver
                        ? "border-[#3ecf8e] bg-[#3ecf8e]/10"
                        : "border-neutral-300 dark:border-[#343842] bg-neutral-50 dark:bg-[#14161a]"
                    }`}
                  >
                    <Upload className="w-9 h-9 text-[#3ecf8e] mx-auto mb-2.5" />
                    <div className="text-sm font-bold text-neutral-900 dark:text-white">
                      {uploading
                        ? "Uploading & Inspecting Map File…"
                        : "Drop your map file here, or browse to upload"}
                    </div>
                    <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 mb-4 max-w-md mx-auto">
                      Supports GeoTIFF (.tif/.tiff), MBTiles (.mbtiles), GeoPackage (.gpkg), GeoJSON (.geojson), Shapefile (.zip), and PNG/JPG images.
                    </p>
                    <label className="inline-flex items-center gap-2 px-4 py-2 rounded-[7px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-bold cursor-pointer shadow-xs">
                      <Upload className="w-3.5 h-3.5" />
                      <span>{uploading ? "Uploading…" : "Choose Map File"}</span>
                      <input
                        type="file"
                        className="hidden"
                        accept=".tif,.tiff,.mbtiles,.gpkg,.geojson,.json,.zip,.png,.jpg,.jpeg"
                        disabled={uploading}
                        onChange={(e) => {
                          const f = e.target.files?.[0]
                          if (f) handleUploadFile(f)
                          e.target.value = ""
                        }}
                      />
                    </label>
                  </div>
                ) : (
                  <div className="space-y-1.5">
                    <label className="block text-xs font-semibold text-neutral-700 dark:text-neutral-300">
                      Choose an Uploaded Dataset
                    </label>
                    {loadingFiles ? (
                      <div className="text-xs text-neutral-500 py-3">Loading datasets…</div>
                    ) : userFiles.length === 0 ? (
                      <div className="p-4 rounded-[8px] border border-dashed border-neutral-300 dark:border-[#343842] text-xs text-neutral-500">
                        No uploaded datasets yet. Switch to &ldquo;Upload New Map File&rdquo; to upload one now.
                      </div>
                    ) : (
                      <select
                        value={selectedDatasetId}
                        onChange={(e) => {
                          const chosen = userFiles.find((f) => f.id === e.target.value)
                          inspectDatasetById(e.target.value, chosen?.original_filename)
                        }}
                        className="w-full px-3 py-2.5 rounded-[7px] text-xs bg-white dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-neutral-900 dark:text-white"
                      >
                        <option value="">-- Select an uploaded dataset --</option>
                        {userFiles.map((f) => (
                          <option key={f.id} value={f.id}>
                            {f.original_filename} ({f.georeference_status === "completed" ? "Georeferenced" : "Needs Georeferencing"})
                          </option>
                        ))}
                      </select>
                    )}
                  </div>
                )}

                {/* Inspection status */}
                {inspectedDataset && (
                  <div
                    className={`rounded-[9px] p-3.5 border text-xs ${
                      isInspectedGeoreferenced
                        ? "bg-emerald-500/10 border-emerald-500/35"
                        : "bg-amber-500/10 border-amber-500/35"
                    }`}
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2 font-semibold">
                        {isInspectedGeoreferenced ? (
                          <>
                            <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0" />
                            <span className="text-emerald-700 dark:text-emerald-300">
                              Georeferenced &amp; Ready — {inspectedDataset.filename} ({inspectedCRS || "Detected CRS"})
                            </span>
                          </>
                        ) : (
                          <>
                            <AlertTriangle className="w-4 h-4 text-amber-500 shrink-0" />
                            <span className="text-amber-700 dark:text-amber-300">
                              {inspectedDataset.filename} has no embedded CRS / coordinates
                            </span>
                          </>
                        )}
                      </div>
                      {!isInspectedGeoreferenced && (
                        <button
                          type="button"
                          onClick={() => {
                            const returnTo = window.location.pathname + window.location.search
                            const blName = registerName.trim() || inspectedDataset.filename.replace(/\.[^/.]+$/, "")
                            const params = new URLSearchParams({
                              returnTo,
                              autoRegisterBaseLayer: "1",
                              baseLayerName: blName,
                            })
                            if (workspaceId) params.set("workspaceId", workspaceId)
                            onCancel?.()
                            navigate(`/datasets/${encodeURIComponent(inspectedDataset.id)}/georeference?${params.toString()}`)
                          }}
                          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-amber-500 hover:bg-amber-400 text-black font-bold text-[11px] cursor-pointer"
                        >
                          Georeference &amp; Use as Base Layer
                          <ExternalLink className="w-3 h-3" />
                        </button>
                      )}
                    </div>
                    {!isInspectedGeoreferenced && (
                      <p className="text-[11px] text-neutral-600 dark:text-neutral-300 mt-1.5 leading-relaxed">
                        Ordinary PNG/JPG or unreferenced TIFF images do not include geographic coordinates. Click <strong>Georeference &amp; Use as Base Layer</strong> to place control points first — once saved, it will automatically be registered and displayed on your map workspace.
                      </p>
                    )}
                  </div>
                )}

                {/* Name & Description */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                      Base Layer Name *
                    </label>
                    <input
                      type="text"
                      required
                      value={registerName}
                      onChange={(e) => setRegisterName(e.target.value)}
                      placeholder="e.g., Delhi Base Map"
                      className="w-full px-3 py-2 rounded-[6px] text-xs bg-white dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                      Description (optional)
                    </label>
                    <input
                      type="text"
                      value={registerDesc}
                      onChange={(e) => setRegisterDesc(e.target.value)}
                      placeholder="e.g., Municipal boundary & road survey"
                      className="w-full px-3 py-2 rounded-[6px] text-xs bg-white dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                    />
                  </div>
                </div>

                {registerError && (
                  <div className="p-3 rounded-[7px] bg-red-500/10 border border-red-500/30 text-xs text-red-500">
                    {registerError}
                  </div>
                )}
              </div>

              {/* Right column live preview of uploaded file */}
              <div>
                <div className="rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#14161a] overflow-hidden">
                  <div className="px-3.5 py-2 border-b border-neutral-200 dark:border-[#2a2d34] text-[11px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
                    Uploaded File Preview
                  </div>
                  <div className="h-44 bg-[#101216] flex items-center justify-center overflow-hidden p-2">
                    {inspectedDataset ? (
                      <img
                        src={datasetSourceImageUrl(inspectedDataset.id)}
                        alt={inspectedDataset.filename}
                        className="max-h-full max-w-full object-contain rounded"
                        onError={(e) => {
                          e.currentTarget.style.display = "none"
                        }}
                      />
                    ) : (
                      <div className="text-center text-neutral-500 px-4">
                        <Upload className="w-7 h-7 mx-auto mb-1.5 opacity-50" />
                        <div className="text-[11px]">Upload or select a map file to preview it here</div>
                      </div>
                    )}
                  </div>
                  <div className="p-3 text-[11px] space-y-1.5">
                    <div className="flex justify-between gap-2">
                      <span className="text-neutral-400">File</span>
                      <span className="font-semibold text-neutral-900 dark:text-white truncate">
                        {inspectedDataset?.filename || "None selected"}
                      </span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-neutral-400">Detected CRS</span>
                      <span className="font-mono text-neutral-800 dark:text-neutral-200">
                        {inspectedCRS || "Unassigned"}
                      </span>
                    </div>
                    <div className="flex justify-between gap-2">
                      <span className="text-neutral-400">Status</span>
                      <span
                        className={
                          isInspectedGeoreferenced
                            ? "text-emerald-500 font-semibold"
                            : "text-amber-500 font-semibold"
                        }
                      >
                        {inspectedDataset
                          ? isInspectedGeoreferenced
                            ? "Ready for Map"
                            : "Needs Georeferencing"
                          : "Waiting for file"}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </form>
          ) : loading ? (
            <div className="py-16 text-center text-xs text-neutral-500">
              Loading available base layers…
            </div>
          ) : (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              {/* Left 2 columns: Options 1 & 2 */}
              <div className="lg:col-span-2 space-y-5">
                {/* Option 1: Default World Map */}
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-[11px] font-bold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
                      Option 1 — Default World Map
                    </span>
                  </div>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    {defaultWorldMap && (
                      <BaseLayerCard
                        layer={defaultWorldMap}
                        selected={
                          selectedLayer?.id === "world_map" || Boolean(selectedLayer?.is_default)
                        }
                        onSelect={setSelectedLayer}
                      />
                    )}
                  </div>
                </div>

                {/* Option 2: My Custom Base Layers */}
                <div>
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 mb-2.5">
                    <span className="text-[11px] font-bold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
                      Option 2 — My Custom Base Layers ({customLayers.length})
                    </span>
                    <div className="flex items-center gap-2">
                      <div className="relative">
                        <Search className="w-3.5 h-3.5 text-neutral-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
                        <input
                          type="text"
                          value={search}
                          onChange={(e) => setSearch(e.target.value)}
                          placeholder="Search base layers..."
                          className="pl-8 pr-2.5 py-1 rounded-[6px] text-xs bg-white dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                        />
                      </div>
                      <button
                        type="button"
                        onClick={() => setMode("upload")}
                        className="inline-flex items-center gap-1 px-2.5 py-1 rounded-[6px] border border-[#3ecf8e]/50 text-[#24b47e] dark:text-[#3ecf8e] hover:bg-[#3ecf8e]/10 text-xs font-semibold cursor-pointer shrink-0"
                      >
                        <Plus className="w-3.5 h-3.5" />
                        Add New Base Layer
                      </button>
                    </div>
                  </div>

                  {filteredCustom.length === 0 ? (
                    <div className="rounded-[10px] border border-dashed border-neutral-300 dark:border-[#2e323a] p-6 text-center text-xs text-neutral-500 dark:text-neutral-400">
                      {customLayers.length === 0
                        ? "You have no custom base layers yet. Click \"Upload Map File\" above to upload a georeferenced city or site map, or open with the Default World Map."
                        : "No custom base layers match your search."}
                    </div>
                  ) : (
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      {filteredCustom.map((layer) => (
                        <BaseLayerCard
                          key={layer.id}
                          layer={layer}
                          selected={selectedLayer?.id === layer.id}
                          onSelect={setSelectedLayer}
                        />
                      ))}
                    </div>
                  )}
                </div>
              </div>

              {/* Right column: Selected Base Layer Preview */}
              <div>
                <BaseLayerPreview
                  layer={selectedLayer}
                  title="Preview Selected Base Layer"
                />
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-3.5 border-t border-neutral-200 dark:border-[#2a2d34] bg-neutral-50 dark:bg-[#15171b] flex items-center justify-between gap-3">
          {mode === "upload" ? (
            <button
              type="button"
              onClick={() => setMode("select")}
              className="inline-flex items-center gap-1.5 text-xs font-semibold text-neutral-600 dark:text-neutral-300 hover:text-[#3ecf8e] cursor-pointer"
            >
              ← Back to Saved Base Layers
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setMode("upload")}
              className="inline-flex items-center gap-1.5 text-xs font-semibold text-neutral-600 dark:text-neutral-300 hover:text-[#3ecf8e] cursor-pointer"
            >
              <Upload className="w-3.5 h-3.5" />
              Upload Map File as Base Layer
            </button>
          )}

          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={onCancel}
              className="px-4 py-2 rounded-[6px] text-xs font-semibold text-neutral-600 dark:text-neutral-300 hover:bg-neutral-200/70 dark:hover:bg-[#2a2d34] cursor-pointer"
            >
              Cancel
            </button>
            {mode === "upload" ? (
              <button
                type="submit"
                form="custom-baselayer-upload-form"
                disabled={registering || !selectedDatasetId || (inspectedDataset && !isInspectedGeoreferenced)}
                className="inline-flex items-center gap-2 px-4 py-2 rounded-[6px] text-xs font-bold bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] disabled:opacity-50 cursor-pointer shadow-xs"
              >
                {registering ? "Applying to Map…" : "Register & Show on Map"}
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            ) : (
              <button
                type="button"
                onClick={handleConfirmSelection}
                disabled={saving || !selectedLayer}
                className="inline-flex items-center gap-2 px-4 py-2 rounded-[6px] text-xs font-bold bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] disabled:opacity-50 cursor-pointer shadow-xs"
              >
                {saving ? "Opening Workspace…" : "Confirm and Open Workspace"}
                <ArrowRight className="w-3.5 h-3.5" />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
