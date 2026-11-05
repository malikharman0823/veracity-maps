import React, { useCallback, useEffect, useState } from "react"
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Eye,
  FileSearch,
  Layers,
  Loader2,
  RotateCcw,
  ScanText,
  Sparkles,
  Wand2,
  X,
} from "lucide-react"
import {
  datasetSourceImageUrl,
  enhanceDataset,
  generateDatasetMbtiles,
  getDatasetInspection,
  readableApiError,
} from "../api"

const PRESET_OPTIONS = [
  {
    value: "text_clarity",
    label: "Text & Label Clarity",
    description: "Edge-gated sharpening + CLAHE contrast for small dimensions, room names, and road labels.",
  },
  {
    value: "line_preservation",
    label: "Engineering Line Preservation",
    description: "Gentle bilateral denoising that protects 1px CAD strokes, elevation ticks, and control points.",
  },
  {
    value: "scanned_drawing",
    label: "Scanned Drawing Cleanup",
    description: "Paper illumination normalization + soft-blended adaptive thresholding for blueprint/paper scans.",
  },
  {
    value: "noise_reduction",
    label: "Sensor & Compression Denoise",
    description: "Non-local means color denoising to suppress JPEG artifacts and scanner grain.",
  },
  {
    value: "balanced",
    label: "Balanced Raster Enhancement",
    description: "General-purpose luminance stretch, subtle CLAHE, and detail preservation.",
  },
]

const RESAMPLING_OPTIONS = [
  { value: "lanczos", label: "Lanczos (Sharpest lines & text)" },
  { value: "cubic", label: "Cubic (Smooth photographic & general)" },
  { value: "bilinear", label: "Bilinear (Continuous gradients)" },
  { value: "nearest", label: "Nearest Neighbour (Categorical / discrete classes)" },
]

export default function EnhancementModal({
  open,
  datasetId,
  datasetName,
  onClose,
  onUpdated,
  onOpenAnnotations,
}) {
  const [inspection, setInspection] = useState(null)
  const [loading, setLoading] = useState(false)
  const [processing, setProcessing] = useState(false)
  const [tiling, setTiling] = useState(false)
  const [error, setError] = useState(null)
  const [statusMessage, setStatusMessage] = useState("")
  const [imageBust, setImageBust] = useState(() => Date.now())

  const [preset, setPreset] = useState("text_clarity")
  const [upscaleFactor, setUpscaleFactor] = useState(1)
  const [forceUpscale, setForceUpscale] = useState(false)
  const [grayscale, setGrayscale] = useState(false)
  const [adaptiveThreshold, setAdaptiveThreshold] = useState(false)
  const [runOcr, setRunOcr] = useState(false)
  const [resampling, setResampling] = useState("lanczos")

  const loadInspection = useCallback(async () => {
    if (!datasetId) return
    setLoading(true)
    setError(null)
    try {
      const data = await getDatasetInspection(datasetId)
      setInspection(data)

      const recPreset = data?.enhancement?.preset || data?.recommendation?.recommended_preset
      if (recPreset && recPreset !== "none") {
        setPreset(recPreset)
      }
      if (data?.recommended_resample) {
        setResampling(data.recommended_resample)
      }
      if (data?.recommendation?.ocr_recommended && !data?.has_annotations) {
        setRunOcr(true)
      }
      setImageBust(Date.now())
    } catch (err) {
      setError(readableApiError(err, "Unable to inspect dataset."))
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

  const enhancement = inspection?.enhancement || null
  const quality = inspection?.quality || null
  const content = inspection?.content || null
  const recommendation = inspection?.recommendation || null
  const hasEnhancedImage = Boolean(inspection?.has_enhanced_image || enhancement?.enabled)
  const activeVariant = enhancement?.enabled && enhancement?.active_variant !== "original" ? "enhanced" : "original"
  const isStandaloneMbtiles = content?.classification === "mbtiles"

  async function handleApplyEnhancement() {
    setProcessing(true)
    setError(null)
    setStatusMessage("")
    try {
      const result = await enhanceDataset(datasetId, {
        action: "apply",
        preset,
        upscale_factor: Number(upscaleFactor) || 1,
        force_upscale: Boolean(forceUpscale),
        grayscale: Boolean(grayscale),
        adaptive_threshold: Boolean(adaptiveThreshold),
        run_ocr: Boolean(runOcr),
        resampling,
      })
      const skippedMsg = result?.skipped_high_res_upscale
        ? " (AI upscaling skipped automatically because source resolution is already high.)"
        : ""
      setStatusMessage(
        `Applied ${result?.preset || preset} enhancement (${result?.enhanced_width || inspection?.width}×${result?.enhanced_height || inspection?.height} px)${skippedMsg}`
      )
      await loadInspection()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Image enhancement failed."))
    } finally {
      setProcessing(false)
    }
  }

  async function handleSwitchVariant(targetVariant) {
    setProcessing(true)
    setError(null)
    setStatusMessage("")
    try {
      await enhanceDataset(datasetId, {
        action: "set_variant",
        active_variant: targetVariant,
      })
      setStatusMessage(
        targetVariant === "enhanced"
          ? "Switched active map layer to enhanced output."
          : "Switched active map layer back to untouched original source."
      )
      await loadInspection()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Failed to switch active variant."))
    } finally {
      setProcessing(false)
    }
  }

  async function handleResetOriginal() {
    setProcessing(true)
    setError(null)
    setStatusMessage("")
    try {
      await enhanceDataset(datasetId, { action: "reset" })
      setStatusMessage("Reset active display to original upload. Original file remains unmodified.")
      await loadInspection()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Failed to reset enhancement state."))
    } finally {
      setProcessing(false)
    }
  }

  async function handleGenerateMbtiles() {
    setTiling(true)
    setError(null)
    setStatusMessage("")
    try {
      const res = await generateDatasetMbtiles(datasetId, resampling)
      const count = res?.inspection?.tile_count || 0
      const minZ = res?.inspection?.min_zoom ?? "–"
      const maxZ = res?.inspection?.max_zoom ?? "–"
      setStatusMessage(`Generated optimized GDAL MBTiles pyramid (${count} tiles, zoom ${minZ}–${maxZ}, ${resampling} resampling).`)
      await loadInspection()
      onUpdated?.()
    } catch (err) {
      setError(readableApiError(err, "Failed to generate GDAL MBTiles pyramid."))
    } finally {
      setTiling(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex items-center justify-center p-4 sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="enhancement-modal-title"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !processing && !tiling) onClose()
      }}
    >
      <div className="bg-white dark:bg-[#202020] text-neutral-900 dark:text-white w-full max-w-5xl max-h-[92vh] rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] flex flex-col overflow-hidden transition-colors">
        {/* Header */}
        <header className="px-6 py-4 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] flex items-center justify-between gap-4">
          <div className="flex items-center gap-3 min-w-0">
            <div className="size-10 rounded-[8px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/20 flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e] shrink-0">
              <Sparkles className="size-5" />
            </div>
            <div className="min-w-0">
              <h2 id="enhancement-modal-title" className="text-base font-semibold text-neutral-900 dark:text-white truncate">
                Inspection, OpenCV Enhancement & AI Upscaling
              </h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 truncate">
                {datasetName || inspection?.filename || datasetId} • Non-destructive pipeline (original file preserved)
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            {onOpenAnnotations && (
              <button
                type="button"
                onClick={() => {
                  onClose()
                  onOpenAnnotations(datasetId, datasetName || inspection?.filename)
                }}
                className="inline-flex items-center gap-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] px-3 py-1.5 text-xs font-medium text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828]"
              >
                <ScanText className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                <span>Labels & OCR ({inspection?.annotation_count || 0})</span>
              </button>
            )}
            <button
              type="button"
              onClick={onClose}
              disabled={processing || tiling}
              className="size-8 rounded-[6px] flex items-center justify-center text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#282828] disabled:opacity-50"
              aria-label="Close enhancement dialog"
            >
              <X className="size-4" />
            </button>
          </div>
        </header>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6">
          {error && (
            <div className="p-3.5 rounded-[8px] bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-start gap-2.5">
              <AlertCircle className="size-4 shrink-0 mt-0.5" />
              <div>
                <p className="font-semibold">{error.message || "Operation failed"}</p>
                {error.details && <p className="mt-0.5 text-red-500/80">{error.details}</p>}
              </div>
            </div>
          )}

          {statusMessage && (
            <div className="p-3.5 rounded-[8px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 text-[#24b47e] dark:text-[#3ecf8e] text-xs flex items-start gap-2.5">
              <CheckCircle2 className="size-4 shrink-0 mt-0.5" />
              <span>{statusMessage}</span>
            </div>
          )}

          {loading && !inspection ? (
            <div className="py-16 flex flex-col items-center justify-center text-xs text-neutral-500 dark:text-neutral-400 gap-2">
              <Loader2 className="size-6 animate-spin text-[#24b47e] dark:text-[#3ecf8e]" />
              <span>Inspecting raster dimensions, DPI, quality metrics, and vector metadata...</span>
            </div>
          ) : (
            <>
              {/* 1. Universal File Inspection Grid */}
              <section className="rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] p-4 space-y-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <FileSearch className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
                    <h3 className="text-xs font-semibold uppercase tracking-wider text-neutral-600 dark:text-neutral-300">
                      Universal Dataset Inspection
                    </h3>
                  </div>
                  <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                    {content?.has_raster && (
                      <span className="px-2 py-0.5 rounded-full bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-medium">
                        Raster Imagery
                      </span>
                    )}
                    {content?.has_vector && (
                      <span className="px-2 py-0.5 rounded-full bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-medium">
                        Vector Geometry ({inspection?.vector_feature_count || 0})
                      </span>
                    )}
                    {content?.has_embedded_text && (
                      <span className="px-2 py-0.5 rounded-full bg-purple-500/10 text-purple-600 dark:text-purple-400 border border-purple-500/20 font-medium">
                        Embedded Text
                      </span>
                    )}
                    <span
                      className={`px-2 py-0.5 rounded-full font-medium border ${
                        inspection?.is_georeferenced
                          ? "bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border-[#3ecf8e]/30"
                          : "bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20"
                      }`}
                    >
                      {inspection?.is_georeferenced
                        ? `Georeferenced (${inspection?.crs || "EPSG:4326"})`
                        : "Ungeoreferenced"}
                    </span>
                  </div>
                </div>

                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 text-xs">
                  <div className="rounded-[8px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] px-3 py-2">
                    <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">Original Dimensions</span>
                    <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                      {inspection?.width && inspection?.height
                        ? `${inspection.width} × ${inspection.height} px`
                        : "Tile Pyramid"}
                    </span>
                  </div>
                  <div className="rounded-[8px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] px-3 py-2">
                    <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">DPI / Color Depth</span>
                    <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                      {inspection?.dpi_x || inspection?.rendering_dpi
                        ? `${Math.round(inspection.dpi_x || inspection.rendering_dpi)} DPI`
                        : "Native px"}{" "}
                      • {inspection?.color_depth_bits ? `${inspection.color_depth_bits}-bit` : inspection?.dtype || "RGBA"}
                    </span>
                  </div>
                  <div className="rounded-[8px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] px-3 py-2">
                    <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">Sharpness / Blur Score</span>
                    <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                      {quality?.blur_score !== undefined
                        ? `${quality.blur_score} (${quality.blur_status || "normal"})`
                        : "N/A"}
                    </span>
                  </div>
                  <div className="rounded-[8px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] px-3 py-2">
                    <span className="text-[11px] text-neutral-500 dark:text-neutral-400 block">Text Density & Compression</span>
                    <span className="font-mono font-semibold text-neutral-900 dark:text-white">
                      {quality?.text_density_level || "standard"} • {inspection?.compression || "lossless"}
                    </span>
                  </div>
                </div>

                {recommendation?.reason && (
                  <p className="text-xs text-neutral-600 dark:text-neutral-300 bg-white dark:bg-[#202020] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] px-3 py-2">
                    <strong className="text-neutral-900 dark:text-white">Recommendation:</strong> {recommendation.reason}
                  </p>
                )}
              </section>

              {/* 2. Original vs. Enhanced Side-by-Side Preview */}
              {!isStandaloneMbtiles && (
                <section className="space-y-2.5">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <h3 className="text-xs font-semibold uppercase tracking-wider text-neutral-600 dark:text-neutral-300 flex items-center gap-1.5">
                      <Eye className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                      Original vs. Enhanced Preview
                    </h3>

                    {hasEnhancedImage && (
                      <div className="flex items-center gap-1.5">
                        <span className="text-[11px] text-neutral-500 dark:text-neutral-400">Active Map Layer:</span>
                        <button
                          type="button"
                          disabled={processing}
                          onClick={() => handleSwitchVariant("original")}
                          className={`px-2.5 py-1 rounded-[6px] text-xs font-medium border transition-colors ${
                            activeVariant === "original"
                              ? "bg-neutral-900 dark:bg-white text-white dark:text-neutral-900 border-neutral-900 dark:border-white font-semibold"
                              : "bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 border-neutral-200 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#282828]"
                          }`}
                        >
                          Original
                        </button>
                        <button
                          type="button"
                          disabled={processing}
                          onClick={() => handleSwitchVariant("enhanced")}
                          className={`px-2.5 py-1 rounded-[6px] text-xs font-medium border transition-colors ${
                            activeVariant === "enhanced"
                              ? "bg-[#3ecf8e] text-[#171717] border-[#3ecf8e] font-semibold"
                              : "bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 border-neutral-200 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#282828]"
                          }`}
                        >
                          Enhanced ({enhancement?.enhanced_width || inspection?.width}×
                          {enhancement?.enhanced_height || inspection?.height})
                        </button>
                      </div>
                    )}
                  </div>

                  <div className={`grid grid-cols-1 ${hasEnhancedImage ? "md:grid-cols-2" : ""} gap-3`}>
                    <div className="rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] overflow-hidden flex flex-col">
                      <div className="px-3 py-1.5 bg-white dark:bg-[#202020] border-b border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-between text-[11px]">
                        <span className="font-semibold text-neutral-900 dark:text-white">Original Source (Untouched)</span>
                        <span className="font-mono text-neutral-500 dark:text-neutral-400">
                          {inspection?.width} × {inspection?.height} px
                        </span>
                      </div>
                      <div className="h-52 flex items-center justify-center p-2">
                        <img
                          src={datasetSourceImageUrl(datasetId, { variant: "original", bust: imageBust })}
                          alt="Original dataset raster"
                          className="max-h-48 max-w-full object-contain rounded"
                        />
                      </div>
                    </div>

                    {hasEnhancedImage && (
                      <div className="rounded-[10px] border border-[#3ecf8e]/40 bg-neutral-50 dark:bg-[#171717] overflow-hidden flex flex-col">
                        <div className="px-3 py-1.5 bg-[#3ecf8e]/10 border-b border-[#3ecf8e]/20 flex items-center justify-between text-[11px]">
                          <span className="font-semibold text-[#24b47e] dark:text-[#3ecf8e]">
                            Enhanced Output ({enhancement?.preset || "OpenCV"})
                            {enhancement?.ai_upscaled ? ` • ${enhancement.ai_upscale_factor}x Real-ESRGAN` : ""}
                          </span>
                          <span className="font-mono text-[#24b47e] dark:text-[#3ecf8e]">
                            {enhancement?.enhanced_width || inspection?.width} ×{" "}
                            {enhancement?.enhanced_height || inspection?.height} px
                          </span>
                        </div>
                        <div className="h-52 flex items-center justify-center p-2">
                          <img
                            src={datasetSourceImageUrl(datasetId, { variant: "enhanced", bust: imageBust })}
                            alt="Enhanced dataset raster"
                            className="max-h-48 max-w-full object-contain rounded"
                          />
                        </div>
                      </div>
                    )}
                  </div>
                </section>
              )}

              {/* 3. OpenCV Enhancement & Real-ESRGAN Controls */}
              {!isStandaloneMbtiles ? (
                <section className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {/* OpenCV Pipeline Card */}
                  <div className="rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] p-4 space-y-3 bg-white dark:bg-[#202020]">
                    <h3 className="text-xs font-semibold uppercase tracking-wider text-neutral-600 dark:text-neutral-300 flex items-center gap-1.5">
                      <Wand2 className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                      OpenCV Enhancement Profile
                    </h3>

                    <div className="space-y-2">
                      {PRESET_OPTIONS.map((opt) => (
                        <label
                          key={opt.value}
                          className={`block rounded-[8px] border p-2.5 cursor-pointer transition-colors ${
                            preset === opt.value
                              ? "border-[#3ecf8e] bg-[#3ecf8e]/10"
                              : "border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-neutral-600"
                          }`}
                        >
                          <div className="flex items-center gap-2">
                            <input
                              type="radio"
                              name="opencvPreset"
                              value={opt.value}
                              checked={preset === opt.value}
                              onChange={() => setPreset(opt.value)}
                              disabled={processing}
                              className="accent-[#3ecf8e]"
                            />
                            <span className="text-xs font-semibold text-neutral-900 dark:text-white">{opt.label}</span>
                            {recommendation?.recommended_preset === opt.value && (
                              <span className="ml-auto text-[10px] px-1.5 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-medium">
                                Suggested
                              </span>
                            )}
                          </div>
                          <p className="text-[11px] text-neutral-500 dark:text-neutral-400 pl-5 mt-0.5">{opt.description}</p>
                        </label>
                      ))}
                    </div>

                    <div className="pt-1 space-y-2 border-t border-neutral-200 dark:border-[#2e2e2e]">
                      <label className="flex items-center gap-2 text-xs text-neutral-900 dark:text-white cursor-pointer">
                        <input
                          type="checkbox"
                          checked={adaptiveThreshold}
                          onChange={(e) => setAdaptiveThreshold(e.target.checked)}
                          disabled={processing}
                          className="rounded-[4px] border-neutral-300 dark:border-[#2e2e2e] accent-[#3ecf8e]"
                        />
                        <span>Soft-blended adaptive thresholding (for faint scanned blueprints)</span>
                      </label>
                      <label className="flex items-center gap-2 text-xs text-neutral-900 dark:text-white cursor-pointer">
                        <input
                          type="checkbox"
                          checked={grayscale}
                          onChange={(e) => setGrayscale(e.target.checked)}
                          disabled={processing}
                          className="rounded-[4px] border-neutral-300 dark:border-[#2e2e2e] accent-[#3ecf8e]"
                        />
                        <span>Monochrome / grayscale normalization</span>
                      </label>
                    </div>
                  </div>

                  {/* Real-ESRGAN AI Upscaling & GDAL Optimization Card */}
                  <div className="rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] p-4 space-y-4 bg-white dark:bg-[#202020] flex flex-col justify-between">
                    <div className="space-y-3">
                      <h3 className="text-xs font-semibold uppercase tracking-wider text-neutral-600 dark:text-neutral-300 flex items-center gap-1.5">
                        <Sparkles className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                        Optional AI Upscaling (Real-ESRGAN)
                      </h3>

                      <div>
                        <label className="text-xs font-medium text-neutral-900 dark:text-white block mb-1">
                          Super-Resolution Scale Factor
                        </label>
                        <select
                          value={upscaleFactor}
                          onChange={(e) => setUpscaleFactor(Number(e.target.value))}
                          disabled={processing}
                          className="w-full text-xs px-3 py-2 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                        >
                          <option value={1}>1x — Keep Native Pixel Dimensions (Default)</option>
                          <option value={2}>2x — Real-ESRGAN Super-Resolution</option>
                          <option value={4}>4x — Real-ESRGAN Super-Resolution (Low-Res Scans)</option>
                        </select>
                      </div>

                      {recommendation?.is_high_resolution && upscaleFactor > 1 && (
                        <div className="p-2.5 rounded-[8px] bg-amber-500/10 border border-amber-500/20 text-amber-600 dark:text-amber-400 text-[11px] space-y-1.5">
                          <p className="font-medium">
                            Source image is already high resolution ({inspection?.width}×{inspection?.height} px).
                            AI upscaling is automatically skipped unless forced.
                          </p>
                          <label className="inline-flex items-center gap-1.5 cursor-pointer font-medium">
                            <input
                              type="checkbox"
                              checked={forceUpscale}
                              onChange={(e) => setForceUpscale(e.target.checked)}
                              disabled={processing}
                              className="accent-[#3ecf8e]"
                            />
                            <span>Force AI upscaling anyway</span>
                          </label>
                        </div>
                      )}

                      {(upscaleFactor > 1 || enhancement?.ai_upscaled) && (
                        <div className="p-2.5 rounded-[8px] bg-amber-500/10 border border-amber-500/20 text-amber-600 dark:text-amber-400 text-[11px] flex items-start gap-2">
                          <AlertTriangle className="size-4 shrink-0 text-amber-500 mt-0.5" />
                          <span>
                            <strong>Engineering Truthfulness Notice:</strong> AI-upscaled pixels enhance visual readability only and are never marked as verified engineering measurements. Original raster dimensions and georeferencing control points remain untouched.
                          </span>
                        </div>
                      )}

                      <div className="pt-3 border-t border-neutral-200 dark:border-[#2e2e2e] space-y-2">
                        <h4 className="text-xs font-semibold uppercase tracking-wider text-neutral-600 dark:text-neutral-300 flex items-center gap-1.5">
                          <Layers className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e]" />
                          GDAL Overview & MBTiles Resampling
                        </h4>
                        <select
                          value={resampling}
                          onChange={(e) => setResampling(e.target.value)}
                          disabled={processing || tiling}
                          className="w-full text-xs px-3 py-2 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                        >
                          {RESAMPLING_OPTIONS.map((opt) => (
                            <option key={opt.value} value={opt.value}>
                              {opt.label}
                            </option>
                          ))}
                        </select>
                        {inspection?.is_georeferenced && (
                          <button
                            type="button"
                            onClick={handleGenerateMbtiles}
                            disabled={tiling || processing}
                            className="w-full inline-flex items-center justify-center gap-1.5 rounded-[6px] border border-[#3ecf8e]/30 bg-[#3ecf8e]/10 px-3 py-2 text-xs font-medium text-[#24b47e] dark:text-[#3ecf8e] hover:bg-[#3ecf8e]/20 disabled:opacity-50"
                          >
                            {tiling ? (
                              <>
                                <Loader2 className="size-3.5 animate-spin" />
                                <span>Building GDAL Overviews & MBTiles Pyramid...</span>
                              </>
                            ) : (
                              <>
                                <Layers className="size-3.5" />
                                <span>Optimize GDAL Overviews & Build MBTiles Now</span>
                              </>
                            )}
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                </section>
              ) : (
                <div className="rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] p-4 text-xs text-neutral-600 dark:text-neutral-400">
                  This layer is backed directly by a standalone SQLite <strong>.mbtiles</strong> tile pyramid and renders natively via MapLibre tile streaming.
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <footer className="px-6 py-4 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] flex flex-wrap items-center justify-between gap-2">
          <div>
            {hasEnhancedImage && !isStandaloneMbtiles && (
              <button
                type="button"
                onClick={handleResetOriginal}
                disabled={processing || tiling}
                className="inline-flex items-center gap-1.5 rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] px-3.5 py-2 text-xs font-medium text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828] disabled:opacity-50 cursor-pointer"
              >
                <RotateCcw className="size-3.5" />
                <span>Reset to Original</span>
              </button>
            )}
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={processing || tiling}
              className="rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] px-4 py-2 text-xs font-medium text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828] disabled:opacity-50 cursor-pointer"
            >
              Close
            </button>
            {!isStandaloneMbtiles && (
              <button
                type="button"
                onClick={handleApplyEnhancement}
                disabled={processing || tiling}
                className="inline-flex items-center gap-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-5 py-2 text-xs font-medium text-[#171717] disabled:opacity-50 shadow-xs cursor-pointer"
              >
                {processing ? (
                  <>
                    <Loader2 className="size-3.5 animate-spin" />
                    <span>
                      {upscaleFactor > 1
                        ? `Running OpenCV + ${upscaleFactor}x Real-ESRGAN...`
                        : "Applying OpenCV Enhancement..."}
                    </span>
                  </>
                ) : (
                  <>
                    <Sparkles className="size-3.5" />
                    <span>Apply Enhancement</span>
                  </>
                )}
              </button>
            )}
          </div>
        </footer>
      </div>
    </div>
  )
}

