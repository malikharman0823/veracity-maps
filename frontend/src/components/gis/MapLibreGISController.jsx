import React, { useEffect, useRef, useState } from "react"
import VectorEditingToolbar from "./VectorEditingToolbar"
import DraggableFloatingBar from "./DraggableFloatingBar"
import {
  getLayerFeatures,
  createLayerFeature,
  updateVectorFeature,
  deleteVectorFeature,
  splitVectorFeature,
  mergeVectorFeatures,
  validateVectorGeometry,
  snapVectorGeometry,
  datasetSourceImageUrl,
} from "../../api"
import {
  isClosedGeometryType,
  minVerticesForType,
  extractEditableVertices,
  buildGeometryFromVertices,
  computeEdgeMidpoints,
  updateVertexAt,
  scaleVerticesFromVertex,
  insertVertexAtEdge,
  removeVertexAt,
} from "../../lib/vertexEditGeometry"

const DRAFT_SOURCE = "gis-draft-edit-source"
const DRAFT_LINE_LAYER = "gis-draft-edit-line"
const DRAFT_FILL_LAYER = "gis-draft-edit-fill"
const DRAFT_MIDPOINT_LAYER = "gis-draft-edit-midpoints"
const DRAFT_VERTEX_LAYER = "gis-draft-edit-vertices"

export default function MapLibreGISController({
  map,
  gisLayers = [],
  activeLayer,
  activeTool: externalTool,
  onSelectTool,
  snapEnabled: externalSnap,
  onToggleSnap,
  selectedFeatureIds = [],
  onSelectFeatures,
  featuresRefreshKey = 0,
  onFeaturesMutated,
  onControllerActionsReady,
  onOpenInRecords,
  notify,
}) {
  const [internalTool, setInternalTool] = useState("select")
  const [internalSnap, setInternalSnap] = useState(false)
  const activeTool = externalTool ?? internalTool
  const setActiveTool = onSelectTool ?? setInternalTool
  const snapEnabled = externalSnap ?? internalSnap
  const setSnapEnabled = onToggleSnap ?? setInternalSnap

  const [draftCoords, setDraftCoords] = useState([])
  const [editingFeature, setEditingFeature] = useState(null)
  const [validationResult, setValidationResult] = useState(null)
  const [undoStack, setUndoStack] = useState([])
  const [redoStack, setRedoStack] = useState([])
  const [featuresByLayer, setFeaturesByLayer] = useState({})
  const [hoveredFeature, setHoveredFeature] = useState(null)
  const [identifiedFeature, setIdentifiedFeature] = useState(null)
  const [styleRevision, setStyleRevision] = useState(0)

  const activeToolRef = useRef(activeTool)
  const activeLayerRef = useRef(activeLayer)
  const gisLayersRef = useRef(gisLayers)
  const snapEnabledRef = useRef(snapEnabled)
  const draftCoordsRef = useRef(draftCoords)
  const editingFeatureRef = useRef(editingFeature)
  const selectedIdsRef = useRef(selectedFeatureIds)

  useEffect(() => {
    activeToolRef.current = activeTool
    activeLayerRef.current = activeLayer
    gisLayersRef.current = gisLayers
    snapEnabledRef.current = snapEnabled
    draftCoordsRef.current = draftCoords
    editingFeatureRef.current = editingFeature
    selectedIdsRef.current = selectedFeatureIds
  }, [activeTool, activeLayer, gisLayers, snapEnabled, draftCoords, editingFeature, selectedFeatureIds])

  // Re-sync GIS layers whenever MapLibre finishes initial load or switches base layer style
  useEffect(() => {
    if (!map) return
    const bump = () => setStyleRevision((r) => r + 1)
    map.on("load", bump)
    map.on("style.load", bump)
    if (map.isStyleLoaded()) {
      bump()
    }
    return () => {
      map.off("load", bump)
      map.off("style.load", bump)
    }
  }, [map])

  // Load features in parallel for all vector GIS layers so point layers appear immediately
  useEffect(() => {
    let active = true
    const loadAll = async () => {
      const vectorLayers = gisLayers.filter((l) => l.layer_type === "vector" || !l.layer_type)
      const entries = await Promise.all(
        vectorLayers.map(async (layer) => {
          try {
            const fc = await getLayerFeatures(layer.layer_id)
            return [layer.layer_id, fc || { type: "FeatureCollection", features: [] }]
          } catch {
            return [layer.layer_id, { type: "FeatureCollection", features: [] }]
          }
        })
      )
      if (active) {
        setFeaturesByLayer(Object.fromEntries(entries))
      }
    }
    loadAll()
    return () => {
      active = false
    }
  }, [gisLayers, featuresRefreshKey])

  // Synchronize GIS layers, styles, visibility, opacity, zoom ranges, and selection highlight with MapLibre
  useEffect(() => {
    if (!map || !map.isStyleLoaded()) return

    gisLayers.forEach((layer) => {
      // Handle Raster GIS Layers (sync existing WorkspaceMap raster layer or render standalone GIS raster)
      if (layer.layer_type === "raster") {
        const vis = layer.is_visible ? "visible" : "none"
        const op = typeof layer.opacity === "number" ? layer.opacity : 1.0
        const minZ = typeof layer.min_zoom === "number" ? layer.min_zoom : 0
        const maxZ = typeof layer.max_zoom === "number" ? layer.max_zoom : 24

        if (layer.dataset_id) {
          const wsRasterId = `workspace-${layer.dataset_id}-raster-layer`
          if (map.getLayer(wsRasterId)) {
            map.setLayoutProperty(wsRasterId, "visibility", vis)
            map.setPaintProperty(wsRasterId, "raster-opacity", op)
            return
          }
        }

        if (layer.dataset_id && Array.isArray(layer.bounds) && layer.bounds.length === 4) {
          const [west, south, east, north] = layer.bounds.map(Number)
          if ([west, south, east, north].every(Number.isFinite)) {
            const rSrcId = `gis-raster-src-${layer.layer_id}`
            const rLyrId = `gis-raster-lyr-${layer.layer_id}`
            if (!map.getSource(rSrcId)) {
              map.addSource(rSrcId, {
                type: "image",
                url: datasetSourceImageUrl(layer.dataset_id),
                coordinates: [
                  [west, north],
                  [east, north],
                  [east, south],
                  [west, south],
                ],
              })
            }
            if (!map.getLayer(rLyrId)) {
              map.addLayer({
                id: rLyrId,
                type: "raster",
                source: rSrcId,
                minzoom: minZ,
                maxzoom: maxZ,
                layout: { visibility: vis },
                paint: { "raster-opacity": op },
              })
            } else {
              map.setLayoutProperty(rLyrId, "visibility", vis)
              map.setLayerZoomRange(rLyrId, minZ, maxZ)
              map.setPaintProperty(rLyrId, "raster-opacity", op)
            }
          }
        }
        return
      }

      if (layer.layer_type && layer.layer_type !== "vector") return
      const srcId = `gis-src-${layer.layer_id}`
      const fillId = `gis-fill-${layer.layer_id}`
      const lineId = `gis-line-${layer.layer_id}`
      const circleId = `gis-circle-${layer.layer_id}`
      const labelId = `gis-label-${layer.layer_id}`
      const highlightId = `gis-sel-${layer.layer_id}`

      const fc = featuresByLayer[layer.layer_id] || { type: "FeatureCollection", features: [] }

      if (!map.getSource(srcId)) {
        map.addSource(srcId, { type: "geojson", data: fc })
      } else {
        map.getSource(srcId).setData(fc)
      }

      const mlStyle = layer.style?.maplibre_style || {}
      const vis = layer.is_visible !== false ? "visible" : "none"
      const op = typeof layer.opacity === "number" ? layer.opacity : 1.0
      const minZ = typeof layer.min_zoom === "number" ? layer.min_zoom : 0
      const maxZ = typeof layer.max_zoom === "number" ? layer.max_zoom : 24

      const fillPaint = {
        "fill-color": mlStyle.fill?.["fill-color"] || "#3ecf8e",
        "fill-opacity": (mlStyle.fill?.["fill-opacity"] ?? 0.4) * op,
        "fill-outline-color": mlStyle.fill?.["fill-outline-color"] || "#24b47e",
      }
      const linePaint = {
        "line-color": mlStyle.line?.["line-color"] || "#24b47e",
        "line-width": mlStyle.line?.["line-width"] ?? 2,
        "line-opacity": (mlStyle.line?.["line-opacity"] ?? 0.9) * op,
      }
      const circleColor =
        mlStyle.circle?.["circle-color"] ||
        layer.style?.style_config?.point_color ||
        layer.style?.style_config?.fill_color ||
        layer.rendering_source?.color ||
        "#ef4444"
      const rawRadius = mlStyle.circle?.["circle-radius"]
      const baseRadius = typeof rawRadius === "number" && Number.isFinite(rawRadius) ? rawRadius : 7
      const circleRadius = Array.isArray(rawRadius)
        ? rawRadius
        : [
            "interpolate",
            ["linear"],
            ["zoom"],
            3, Math.max(3.5, Number((baseRadius * 0.55).toFixed(2))),
            8, Math.max(5, Number((baseRadius * 0.8).toFixed(2))),
            12, baseRadius,
            16, Math.min(14, Number((baseRadius * 1.35).toFixed(2))),
          ]
      const circlePaint = {
        "circle-color": circleColor,
        "circle-radius": circleRadius,
        "circle-stroke-color": mlStyle.circle?.["circle-stroke-color"] || "#ffffff",
        "circle-stroke-width": mlStyle.circle?.["circle-stroke-width"] ?? 2,
        "circle-opacity": (mlStyle.circle?.["circle-opacity"] ?? 0.95) * op,
      }

      if (!map.getLayer(fillId)) {
        map.addLayer({
          id: fillId,
          type: "fill",
          source: srcId,
          minzoom: minZ,
          maxzoom: maxZ,
          filter: ["in", ["geometry-type"], ["literal", ["Polygon", "MultiPolygon"]]],
          layout: { visibility: vis },
          paint: fillPaint,
        })
      } else {
        map.setLayoutProperty(fillId, "visibility", vis)
        map.setLayerZoomRange(fillId, minZ, maxZ)
        Object.entries(fillPaint).forEach(([k, v]) => map.setPaintProperty(fillId, k, v))
      }

      if (!map.getLayer(lineId)) {
        map.addLayer({
          id: lineId,
          type: "line",
          source: srcId,
          minzoom: minZ,
          maxzoom: maxZ,
          filter: ["in", ["geometry-type"], ["literal", ["LineString", "MultiLineString", "Polygon", "MultiPolygon"]]],
          layout: { visibility: vis },
          paint: linePaint,
        })
      } else {
        map.setLayoutProperty(lineId, "visibility", vis)
        map.setLayerZoomRange(lineId, minZ, maxZ)
        Object.entries(linePaint).forEach(([k, v]) => map.setPaintProperty(lineId, k, v))
      }

      if (!map.getLayer(circleId)) {
        map.addLayer({
          id: circleId,
          type: "circle",
          source: srcId,
          minzoom: minZ,
          maxzoom: maxZ,
          filter: ["in", ["geometry-type"], ["literal", ["Point", "MultiPoint"]]],
          layout: { visibility: vis },
          paint: circlePaint,
        })
      } else {
        map.setLayoutProperty(circleId, "visibility", vis)
        map.setLayerZoomRange(circleId, minZ, maxZ)
        Object.entries(circlePaint).forEach(([k, v]) => map.setPaintProperty(circleId, k, v))
      }

      // Highlight selected features layer
      const selFilter = ["in", ["get", "feature_id"], ["literal", selectedFeatureIds.length > 0 ? selectedFeatureIds : ["__none__"]]]
      if (!map.getLayer(highlightId)) {
        map.addLayer({
          id: highlightId,
          type: "line",
          source: srcId,
          filter: selFilter,
          layout: { visibility: vis },
          paint: {
            "line-color": "#facc15",
            "line-width": 4,
          },
        })
      } else {
        map.setFilter(highlightId, selFilter)
        map.setLayoutProperty(highlightId, "visibility", vis)
      }

      // Data-driven label layer
      const labelCfg = mlStyle.label
      if (labelCfg?.enabled && labelCfg.field) {
        const labelLayout = {
          visibility: vis,
          "text-field": ["to-string", ["get", labelCfg.field]],
          "text-size": labelCfg.text_size || 12,
          "text-offset": [0, 1.1],
        }
        const labelPaint = {
          "text-color": labelCfg.text_color || "#ffffff",
          "text-halo-color": labelCfg.halo_color || "#171717",
          "text-halo-width": labelCfg.halo_width || 1.5,
        }
        if (!map.getLayer(labelId)) {
          map.addLayer({
            id: labelId,
            type: "symbol",
            source: srcId,
            minzoom: minZ,
            maxzoom: maxZ,
            layout: labelLayout,
            paint: labelPaint,
          })
        } else {
          Object.entries(labelLayout).forEach(([k, v]) => map.setLayoutProperty(labelId, k, v))
          Object.entries(labelPaint).forEach(([k, v]) => map.setPaintProperty(labelId, k, v))
        }
      } else if (map.getLayer(labelId)) {
        map.setLayoutProperty(labelId, "visibility", "none")
      }
    })

    // Remove stale MapLibre layers for deleted GIS layers so deleting one layer preserves all sibling layers cleanly
    const activeIds = new Set(gisLayers.map((l) => String(l.layer_id)))
    const styleLayers = map.getStyle()?.layers || []
    styleLayers.forEach((sl) => {
      const m = /^gis-(?:fill|line|circle|label|sel|raster-lyr)-(.+)$/.exec(sl.id)
      if (m && !activeIds.has(m[1])) {
        try {
          if (map.getLayer(sl.id)) map.removeLayer(sl.id)
        } catch {}
      }
    })

    // Enforce strict Viewscape layer rendering hierarchy:
    //   Base Layer (bottom) -> Raster Layers (middle) -> Vector Layers (top: Polygons -> Lines -> Points -> Labels -> Highlights -> Draft)
    const orderedLayers = [...gisLayers].sort((a, b) => (a.display_order ?? 0) - (b.display_order ?? 0))
    const safeMoveToTop = (lid) => {
      if (map.getLayer(lid)) {
        try {
          map.moveLayer(lid)
        } catch {}
      }
    }
    orderedLayers.forEach((l) => {
      if (l.layer_type === "raster") {
        safeMoveToTop(`gis-raster-lyr-${l.layer_id}`)
      }
    })
    orderedLayers.forEach((l) => {
      if (!l.layer_type || l.layer_type === "vector") safeMoveToTop(`gis-fill-${l.layer_id}`)
    })
    orderedLayers.forEach((l) => {
      if (!l.layer_type || l.layer_type === "vector") safeMoveToTop(`gis-line-${l.layer_id}`)
    })
    orderedLayers.forEach((l) => {
      if (!l.layer_type || l.layer_type === "vector") safeMoveToTop(`gis-circle-${l.layer_id}`)
    })
    orderedLayers.forEach((l) => {
      if (!l.layer_type || l.layer_type === "vector") safeMoveToTop(`gis-label-${l.layer_id}`)
    })
    orderedLayers.forEach((l) => {
      if (!l.layer_type || l.layer_type === "vector") safeMoveToTop(`gis-sel-${l.layer_id}`)
    })
    ;[DRAFT_FILL_LAYER, DRAFT_LINE_LAYER, DRAFT_MIDPOINT_LAYER, DRAFT_VERTEX_LAYER].forEach(safeMoveToTop)
  }, [map, gisLayers, featuresByLayer, selectedFeatureIds, styleRevision])

  // Helper to build GeoJSON FeatureCollection for draft/vertex overlay
  const buildDraftFeatureCollection = (coords, tool, geomType) => {
    const features = []
    if (!coords || coords.length === 0) {
      return { type: "FeatureCollection", features }
    }
    const isClosed = tool === "polygon" || (tool === "vertex" && isClosedGeometryType(geomType))
    coords.forEach((pt, idx) => {
      features.push({
        type: "Feature",
        properties: { role: "vertex", vertex_index: idx },
        geometry: { type: "Point", coordinates: pt },
      })
    })
    if (tool === "vertex" && coords.length >= 2) {
      computeEdgeMidpoints(coords, isClosed).forEach((m) => {
        features.push({
          type: "Feature",
          properties: { role: "midpoint", edge_index: m.edgeIndex },
          geometry: { type: "Point", coordinates: m.coordinates },
        })
      })
    }
    if (coords.length >= 2) {
      const lineCoords = isClosed && coords.length >= 3 ? [...coords, coords[0]] : coords
      features.push({
        type: "Feature",
        properties: {},
        geometry: { type: "LineString", coordinates: lineCoords },
      })
    }
    if (isClosed && coords.length >= 3) {
      features.push({
        type: "Feature",
        properties: {},
        geometry: { type: "Polygon", coordinates: [[...coords, coords[0]]] },
      })
    }
    return { type: "FeatureCollection", features }
  }

  // Render Draft Drawing / Vertex Editor Overlay in MapLibre
  useEffect(() => {
    if (!map || !map.getStyle()) return

    const fc = buildDraftFeatureCollection(draftCoords, activeTool, editingFeature?.geometry?.type)
    if (!map.getSource(DRAFT_SOURCE)) {
      map.addSource(DRAFT_SOURCE, { type: "geojson", data: fc })
      map.addLayer({
        id: DRAFT_FILL_LAYER,
        type: "fill",
        source: DRAFT_SOURCE,
        filter: ["==", ["geometry-type"], "Polygon"],
        paint: { "fill-color": "#3ecf8e", "fill-opacity": 0.2 },
      })
      map.addLayer({
        id: DRAFT_LINE_LAYER,
        type: "line",
        source: DRAFT_SOURCE,
        filter: ["==", ["geometry-type"], "LineString"],
        paint: { "line-color": "#3ecf8e", "line-width": 2.5, "line-dasharray": [2, 2] },
      })
      map.addLayer({
        id: DRAFT_MIDPOINT_LAYER,
        type: "circle",
        source: DRAFT_SOURCE,
        filter: ["all", ["==", ["geometry-type"], "Point"], ["==", ["get", "role"], "midpoint"]],
        paint: {
          "circle-color": "#3ecf8e",
          "circle-opacity": 0.85,
          "circle-radius": 4.5,
          "circle-stroke-color": "#111827",
          "circle-stroke-width": 1.5,
        },
      })
      map.addLayer({
        id: DRAFT_VERTEX_LAYER,
        type: "circle",
        source: DRAFT_SOURCE,
        filter: ["all", ["==", ["geometry-type"], "Point"], ["==", ["get", "role"], "vertex"]],
        paint: {
          "circle-color": "#ffffff",
          "circle-radius": 7,
          "circle-stroke-color": "#3ecf8e",
          "circle-stroke-width": 2.5,
        },
      })
    } else {
      map.getSource(DRAFT_SOURCE).setData(fc)
    }

    if (draftCoords.length > 0) {
      ;[DRAFT_FILL_LAYER, DRAFT_LINE_LAYER, DRAFT_MIDPOINT_LAYER, DRAFT_VERTEX_LAYER].forEach((lid) => {
        if (map.getLayer(lid)) {
          try {
            map.moveLayer(lid)
          } catch {}
        }
      })
    }
  }, [map, draftCoords, activeTool, editingFeature, featuresByLayer])

  // Interactive Vertex Drag-to-Resize & Reshape when activeTool === "vertex"
  useEffect(() => {
    if (!map || activeTool !== "vertex" || activeLayer?.layer_type !== "vector") return

    let dragState = null
    const canvas = map.getCanvas?.()

    const queryHandleAt = (point) => {
      const box = [
        [point.x - 10, point.y - 10],
        [point.x + 10, point.y + 10],
      ]
      if (map.getLayer(DRAFT_VERTEX_LAYER)) {
        const vHits = map.queryRenderedFeatures(box, { layers: [DRAFT_VERTEX_LAYER] })
        if (vHits.length > 0) {
          return { kind: "vertex", index: Number(vHits[0].properties?.vertex_index) }
        }
      }
      if (map.getLayer(DRAFT_MIDPOINT_LAYER)) {
        const mHits = map.queryRenderedFeatures(box, { layers: [DRAFT_MIDPOINT_LAYER] })
        if (mHits.length > 0) {
          return { kind: "midpoint", edgeIndex: Number(mHits[0].properties?.edge_index) }
        }
      }
      return null
    }

    const updateLiveGeometry = (layerId, featureId, nextVertices, origGeom) => {
      const nextGeom = buildGeometryFromVertices(origGeom, nextVertices)
      if (!nextGeom) return null

      const draftSrc = map.getSource(DRAFT_SOURCE)
      if (draftSrc) {
        draftSrc.setData(buildDraftFeatureCollection(nextVertices, "vertex", origGeom.type))
      }

      const src = map.getSource(`gis-src-${layerId}`)
      const layerFC = featuresByLayer[layerId]
      if (src && layerFC) {
        src.setData({
          ...layerFC,
          features: layerFC.features.map((f) =>
            f.properties?.feature_id === featureId ? { ...f, geometry: nextGeom } : f
          ),
        })
      }
      return nextGeom
    }

    const handleMouseDown = async (event) => {
      const layer = activeLayerRef.current
      if (!layer || layer.layer_type !== "vector" || layer.is_locked) return

      const hit = queryHandleAt(event.point)
      const feat = editingFeatureRef.current
      if (hit && feat?.geometry) {
        const fid = feat.properties?.feature_id || feat.id
        const currentVerts = draftCoordsRef.current

        // Alt+Click on a vertex removes it (if above minimum count)
        if (hit.kind === "vertex" && event.originalEvent?.altKey) {
          event.preventDefault()
          const minCount = minVerticesForType(feat.geometry.type)
          const reduced = removeVertexAt(currentVerts, hit.index, minCount)
          if (!reduced) {
            notify?.(`Cannot remove vertex: ${feat.geometry.type} requires at least ${minCount} vertices`, "error")
            return
          }
          const nextGeom = updateLiveGeometry(layer.layer_id, fid, reduced, feat.geometry)
          if (nextGeom) {
            setDraftCoords(reduced)
            setEditingFeature({ ...feat, geometry: nextGeom })
            try {
              await updateVectorFeature(fid, { geometry: nextGeom })
              setUndoStack((s) => [...s, { type: "update", featureId: fid, before: feat.geometry, after: nextGeom }])
              notify?.("Removed vertex")
              onFeaturesMutated?.()
            } catch (err) {
              notify?.(err?.response?.data?.error?.message || "Failed removing vertex", "error")
              onFeaturesMutated?.()
            }
          }
          return
        }

        event.preventDefault()
        map.dragPan?.disable()
        if (canvas) canvas.style.cursor = "grabbing"

        if (hit.kind === "midpoint") {
          const pt = [Number(event.lngLat.lng.toFixed(7)), Number(event.lngLat.lat.toFixed(7))]
          const inserted = insertVertexAtEdge(currentVerts, hit.edgeIndex, pt)
          const newIdx = hit.edgeIndex + 1
          const nextGeom = updateLiveGeometry(layer.layer_id, fid, inserted, feat.geometry)
          dragState = {
            layerId: layer.layer_id,
            featureId: fid,
            origGeom: feat.geometry,
            origVertices: inserted,
            currentBaseVertices: inserted,
            vertexIndex: newIdx,
            latestVertices: inserted,
            latestGeom: nextGeom,
            isInserted: true,
            usedShiftScale: false,
            moved: true,
          }
          return
        }

        dragState = {
          layerId: layer.layer_id,
          featureId: fid,
          origGeom: feat.geometry,
          origVertices: currentVerts,
          currentBaseVertices: currentVerts,
          vertexIndex: hit.index,
          latestVertices: currentVerts,
          latestGeom: feat.geometry,
          isInserted: false,
          usedShiftScale: false,
          moved: false,
        }
        return
      }

      // Clicking another feature in the active layer switches which feature's vertices are being edited
      const queryLayers = [
        `gis-fill-${layer.layer_id}`,
        `gis-line-${layer.layer_id}`,
        `gis-circle-${layer.layer_id}`,
      ].filter((id) => map.getLayer(id))
      if (queryLayers.length > 0) {
        const rendered = map.queryRenderedFeatures(event.point, { layers: queryLayers })
        if (rendered.length > 0 && rendered[0].properties?.feature_id) {
          const clickedFid = rendered[0].properties.feature_id
          if (!selectedIdsRef.current.includes(clickedFid)) {
            onSelectFeatures?.([clickedFid])
          }
        }
      }
    }

    const handleMouseMove = (event) => {
      if (!dragState) {
        const hit = queryHandleAt(event.point)
        if (canvas) {
          canvas.style.cursor = hit ? "grab" : "default"
        }
        return
      }

      const nextPt = [Number(event.lngLat.lng.toFixed(7)), Number(event.lngLat.lat.toFixed(7))]
      const shiftResize = Boolean(event.originalEvent?.shiftKey) && !dragState.isInserted && dragState.origVertices.length >= 2
      dragState.usedShiftScale = shiftResize
      const nextVertices = shiftResize
        ? scaleVerticesFromVertex(dragState.origVertices, dragState.vertexIndex, nextPt)
        : updateVertexAt(dragState.currentBaseVertices, dragState.vertexIndex, nextPt)

      const nextGeom = updateLiveGeometry(dragState.layerId, dragState.featureId, nextVertices, dragState.origGeom)
      if (nextGeom) {
        dragState.moved = true
        dragState.latestVertices = nextVertices
        dragState.latestGeom = nextGeom
      }
    }

    const handleMouseUp = async () => {
      if (!dragState) return
      const finished = dragState
      dragState = null
      map.dragPan?.enable()
      if (canvas) canvas.style.cursor = "grab"

      if (!finished.moved || !finished.latestGeom) return

      setDraftCoords(finished.latestVertices)
      setEditingFeature((prev) => (prev ? { ...prev, geometry: finished.latestGeom } : prev))

      try {
        const layer = activeLayerRef.current
        const v = await validateVectorGeometry(finished.latestGeom, layer?.crs || "EPSG:4326").catch(() => null)
        setValidationResult(v)
        if (v && !v.is_valid) {
          notify?.(`Invalid geometry: ${v.reason || "Check self-intersections"}`, "error")
          onFeaturesMutated?.()
          return
        }
        await updateVectorFeature(finished.featureId, { geometry: finished.latestGeom })
        setUndoStack((s) => [
          ...s,
          { type: "update", featureId: finished.featureId, before: finished.origGeom, after: finished.latestGeom },
        ])
        notify?.(
          finished.usedShiftScale
            ? "Proportionally resized layer geometry"
            : finished.isInserted
              ? "Added & positioned vertex"
              : "Resized layer geometry using vertex"
        )
        onFeaturesMutated?.()
      } catch (err) {
        notify?.(err?.response?.data?.error?.message || "Failed updating vertex", "error")
        onFeaturesMutated?.()
      }
    }

    map.on("mousedown", handleMouseDown)
    map.on("mousemove", handleMouseMove)
    map.on("mouseup", handleMouseUp)
    return () => {
      map.dragPan?.enable()
      if (canvas) canvas.style.cursor = ""
      map.off("mousedown", handleMouseDown)
      map.off("mousemove", handleMouseMove)
      map.off("mouseup", handleMouseUp)
    }
  }, [map, activeTool, activeLayer, featuresByLayer])

  // Disable map dragPan and set cursor when Move tool is active so dragging moves geometry instead of panning the map
  useEffect(() => {
    if (!map) return
    const canvas = map.getCanvas?.()
    if (activeTool === "move" && activeLayer?.layer_type === "vector") {
      map.dragPan?.disable()
      if (canvas) canvas.style.cursor = "move"
    } else {
      map.dragPan?.enable()
      if (canvas) {
        canvas.style.cursor =
          activeTool === "select" || activeTool === "vertex"
            ? ""
            : activeTool === "identify"
              ? "help"
              : "crosshair"
      }
    }
    return () => {
      map.dragPan?.enable()
      if (canvas) canvas.style.cursor = ""
    }
  }, [map, activeTool, activeLayer])

  // Drag-to-Move interaction for geometric features when activeTool === "move"
  useEffect(() => {
    if (!map || activeTool !== "move" || activeLayer?.layer_type !== "vector") return

    let dragState = null

    const shiftGeometry = (geom, dx, dy) => {
      if (!geom) return geom
      if (geom.type === "Point") {
        return { type: "Point", coordinates: [geom.coordinates[0] + dx, geom.coordinates[1] + dy] }
      }
      if (geom.type === "LineString") {
        return {
          type: "LineString",
          coordinates: geom.coordinates.map((c) => [c[0] + dx, c[1] + dy]),
        }
      }
      if (geom.type === "Polygon") {
        return {
          type: "Polygon",
          coordinates: geom.coordinates.map((ring) => ring.map((c) => [c[0] + dx, c[1] + dy])),
        }
      }
      if (geom.type === "MultiPolygon") {
        return {
          type: "MultiPolygon",
          coordinates: geom.coordinates.map((poly) =>
            poly.map((ring) => ring.map((c) => [c[0] + dx, c[1] + dy]))
          ),
        }
      }
      return geom
    }

    const handleMouseDown = (event) => {
      const layer = activeLayerRef.current
      if (!layer || layer.layer_type !== "vector" || layer.is_locked) return

      const queryLayers = [
        `gis-fill-${layer.layer_id}`,
        `gis-line-${layer.layer_id}`,
        `gis-circle-${layer.layer_id}`,
      ].filter((id) => map.getLayer(id))

      let fid = selectedIdsRef.current[0] || null
      if (queryLayers.length > 0) {
        const rendered = map.queryRenderedFeatures(event.point, { layers: queryLayers })
        if (rendered.length > 0 && rendered[0].properties?.feature_id) {
          fid = rendered[0].properties.feature_id
          if (!selectedIdsRef.current.includes(fid)) {
            onSelectFeatures?.([fid])
          }
        }
      }

      if (!fid) return
      const layerFC = featuresByLayer[layer.layer_id]
      const feat = layerFC?.features?.find((f) => f.properties?.feature_id === fid)
      if (!feat) return

      event.preventDefault()
      dragState = {
        layerId: layer.layer_id,
        featureId: fid,
        startLng: event.lngLat.lng,
        startLat: event.lngLat.lat,
        origGeom: feat.geometry,
        movedGeom: feat.geometry,
        moved: false,
      }
    }

    const handleMouseMove = (event) => {
      if (!dragState) return
      const dx = event.lngLat.lng - dragState.startLng
      const dy = event.lngLat.lat - dragState.startLat
      if (Math.abs(dx) > 1e-8 || Math.abs(dy) > 1e-8) {
        dragState.moved = true
      }
      dragState.movedGeom = shiftGeometry(dragState.origGeom, dx, dy)

      // Live preview on MapLibre GeoJSON source
      const srcId = `gis-src-${dragState.layerId}`
      const src = map.getSource(srcId)
      const layerFC = featuresByLayer[dragState.layerId]
      if (src && layerFC) {
        src.setData({
          ...layerFC,
          features: layerFC.features.map((f) =>
            f.properties?.feature_id === dragState.featureId
              ? { ...f, geometry: dragState.movedGeom }
              : f
          ),
        })
      }
    }

    const handleMouseUp = async () => {
      if (!dragState) return
      const finished = dragState
      dragState = null
      if (!finished.moved) return

      try {
        await updateVectorFeature(finished.featureId, { geometry: finished.movedGeom })
        setUndoStack((s) => [
          ...s,
          { type: "update", featureId: finished.featureId, before: finished.origGeom, after: finished.movedGeom },
        ])
        notify?.("Moved geometric feature")
        onFeaturesMutated?.()
      } catch (err) {
        notify?.(err?.response?.data?.error?.message || "Move failed", "error")
        onFeaturesMutated?.()
      }
    }

    map.on("mousedown", handleMouseDown)
    map.on("mousemove", handleMouseMove)
    map.on("mouseup", handleMouseUp)
    return () => {
      map.off("mousedown", handleMouseDown)
      map.off("mousemove", handleMouseMove)
      map.off("mouseup", handleMouseUp)
    }
  }, [map, activeTool, activeLayer, featuresByLayer])

  // Map Click Listener for Vector Drawing, Selection, Move, Vertex Editing, & Splitting
  useEffect(() => {
    if (!map) return

    const handleMapClick = async (event) => {
      const layer = activeLayerRef.current
      const tool = activeToolRef.current

      // Identify Tool works across ALL visible vector layers regardless of activeLayer
      if (tool === "identify") {
        const allLayers = gisLayersRef.current || []
        const visibleVectors = allLayers.filter(
          (l) => l.layer_type === "vector" && l.is_visible !== false && l.visible !== false
        )
        const queryLayers = [
          ...visibleVectors.map((l) => `gis-circle-${l.layer_id}`),
          ...visibleVectors.map((l) => `gis-line-${l.layer_id}`),
          ...visibleVectors.map((l) => `gis-fill-${l.layer_id}`),
        ].filter((id) => map.getLayer(id))
        if (queryLayers.length === 0) {
          setIdentifiedFeature(null)
          return
        }
        const rendered = map.queryRenderedFeatures(event.point, { layers: queryLayers })
        if (rendered.length > 0) {
          const top = rendered[0]
          const rawLayerId = String(top.layer?.id || "").replace(/^gis-(?:fill|line|circle)-/, "")
          const matchedLayer = allLayers.find((l) => String(l.layer_id) === rawLayerId)
          setIdentifiedFeature({
            layerId: rawLayerId,
            layerName: matchedLayer?.layer_name || "Vector Layer",
            crs: matchedLayer?.crs || "EPSG:4326",
            featureId: top.properties?.feature_id || top.id || "",
            geometryType: top.geometry?.type || matchedLayer?.geometry_type || "Feature",
            lngLat: [Number(event.lngLat.lng.toFixed(6)), Number(event.lngLat.lat.toFixed(6))],
            properties: top.properties || {},
          })
          if (top.properties?.feature_id) {
            onSelectFeatures?.([top.properties.feature_id])
          }
        } else {
          setIdentifiedFeature(null)
        }
        return
      }

      if (!layer || layer.layer_type !== "vector") return

      let pt = [Number(event.lngLat.lng.toFixed(7)), Number(event.lngLat.lat.toFixed(7))]

      // Apply vertex snapping if enabled
      if (snapEnabledRef.current && tool !== "select") {
        try {
          const snapped = await snapVectorGeometry({
            geometry: { type: "Point", coordinates: pt },
            target_layer_id: layer.layer_id,
            tolerance: 0.002,
          })
          if (snapped?.snapped && Array.isArray(snapped.geometry?.coordinates)) {
            pt = snapped.geometry.coordinates
          }
        } catch {}
      }

      if (tool === "select") {
        const queryLayers = [
          `gis-fill-${layer.layer_id}`,
          `gis-line-${layer.layer_id}`,
          `gis-circle-${layer.layer_id}`,
        ].filter((id) => map.getLayer(id))
        if (queryLayers.length === 0) return
        const rendered = map.queryRenderedFeatures(event.point, { layers: queryLayers })
        if (rendered.length > 0) {
          const fid = rendered[0].properties?.feature_id
          if (fid) {
            const multi = event.originalEvent?.shiftKey
            if (multi) {
              const cur = selectedIdsRef.current
              const next = cur.includes(fid) ? cur.filter((x) => x !== fid) : [...cur, fid]
              onSelectFeatures?.(next)
            } else {
              onSelectFeatures?.([fid])
            }
          }
        }
        return
      }

      if (tool === "point") {
        setDraftCoords([pt])
        setValidationResult({ is_valid: true, geometry_type: "Point" })
        return
      }

      if (tool === "line" || tool === "polygon" || tool === "split") {
        const nextCoords = [...draftCoordsRef.current, pt]
        setDraftCoords(nextCoords)
        if ((tool === "line" || tool === "split") && nextCoords.length >= 2) {
          const v = await validateVectorGeometry(
            { type: "LineString", coordinates: nextCoords },
            layer.crs || "EPSG:4326"
          ).catch(() => null)
          setValidationResult(v)
        } else if (tool === "polygon" && nextCoords.length >= 3) {
          const v = await validateVectorGeometry(
            { type: "Polygon", coordinates: [[...nextCoords, nextCoords[0]]] },
            layer.crs || "EPSG:4326"
          ).catch(() => null)
          setValidationResult(v)
        }
        return
      }

      if (tool === "move" && selectedIdsRef.current.length === 1) {
        const fid = selectedIdsRef.current[0]
        const layerFC = featuresByLayer[layer.layer_id]
        const feat = layerFC?.features?.find((f) => f.properties?.feature_id === fid)
        if (!feat) return
        const prevGeom = feat.geometry
        let nextGeom = null
        if (prevGeom.type === "Point") {
          nextGeom = { type: "Point", coordinates: pt }
        } else if (prevGeom.type === "LineString") {
          const c0 = prevGeom.coordinates[0]
          const dx = pt[0] - c0[0]
          const dy = pt[1] - c0[1]
          nextGeom = {
            type: "LineString",
            coordinates: prevGeom.coordinates.map((c) => [c[0] + dx, c[1] + dy]),
          }
        } else if (prevGeom.type === "Polygon") {
          const c0 = prevGeom.coordinates[0][0]
          const dx = pt[0] - c0[0]
          const dy = pt[1] - c0[1]
          nextGeom = {
            type: "Polygon",
            coordinates: prevGeom.coordinates.map((ring) => ring.map((c) => [c[0] + dx, c[1] + dy])),
          }
        }
        if (nextGeom) {
          try {
            await updateVectorFeature(fid, { geometry: nextGeom })
            setUndoStack((s) => [...s, { type: "update", featureId: fid, before: prevGeom, after: nextGeom }])
            notify?.("Moved feature")
            onFeaturesMutated?.()
          } catch (err) {
            notify?.(err?.response?.data?.error?.message || "Move failed", "error")
          }
        }
      }
    }

    map.on("click", handleMapClick)
    return () => {
      map.off("click", handleMapClick)
    }
  }, [map, featuresByLayer])

  // Load selected feature vertices when entering Vertex Editor mode (auto-selects first feature if none selected)
  useEffect(() => {
    if (activeTool !== "vertex" || !activeLayer || activeLayer.layer_type !== "vector") return
    const layerFC = featuresByLayer[activeLayer.layer_id]
    const feats = layerFC?.features || []
    if (feats.length === 0) return

    let targetFid = selectedFeatureIds[0]
    if (!targetFid) {
      targetFid = feats[0]?.properties?.feature_id
      if (targetFid) {
        onSelectFeatures?.([targetFid])
      }
    }
    const feat = feats.find((f) => f.properties?.feature_id === targetFid)
    if (feat?.geometry) {
      setEditingFeature(feat)
      setDraftCoords(extractEditableVertices(feat.geometry))
    }
  }, [activeTool, selectedFeatureIds, activeLayer, featuresByLayer])

  const handleConfirmSaveDraft = async () => {
    if (!activeLayer || draftCoords.length === 0) return

    try {
      if (activeTool === "split") {
        if (selectedFeatureIds.length !== 1 || draftCoords.length < 2) {
          notify?.("Select 1 polygon feature and draw a 2+ point split line across it", "error")
          return
        }
        const splitGeom = { type: "LineString", coordinates: draftCoords }
        await splitVectorFeature(selectedFeatureIds[0], splitGeom)
        notify?.("Polygon split into 2 features")
        setDraftCoords([])
        setActiveTool("select")
        onFeaturesMutated?.()
        return
      }

      let geometry = null
      if (activeTool === "vertex" && editingFeature?.geometry) {
        geometry = buildGeometryFromVertices(editingFeature.geometry, draftCoords)
        if (!geometry) {
          notify?.(`Insufficient vertices for ${editingFeature.geometry.type}`, "error")
          return
        }
      } else if (activeTool === "point") {
        geometry = { type: "Point", coordinates: draftCoords[0] }
      } else if (activeTool === "line") {
        geometry = { type: "LineString", coordinates: draftCoords }
      } else if (activeTool === "polygon") {
        if (draftCoords.length < 3) {
          notify?.("Polygon requires at least 3 vertices", "error")
          return
        }
        geometry = { type: "Polygon", coordinates: [[...draftCoords, draftCoords[0]]] }
      }

      if (!geometry) return

      const vRes = await validateVectorGeometry(geometry, activeLayer.crs || "EPSG:4326")
      if (!vRes?.is_valid) {
        setValidationResult(vRes)
        notify?.(`Invalid geometry: ${vRes?.reason || "check coordinates"}`, "error")
        return
      }

      if (activeTool === "vertex" && editingFeature) {
        const fid = editingFeature.properties?.feature_id || editingFeature.id
        const prevGeom = editingFeature.geometry
        await updateVectorFeature(fid, { geometry })
        setUndoStack((s) => [...s, { type: "update", featureId: fid, before: prevGeom, after: geometry }])
        notify?.("Updated feature vertices")
      } else {
        const created = await createLayerFeature(activeLayer.layer_id, {
          geometry,
          crs: activeLayer.crs || "EPSG:4326",
          properties: {
            name: `${geometry.type} ${new Date().toLocaleTimeString()}`,
          },
        })
        setUndoStack((s) => [...s, { type: "create", featureId: created.feature_id, feature: created }])
        notify?.(`Created ${geometry.type} feature`)
      }

      setDraftCoords([])
      setEditingFeature(null)
      setValidationResult(null)
      if (activeTool === "vertex") {
        setActiveTool("select")
      }
      onFeaturesMutated?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed saving feature", "error")
    }
  }

  const handleDeleteSelected = async () => {
    if (selectedFeatureIds.length === 0) return
    try {
      for (const fid of selectedFeatureIds) {
        await deleteVectorFeature(fid)
      }
      notify?.(`Deleted ${selectedFeatureIds.length} feature(s)`)
      onSelectFeatures?.([])
      onFeaturesMutated?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed deleting feature", "error")
    }
  }

  const handleDuplicateSelected = async () => {
    if (!activeLayer || selectedFeatureIds.length === 0) return
    const layerFC = featuresByLayer[activeLayer.layer_id]
    try {
      for (const fid of selectedFeatureIds) {
        const feat = layerFC?.features?.find((f) => f.properties?.feature_id === fid)
        if (!feat) continue
        await createLayerFeature(activeLayer.layer_id, {
          geometry: feat.geometry,
          crs: activeLayer.crs || "EPSG:4326",
          properties: {
            ...feat.properties,
            name: `${feat.properties?.name || "Feature"} (Copy)`,
          },
        })
      }
      notify?.("Duplicated selected feature(s)")
      onFeaturesMutated?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Duplicate failed", "error")
    }
  }

  const handleMergeSelected = async () => {
    if (selectedFeatureIds.length < 2) return
    try {
      await mergeVectorFeatures({
        feature_ids: selectedFeatureIds,
        output_name: "Merged Feature",
      })
      notify?.(`Merged ${selectedFeatureIds.length} features`)
      onSelectFeatures?.([])
      onFeaturesMutated?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Merge failed", "error")
    }
  }

  const handleUndo = async () => {
    if (undoStack.length === 0) return
    const last = undoStack[undoStack.length - 1]
    setUndoStack((s) => s.slice(0, -1))
    try {
      if (last.type === "create") {
        await deleteVectorFeature(last.featureId)
        setRedoStack((r) => [...r, last])
      } else if (last.type === "update") {
        await updateVectorFeature(last.featureId, { geometry: last.before })
        setRedoStack((r) => [...r, last])
      }
      notify?.("Undid last vector edit")
      onFeaturesMutated?.()
    } catch {}
  }

  const handleRedo = async () => {
    if (redoStack.length === 0) return
    const next = redoStack[redoStack.length - 1]
    setRedoStack((r) => r.slice(0, -1))
    try {
      if (next.type === "create" && activeLayer) {
        const created = await createLayerFeature(activeLayer.layer_id, {
          geometry: next.feature.geometry,
          properties: next.feature.properties,
        })
        setUndoStack((u) => [...u, { ...next, featureId: created.feature_id }])
      } else if (next.type === "update") {
        await updateVectorFeature(next.featureId, { geometry: next.after })
        setUndoStack((u) => [...u, next])
      }
      notify?.("Redid vector edit")
      onFeaturesMutated?.()
    } catch {}
  }

  useEffect(() => {
    onControllerActionsReady?.({
      canUndo: undoStack.length > 0,
      canRedo: redoStack.length > 0,
      undo: handleUndo,
      redo: handleRedo,
      deleteSelected: handleDeleteSelected,
      clearDraft: () => {
        setDraftCoords([])
        setEditingFeature(null)
        setValidationResult(null)
        setIdentifiedFeature(null)
      },
    })
  }, [undoStack.length, redoStack.length, selectedFeatureIds])

  useEffect(() => {
    if (!map) return

    const layerNameById = new Map(
      (gisLayers || []).map((l) => [l.layer_id, l.layer_name])
    )

    const handleMouseMove = (e) => {
      const polygonLayerIds = (gisLayers || [])
        .filter((l) => l.layer_type === "vector" && l.is_visible !== false && l.visible !== false)
        .map((l) => `gis-fill-${l.layer_id}`)
        .filter((id) => Boolean(map.getLayer(id)))

      if (polygonLayerIds.length === 0) {
        setHoveredFeature((prev) => (prev ? null : prev))
        return
      }

      let hits = []
      try {
        hits = map.queryRenderedFeatures(e.point, { layers: polygonLayerIds })
      } catch {
        hits = []
      }

      if (!hits || hits.length === 0) {
        setHoveredFeature((prev) => (prev ? null : prev))
        if (activeToolRef.current === "select") {
          map.getCanvas().style.cursor = ""
        }
        return
      }

      const top = hits[0]
      const rawLayerId = String(top.layer?.id || "").replace(/^gis-fill-/, "")
      const props = top.properties || {}
      if (activeToolRef.current === "select") {
        map.getCanvas().style.cursor = "pointer"
      }
      setHoveredFeature({
        x: e.point.x,
        y: e.point.y,
        layerName: layerNameById.get(rawLayerId) || "Polygon Layer",
        geometryType: top.geometry?.type || "Polygon",
        properties: props,
      })
    }

    const handleMouseLeave = () => {
      setHoveredFeature(null)
    }

    map.on("mousemove", handleMouseMove)
    map.on("mouseleave", handleMouseLeave)
    return () => {
      map.off("mousemove", handleMouseMove)
      map.off("mouseleave", handleMouseLeave)
    }
  }, [map, gisLayers])

  const formatMetric = (val) => {
    const num = Number(val)
    if (!Number.isFinite(num)) return String(val)
    if (Math.abs(num) >= 1000) return num.toLocaleString(undefined, { maximumFractionDigits: 2 })
    if (Math.abs(num) >= 1) return num.toFixed(4).replace(/\.?0+$/, "")
    return num.toFixed(6).replace(/\.?0+$/, "")
  }

  const renderHoverOverlay = () => {
    if (!hoveredFeature) return null
    const props = hoveredFeature.properties || {}
    const metricDefs = [
      { key: "geom_area", label: "Area" },
      { key: "geom_length", label: "Length / Perim." },
      { key: "centroid_x", label: "Centroid X" },
      { key: "centroid_y", label: "Centroid Y" },
      { key: "buffer_distance", label: "Buffer Dist." },
      { key: "join_count", label: "Join Count" },
      { key: "dissolved_count", label: "Dissolved" },
    ]
    const activeMetrics = metricDefs.filter(
      (m) => props[m.key] !== undefined && props[m.key] !== null && props[m.key] !== ""
    )
    const metricKeys = new Set(metricDefs.map((m) => m.key))
    const extraAttrs = Object.entries(props)
      .filter(
        ([k, v]) =>
          !k.startsWith("_") &&
          !metricKeys.has(k) &&
          v !== undefined &&
          v !== null &&
          v !== ""
      )
      .slice(0, 6)

    return (
      <div
        className="pointer-events-none absolute z-[950] w-64 rounded-lg border border-emerald-500/40 bg-[#181a1f]/95 p-2.5 text-[11px] text-slate-200 shadow-2xl backdrop-blur-md"
        style={{
          left: Math.max(12, hoveredFeature.x + 16),
          top: Math.max(12, hoveredFeature.y - 12),
        }}
      >
        <div className="mb-1.5 flex items-center justify-between gap-2 border-b border-slate-700/70 pb-1">
          <span className="truncate font-semibold text-emerald-300">
            {hoveredFeature.layerName}
          </span>
          <span className="shrink-0 rounded bg-emerald-500/20 px-1.5 py-0.5 text-[9px] font-medium text-emerald-300">
            {hoveredFeature.geometryType}
          </span>
        </div>

        {activeMetrics.length > 0 && (
          <div className="mb-1.5 grid grid-cols-2 gap-1 rounded bg-slate-900/80 p-1.5">
            {activeMetrics.map((m) => (
              <div key={m.key} className="min-w-0">
                <div className="text-[9px] uppercase tracking-wide text-slate-400">
                  {m.label}
                </div>
                <div className="truncate font-mono text-[11px] font-semibold text-white">
                  {formatMetric(props[m.key])}
                </div>
              </div>
            ))}
          </div>
        )}

        {extraAttrs.length > 0 ? (
          <div className="space-y-0.5">
            {extraAttrs.map(([k, v]) => (
              <div key={k} className="flex items-center justify-between gap-2">
                <span className="truncate text-slate-400">{k}</span>
                <span className="truncate font-medium text-slate-100">{String(v)}</span>
              </div>
            ))}
          </div>
        ) : activeMetrics.length === 0 ? (
          <div className="text-[10px] text-slate-400">
            Polygon feature (run Calculate Area &amp; Length to inspect metrics)
          </div>
        ) : null}
      </div>
    )
  }

  const renderIdentifyCard = () => {
    if (!identifiedFeature) return null
    const entries = Object.entries(identifiedFeature.properties || {}).filter(
      ([k, v]) => !k.startsWith("_") && k !== "feature_id" && v !== undefined && v !== null && v !== ""
    )
    return (
      <div
        className="absolute right-4 bottom-12 z-[960] w-80 max-h-[60vh] flex flex-col rounded-[12px] border border-[#3ecf8e]/45 bg-[#16181d]/95 text-slate-100 shadow-2xl backdrop-blur-md overflow-hidden"
        role="region"
        aria-label="Identified Feature Details"
      >
        <div className="px-3.5 py-2.5 border-b border-slate-700/80 flex items-center justify-between gap-2 bg-[#1c2028]">
          <div className="min-w-0">
            <div className="text-xs font-bold text-[#3ecf8e] truncate">
              {identifiedFeature.layerName}
            </div>
            <div className="text-[10px] text-slate-400 font-mono">
              {identifiedFeature.geometryType} • {identifiedFeature.crs}
            </div>
          </div>
          <button
            type="button"
            onClick={() => setIdentifiedFeature(null)}
            className="p-1 rounded hover:bg-slate-700/70 text-slate-300 text-xs cursor-pointer"
            aria-label="Close Identify Card"
          >
            ✕
          </button>
        </div>

        <div className="p-3 overflow-y-auto flex-1 space-y-1.5 text-[11px]">
          <div className="flex items-center justify-between text-[10px] text-slate-400 pb-1 border-b border-slate-800">
            <span>Clicked Coordinates</span>
            <span className="font-mono text-slate-200">
              {identifiedFeature.lngLat[1]}, {identifiedFeature.lngLat[0]}
            </span>
          </div>
          {entries.length === 0 ? (
            <div className="text-[11px] text-slate-400 py-2">
              No custom attributes on this feature.
            </div>
          ) : (
            entries.map(([k, v]) => (
              <div
                key={k}
                className="flex items-start justify-between gap-3 py-1 border-b border-slate-800/60"
              >
                <span className="text-slate-400 shrink-0">{k}</span>
                <span className="text-right font-medium text-slate-100 break-all">
                  {String(v)}
                </span>
              </div>
            ))
          )}
        </div>

        {onOpenInRecords && (
          <div className="px-3 py-2 border-t border-slate-700/80 bg-[#1c2028] flex justify-end">
            <button
              type="button"
              onClick={() => onOpenInRecords(identifiedFeature)}
              className="px-2.5 py-1 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] text-[11px] font-bold cursor-pointer"
            >
              Open in Workspace Records
            </button>
          </div>
        )}
      </div>
    )
  }

  return (
    <>
      {renderHoverOverlay()}
      {renderIdentifyCard()}
      {activeLayer && activeLayer.layer_type === "vector" && (
        <DraggableFloatingBar
          storageKey="vector-editing-bar"
          label="Vector Edit"
          defaultStyle={{ top: 12, left: 14 }}
        >
          <VectorEditingToolbar
            activeLayer={activeLayer}
            activeTool={activeTool}
            onSelectTool={(tool) => {
              setActiveTool(tool)
              setDraftCoords([])
              setValidationResult(null)
            }}
            snapEnabled={snapEnabled}
            onToggleSnap={() => setSnapEnabled((p) => !p)}
            selectedFeatureIds={selectedFeatureIds}
            draftCoordinates={draftCoords}
            validationResult={validationResult}
            canUndo={undoStack.length > 0}
            canRedo={redoStack.length > 0}
            onUndo={handleUndo}
            onRedo={handleRedo}
            onConfirmSaveDraft={handleConfirmSaveDraft}
            onCancelDraft={() => {
              setDraftCoords([])
              setEditingFeature(null)
              setValidationResult(null)
              if (activeTool === "vertex") {
                setActiveTool("select")
              }
            }}
            onDeleteSelected={handleDeleteSelected}
            onDuplicateSelected={handleDuplicateSelected}
            onMergeSelected={handleMergeSelected}
          />
        </DraggableFloatingBar>
      )}
    </>
  )
}
