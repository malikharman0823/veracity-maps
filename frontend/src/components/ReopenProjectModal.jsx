import React, { useCallback, useEffect, useRef, useState } from "react"
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Database,
  FileArchive,
  FolderOpen,
  Layers,
  Loader2,
  MapPin,
  UploadCloud,
  X,
} from "lucide-react"
import {
  ensureActiveSession,
  inspectProjectPackage,
  readableApiError,
  reopenProjectPackage,
} from "../api"
import { fileSize } from "./DatasetCard"

export const REOPEN_ACCEPTED_FORMATS = ".mbtiles,.zip,application/zip,application/octet-stream,application/x-sqlite3"

export default function ReopenProjectModal({
  open,
  initialFile = null,
  onClose,
  onReopened,
}) {
  const inputRef = useRef(null)
  const [selectedFile, setSelectedFile] = useState(null)
  const [inspection, setInspection] = useState(null)
  const [inspecting, setInspecting] = useState(false)
  const [reopening, setReopening] = useState(false)
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState(null)
  const [dragging, setDragging] = useState(false)

  const handleSelectFile = useCallback(async (file) => {
    if (!file) return
    setSelectedFile(file)
    setInspection(null)
    setError(null)
    setProgress(0)
    setInspecting(true)

    try {
      await ensureActiveSession()
      const preview = await inspectProjectPackage(file, (evt) => {
        if (evt.total) {
          setProgress(Math.round((evt.loaded / evt.total) * 100))
        }
      })
      setInspection(preview)
    } catch (err) {
      setError(
        readableApiError(
          err,
          "Project package failed validation. Ensure it is a valid SQLite .mbtiles file or an MBTiles-backed Veracity Maps .zip project."
        )
      )
    } finally {
      setInspecting(false)
    }
  }, [])

  useEffect(() => {
    if (!open) {
      setSelectedFile(null)
      setInspection(null)
      setInspecting(false)
      setReopening(false)
      setError(null)
      setProgress(0)
      return
    }
    if (initialFile) {
      handleSelectFile(initialFile)
    }
  }, [open, initialFile, handleSelectFile])

  if (!open) return null

  async function handleConfirmReopen() {
    if (!selectedFile || !inspection?.valid) return
    setReopening(true)
    setError(null)
    setProgress(0)
    try {
      await ensureActiveSession()
      const res = await reopenProjectPackage(selectedFile, (evt) => {
        if (evt.total) {
          setProgress(Math.round((evt.loaded / evt.total) * 100))
        }
      })
      const dataset = res?.data || res
      if (!dataset?.id) {
        throw new Error("Server did not return a valid restored dataset ID.")
      }
      onReopened?.(dataset)
    } catch (err) {
      setError(readableApiError(err, "Failed to reopen project package."))
      setReopening(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 bg-black/55 backdrop-blur-xs flex items-center justify-center p-4 sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="reopen-project-title"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !inspecting && !reopening) onClose()
      }}
    >
      <div className="bg-white text-[#1d1d1f] w-full max-w-3xl max-h-[90vh] rounded-2xl shadow-2xl border border-[#e0e0e0] flex flex-col overflow-hidden">
        {/* Header */}
        <header className="px-6 py-4 border-b border-[#e0e0e0] bg-[#fafafc] flex items-center justify-between gap-4">
          <div className="flex items-center gap-3 min-w-0">
            <div className="size-10 rounded-xl bg-[#0066cc]/10 border border-[#0066cc]/20 flex items-center justify-center text-[#0066cc] shrink-0">
              <FolderOpen className="size-5" />
            </div>
            <div className="min-w-0">
              <h2 id="reopen-project-title" className="text-base font-semibold text-[#1d1d1f] truncate">
                Reopen Exported Project (.mbtiles or .zip)
              </h2>
              <p className="text-xs text-[#7a7a7a] truncate">
                Validates SQLite MBTiles 1.3 tiles, geographic bounds, multi-floor layers, control points, and landmarks
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            disabled={inspecting || reopening}
            className="size-8 rounded-full flex items-center justify-center text-[#7a7a7a] hover:text-[#1d1d1f] hover:bg-[#e8e8ed] disabled:opacity-50"
            aria-label="Close reopen project dialog"
          >
            <X className="size-4" />
          </button>
        </header>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-5">
          {/* Dropzone / Picker */}
          <div
            onClick={() => !inspecting && !reopening && inputRef.current?.click()}
            onDragEnter={(e) => {
              e.preventDefault()
              if (!inspecting && !reopening) setDragging(true)
            }}
            onDragOver={(e) => e.preventDefault()}
            onDragLeave={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget)) setDragging(false)
            }}
            onDrop={(e) => {
              e.preventDefault()
              setDragging(false)
              if (!inspecting && !reopening && e.dataTransfer.files?.[0]) {
                handleSelectFile(e.dataTransfer.files[0])
              }
            }}
            className={`rounded-xl border-2 border-dashed p-6 text-center cursor-pointer transition-colors ${
              dragging
                ? "border-[#0066cc] bg-[#f0f6ff]"
                : "border-[#d8d8de] bg-[#fafafc] hover:border-[#0066cc]/60"
            } ${inspecting || reopening ? "opacity-60 cursor-not-allowed" : ""}`}
          >
            <UploadCloud className="size-7 text-[#0066cc] mx-auto mb-2" />
            <p className="text-xs font-semibold text-[#1d1d1f]">
              {selectedFile
                ? `${selectedFile.name} (${fileSize(selectedFile.size)})`
                : "Drop a standalone .mbtiles database or Veracity Maps .zip project package"}
            </p>
            <p className="text-[11px] text-[#7a7a7a] mt-1">
              Click to browse another file • Deprecated loose-tile archives without MBTiles are rejected automatically
            </p>
            <input
              ref={inputRef}
              type="file"
              accept={REOPEN_ACCEPTED_FORMATS}
              className="sr-only"
              disabled={inspecting || reopening}
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) handleSelectFile(file)
                e.target.value = ""
              }}
            />
          </div>

          {/* Inspecting state */}
          {inspecting && (
            <div className="p-5 rounded-xl border border-[#e0e0e0] bg-[#fafafc] flex items-center gap-3 text-xs text-[#1d1d1f]">
              <Loader2 className="size-5 animate-spin text-[#0066cc] shrink-0" />
              <div className="flex-1">
                <p className="font-semibold">Validating package structure & SQLite MBTiles schema...</p>
                <p className="text-[11px] text-[#7a7a7a] mt-0.5">
                  Checking archive paths, SQLite magic header, tile count, geographic bounds, and layer manifest ({progress}%)
                </p>
              </div>
            </div>
          )}

          {/* Validation / Reopen Error */}
          {error && (
            <div className="p-4 rounded-xl bg-red-50 border border-red-200 text-red-800 text-xs flex items-start gap-2.5">
              <AlertCircle className="size-4 shrink-0 text-red-600 mt-0.5" />
              <div>
                <p className="font-semibold">{error.message || "Package validation failed"}</p>
                {error.details && <p className="mt-1 text-red-700 font-mono text-[11px]">{error.details}</p>}
              </div>
            </div>
          )}

          {/* Validated Package Preview */}
          {inspection && inspection.valid && !inspecting && (
            <div className="space-y-4">
              <div className="p-4 rounded-xl border border-emerald-200 bg-emerald-50/60 flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2.5">
                  <CheckCircle2 className="size-5 text-emerald-600 shrink-0" />
                  <div>
                    <p className="text-sm font-semibold text-[#1d1d1f]">
                      {inspection.project_name || selectedFile?.name}
                    </p>
                    <p className="text-[11px] text-[#555558]">
                      Verified {inspection.package_type === "mbtiles" ? "Standalone SQLite MBTiles 1.3" : `Veracity Project Package (Schema v${inspection.schema_version || 2})`}
                    </p>
                  </div>
                </div>

                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-white border border-emerald-300 text-emerald-800">
                  {inspection.package_type === "mbtiles" ? (
                    <>
                      <Database className="size-3.5" />
                      <span>Standalone .mbtiles</span>
                    </>
                  ) : (
                    <>
                      <FileArchive className="size-3.5" />
                      <span>MBTiles Project .zip</span>
                    </>
                  )}
                </span>
              </div>

              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 text-xs">
                <div className="rounded-lg border border-[#e0e0e0] bg-[#fafafc] px-3 py-2">
                  <span className="text-[11px] text-[#7a7a7a] block">Tiles & Format</span>
                  <span className="font-mono font-semibold text-[#1d1d1f]">
                    {inspection.tile_count} tiles ({(inspection.tile_format || "png").toUpperCase()})
                  </span>
                </div>
                <div className="rounded-lg border border-[#e0e0e0] bg-[#fafafc] px-3 py-2">
                  <span className="text-[11px] text-[#7a7a7a] block">Zoom Configuration</span>
                  <span className="font-mono font-semibold text-[#1d1d1f]">
                    z{inspection.min_zoom}–z{inspection.max_zoom} (init z{Math.round(inspection.initial_zoom || inspection.min_zoom)})
                  </span>
                </div>
                <div className="rounded-lg border border-[#e0e0e0] bg-[#fafafc] px-3 py-2">
                  <span className="text-[11px] text-[#7a7a7a] block">Layers & Floors</span>
                  <span className="font-mono font-semibold text-[#1d1d1f]">
                    {inspection.layer_count} {inspection.layer_count === 1 ? "layer" : "layers"}
                    {inspection.building_count > 0 ? ` • ${inspection.building_count} bldg` : ""}
                  </span>
                </div>
                <div className="rounded-lg border border-[#e0e0e0] bg-[#fafafc] px-3 py-2">
                  <span className="text-[11px] text-[#7a7a7a] block">GCPs / Landmarks / Labels</span>
                  <span className="font-mono font-semibold text-[#1d1d1f]">
                    {inspection.control_point_count || 0} GCP • {inspection.landmark_count || 0} POI • {inspection.annotation_count || 0} lbl
                  </span>
                </div>
              </div>

              {Array.isArray(inspection.bounds) && inspection.bounds.length === 4 && (
                <div className="rounded-lg border border-[#e0e0e0] bg-white px-3 py-2 text-xs flex items-center gap-2">
                  <MapPin className="size-3.5 text-[#0066cc] shrink-0" />
                  <span className="text-[#7a7a7a]">WGS84 Extent:</span>
                  <span className="font-mono text-[#1d1d1f] truncate">
                    [{inspection.bounds.map((n) => Number(n).toFixed(5)).join(", ")}]
                  </span>
                </div>
              )}

              {Array.isArray(inspection.warnings) && inspection.warnings.length > 0 && (
                <div className="p-3 rounded-xl bg-amber-50 border border-amber-200 text-amber-900 text-xs space-y-1">
                  {inspection.warnings.map((w, idx) => (
                    <div key={idx} className="flex items-start gap-2">
                      <AlertTriangle className="size-3.5 shrink-0 text-amber-600 mt-0.5" />
                      <span>{w}</span>
                    </div>
                  ))}
                </div>
              )}

              {Array.isArray(inspection.layers) && inspection.layers.length > 0 && (
                <div className="rounded-xl border border-[#e0e0e0] overflow-hidden">
                  <div className="px-3.5 py-2 bg-[#fafafc] border-b border-[#e0e0e0] text-[11px] font-semibold text-[#555558] flex items-center gap-1.5">
                    <Layers className="size-3.5 text-[#0066cc]" />
                    <span>Layers to Restore ({inspection.layers.length})</span>
                  </div>
                  <div className="divide-y divide-[#f0f0f2] max-h-44 overflow-y-auto">
                    {inspection.layers.map((lyr) => (
                      <div key={lyr.id} className="px-3.5 py-2 text-xs flex items-center justify-between gap-2 bg-white">
                        <div className="min-w-0">
                          <p className="font-medium text-[#1d1d1f] truncate">{lyr.name}</p>
                          <p className="text-[11px] text-[#7a7a7a]">
                            {lyr.building_name ? `${lyr.building_name} • ` : ""}
                            {lyr.floor_label || `Floor ${lyr.floor_level}`} • Opacity {Math.round((lyr.opacity ?? 1) * 100)}% • {lyr.visible ? "Visible" : "Hidden"}
                          </p>
                        </div>
                        <div className="flex items-center gap-1.5 shrink-0 text-[10px] font-mono">
                          {lyr.has_mbtiles && (
                            <span className="px-1.5 py-0.5 rounded bg-blue-50 text-blue-700 border border-blue-200">
                              MBTiles
                            </span>
                          )}
                          {lyr.has_vectors && (
                            <span className="px-1.5 py-0.5 rounded bg-emerald-50 text-emerald-700 border border-emerald-200">
                              Vectors
                            </span>
                          )}
                          {lyr.control_points > 0 && (
                            <span className="px-1.5 py-0.5 rounded bg-slate-100 text-slate-700">
                              {lyr.control_points} GCP
                            </span>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <footer className="px-6 py-4 border-t border-[#e0e0e0] bg-[#fafafc] flex items-center justify-between gap-3">
          <button
            type="button"
            onClick={onClose}
            disabled={inspecting || reopening}
            className="rounded-full border border-[#e0e0e0] bg-white px-4 py-2 text-xs font-medium text-[#1d1d1f] hover:bg-[#f5f5f7] disabled:opacity-50"
          >
            Cancel
          </button>

          <button
            type="button"
            onClick={handleConfirmReopen}
            disabled={!inspection?.valid || inspecting || reopening}
            className="inline-flex items-center gap-1.5 rounded-full bg-[#0066cc] px-5 py-2 text-xs font-medium text-white hover:bg-[#0055b3] disabled:opacity-50 shadow-xs"
          >
            {reopening ? (
              <>
                <Loader2 className="size-3.5 animate-spin" />
                <span>Restoring Project into MapLibre ({progress}%)...</span>
              </>
            ) : (
              <>
                <FolderOpen className="size-3.5" />
                <span>Reopen in Workspace</span>
              </>
            )}
          </button>
        </footer>
      </div>
    </div>
  )
}

