import React, { useCallback, useEffect, useState } from "react"
import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Contrast,
  Eraser,
  Loader2,
  RotateCcw,
  SlidersHorizontal,
  Sparkles,
  X,
} from "lucide-react"
import {
  applyBackgroundRemoval,
  datasetSourceImageUrl,
  enhanceDataset,
  getDatasetInspection,
  readableApiError,
} from "../api"

/**
 * Simplified Raster Processing Modal — strictly the 4 essential raster operations:
 * 1. Simple AI Upscaling (1x / 2x / 4x)
 * 2. Adaptive Histogram Equalization (CLAHE)
 * 3. Brightness & Contrast
 * 4. Background Removal
 */
export default function EnhancementModal({
  open,
  datasetId,
  datasetName,
  initialMode = "clahe",
  onClose,
  onUpdated,
}) {
  const [inspection, setInspection] = useState(null)
  const [loading, setLoading] = useState(false)
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState(null)
  const [statusMessage, setStatusMessage] = useState("")
  const [imageBust, setImageBust] = useState(() => Date.now())

  // 4 essential operations
  const [upscaleFactor, setUpscaleFactor] = useState(1)
  const [enableClahe, setEnableClahe] = useState(true)
  const [contrastMode, setContrastMode] = useState("balanced") // "balanced" | "high"
  const [removeBg, setRemoveBg] = useState(false)
  const [bgTolerance, setBgTolerance] = useState(32)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [resampling, setResampling] = useState("lanczos")

  useEffect(() => {
    if (!open) return
    if (initialMode === "upscale") {
      setUpscaleFactor(2)
      setRemoveBg(false)
    } else if (initialMode === "background") {
      setRemoveBg(true)
    } else if (initialMode === "contrast") {
      setContrastMode("high")
      setRemoveBg(false)
    } else {
      setEnableClahe(true)
    }
  }, [open, initialMode])

  const loadInspection = useCallback(async () => {
    if (!datasetId) return
    setLoading(true)
    setError(null)
    try {
      const data = await getDatasetInspection(datasetId)
      setInspection(data)
      setImageBust(Date.now())
    } catch (err) {
      setError(readableApiError(err, "Unable to inspect raster dataset."))
    } finally {
      setLoading(false)
    }
  }, [datasetId])

  useEffect(() => {
    if (open && datasetId) {
      setStatusMessage("")
      loadInspection()
    }
  }, [open, datasetId, loadInspection])

  if (!open || !datasetId) return null

  const handleApply = async () => {
    if (processing) return
    setProcessing(true)
    setError(null)
    setStatusMessage("")
    try {
      if (removeBg) {
        await applyBackgroundRemoval(datasetId, {
          color: "#ffffff",
          threshold: bgTolerance,
        })
      }
      if (enableClahe || upscaleFactor > 1 || !removeBg) {
        await enhanceDataset(datasetId, {
          preset: enableClahe ? (contrastMode === "high" ? "text_clarity" : "balanced") : "balanced",
          upscale_factor: upscaleFactor,
          force_upscale: upscaleFactor > 1,
          resampling,
        })
      }
      setImageBust(Date.now())
      setStatusMessage("Raster processing applied.")
      await loadInspection()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Raster processing failed."))
    } finally {
      setProcessing(false)
    }
  }

  const handleReset = async () => {
    if (processing) return
    setProcessing(true)
    setError(null)
    try {
      await enhanceDataset(datasetId, { reset: true })
      setUpscaleFactor(1)
      setRemoveBg(false)
      setImageBust(Date.now())
      setStatusMessage("Restored original raster image.")
      await loadInspection()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Unable to reset raster."))
    } finally {
      setProcessing(false)
    }
  }

  const previewUrl = `${datasetSourceImageUrl(datasetId)}?v=${imageBust}`

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-xs">
      <div
        role="dialog"
        aria-modal="true"
        className="w-full max-w-2xl rounded-lg border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1a1a1a] text-neutral-900 dark:text-neutral-100 shadow-2xl overflow-hidden flex flex-col max-h-[88vh]"
      >
        {/* Header */}
        <div className="px-5 py-3.5 border-b border-neutral-200 dark:border-[#2a2a2a] flex items-center justify-between">
          <div>
            <h2 className="text-sm font-semibold text-neutral-900 dark:text-white">
              Raster Processing
            </h2>
            <p className="text-[11px] text-neutral-500 dark:text-neutral-400 truncate">
              {datasetName || datasetId}
              {inspection?.width && inspection?.height
                ? ` • ${inspection.width} × ${inspection.height}px`
                : ""}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-[6px] text-neutral-400 hover:text-neutral-800 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#262626] cursor-pointer"
            aria-label="Close raster processing"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Body */}
        <div className="p-5 overflow-y-auto grid grid-cols-1 md:grid-cols-2 gap-5">
          {/* Preview */}
          <div className="space-y-2">
            <div className="aspect-square rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#111111] flex items-center justify-center overflow-hidden">
              {loading ? (
                <Loader2 className="size-5 animate-spin text-[#3ecf8e]" />
              ) : (
                <img
                  src={previewUrl}
                  alt={datasetName || "Raster preview"}
                  className="max-h-full max-w-full object-contain"
                />
              )}
            </div>
            {error && (
              <div className="p-2.5 rounded-[6px] bg-red-500/10 border border-red-500/30 text-xs text-red-600 dark:text-red-400 flex items-center gap-2">
                <AlertCircle className="size-3.5 shrink-0" />
                <span>{error}</span>
              </div>
            )}
            {statusMessage && (
              <div className="p-2.5 rounded-[6px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-xs text-[#24b47e] dark:text-[#3ecf8e] flex items-center gap-2">
                <CheckCircle2 className="size-3.5 shrink-0" />
                <span>{statusMessage}</span>
              </div>
            )}
          </div>

          {/* 4 Essential Operations */}
          <div className="space-y-3 text-xs">
            {/* 1. Simple AI Upscaling */}
            <div className="p-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] space-y-2">
              <div className="flex items-center gap-2 font-medium text-neutral-900 dark:text-white">
                <Sparkles className="size-3.5 text-[#3ecf8e]" />
                <span>Simple AI Upscaling</span>
              </div>
              <div className="grid grid-cols-3 gap-1.5">
                {[1, 2, 4].map((factor) => (
                  <button
                    key={factor}
                    type="button"
                    onClick={() => setUpscaleFactor(factor)}
                    className={`py-1.5 rounded-[5px] border text-xs font-medium cursor-pointer transition-colors ${
                      upscaleFactor === factor
                        ? "border-[#3ecf8e] bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]"
                        : "border-neutral-200 dark:border-[#343434] text-neutral-600 dark:text-neutral-300"
                    }`}
                  >
                    {factor === 1 ? "Off (1×)" : `${factor}×`}
                  </button>
                ))}
              </div>
            </div>

            {/* 2. Adaptive Histogram Equalization */}
            <label className="p-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between gap-2 cursor-pointer">
              <div className="flex items-center gap-2">
                <Contrast className="size-3.5 text-[#3ecf8e]" />
                <div>
                  <div className="font-medium text-neutral-900 dark:text-white">
                    Adaptive Histogram Equalization
                  </div>
                  <div className="text-[10.5px] text-neutral-500">CLAHE local contrast enhancement</div>
                </div>
              </div>
              <input
                type="checkbox"
                checked={enableClahe}
                onChange={(e) => setEnableClahe(e.target.checked)}
                className="accent-[#3ecf8e] size-4"
              />
            </label>

            {/* 3. Brightness & Contrast */}
            <div className="p-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] space-y-2">
              <div className="flex items-center gap-2 font-medium text-neutral-900 dark:text-white">
                <SlidersHorizontal className="size-3.5 text-[#3ecf8e]" />
                <span>Brightness &amp; Contrast</span>
              </div>
              <div className="grid grid-cols-2 gap-1.5">
                {[
                  { id: "balanced", label: "Balanced" },
                  { id: "high", label: "High Clarity" },
                ].map((opt) => (
                  <button
                    key={opt.id}
                    type="button"
                    onClick={() => setContrastMode(opt.id)}
                    className={`py-1.5 rounded-[5px] border text-xs font-medium cursor-pointer ${
                      contrastMode === opt.id
                        ? "border-[#3ecf8e] bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]"
                        : "border-neutral-200 dark:border-[#343434] text-neutral-600 dark:text-neutral-300"
                    }`}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            </div>

            {/* 4. Background Removal */}
            <div className="p-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] space-y-2">
              <label className="flex items-center justify-between gap-2 cursor-pointer">
                <div className="flex items-center gap-2">
                  <Eraser className="size-3.5 text-[#3ecf8e]" />
                  <div>
                    <div className="font-medium text-neutral-900 dark:text-white">Background Removal</div>
                    <div className="text-[10.5px] text-neutral-500">Convert white paper background to transparent</div>
                  </div>
                </div>
                <input
                  type="checkbox"
                  checked={removeBg}
                  onChange={(e) => setRemoveBg(e.target.checked)}
                  className="accent-[#3ecf8e] size-4"
                />
              </label>
              {removeBg && (
                <div className="flex items-center gap-2 pt-1 text-[11px] text-neutral-500">
                  <span>Tolerance</span>
                  <input
                    type="range"
                    min="5"
                    max="90"
                    value={bgTolerance}
                    onChange={(e) => setBgTolerance(Number(e.target.value))}
                    className="flex-1 accent-[#3ecf8e] h-1"
                  />
                  <span className="font-mono w-6 text-right">{bgTolerance}</span>
                </div>
              )}
            </div>

            {/* Expandable Advanced Settings */}
            <div className="pt-1">
              <button
                type="button"
                onClick={() => setShowAdvanced((p) => !p)}
                className="flex items-center gap-1 text-[11px] text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200 cursor-pointer"
              >
                {showAdvanced ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
                <span>Advanced Settings</span>
              </button>
              {showAdvanced && (
                <div className="mt-2 p-2.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] space-y-1.5">
                  <label className="block text-[11px] text-neutral-500">Resampling Method</label>
                  <select
                    value={resampling}
                    onChange={(e) => setResampling(e.target.value)}
                    className="w-full h-8 px-2 rounded-[5px] border border-neutral-300 dark:border-[#363636] bg-white dark:bg-[#141414] text-xs"
                  >
                    <option value="lanczos">Lanczos (Sharpest)</option>
                    <option value="cubic">Cubic (Smooth)</option>
                    <option value="bilinear">Bilinear</option>
                    <option value="nearest">Nearest Neighbour</option>
                  </select>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="px-5 py-3 border-t border-neutral-200 dark:border-[#2a2a2a] flex items-center justify-between">
          <button
            type="button"
            disabled={processing}
            onClick={handleReset}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#323232] text-xs font-medium text-neutral-600 dark:text-neutral-300 hover:border-neutral-400 cursor-pointer disabled:opacity-40"
          >
            <RotateCcw className="size-3.5" />
            <span>Reset Original</span>
          </button>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 rounded-[6px] text-xs text-neutral-500 hover:bg-neutral-100 dark:hover:bg-[#262626] cursor-pointer"
            >
              Close
            </button>
            <button
              type="button"
              disabled={processing}
              onClick={handleApply}
              className="inline-flex items-center gap-1.5 px-4 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#34b77c] text-[#141414] text-xs font-semibold cursor-pointer disabled:opacity-40"
            >
              {processing ? <Loader2 className="size-3.5 animate-spin" /> : null}
              <span>{processing ? "Processing…" : "Apply"}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
