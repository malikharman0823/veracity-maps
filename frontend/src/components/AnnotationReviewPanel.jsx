import React, { useCallback, useEffect, useMemo, useState } from "react"
import {
  AlertCircle,
  CheckCircle2,
  Eye,
  EyeOff,
  Loader2,
  Plus,
  Save,
  ScanText,
  Search,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react"
import {
  deleteDatasetAnnotation,
  listDatasetAnnotations,
  readableApiError,
  runDatasetOcr,
  saveDatasetAnnotation,
} from "../api"

const CATEGORY_LABELS = {
  dimension: "Dimension",
  elevation_level: "Elevation / Level",
  pipe_culvert: "Pipe / Culvert",
  road_label: "Road / Corridor",
  general: "General Label",
}

const SOURCE_LABELS = {
  paddleocr: "PaddleOCR",
  pdf_vector: "PDF Vector",
  cad_vector: "CAD Vector",
  manual: "Manual",
}

export default function AnnotationReviewPanel({
  open,
  datasetId,
  datasetName,
  onClose,
  onUpdated,
}) {
  const [annotations, setAnnotations] = useState([])
  const [draftTexts, setDraftTexts] = useState({})
  const [loading, setLoading] = useState(false)
  const [runningOcr, setRunningOcr] = useState(false)
  const [savingId, setSavingId] = useState("")
  const [error, setError] = useState(null)
  const [search, setSearch] = useState("")
  const [categoryFilter, setCategoryFilter] = useState("all")
  const [minConfidence, setMinConfidence] = useState(0.35)

  const loadAnnotations = useCallback(async () => {
    if (!datasetId) return
    setLoading(true)
    setError(null)
    try {
      const data = await listDatasetAnnotations(datasetId)
      const list = Array.isArray(data?.annotations) ? data.annotations : []
      setAnnotations(list)
      const drafts = {}
      list.forEach((a) => {
        drafts[a.id] = a.text || ""
      })
      setDraftTexts(drafts)
    } catch (err) {
      setError(readableApiError(err, "Failed to load label annotations."))
    } finally {
      setLoading(false)
    }
  }, [datasetId])

  useEffect(() => {
    if (open && datasetId) {
      loadAnnotations()
    }
  }, [open, datasetId, loadAnnotations])

  const filteredAnnotations = useMemo(() => {
    const q = search.trim().toLowerCase()
    return annotations.filter((ann) => {
      if (categoryFilter !== "all" && ann.category !== categoryFilter) return false
      if (!q) return true
      return (
        String(ann.text || "").toLowerCase().includes(q) ||
        String(ann.cad_layer || "").toLowerCase().includes(q) ||
        String(ann.source || "").toLowerCase().includes(q)
      )
    })
  }, [annotations, categoryFilter, search])

  if (!open || !datasetId) return null

  async function handleRunOcr() {
    setRunningOcr(true)
    setError(null)
    try {
      await runDatasetOcr(datasetId, Number(minConfidence) || 0.35)
      await loadAnnotations()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "PaddleOCR label detection failed."))
    } finally {
      setRunningOcr(false)
    }
  }

  async function handleSaveText(ann) {
    const nextText = (draftTexts[ann.id] ?? ann.text ?? "").trim()
    if (!nextText) return
    setSavingId(ann.id)
    setError(null)
    try {
      await saveDatasetAnnotation(datasetId, {
        ...ann,
        text: nextText,
      })
      await loadAnnotations()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Failed to update annotation."))
    } finally {
      setSavingId("")
    }
  }

  async function handleToggleField(ann, field) {
    setSavingId(ann.id)
    setError(null)
    try {
      await saveDatasetAnnotation(datasetId, {
        ...ann,
        [field]: !ann[field],
      })
      await loadAnnotations()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Failed to update annotation state."))
    } finally {
      setSavingId("")
    }
  }

  async function handleDelete(annId) {
    setSavingId(annId)
    setError(null)
    try {
      await deleteDatasetAnnotation(datasetId, annId)
      setAnnotations((prev) => prev.filter((item) => item.id !== annId))
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Failed to delete annotation."))
    } finally {
      setSavingId("")
    }
  }

  async function handleAddManual() {
    setSavingId("new")
    setError(null)
    try {
      await saveDatasetAnnotation(datasetId, {
        text: "NEW LABEL",
        category: "general",
        source: "manual",
        confidence: 1.0,
        image_x: 50,
        image_y: 50,
        visible: true,
        verified: true,
      })
      await loadAnnotations()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Failed to create manual label."))
    } finally {
      setSavingId("")
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex items-center justify-center p-4 sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="annotation-review-title"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !runningOcr) onClose()
      }}
    >
      <div className="bg-white dark:bg-[#202020] text-neutral-900 dark:text-white w-full max-w-4xl max-h-[90vh] rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] flex flex-col overflow-hidden transition-colors">
        {/* Header */}
        <header className="px-6 py-4 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] flex items-center justify-between gap-4">
          <div className="flex items-center gap-3 min-w-0">
            <div className="size-10 rounded-[8px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/20 flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e] shrink-0">
              <ScanText className="size-5" />
            </div>
            <div className="min-w-0">
              <h2 id="annotation-review-title" className="text-base font-semibold text-neutral-900 dark:text-white truncate">
                Vector & PaddleOCR Label Overlays
              </h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 truncate">
                {datasetName || datasetId} • {annotations.length} extracted labels (rendered as crisp MapLibre symbol layers)
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            disabled={runningOcr}
            className="size-8 rounded-[6px] flex items-center justify-center text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-white hover:bg-neutral-200/50 dark:hover:bg-[#282828] cursor-pointer"
            aria-label="Close label review panel"
          >
            <X className="size-4" />
          </button>
        </header>

        {/* Controls Bar */}
        <div className="px-6 py-3 border-b border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2 flex-1 min-w-[220px]">
            <div className="relative flex-1 min-w-[160px] max-w-xs">
              <Search className="size-3.5 text-neutral-500 dark:text-neutral-400 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Filter labels or CAD layers..."
                className="w-full pl-8 pr-3 py-1.5 text-xs rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
              />
            </div>

            <select
              value={categoryFilter}
              onChange={(e) => setCategoryFilter(e.target.value)}
              className="text-xs px-2.5 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
            >
              <option value="all">All Categories</option>
              {Object.entries(CATEGORY_LABELS).map(([val, label]) => (
                <option key={val} value={val}>
                  {label}
                </option>
              ))}
            </select>
          </div>

          <div className="flex items-center gap-2">
            <select
              value={minConfidence}
              onChange={(e) => setMinConfidence(Number(e.target.value))}
              disabled={runningOcr}
              title="Minimum OCR Confidence"
              className="text-xs px-2 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] text-neutral-900 dark:text-white"
            >
              <option value={0.35}>Conf ≥ 35%</option>
              <option value={0.5}>Conf ≥ 50%</option>
              <option value={0.7}>Conf ≥ 70%</option>
            </select>

            <button
              type="button"
              onClick={handleAddManual}
              disabled={runningOcr || savingId === "new"}
              className="inline-flex items-center gap-1 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] px-3 py-1.5 text-xs font-medium text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828]"
            >
              <Plus className="size-3.5" />
              <span>Add Label</span>
            </button>

            <button
              type="button"
              onClick={handleRunOcr}
              disabled={runningOcr}
              className="inline-flex items-center gap-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-3.5 py-1.5 text-xs font-semibold text-[#171717] disabled:opacity-50"
            >
              {runningOcr ? (
                <>
                  <Loader2 className="size-3.5 animate-spin" />
                  <span>Running PaddleOCR...</span>
                </>
              ) : (
                <>
                  <ScanText className="size-3.5" />
                  <span>Run PaddleOCR</span>
                </>
              )}
            </button>
          </div>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-3">
          <div className="p-3 rounded-[8px] bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-[11px] text-neutral-600 dark:text-neutral-400">
            <strong className="text-neutral-900 dark:text-white">Truthfulness & Source Integrity:</strong> Labels are stored separately from original raster pixels. Unreviewed OCR detections remain marked <em>Unverified</em> until confirmed by an engineer, and false positives can be hidden or deleted at any time.
          </div>

          {error && (
            <div className="p-3 rounded-[8px] bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-start gap-2">
              <AlertCircle className="size-4 shrink-0 mt-0.5" />
              <div>
                <p className="font-semibold">{error.message || "Annotation error"}</p>
                {error.details && <p className="text-red-500/80 mt-0.5">{error.details}</p>}
              </div>
            </div>
          )}

          {loading ? (
            <div className="py-12 flex flex-col items-center justify-center text-xs text-neutral-500 dark:text-neutral-400 gap-2">
              <Loader2 className="size-5 animate-spin text-[#24b47e] dark:text-[#3ecf8e]" />
              <span>Loading extracted vector & OCR annotations...</span>
            </div>
          ) : filteredAnnotations.length === 0 ? (
            <div className="py-12 text-center border border-dashed border-neutral-200 dark:border-[#2e2e2e] rounded-[10px] bg-neutral-50 dark:bg-[#171717] px-4">
              <ScanText className="size-7 text-neutral-400 mx-auto mb-2" />
              <p className="text-xs font-semibold text-neutral-900 dark:text-white">No label annotations found</p>
              <p className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-1 max-w-md mx-auto">
                Click <strong>Run PaddleOCR</strong> above to detect dimensions, elevations, pipe/culvert tags, and road labels, or import a vector PDF / CAD DXF drawing.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {filteredAnnotations.map((ann) => {
                const draft = draftTexts[ann.id] ?? ann.text ?? ""
                const isDirty = draft.trim() !== (ann.text || "").trim()
                const isBusy = savingId === ann.id
                return (
                  <div
                    key={ann.id}
                    className={`rounded-[8px] border p-3 transition-colors ${
                      ann.visible
                        ? "border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020]"
                        : "border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] opacity-70"
                    }`}
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2 mb-2">
                      <div className="flex flex-wrap items-center gap-1.5 text-[10px]">
                        <span className="px-2 py-0.5 rounded-full bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold">
                          {SOURCE_LABELS[ann.source] || ann.source || "OCR"}
                        </span>
                        <span className="px-2 py-0.5 rounded-full bg-neutral-100 dark:bg-[#171717] text-neutral-600 dark:text-neutral-300 border border-neutral-200 dark:border-[#2e2e2e] font-medium">
                          {CATEGORY_LABELS[ann.category] || ann.category || "General"}
                        </span>
                        {typeof ann.confidence === "number" && (
                          <span className="font-mono text-neutral-500 dark:text-neutral-400">
                            {Math.round(ann.confidence * 100)}% conf
                          </span>
                        )}
                        {ann.cad_layer && (
                          <span className="font-mono px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-[#171717] text-neutral-700 dark:text-neutral-300 border border-neutral-200 dark:border-[#2e2e2e]">
                            Layer: {ann.cad_layer}
                          </span>
                        )}
                        {ann.has_geo ? (
                          <span className="px-1.5 py-0.5 rounded bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30">
                            WGS84 Map Overlay
                          </span>
                        ) : (
                          <span className="px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20">
                            Pixel Space ({Math.round(ann.image_x)}, {Math.round(ann.image_y)})
                          </span>
                        )}
                      </div>

                      <div className="flex items-center gap-1">
                        <button
                          type="button"
                          disabled={isBusy}
                          onClick={() => handleToggleField(ann, "verified")}
                          className={`inline-flex items-center gap-1 px-2 py-1 rounded-[6px] text-[11px] font-medium border transition-colors ${
                            ann.verified
                              ? "bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border-[#3ecf8e]/30"
                              : "bg-white dark:bg-[#202020] text-neutral-500 dark:text-neutral-400 border-neutral-200 dark:border-[#2e2e2e] hover:text-neutral-900 dark:hover:text-white"
                          }`}
                          title={ann.verified ? "Verified engineering label" : "Mark label as verified"}
                        >
                          {ann.verified ? (
                            <>
                              <CheckCircle2 className="size-3" />
                              <span>Verified</span>
                            </>
                          ) : (
                            <>
                              <ShieldCheck className="size-3" />
                              <span>Unverified</span>
                            </>
                          )}
                        </button>

                        <button
                          type="button"
                          disabled={isBusy}
                          onClick={() => handleToggleField(ann, "visible")}
                          className="p-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"
                          title={ann.visible ? "Hide from MapLibre overlay" : "Show on MapLibre overlay"}
                        >
                          {ann.visible ? <Eye className="size-3.5" /> : <EyeOff className="size-3.5" />}
                        </button>

                        <button
                          type="button"
                          disabled={isBusy}
                          onClick={() => handleDelete(ann.id)}
                          className="p-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-red-500 hover:bg-red-500/10"
                          title="Delete false-positive detection"
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      <input
                        type="text"
                        value={draft}
                        onChange={(e) =>
                          setDraftTexts((prev) => ({
                            ...prev,
                            [ann.id]: e.target.value,
                          }))
                        }
                        onKeyDown={(e) => {
                          if (e.key === "Enter" && isDirty) handleSaveText(ann)
                        }}
                        className="flex-1 text-xs px-3 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e] font-medium"
                      />
                      {isDirty && (
                        <button
                          type="button"
                          disabled={isBusy}
                          onClick={() => handleSaveText(ann)}
                          className="inline-flex items-center gap-1 rounded-[6px] bg-[#3ecf8e] px-3 py-1.5 text-xs font-semibold text-[#171717] hover:bg-[#24b47e]"
                        >
                          {isBusy ? <Loader2 className="size-3 animate-spin" /> : <Save className="size-3" />}
                          <span>Save</span>
                        </button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        {/* Footer */}
        <footer className="px-6 py-3.5 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] flex items-center justify-end">
          <button
            type="button"
            onClick={onClose}
            className="rounded-[6px] bg-[#3ecf8e] px-5 py-1.5 text-xs font-semibold text-[#171717] hover:bg-[#24b47e]"
          >
            Done
          </button>
        </footer>
      </div>
    </div>
  )
}

