import React, { useEffect, useMemo, useRef, useState } from "react"
import {
  Map as MapLibreMap,
  NavigationControl,
  setWorkerUrl,
} from "maplibre-gl"
import mapLibreWorkerUrl from "maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url"
import {
  MapPin,
  Plus,
  X,
  Layers,
  Grid,
  Trash2,
  Move,
  Eye,
  EyeOff,
  Building,
} from "lucide-react"

import {
  BASEMAP_ATTRIBUTION,
  BASEMAP_TILE_URL,
  datasetTileUrl,
  API_URL,
  getActiveSessionId,
  getIconUrl,
  createIconPoint,
  updateIconPoint,
  deleteIconPoint,
} from "../api"
import MapCoordinates from "./MapCoordinates"
import RasterTransformOverlay from "./RasterTransformOverlay"
import IconPicker from "./IconPicker"
import IconPointForm from "./IconPointForm"
import FloorTraversalOverlay from "./FloorTraversalOverlay"
import BackgroundRemovalDialog from "./BackgroundRemovalDialog"
import { processIconForMap } from "../lib/iconProcessing"
import { resolveBuildingName, isMemberBuildingMatch } from "../lib/buildingUtils"

setWorkerUrl(mapLibreWorkerUrl)


const BASEMAP_LAYER = "workspace-basemap"

function resourceIds(datasetId) {
  const prefix = `workspace-${datasetId}`
  return {
    rasterSource: `${prefix}-raster-source`,
    rasterLayer: `${prefix}-raster-layer`,
    controlSource: `${prefix}-control-source`,
    controlCircle: `${prefix}-control-circle`,
    controlLabel: `${prefix}-control-label`,
    iconSource: `${prefix}-icon-source`,
    iconCircle: `${prefix}-icon-circle`,
    iconSymbol: `${prefix}-icon-symbol`,
    iconLabel: `${prefix}-icon-label`,
  }
}

function boundsArray(bounds) {
  if (!bounds) return null
  const values = Array.isArray(bounds)
    ? bounds
    : [
        bounds.min_longitude ?? bounds.min_x,
        bounds.min_latitude ?? bounds.min_y,
        bounds.max_longitude ?? bounds.max_x,
        bounds.max_latitude ?? bounds.max_y,
      ]
  const parsed = values.map(Number)
  return parsed.length === 4 && parsed.every(Number.isFinite) ? parsed : null
}

function mercatorToLngLat(x, y) {
  const lng = (x / 20037508.342789244) * 180
  let lat = (Math.atan(Math.exp((y / 20037508.342789244) * Math.PI)) * 360) / Math.PI - 90
  lat = Math.max(-85.05112878, Math.min(85.05112878, lat))
  return [lng, lat]
}

function toValidMapLibreBounds(boundsInput) {
  const arr = boundsArray(boundsInput)
  if (!arr) return null
  let [minX, minY, maxX, maxY] = arr

  // Check if coordinates appear to be in EPSG:3857 Web Mercator meters
  const isMercator =
    (Math.abs(minX) > 180 || Math.abs(maxX) > 180 || Math.abs(minY) > 90 || Math.abs(maxY) > 90) &&
    Math.abs(minX) <= 20037509 &&
    Math.abs(maxX) <= 20037509 &&
    Math.abs(minY) <= 20037509 &&
    Math.abs(maxY) <= 20037509

  if (isMercator) {
    const sw = mercatorToLngLat(minX, minY)
    const ne = mercatorToLngLat(maxX, maxY)
    minX = sw[0]
    minY = sw[1]
    maxX = ne[0]
    maxY = ne[1]
  }

  // Must be finite and within plausible geographic limits
  if (
    !Number.isFinite(minX) ||
    !Number.isFinite(minY) ||
    !Number.isFinite(maxX) ||
    !Number.isFinite(maxY) ||
    minX < -180 ||
    maxX > 180 ||
    minY < -90 ||
    maxY > 90 ||
    minX >= maxX ||
    minY >= maxY
  ) {
    return null
  }

  // Clamp latitudes to Web Mercator limits for MapLibre
  const clampedMinY = Math.max(-85.05112878, Math.min(85.05112878, minY))
  const clampedMaxY = Math.max(-85.05112878, Math.min(85.05112878, maxY))

  if (clampedMinY >= clampedMaxY) return null

  return [minX, clampedMinY, maxX, clampedMaxY]
}

function safeFitBounds(map, bounds, options = { padding: 60 }) {
  if (!map || !bounds) return false
  const valid = toValidMapLibreBounds(bounds)
  if (!valid) return false
  try {
    map.fitBounds(
      [
        [valid[0], valid[1]],
        [valid[2], valid[3]],
      ],
      options
    )
    return true
  } catch (err) {
    console.warn("safeFitBounds caught error:", err)
    return false
  }
}

function overlayCoordinates(parameters, imageSize) {
  if (!parameters) return null
  const width = Number(imageSize?.width)
  const height = Number(imageSize?.height)
  if (!width || !height) return null

  let a, b, c, d, e, f

  const transformArray = Array.isArray(parameters)
    ? parameters
    : Array.isArray(parameters?.transform)
      ? parameters.transform
      : null

  if (transformArray && transformArray.length === 6) {
    const [t0, t1, t2, t3, t4, t5] = transformArray.map(Number)
    if ([t0, t1, t2, t3, t4, t5].every(Number.isFinite)) {
      a = t0
      b = -t1
      c = t2
      d = t3
      e = -t4
      f = t5
    }
  }

  if (a === undefined) {
    const params = parameters?.parameters || parameters?.transformation_parameters || parameters
    if (params && typeof params === "object") {
      const values = [params.a, params.b, params.c, params.d, params.e, params.f].map(Number)
      if (values.every(Number.isFinite)) {
        ;[a, b, c, d, e, f] = values
      }
    }
  }

  if (a === undefined || !Number.isFinite(a) || !Number.isFinite(d)) return null

  const project = (imageX, imageY) => [
    a * imageX + b * -imageY + c,
    d * imageX + e * -imageY + f,
  ]

  return [project(0, 0), project(width, 0), project(width, height), project(0, height)]
}

function getOverlayCoordinates(member, layer) {
  const isCoordValid = (c) =>
    Array.isArray(c) &&
    c.length === 2 &&
    Number.isFinite(c[0]) &&
    Number.isFinite(c[1]) &&
    c[0] >= -180 &&
    c[0] <= 180 &&
    c[1] >= -85.051129 &&
    c[1] <= 85.051129

  if (Array.isArray(layer?.coordinates) && layer.coordinates.length === 4) {
    if (layer.coordinates.every(isCoordValid)) {
      return layer.coordinates
    }
  }

  const meta = member.dataset?.extracted_metadata || member.dataset?.metadata || {}
  const transform = meta.transform || meta.parameters || meta.transformation_parameters
  const width = Number(meta.width)
  const height = Number(meta.height)

  if (transform && width > 0 && height > 0) {
    const coords = overlayCoordinates(transform, { width, height })
    if (coords && coords.length === 4 && coords.every(isCoordValid)) {
      return coords
    }
  }

  const validBounds = toValidMapLibreBounds(layer?.bounds || meta.bounds || member.bounds)
  if (validBounds) {
    const [minX, minY, maxX, maxY] = validBounds
    return [
      [minX, maxY], // top-left
      [maxX, maxY], // top-right
      [maxX, minY], // bottom-right
      [minX, minY], // bottom-left
    ]
  }

  return null
}

function pointCollection(records, kind, selectedRecordId) {
  return {
    type: "FeatureCollection",
    features: records
      .filter((record) => record.kind === kind)
      .map((record) => ({
        type: "Feature",
        id: record.id,
        properties: {
          record_id: record.id,
          number: record.pair_number || "",
          label: record.name || record.description || "Point",
          name: record.name || record.description || "Point",
          description: record.description || "",
          icon_key: record.icon_id || record.icon_key || "cinema",
          scope: record.scope || "floor",
          floor_level: record.floor_level ?? 0,
          dataset_id: record.dataset_id,
          selected: record.id === selectedRecordId,
        },
        geometry: {
          type: "Point",
          coordinates: [record.longitude, record.latitude],
        },
      })),
  }
}

function setVisibility(map, layerIds, visible) {
  layerIds.forEach((layerId) => {
    if (map.getLayer(layerId)) {
      map.setLayoutProperty(layerId, "visibility", visible ? "visible" : "none")
    }
  })
}

export default function WorkspaceMap({
  members = [],
  records = [],
  selectedRecordId,
  onSelectRecord,
  onCameraChange,
  savedCamera,
  transformMember,
  onEndTransform,
  onTransformApplied,
  activeFloorLevel = 0,
  activeBuildingName = "",
  onActiveFloorChange,
  onRecordsRefresh,
  primaryDatasetId,
  onMemberUpdate,
  buildings = [],
  isolateActiveFloor = true,
  onToggleIsolateActiveFloor,
}) {
  const containerRef = useRef(null)
  const mapRef = useRef(null)
  const membersRef = useRef(members)
  const selectRecordRef = useRef(onSelectRecord)
  const cameraChangeRef = useRef(onCameraChange)
  const savedCameraRef = useRef(savedCamera)
  const activeFloorLevelRef = useRef(activeFloorLevel)
  const activeBuildingNameRef = useRef(activeBuildingName)
  const fittedRef = useRef(Boolean(savedCamera && (savedCamera.zoom || savedCamera.longitude || savedCamera.center)))
  const lastPannedRecordIdRef = useRef(null)
  const [ready, setReady] = useState(false)
  const [coordinate, setCoordinate] = useState(null)

  // Icon points state & placement mode
  const [selectedIconId, setSelectedIconId] = useState("cinema")
  const [showIconPicker, setShowIconPicker] = useState(false)
  const [isPlacingPoint, setIsPlacingPoint] = useState(false)
  const [pendingPlacementCoord, setPendingPlacementCoord] = useState(null)
  const [editingPoint, setEditingPoint] = useState(null)
  const [movingPointId, setMovingPointId] = useState(null)
  const [showAllFloorPoints, setShowAllFloorPoints] = useState(false)
  const [iconPointsVisible, setIconPointsVisible] = useState(true)
  const [hoveredPoint, setHoveredPoint] = useState(null)
  const [bgRemovalMember, setBgRemovalMember] = useState(null)

  // Internal floor level and building synchronized with props
  const [internalFloorLevel, setInternalFloorLevel] = useState(activeFloorLevel ?? 0)
  const [internalBuildingName, setInternalBuildingName] = useState(activeBuildingName || "")

  const currentFloorLevel = activeFloorLevel !== undefined ? activeFloorLevel : internalFloorLevel
  const currentBuildingName = activeBuildingName !== undefined && activeBuildingName !== "" ? activeBuildingName : internalBuildingName
  const [internalIsolate, setInternalIsolate] = useState(true)
  const isIsolateActive = isolateActiveFloor !== undefined ? isolateActiveFloor : internalIsolate

  const handleToggleIsolate = () => {
    if (onToggleIsolateActiveFloor) {
      onToggleIsolateActiveFloor(!isIsolateActive)
    } else {
      setInternalIsolate(!internalIsolate)
    }
  }

  const handleSelectFloor = (level, buildingName) => {
    const numLevel = Number(level)
    setInternalFloorLevel(numLevel)
    if (buildingName !== undefined) {
      setInternalBuildingName(buildingName || "")
    }
    if (onActiveFloorChange) {
      onActiveFloorChange(numLevel, buildingName)
    }
  }

  // Memoized building floors for point forms and floor switcher
  const buildingFloors = useMemo(() => {
    return members
      .filter((m) => Boolean(
        m.is_building_floor ||
        m.building_name ||
        m.floor_level !== undefined ||
        m.dataset?.extracted_metadata?.is_building_floor ||
        m.dataset?.extracted_metadata?.building_name ||
        m.dataset?.extracted_metadata?.floor_level !== undefined
      ))
      .map((m) => {
        const lvl = m.floor_level ?? m.dataset?.extracted_metadata?.floor_level ?? 0
        const lbl =
          m.floor_label ||
          m.dataset?.extracted_metadata?.floor_label ||
          m.name ||
          m.dataset?.filename ||
          `Floor ${lvl}`
        return {
          id: m.dataset_id,
          dataset_id: m.dataset_id,
          level: lvl,
          floor_level: lvl,
          name: lbl,
          floor_label: lbl,
          building_name: m.building_name || m.dataset?.extracted_metadata?.building_name || "Building",
        }
      })
      .sort((a, b) => b.level - a.level)
  }, [members])

  useEffect(() => {
    membersRef.current = members
  }, [members])

  useEffect(() => {
    selectRecordRef.current = onSelectRecord
  }, [onSelectRecord])

  useEffect(() => {
    cameraChangeRef.current = onCameraChange
  }, [onCameraChange])

  useEffect(() => {
    savedCameraRef.current = savedCamera
  }, [savedCamera])

  const selectFloorRef = useRef(handleSelectFloor)
  useEffect(() => {
    selectFloorRef.current = handleSelectFloor
  })

  useEffect(() => {
    activeFloorLevelRef.current = currentFloorLevel
  }, [currentFloorLevel])

  // Proactively register selected icon
  useEffect(() => {
    const map = mapRef.current
    if (!ready || !map || !selectedIconId) return
    if (!map.hasImage(selectedIconId)) {
      processIconForMap(getIconUrl(selectedIconId), 36)
        .then((canvasOrImg) => {
          if (mapRef.current && !mapRef.current.hasImage(selectedIconId)) {
            try {
              mapRef.current.addImage(selectedIconId, canvasOrImg)
            } catch {}
          }
        })
        .catch(() => {})
    }
  }, [ready, selectedIconId])

  // Initialize MapLibre Map
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
        layers: [{ id: BASEMAP_LAYER, type: "raster", source: "basemap" }],
      },
      center:
        savedCamera && Number.isFinite(savedCamera.longitude) && Number.isFinite(savedCamera.latitude)
          ? [savedCamera.longitude, savedCamera.latitude]
          : savedCamera && Array.isArray(savedCamera.center) && Number.isFinite(savedCamera.center[0]) && Number.isFinite(savedCamera.center[1])
          ? [savedCamera.center[0], savedCamera.center[1]]
          : [0, 20],
      zoom: Number.isFinite(savedCamera?.zoom) ? savedCamera.zoom : 1.5,
      bearing: Number.isFinite(savedCamera?.bearing) ? savedCamera.bearing : 0,
      pitch: Number.isFinite(savedCamera?.pitch) ? savedCamera.pitch : 0,
      maxZoom: 22,
    })
    map.addControl(new NavigationControl({ showCompass: true }), "bottom-right")

    // Dynamic Image Loading for Custom Icons (MapLibre v6 resolver)
    if (typeof map.setMissingStyleImageResolver === "function") {
      map.setMissingStyleImageResolver(async (id) => {
        try {
          const canvasOrImg = await processIconForMap(getIconUrl(id), 36)
          if (!map.hasImage(id)) {
            map.addImage(id, canvasOrImg)
          }
        } catch {
          // Fallback marker circle to prevent missing image crashes
          try {
            const canvas = document.createElement("canvas")
            canvas.width = 24
            canvas.height = 24
            const ctx = canvas.getContext("2d")
            ctx.fillStyle = "#ff3b30"
            ctx.beginPath()
            ctx.arc(12, 12, 9, 0, Math.PI * 2)
            ctx.fill()
            ctx.strokeStyle = "#ffffff"
            ctx.lineWidth = 2
            ctx.stroke()
            if (!map.hasImage(id)) {
              map.addImage(id, ctx.getImageData(0, 0, 24, 24))
            }
          } catch {}
        }
      })
    }

    map.on("styleimagemissing", (e) => {
      const id = e.id
      if (map.hasImage(id)) return
      processIconForMap(getIconUrl(id), 36)
        .then((canvasOrImg) => {
          if (!map.hasImage(id)) {
            map.addImage(id, canvasOrImg)
          }
        })
        .catch(() => {
          try {
            const canvas = document.createElement("canvas")
            canvas.width = 24
            canvas.height = 24
            const ctx = canvas.getContext("2d")
            ctx.fillStyle = "#ff3b30"
            ctx.beginPath()
            ctx.arc(12, 12, 9, 0, Math.PI * 2)
            ctx.fill()
            ctx.strokeStyle = "#ffffff"
            ctx.lineWidth = 2
            ctx.stroke()
            if (!map.hasImage(id)) {
              map.addImage(id, ctx.getImageData(0, 0, 24, 24))
            }
          } catch {}
        })
    })

    map.on("load", () => {
      setReady(true)
    })

    map.on("error", (e) => {
      const msg = e?.error?.message || (typeof e?.error === "string" ? e.error : "") || String(e?.error || "")
      const errName = e?.error?.name || ""
      if (
        msg.includes("could not be decoded") ||
        msg.includes("Failed to fetch") ||
        msg.includes("ERR_NETWORK_CHANGED") ||
        msg.includes("AJAXError") ||
        errName === "InvalidStateError"
      ) {
        console.warn("MapLibre resource notice:", msg || errName)
        return
      }
      console.warn("MapLibre map error:", e?.error || e)
    })

    map.on("mousemove", (event) => {
      setCoordinate({ latitude: event.lngLat.lat, longitude: event.lngLat.lng })

      // Hover check for icon points
      const iconLayers = membersRef.current
        .map((m) => resourceIds(m.dataset_id).iconSymbol)
        .filter((l) => map.getLayer(l))

      if (iconLayers.length > 0) {
        const features = map.queryRenderedFeatures(event.point, { layers: iconLayers })
        if (features.length > 0) {
          const props = features[0].properties
          setHoveredPoint({
            name: props.name || props.label,
            description: props.description,
            icon_key: props.icon_key,
            floor_level: props.floor_level,
            scope: props.scope,
            x: event.point.x,
            y: event.point.y,
          })
          map.getCanvas().style.cursor = "pointer"
          return
        }
      }
      setHoveredPoint(null)
      if (!isPlacingPoint && !movingPointId) {
        map.getCanvas().style.cursor = ""
      }
    })

    map.on("mouseleave", () => {
      setCoordinate(null)
      setHoveredPoint(null)
    })

    let cameraTimer
    map.on("moveend", () => {
      clearTimeout(cameraTimer)
      cameraTimer = setTimeout(() => {
        const center = map.getCenter()
        const zoom = map.getZoom()
        const bearing = map.getBearing()
        const pitch = map.getPitch()
        const last = savedCameraRef.current
        if (last) {
          const lastLng = last.longitude ?? last.center?.[0]
          const lastLat = last.latitude ?? last.center?.[1]
          const lastZoom = last.zoom
          if (
            Number.isFinite(lastLng) &&
            Number.isFinite(lastLat) &&
            Math.abs(center.lng - lastLng) < 0.00001 &&
            Math.abs(center.lat - lastLat) < 0.00001 &&
            Math.abs(zoom - (lastZoom ?? zoom)) < 0.05
          ) {
            return
          }
        }
        cameraChangeRef.current?.({
          longitude: center.lng,
          latitude: center.lat,
          zoom,
          bearing,
          pitch,
        })
      }, 800)
    })

    mapRef.current = map
    const setOpacityFn = (datasetId, opacityVal) => {
      const activeMap = mapRef.current
      if (!activeMap || !activeMap.isStyleLoaded()) return
      const ids = resourceIds(datasetId)
      if (activeMap.getLayer(ids.rasterLayer)) {
        const safe = Number.isFinite(opacityVal) ? Math.max(0, Math.min(1, opacityVal)) : 1.0
        activeMap.setPaintProperty(ids.rasterLayer, "raster-opacity", safe)
      }
    }

    window.__veracity_map = map
    window.__maptiler_map = map
    window.__veracity_setLayerOpacity = setOpacityFn
    window.__maptiler_setLayerOpacity = setOpacityFn
    window.__veracity_setFloor = (lvl, bName) => selectFloorRef.current?.(lvl, bName)
    window.__maptiler_setFloor = (lvl, bName) => selectFloorRef.current?.(lvl, bName)

    return () => {
      clearTimeout(cameraTimer)
      mapRef.current = null
      delete window.__veracity_map
      delete window.__maptiler_map
      delete window.__veracity_setFloor
      delete window.__maptiler_setFloor
      delete window.__veracity_setLayerOpacity
      delete window.__maptiler_setLayerOpacity
      map.remove()
    }
  }, [])

  // Handle map click for placement, moving, or selecting points
  useEffect(() => {
    const map = mapRef.current
    if (!map) return

    const handleClick = async (event) => {
      // 1. Point placement mode
      if (isPlacingPoint) {
        setPendingPlacementCoord({
          latitude: event.lngLat.lat,
          longitude: event.lngLat.lng,
        })
        setIsPlacingPoint(false)
        return
      }

      // 2. Moving existing point mode
      if (movingPointId) {
        try {
          await updateIconPoint(movingPointId, {
            latitude: event.lngLat.lat,
            longitude: event.lngLat.lng,
          })
          if (onRecordsRefresh) await onRecordsRefresh()
        } catch (err) {
          console.error("Failed moving point:", err)
        } finally {
          setMovingPointId(null)
        }
        return
      }

      // 3. Selection of existing points
      const pointLayers = membersRef.current.flatMap((member) => {
        const ids = resourceIds(member.dataset_id)
        return [ids.controlCircle, ids.controlLabel, ids.iconCircle, ids.iconSymbol, ids.iconLabel]
      }).filter((layerId) => map.getLayer(layerId))

      if (!pointLayers.length) return
      const feature = map.queryRenderedFeatures(event.point, { layers: pointLayers })[0]
      if (feature?.properties?.record_id) {
        const recId = feature.properties.record_id
        selectRecordRef.current?.(recId)

        // If it's an icon point, open edit form
        const existingRecord = records.find((r) => r.id === recId && r.kind === "icon_point")
        if (existingRecord) {
          setEditingPoint(existingRecord)
        }
      }
    }

    map.on("click", handleClick)
    return () => {
      map.off("click", handleClick)
    }
  }, [isPlacingPoint, movingPointId, records, onRecordsRefresh])

  // Set cursor based on mode
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    if (isPlacingPoint || movingPointId) {
      map.getCanvas().style.cursor = "crosshair"
    } else {
      map.getCanvas().style.cursor = ""
    }
  }, [isPlacingPoint, movingPointId])

  const sortedMembers = useMemo(
    () => [...members].sort((a, b) => a.sort_order - b.sort_order),
    [members],
  )


  // Synchronize Map Layers & Sources
  useEffect(() => {
    const map = mapRef.current
    if (!ready || !map || !map.isStyleLoaded()) return
    const activeSourceIds = new Set()
    const activeLayerIds = new Set()

    // 1. Raster Layers
    sortedMembers.forEach((member) => {
      const ids = resourceIds(member.dataset_id)
      if (member.file_missing) return
      const layer = member.dataset?.map_layer || (
        (member.dataset?.georeference_status === "completed" || member.bounds || member.dataset?.extracted_metadata?.bounds)
          ? {
              id: `lyr_${member.dataset_id}`,
              name: member.name || member.dataset?.filename,
              type: "raster",
              bounds: member.bounds || member.dataset?.extracted_metadata?.bounds,
              bounds_crs: "EPSG:4326",
            }
          : null
      )
      if (!layer) return

      activeSourceIds.add(ids.rasterSource)
      activeLayerIds.add(ids.rasterLayer)

      const bgRemovalUpdate = member.dataset?.metadata?.background_removal?.updated_at
        || member.dataset?.extracted_metadata?.background_removal?.updated_at
        || ""
      const transformUpdate = member.dataset?.metadata?.raster_transform?.updated_at
        || member.dataset?.extracted_metadata?.raster_transform?.updated_at
        || ""
      const updateVersion = [transformUpdate, bgRemovalUpdate].filter(Boolean).join("_")

      const coordinates = getOverlayCoordinates(member, layer)
      const safeOpacity = typeof member.raster_opacity === "number" && Number.isFinite(member.raster_opacity)
        ? Math.max(0, Math.min(1, member.raster_opacity))
        : 1.0

      if (coordinates) {
        const sid = member.dataset?.session_id || getActiveSessionId()
        const queryParams = []
        if (sid) queryParams.push(`session_id=${encodeURIComponent(sid)}`)
        if (updateVersion) queryParams.push(`v=${encodeURIComponent(updateVersion)}`)
        const qStr = queryParams.length > 0 ? `?${queryParams.join("&")}` : ""
        const imageUrl = `${API_URL}/datasets/${encodeURIComponent(member.dataset_id)}/source-image${qStr}`
        const coordsKey = coordinates.map(([lng, lat]) => `${lng.toFixed(6)},${lat.toFixed(6)}`).join(";")
        const existing = map.getSource(ids.rasterSource)

        if (existing) {
          if (existing.type === "image") {
            const urlChanged = existing._currentImageUrl && existing._currentImageUrl !== imageUrl
            const coordsChanged = existing._currentCoordsKey && existing._currentCoordsKey !== coordsKey
            if (urlChanged || coordsChanged) {
              try {
                if (typeof existing.updateImage === "function") {
                  existing.updateImage({ url: imageUrl, coordinates })
                  existing._currentImageUrl = imageUrl
                  existing._currentCoordsKey = coordsKey
                } else {
                  throw new Error("updateImage not supported")
                }
              } catch {
                if (map.getLayer(ids.rasterLayer)) map.removeLayer(ids.rasterLayer)
                map.removeSource(ids.rasterSource)
              }
            }
          } else {
            if (map.getLayer(ids.rasterLayer)) map.removeLayer(ids.rasterLayer)
            map.removeSource(ids.rasterSource)
          }
        }

        if (!map.getSource(ids.rasterSource)) {
          try {
            map.addSource(ids.rasterSource, {
              type: "image",
              url: imageUrl,
              coordinates: coordinates,
            })
            const added = map.getSource(ids.rasterSource)
            if (added) {
              added._currentImageUrl = imageUrl
              added._currentCoordsKey = coordsKey
            }
          } catch (srcErr) {
            console.warn("Could not add image source to map:", srcErr)
          }
        }
      } else if (layer.tile_url_template) {
        const baseTileUrl = datasetTileUrl(member.dataset_id, layer.tile_url_template)
        const tileUrl = updateVersion
          ? `${baseTileUrl}${baseTileUrl.includes("?") ? "&" : "?"}v=${encodeURIComponent(updateVersion)}`
          : baseTileUrl

        const bounds = boundsArray(layer.bounds)
        if (!map.getSource(ids.rasterSource)) {
          const options = {
            type: "raster",
            tiles: [tileUrl],
            tileSize: layer.tile_size || 256,
            minzoom: layer.tile_min_zoom ?? 0,
            maxzoom: layer.tile_max_zoom ?? 22,
          }
          if (bounds) options.bounds = bounds
          map.addSource(ids.rasterSource, options)
        }
      }

      if (map.getSource(ids.rasterSource) && !map.getLayer(ids.rasterLayer)) {
        try {
          map.addLayer({
            id: ids.rasterLayer,
            type: "raster",
            source: ids.rasterSource,
            paint: { "raster-opacity": safeOpacity },
          })
        } catch (layerErr) {
          console.warn("Could not add raster layer:", layerErr)
        }
      }

      const isBuildingFloor = Boolean(
        member.is_building_floor ||
        member.building_name ||
        member.floor_level !== undefined ||
        member.dataset?.extracted_metadata?.is_building_floor ||
        member.dataset?.extracted_metadata?.building_name ||
        member.dataset?.extracted_metadata?.floor_level !== undefined
      )
      const memberFloorLevel = Number(member.floor_level ?? member.dataset?.extracted_metadata?.floor_level ?? 0)
      const isBuildingMatch = isMemberBuildingMatch(member, currentBuildingName, sortedMembers, buildings)

      const isFloorActive = !isIsolateActive || !isBuildingFloor || (isBuildingMatch && memberFloorLevel === Number(currentFloorLevel ?? 0))
      const isVisible = isIsolateActive && isBuildingFloor
        ? (isBuildingMatch && memberFloorLevel === Number(currentFloorLevel ?? 0) && member.raster_visible !== false)
        : (member.visible !== false && member.raster_visible !== false)
      const isLayerVisible = isVisible && isFloorActive

      if (map.getLayer(ids.rasterLayer)) {
        map.setPaintProperty(ids.rasterLayer, "raster-opacity", safeOpacity)
        setVisibility(map, [ids.rasterLayer], isLayerVisible)
      }
    })

    // 2. Point Layers (Control Points & Geographic Icon Points)
    sortedMembers.forEach((member) => {
      const ids = resourceIds(member.dataset_id)
      const isBuildingFloor = Boolean(
        member.is_building_floor ||
        member.building_name ||
        member.floor_level !== undefined ||
        member.dataset?.extracted_metadata?.is_building_floor ||
        member.dataset?.extracted_metadata?.building_name ||
        member.dataset?.extracted_metadata?.floor_level !== undefined
      )
      const memberFloorLevel = Number(member.floor_level ?? member.dataset?.extracted_metadata?.floor_level ?? 0)
      const isBuildingMatch = isMemberBuildingMatch(member, currentBuildingName, sortedMembers, buildings)
      const isFloorActive = !isIsolateActive || !isBuildingFloor || (isBuildingMatch && memberFloorLevel === Number(currentFloorLevel ?? 0))

      // Filter points respecting floor and scope
      const memberRecords = records.filter((record) => {
        if (record.dataset_id !== member.dataset_id) return false
        if (showAllFloorPoints) return true

        if (record.kind === "control_point") {
          if (isIsolateActive) {
            // When floor/building isolation is active, ONLY show control points for the active floor of the active building
            return isBuildingFloor ? (isBuildingMatch && memberFloorLevel === Number(currentFloorLevel ?? 0)) : false
          }
          if (!isBuildingFloor) return true
          return isFloorActive
        }

        const scope = record.scope || (record.floor_level !== undefined && record.floor_level !== null ? "floor" : "building")
        if (scope === "building") return true
        if (scope === "floor") {
          return record.floor_level === currentFloorLevel
        }
        return !isBuildingFloor || memberFloorLevel === currentFloorLevel
      })

      const controlData = pointCollection(memberRecords, "control_point", selectedRecordId)
      const iconData = pointCollection(memberRecords, "icon_point", selectedRecordId)

      // Proactively ensure map has images for icon features
      if (iconData?.features) {
        iconData.features.forEach((f) => {
          const iconId = f.properties?.icon_key || f.properties?.icon_id
          if (iconId && !map.hasImage(iconId)) {
            processIconForMap(getIconUrl(iconId), 36)
              .then((canvasOrImg) => {
                if (mapRef.current && !mapRef.current.hasImage(iconId)) {
                  try {
                    mapRef.current.addImage(iconId, canvasOrImg)
                  } catch {}
                }
              })
              .catch(() => {})
          }
        })
      }

      const isControlVisible = Boolean(
        member.visible &&
        member.control_points_visible &&
        (!isIsolateActive || (isBuildingMatch && memberFloorLevel === Number(currentFloorLevel ?? 0)))
      )

      const definitions = [
        {
          sourceId: ids.controlSource,
          data: controlData,
          layers: [
            {
              id: ids.controlCircle,
              type: "circle",
              paint: {
                "circle-radius": ["case", ["get", "selected"], 8, 6],
                "circle-color": "#0066cc",
                "circle-stroke-color": "#ffffff",
                "circle-stroke-width": ["case", ["get", "selected"], 3, 2],
              },
            },
            {
              id: ids.controlLabel,
              type: "symbol",
              layout: {
                "text-field": ["to-string", ["get", "number"]],
                "text-size": 11,
                "text-allow-overlap": true,
                "text-ignore-placement": true,
              },
              paint: { "text-color": "#ffffff" },
            },
          ],
          visible: isControlVisible,
        },
        {
          sourceId: ids.iconSource,
          data: iconData,
          layers: [
            {
              id: ids.iconCircle,
              type: "circle",
              paint: {
                "circle-radius": ["case", ["get", "selected"], 16, 0],
                "circle-color": "rgba(0, 102, 204, 0.2)",
                "circle-stroke-color": "#0066cc",
                "circle-stroke-width": ["case", ["get", "selected"], 2, 0],
                "circle-translate": [0, -12],
              },
            },
            {
              id: ids.iconSymbol,
              type: "symbol",
              layout: {
                "icon-image": ["coalesce", ["get", "icon_key"], "cinema"],
                "icon-size": 0.8,
                "icon-anchor": "bottom",
                "icon-allow-overlap": true,
                "icon-ignore-placement": true,
                "text-field": ["get", "name"],
                "text-size": 11,
                "text-offset": [0, 0.4],
                "text-anchor": "top",
                "text-allow-overlap": false,
              },
              paint: {
                "text-color": "#1d1d1f",
                "text-halo-color": "#ffffff",
                "text-halo-width": 2,
              },
            },
          ],
          visible: Boolean(iconPointsVisible && member.visible && member.icon_points_visible !== false),
        },
      ]

      definitions.forEach((definition) => {
        activeSourceIds.add(definition.sourceId)
        definition.layers.forEach((layer) => activeLayerIds.add(layer.id))
        const source = map.getSource(definition.sourceId)
        if (source?.setData) source.setData(definition.data)
        if (!source) map.addSource(definition.sourceId, { type: "geojson", data: definition.data })
        definition.layers.forEach((layer) => {
          if (!map.getLayer(layer.id)) {
            map.addLayer({ ...layer, source: definition.sourceId })
          }
        })
        setVisibility(map, definition.layers.map((layer) => layer.id), definition.visible)
      })
    })

    // Ensure raster layers are beneath point layers, with active floor raster layers on top of other rasters
    const activeFloorMembers = sortedMembers.filter((m) => {
      const isFloor = Boolean(
        m.is_building_floor ||
        m.building_name ||
        m.floor_level !== undefined ||
        m.dataset?.extracted_metadata?.is_building_floor ||
        m.dataset?.extracted_metadata?.building_name ||
        m.dataset?.extracted_metadata?.floor_level !== undefined
      )
      const lvl = Number(m.floor_level ?? m.dataset?.extracted_metadata?.floor_level ?? 0)
      const isBldMatch = isMemberBuildingMatch(m, currentBuildingName, sortedMembers, buildings)
      return isFloor && lvl === Number(currentFloorLevel ?? 0) && isBldMatch
    })

    const inactiveMembers = sortedMembers.filter((m) => !activeFloorMembers.includes(m))

    // 1. Position inactive raster layers first
    inactiveMembers.forEach((member) => {
      const rasterLayer = resourceIds(member.dataset_id).rasterLayer
      if (map.getLayer(rasterLayer)) {
        try {
          map.moveLayer(rasterLayer)
        } catch {}
      }
    })

    // 2. Position active floor raster layers on TOP of other rasters
    activeFloorMembers.forEach((member) => {
      const rasterLayer = resourceIds(member.dataset_id).rasterLayer
      if (map.getLayer(rasterLayer)) {
        try {
          map.moveLayer(rasterLayer)
        } catch {}
      }
    })

    // 3. ALWAYS elevate all point layers (controls and icons) to the top of the stack above all rasters
    sortedMembers.forEach((member) => {
      const ids = resourceIds(member.dataset_id)
      const pointLayers = [ids.controlCircle, ids.controlLabel, ids.iconCircle, ids.iconSymbol, ids.iconLabel]
      pointLayers.forEach((layerId) => {
        if (map.getLayer(layerId)) {
          try {
            map.moveLayer(layerId)
          } catch {}
        }
      })
    })

    // Cleanup stale layers
    const style = map.getStyle()
    ;(style.layers || [])
      .filter((layer) => layer.id.startsWith("workspace-") && layer.id !== BASEMAP_LAYER)
      .forEach((layer) => {
        if (!activeLayerIds.has(layer.id) && map.getLayer(layer.id)) map.removeLayer(layer.id)
      })
    Object.keys(style.sources || {})
      .filter((sourceId) => sourceId.startsWith("workspace-"))
      .forEach((sourceId) => {
        if (!activeSourceIds.has(sourceId) && map.getSource(sourceId)) map.removeSource(sourceId)
      })

    // Initial camera fit
    if (!fittedRef.current) {
      const visibleBounds = sortedMembers
        .filter((member) => member.visible && (member.dataset?.georeference_status === "completed" || member.dataset?.map_layer))
        .map((member) => toValidMapLibreBounds(member.dataset?.map_layer?.bounds || member.bounds || member.dataset?.extracted_metadata?.bounds))
        .filter(Boolean)
      if (visibleBounds.length) {
        const union = visibleBounds.reduce((current, bounds) => [
          Math.min(current[0], bounds[0]),
          Math.min(current[1], bounds[1]),
          Math.max(current[2], bounds[2]),
          Math.max(current[3], bounds[3]),
        ])
        if (safeFitBounds(map, union, { padding: 60, duration: 0 })) {
          fittedRef.current = true
        }
      }
    }
  }, [ready, records, selectedRecordId, sortedMembers, currentFloorLevel, currentBuildingName, isIsolateActive, showAllFloorPoints, iconPointsVisible])

  // Pan to selected record
  useEffect(() => {
    if (!selectedRecordId) {
      lastPannedRecordIdRef.current = null
      return
    }
    if (lastPannedRecordIdRef.current === selectedRecordId) return

    const selected = records.find((record) => record.id === selectedRecordId)
    const map = mapRef.current
    if (!map || !selected) return

    lastPannedRecordIdRef.current = selectedRecordId

    if (selected.kind === "raster") {
      if (selected.bounds) {
        safeFitBounds(map, selected.bounds, { padding: 60, duration: 1000 })
      }
      return
    }
    if (
      selected.longitude != null &&
      selected.latitude != null &&
      Number.isFinite(selected.longitude) &&
      Number.isFinite(selected.latitude) &&
      selected.longitude >= -180 &&
      selected.longitude <= 180 &&
      selected.latitude >= -85.051129 &&
      selected.latitude <= 85.051129
    ) {
      try {
        map.easeTo({ center: [selected.longitude, selected.latitude], zoom: Math.max(map.getZoom(), 16), duration: 1000 })
      } catch (easeErr) {
        console.warn("easeTo failed:", easeErr)
      }
    }
  }, [records, selectedRecordId])

  // Save point handler
  async function handleSaveNewPoint(pointData) {
    const targetDatasetId = primaryDatasetId || sortedMembers[0]?.dataset_id
    if (!targetDatasetId) return
    try {
      await createIconPoint(targetDatasetId, {
        ...pointData,
        floor_level: activeFloorLevel,
      })
      if (onRecordsRefresh) await onRecordsRefresh()
    } catch (err) {
      console.error("Error creating icon point:", err)
    } finally {
      setPendingPlacementCoord(null)
    }
  }

  // Update existing point handler
  async function handleUpdateExistingPoint(pointData) {
    if (!editingPoint) return
    try {
      await updateIconPoint(editingPoint.id, pointData)
      if (onRecordsRefresh) await onRecordsRefresh()
    } catch (err) {
      console.error("Error updating icon point:", err)
    } finally {
      setEditingPoint(null)
    }
  }

  // Delete point handler
  async function handleDeletePoint() {
    if (!editingPoint) return
    try {
      await deleteIconPoint(editingPoint.id)
      if (onRecordsRefresh) await onRecordsRefresh()
    } catch (err) {
      console.error("Error deleting icon point:", err)
    } finally {
      setEditingPoint(null)
    }
  }

  return (
    <div className="workspace-map-canvas relative w-full h-full">
      <div ref={containerRef} className="workspace-map w-full h-full" />
      <MapCoordinates coordinate={coordinate} />

      {/* Floating Map Tools: Icon Placement & Floor View Controls */}
      <div className="absolute top-4 left-4 z-20 flex flex-col gap-2">
        <div className="bg-white/95 backdrop-blur-xs rounded-xl shadow-lg border border-gray-200 p-1.5 flex items-center gap-1.5">
          {/* Icon Picker Trigger */}
          <button
            type="button"
            onClick={() => setShowIconPicker(true)}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-gray-50 hover:bg-gray-100 border border-gray-200 text-xs font-medium text-gray-800 transition-colors"
            title="Choose icon asset"
          >
            <div className="size-5 rounded bg-white border border-gray-200 flex items-center justify-center p-0.5">
              <img
                src={getIconUrl(selectedIconId)}
                alt={selectedIconId}
                className="max-size-full object-contain"
              />
            </div>
            <span className="capitalize">{selectedIconId}</span>
          </button>

          {/* Place Point Toggle */}
          <button
            type="button"
            onClick={() => setIsPlacingPoint((prev) => !prev)}
            className={`flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-semibold transition-all shadow-2xs ${
              isPlacingPoint
                ? "bg-amber-600 text-white animate-pulse"
                : "bg-blue-600 hover:bg-blue-700 text-white"
            }`}
          >
            <Plus className="size-3.5" />
            <span>{isPlacingPoint ? "Click Map to Place" : "Place Point"}</span>
          </button>

          {/* Icon Points Visibility */}
          <button
            type="button"
            onClick={() => setIconPointsVisible((v) => !v)}
            className={`p-1.5 rounded-lg border text-xs transition-colors ${
              iconPointsVisible
                ? "text-gray-700 bg-gray-50 border-gray-200 hover:bg-gray-100"
                : "text-gray-400 bg-gray-100 border-gray-200"
            }`}
            title={iconPointsVisible ? "Hide icon points" : "Show icon points"}
          >
            {iconPointsVisible ? <Eye className="size-4" /> : <EyeOff className="size-4" />}
          </button>

          {/* Cross-floor visibility toggle */}
          {buildingFloors.length > 1 && (
            <button
              type="button"
              onClick={() => setShowAllFloorPoints((v) => !v)}
              className={`px-2 py-1 rounded-lg border text-[11px] font-medium transition-colors ${
                showAllFloorPoints
                  ? "bg-purple-50 border-purple-300 text-purple-700 font-semibold"
                  : "bg-gray-50 border-gray-200 text-gray-600 hover:bg-gray-100"
              }`}
              title="Show points from all floors"
            >
              All Floors
            </button>
          )}
        </div>

        {/* Banner indicator when in placement or moving mode */}
        {(isPlacingPoint || movingPointId) && (
          <div className="bg-amber-500 text-white text-xs px-3 py-1.5 rounded-lg shadow-md flex items-center justify-between gap-3 animate-in fade-in slide-in-from-top-1">
            <span>
              {isPlacingPoint
                ? `Click map to place "${selectedIconId}" point`
                : "Click map at new location to move point"}
            </span>
            <button
              type="button"
              onClick={() => {
                setIsPlacingPoint(false)
                setMovingPointId(null)
              }}
              className="text-white/80 hover:text-white"
            >
              <X className="size-3.5" />
            </button>
          </div>
        )}
      </div>

      {/* Hover Tooltip */}
      {hoveredPoint && !editingPoint && !pendingPlacementCoord && (
        <div
          className="absolute z-30 pointer-events-none bg-gray-900/90 text-white rounded-lg px-2.5 py-1.5 text-xs shadow-xl backdrop-blur-xs flex items-center gap-2 transform -translate-x-1/2 -translate-y-full mb-3"
          style={{ left: hoveredPoint.x, top: hoveredPoint.y }}
        >
          <div className="size-5 rounded bg-white/10 p-0.5 flex items-center justify-center">
            <img
              src={getIconUrl(hoveredPoint.icon_key)}
              alt=""
              className="max-size-full object-contain"
            />
          </div>
          <div>
            <div className="font-semibold text-white leading-tight">
              {hoveredPoint.name}
            </div>
            {hoveredPoint.description && (
              <div className="text-[10px] text-gray-300 max-w-[160px] truncate">
                {hoveredPoint.description}
              </div>
            )}
            <div className="text-[9px] text-blue-300 capitalize">
              Floor {hoveredPoint.floor_level} • {hoveredPoint.scope}
            </div>
          </div>
        </div>
      )}

      {/* Floor Traversal Overlay for Multi-floor navigation */}
      <FloorTraversalOverlay
        members={sortedMembers}
        buildings={buildings}
        activeFloorLevel={currentFloorLevel}
        onSelectFloor={handleSelectFloor}
        onUpdateMember={onMemberUpdate}
        isolateActiveFloor={isIsolateActive}
        onToggleIsolateActiveFloor={handleToggleIsolate}
        onOpenBackgroundRemoval={(m) => setBgRemovalMember(m)}
      />

      {/* Background Removal Dialog for Selected Floor */}
      {bgRemovalMember && (
        <BackgroundRemovalDialog
          open={Boolean(bgRemovalMember)}
          datasetId={bgRemovalMember.dataset_id}
          datasetName={bgRemovalMember.name || "Floor Plan"}
          isGeoreferenced={Boolean(
            bgRemovalMember.dataset?.georeference_status === "completed" ||
            bgRemovalMember.dataset?.map_layer ||
            bgRemovalMember.map_layer
          )}
          onClose={() => setBgRemovalMember(null)}
          onApplied={() => {
            if (onTransformApplied) onTransformApplied()
            // Invalidate tile cache by updating timestamp
            const map = mapRef.current
            if (map && map.isStyleLoaded()) {
              sortedMembers.forEach((m) => {
                const ids = resourceIds(m.dataset_id)
                if (map.getSource(ids.rasterSource)) {
                  // Trigger source refresh
                  const src = map.getSource(ids.rasterSource)
                  if (src && typeof src.updateImage === "function") {
                    // updated via url
                  }
                }
              })
            }
          }}
        />
      )}


      {/* New Point Placement Form Modal */}
      {pendingPlacementCoord && (
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-40 animate-in fade-in zoom-in-95 duration-100">
          <IconPointForm
            coordinate={pendingPlacementCoord}
            initialIconKey={selectedIconId}
            initialFloorLevel={currentFloorLevel}
            floors={buildingFloors}
            title="Create Geographic Icon Point"
            submitLabel="Save Point"
            onCancel={() => setPendingPlacementCoord(null)}
            onSubmit={handleSaveNewPoint}
          />
        </div>
      )}

      {/* Editing Existing Point Form Modal */}
      {editingPoint && (
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-40 animate-in fade-in zoom-in-95 duration-100">
          <IconPointForm
            coordinate={{
              latitude: editingPoint.latitude,
              longitude: editingPoint.longitude,
            }}
            initialName={editingPoint.name}
            initialDescription={editingPoint.description}
            initialIconKey={editingPoint.icon_id || editingPoint.icon_key}
            initialScope={editingPoint.scope || "floor"}
            initialFloorLevel={editingPoint.floor_level ?? currentFloorLevel}
            floors={buildingFloors}
            title="Edit Geographic Point"
            submitLabel="Update Point"
            isEditing={true}
            onCancel={() => setEditingPoint(null)}
            onSubmit={handleUpdateExistingPoint}
            onDelete={handleDeletePoint}
            onStartMove={() => {
              setMovingPointId(editingPoint.id)
              setEditingPoint(null)
            }}
          />
        </div>
      )}

      {/* Icon Picker Dialog */}
      {showIconPicker && (
        <IconPicker
          selectedIconId={selectedIconId}
          onSelectIcon={(icon) => setSelectedIconId(icon.id)}
          onClose={() => setShowIconPicker(false)}
          onStartPlacement={() => {
            setIsPlacingPoint(true)
            setShowIconPicker(false)
          }}
        />
      )}

      {/* Raster Transform Overlay */}
      {transformMember && mapRef.current && (
        <RasterTransformOverlay
          map={mapRef.current}
          datasetId={transformMember.dataset_id}
          datasetName={transformMember.name}
          onClose={onEndTransform}
          onApplied={onTransformApplied}
        />
      )}


      {/* Empty State */}
      {!sortedMembers.some((member) => member.dataset?.map_layer || member.dataset?.georeference_status === "completed" || member.bounds || member.dataset?.extracted_metadata?.bounds) && (
        <div className="workspace-map-empty">
          <strong>No mapped raster is available</strong>
          <span>Open Database to inspect raw datasets or georeference a layer.</span>
        </div>
      )}
    </div>
  )
}
