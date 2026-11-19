import React, { useEffect, useState } from "react"
import { useNavigate } from "react-router-dom"
import {
  Plus,
  Search,
  Upload,
  X,
  AlertTriangle,
  CheckCircle2,
  ExternalLink,
  RefreshCw,
} from "lucide-react"
import {
  listBaseLayers,
  registerBaseLayer,
  updateBaseLayer,
  deleteBaseLayer,
  listProfileFiles,
  uploadDataset,
  getDataset,
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

export default function BaseLayerManager({
  onBaseLayerCreated,
  onNotify,
  initialShowAddModal = false,
  compact = false,
}) {
  const navigate = useNavigate()
  const [defaultWorldMap, setDefaultWorldMap] = useState(null)
  const [baseLayers, setBaseLayers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [search, setSearch] = useState("")
  const [previewLayer, setPreviewLayer] = useState(null)

  // Register New Base Layer Modal
  const [showAddModal, setShowAddModal] = useState(initialShowAddModal)
  const [sourceMode, setSourceMode] = useState("existing") // "existing" | "upload"
  const [userFiles, setUserFiles] = useState([])
  const [loadingFiles, setLoadingFiles] = useState(false)
  const [selectedDatasetId, setSelectedDatasetId] = useState("")
  const [inspectedDataset, setInspectedDataset] = useState(null)
  const [uploading, setUploading] = useState(false)
  const [registerName, setRegisterName] = useState("")
  const [registerDesc, setRegisterDesc] = useState("")
  const [registerError, setRegisterError] = useState("")
  const [submitting, setSubmitting] = useState(false)

  // Edit Modal
  const [editingLayer, setEditingLayer] = useState(null)
  const [editName, setEditName] = useState("")
  const [editDesc, setEditDesc] = useState("")
  const [savingEdit, setSavingEdit] = useState(false)

  const loadBaseLayers = async () => {
    setLoading(true)
    setError("")
    try {
      const data = await listBaseLayers()
      setDefaultWorldMap(data?.default_base_layer || null)
      const list = data?.base_layers || []
      setBaseLayers(list)
      if (!previewLayer && data?.default_base_layer) {
        setPreviewLayer(data.default_base_layer)
      }
    } catch (err) {
      setError(errorText(err, "Failed to load base layers."))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadBaseLayers()
  }, [])

  const loadExistingDatasets = async () => {
    setLoadingFiles(true)
    try {
      const data = await listProfileFiles()
      setUserFiles(data?.files || [])
    } catch {
      setUserFiles([])
    } finally {
      setLoadingFiles(false)
    }
  }

  useEffect(() => {
    if (showAddModal) {
      loadExistingDatasets()
    }
  }, [showAddModal])

  const inspectSelectedDataset = async (dsId, fallbackName = "") => {
    setSelectedDatasetId(dsId)
    setRegisterError("")
    if (!dsId) {
      setInspectedDataset(null)
      return
    }
    try {
      const ds = await getDataset(dsId)
      setInspectedDataset(ds)
      if (!registerName.trim()) {
        const clean = (ds?.filename || fallbackName || "").replace(/\.[^/.]+$/, "")
        if (clean) setRegisterName(clean)
      }
    } catch (err) {
      setInspectedDataset(null)
      setRegisterError(errorText(err, "Unable to inspect dataset metadata."))
    }
  }

  const handleFileUpload = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    setRegisterError("")
    try {
      const uploaded = await uploadDataset(file)
      const dsId = uploaded?.id || uploaded?.dataset_id
      if (dsId) {
        setRegisterName((prev) => prev || file.name.replace(/\.[^/.]+$/, ""))
        await inspectSelectedDataset(dsId, file.name)
        await loadExistingDatasets()
      }
    } catch (err) {
      setRegisterError(errorText(err, "File upload failed."))
    } finally {
      setUploading(false)
      e.target.value = ""
    }
  }

  const isInspectedGeoreferenced = Boolean(
    inspectedDataset &&
      (inspectedDataset.georeference_status === "completed" ||
        inspectedDataset.extracted_metadata?.is_georeferenced ||
        (inspectedDataset.extracted_metadata?.crs &&
          (inspectedDataset.map_layer?.bounds || inspectedDataset.extracted_metadata?.bounds)))
  )

  const handleRegisterSubmit = async (e) => {
    e.preventDefault()
    if (!selectedDatasetId) {
      setRegisterError("Select or upload a dataset first.")
      return
    }
    setSubmitting(true)
    setRegisterError("")
    try {
      const res = await registerBaseLayer({
        dataset_id: selectedDatasetId,
        name: registerName.trim() || inspectedDataset?.filename || "Custom Base Layer",
        description: registerDesc.trim(),
      })
      const created = res?.base_layer
      setShowAddModal(false)
      setSelectedDatasetId("")
      setInspectedDataset(null)
      setRegisterName("")
      setRegisterDesc("")
      await loadBaseLayers()
      if (created) {
        setPreviewLayer(created)
        onBaseLayerCreated?.(created)
        onNotify?.(`Base layer "${created.name}" registered!`)
      }
    } catch (err) {
      setRegisterError(errorText(err, "Failed to register base layer."))
    } finally {
      setSubmitting(false)
    }
  }

  const handleSaveEdit = async (e) => {
    e.preventDefault()
    if (!editingLayer || !editName.trim()) return
    setSavingEdit(true)
    try {
      const res = await updateBaseLayer(editingLayer.id, {
        name: editName.trim(),
        description: editDesc.trim(),
      })
      setEditingLayer(null)
      await loadBaseLayers()
      if (res?.base_layer) setPreviewLayer(res.base_layer)
      onNotify?.("Base layer updated.")
    } catch (err) {
      onNotify?.(errorText(err, "Failed to update base layer."))
    } finally {
      setSavingEdit(false)
    }
  }

  const handleDelete = async (layer) => {
    if (
      !window.confirm(
        `Remove base layer registration "${layer.name}"? Your original uploaded dataset will NOT be deleted.`
      )
    ) {
      return
    }
    try {
      await deleteBaseLayer(layer.id)
      if (previewLayer?.id === layer.id) {
        setPreviewLayer(defaultWorldMap)
      }
      await loadBaseLayers()
      onNotify?.(`Base layer "${layer.name}" removed.`)
    } catch (err) {
      onNotify?.(errorText(err, "Failed to remove base layer."))
    }
  }

  const filteredLayers = baseLayers.filter((bl) => {
    if (!search.trim()) return true
    const q = search.trim().toLowerCase()
    return (
      (bl.name || "").toLowerCase().includes(q) ||
      (bl.description || "").toLowerCase().includes(q) ||
      (bl.source_crs || "").toLowerCase().includes(q) ||
      (bl.dataset_name || "").toLowerCase().includes(q)
    )
  })

  return (
    <div className="space-y-5">
      {/* Header toolbar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
            Base Layer Library
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            Manage the default World Map and your registered custom geographic base layers (e.g., city maps, campus plans, GeoTIFFs).
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-neutral-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search base layers..."
              className="pl-8 pr-3 py-1.5 rounded-[6px] text-xs bg-white dark:bg-[#1e1e1e] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
            />
          </div>
          <button
            type="button"
            onClick={loadBaseLayers}
            className="p-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] cursor-pointer"
            title="Refresh base layers"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={() => {
              setRegisterError("")
              setShowAddModal(true)
            }}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-semibold cursor-pointer shadow-xs"
          >
            <Plus className="w-3.5 h-3.5" />
            Add Base Layer
          </button>
        </div>
      </div>

      {error && (
        <div className="p-3 rounded-[8px] bg-red-500/10 border border-red-500/20 text-xs text-red-500">
          {error}
        </div>
      )}

      {loading ? (
        <div className="py-12 text-center text-xs text-neutral-500">Loading base layers…</div>
      ) : (
        <div className={`grid gap-5 ${compact ? "grid-cols-1" : "grid-cols-1 lg:grid-cols-3"}`}>
          <div className={compact ? "space-y-4" : "lg:col-span-2 space-y-5"}>
            {/* System Default World Map */}
            {defaultWorldMap && (
              <div>
                <div className="text-[11px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400 mb-2">
                  Default System Base Layer
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <BaseLayerCard
                    layer={defaultWorldMap}
                    selected={previewLayer?.id === defaultWorldMap.id}
                    onSelect={setPreviewLayer}
                    onPreview={setPreviewLayer}
                    showManagementActions
                  />
                </div>
              </div>
            )}

            {/* User Custom Base Layers */}
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400 mb-2">
                My Custom Base Layers ({filteredLayers.length})
              </div>
              {filteredLayers.length === 0 ? (
                <div className="rounded-[10px] border border-dashed border-neutral-300 dark:border-[#2e2e2e] p-8 text-center">
                  <p className="text-xs font-medium text-neutral-700 dark:text-neutral-300">
                    No custom base layers registered yet.
                  </p>
                  <p className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-1 mb-3">
                    Upload a GeoTIFF, MBTiles, GeoPackage, or georeferenced city/campus map to use it as a workspace background.
                  </p>
                  <button
                    type="button"
                    onClick={() => setShowAddModal(true)}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-semibold cursor-pointer"
                  >
                    <Plus className="w-3.5 h-3.5" />
                    Register Custom Base Layer
                  </button>
                </div>
              ) : (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {filteredLayers.map((layer) => (
                    <BaseLayerCard
                      key={layer.id}
                      layer={layer}
                      selected={previewLayer?.id === layer.id}
                      onSelect={setPreviewLayer}
                      onPreview={setPreviewLayer}
                      onEdit={(l) => {
                        setEditingLayer(l)
                        setEditName(l.name)
                        setEditDesc(l.description || "")
                      }}
                      onDelete={handleDelete}
                      showManagementActions
                    />
                  ))}
                </div>
              )}
            </div>
          </div>

          {!compact && (
            <div>
              <BaseLayerPreview layer={previewLayer} />
            </div>
          )}
        </div>
      )}

      {/* Add New Base Layer Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-[1100] bg-black/60 backdrop-blur-xs flex items-center justify-center p-4">
          <div className="w-full max-w-xl rounded-[12px] bg-white dark:bg-[#1e1e1e] border border-neutral-200 dark:border-[#2e2e2e] shadow-2xl overflow-hidden">
            <div className="flex items-center justify-between px-5 py-3.5 border-b border-neutral-200 dark:border-[#2e2e2e]">
              <div>
                <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
                  Register Custom Base Layer
                </h3>
                <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
                  Reference an uploaded geographic dataset without duplicating its files.
                </p>
              </div>
              <button
                type="button"
                onClick={() => setShowAddModal(false)}
                className="p-1 rounded text-neutral-400 hover:text-neutral-700 dark:hover:text-white cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <form onSubmit={handleRegisterSubmit} className="p-5 space-y-4 max-h-[80vh] overflow-y-auto">
              {/* Source selection tabs */}
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => setSourceMode("existing")}
                  className={`px-3 py-1.5 rounded-[6px] text-xs font-semibold cursor-pointer ${
                    sourceMode === "existing"
                      ? "bg-[#3ecf8e] text-[#171717]"
                      : "bg-neutral-100 dark:bg-[#2a2a2a] text-neutral-600 dark:text-neutral-300"
                  }`}
                >
                  Select Existing Dataset
                </button>
                <button
                  type="button"
                  onClick={() => setSourceMode("upload")}
                  className={`px-3 py-1.5 rounded-[6px] text-xs font-semibold cursor-pointer ${
                    sourceMode === "upload"
                      ? "bg-[#3ecf8e] text-[#171717]"
                      : "bg-neutral-100 dark:bg-[#2a2a2a] text-neutral-600 dark:text-neutral-300"
                  }`}
                >
                  Upload New Map File
                </button>
              </div>

              {sourceMode === "existing" ? (
                <div>
                  <label className="block text-[11px] font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                    Uploaded Dataset
                  </label>
                  {loadingFiles ? (
                    <div className="text-xs text-neutral-500 py-2">Loading your datasets…</div>
                  ) : (
                    <select
                      value={selectedDatasetId}
                      onChange={(e) => {
                        const chosen = userFiles.find((f) => f.id === e.target.value)
                        inspectSelectedDataset(e.target.value, chosen?.original_filename)
                      }}
                      className="w-full px-3 py-2 rounded-[6px] text-xs bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white"
                    >
                      <option value="">-- Choose an uploaded dataset --</option>
                      {userFiles.map((f) => (
                        <option key={f.id} value={f.id}>
                          {f.original_filename} ({f.georeference_status === "completed" ? "Georeferenced" : "Unreferenced"})
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              ) : (
                <div className="rounded-[8px] border border-dashed border-neutral-300 dark:border-[#343434] p-5 text-center">
                  <Upload className="w-6 h-6 text-[#3ecf8e] mx-auto mb-2" />
                  <p className="text-xs font-medium text-neutral-800 dark:text-neutral-200">
                    Upload GeoTIFF, TIFF, MBTiles, GeoPackage, GeoJSON, or PNG/JPG
                  </p>
                  <p className="text-[11px] text-neutral-500 mt-0.5 mb-3">
                    Unreferenced images will prompt you to georeference them before base layer registration.
                  </p>
                  <label className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-[6px] bg-neutral-900 dark:bg-[#2e2e2e] text-white text-xs font-semibold cursor-pointer hover:opacity-90">
                    <Upload className="w-3.5 h-3.5" />
                    {uploading ? "Uploading & Inspecting…" : "Choose File"}
                    <input
                      type="file"
                      className="hidden"
                      accept=".tif,.tiff,.png,.jpg,.jpeg,.gpkg,.mbtiles,.geojson,.json,.zip"
                      onChange={handleFileUpload}
                      disabled={uploading}
                    />
                  </label>
                </div>
              )}

              {/* Inspection summary */}
              {inspectedDataset && (
                <div
                  className={`rounded-[8px] p-3 border text-xs ${
                    isInspectedGeoreferenced
                      ? "bg-emerald-500/8 border-emerald-500/30"
                      : "bg-amber-500/10 border-amber-500/30"
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-1.5 font-semibold">
                      {isInspectedGeoreferenced ? (
                        <>
                          <CheckCircle2 className="w-4 h-4 text-emerald-500" />
                          <span className="text-emerald-700 dark:text-emerald-300">
                            Georeferenced & Ready ({inspectedDataset.extracted_metadata?.crs || inspectedDataset.map_layer?.bounds_crs || "Detected CRS"})
                          </span>
                        </>
                      ) : (
                        <>
                          <AlertTriangle className="w-4 h-4 text-amber-500" />
                          <span className="text-amber-700 dark:text-amber-300">
                            Missing CRS / Not Georeferenced
                          </span>
                        </>
                      )}
                    </div>
                    {!isInspectedGeoreferenced && (
                      <button
                        type="button"
                        onClick={() => {
                          setShowAddModal(false)
                          navigate(`/datasets/${encodeURIComponent(inspectedDataset.id)}/georeference?returnTo=${encodeURIComponent("/dashboard?tab=base-layers")}`)
                        }}
                        className="inline-flex items-center gap-1 px-2.5 py-1 rounded bg-amber-500 text-black font-semibold text-[11px] cursor-pointer"
                      >
                        Georeference First
                        <ExternalLink className="w-3 h-3" />
                      </button>
                    )}
                  </div>
                  {!isInspectedGeoreferenced && (
                    <p className="text-[11px] text-neutral-600 dark:text-neutral-300 mt-1">
                      Ordinary PNG/JPG or unreferenced TIFF files cannot be assigned a CRS silently. Georeference this dataset first, then register it as a base layer.
                    </p>
                  )}
                </div>
              )}

              <div className="grid grid-cols-1 gap-3">
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
                    className="w-full px-3 py-2 rounded-[6px] text-xs bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                  />
                </div>
                <div>
                  <label className="block text-[11px] font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                    Description
                  </label>
                  <textarea
                    rows={2}
                    value={registerDesc}
                    onChange={(e) => setRegisterDesc(e.target.value)}
                    placeholder="Optional notes on coverage, survey year, or source..."
                    className="w-full px-3 py-2 rounded-[6px] text-xs bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                  />
                </div>
              </div>

              {registerError && (
                <div className="p-2.5 rounded-[6px] bg-red-500/10 border border-red-500/30 text-[11px] text-red-500">
                  {registerError}
                </div>
              )}

              <div className="flex items-center justify-end gap-2 pt-2 border-t border-neutral-200 dark:border-[#2e2e2e]">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-3.5 py-1.5 rounded-[6px] text-xs font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting || !selectedDatasetId || (inspectedDataset && !isInspectedGeoreferenced)}
                  className="px-4 py-1.5 rounded-[6px] text-xs font-semibold bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] disabled:opacity-50 cursor-pointer"
                >
                  {submitting ? "Registering…" : "Save Base Layer"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Base Layer Modal */}
      {editingLayer && (
        <div className="fixed inset-0 z-[1100] bg-black/60 backdrop-blur-xs flex items-center justify-center p-4">
          <div className="w-full max-w-md rounded-[12px] bg-white dark:bg-[#1e1e1e] border border-neutral-200 dark:border-[#2e2e2e] shadow-2xl overflow-hidden">
            <div className="flex items-center justify-between px-4 py-3 border-b border-neutral-200 dark:border-[#2e2e2e]">
              <h4 className="text-sm font-bold text-neutral-900 dark:text-white">Edit Base Layer</h4>
              <button
                type="button"
                onClick={() => setEditingLayer(null)}
                className="p-1 text-neutral-400 hover:text-white cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <form onSubmit={handleSaveEdit} className="p-4 space-y-3">
              <div>
                <label className="block text-[11px] font-semibold text-neutral-600 dark:text-neutral-300 mb-1">
                  Name
                </label>
                <input
                  type="text"
                  required
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  className="w-full px-3 py-2 rounded-[6px] text-xs bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white"
                />
              </div>
              <div>
                <label className="block text-[11px] font-semibold text-neutral-600 dark:text-neutral-300 mb-1">
                  Description
                </label>
                <textarea
                  rows={2}
                  value={editDesc}
                  onChange={(e) => setEditDesc(e.target.value)}
                  className="w-full px-3 py-2 rounded-[6px] text-xs bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white"
                />
              </div>
              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setEditingLayer(null)}
                  className="px-3 py-1.5 rounded-[6px] text-xs text-neutral-500 hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={savingEdit}
                  className="px-4 py-1.5 rounded-[6px] text-xs font-semibold bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] cursor-pointer"
                >
                  {savingEdit ? "Saving…" : "Save Changes"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
