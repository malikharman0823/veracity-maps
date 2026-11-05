import React, { useEffect, useRef, useState, useMemo } from "react"
import {
  Check,
  Compass,
  CornerDownLeft,
  CornerDownRight,
  Eye,
  Hand,
  Loader2,
  Lock,
  Maximize2,
  Move,
  Redo2,
  RotateCw,
  Sliders,
  Sparkles,
  Undo2,
  Unlock,
  X,
  AlertCircle,
} from "lucide-react"

import {
  applyRasterTransform,
  getRasterTransform,
  previewRasterTransform,
  readableApiError,
  redoRasterTransform,
  resetRasterTransform,
  undoRasterTransform,
} from "../api"

// MapLibre safety helpers
// MapLibre 6.x Style keeps sources in `tileManagers`. After map.remove() or a
// style teardown these can be missing, and getSource()/getLayer() then throw.
function isMapReady(map) {
  return Boolean(
    map
    && !map._removed
    && map.style
    && map.style.tileManagers
    && map.style._layers
  )
}

function safeGetSource(map, id) {
  if (!isMapReady(map)) return null
  try {
    return map.getSource(id) || null
  } catch {
    return null
  }
}

function safeGetLayer(map, id) {
  if (!isMapReady(map)) return null
  try {
    return map.getLayer(id) || null
  } catch {
    return null
  }
}

function safeRemoveLayer(map, id) {
  if (!map || map._removed) return
  try {
    if (map.style && map.style._layers && map.style._layers[id]) {
      map.removeLayer(id)
    }
  } catch {
    // ignore
  }
}

function safeRemoveSource(map, id) {
  if (!map || map._removed) return
  try {
    if (map.style && map.style.tileManagers && map.style.tileManagers[id]) {
      map.removeSource(id)
    }
  } catch {
    // ignore
  }
}

export default function RasterTransformOverlay({
  map,
  datasetId,
  datasetName,
  onClose,
  onApplied,
}) {
  const [loading, setLoading] = useState(true)
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState(null)
  const [mode, setMode] = useState("free") // 'move' | 'resize' | 'rotate' | 'free'
  const [lockAspect, setLockAspect] = useState(false)

  // 4 corners: top_left, top_right, bottom_right, bottom_left
  const [corners, setCorners] = useState(null)
  const [originalCorners, setOriginalCorners] = useState(null)
  const [rotationAngle, setRotationAngle] = useState(0)
  const [canUndo, setCanUndo] = useState(false)
  const [canRedo, setCanRedo] = useState(false)

  // Dragging interaction state
  const dragRef = useRef({
    active: false,
    handleType: null, // 'tl' | 'tr' | 'br' | 'bl' | 'edge-top' | 'edge-right' | 'edge-bottom' | 'edge-left' | 'body' | 'rot'
    startPoint: null, // { x, y, lng, lat }
    startCorners: null,
    startAngle: 0,
    center: null,
  })

  // Load initial status
  useEffect(() => {
    if (!datasetId) return
    let active = true
    setLoading(true)
    setError(null)

    getRasterTransform(datasetId)
      .then((data) => {
        if (!active) return
        if (data.corners) {
          setCorners(data.corners)
        }
        if (data.original_corners) {
          setOriginalCorners(data.original_corners)
        }
        if (typeof data.rotation_angle === "number") {
          setRotationAngle(data.rotation_angle)
        }
        setLockAspect(Boolean(data.lock_aspect_ratio))
        setCanUndo(Boolean(data.can_undo))
        setCanRedo(Boolean(data.can_redo))
        setLoading(false)
      })
      .catch((err) => {
        if (!active) return
        setLoading(false)
        setError(readableApiError(err, "Failed to load raster transformation status."))
      })

    return () => {
      active = false
    }
  }, [datasetId])

  // Stable Map Source & Layer IDs
  const {
    SOURCE_ID,
    LINE_LAYER_ID,
    FILL_LAYER_ID,
    HANDLES_SOURCE_ID,
    HANDLES_LAYER_ID,
    ROT_LINE_SOURCE_ID,
    ROT_LINE_LAYER_ID,
  } = useMemo(() => ({
    SOURCE_ID: `raster-tf-${datasetId}-outline`,
    LINE_LAYER_ID: `raster-tf-${datasetId}-line`,
    FILL_LAYER_ID: `raster-tf-${datasetId}-fill`,
    HANDLES_SOURCE_ID: `raster-tf-${datasetId}-handles`,
    HANDLES_LAYER_ID: `raster-tf-${datasetId}-handles-layer`,
    ROT_LINE_SOURCE_ID: `raster-tf-${datasetId}-rot-line`,
    ROT_LINE_LAYER_ID: `raster-tf-${datasetId}-rot-line-layer`,
  }), [datasetId])

  // Compute centers and midpoints
  const geometryInfo = useMemo(() => {
    if (!corners) return null
    const { top_left: tl, top_right: tr, bottom_right: br, bottom_left: bl } = corners
    const centerLng = (tl.longitude + tr.longitude + br.longitude + bl.longitude) / 4
    const centerLat = (tl.latitude + tr.latitude + br.latitude + bl.latitude) / 4

    const topMid = { longitude: (tl.longitude + tr.longitude) / 2, latitude: (tl.latitude + tr.latitude) / 2 }
    const rightMid = { longitude: (tr.longitude + br.longitude) / 2, latitude: (tr.latitude + br.latitude) / 2 }
    const bottomMid = { longitude: (bl.longitude + br.longitude) / 2, latitude: (bl.latitude + br.latitude) / 2 }
    const leftMid = { longitude: (tl.longitude + bl.longitude) / 2, latitude: (tl.latitude + bl.latitude) / 2 }

    // Rotation handle sticking out from top center
    const dLng = tr.longitude - tl.longitude
    const dLat = tr.latitude - tl.latitude
    const norm = Math.hypot(dLng, dLat) || 1
    // Normal perpendicular vector pointing "outward" (upwards)
    const perpLng = -dLat / norm
    const perpLat = dLng / norm
    const offset = norm * 0.18 // offset distance
    const rotHandle = {
      longitude: topMid.longitude + perpLng * offset,
      latitude: topMid.latitude + perpLat * offset,
    }

    return {
      center: { longitude: centerLng, latitude: centerLat },
      topMid,
      rightMid,
      bottomMid,
      leftMid,
      rotHandle,
    }
  }, [corners])

  // Draw GeoJSON bounding box & handles on MapLibre map
  useEffect(() => {
    if (!isMapReady(map) || !corners || !geometryInfo) return

    const { top_left: tl, top_right: tr, bottom_right: br, bottom_left: bl } = corners
    const { center, topMid, rightMid, bottomMid, leftMid, rotHandle } = geometryInfo

    const polyGeoJson = {
      type: "FeatureCollection",
      features: [
        {
          type: "Feature",
          geometry: {
            type: "Polygon",
            coordinates: [[
              [tl.longitude, tl.latitude],
              [tr.longitude, tr.latitude],
              [br.longitude, br.latitude],
              [bl.longitude, bl.latitude],
              [tl.longitude, tl.latitude],
            ]],
          },
        },
      ],
    }

    const rotLineGeoJson = {
      type: "FeatureCollection",
      features: [
        {
          type: "Feature",
          geometry: {
            type: "LineString",
            coordinates: [
              [topMid.longitude, topMid.latitude],
              [rotHandle.longitude, rotHandle.latitude],
            ],
          },
        },
      ],
    }

    const handlesGeoJson = {
      type: "FeatureCollection",
      features: [
        // Corners
        { type: "Feature", properties: { kind: "corner", handle: "tl" }, geometry: { type: "Point", coordinates: [tl.longitude, tl.latitude] } },
        { type: "Feature", properties: { kind: "corner", handle: "tr" }, geometry: { type: "Point", coordinates: [tr.longitude, tr.latitude] } },
        { type: "Feature", properties: { kind: "corner", handle: "br" }, geometry: { type: "Point", coordinates: [br.longitude, br.latitude] } },
        { type: "Feature", properties: { kind: "corner", handle: "bl" }, geometry: { type: "Point", coordinates: [bl.longitude, bl.latitude] } },
        // Midpoints
        { type: "Feature", properties: { kind: "edge", handle: "edge-top" }, geometry: { type: "Point", coordinates: [topMid.longitude, topMid.latitude] } },
        { type: "Feature", properties: { kind: "edge", handle: "edge-right" }, geometry: { type: "Point", coordinates: [rightMid.longitude, rightMid.latitude] } },
        { type: "Feature", properties: { kind: "edge", handle: "edge-bottom" }, geometry: { type: "Point", coordinates: [bottomMid.longitude, bottomMid.latitude] } },
        { type: "Feature", properties: { kind: "edge", handle: "edge-left" }, geometry: { type: "Point", coordinates: [leftMid.longitude, leftMid.latitude] } },
        // Pivot
        { type: "Feature", properties: { kind: "pivot", handle: "pivot" }, geometry: { type: "Point", coordinates: [center.longitude, center.latitude] } },
        // Rotation
        { type: "Feature", properties: { kind: "rotation", handle: "rot" }, geometry: { type: "Point", coordinates: [rotHandle.longitude, rotHandle.latitude] } },
      ],
    }

    // 1. Polygon outline & fill
    const src = safeGetSource(map, SOURCE_ID)
    if (!src) {
      try {
        map.addSource(SOURCE_ID, { type: "geojson", data: polyGeoJson })
        map.addLayer({
          id: FILL_LAYER_ID,
          type: "fill",
          source: SOURCE_ID,
          paint: {
            "fill-color": "#3ecf8e",
            "fill-opacity": 0.08,
          },
        })
        map.addLayer({
          id: LINE_LAYER_ID,
          type: "line",
          source: SOURCE_ID,
          paint: {
            "line-color": "#3ecf8e",
            "line-width": 2.5,
            "line-dasharray": [2, 1.5],
          },
        })
      } catch {
        // layer/source may already exist
      }
    } else {
      try {
        src.setData(polyGeoJson)
      } catch {
        // ignore
      }
    }

    // 2. Rotation arm line
    const rotLineSrc = safeGetSource(map, ROT_LINE_SOURCE_ID)
    if (!rotLineSrc) {
      try {
        map.addSource(ROT_LINE_SOURCE_ID, { type: "geojson", data: rotLineGeoJson })
        map.addLayer({
          id: ROT_LINE_LAYER_ID,
          type: "line",
          source: ROT_LINE_SOURCE_ID,
          paint: {
            "line-color": "#ff9500",
            "line-width": 2,
          },
        })
      } catch {
        // layer/source may already exist
      }
    } else {
      try {
        rotLineSrc.setData(rotLineGeoJson)
      } catch {
        // ignore
      }
    }

    // 3. Handles circles
    const handlesSrc = safeGetSource(map, HANDLES_SOURCE_ID)
    if (!handlesSrc) {
      try {
        map.addSource(HANDLES_SOURCE_ID, { type: "geojson", data: handlesGeoJson })
        map.addLayer({
          id: HANDLES_LAYER_ID,
          type: "circle",
          source: HANDLES_SOURCE_ID,
          paint: {
            "circle-radius": [
              "match",
              ["get", "kind"],
              "corner", 7,
              "rotation", 7.5,
              "pivot", 5,
              5.5,
            ],
            "circle-color": [
              "match",
              ["get", "kind"],
              "rotation", "#ff9500",
              "pivot", "#ff3b30",
              "#ffffff",
            ],
            "circle-stroke-width": 2.5,
            "circle-stroke-color": [
              "match",
              ["get", "kind"],
              "rotation", "#ffffff",
              "pivot", "#ffffff",
              "#3ecf8e",
            ],
          },
        })
      } catch {
        // layer/source may already exist
      }
    } else {
      try {
        handlesSrc.setData(handlesGeoJson)
      } catch {
        // ignore
      }
    }
  }, [map, corners, geometryInfo, SOURCE_ID, LINE_LAYER_ID, FILL_LAYER_ID, HANDLES_SOURCE_ID, HANDLES_LAYER_ID, ROT_LINE_SOURCE_ID, ROT_LINE_LAYER_ID])

  // Cleanup on unmount or datasetId change
  useEffect(() => {
    return () => {
      safeRemoveLayer(map, HANDLES_LAYER_ID)
      safeRemoveLayer(map, ROT_LINE_LAYER_ID)
      safeRemoveLayer(map, LINE_LAYER_ID)
      safeRemoveLayer(map, FILL_LAYER_ID)

      safeRemoveSource(map, HANDLES_SOURCE_ID)
      safeRemoveSource(map, ROT_LINE_SOURCE_ID)
      safeRemoveSource(map, SOURCE_ID)

      try {
        if (map && map.getCanvas?.()) {
          map.getCanvas().style.cursor = ""
        }
      } catch {
        // ignore
      }
    }
  }, [map, SOURCE_ID, LINE_LAYER_ID, FILL_LAYER_ID, HANDLES_SOURCE_ID, HANDLES_LAYER_ID, ROT_LINE_SOURCE_ID, ROT_LINE_LAYER_ID])

  // Mouse / Pointer Dragging Handlers
  useEffect(() => {
    if (!isMapReady(map)) return

    function onMouseDown(e) {
      if (!corners || !geometryInfo || !isMapReady(map)) return

      // Check if handle was clicked
      const handleFeatures = safeGetLayer(map, HANDLES_LAYER_ID)
        ? map.queryRenderedFeatures(e.point, { layers: [HANDLES_LAYER_ID] })
        : []
      let targetHandle = null

      if (handleFeatures.length > 0) {
        targetHandle = handleFeatures[0].properties.handle
      } else {
        // Check if inside polygon body
        const fillFeatures = safeGetLayer(map, FILL_LAYER_ID)
          ? map.queryRenderedFeatures(e.point, { layers: [FILL_LAYER_ID] })
          : []
        if (fillFeatures.length > 0) {
          targetHandle = "body"
        }
      }

      if (!targetHandle) return

      // Disable map dragging so user drags transform handles smoothly
      try {
        map.dragPan.disable()
      } catch {
        // ignore
      }

      const center = geometryInfo.center
      const centerScreen = map.project([center.longitude, center.latitude])
      const curAngle = Math.atan2(e.point.y - centerScreen.y, e.point.x - centerScreen.x) * (180 / Math.PI)

      dragRef.current = {
        active: true,
        handleType: targetHandle,
        startPoint: { x: e.point.x, y: e.point.y, lng: e.lngLat.lng, lat: e.lngLat.lat },
        startCorners: JSON.parse(JSON.stringify(corners)),
        startAngle: curAngle,
        center: center,
      }
    }

    function onMouseMove(e) {
      if (!isMapReady(map)) return

      if (!dragRef.current.active) {
        // Change cursor icon when hovering handles
        if (safeGetLayer(map, HANDLES_LAYER_ID)) {
          const handleFeatures = map.queryRenderedFeatures(e.point, { layers: [HANDLES_LAYER_ID] })
          if (handleFeatures.length > 0) {
            const kind = handleFeatures[0].properties.kind
            map.getCanvas().style.cursor = kind === "rotation" ? "grab" : "pointer"
            return
          }
        }
        if (safeGetLayer(map, FILL_LAYER_ID)) {
          const fillFeatures = map.queryRenderedFeatures(e.point, { layers: [FILL_LAYER_ID] })
          if (fillFeatures.length > 0) {
            map.getCanvas().style.cursor = "move"
            return
          }
        }
        try {
          if (map.getCanvas?.()) map.getCanvas().style.cursor = ""
        } catch {
          // ignore
        }
        return
      }

      const { handleType, startPoint, startCorners, center } = dragRef.current
      const deltaLng = e.lngLat.lng - startPoint.lng
      const deltaLat = e.lngLat.lat - startPoint.lat

      // 1. Move mode or body drag: translate all 4 corners
      if (mode === "move" || handleType === "body") {
        setCorners({
          top_left: { longitude: startCorners.top_left.longitude + deltaLng, latitude: startCorners.top_left.latitude + deltaLat },
          top_right: { longitude: startCorners.top_right.longitude + deltaLng, latitude: startCorners.top_right.latitude + deltaLat },
          bottom_right: { longitude: startCorners.bottom_right.longitude + deltaLng, latitude: startCorners.bottom_right.latitude + deltaLat },
          bottom_left: { longitude: startCorners.bottom_left.longitude + deltaLng, latitude: startCorners.bottom_left.latitude + deltaLat },
        })
        return
      }

      // 2. Rotate mode or rotation handle drag
      if (mode === "rotate" || handleType === "rot") {
        const centerScreen = map.project([center.longitude, center.latitude])
        const curAngle = Math.atan2(e.point.y - centerScreen.y, e.point.x - centerScreen.x) * (180 / Math.PI)
        const angleDiff = curAngle - dragRef.current.startAngle
        const rad = (angleDiff * Math.PI) / 180

        const cos = Math.cos(rad)
        const sin = Math.sin(rad)

        function rotPoint(pt) {
          const dx = pt.longitude - center.longitude
          const dy = pt.latitude - center.latitude
          return {
            longitude: center.longitude + (dx * cos - dy * sin),
            latitude: center.latitude + (dx * sin + dy * cos),
          }
        }

        setRotationAngle((prev) => Math.round((prev + angleDiff) % 360))
        setCorners({
          top_left: rotPoint(startCorners.top_left),
          top_right: rotPoint(startCorners.top_right),
          bottom_right: rotPoint(startCorners.bottom_right),
          bottom_left: rotPoint(startCorners.bottom_left),
        })
        return
      }

      // 3. Free transform: move single corner independently
      if (mode === "free" && ["tl", "tr", "br", "bl"].includes(handleType)) {
        const next = JSON.parse(JSON.stringify(startCorners))
        const keyMap = { tl: "top_left", tr: "top_right", br: "bottom_right", bl: "bottom_left" }
        const key = keyMap[handleType]
        next[key].longitude += deltaLng
        next[key].latitude += deltaLat
        setCorners(next)
        return
      }

      // 4. Resize mode or corner/edge drag
      if (mode === "resize" || ["tl", "tr", "br", "bl", "edge-top", "edge-right", "edge-bottom", "edge-left"].includes(handleType)) {
        const next = JSON.parse(JSON.stringify(startCorners))

        if (lockAspect && ["tl", "tr", "br", "bl"].includes(handleType)) {
          // Scale relative to opposite corner
          const oppMap = { tl: "bottom_right", tr: "bottom_left", br: "top_left", bl: "top_right" }
          const oppKey = oppMap[handleType]
          const curKey = { tl: "top_left", tr: "top_right", br: "bottom_right", bl: "bottom_left" }[handleType]
          const opp = startCorners[oppKey]
          const curStart = startCorners[curKey]

          const initW = Math.hypot(curStart.longitude - opp.longitude, curStart.latitude - opp.latitude) || 1e-6
          const newW = Math.hypot(curStart.longitude + deltaLng - opp.longitude, curStart.latitude + deltaLat - opp.latitude)
          const scale = newW / initW

          // Apply scale from opposite corner
          for (const k of ["top_left", "top_right", "bottom_right", "bottom_left"]) {
            next[k].longitude = opp.longitude + (startCorners[k].longitude - opp.longitude) * scale
            next[k].latitude = opp.latitude + (startCorners[k].latitude - opp.latitude) * scale
          }
        } else if (["tl", "tr", "br", "bl"].includes(handleType)) {
          // Rotated-aware corner drag: project cursor movement along box axes
          const curKey = { tl: "top_left", tr: "top_right", br: "bottom_right", bl: "bottom_left" }[handleType]
          const adjHMap = { tl: "tr", tr: "tl", br: "bl", bl: "br" }
          const adjVMap = { tl: "bl", tr: "br", br: "tr", bl: "tl" }
          const adjHKey = { tl: "top_right", tr: "top_left", br: "bottom_left", bl: "bottom_right" }[handleType]
          const adjVKey = { tl: "bottom_left", tr: "bottom_right", br: "top_right", bl: "top_left" }[handleType]

          // Horizontal edge vector (unit)
          const hUx = startCorners.top_right.longitude - startCorners.top_left.longitude
          const hUy = startCorners.top_right.latitude - startCorners.top_left.latitude
          const hLen = Math.hypot(hUx, hUy) || 1
          const hNx = hUx / hLen
          const hNy = hUy / hLen

          // Vertical edge vector (unit)
          const vUx = startCorners.bottom_left.longitude - startCorners.top_left.longitude
          const vUy = startCorners.bottom_left.latitude - startCorners.top_left.latitude
          const vLen = Math.hypot(vUx, vUy) || 1
          const vNx = vUx / vLen
          const vNy = vUy / vLen

          // Project delta onto horizontal and vertical axes
          const dotH = deltaLng * hNx + deltaLat * hNy
          const dotV = deltaLng * vNx + deltaLat * vNy

          const deltaH_lng = dotH * hNx
          const deltaH_lat = dotH * hNy
          const deltaV_lng = dotV * vNx
          const deltaV_lat = dotV * vNy

          next[curKey].longitude = startCorners[curKey].longitude + deltaH_lng + deltaV_lng
          next[curKey].latitude = startCorners[curKey].latitude + deltaH_lat + deltaV_lat

          // Move adjacent corners along their respective constraint axes so rectangle remains a rectangle
          next[adjHKey].longitude = startCorners[adjHKey].longitude + deltaV_lng
          next[adjHKey].latitude = startCorners[adjHKey].latitude + deltaV_lat

          next[adjVKey].longitude = startCorners[adjVKey].longitude + deltaH_lng
          next[adjVKey].latitude = startCorners[adjVKey].latitude + deltaH_lat
        } else {
          // Edge handle dragging
          if (handleType === "edge-top" || handleType === "edge-bottom") {
            const keys = handleType === "edge-top" ? ["top_left", "top_right"] : ["bottom_left", "bottom_right"]
            // Project delta perpendicular to top edge
            const eUx = startCorners.top_right.longitude - startCorners.top_left.longitude
            const eUy = startCorners.top_right.latitude - startCorners.top_left.latitude
            const eLen = Math.hypot(eUx, eUy) || 1
            const perpX = -eUy / eLen
            const perpY = eUx / eLen
            const dotPerp = deltaLng * perpX + deltaLat * perpY
            for (const k of keys) {
              next[k].longitude = startCorners[k].longitude + dotPerp * perpX
              next[k].latitude = startCorners[k].latitude + dotPerp * perpY
            }
          } else if (handleType === "edge-left" || handleType === "edge-right") {
            const keys = handleType === "edge-left" ? ["top_left", "bottom_left"] : ["top_right", "bottom_right"]
            const eUx = startCorners.bottom_left.longitude - startCorners.top_left.longitude
            const eUy = startCorners.bottom_left.latitude - startCorners.top_left.latitude
            const eLen = Math.hypot(eUx, eUy) || 1
            const perpX = -eUy / eLen
            const perpY = eUx / eLen
            const dotPerp = deltaLng * perpX + deltaLat * perpY
            for (const k of keys) {
              next[k].longitude = startCorners[k].longitude + dotPerp * perpX
              next[k].latitude = startCorners[k].latitude + dotPerp * perpY
            }
          }
        }
        setCorners(next)
      }
    }

    function onMouseUp() {
      if (dragRef.current.active) {
        dragRef.current.active = false
        map.dragPan.enable()
      }
    }

    try {
      map.on("mousedown", onMouseDown)
      map.on("mousemove", onMouseMove)
      map.on("mouseup", onMouseUp)
    } catch {
      // ignore
    }

    return () => {
      try {
        map.off("mousedown", onMouseDown)
        map.off("mousemove", onMouseMove)
        map.off("mouseup", onMouseUp)
        if (map.dragPan?.enable) map.dragPan.enable()
      } catch {
        // ignore
      }
    }
  }, [map, corners, geometryInfo, mode, lockAspect, HANDLES_LAYER_ID, FILL_LAYER_ID])

  // Action handlers
  async function handleApply() {
    if (!corners || processing) return
    setProcessing(true)
    setError(null)
    try {
      const res = await applyRasterTransform(datasetId, {
        corners,
        mode,
        rotationAngle,
        lockAspectRatio: lockAspect,
      })
      setCanUndo(Boolean(res.can_undo))
      setCanRedo(Boolean(res.can_redo))
      setProcessing(false)
      if (onApplied) onApplied(res)
    } catch (err) {
      setProcessing(false)
      setError(readableApiError(err, "Failed to apply transformation."))
    }
  }

  async function handleUndo() {
    if (!canUndo || processing) return
    setProcessing(true)
    setError(null)
    try {
      const res = await undoRasterTransform(datasetId)
      if (res.corners) setCorners(res.corners)
      setCanUndo(Boolean(res.can_undo))
      setCanRedo(Boolean(res.can_redo))
      setProcessing(false)
      if (onApplied) onApplied(res)
    } catch (err) {
      setProcessing(false)
      setError(readableApiError(err, "Failed to undo."))
    }
  }

  async function handleRedo() {
    if (!canRedo || processing) return
    setProcessing(true)
    setError(null)
    try {
      const res = await redoRasterTransform(datasetId)
      if (res.corners) setCorners(res.corners)
      setCanUndo(Boolean(res.can_undo))
      setCanRedo(Boolean(res.can_redo))
      setProcessing(false)
      if (onApplied) onApplied(res)
    } catch (err) {
      setProcessing(false)
      setError(readableApiError(err, "Failed to redo."))
    }
  }

  async function handleReset() {
    if (processing) return
    if (!window.confirm("Reset raster to its original geographic footprint?")) return
    setProcessing(true)
    setError(null)
    try {
      const res = await resetRasterTransform(datasetId)
      if (res.corners) setCorners(res.corners)
      setCanUndo(false)
      setCanRedo(false)
      setRotationAngle(0)
      setProcessing(false)
      if (onApplied) onApplied(res)
    } catch (err) {
      setProcessing(false)
      setError(readableApiError(err, "Failed to reset."))
    }
  }

  return (
    <aside className="workspace-transform-toolbar bg-white/95 dark:bg-[#202020]/95 backdrop-blur-md border border-neutral-200 dark:border-[#2e2e2e] text-neutral-900 dark:text-white rounded-[12px] shadow-2xl" aria-label="Raster Transformation Controls">
      <div className="workspace-transform-header">
        <div className="workspace-transform-title font-semibold text-neutral-900 dark:text-white">
          <Sparkles className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
          <span>Transform: {datasetName}</span>
        </div>
        <button
          type="button"
          className="workspace-dialog-close text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-white"
          onClick={onClose}
          aria-label="Close transform tool"
        >
          <X className="size-4" />
        </button>
      </div>

      {error && (
        <div className="workspace-dialog-error bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 p-2 rounded-[6px]" style={{ margin: "4px 0" }}>
          <AlertCircle className="size-4" />
          <div>
            <strong>{error.message}</strong>
            <span>{error.details}</span>
          </div>
        </div>
      )}

      {/* Mode Switcher */}
      <div className="workspace-transform-modes bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[8px] p-1">
        <button
          type="button"
          className={`workspace-transform-mode-btn rounded-[6px] transition-colors ${mode === "move" ? "is-active bg-[#3ecf8e] text-[#171717] font-semibold shadow-xs" : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"}`}
          onClick={() => setMode("move")}
          title="Move: Translate entire raster"
        >
          <Move className="size-3.5" />
          <span>Move</span>
        </button>
        <button
          type="button"
          className={`workspace-transform-mode-btn rounded-[6px] transition-colors ${mode === "resize" ? "is-active bg-[#3ecf8e] text-[#171717] font-semibold shadow-xs" : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"}`}
          onClick={() => setMode("resize")}
          title="Resize: Scale corners and edges"
        >
          <Maximize2 className="size-3.5" />
          <span>Resize</span>
        </button>
        <button
          type="button"
          className={`workspace-transform-mode-btn rounded-[6px] transition-colors ${mode === "rotate" ? "is-active bg-[#3ecf8e] text-[#171717] font-semibold shadow-xs" : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"}`}
          onClick={() => setMode("rotate")}
          title="Rotate: Rotate around pivot"
        >
          <RotateCw className="size-3.5" />
          <span>Rotate</span>
        </button>
        <button
          type="button"
          className={`workspace-transform-mode-btn rounded-[6px] transition-colors ${mode === "free" ? "is-active bg-[#3ecf8e] text-[#171717] font-semibold shadow-xs" : "text-neutral-600 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-white"}`}
          onClick={() => setMode("free")}
          title="Free Transform: Move individual corners"
        >
          <Sliders className="size-3.5" />
          <span>Free</span>
        </button>
      </div>

      {/* Options & Metrics */}
      <div className="workspace-transform-options">
        <label className="workspace-transform-lock-label text-neutral-700 dark:text-neutral-300">
          <input
            type="checkbox"
            checked={lockAspect}
            onChange={(e) => setLockAspect(e.target.checked)}
          />
          {lockAspect ? <Lock className="size-3 text-[#24b47e] dark:text-[#3ecf8e]" /> : <Unlock className="size-3 text-neutral-400" />}
          <span>Lock Aspect Ratio</span>
        </label>
        {rotationAngle !== 0 && (
          <span className="workspace-transform-badge bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200">
            {rotationAngle}°
          </span>
        )}
      </div>

      {/* Action Buttons */}
      <div className="workspace-transform-actions border-t border-neutral-200 dark:border-[#2e2e2e] pt-2">
        <div className="workspace-transform-history-btns">
          <button
            type="button"
            className="workspace-secondary-button size-8 p-0 bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 border border-neutral-300 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px]"
            onClick={handleUndo}
            disabled={!canUndo || processing}
            title="Undo applied transformation"
          >
            <Undo2 className="size-3.5" />
          </button>
          <button
            type="button"
            className="workspace-secondary-button size-8 p-0 bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 border border-neutral-300 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px]"
            onClick={handleRedo}
            disabled={!canRedo || processing}
            title="Redo transformation"
          >
            <Redo2 className="size-3.5" />
          </button>
          <button
            type="button"
            className="workspace-secondary-button size-8 p-0 bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 border border-neutral-300 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px]"
            onClick={handleReset}
            disabled={processing}
            title="Reset to original"
          >
            <CornerDownLeft className="size-3.5" />
          </button>
        </div>

        <div className="workspace-transform-commit-btns">
          <button
            type="button"
            className="workspace-secondary-button text-xs py-1.5 px-3 bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 border border-neutral-300 dark:border-[#2e2e2e] hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px]"
            onClick={onClose}
            disabled={processing}
          >
            Cancel
          </button>
          <button
            type="button"
            className="workspace-primary-button text-xs py-1.5 px-3 bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold rounded-[6px] shadow-xs"
            onClick={handleApply}
            disabled={processing || loading}
          >
            {processing ? <Loader2 className="size-3 animate-spin mr-1" /> : <Check className="size-3 mr-1" />}
            <span>Apply</span>
          </button>
        </div>
      </div>
    </aside>
  )
}
