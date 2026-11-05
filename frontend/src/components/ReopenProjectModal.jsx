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
      className="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex items-center justify-center p-4 sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="reopen-project-title"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !inspecting && !reopening) onClose()
      }}
    >
      <div className="bg-white dark:bg-[#202020] text-neutral-900 dark:text-white w-full max-w-3xl max-h-[90vh] rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] flex flex-col overflow-hidden transition-colors">
        {/* Header */}
        <header className="px-6 py-4 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] flex items-center justify-between gap-4">
          <div className="flex items-center gap-3 min-w-0">
            <div className="size-10 rounded-[8px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/20 flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e] shrink-0">
              <FolderOpen className="size-5" />
            </div>
            <div className="min-w-0">
              <h2 id="reopen-project-title" className="text-base font-semibold text-neutral-900 dark:text-white truncate">
                Reopen Exported Project (.mbtiles or .zip)
              </h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 truncate">
                Validates SQLite MBTiles 1.3 tiles, geographic bounds, multi-floor layers, control points, and landmarks
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            disabled={inspecting || reopening}
            className="size-8 rounded-[6px] flex items-center justify-center text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-white hover:bg-neutral-200/50 dark:hover:bg-[#282828] disabled:opacity-50 cursor-pointer"
            aria-label="Close reopen project dialog"
          >
            <X className="size-4" />
          </button>
        </header>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-5 bg-white dark:bg-[#202020]">
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
            className={`rounded-[8px] border-2 border-dashed p-6 text-center cursor-pointer transition-colors ${
              dragging
                ? "border-[#3ecf8e] bg-[#3ecf8e]/10"
                : "border-neutral-300 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] hover:border-[#3ecf8e]/60"
            } ${inspecting || reopening ? "opacity-60 cursor-not-allowed" : ""}`}
          >
            <UploadCloud className="size-7 text-[#24b47e] dark:text-[#3ecf8e] mx-auto mb-2" />
            <p className="text-xs font-semibold text-neutral-900 dark:text-white">
              {selectedFile
                ? `${selectedFile.name} (${fileSize(selectedFile.size)})`
                : "Drop a standalone .mbtiles database or Veracity Maps .zip project package"}
            </p>
            <p className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-1">
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
            <div className="p-5 rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] flex items-center gap-3 text-xs text-neutral-900 dark:text-white">
              <Loader2 className="size-5 animate-spin text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
              <div className="flex-1">
                <p className="font-semibold">Validating package structure & SQLite MBTiles schema...</p>
                <p className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">
                  Checking archive paths, SQLite magic header, tile count, geographic bounds, and layer manifest ({progress}%)
                </p>
              </div>
            </div>
          )}

          {/* Validation / Reopen Error */}
          {error && (
            <div className="p-4 rounded-[8px] bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-start gap-2.5">
              <AlertCircle className="size-4 shrink-0 text-red-500 mt-0.5" />
              <div>
                <p className="font-semibold">{error.message || "Package validation failed"}</p>
                {error.details && <p className="mt-1 text-red-500/80 font-mono text-[11px]">{error.details}</p>}
              </div>
            </div>
          )}

          {/* Validated Package Preview */}
          {inspection && inspection.valid && !inspecting && (
            <div className="space-y-4">
              <div className="p-4 rounded-[8px] border border-[#3ecf8e]/30 bg-[#3ecf8e]/10 flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2.5">
                  <CheckCircle2 className="size-5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                  <div>
                    <p className="text-sm font-semibold text-neutral-900 dark:text-white">
                      {inspection.project_name || selectedFile?.name}
                    </p>
                    <p className="text-[11px] text-neutral-600 dark:text-neutral-400">
                      Verified {inspection.package_type === "mbtiles" ? "Standalone SQLite MBTiles 1.3" : `Veracity Project Package (Schema v${inspection.schema_version || 2})`}
                    </p>
                  </div>
                </div>

                <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-[4px] text-xs font-semibold bg-white dark:bg-[#202020] border border-[#3ecf8e]/40 text-[#24b47e] dark:text-[#3ecf8e]">
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
                <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-3 py-2">
                  <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">Tiles & Format</span>
                  <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                    {inspection.tile_count} tiles ({(inspection.tile_format || "png").toUpperCase()})
                  </span>
                </div>
                <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-3 py-2">
                  <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">Zoom Configuration</span>
                  <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                    z{inspection.min_zoom}–z{inspection.max_zoom} (init z{Math.round(inspection.initial_zoom || inspection.min_zoom)})
                  </span>
                </div>
                <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-3 py-2">
                  <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">Layers & Floors</span>
                  <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                    {inspection.layer_count} {inspection.layer_count === 1 ? "layer" : "layers"}
                    {inspection.building_count > 0 ? ` • ${inspection.building_count} bldg` : ""}
                  </span>
                </div>
                <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-3 py-2">
                  <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">GCPs / Landmarks / Labels</span>
                  <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                    {inspection.control_point_count || 0} GCP • {inspection.landmark_count || 0} POI • {inspection.annotation_count || 0} lbl
                  </span>
                </div>
              </div>

              {Array.isArray(inspection.bounds) && inspection.bounds.length === 4 && (
                <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] px-3 py-2 text-xs flex items-center gap-2">
                  <MapPin className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
                  <span className="text-neutral-500 dark:text-neutral-400">WGS84 Extent:</span>
                  <span className="font-mono text-neutral-900 dark:text-white truncate">
                    [{inspection.bounds.map((n) => Number(n).toFixed(5)).join(", ")}]
                  </span>
                </div>
              )}

              {Array.isArray(inspection.warnings) && inspection.warnings.length > 0 && (
                <div className="p-3 rounded-[8px] bg-amber-500/10 border border-amber-500/20 text-amber-600 dark:text-amber-400 text-xs space-y-1">
                  {inspection.warnings.map((w, idx) => (
                    <div key={idx} className="flex items-start gap-2">
                      <AlertTriangle className="size-3.5 shrink-0 text-amber-500 mt-0.5" />
                      <span>{w}</span>
                    </div>
                  ))}
                </div>
              )}

              {Array.isArray(inspection.layers) && inspection.layers.length > 0 && (
                <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] overflow-hidden">
                  <div className="px-3.5 py-2 bg-neutral-50 dark:bg-[#171717] border-b border-neutral-200 dark:border-[#2e2e2e] text-[11px] font-semibold text-neutral-600 dark:text-neutral-300 flex items-center gap-1.5">
                    <Layers className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <span>Layers to Restore ({inspection.layers.length})</span>
                  </div>
                  <div className="divide-y divide-neutral-200 dark:divide-[#2e2e2e] max-h-44 overflow-y-auto">
                    {inspection.layers.map((lyr) => (
                      <div key={lyr.id} className="px-3.5 py-2 text-xs flex items-center justify-between gap-2 bg-white dark:bg-[#202020]">
                        <div className="min-w-0">
                          <p className="font-medium text-neutral-900 dark:text-white truncate">{lyr.name}</p>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
                            {lyr.building_name ? `${lyr.building_name} • ` : ""}
                            {lyr.floor_label || `Floor ${lyr.floor_level}`} • Opacity {Math.round((lyr.opacity ?? 1) * 100)}% • {lyr.visible ? "Visible" : "Hidden"}
                          </p>
                        </div>
                        <div className="flex items-center gap-1.5 shrink-0 text-[10px] font-mono">
                          {lyr.has_mbtiles && (
                            <span className="px-1.5 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/20">
                              MBTiles
                            </span>
                          )}
                          {lyr.has_vectors && (
                            <span className="px-1.5 py-0.5 rounded-[4px] bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
                              Vectors
                            </span>
                          )}
                          {lyr.control_points > 0 && (
                            <span className="px-1.5 py-0.5 rounded-[4px] bg-neutral-100 dark:bg-[#282828] text-neutral-700 dark:text-neutral-300">
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
        <footer className="px-6 py-4 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] flex items-center justify-between gap-3">
          <button
            type="button"
            onClick={onClose}
            disabled={inspecting || reopening}
            className="rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] px-4 py-2 text-xs font-medium text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828] disabled:opacity-50 cursor-pointer"
          >
            Cancel
          </button>

          <button
            type="button"
            onClick={handleConfirmReopen}
            disabled={!inspection?.valid || inspecting || reopening}
            className="inline-flex items-center gap-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-5 py-2 text-xs font-medium text-[#171717] disabled:opacity-50 shadow-xs cursor-pointer"
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
