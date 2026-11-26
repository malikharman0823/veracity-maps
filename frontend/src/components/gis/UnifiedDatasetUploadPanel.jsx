import React, { useRef, useState } from "react"
import {
  Upload,
  Layers,
  Image as ImageIcon,
  CheckCircle2,
  AlertTriangle,
  Loader2,
  RotateCcw,
  Trash2,
  XCircle,
  ArrowLeft,
  ArrowRight,
  Compass,
  FileText,
} from "lucide-react"
import {
  uploadUnifiedFile,
  startUnifiedImport,
  getUnifiedJob,
  cancelUnifiedJob,
  readableApiError,
} from "../../api"

// Strictly approved formats per import category
const RASTER_ACCEPT = ".png,.jpg,.jpeg,.pdf,.dxf"
const VECTOR_ACCEPT = ".gpkg,.geojson,.png,.jpg,.jpeg"

const RASTER_EXTS = new Set([".png", ".jpg", ".jpeg", ".pdf", ".dxf"])
const VECTOR_EXTS = new Set([".gpkg", ".geojson", ".png", ".jpg", ".jpeg"])

const STATUS_BADGE = {
  uploaded: "bg-sky-500/10 text-sky-600 dark:text-sky-400 border-sky-500/25",
  inspecting: "bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/25",
  validating: "bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/25",
  processing: "bg-indigo-500/10 text-indigo-600 dark:text-indigo-400 border-indigo-500/25",
  ready: "bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] border-[#3ecf8e]/30",
  failed: "bg-red-500/10 text-red-600 dark:text-red-400 border-red-500/25",
  cancelled: "bg-neutral-500/10 text-neutral-500 dark:text-neutral-400 border-neutral-500/25",
}

const STATUS_LABEL = {
  uploaded: "Uploaded",
  inspecting: "Inspecting",
  validating: "Validating",
  processing: "Processing",
  ready: "Ready",
  failed: "Failed",
  cancelled: "Cancelled",
}

function formatBytes(bytes) {
  if (!bytes || bytes <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB"]
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function getExt(name) {
  const idx = String(name || "").lastIndexOf(".")
  return idx >= 0 ? name.slice(idx).toLowerCase() : ""
}

function errorText(err, fallback) {
  const res = readableApiError(err, fallback)
  if (typeof res === "string") return res
  if (res && typeof res === "object") {
    return res.details ? `${res.message} (${typeof res.details === "string" ? res.details : res.details.stage || ""})` : res.message || fallback
  }
  return fallback
}

/**
 * Single Unified Add Data & Import Interface for Veracity Maps.
 * Enforces strict Raster (.png, .jpg, .jpeg, .pdf, .dxf) and Vector (.gpkg, .geojson, .png, .jpg, .jpeg) formats,
 * processes each queued file independently, previews sub-layers/pages & georeferencing status,
 * and registers only fully validated, atomically committed layers into the workspace.
 */
export default function UnifiedDatasetUploadPanel({
  workspaceId = "",
  workspaceName = "Workspace",
  initialFocusSection = null, // "raster" | "vector" | null
  primaryActionLabel = "Open Workspace",
  showBackButton = true,
  autoAddToWorkspace = true,
  resolveProjectId = null,
  onBack,
  onComplete,
}) {
  const [category, setCategory] = useState(initialFocusSection === "vector" ? "vector" : "raster")
  const [queue, setQueue] = useState([])
  const [dragging, setDragging] = useState(false)
  const fileInputRef = useRef(null)
  const resolvedProjectIdRef = useRef(workspaceId || "")

  const ensureProjectId = async (fallbackName = "") => {
    if (workspaceId && workspaceId !== "default") return workspaceId
    if (resolvedProjectIdRef.current && resolvedProjectIdRef.current !== "default") {
      return resolvedProjectIdRef.current
    }
    if (typeof resolveProjectId === "function") {
      const pid = await resolveProjectId(fallbackName)
      if (pid) {
        resolvedProjectIdRef.current = pid
        return pid
      }
    }
    return workspaceId || "default"
  }

  const buildCompletionPayload = (layers, targetProjectId) => {
    const addedVectorLayers = layers.filter((l) => l.layer_type === "vector")
    const addedRasterLayers = layers.filter(
      (l) => l.layer_type === "raster" || l.layer_type === "image_overlay"
    )
    const rasterItems = addedRasterLayers.map((l) => ({
      datasetId: l.session_dataset_id || l.dataset_id,
      filename: l.name,
      width: l.width || 0,
      height: l.height || 0,
      crs: l.crs || "",
      isGeoreferenced: !l.requires_georeferencing,
      layerType: l.layer_type,
    }))
    return {
      projectId: targetProjectId,
      addedVectorLayers,
      addedRasterLayers,
      addedLayers: layers,
      rasterItems,
    }
  }

  const updateItem = (localId, patch) => {
    setQueue((prev) => prev.map((it) => (it.localId === localId ? { ...it, ...patch } : it)))
  }

  const processUpload = async (item) => {
    const allowed = item.category === "vector" ? VECTOR_EXTS : RASTER_EXTS
    const ext = getExt(item.file.name)
    if (!allowed.has(ext)) {
      updateItem(item.localId, {
        status: "failed",
        error: `Unsupported file extension "${ext || "none"}" for ${item.category} import.`,
      })
      return { layers: [], needsSelection: false }
    }

    updateItem(item.localId, { status: "inspecting", error: "" })
    try {
      const targetProjectId = await ensureProjectId(item.file.name.replace(/\.[^/.]+$/, ""))
      const uploaded = await uploadUnifiedFile(item.file, {
        projectId: targetProjectId,
        importCategory: item.category,
      })
      const insp = uploaded?.inspection || {}
      const subLayers = Array.isArray(insp.sub_layers) ? insp.sub_layers : []
      const selectedLayers = new Set(
        insp.detected_format === "gpkg" ? subLayers.map((l) => l.name) : []
      )
      const selectedPages = new Set(
        insp.detected_format === "pdf" ? subLayers.map((l) => l.page_number) : []
      )

      const updatedItem = {
        ...item,
        projectId: targetProjectId,
        status: "uploaded",
        uploadId: uploaded.id,
        uploadRecord: uploaded,
        inspection: insp,
        selectedLayers,
        selectedPages,
        error: "",
      }
      updateItem(item.localId, updatedItem)

      // Only pause for user selection when a GeoPackage has >1 layer or a PDF has >1 page.
      // All single-layer files (PNG, JPG, GeoJSON, DXF, 1-page PDF, 1-layer GPKG) import automatically!
      const needsSelection =
        (insp.detected_format === "gpkg" && subLayers.length > 1) ||
        (insp.detected_format === "pdf" && subLayers.length > 1)

      if (!needsSelection) {
        const layers = await handleConfirmImport(updatedItem, targetProjectId)
        return { layers, needsSelection: false, projectId: targetProjectId }
      }
      return { layers: [], needsSelection: true, projectId: targetProjectId }
    } catch (err) {
      updateItem(item.localId, {
        status: "failed",
        error: errorText(err, `Validation failed for "${item.file.name}".`),
      })
      return { layers: [], needsSelection: false }
    }
  }

  const enqueueFiles = async (fileList) => {
    const files = Array.from(fileList || [])
    if (files.length === 0) return
    const newItems = files.map((file) => ({
      localId: `${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
      file,
      name: file.name,
      size: file.size,
      category,
      status: "validating",
      uploadId: null,
      uploadRecord: null,
      inspection: null,
      selectedLayers: new Set(),
      selectedPages: new Set(),
      jobId: null,
      importedDataset: null,
      importedLayers: [],
      error: "",
    }))
    setQueue((prev) => [...newItems, ...prev])

    // Process each file independently so one invalid file never blocks or corrupts others.
    const results = await Promise.all(newItems.map((item) => processUpload(item)))
    const allAutoLayers = results.flatMap((r) => r?.layers || [])
    const anyNeedsSelection = results.some((r) => r?.needsSelection)
    const targetProjectId = results.find((r) => r?.projectId)?.projectId || workspaceId || resolvedProjectIdRef.current

    if (autoAddToWorkspace && !anyNeedsSelection && allAutoLayers.length > 0) {
      onComplete?.(buildCompletionPayload(allAutoLayers, targetProjectId))
    }
  }

  const handleConfirmImport = async (item, overrideProjectId = "") => {
    if (!item.uploadId) return []
    updateItem(item.localId, { status: "processing", error: "" })
    try {
      const targetProjectId = overrideProjectId || item.projectId || (await ensureProjectId(item.name))
      const payload = {
        project_id: targetProjectId,
        import_category: item.category,
        idempotency_key: `${item.uploadId}_${item.category}`,
        sync: true,
      }
      if (item.inspection?.detected_format === "gpkg") {
        payload.selected_layers = Array.from(item.selectedLayers)
      }
      if (item.inspection?.detected_format === "pdf") {
        payload.selected_pages = Array.from(item.selectedPages).sort((a, b) => a - b)
      }

      const job = await startUnifiedImport(item.uploadId, payload)
      if (job?.status === "ready") {
        const layers = job.layers || []
        updateItem(item.localId, {
          status: "ready",
          jobId: job.id,
          importedDataset: job.dataset || null,
          importedLayers: layers,
          error: "",
        })
        return layers
      }
      if (job?.status === "cancelled") {
        updateItem(item.localId, { status: "cancelled", jobId: job.id, error: "Import cancelled." })
        return []
      }
      if (job?.status === "failed") {
        updateItem(item.localId, {
          status: "failed",
          jobId: job.id,
          error: job.error_message || "Import failed during processing.",
        })
        return []
      }

      // Poll if backend returned async job
      if (job?.id) {
        updateItem(item.localId, { jobId: job.id })
        for (let i = 0; i < 60; i++) {
          await new Promise((r) => setTimeout(r, 800))
          const current = await getUnifiedJob(job.id)
          if (current?.status === "ready") {
            const layers = current.layers || []
            updateItem(item.localId, {
              status: "ready",
              importedDataset: current.dataset || null,
              importedLayers: layers,
            })
            return layers
          }
          if (current?.status === "failed" || current?.status === "cancelled") {
            updateItem(item.localId, {
              status: current.status,
              error: current.error_message || `Job ${current.status}.`,
            })
            return []
          }
        }
      }
    } catch (err) {
      updateItem(item.localId, {
        status: "failed",
        error: errorText(err, `Import failed for "${item.name}".`),
      })
    }
    return []
  }

  const handleCancelJob = async (item) => {
    if (item.jobId && item.status === "processing") {
      try {
        await cancelUnifiedJob(item.jobId)
      } catch {}
    }
    updateItem(item.localId, { status: "cancelled", error: "Cancelled by user." })
  }

  const handleRemoveItem = (localId) => {
    setQueue((prev) => prev.filter((it) => it.localId !== localId))
  }

  const toggleSubSelection = (localId, field, value) => {
    setQueue((prev) =>
      prev.map((it) => {
        if (it.localId !== localId) return it
        const next = new Set(it[field])
        if (next.has(value)) next.delete(value)
        else next.add(value)
        return { ...it, [field]: next }
      })
    )
  }

  const readyItems = queue.filter((it) => it.status === "ready")
  const pendingConfirmItems = queue.filter((it) => it.status === "uploaded")

  const handleConfirmAllReadyToImport = async () => {
    for (const item of pendingConfirmItems) {
      await handleConfirmImport(item)
    }
  }

  const [finishing, setFinishing] = useState(false)

  const handleFinish = async () => {
    if (finishing) return
    setFinishing(true)
    try {
      // Auto-confirm any validated files still waiting in "uploaded" status so nothing is silently skipped
      const newlyImported = []
      for (const item of pendingConfirmItems) {
        const layers = await handleConfirmImport(item)
        if (Array.isArray(layers) && layers.length > 0) {
          newlyImported.push(...layers)
        }
      }
      const addedLayers = [...readyItems.flatMap((it) => it.importedLayers || []), ...newlyImported]
      const addedVectorLayers = addedLayers.filter((l) => l.layer_type === "vector")
      const addedRasterLayers = addedLayers.filter(
        (l) => l.layer_type === "raster" || l.layer_type === "image_overlay"
      )
      const rasterItems = addedRasterLayers.map((l) => ({
        datasetId: l.session_dataset_id || l.dataset_id,
        filename: l.name,
        width: l.width || 0,
        height: l.height || 0,
        crs: l.crs || "",
        isGeoreferenced: !l.requires_georeferencing,
        layerType: l.layer_type,
      }))
      onComplete?.({
        addedVectorLayers,
        addedRasterLayers,
        addedLayers,
        rasterItems,
      })
    } finally {
      setFinishing(false)
    }
  }

  return (
    <div className="w-full max-w-4xl mx-auto space-y-5 text-neutral-900 dark:text-neutral-100">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">Add Data — {workspaceName}</h2>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            Every file is deeply parsed, validated, and staged atomically before any layer is added.
          </p>
        </div>
        {showBackButton && onBack && (
          <button
            type="button"
            onClick={onBack}
            className="inline-flex items-center gap-1.5 h-8 px-3 rounded-[6px] text-xs font-medium border border-neutral-200 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#262626] cursor-pointer"
          >
            <ArrowLeft className="w-3.5 h-3.5" />
            Back
          </button>
        )}
      </div>

      {/* Category Switcher */}
      <div className="grid grid-cols-2 gap-2 p-1 rounded-lg bg-neutral-100 dark:bg-[#1a1a1a] border border-neutral-200 dark:border-[#2a2a2a]">
        <button
          type="button"
          onClick={() => setCategory("raster")}
          className={`flex items-center justify-center gap-2 py-2 px-3 rounded-md text-xs font-medium transition-colors cursor-pointer ${
            category === "raster"
              ? "bg-white dark:bg-[#262626] text-neutral-900 dark:text-white shadow-xs"
              : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200"
          }`}
        >
          <ImageIcon className="w-4 h-4 text-[#3ecf8e]" />
          <span>Raster Import</span>
          <span className="text-[11px] opacity-70">(.png, .jpg, .pdf, .dxf)</span>
        </button>
        <button
          type="button"
          onClick={() => setCategory("vector")}
          className={`flex items-center justify-center gap-2 py-2 px-3 rounded-md text-xs font-medium transition-colors cursor-pointer ${
            category === "vector"
              ? "bg-white dark:bg-[#262626] text-neutral-900 dark:text-white shadow-xs"
              : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200"
          }`}
        >
          <Layers className="w-4 h-4 text-[#3ecf8e]" />
          <span>Vector Import</span>
          <span className="text-[11px] opacity-70">(.gpkg, .geojson, .png/.jpg overlay)</span>
        </button>
      </div>

      {/* Unified Drag-and-Drop Zone */}
      <div
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault()
          setDragging(false)
          enqueueFiles(e.dataTransfer?.files)
        }}
        onClick={() => fileInputRef.current?.click()}
        className={`rounded-xl border-2 border-dashed p-8 text-center transition-colors cursor-pointer ${
          dragging
            ? "border-[#3ecf8e] bg-[#3ecf8e]/10"
            : "border-neutral-300 dark:border-[#333333] hover:border-[#3ecf8e]/60 bg-neutral-50/70 dark:bg-[#161616]"
        }`}
      >
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept={category === "vector" ? VECTOR_ACCEPT : RASTER_ACCEPT}
          onChange={(e) => {
            enqueueFiles(e.target.files)
            e.target.value = ""
          }}
          className="hidden"
        />
        <Upload className="w-7 h-7 mx-auto mb-2 text-[#3ecf8e]" />
        <p className="text-sm font-medium">
          Drop {category === "vector" ? "Vector" : "Raster"} files here, or{" "}
          <span className="text-[#24b47e] dark:text-[#3ecf8e] underline">Browse Files</span>
        </p>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1">
          {category === "raster"
            ? "Strictly supported: PNG, JPG/JPEG, PDF (per-page raster), DXF (parsed & rasterized). Max 500 MB per file."
            : "Strictly supported: GeoPackage (.gpkg), GeoJSON (.geojson, WGS84), and PNG/JPG (registered as Image Overlay). Max 500 MB per file."}
        </p>
      </div>

      {/* Independent File Queue */}
      {queue.length > 0 && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">
              Import Queue ({queue.length})
            </h3>
            {pendingConfirmItems.length > 1 && (
              <button
                type="button"
                onClick={handleConfirmAllReadyToImport}
                className="inline-flex items-center gap-1.5 h-7 px-3 rounded-[6px] text-xs font-medium bg-[#3ecf8e] text-black hover:bg-[#34b27b] cursor-pointer"
              >
                Import All Validated ({pendingConfirmItems.length})
              </button>
            )}
          </div>

          <div className="space-y-2.5">
            {queue.map((item) => {
              const insp = item.inspection
              const isGpkg = insp?.detected_format === "gpkg"
              const isPdf = insp?.detected_format === "pdf"
              const isOverlay = insp?.result_layer_type === "image_overlay"

              return (
                <div
                  key={item.localId}
                  className="rounded-lg border border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1c1c1c] p-3.5 space-y-2.5"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <FileText className="w-4 h-4 text-neutral-400 shrink-0" />
                        <span className="text-sm font-medium truncate">{item.name}</span>
                        <span className="text-[11px] text-neutral-500">{formatBytes(item.size)}</span>
                        <span
                          className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium border ${
                            STATUS_BADGE[item.status] || STATUS_BADGE.uploaded
                          }`}
                        >
                          {(item.status === "inspecting" || item.status === "validating" || item.status === "processing") && (
                            <Loader2 className="w-3 h-3 animate-spin" />
                          )}
                          {item.status === "ready" && <CheckCircle2 className="w-3 h-3" />}
                          {item.status === "failed" && <AlertTriangle className="w-3 h-3" />}
                          {STATUS_LABEL[item.status] || item.status}
                        </span>
                        {isOverlay && (
                          <span className="px-2 py-0.5 rounded-full text-[11px] font-medium bg-purple-500/10 text-purple-600 dark:text-purple-400 border border-purple-500/25">
                            Image Overlay
                          </span>
                        )}
                        {insp?.requires_georeferencing && (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/25">
                            <Compass className="w-3 h-3" />
                            Requires Georeferencing
                          </span>
                        )}
                      </div>

                      {/* Inspection Metadata Summary */}
                      {insp && (
                        <div className="flex flex-wrap gap-x-4 gap-y-1 mt-1.5 text-[11px] text-neutral-500 dark:text-neutral-400">
                          <span>Format: {String(insp.detected_format || "").toUpperCase()}</span>
                          {insp.width > 0 && insp.height > 0 && (
                            <span>Dimensions: {insp.width} × {insp.height}px</span>
                          )}
                          {insp.page_count > 0 && <span>Pages: {insp.page_count}</span>}
                          {insp.feature_count > 0 && <span>Features: {insp.feature_count}</span>}
                          {insp.entity_count > 0 && <span>CAD Entities: {insp.entity_count}</span>}
                          {insp.crs && <span>CRS: {insp.crs}</span>}
                        </div>
                      )}

                      {item.error && (
                        <p className="mt-1.5 text-xs text-red-600 dark:text-red-400">{item.error}</p>
                      )}
                    </div>

                    {/* Row Actions */}
                    <div className="flex items-center gap-1.5 shrink-0">
                      {item.status === "uploaded" && (
                        <button
                          type="button"
                          onClick={() => handleConfirmImport(item)}
                          disabled={
                            (isGpkg && item.selectedLayers.size === 0) ||
                            (isPdf && item.selectedPages.size === 0)
                          }
                          className="inline-flex items-center gap-1 h-7 px-2.5 rounded-[6px] text-xs font-medium bg-[#3ecf8e] text-black hover:bg-[#34b27b] disabled:opacity-40 cursor-pointer"
                        >
                          Confirm Import
                        </button>
                      )}
                      {item.status === "processing" && (
                        <button
                          type="button"
                          onClick={() => handleCancelJob(item)}
                          title="Cancel import"
                          className="inline-flex items-center gap-1 h-7 px-2 rounded-[6px] text-xs border border-neutral-200 dark:border-[#333] hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] cursor-pointer"
                        >
                          <XCircle className="w-3.5 h-3.5" />
                          Cancel
                        </button>
                      )}
                      {(item.status === "failed" || item.status === "cancelled") && (
                        <button
                          type="button"
                          onClick={() => (item.uploadId ? handleConfirmImport(item) : processUpload(item))}
                          title="Retry"
                          className="inline-flex items-center gap-1 h-7 px-2 rounded-[6px] text-xs border border-neutral-200 dark:border-[#333] hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] cursor-pointer"
                        >
                          <RotateCcw className="w-3.5 h-3.5" />
                          Retry
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() => handleRemoveItem(item.localId)}
                        title="Remove from queue"
                        className="p-1.5 rounded-[6px] text-neutral-400 hover:text-red-500 hover:bg-red-500/10 cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  {/* Preview Selection: GeoPackage Feature Layers */}
                  {item.status === "uploaded" && isGpkg && insp?.sub_layers?.length > 0 && (
                    <div className="pt-2 border-t border-neutral-100 dark:border-[#2a2a2a] space-y-1.5">
                      <p className="text-[11px] font-medium text-neutral-600 dark:text-neutral-300">
                        Select GeoPackage Layers to Import ({item.selectedLayers.size}/{insp.sub_layers.length}):
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {insp.sub_layers.map((sl) => (
                          <label
                            key={sl.name}
                            className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs border border-neutral-200 dark:border-[#333] bg-neutral-50 dark:bg-[#141414] cursor-pointer"
                          >
                            <input
                              type="checkbox"
                              checked={item.selectedLayers.has(sl.name)}
                              onChange={() => toggleSubSelection(item.localId, "selectedLayers", sl.name)}
                            />
                            <span>{sl.name}</span>
                            <span className="text-[10px] text-neutral-500">
                              ({sl.geometry_type}, {sl.feature_count})
                            </span>
                          </label>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Preview Selection: PDF Pages */}
                  {item.status === "uploaded" && isPdf && insp?.sub_layers?.length > 0 && (
                    <div className="pt-2 border-t border-neutral-100 dark:border-[#2a2a2a] space-y-1.5">
                      <p className="text-[11px] font-medium text-neutral-600 dark:text-neutral-300">
                        Select PDF Pages to Render as Raster Layers ({item.selectedPages.size}/{insp.sub_layers.length}):
                      </p>
                      <div className="flex flex-wrap gap-2">
                        {insp.sub_layers.map((pg) => (
                          <label
                            key={pg.page_number}
                            className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs border border-neutral-200 dark:border-[#333] bg-neutral-50 dark:bg-[#141414] cursor-pointer"
                          >
                            <input
                              type="checkbox"
                              checked={item.selectedPages.has(pg.page_number)}
                              onChange={() => toggleSubSelection(item.localId, "selectedPages", pg.page_number)}
                            />
                            <span>Page {pg.page_number}</span>
                          </label>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Imported Layers Confirmation */}
                  {item.status === "ready" && item.importedLayers.length > 0 && (
                    <div className="pt-2 border-t border-neutral-100 dark:border-[#2a2a2a] text-xs text-[#24b47e] dark:text-[#3ecf8e]">
                      Registered {item.importedLayers.length} layer(s):{" "}
                      {item.importedLayers.map((l) => `${l.name} [${l.layer_type}]`).join(", ")}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* Footer Action */}
      <div className="flex items-center justify-end gap-2 pt-2">
        <button
          type="button"
          onClick={handleFinish}
          className="inline-flex items-center gap-1.5 h-9 px-4 rounded-[6px] text-xs font-medium bg-[#3ecf8e] text-black hover:bg-[#34b27b] cursor-pointer"
        >
          <span>{primaryActionLabel}</span>
          <ArrowRight className="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  )
}
