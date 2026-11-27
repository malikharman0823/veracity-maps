import React, { useEffect, useState } from "react"
import { Eraser, Loader2, RotateCcw, Sparkles } from "lucide-react"
import {
  getBackgroundRemoval,
  applyBackgroundRemoval,
  resetBackgroundRemoval,
  getDatasetEnhancement,
  enhanceDataset,
  readableApiError,
} from "../../api"

export default function LayerRasterToolsPanel({
  datasetId,
  isVectorOnly = false,
  onRasterImageUpdated,
  notify,
}) {
  const [loadingStatus, setLoadingStatus] = useState(false)
  const [bgEnabled, setBgEnabled] = useState(false)
  const [bgColor, setBgColor] = useState("#ffffff")
  const [bgThreshold, setBgThreshold] = useState(30)
  const [bgSoftness, setBgSoftness] = useState(10)
  const [bgBusy, setBgBusy] = useState(false)

  const [upscaleScale, setUpscaleScale] = useState(2)
  const [enhState, setEnhState] = useState(null)
  const [upscaleBusy, setUpscaleBusy] = useState(false)
  const [errorMsg, setErrorMsg] = useState("")

  useEffect(() => {
    if (!datasetId || isVectorOnly) return
    let active = true
    setLoadingStatus(true)
    setErrorMsg("")

    Promise.all([
      getBackgroundRemoval(datasetId).catch(() => null),
      getDatasetEnhancement(datasetId).catch(() => null),
    ])
      .then(([bgRes, enhRes]) => {
        if (!active) return
        if (bgRes) {
          setBgEnabled(Boolean(bgRes.enabled))
          if (bgRes.color) setBgColor(bgRes.color)
          if (typeof bgRes.threshold === "number") setBgThreshold(Math.round(bgRes.threshold))
          if (typeof bgRes.edge_softness === "number") setBgSoftness(Math.round(bgRes.edge_softness))
        }
        if (enhRes) {
          setEnhState(enhRes)
          if (enhRes.ai_upscale_factor === 4) setUpscaleScale(4)
          else if (enhRes.ai_upscale_factor === 2) setUpscaleScale(2)
        }
      })
      .finally(() => {
        if (active) setLoadingStatus(false)
      })

    return () => {
      active = false
    }
  }, [datasetId, isVectorOnly])

  if (isVectorOnly || !datasetId) {
    return (
      <div className="mt-1.5 pt-1.5 border-t border-neutral-200 dark:border-[#2e2e2e] space-y-1 text-[10px] text-neutral-400">
        <div className="flex items-center gap-1.5">
          <Eraser className="size-3 shrink-0 opacity-60" />
          <span>Background Clearing</span>
          <span className="ml-auto text-[9px] italic">Raster only</span>
        </div>
        <div className="flex items-center gap-1.5">
          <Sparkles className="size-3 shrink-0 opacity-60" />
          <span>AI Upscaling</span>
          <span className="ml-auto text-[9px] italic">Raster only</span>
        </div>
      </div>
    )
  }

  const handleApplyBg = async () => {
    if (bgBusy) return
    setBgBusy(true)
    setErrorMsg("")
    try {
      const res = await applyBackgroundRemoval(datasetId, {
        color: bgColor,
        threshold: Number(bgThreshold),
        edgeSoftness: Number(bgSoftness),
      })
      setBgEnabled(Boolean(res?.enabled ?? true))
      onRasterImageUpdated?.(datasetId)
      notify?.("Background cleared on layer.")
    } catch (err) {
      const readable = readableApiError(err, "Failed to clear background.")
      setErrorMsg(readable?.message || "Failed to clear background.")
    } finally {
      setBgBusy(false)
    }
  }

  const handleResetBg = async () => {
    if (bgBusy) return
    setBgBusy(true)
    setErrorMsg("")
    try {
      await resetBackgroundRemoval(datasetId)
      setBgEnabled(false)
      onRasterImageUpdated?.(datasetId)
      notify?.("Background restored to original.")
    } catch (err) {
      const readable = readableApiError(err, "Failed to reset background.")
      setErrorMsg(readable?.message || "Failed to reset background.")
    } finally {
      setBgBusy(false)
    }
  }

  const handleApplyUpscale = async () => {
    if (upscaleBusy) return
    setUpscaleBusy(true)
    setErrorMsg("")
    try {
      const res = await enhanceDataset(datasetId, {
        action: "apply",
        preset: "balanced",
        upscale_factor: upscaleScale,
        force_upscale: true,
      })
      setEnhState(res)
      onRasterImageUpdated?.(datasetId)
      notify?.(`AI ${upscaleScale}× upscaling applied.`)
    } catch (err) {
      const readable = readableApiError(err, "Failed to run AI upscaling.")
      setErrorMsg(readable?.message || "Failed to run AI upscaling.")
    } finally {
      setUpscaleBusy(false)
    }
  }

  const handleResetUpscale = async () => {
    if (upscaleBusy) return
    setUpscaleBusy(true)
    setErrorMsg("")
    try {
      const res = await enhanceDataset(datasetId, { action: "reset" })
      setEnhState(res)
      onRasterImageUpdated?.(datasetId)
      notify?.("Restored original raster resolution.")
    } catch (err) {
      const readable = readableApiError(err, "Failed to reset AI upscaling.")
      setErrorMsg(readable?.message || "Failed to reset AI upscaling.")
    } finally {
      setUpscaleBusy(false)
    }
  }

  const isUpscaledActive = Boolean(enhState?.enabled && enhState?.active_variant !== "original")

  return (
    <div
      className="mt-1.5 pt-1.5 border-t border-neutral-200 dark:border-[#2e2e2e] space-y-2.5 text-[10px]"
      onClick={(e) => e.stopPropagation()}
    >
      {loadingStatus && (
        <div className="flex items-center gap-1 text-neutral-400">
          <Loader2 className="size-3 animate-spin" />
          <span>Loading raster settings…</span>
        </div>
      )}

      {/* Background Clearing */}
      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <span className="flex items-center gap-1 font-semibold text-neutral-700 dark:text-neutral-200">
            <Eraser className="size-3 text-sky-500" />
            Background Clearing
          </span>
          {bgEnabled && (
            <span className="px-1.5 py-0.2 rounded text-[9px] font-semibold bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]">
              Active
            </span>
          )}
        </div>

        <div className="flex items-center gap-2">
          <label className="flex items-center gap-1 text-neutral-500 shrink-0">
            <span>Color</span>
            <input
              type="color"
              value={bgColor}
              onChange={(e) => setBgColor(e.target.value)}
              disabled={bgBusy}
              className="w-4 h-4 rounded border border-neutral-300 dark:border-[#3a3a3a] cursor-pointer bg-transparent p-0"
              title="Background color to remove"
            />
          </label>
          <span className="text-neutral-500 shrink-0">Tol</span>
          <input
            type="range"
            min="1"
            max="100"
            value={bgThreshold}
            onChange={(e) => setBgThreshold(Number(e.target.value))}
            disabled={bgBusy}
            className="flex-1 accent-sky-500 h-1 min-w-0"
          />
          <span className="font-mono w-5 text-right text-neutral-500">{bgThreshold}</span>
        </div>

        <div className="flex items-center gap-2">
          <span className="text-neutral-500 shrink-0">Softness</span>
          <input
            type="range"
            min="0"
            max="50"
            value={bgSoftness}
            onChange={(e) => setBgSoftness(Number(e.target.value))}
            disabled={bgBusy}
            className="flex-1 accent-sky-500 h-1 min-w-0"
          />
          <span className="font-mono w-5 text-right text-neutral-500">{bgSoftness}</span>
        </div>

        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={handleApplyBg}
            disabled={bgBusy}
            className="flex-1 h-6 px-2 rounded bg-sky-500/15 hover:bg-sky-500/25 text-sky-600 dark:text-sky-400 font-semibold flex items-center justify-center gap-1 cursor-pointer disabled:opacity-50"
          >
            {bgBusy ? <Loader2 className="size-3 animate-spin" /> : <Eraser className="size-3" />}
            <span>{bgBusy ? "Clearing…" : "Clear Background"}</span>
          </button>
          {bgEnabled && (
            <button
              type="button"
              onClick={handleResetBg}
              disabled={bgBusy}
              className="h-6 px-1.5 rounded border border-neutral-300 dark:border-[#3a3a3a] text-neutral-500 hover:text-neutral-900 dark:hover:text-white cursor-pointer disabled:opacity-50"
              title="Restore original background"
            >
              <RotateCcw className="size-3" />
            </button>
          )}
        </div>
      </div>

      {/* AI Upscaling */}
      <div className="pt-1.5 border-t border-neutral-200/70 dark:border-[#2a2a2a] space-y-1.5">
        <div className="flex items-center justify-between">
          <span className="flex items-center gap-1 font-semibold text-neutral-700 dark:text-neutral-200">
            <Sparkles className="size-3 text-[#3ecf8e]" />
            AI Upscaling
          </span>
          {isUpscaledActive && (
            <span className="px-1.5 py-0.2 rounded text-[9px] font-semibold bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]">
              {enhState?.ai_upscale_factor ? `${enhState.ai_upscale_factor}×` : "Active"}
              {enhState?.enhanced_width && enhState?.enhanced_height
                ? ` · ${enhState.enhanced_width}×${enhState.enhanced_height}`
                : ""}
            </span>
          )}
        </div>

        <div className="flex items-center gap-1">
          {[2, 4].map((factor) => (
            <button
              key={factor}
              type="button"
              onClick={() => setUpscaleScale(factor)}
              disabled={upscaleBusy}
              className={`h-6 px-2 rounded text-[10px] font-semibold border cursor-pointer transition-colors ${
                upscaleScale === factor
                  ? "border-[#3ecf8e] bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]"
                  : "border-neutral-300 dark:border-[#3a3a3a] text-neutral-500 hover:text-neutral-800 dark:hover:text-neutral-200"
              }`}
            >
              {factor}×
            </button>
          ))}
          <button
            type="button"
            onClick={handleApplyUpscale}
            disabled={upscaleBusy}
            className="flex-1 h-6 px-2 rounded bg-[#3ecf8e]/15 hover:bg-[#3ecf8e]/25 text-[#24b47e] dark:text-[#3ecf8e] font-semibold flex items-center justify-center gap-1 cursor-pointer disabled:opacity-50"
          >
            {upscaleBusy ? <Loader2 className="size-3 animate-spin" /> : <Sparkles className="size-3" />}
            <span>{upscaleBusy ? "Upscaling…" : `AI Upscale ${upscaleScale}×`}</span>
          </button>
          {isUpscaledActive && (
            <button
              type="button"
              onClick={handleResetUpscale}
              disabled={upscaleBusy}
              className="h-6 px-1.5 rounded border border-neutral-300 dark:border-[#3a3a3a] text-neutral-500 hover:text-neutral-900 dark:hover:text-white cursor-pointer disabled:opacity-50"
              title="Reset AI upscaling"
            >
              <RotateCcw className="size-3" />
            </button>
          )}
        </div>
      </div>

      {errorMsg && (
        <div className="text-[10px] text-red-500 bg-red-500/10 border border-red-500/20 rounded px-1.5 py-1">
          {errorMsg}
        </div>
      )}
    </div>
  )
}
