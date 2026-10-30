import React, { useEffect, useRef, useState } from "react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { ArrowLeftRight, Check, MapPin, Search, X } from "lucide-react"
import { Map as MapLibreMap, NavigationControl, setWorkerUrl } from "maplibre-gl"
import mapLibreWorkerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url"

import { BASEMAP_ATTRIBUTION, BASEMAP_TILE_URL } from "../api"
import { parseGoogleMapsCoordinates } from "../lib/coordinateParser"
import MapCoordinates from "./MapCoordinates"

const OVERLAY_SOURCE_ID = "georeference-preview-source"
const OVERLAY_LAYER_ID = "georeference-preview-layer"
const TARGET_PIN_SOURCE_ID = "georeference-target-pin-source"
const TARGET_PIN_OUTER_LAYER_ID = "georeference-target-pin-outer"
const TARGET_PIN_INNER_LAYER_ID = "georeference-target-pin-inner"

const WEB_MERCATOR_MAX_LAT = 85.051129

setWorkerUrl(mapLibreWorkerUrl)

function clampMercatorLat(lat) {
  return Math.max(-WEB_MERCATOR_MAX_LAT, Math.min(WEB_MERCATOR_MAX_LAT, lat))
}

export default function ReferenceMap({
  points,
  draft = null,
  selectedId,
  onSelect,
  onMapClick,
  onMoveEnd,
  overlay,
  focusTarget = null,
  readOnly = false,
  placementMode = false,
}) {
  const containerRef = useRef(null)
  const mapRef = useRef(null)
  const initialFitDoneRef = useRef(false)

  const mapClickRef = useRef(onMapClick)
  const selectRef = useRef(onSelect)
  const moveEndRef = useRef(onMoveEnd)
  const readOnlyRef = useRef(readOnly)
  const placementModeRef = useRef(placementMode)
  const draftRef = useRef(draft)

  const [searchValue, setSearchValue] = useState("")
  const [searchError, setSearchError] = useState(null)
  const [targetPin, setTargetPin] = useState(null)
  const [cursorCoordinate, setCursorCoordinate] = useState(null)

  mapClickRef.current = onMapClick
  selectRef.current = onSelect
  moveEndRef.current = onMoveEnd
  readOnlyRef.current = readOnly
  placementModeRef.current = placementMode
  draftRef.current = draft

  useEffect(() => {
    if (!containerRef.current) return undefined
    const map = new MapLibreMap({
      container: containerRef.current,
      style: {
        version: 8,
        sources: {
          basemap: {
            type: "raster",
            tiles: [BASEMAP_TILE_URL],
            tileSize: 256,
            attribution: BASEMAP_ATTRIBUTION,
            maxzoom: 19,
          },
        },
        layers: [{ id: "basemap", type: "raster", source: "basemap" }],
      },
      center: [0, 20],
      zoom: 1.5,
      minZoom: 0,
      maxZoom: 22,
    })
    map.addControl(new NavigationControl({ showCompass: false }), "bottom-left")
    map.on("click", (event) => {
      if (!readOnlyRef.current && (placementModeRef.current || Boolean(draftRef.current))) {
        const location = event.lngLat.wrap()
        const lat = Math.min(Math.max(location.lat, -90), 90)
        mapClickRef.current({ lng: location.lng, lat })
      }
    })
    map.on("mousemove", (event) => {
      setCursorCoordinate({ latitude: event.lngLat.lat, longitude: event.lngLat.lng })
    })
    map.on("mouseleave", () => setCursorCoordinate(null))
    mapRef.current = map
    return () => {
      mapRef.current = null
      map.remove()
    }
  }, [])

  useEffect(() => {
    const map = mapRef.current
    if (map) {
      map.getCanvas().style.cursor = !readOnly && (placementMode || Boolean(draft)) ? "crosshair" : ""
    }
  }, [placementMode, readOnly, draft])

  // Auto-fit once on initial load when existing control points are present
  useEffect(() => {
    const map = mapRef.current
    if (!map || initialFitDoneRef.current || !Array.isArray(points) || points.length === 0) {
      return undefined
    }

    const fitInitial = () => {
      if (initialFitDoneRef.current) return
      const valid = points.filter(
        (p) =>
          Number.isFinite(Number(p.longitude)) &&
          Number.isFinite(Number(p.latitude)) &&
          Math.abs(Number(p.longitude)) <= 180 &&
          Math.abs(Number(p.latitude)) <= 90,
      )
      if (!valid.length) return
      initialFitDoneRef.current = true

      if (valid.length === 1) {
        map.jumpTo({
          center: [Number(valid[0].longitude), clampMercatorLat(Number(valid[0].latitude))],
          zoom: 17,
        })
        return
      }

      let minLng = Infinity
      let maxLng = -Infinity
      let minLat = Infinity
      let maxLat = -Infinity
      for (const p of valid) {
        const lng = Number(p.longitude)
        const lat = clampMercatorLat(Number(p.latitude))
        if (lng < minLng) minLng = lng
        if (lng > maxLng) maxLng = lng
        if (lat < minLat) minLat = lat
        if (lat > maxLat) maxLat = lat
      }
      if (maxLng - minLng < 1e-6 && maxLat - minLat < 1e-6) {
        map.jumpTo({ center: [minLng, minLat], zoom: 17 })
      } else {
        map.fitBounds(
          [
            [minLng, minLat],
            [maxLng, maxLat],
          ],
          { padding: 70, maxZoom: 18, duration: 0 },
        )
      }
    }

    if (map.isStyleLoaded()) fitInitial()
    else map.once("load", fitInitial)
    return () => map.off("load", fitInitial)
  }, [points])

  // Programmatic camera focus from external coordinate actions
  useEffect(() => {
    const map = mapRef.current
    if (!map || !focusTarget) return undefined

    const applyFocus = () => {
      initialFitDoneRef.current = true
      if (Array.isArray(focusTarget.bounds) && focusTarget.bounds.length === 4) {
        const [west, south, east, north] = focusTarget.bounds.map(Number)
        if ([west, south, east, north].every(Number.isFinite) && east > west && north > south) {
          map.fitBounds(
            [
              [west, clampMercatorLat(south)],
              [east, clampMercatorLat(north)],
            ],
            { padding: 64, maxZoom: 19, essential: true },
          )
          return
        }
      }
      if (Number.isFinite(focusTarget.latitude) && Number.isFinite(focusTarget.longitude)) {
        map.flyTo({
          center: [focusTarget.longitude, clampMercatorLat(focusTarget.latitude)],
          zoom: focusTarget.zoom ?? 17,
          essential: true,
        })
      }
    }

    if (map.isStyleLoaded()) applyFocus()
    else map.once("load", applyFocus)
    return () => map.off("load", applyFocus)
  }, [focusTarget])

  // Render searched target coordinate pin
  useEffect(() => {
    const map = mapRef.current
    if (!map) return undefined

    const syncTargetPin = () => {
      if (!targetPin) {
        if (map.getLayer(TARGET_PIN_INNER_LAYER_ID)) map.removeLayer(TARGET_PIN_INNER_LAYER_ID)
        if (map.getLayer(TARGET_PIN_OUTER_LAYER_ID)) map.removeLayer(TARGET_PIN_OUTER_LAYER_ID)
        if (map.getSource(TARGET_PIN_SOURCE_ID)) map.removeSource(TARGET_PIN_SOURCE_ID)
        return
      }

      const geojson = {
        type: "FeatureCollection",
        features: [
          {
            type: "Feature",
            geometry: {
              type: "Point",
              coordinates: [targetPin.longitude, clampMercatorLat(targetPin.latitude)],
            },
            properties: {},
          },
        ],
      }

      if (map.getSource(TARGET_PIN_SOURCE_ID)) {
        map.getSource(TARGET_PIN_SOURCE_ID).setData(geojson)
      } else {
        map.addSource(TARGET_PIN_SOURCE_ID, { type: "geojson", data: geojson })
        map.addLayer({
          id: TARGET_PIN_OUTER_LAYER_ID,
          type: "circle",
          source: TARGET_PIN_SOURCE_ID,
          paint: {
            "circle-radius": 13,
            "circle-color": "rgba(255, 59, 48, 0.22)",
            "circle-stroke-width": 2,
            "circle-stroke-color": "#ff3b30",
          },
        })
        map.addLayer({
          id: TARGET_PIN_INNER_LAYER_ID,
          type: "circle",
          source: TARGET_PIN_SOURCE_ID,
          paint: {
            "circle-radius": 5,
            "circle-color": "#ff3b30",
            "circle-stroke-width": 2,
            "circle-stroke-color": "#ffffff",
          },
        })
      }
    }

    if (map.isStyleLoaded()) syncTargetPin()
    else map.once("load", syncTargetPin)
    return () => map.off("load", syncTargetPin)
  }, [targetPin])

  useEffect(() => {
    const map = mapRef.current
    if (!map) return undefined

    const renderGeoJSON = () => {
      const geojson = {
        type: "FeatureCollection",
        features: points.map((point, index) => ({
          type: "Feature",
          geometry: { type: "Point", coordinates: [point.longitude, point.latitude] },
          properties: { id: point.id, number: String(index + 1), selected: selectedId === point.id },
        })),
      }

      if (map.getSource("control-points")) {
        map.getSource("control-points").setData(geojson)
      } else {
        map.addSource("control-points", { type: "geojson", data: geojson })
        map.addLayer({
          id: "control-points-circle",
          type: "circle",
          source: "control-points",
          paint: {
            "circle-radius": ["case", ["boolean", ["get", "selected"], false], 12, 10],
            "circle-color": ["case", ["boolean", ["get", "selected"], false], "#0052a3", "#0066cc"],
            "circle-stroke-width": 2,
            "circle-stroke-color": "#ffffff",
          },
        })
        map.addLayer({
          id: "control-points-label",
          type: "symbol",
          source: "control-points",
          layout: {
            "text-field": ["get", "number"],
            "text-font": ["Open Sans Bold", "Arial Unicode MS Bold"],
            "text-size": 11,
            "text-anchor": "center",
            "text-allow-overlap": true,
            "text-ignore-placement": true,
          },
          paint: {
            "text-color": "#ffffff",
          },
        })
      }
    }

    if (map.isStyleLoaded()) renderGeoJSON()
    else map.once("load", renderGeoJSON)

    return () => map.off("load", renderGeoJSON)
  }, [points, readOnly, selectedId])

  useEffect(() => {
    const map = mapRef.current
    if (!map || readOnly) return undefined

    let isDragging = false
    let draggedId = null

    const onMouseDown = (e) => {
      if (e.defaultPrevented || isDragging) return
      const features = map.queryRenderedFeatures(e.point, { layers: ["control-points-circle", "control-points-label"] })
      if (!features.length) return
      e.preventDefault()
      isDragging = true
      draggedId = features[0].properties.id
      selectRef.current(draggedId)
      map.getCanvas().style.cursor = "grabbing"
      map.dragPan.disable()
    }

    const onMouseMove = (e) => {
      if (!isDragging || !draggedId) return
      const source = map.getSource("control-points")
      if (source) {
        const currentData = {
          type: "FeatureCollection",
          features: points.map((p, i) => {
            if (p.id === draggedId) {
              return {
                type: "Feature",
                geometry: { type: "Point", coordinates: [e.lngLat.lng, e.lngLat.lat] },
                properties: { id: p.id, number: String(i + 1), selected: true },
              }
            }
            return {
              type: "Feature",
              geometry: { type: "Point", coordinates: [p.longitude, p.latitude] },
              properties: { id: p.id, number: String(i + 1), selected: selectedId === p.id },
            }
          }),
        }
        source.setData(currentData)
      }
    }

    const onMouseUp = (e) => {
      if (!isDragging || !draggedId) return
      isDragging = false
      map.getCanvas().style.cursor = ""
      map.dragPan.enable()
      const location = e.lngLat.wrap()
      const lat = Math.min(Math.max(location.lat, -90), 90)
      moveEndRef.current(draggedId, { longitude: location.lng, latitude: lat })
      draggedId = null
    }

    map.on("mousedown", onMouseDown)
    map.on("mousemove", onMouseMove)
    map.on("mouseup", onMouseUp)

    return () => {
      map.off("mousedown", onMouseDown)
      map.off("mousemove", onMouseMove)
      map.off("mouseup", onMouseUp)
    }
  }, [points, readOnly, selectedId])

  useEffect(() => {
    const map = mapRef.current
    if (!map) return undefined
    const syncOverlay = () => {
      const source = map.getSource(OVERLAY_SOURCE_ID)
      if (!overlay?.visible || !overlay.coordinates || !overlay.imageUrl) {
        if (map.getLayer(OVERLAY_LAYER_ID)) map.removeLayer(OVERLAY_LAYER_ID)
        if (source) map.removeSource(OVERLAY_SOURCE_ID)
        return
      }

      const beforeId = map.getLayer("control-points-circle") ? "control-points-circle" : undefined

      if (source) {
        try {
          if (typeof source.updateImage === "function") {
            source.updateImage({
              url: overlay.imageUrl,
              coordinates: overlay.coordinates,
            })
          } else {
            throw new Error("updateImage not available")
          }
        } catch {
          if (map.getLayer(OVERLAY_LAYER_ID)) map.removeLayer(OVERLAY_LAYER_ID)
          if (map.getSource(OVERLAY_SOURCE_ID)) map.removeSource(OVERLAY_SOURCE_ID)
          map.addSource(OVERLAY_SOURCE_ID, {
            type: "image",
            url: overlay.imageUrl,
            coordinates: overlay.coordinates,
          })
          map.addLayer(
            {
              id: OVERLAY_LAYER_ID,
              type: "raster",
              source: OVERLAY_SOURCE_ID,
              paint: { "raster-opacity": overlay.opacity },
            },
            beforeId,
          )
        }
      } else {
        map.addSource(OVERLAY_SOURCE_ID, {
          type: "image",
          url: overlay.imageUrl,
          coordinates: overlay.coordinates,
        })
        map.addLayer(
          {
            id: OVERLAY_LAYER_ID,
            type: "raster",
            source: OVERLAY_SOURCE_ID,
            paint: { "raster-opacity": overlay.opacity },
          },
          beforeId,
        )
      }
      if (map.getLayer(OVERLAY_LAYER_ID)) {
        map.setPaintProperty(OVERLAY_LAYER_ID, "raster-opacity", overlay.opacity)
      }
    }
    if (map.isStyleLoaded()) syncOverlay()
    else map.once("load", syncOverlay)
    return () => map.off("load", syncOverlay)
  }, [overlay])

  function submitSearch(event) {
    event.preventDefault()
    const parsed = parseGoogleMapsCoordinates(searchValue, { order: "lat-lng" })
    if (!parsed.ok) {
      setSearchError(parsed.error)
      return
    }
    setSearchError(null)
    initialFitDoneRef.current = true
    const zoom = parsed.zoom ?? 17
    setTargetPin({
      latitude: parsed.latitude,
      longitude: parsed.longitude,
      ambiguous: Boolean(parsed.ambiguous),
      autoSwapped: Boolean(parsed.autoSwapped),
    })
    mapRef.current?.flyTo({
      center: [parsed.longitude, clampMercatorLat(parsed.latitude)],
      zoom,
      essential: true,
    })
  }

  function handleSwapPinAxes() {
    if (!targetPin) return
    const nextLat = targetPin.longitude
    const nextLng = targetPin.latitude
    if (Math.abs(nextLat) > 90 || Math.abs(nextLng) > 180) return
    setTargetPin({
      latitude: nextLat,
      longitude: nextLng,
      ambiguous: true,
      autoSwapped: false,
    })
    setSearchValue(`${nextLat.toFixed(6)}, ${nextLng.toFixed(6)}`)
    mapRef.current?.flyTo({
      center: [nextLng, clampMercatorLat(nextLat)],
      zoom: Math.max(mapRef.current?.getZoom?.() || 17, 16),
      essential: true,
    })
  }

  function handlePairTargetPin() {
    if (!targetPin || readOnly) return
    mapClickRef.current({ lng: targetPin.longitude, lat: targetPin.latitude })
  }

  return (
    <section className="relative flex min-h-[360px] min-w-0 flex-1 flex-col overflow-hidden rounded-utility border border-hairline bg-white">
      <header className="flex min-h-14 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-hairline px-4 py-2">
        <div>
          <h2 className="text-[13px] font-semibold tracking-[0.08em] text-ink">REFERENCE MAP</h2>
          <p className="mt-0.5 text-[12px] text-muted">
            {readOnly ? "Inspect saved geographic references" : "Search Google Maps Lat, Lng or click to pair"}
          </p>
        </div>
        <form className="flex items-center gap-1.5" onSubmit={submitSearch}>
          <div className="relative">
            <Input
              className="w-52 rounded-full pl-3.5 pr-8 text-[12px] phone:w-64"
              value={searchValue}
              placeholder="Lat, Lng (e.g. 28.6139, 77.2090)"
              aria-label="Search Google Maps latitude and longitude"
              onChange={(event) => {
                setSearchValue(event.target.value)
                if (searchError) setSearchError(null)
              }}
            />
            {searchValue && (
              <button
                type="button"
                aria-label="Clear coordinate search"
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[#7a7a7a] hover:text-[#1d1d1f]"
                onClick={() => {
                  setSearchValue("")
                  setSearchError(null)
                }}
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>
          <Button
            type="submit"
            variant="outline"
            size="sm"
            className="h-8 shrink-0 rounded-full border-[#e0e0e0] px-3 text-[12px]"
            aria-label="Go to coordinates"
          >
            <Search className="mr-1 size-3.5" />
            Go
          </Button>
        </form>
      </header>

      <div ref={containerRef} className="min-h-0 flex-1" aria-label="Interactive geographic reference map" />
      <MapCoordinates coordinate={cursorCoordinate} />

      {searchError && (
        <div
          role="alert"
          className="absolute right-3 top-[62px] z-20 max-w-xs rounded-xl border border-[#e9b4b4] bg-[#fff6f6] px-3 py-2 text-[12px] text-[#b42318] shadow-sm"
        >
          {searchError}
        </div>
      )}

      {targetPin && !searchError && (
        <div className="absolute left-3 right-3 top-[62px] z-20 flex flex-wrap items-center justify-between gap-2 rounded-xl border border-[#e0e0e0] bg-white/95 px-3 py-2 text-[12px] shadow-sm backdrop-blur-xs">
          <div className="flex items-center gap-1.5 font-mono text-[11px] text-[#1d1d1f]">
            <MapPin className="size-3.5 shrink-0 text-[#ff3b30]" />
            <span>
              Lat <strong>{targetPin.latitude.toFixed(6)}</strong>, Lng{" "}
              <strong>{targetPin.longitude.toFixed(6)}</strong>
            </span>
          </div>
          <div className="flex items-center gap-1.5">
            {!readOnly && draft && (
              <Button
                type="button"
                size="sm"
                className="h-7 rounded-full bg-[#0066cc] px-2.5 text-[11px] font-medium text-white hover:bg-[#0071e3]"
                onClick={handlePairTargetPin}
              >
                <Check className="mr-1 size-3" />
                Pair Point #{points.length + 1} Here
              </Button>
            )}
            {targetPin.ambiguous && (
              <button
                type="button"
                title="Swap Latitude and Longitude"
                className="inline-flex h-7 items-center gap-1 rounded-full border border-[#e0e0e0] bg-white px-2.5 text-[11px] text-[#555] hover:bg-[#f5f5f7] hover:text-[#1d1d1f]"
                onClick={handleSwapPinAxes}
              >
                <ArrowLeftRight className="size-3" />
                Swap Lat/Lng
              </button>
            )}
            <button
              type="button"
              aria-label="Dismiss target pin"
              className="inline-flex size-6 items-center justify-center rounded-full text-[#7a7a7a] hover:bg-[#f5f5f7] hover:text-[#1d1d1f]"
              onClick={() => setTargetPin(null)}
            >
              <X className="size-3.5" />
            </button>
          </div>
        </div>
      )}
    </section>
  )
}
