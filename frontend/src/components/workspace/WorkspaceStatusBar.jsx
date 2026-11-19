import React, { useEffect, useState } from "react"
import { Compass, Globe, Loader2, CheckCircle2, Copy, Check } from "lucide-react"
import { formatMapScale, computeZoomPercent } from "../../lib/workspaceLayout"

export default function WorkspaceStatusBar({
  map,
  activeCRS = "EPSG:4326",
  loadingStatus = "",
  activeLayerName = "",
  onOpenCRS,
  onResetNorth,
}) {
  const [cursorCoord, setCursorCoord] = useState(null)
  const [viewMeta, setViewMeta] = useState({ zoom: 2, lat: 0, bearing: 0 })
  const [rendering, setRendering] = useState(false)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!map) return undefined

    const syncCamera = () => {
      try {
        const center = map.getCenter()
        setViewMeta({
          zoom: map.getZoom(),
          lat: center?.lat ?? 0,
          bearing: map.getBearing() ?? 0,
        })
      } catch {}
    }

    const onMouseMove = (e) => {
      if (e?.lngLat) {
        setCursorCoord({ lng: e.lngLat.lng, lat: e.lngLat.lat })
      }
    }

    const onDataLoading = () => setRendering(true)
    const onIdle = () => setRendering(false)

    syncCamera()
    map.on("mousemove", onMouseMove)
    map.on("move", syncCamera)
    map.on("zoom", syncCamera)
    map.on("rotate", syncCamera)
    map.on("dataloading", onDataLoading)
    map.on("idle", onIdle)

    return () => {
      map.off("mousemove", onMouseMove)
      map.off("move", syncCamera)
      map.off("zoom", syncCamera)
      map.off("rotate", syncCamera)
      map.off("dataloading", onDataLoading)
      map.off("idle", onIdle)
    }
  }, [map])

  const coordText = cursorCoord
    ? `${cursorCoord.lng.toFixed(6)}, ${cursorCoord.lat.toFixed(6)}`
    : "Move cursor over map"

  const handleCopyCoords = () => {
    if (!cursorCoord || !navigator?.clipboard) return
    navigator.clipboard
      .writeText(`${cursorCoord.lat.toFixed(6)}, ${cursorCoord.lng.toFixed(6)}`)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      })
      .catch(() => {})
  }

  return (
    <footer
      aria-label="Workspace Status Bar"
      className="h-7 shrink-0 px-3 bg-neutral-100 dark:bg-[#1a1a1a] border-t border-neutral-300 dark:border-[#303030] text-[11px] text-neutral-600 dark:text-neutral-300 flex items-center justify-between gap-3 select-none z-30 overflow-x-auto"
    >
      {/* Left: Coordinates | Scale | Zoom | Rotation */}
      <div className="flex items-center gap-2.5 shrink-0">
        <button
          type="button"
          onClick={handleCopyCoords}
          className="inline-flex items-center gap-1 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
          title="Cursor Coordinates (Lng, Lat) — click to copy"
        >
          <span className="text-neutral-400">Coord:</span>
          <span className="font-mono text-neutral-900 dark:text-neutral-100 min-w-[138px] text-left">
            {coordText}
          </span>
          {cursorCoord &&
            (copied ? <Check className="size-2.5 text-[#3ecf8e]" /> : <Copy className="size-2.5 opacity-50" />)}
        </button>

        <span className="text-neutral-300 dark:text-[#3a3a3a]">|</span>

        <div className="inline-flex items-center gap-1" title="Approximate Cartographic Scale">
          <span className="text-neutral-400">Scale:</span>
          <span className="font-mono text-neutral-800 dark:text-neutral-100">
            {formatMapScale(viewMeta.zoom, viewMeta.lat)}
          </span>
        </div>

        <span className="text-neutral-300 dark:text-[#3a3a3a]">|</span>

        <div className="inline-flex items-center gap-1" title="Map Zoom Level & Percentage">
          <span className="text-neutral-400">Zoom:</span>
          <span className="font-mono text-neutral-800 dark:text-neutral-100">
            {viewMeta.zoom.toFixed(1)} ({computeZoomPercent(viewMeta.zoom)}%)
          </span>
        </div>

        <span className="text-neutral-300 dark:text-[#3a3a3a]">|</span>

        <button
          type="button"
          onClick={onResetNorth}
          className="inline-flex items-center gap-1 hover:text-[#3ecf8e] cursor-pointer"
          title="Map Rotation (Bearing) — click to reset north (0°)"
        >
          <Compass className="size-3 text-neutral-400" />
          <span className="font-mono text-neutral-800 dark:text-neutral-100">
            {Number(viewMeta.bearing || 0).toFixed(1)}°
          </span>
        </button>
      </div>

      {/* Right: Active Layer | CRS | Render / Loading Status */}
      <div className="flex items-center gap-2.5 shrink-0">
        {activeLayerName && (
          <span className="hidden lg:inline truncate max-w-[180px] text-neutral-500 dark:text-neutral-400">
            Active: <strong className="text-neutral-800 dark:text-neutral-100">{activeLayerName}</strong>
          </span>
        )}

        <button
          type="button"
          onClick={onOpenCRS}
          className="inline-flex items-center gap-1 px-1.5 py-0.2 rounded bg-neutral-200/70 dark:bg-[#272727] hover:bg-[#3ecf8e]/20 text-neutral-800 dark:text-neutral-100 font-mono cursor-pointer"
          title="Active Coordinate Reference System — click to configure CRS"
        >
          <Globe className="size-3 text-[#3ecf8e]" />
          <span>{activeCRS || "EPSG:4326"}</span>
        </button>

        <span className="text-neutral-300 dark:text-[#3a3a3a]">|</span>

        <div className="inline-flex items-center gap-1 min-w-[92px] justify-end">
          {loadingStatus ? (
            <>
              <Loader2 className="size-3 animate-spin text-[#3ecf8e]" />
              <span className="text-amber-500 font-medium truncate max-w-[120px]">{loadingStatus}</span>
            </>
          ) : rendering ? (
            <>
              <Loader2 className="size-3 animate-spin text-[#3ecf8e]" />
              <span>Rendering…</span>
            </>
          ) : (
            <>
              <CheckCircle2 className="size-3 text-[#3ecf8e]" />
              <span>Render: Ready</span>
            </>
          )}
        </div>
      </div>
    </footer>
  )
}
