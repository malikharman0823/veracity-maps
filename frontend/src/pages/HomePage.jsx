import React, { useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router-dom"
import {
  listDatasets,
  deleteDataset,
  readableApiError,
  importProjectFolder,
  importDatasetZip,
  uploadDataset,
  listSavedConfigs,
  deleteSavedConfig,
  ensureActiveSession,
} from "../api"
import UploadBox, { ACCEPTED_FORMATS } from "../components/UploadBox"
import UploadStatus from "../components/UploadStatus"
import DatasetCard from "../components/DatasetCard"
import TopHeader from "../components/TopHeader"
import ImportPdfModal from "../components/ImportPdfModal"
import FilePreviewDialog from "../components/FilePreviewDialog"
import ReopenProjectModal from "../components/ReopenProjectModal"
import { Button } from "@/components/ui/button"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import {
  FolderOpen,
  Archive,
  Loader2,
  Sliders,
  Globe2,
  FileCheck2,
  MapPinned,
  Database,
  X,
  AlertCircle,
  Layers,
  Bookmark,
  Trash2,
  ArrowUpRight,
} from "lucide-react"
import { loadSessionResources } from "../lib/sessionLoading"

export default function HomePage() {
  const navigate = useNavigate()
  const replacementInputRef = useRef(null)

  // Upload state
  const [file, setFile] = useState(null)
  const [state, setState] = useState("idle")
  const [progress, setProgress] = useState(0)
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)
  const [pdfInspection, setPdfInspection] = useState(null)
  const [filePreviewOpen, setFilePreviewOpen] = useState(false)
  const [reopenModalOpen, setReopenModalOpen] = useState(false)
  const [reopenInitialFile, setReopenInitialFile] = useState(null)

  // Session dataset catalog state
  const [datasets, setDatasets] = useState([])
  const [loading, setLoading] = useState(true)
  const [savedConfigs, setSavedConfigs] = useState([])
  const [loadingConfigs, setLoadingConfigs] = useState(true)
  const [configToDelete, setConfigToDelete] = useState(null)
  const [deletingConfig, setDeletingConfig] = useState(false)
  const [importing, setImporting] = useState(false)
  const [datasetToDelete, setDatasetToDelete] = useState(null)
  const [deleting, setDeleting] = useState(false)

  const loadDatasets = () => {
    setLoading(true)
    return listDatasets()
      .then(setDatasets)
      .catch((err) => {
        if (err.code !== "ERR_CANCELED") {
          console.warn("Unable to load workspace datasets:", err)
        }
      })
      .finally(() => setLoading(false))
  }

  const loadConfigs = () => {
    setLoadingConfigs(true)
    return listSavedConfigs()
      .then(setSavedConfigs)
      .catch((err) => {
        console.warn("Unable to load saved configurations:", err)
      })
      .finally(() => setLoadingConfigs(false))
  }

  useEffect(() => {
    loadSessionResources({
      ensureSession: ensureActiveSession,
      loadResources: [loadDatasets, loadConfigs],
    }).catch((err) => {
      console.warn("Unable to initialize the session dashboard:", err)
    })
  }, [])

  async function handleDeleteConfig(cfg) {
    if (!cfg || deletingConfig) return
    setDeletingConfig(true)
    try {
      await deleteSavedConfig(null, cfg.id)
      setSavedConfigs((prev) => prev.filter((c) => c.id !== cfg.id))
      setConfigToDelete(null)
    } catch (err) {
      console.error("Failed to delete saved config:", err)
    } finally {
      setDeletingConfig(false)
    }
  }

  function selectFile(nextFile) {
    const lowerName = String(nextFile?.name || "").toLowerCase()
    if (lowerName.endsWith(".mbtiles")) {
      setReopenInitialFile(nextFile)
      setReopenModalOpen(true)
      return
    }
    setFile(nextFile)
    setState("selected")
    setProgress(0)
    setResult(null)
    setError(null)
    setFilePreviewOpen(true)
  }

  function removeFile() {
    setFile(null)
    setState("idle")
    setProgress(0)
    setFilePreviewOpen(false)
  }

  async function upload() {
    if (!file) return
    setState("uploading")
    setProgress(0)
    setError(null)
    try {
      const isZip = (file.name || "").toLowerCase().endsWith(".zip")
      const uploadFn = isZip ? importDatasetZip : uploadDataset
      const uploaded = await uploadFn(file, (event) => {
        if (event.total) setProgress(Math.round((event.loaded / event.total) * 100))
      })

      if (uploaded && (uploaded.type === "pdf_pending" || uploaded.status === "pdf_pending")) {
        setPdfInspection(uploaded)
        setFile(null)
        setState("idle")
        return
      }

      setResult(uploaded)
      setFile(null)
      loadDatasets()

      if (uploaded.is_georeferenced) {
        navigate(`/datasets/${uploaded.id}/workspace?view=map`)
        return
      }
      setState("success")
    } catch (requestError) {
      setError(readableApiError(requestError, "Unable to upload the file."))
      setState("error")
    }
  }

  function continueAfterUpload() {
    if (!result) return
    if (result.is_georeferenced) {
      navigate(`/datasets/${result.id}/workspace?view=map`)
    } else {
      navigate(`/datasets/${result.id}/workspace?view=database`)
    }
  }

  const handleImportFolder = async (event) => {
    const files = Array.from(event.target.files)
    if (!files.length) return
    setImporting(true)
    setError(null)
    try {
      const res = await importProjectFolder(files)
      navigate(`/datasets/${res.dataset_id}`)
    } catch (err) {
      setError(readableApiError(err, "Failed to import project folder."))
    } finally {
      setImporting(false)
      event.target.value = ""
    }
  }

  const handleDeleteConfirm = async () => {
    if (!datasetToDelete) return
    setDeleting(true)
    try {
      await deleteDataset(datasetToDelete.id)
      setDatasets((prev) => prev.filter((d) => d.id !== datasetToDelete.id))
      setDatasetToDelete(null)
    } catch (err) {
      setError(readableApiError(err, "Failed to delete dataset."))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="flex flex-col min-h-screen bg-slate-950 text-slate-100">
      <TopHeader breadcrumbs={[{ label: "Upload Dataset" }]} />

      <main className="flex-1 pb-16">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 md:px-8 py-8 flex flex-col gap-8">
          {/* Header Section */}
          <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 pb-6 border-b border-slate-800">
            <div>
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wider text-slate-400 mb-1">
                <span>Spatial Workspace</span>
                <span>•</span>
                <span className="text-blue-400">New Dataset</span>
              </div>
              <h1 className="text-3xl font-semibold tracking-tight text-white">
                Upload Dataset
              </h1>
              <p className="text-sm text-slate-400 mt-1.5 max-w-xl">
                Upload GeoTIFF/TIFF, PNG, JPG, PDF, CAD (.dxf, .dwg, .svf), standalone MBTiles (.mbtiles), or Veracity Maps project packages (.zip).
              </p>
            </div>

            <div className="flex items-center gap-2.5 shrink-0 flex-wrap sm:flex-nowrap">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  setReopenInitialFile(null)
                  setReopenModalOpen(true)
                }}
                disabled={importing}
                className="h-9 rounded-full border-blue-500/30 bg-blue-950/40 hover:bg-blue-900/50 text-xs text-blue-400 cursor-pointer font-medium"
              >
                <Archive className="size-3.5 mr-1.5 text-blue-400" />
                <span>Reopen Project (.mbtiles / .zip)</span>
              </Button>

              <input
                type="file"
                multiple
                webkitdirectory=""
                directory=""
                onChange={handleImportFolder}
                className="hidden"
                id="import-folder-input"
                disabled={importing}
              />
              <Button
                asChild
                variant="outline"
                size="sm"
                className="h-9 rounded-full border-slate-700 bg-slate-900 hover:bg-slate-800 text-xs text-slate-200 hover:text-white cursor-pointer"
              >
                <label htmlFor="import-folder-input">
                  {importing ? (
                    <Loader2 className="size-3.5 mr-1.5 animate-spin text-blue-400" />
                  ) : (
                    <FolderOpen className="size-3.5 mr-1.5 text-slate-400" />
                  )}
                  <span>{importing ? "Importing…" : "Open Project Folder"}</span>
                </label>
              </Button>

              <div className="flex items-center gap-2 bg-slate-900 border border-slate-700 px-3.5 py-1.5 rounded-full shadow-xs">
                <span className="size-2 rounded-full bg-emerald-400 animate-pulse"></span>
                <span className="text-xs font-medium text-slate-200">Ready</span>
              </div>
            </div>
          </div>

          {/* Global Error Alert */}
          {error && (
            <div className="rounded-xl border border-red-500/30 bg-red-950/30 p-4 flex items-center gap-3">
              <AlertCircle className="size-5 text-red-400 shrink-0" />
              <div className="text-xs">
                <p className="font-semibold text-red-400">{error.message || "An error occurred"}</p>
                {error.details && <p className="text-red-400/80 mt-0.5">{error.details}</p>}
              </div>
            </div>
          )}

          {/* Main Upload Area */}
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
            {/* Left Column: Dropzone & Upload Box (7 Cols) */}
            <div className="lg:col-span-7 flex flex-col gap-6">
              {state === "idle" ? (
                <UploadBox disabled={false} onSelect={selectFile} />
              ) : (
                <UploadStatus
                  state={state}
                  file={file}
                  progress={progress}
                  result={result}
                  error={error}
                  onRemove={removeFile}
                  onReplace={() => replacementInputRef.current?.click()}
                  onUpload={upload}
                  onRetry={() => setState(file ? "selected" : "idle")}
                  onOpen={continueAfterUpload}
                />
              )}
            </div>

            {/* Right Column: Ingestion Parameters (5 Cols) */}
            <div className="lg:col-span-5 flex flex-col gap-4">
              <div className="border border-slate-800 rounded-2xl bg-slate-900/60 p-6 space-y-5 shadow-xs">
                <div className="flex items-center justify-between pb-3 border-b border-slate-800">
                  <div className="flex items-center gap-2">
                    <Sliders className="size-4 text-blue-400" />
                    <h3 className="text-sm font-semibold text-white">
                      Ingestion Parameters
                    </h3>
                  </div>
                  <span className="text-[10px] font-mono text-slate-400 px-2 py-0.5 rounded-full bg-slate-800 border border-slate-700">
                    Auto
                  </span>
                </div>

                <div className="space-y-2">
                  <label className="text-xs font-medium text-slate-200 flex items-center justify-between">
                    <span className="flex items-center gap-1.5">
                      <Globe2 className="size-3.5 text-slate-400" />
                      Georeference detection
                    </span>
                    <span className="text-[10px] text-blue-400 font-mono font-semibold">
                      AUTOMATIC
                    </span>
                  </label>
                  <div className="w-full bg-slate-950 border border-slate-800 rounded-xl py-2 px-3 text-xs text-white font-mono flex items-center justify-between">
                    <span>Raster metadata + sidecars</span>
                    <FileCheck2 className="size-3.5 text-emerald-400" />
                  </div>
                  <p className="text-[11px] text-slate-400">
                    Georeferenced rasters open directly on the map. Raw rasters allow point-based georeferencing.
                  </p>
                </div>

                <div className="pt-2 border-t border-slate-800 space-y-3">
                  <div className="flex items-start justify-between gap-3 p-3 rounded-xl bg-slate-950/50 border border-slate-800">
                    <div className="space-y-0.5">
                      <div className="text-xs font-semibold text-white">
                        Auto-Tiling Pipeline
                      </div>
                      <p className="text-[11px] text-slate-400">
                        Rasters are automatically processed and converted into Slippy tiles.
                      </p>
                    </div>
                    <span className="size-2 rounded-full bg-emerald-400 mt-1 shrink-0"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Saved Map Configurations in this Session (if any exist) */}
          {savedConfigs.length > 0 && (
            <div className="pt-8 border-t border-slate-800 space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Bookmark className="size-4 text-blue-400" />
                  <h2 className="text-sm font-semibold tracking-tight text-white">
                    Saved Configurations ({savedConfigs.length})
                  </h2>
                </div>
                <span className="text-[11px] text-slate-400">
                  Reopen your saved map layouts, camera views, and floor layers
                </span>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                {savedConfigs.map((cfg) => (
                  <div
                    key={cfg.id}
                    className="group relative bg-slate-900/60 border border-slate-800 hover:border-blue-500/50 rounded-2xl p-4 shadow-xs hover:shadow-md transition-all flex flex-col justify-between"
                  >
                    <div>
                      <div className="flex items-start justify-between gap-2 mb-2">
                        <div className="flex items-center gap-2">
                          <div className="size-7 rounded-lg bg-blue-950/60 text-blue-400 flex items-center justify-center shrink-0">
                            <Bookmark className="size-3.5" />
                          </div>
                          <div>
                            <h3 className="text-xs font-semibold text-white line-clamp-1" title={cfg.name}>
                              {cfg.name}
                            </h3>
                            <span className="text-[10px] text-slate-400">
                              {cfg.members?.length || 1} layer(s) attached
                            </span>
                          </div>
                        </div>
                        <button
                          type="button"
                          onClick={() => setConfigToDelete(cfg)}
                          className="size-7 rounded-lg text-slate-400 hover:text-red-400 hover:bg-red-500/10 flex items-center justify-center transition-colors opacity-0 group-hover:opacity-100"
                          title="Delete saved configuration"
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      </div>

                      <div className="mt-2 py-2 px-2.5 rounded-lg bg-slate-950/60 border border-slate-800 text-[11px] text-slate-400 space-y-1">
                        <div className="flex justify-between items-center">
                          <span>Primary:</span>
                          <span className="font-mono text-slate-200 text-[10px]">{cfg.primary_dataset_id?.slice(0, 14)}…</span>
                        </div>
                        {cfg.configuration?.active_view && (
                          <div className="flex justify-between items-center">
                            <span>Default View:</span>
                            <span className="uppercase text-[9px] font-semibold tracking-wider text-blue-400">{cfg.configuration.active_view}</span>
                          </div>
                        )}
                      </div>
                    </div>

                    <div className="mt-4 pt-3 border-t border-slate-800 flex items-center justify-between">
                      <span className="text-[10px] text-slate-400">
                        {cfg.created_at ? new Date(cfg.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : "Recent"}
                      </span>
                      <button
                        type="button"
                        onClick={() => navigate(`/datasets/${cfg.primary_dataset_id}/workspace?view=map`)}
                        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-semibold shadow-xs transition-colors cursor-pointer"
                      >
                        <span>Open Map</span>
                        <ArrowUpRight className="size-3" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Active Datasets in this Session (if any exist) */}
          {datasets.length > 0 && (
            <div className="pt-8 border-t border-slate-800 space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Layers className="size-4 text-blue-400" />
                  <h2 className="text-sm font-semibold tracking-tight text-white">
                    Active Workspace Datasets ({datasets.length})
                  </h2>
                </div>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                {datasets.map((dataset) => (
                  <DatasetCard
                    key={dataset.id}
                    dataset={dataset}
                    viewMode="grid"
                    onDeleteClick={setDatasetToDelete}
                  />
                ))}
              </div>
            </div>
          )}
        </div>

        <input
          ref={replacementInputRef}
          type="file"
          accept={ACCEPTED_FORMATS}
          className="sr-only"
          onChange={(event) => {
            const [nextFile] = event.target.files
            if (nextFile) selectFile(nextFile)
          }}
        />
      </main>

      {/* Un-georeferenced Dialog */}
      {state === "success" && result && !result.is_georeferenced && (
        <div className="workspace-dialog-backdrop" role="presentation">
          <section
            className="workspace-dialog bg-slate-900 border border-slate-800 text-white"
            role="dialog"
            aria-modal="true"
            aria-labelledby="upload-route-title"
          >
            <button
              type="button"
              className="workspace-dialog-close text-slate-400 hover:text-white"
              onClick={() => setState("idle")}
              aria-label="Close"
            >
              <X />
            </button>
            <div className="workspace-dialog-icon bg-blue-950/60 text-blue-400">
              <MapPinned />
            </div>
            <h2 id="upload-route-title" className="text-white">This raster is not georeferenced</h2>
            <p className="text-slate-400">The dataset is safely stored. Choose where you want to continue.</p>
            <div className="workspace-dialog-actions workspace-dialog-actions-stacked">
              <button
                type="button"
                className="workspace-primary-button rounded-[6px]"
                onClick={() =>
                  navigate(
                    `/datasets/${result.id}/georeference?returnTo=${encodeURIComponent(
                      `/datasets/${result.id}/workspace?view=database`
                    )}`
                  )
                }
              >
                <MapPinned /> Open georeferencer
              </button>
              <button
                type="button"
                className="workspace-secondary-button bg-slate-800 hover:bg-slate-700 text-slate-200"
                onClick={() => navigate(`/datasets/${result.id}/workspace?view=database`)}
              >
                <Database /> Inspect database first
              </button>
            </div>
          </section>
        </div>
      )}

      {/* Delete Dataset Confirmation Dialog */}
      <AlertDialog
        open={Boolean(datasetToDelete)}
        onOpenChange={(open) => !open && setDatasetToDelete(null)}
      >
        <AlertDialogContent className="rounded-2xl bg-slate-900 border border-slate-800 text-white shadow-xl max-w-md">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-base font-semibold text-white">
              Delete Dataset?
            </AlertDialogTitle>
            <AlertDialogDescription className="text-xs text-slate-400 mt-1.5">
              Are you sure you want to delete{" "}
              <strong className="text-white">
                {datasetToDelete?.name || datasetToDelete?.filename}
              </strong>
              ? This action will permanently remove all associated rasters, vector caches, and tilesets.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter className="mt-4 gap-2">
            <AlertDialogCancel
              disabled={deleting}
              className="rounded-full border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-300 hover:text-white"
            >
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeleteConfirm}
              disabled={deleting}
              className="rounded-full bg-red-600 hover:bg-red-500 text-white text-xs font-normal"
            >
              {deleting ? (
                <>
                  <Loader2 className="size-3.5 mr-1.5 animate-spin" />
                  <span>Deleting…</span>
                </>
              ) : (
                <span>Delete Permanently</span>
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Saved Configuration Confirmation Dialog */}
      <AlertDialog
        open={Boolean(configToDelete)}
        onOpenChange={(open) => !open && setConfigToDelete(null)}
      >
        <AlertDialogContent className="rounded-2xl bg-slate-900 border border-slate-800 text-white shadow-xl max-w-md">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-base font-semibold text-white">
              Delete Saved Configuration?
            </AlertDialogTitle>
            <AlertDialogDescription className="text-xs text-slate-400 mt-1.5">
              Are you sure you want to remove{" "}
              <strong className="text-white">
                {configToDelete?.name}
              </strong>
              ? The underlying datasets will remain available in your session.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter className="mt-4 gap-2">
            <AlertDialogCancel
              disabled={deletingConfig}
              className="rounded-full border-slate-700 bg-slate-800 hover:bg-slate-700 text-xs text-slate-300 hover:text-white"
            >
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => handleDeleteConfig(configToDelete)}
              disabled={deletingConfig}
              className="rounded-full bg-red-600 hover:bg-red-500 text-white text-xs font-normal"
            >
              {deletingConfig ? (
                <>
                  <Loader2 className="size-3.5 mr-1.5 animate-spin" />
                  <span>Deleting…</span>
                </>
              ) : (
                <span>Delete Configuration</span>
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {pdfInspection && (
        <ImportPdfModal
          pdfInspection={pdfInspection}
          onClose={() => setPdfInspection(null)}
          onImportSuccess={(importRes) => {
            setPdfInspection(null)
            loadDatasets()
            if (importRes.first_dataset_id) {
              navigate(`/datasets/${importRes.first_dataset_id}/workspace?view=map`)
            } else if (importRes.datasets?.length > 0) {
              navigate(`/datasets/${importRes.datasets[0].id}/workspace?view=map`)
            }
          }}
        />
      )}

      <FilePreviewDialog
        file={file}
        open={filePreviewOpen}
        onCancel={removeFile}
        onConfirm={() => setFilePreviewOpen(false)}
      />

      <ReopenProjectModal
        open={reopenModalOpen}
        initialFile={reopenInitialFile}
        onClose={() => {
          setReopenModalOpen(false)
          setReopenInitialFile(null)
        }}
        onReopened={(restored) => {
          setReopenModalOpen(false)
          setReopenInitialFile(null)
          loadDatasets()
          if (restored?.id) {
            navigate(`/datasets/${restored.id}/workspace?view=map`)
          }
        }}
      />
    </div>
  )
}
