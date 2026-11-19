import React, { useEffect, useState } from "react"
import { Ruler, Scan, Trash2, Undo2, X } from "lucide-react"
import {
  haversineDistanceMeters,
  polylineDistanceMeters,
  sphericalPolygonAreaSqMeters,
  formatDistance,
  formatArea,
} from "../../lib/workspaceLayout"

const MEASURE_SRC = "ws-measure-source"
const MEASURE_FILL = "ws-measure-fill"
const MEASURE_LINE = "ws-measure-line"
const MEASURE_PTS = "ws-measure-pts"

export default function MapMeasurementOverlay({ map, mode = null, onClose }) {
  const [points, setPoints] = useState([])

  // Clear points when mode switches or closes
  useEffect(() => {
    setPoints([])
  }, [mode])

  // Update MapLibre overlay geometry
  useEffect(() => {
    if (!map || !map.getStyle()) return undefined

    const features = []
    points.forEach((pt, idx) => {
      features.push({
        type: "Feature",
        properties: { idx },
        geometry: { type: "Point", coordinates: pt },
      })
    })
    if (points.length >= 2) {
      const lineCoords = mode === "area" && points.length >= 3 ? [...points, points[0]] : points
      features.push({
        type: "Feature",
        properties: {},
        geometry: { type: "LineString", coordinates: lineCoords },
      })
    }
    if (mode === "area" && points.length >= 3) {
      features.push({
        type: "Feature",
        properties: {},
        geometry: { type: "Polygon", coordinates: [[...points, points[0]]] },
      })
    }

    const fc = { type: "FeatureCollection", features }

    if (!map.getSource(MEASURE_SRC)) {
      map.addSource(MEASURE_SRC, { type: "geojson", data: fc })
      map.addLayer({
        id: MEASURE_FILL,
        type: "fill",
        source: MEASURE_SRC,
        filter: ["==", ["geometry-type"], "Polygon"],
        paint: { "fill-color": "#f59e0b", "fill-opacity": 0.2 },
      })
      map.addLayer({
        id: MEASURE_LINE,
        type: "line",
        source: MEASURE_SRC,
        filter: ["==", ["geometry-type"], "LineString"],
        paint: { "line-color": "#f59e0b", "line-width": 2.5, "line-dasharray": [3, 2] },
      })
      map.addLayer({
        id: MEASURE_PTS,
        type: "circle",
        source: MEASURE_SRC,
        filter: ["==", ["geometry-type"], "Point"],
        paint: {
          "circle-color": "#ffffff",
          "circle-radius": 5,
          "circle-stroke-color": "#f59e0b",
          "circle-stroke-width": 2,
        },
      })
    } else {
      map.getSource(MEASURE_SRC).setData(fc)
    }

    if (points.length > 0) {
      ;[MEASURE_FILL, MEASURE_LINE, MEASURE_PTS].forEach((lid) => {
        if (map.getLayer(lid)) {
          try {
            map.moveLayer(lid)
          } catch {}
        }
      })
    }
  }, [map, points, mode])

  // Click listener while measuring
  useEffect(() => {
    if (!map || !mode) return undefined
    const canvas = map.getCanvas?.()
    if (canvas) canvas.style.cursor = "crosshair"

    const onClick = (e) => {
      if (!e?.lngLat) return
      const pt = [Number(e.lngLat.lng.toFixed(7)), Number(e.lngLat.lat.toFixed(7))]
      setPoints((prev) => [...prev, pt])
    }

    map.on("click", onClick)
    return () => {
      if (canvas) canvas.style.cursor = ""
      map.off("click", onClick)
    }
  }, [map, mode])

  if (!mode) return null

  const totalDist = polylineDistanceMeters(points)
  const lastSeg =
    points.length >= 2 ? haversineDistanceMeters(points[points.length - 2], points[points.length - 1]) : 0
  const areaSqM = mode === "area" ? sphericalPolygonAreaSqMeters(points) : 0
  const perimeterM =
    mode === "area" && points.length >= 3
      ? totalDist + haversineDistanceMeters(points[points.length - 1], points[0])
      : totalDist

  return (
    <div className="absolute top-3 left-1/2 -translate-x-1/2 z-[940] rounded-[8px] border border-amber-500/45 bg-[#191b1f]/95 text-neutral-100 shadow-2xl px-3 py-2 flex items-center gap-3 text-[11px] backdrop-blur-md">
      <div className="flex items-center gap-1.5 font-bold text-amber-400">
        {mode === "area" ? <Scan className="size-3.5" /> : <Ruler className="size-3.5" />}
        <span>{mode === "area" ? "Measure Area" : "Measure Distance"}</span>
      </div>

      <div className="h-3.5 w-px bg-neutral-700" />

      {points.length === 0 ? (
        <span className="text-neutral-300">Click on map to add vertices</span>
      ) : mode === "distance" ? (
        <div className="flex items-center gap-3 font-mono">
          <span>
            Total: <strong className="text-white">{formatDistance(totalDist)}</strong>
          </span>
          {points.length >= 2 && (
            <span className="text-neutral-400">Seg: {formatDistance(lastSeg)}</span>
          )}
        </div>
      ) : (
        <div className="flex items-center gap-3 font-mono">
          <span>
            Area: <strong className="text-white">{formatArea(areaSqM)}</strong>
          </span>
          <span className="text-neutral-400">Perim: {formatDistance(perimeterM)}</span>
        </div>
      )}

      <div className="flex items-center gap-1 ml-1">
        <button
          type="button"
          disabled={points.length === 0}
          onClick={() => setPoints((p) => p.slice(0, -1))}
          className="p-1 rounded hover:bg-neutral-700 disabled:opacity-35 cursor-pointer"
          title="Remove last point"
        >
          <Undo2 className="size-3" />
        </button>
        <button
          type="button"
          disabled={points.length === 0}
          onClick={() => setPoints([])}
          className="p-1 rounded hover:bg-neutral-700 disabled:opacity-35 cursor-pointer"
          title="Clear measurement"
        >
          <Trash2 className="size-3" />
        </button>
        <button
          type="button"
          onClick={() => {
            setPoints([])
            onClose?.()
          }}
          className="p-1 rounded hover:bg-neutral-700 cursor-pointer"
          title="Close measurement tool (Esc)"
        >
          <X className="size-3" />
        </button>
      </div>
    </div>
  )
}
