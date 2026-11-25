import React, { useEffect, useMemo, useRef, useState } from "react"
import { useParams, useSearchParams } from "react-router-dom"
import { Map as MapLibreMap, NavigationControl, setWorkerUrl } from "maplibre-gl"
import mapLibreWorkerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url"
import "maplibre-gl/dist/maplibre-gl.css"

setWorkerUrl(mapLibreWorkerUrl)
import {
  Eye,
  EyeOff,
  Layers,
  Image as ImageIcon,
  Shapes,
  MapPin,
  Copy,
  Check,
  Code2,
  X,
  AlertCircle,
  Loader2,
  ArrowUpDown,
} from "lucide-react"
import { getPublishedMap } from "../api"

export default function PublishedMapViewerPage() {
  const { mapId } = useParams()
  const [searchParams] = useSearchParams()
  const token = searchParams.get("token") || ""

  const mapContainerRef = useRef(null)
  const mapRef = useRef(null)

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [mapError, setMapError] = useState(null)
  const [publishedMap, setPublishedMap] = useState(null)
  const [rasterOnTop, setRasterOnTop] = useState(false)
  const [styleUrl, setStyleUrl] = useState("")
  const [apiOrigin, setApiOrigin] = useState("")

  // Independent client-side visibility state (never modifies workspace)
  const [rasterVis, setRasterVis] = useState({})
  const [vectorVis, setVectorVis] = useState({})
  const [pointsOnlyVis, setPointsOnlyVis] = useState(true)
  const [panelOpen, setPanelOpen] = useState(true)
  const [showUrls, setShowUrls] = useState(false)
  const [copiedKey, setCopiedKey] = useState("")
  const [identified, setIdentified] = useState(null)

  // 1. Fetch published map configuration (no workspace session required)
  useEffect(() => {
    let active = true
    setLoading(true)
    setError(null)
    getPublishedMap(mapId, token)
      .then((res) => {
        if (!active) return
        const pm = res?.published_map
        if (!pm) {
          setError("Published map data is unavailable.")
          return
        }
        setPublishedMap(pm)
        setStyleUrl(res.style_url || "")
        setApiOrigin(res.api_origin || "")

        const initR = {}
        ;(pm.raster_layers || []).forEach((rl) => {
          initR[rl.layer_id] = rl.visible !== false
        })
        const initV = {}
        ;(pm.vector_layers || []).forEach((vl) => {
          initV[vl.layer_id] = vl.visible !== false
        })
        setRasterVis(initR)
        setVectorVis(initV)
      })
      .catch((err) => {
        if (!active) return
        const msg =
          err?.response?.data?.error?.message ||
          "Unable to load published map. It may be restricted or revoked."
        setError(msg)
      })
      .finally(() => {
        if (active) setLoading(false)
      })

    return () => {
      active = false
    }
  }, [mapId, token])

  // 2. Initialize lightweight MapLibre GL viewer directly from /maps/{map_id}/style.json
  useEffect(() => {
    if (loading || !publishedMap || !styleUrl || !mapContainerRef.current) return

    let cancelled = false
    let map = null
    setMapError(null)

    fetch(styleUrl, { mode: "cors" })
      .then((res) => {
        if (!res.ok) throw new Error(`Style fetch failed (HTTP ${res.status})`)
        return res.json()
      })
      .then((styleDoc) => {
        if (cancelled || !mapContainerRef.current) return

        map = new MapLibreMap({
          container: mapContainerRef.current,
          style: styleDoc,
          center:
            Array.isArray(publishedMap.center) && publishedMap.center.length === 2
              ? publishedMap.center
              : [0, 0],
          zoom: Number.isFinite(publishedMap.initial_zoom) ? publishedMap.initial_zoom : 12,
          attributionControl: true,
        })
        map.addControl(new NavigationControl(), "top-right")
        mapRef.current = map

        map.on("error", (ev) => {
          const msg = ev?.error?.message || ""
          if (msg) {
            console.error("[PublishedMapViewer] MapLibre error:", msg, ev)
            setMapError((prev) => prev || msg)
          }
        })

        map.on("load", () => {
          map.resize()
          if (Array.isArray(publishedMap.bounds) && publishedMap.bounds.length === 4) {
            const [w, s, e, n] = publishedMap.bounds
            if (w < e && s < n && w > -179 && e < 179) {
              try {
                map.fitBounds(
                  [
                    [w, s],
                    [e, n],
                  ],
                  { padding: 48, maxZoom: publishedMap.initial_zoom || 15, duration: 0 }
                )
              } catch {}
            }
          }
        })

        // Interactive point, line & polygon identification via queryRenderedFeatures
        const getQueryLayers = () => {
          const styleLayers = map.getStyle()?.layers || []
          const circles = styleLayers.filter((l) => l.id.startsWith("pub-vec-circle-")).map((l) => l.id)
          const lines = styleLayers.filter((l) => l.id.startsWith("pub-vec-line-")).map((l) => l.id)
          const fills = styleLayers.filter((l) => l.id.startsWith("pub-vec-fill-")).map((l) => l.id)
          return [...circles, ...lines, ...fills]
        }

        map.on("mousemove", (e) => {
          const qLayers = getQueryLayers()
          if (qLayers.length === 0) return
          const hits = map.queryRenderedFeatures(e.point, { layers: qLayers })
          map.getCanvas().style.cursor = hits.length > 0 ? "pointer" : ""
        })

        map.on("click", (e) => {
          const qLayers = getQueryLayers()
          if (qLayers.length === 0) return
          const hits = map.queryRenderedFeatures(e.point, { layers: qLayers })
          if (!hits.length) {
            setIdentified(null)
            return
          }
          const top = hits[0]
          const props = top.properties || {}
          const layerSlug = String(top.layer?.id || "").replace(/^pub-vec-(?:circle|line|fill)-/, "")
          const matched = (publishedMap.vector_layers || []).find((v) => v.layer_id === layerSlug)
          setIdentified({
            layerName: matched?.name || layerSlug,
            sourceLayer: top.sourceLayer || props._source_layer || matched?.source_layer || layerSlug,
            featureId: String(props._source_feature_id || props.feature_id || top.id || ""),
            geometryType: top.geometry?.type || matched?.geometry_type || "Feature",
            crs: matched?.source_crs || "EPSG:4326",
            lngLat: [Number(e.lngLat.lng.toFixed(6)), Number(e.lngLat.lat.toFixed(6))],
            properties: props,
          })
        })
      })
      .catch((err) => {
        if (!cancelled) {
          console.error("[PublishedMapViewer] Failed to initialize map:", err)
          setMapError(err?.message || "Failed to load map style.")
        }
      })

    return () => {
      cancelled = true
      if (map) {
        map.remove()
      }
      mapRef.current = null
    }
  }, [loading, publishedMap, styleUrl])

  // 3. Synchronize independent visibility toggles to MapLibre layers (client-side only)
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.style || !map.isStyleLoaded() || !publishedMap) return

    ;(publishedMap.raster_layers || []).forEach((rl) => {
      const lid = `pub-raster-lyr-${rl.layer_id}`
      if (map.getLayer(lid)) {
        map.setLayoutProperty(lid, "visibility", rasterVis[rl.layer_id] !== false ? "visible" : "none")
      }
    })

    ;(publishedMap.vector_layers || []).forEach((vl) => {
      const visible = vectorVis[vl.layer_id] !== false
      const fillId = `pub-vec-fill-${vl.layer_id}`
      const lineId = `pub-vec-line-${vl.layer_id}`
      const circleId = `pub-vec-circle-${vl.layer_id}`
      if (map.getLayer(fillId)) {
        map.setLayoutProperty(fillId, "visibility", visible ? "visible" : "none")
      }
      if (map.getLayer(lineId)) {
        map.setLayoutProperty(lineId, "visibility", visible ? "visible" : "none")
      }
      if (map.getLayer(circleId)) {
        map.setLayoutProperty(circleId, "visibility", visible && pointsOnlyVis ? "visible" : "none")
      }
    })
  }, [rasterVis, vectorVis, pointsOnlyVis, publishedMap])

  const rasterLayers = publishedMap?.raster_layers || []
  const vectorLayers = publishedMap?.vector_layers || []
  const pointLayers = useMemo(
    () => vectorLayers.filter((v) => String(v.geometry_type || "").toLowerCase().includes("point")),
    [vectorLayers]
  )

  const allRastersVisible = rasterLayers.length > 0 && rasterLayers.every((r) => rasterVis[r.layer_id] !== false)
  const allVectorsVisible = vectorLayers.length > 0 && vectorLayers.every((v) => vectorVis[v.layer_id] !== false)

  // Initialize rasterOnTop from published map's layer_order
  useEffect(() => {
    const order = publishedMap?.layer_order
    if (Array.isArray(order) && order.length > 0) {
      setRasterOnTop(String(order[0]).toLowerCase() === "raster")
    }
  }, [publishedMap?.map_id])

  // Live MapLibre z-order sync when viewer toggles Raster <-> Vector stacking
  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.style || !map.isStyleLoaded()) return
    const moveSafe = (id) => {
      if (map.getLayer(id)) {
        try { map.moveLayer(id) } catch {}
      }
    }
    const moveRasters = () => {
      rasterLayers.forEach((rl) => moveSafe(`pub-raster-lyr-${rl.layer_id}`))
    }
    const moveVectors = () => {
      vectorLayers.forEach((vl) => moveSafe(`pub-vec-fill-${vl.layer_id}`))
      vectorLayers.forEach((vl) => moveSafe(`pub-vec-line-${vl.layer_id}`))
      vectorLayers.forEach((vl) => moveSafe(`pub-vec-circle-${vl.layer_id}`))
    }
    if (rasterOnTop) {
      moveVectors()
      moveRasters()
    } else {
      moveRasters()
      moveVectors()
    }
  }, [rasterOnTop, rasterLayers, vectorLayers])

  const toggleAllRasters = () => {
    const next = !allRastersVisible
    const m = {}
    rasterLayers.forEach((r) => {
      m[r.layer_id] = next
    })
    setRasterVis(m)
  }

  const toggleAllVectors = () => {
    const next = !allVectorsVisible
    const m = {}
    vectorLayers.forEach((v) => {
      m[v.layer_id] = next
    })
    setVectorVis(m)
  }

  const copyText = (key, text) => {
    navigator.clipboard?.writeText(text).catch(() => {})
    setCopiedKey(key)
    setTimeout(() => setCopiedKey(""), 2000)
  }

  if (loading) {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-[#141416] text-slate-200 gap-2 text-sm">
        <Loader2 className="size-5 animate-spin text-[#3ecf8e]" />
        <span>Loading published map…</span>
      </div>
    )
  }

  if (error || !publishedMap) {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-[#141416] text-slate-200 p-6">
        <div className="max-w-md rounded-xl border border-red-500/30 bg-[#1c1c20] p-6 text-center space-y-2">
          <AlertCircle className="size-8 text-red-400 mx-auto" />
          <h1 className="text-base font-bold text-white">Published Map Unavailable</h1>
          <p className="text-xs text-slate-400">{error || "Map not found."}</p>
        </div>
      </div>
    )
  }

  const tokenSuffix = !publishedMap.is_public && token ? `?token=${token}` : ""

  return (
    <div
      className="relative h-screen w-screen overflow-hidden bg-[#141416] text-slate-100"
      style={{ width: "100vw", height: "100vh", position: "relative" }}
    >
      <div
        ref={mapContainerRef}
        className="absolute inset-0"
        style={{ position: "absolute", top: 0, right: 0, bottom: 0, left: 0, width: "100%", height: "100%" }}
      />

      {mapError && (
        <div className="absolute top-3 left-1/2 -translate-x-1/2 z-30 max-w-lg rounded-lg border border-red-500/40 bg-red-950/90 px-3.5 py-2 text-xs text-red-200 shadow-lg flex items-center gap-2">
          <AlertCircle className="size-4 shrink-0 text-red-400" />
          <span className="truncate">Map error: {mapError}</span>
          <button
            type="button"
            onClick={() => setMapError(null)}
            className="ml-1 text-red-300 hover:text-white cursor-pointer"
          >
            <X className="size-3.5" />
          </button>
        </div>
      )}

      {/* Top-Left Layer Visibility Controls Panel */}
      <div className="absolute top-3 left-3 z-20 w-72 max-h-[84vh] flex flex-col rounded-xl border border-[#2e2e34] bg-[#16171b]/95 shadow-2xl backdrop-blur-md overflow-hidden">
        <div className="flex items-center justify-between px-3.5 py-2.5 border-b border-[#27272d] bg-[#1b1d22]">
          <div className="min-w-0">
            <div className="text-xs font-bold text-white truncate">{publishedMap.name}</div>
            <div className="text-[10px] font-mono text-[#3ecf8e] uppercase">
              {publishedMap.mode} map • {publishedMap.is_public ? "Public" : "Restricted"}
            </div>
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setShowUrls((p) => !p)}
              className="p-1.5 rounded hover:bg-white/10 text-slate-300 cursor-pointer"
              title="Tile & Style URLs"
            >
              <Code2 className="size-3.5" />
            </button>
            <button
              type="button"
              onClick={() => setPanelOpen((p) => !p)}
              className="p-1.5 rounded hover:bg-white/10 text-slate-300 cursor-pointer"
              title="Toggle layer panel"
            >
              <Layers className="size-3.5" />
            </button>
          </div>
        </div>

        {panelOpen && (
          <div className="overflow-y-auto p-3 flex flex-col gap-3 text-xs">
            {rasterLayers.length > 0 && vectorLayers.length > 0 && (
              <button
                type="button"
                onClick={() => setRasterOnTop((p) => !p)}
                className="w-full flex items-center justify-between px-2.5 py-1.5 rounded-lg border border-[#2e2e36] bg-[#202229] hover:border-[#3ecf8e] text-[11px] text-slate-200 cursor-pointer transition-colors"
                title="Swap Vector and Raster stacking order"
              >
                <span className="flex items-center gap-1.5 font-semibold">
                  <ArrowUpDown className="size-3.5 text-[#3ecf8e]" />
                  <span>{rasterOnTop ? "Raster Above Vector" : "Vector Above Raster"}</span>
                </span>
                <span className="text-[10px] font-mono text-[#3ecf8e]">Swap</span>
              </button>
            )}
            {/* Vector Layers Group */}
            {vectorLayers.length > 0 && (
              <div className="space-y-1.5" style={{ order: rasterOnTop ? 2 : 1 }}>
                <div className="flex items-center justify-between">
                  <button
                    type="button"
                    onClick={toggleAllVectors}
                    className="flex items-center gap-1.5 font-semibold text-slate-200 hover:text-[#3ecf8e] cursor-pointer"
                  >
                    {allVectorsVisible ? (
                      <Eye className="size-3.5 text-[#3ecf8e]" />
                    ) : (
                      <EyeOff className="size-3.5 text-slate-500" />
                    )}
                    <Shapes className="size-3.5 text-[#3ecf8e]" />
                    <span>Vector Layers ({vectorLayers.length})</span>
                  </button>
                </div>

                {/* Point Features Master Toggle */}
                {pointLayers.length > 0 && (
                  <button
                    type="button"
                    onClick={() => setPointsOnlyVis((p) => !p)}
                    className="w-full flex items-center justify-between px-2 py-1 rounded bg-[#202229] hover:bg-[#282b33] text-[11px] cursor-pointer"
                  >
                    <span className="flex items-center gap-1.5 text-amber-300">
                      <MapPin className="size-3" />
                      Point Markers
                    </span>
                    <span className="text-[10px] font-mono text-slate-400">
                      {pointsOnlyVis ? "Visible" : "Hidden"}
                    </span>
                  </button>
                )}

                <div className="space-y-1 pl-1">
                  {vectorLayers.map((vl) => {
                    const isVis = vectorVis[vl.layer_id] !== false
                    return (
                      <button
                        key={vl.layer_id}
                        type="button"
                        onClick={() =>
                          setVectorVis((prev) => ({ ...prev, [vl.layer_id]: !isVis }))
                        }
                        className="w-full flex items-center justify-between gap-2 px-2 py-1 rounded hover:bg-white/5 text-left cursor-pointer"
                      >
                        <span className="flex items-center gap-2 min-w-0">
                          {isVis ? (
                            <Eye className="size-3 text-[#3ecf8e] shrink-0" />
                          ) : (
                            <EyeOff className="size-3 text-slate-500 shrink-0" />
                          )}
                          <span className="truncate text-slate-200">{vl.name}</span>
                        </span>
                        <span className="text-[9.5px] font-mono text-slate-400 shrink-0">
                          {vl.geometry_type}
                        </span>
                      </button>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Raster Layers Group */}
            {rasterLayers.length > 0 && (
              <div className="space-y-1.5 pt-2 border-t border-[#27272d]" style={{ order: rasterOnTop ? 1 : 2 }}>
                <button
                  type="button"
                  onClick={toggleAllRasters}
                  className="flex items-center gap-1.5 font-semibold text-slate-200 hover:text-sky-400 cursor-pointer"
                >
                  {allRastersVisible ? (
                    <Eye className="size-3.5 text-sky-400" />
                  ) : (
                    <EyeOff className="size-3.5 text-slate-500" />
                  )}
                  <ImageIcon className="size-3.5 text-sky-400" />
                  <span>Raster Layers ({rasterLayers.length})</span>
                </button>
                <div className="space-y-1 pl-1">
                  {rasterLayers.map((rl) => {
                    const isVis = rasterVis[rl.layer_id] !== false
                    return (
                      <button
                        key={rl.layer_id}
                        type="button"
                        onClick={() =>
                          setRasterVis((prev) => ({ ...prev, [rl.layer_id]: !isVis }))
                        }
                        className="w-full flex items-center justify-between gap-2 px-2 py-1 rounded hover:bg-white/5 text-left cursor-pointer"
                      >
                        <span className="flex items-center gap-2 min-w-0">
                          {isVis ? (
                            <Eye className="size-3 text-sky-400 shrink-0" />
                          ) : (
                            <EyeOff className="size-3 text-slate-500 shrink-0" />
                          )}
                          <span className="truncate text-slate-200">{rl.name}</span>
                        </span>
                        <span className="text-[9.5px] font-mono text-slate-400 uppercase shrink-0">
                          {rl.format}
                        </span>
                      </button>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Custom Web Tile & MapLibre Style URLs Drawer */}
            {showUrls && (
              <div className="pt-2 border-t border-[#27272d] space-y-2 text-[10.5px]" style={{ order: 3 }}>
                <div className="font-bold uppercase tracking-wider text-slate-400">
                  Style &amp; Custom Web Tile URLs
                </div>
                <div className="p-2 rounded bg-[#111216] border border-[#2a2a30] space-y-1">
                  <div className="flex items-center justify-between text-slate-400">
                    <span>MapLibre style.json</span>
                    <button
                      type="button"
                      onClick={() => copyText("style", styleUrl)}
                      className="text-[#3ecf8e] hover:underline flex items-center gap-1 cursor-pointer"
                    >
                      {copiedKey === "style" ? <Check className="size-3" /> : <Copy className="size-3" />}
                      Copy
                    </button>
                  </div>
                  <div className="font-mono text-[10px] text-slate-200 break-all">{styleUrl}</div>
                </div>
                {rasterLayers.map((rl) => {
                  const full = `${apiOrigin}${rl.tile_url}${tokenSuffix}`
                  return (
                    <div key={rl.layer_id} className="p-2 rounded bg-[#111216] border border-[#2a2a30] space-y-1">
                      <div className="flex items-center justify-between text-sky-400">
                        <span className="truncate">Raster: {rl.name}</span>
                        <button
                          type="button"
                          onClick={() => copyText("r_" + rl.layer_id, full)}
                          className="text-[#3ecf8e] hover:underline flex items-center gap-1 cursor-pointer shrink-0"
                        >
                          {copiedKey === "r_" + rl.layer_id ? <Check className="size-3" /> : <Copy className="size-3" />}
                          Copy
                        </button>
                      </div>
                      <div className="font-mono text-[10px] text-slate-200 break-all">{full}</div>
                    </div>
                  )
                })}
                {vectorLayers.map((vl) => {
                  const full = `${apiOrigin}${vl.tile_url}${tokenSuffix}`
                  return (
                    <div key={vl.layer_id} className="p-2 rounded bg-[#111216] border border-[#2a2a30] space-y-1">
                      <div className="flex items-center justify-between text-[#3ecf8e]">
                        <span className="truncate">Vector: {vl.name}</span>
                        <button
                          type="button"
                          onClick={() => copyText("v_" + vl.layer_id, full)}
                          className="text-[#3ecf8e] hover:underline flex items-center gap-1 cursor-pointer shrink-0"
                        >
                          {copiedKey === "v_" + vl.layer_id ? <Check className="size-3" /> : <Copy className="size-3" />}
                          Copy
                        </button>
                      </div>
                      <div className="font-mono text-[10px] text-slate-200 break-all">{full}</div>
                      <div className="text-[9.5px] font-mono text-slate-400">
                        source-layer: {vl.source_layer}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Feature Attribute Identification Popup */}
      {identified && (
        <div className="absolute right-4 bottom-6 z-20 w-80 max-h-[55vh] flex flex-col rounded-xl border border-[#3ecf8e]/45 bg-[#16171b]/95 shadow-2xl backdrop-blur-md overflow-hidden">
          <div className="px-3.5 py-2.5 border-b border-[#27272d] bg-[#1b1d22] flex items-center justify-between gap-2">
            <div className="min-w-0">
              <div className="text-xs font-bold text-[#3ecf8e] truncate">{identified.layerName}</div>
              <div className="text-[10px] font-mono text-slate-400">
                {identified.geometryType} • {identified.crs}
                {identified.featureId ? ` • ID: ${identified.featureId}` : ""}
              </div>
              <div className="text-[9.5px] font-mono text-slate-500 truncate">
                source-layer: {identified.sourceLayer}
              </div>
            </div>
            <button
              type="button"
              onClick={() => setIdentified(null)}
              className="p-1 rounded hover:bg-white/10 text-slate-300 cursor-pointer"
              aria-label="Close popup"
            >
              <X className="size-3.5" />
            </button>
          </div>
          <div className="p-3 overflow-y-auto space-y-1.5 text-[11px]">
            <div className="flex justify-between text-[10px] text-slate-400 pb-1 border-b border-[#24262e]">
              <span>Coordinates</span>
              <span className="font-mono text-slate-200">
                {identified.lngLat[1]}, {identified.lngLat[0]}
              </span>
            </div>
            {Object.entries(identified.properties || {})
              .filter(([k, v]) => !k.startsWith("_") && k !== "feature_id" && v !== null && v !== undefined && v !== "")
              .map(([k, v]) => (
                <div key={k} className="flex justify-between gap-3 py-0.5 border-b border-[#20222a]">
                  <span className="text-slate-400 shrink-0">{k}</span>
                  <span className="text-right font-medium text-slate-100 break-all">{String(v)}</span>
                </div>
              ))}
          </div>
        </div>
      )}
    </div>
  )
}
