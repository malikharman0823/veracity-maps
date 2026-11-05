import React, { useRef, useState } from "react"
import { Link, useNavigate } from "react-router-dom"
import { importDatasetZip, uploadDataset, inspectArchive, readableApiError } from "../api"
import UploadBox, { ACCEPTED_FORMATS } from "../components/UploadBox"
import UploadStatus from "../components/UploadStatus"
import TopHeader from "../components/TopHeader"
import ZipImportReview from "../components/ZipImportReview"
import ImportPdfModal from "../components/ImportPdfModal"
import FilePreviewDialog from "../components/FilePreviewDialog"
import { Button } from "@/components/ui/button"
import {
  ArrowLeft,
  Upload,
  CheckCircle,
  Sliders,
  Globe2,
  FileCheck2,
  Database,
  MapPinned,
  X,
} from "lucide-react"

export default function UploadPage() {
  const navigate = useNavigate()
  const replacementInputRef = useRef(null)
  const [file, setFile] = useState(null)
  const [state, setState] = useState("idle")
  const [progress, setProgress] = useState(0)
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)
  const [archiveInspection, setArchiveInspection] = useState(null)
  const [pdfInspection, setPdfInspection] = useState(null)
  const [filePreviewOpen, setFilePreviewOpen] = useState(false)

  function selectFile(nextFile) {
    setFile(nextFile)
    setState("selected")
    setProgress(0)
    setResult(null)
    setError(null)
    setArchiveInspection(null)
    setPdfInspection(null)
    setFilePreviewOpen(true)
  }

  function removeFile() {
    setFile(null)
    setState("idle")
    setProgress(0)
    setArchiveInspection(null)
    setPdfInspection(null)
    setFilePreviewOpen(false)
  }

  async function upload() {
    if (!file) return
    setState("uploading")
    setProgress(0)
    try {
      const isZip = (file.name || "").toLowerCase().endsWith(".zip")

      // Feature C: Staged multi-dataset ZIP inspection
      if (isZip) {
        try {
          const inspection = await inspectArchive(file, {
            onUploadProgress: (event) => {
              if (event.total) setProgress(Math.round((event.loaded / event.total) * 100))
            },
          })
          if (inspection && inspection.datasets?.length > 0) {
            setArchiveInspection(inspection)
            setState("idle")
            return
          }
        } catch (inspectErr) {
          console.warn("inspectArchive fallback to legacy zip import:", inspectErr)
        }
      }

      const uploadFn = isZip ? importDatasetZip : uploadDataset
      const uploaded = await uploadFn(file, (event) => {
        if (event.total) setProgress(Math.round((event.loaded / event.total) * 100))
      })

      // PDF multi-page import pending modal
      if (uploaded && (uploaded.type === "pdf_pending" || uploaded.status === "pdf_pending")) {
        setPdfInspection(uploaded)
        setFile(null)
        setState("idle")
        return
      }

      setResult(uploaded)
      setFile(null)
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
    }
  }

  return (
    <div className="flex flex-col min-h-screen bg-white dark:bg-[#1c1c1c] text-neutral-900 dark:text-white transition-colors">
      <TopHeader
        breadcrumbs={[
          { label: "Datasets", path: "/" },
          { label: "Upload Dataset" },
        ]}
        actions={
          <Button
            asChild
            variant="outline"
            size="sm"
            className="h-8 rounded-[6px] border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] hover:bg-neutral-100 dark:hover:bg-[#282828] text-xs text-neutral-800 dark:text-neutral-200 cursor-pointer shadow-xs"
          >
            <Link to="/">
              <ArrowLeft className="size-3.5 mr-1" />
              <span>Back to Datasets</span>
            </Link>
          </Button>
        }
      />

      <main className="flex-1 pb-16">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 md:px-8 py-8 flex flex-col gap-8">
          {/* Header Section */}
          <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 pb-6 border-b border-neutral-200 dark:border-[#2e2e2e]">
            <div>
              <Link
                to="/"
                className="inline-flex items-center gap-1.5 text-xs text-neutral-500 dark:text-neutral-400 hover:text-[#24b47e] dark:hover:text-[#3ecf8e] transition-colors mb-2.5 group"
              >
                <ArrowLeft className="size-3.5 group-hover:-translate-x-0.5 transition-transform" />
                <span>Back to datasets</span>
              </Link>
              <div className="flex items-center gap-3">
                <h1 className="text-3xl font-semibold tracking-tight text-neutral-900 dark:text-white">
                  Upload Dataset
                </h1>
                <span className="px-2.5 py-0.5 rounded-[4px] text-[11px] font-mono border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#202020] text-neutral-600 dark:text-neutral-400 font-semibold">
                  MVP
                </span>
              </div>
              <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1.5 max-w-xl">
                Upload PNG, JPG, PDF, CAD (.dxf, .dwg), SVF, or exported Map Studio project archives (.zip).
              </p>
            </div>

            <div className="flex items-center gap-2 bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] px-3.5 py-2 rounded-[6px] shrink-0 shadow-xs">
              <span className="size-2 rounded-full bg-[#3ecf8e] animate-pulse"></span>
              <span className="text-xs font-medium text-neutral-800 dark:text-neutral-200">Backend Ready</span>
            </div>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
            {/* Left Column: Dropzone & Upload Box (7 Cols) */}
            <div className="lg:col-span-7 flex flex-col gap-6">
              {state === "idle" ? (
                <UploadBox
                  disabled={false}
                  onSelect={selectFile}
                />
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
              <div className="border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] bg-white dark:bg-[#202020] p-6 space-y-5 shadow-xs">
                <div className="flex items-center justify-between pb-3 border-b border-neutral-200 dark:border-[#2e2e2e]">
                  <div className="flex items-center gap-2">
                    <Sliders className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
                      Ingestion Parameters
                    </h3>
                  </div>
                  <span className="text-[10px] font-mono text-neutral-600 dark:text-neutral-400 px-2 py-0.5 rounded-[4px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                    Auto
                  </span>
                </div>

                <div className="space-y-2">
                  <label className="text-xs font-medium text-neutral-800 dark:text-neutral-200 flex items-center justify-between">
                    <span className="flex items-center gap-1.5">
                      <Globe2 className="size-3.5 text-neutral-500 dark:text-neutral-400" />
                      Georeference detection
                    </span>
                    <span className="text-[10px] text-[#24b47e] dark:text-[#3ecf8e] font-mono font-semibold">
                      AUTOMATIC
                    </span>
                  </label>
                  <div className="w-full bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[8px] py-2 px-3 text-xs text-neutral-900 dark:text-neutral-200 font-mono flex items-center justify-between">
                    <span>Raster metadata + sidecars</span>
                    <FileCheck2 className="size-3.5 text-[#3ecf8e]" />
                  </div>
                  <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
                    Georeferenced rasters open on the map. Raw rasters let you choose georeferencing or database inspection.
                  </p>
                </div>

                <div className="pt-2 border-t border-neutral-200 dark:border-[#2e2e2e] space-y-3">
                  <div className="flex items-start justify-between gap-3 p-3 rounded-[8px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e]">
                    <div className="space-y-0.5">
                      <div className="text-xs font-semibold text-neutral-900 dark:text-white">
                        Auto-Tiling Pipeline
                      </div>
                      <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
                        Rasters are automatically processed and converted into Slippy tiles.
                      </p>
                    </div>
                    <span className="size-2 rounded-full bg-[#3ecf8e] mt-1 shrink-0"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>
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

      {state === "success" && result && !result.is_georeferenced && (
        <div className="workspace-dialog-backdrop" role="presentation">
          <section className="workspace-dialog" role="dialog" aria-modal="true" aria-labelledby="upload-route-title">
            <button type="button" className="workspace-dialog-close" onClick={() => setState("idle")} aria-label="Close"><X /></button>
            <div className="workspace-dialog-icon"><MapPinned /></div>
            <h2 id="upload-route-title">This raster is not georeferenced</h2>
            <p>The dataset is safely stored. Choose where you want to continue.</p>
            <div className="workspace-dialog-actions workspace-dialog-actions-stacked">
              <button type="button" className="workspace-primary-button" onClick={() => navigate(`/datasets/${result.id}/georeference?returnTo=${encodeURIComponent(`/datasets/${result.id}/workspace?view=database`)}`)}>
                <MapPinned /> Open georeferencer
              </button>
              <button type="button" className="workspace-secondary-button" onClick={() => navigate(`/datasets/${result.id}/workspace?view=database`)}>
                <Database /> Inspect database first
              </button>
            </div>
          </section>
        </div>
      )}

      {archiveInspection && (
        <ZipImportReview
          inspectionResult={archiveInspection}
          onClose={() => setArchiveInspection(null)}
          onImportComplete={(importRes) => {
            setArchiveInspection(null)
            const firstSuccess = importRes.results?.find(
              (r) => (r.status === "imported" || r.status === "success") && r.dataset_id
            )
            if (firstSuccess) {
              navigate(`/datasets/${firstSuccess.dataset_id}/workspace?view=map`)
            } else {
              navigate("/")
            }
          }}
        />
      )}

      {pdfInspection && (
        <ImportPdfModal
          pdfInspection={pdfInspection}
          onClose={() => setPdfInspection(null)}
          onImportSuccess={(importRes) => {
            setPdfInspection(null)
            if (importRes.first_dataset_id) {
              navigate(`/datasets/${importRes.first_dataset_id}/workspace?view=map`)
            } else if (importRes.datasets?.length > 0) {
              navigate(`/datasets/${importRes.datasets[0].id}/workspace?view=map`)
            } else {
              navigate("/")
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
    </div>
  )
}
