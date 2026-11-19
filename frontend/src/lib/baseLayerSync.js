import { API_URL, BASEMAP_ATTRIBUTION, BASEMAP_TILE_URL, datasetTileUrl, getActiveSessionId } from "../api"
import {
  BASEMAP_SOURCE_ID,
  BASEMAP_LAYER_ID,
  BASEMAP_ROADS_SOURCE_ID,
  BASEMAP_ROADS_LAYER_ID,
  BASEMAP_LABELS_SOURCE_ID,
  BASEMAP_LABELS_LAYER_ID,
  CUSTOM_BASE_SOURCE_ID,
  CUSTOM_BASE_RASTER_LAYER_ID,
  CUSTOM_BASE_VECTOR_FILL_ID,
  CUSTOM_BASE_VECTOR_LINE_ID,
  isBackgroundLayerId,
  findFirstWorkspaceLayerId,
  resolveBaseLayerCoordinates,
} from "./baseLayerUtils"

export {
  BASEMAP_SOURCE_ID,
  BASEMAP_LAYER_ID,
  BASEMAP_ROADS_SOURCE_ID,
  BASEMAP_ROADS_LAYER_ID,
  BASEMAP_LABELS_SOURCE_ID,
  BASEMAP_LABELS_LAYER_ID,
  CUSTOM_BASE_SOURCE_ID,
  CUSTOM_BASE_RASTER_LAYER_ID,
  CUSTOM_BASE_VECTOR_FILL_ID,
  CUSTOM_BASE_VECTOR_LINE_ID,
  isBackgroundLayerId,
  findFirstWorkspaceLayerId,
  resolveBaseLayerCoordinates,
}

export const DEFAULT_MAP_VIEWS = [
  {
    type: "world",
    name: "World",
    description: "Default OpenStreetMap global geographic reference.",
    tile_url: BASEMAP_TILE_URL,
    attribution: BASEMAP_ATTRIBUTION,
    min_zoom: 0,
    max_zoom: 19,
    available: true,
    capabilities: { supports_labels: false, supports_roads: false, supports_places: false, supports_opacity: true },
  },
  {
    type: "satellite",
    name: "Satellite",
    description: "Satellite imagery with optional road and place overlays.",
    tile_url: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}",
    labels_tile_url: "https://services.arcgisonline.com/ArcGIS/rest/services/Reference/World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}",
    roads_tile_url: "https://services.arcgisonline.com/ArcGIS/rest/services/Reference/World_Transportation/MapServer/tile/{z}/{y}/{x}",
    places_tile_url: "https://services.arcgisonline.com/ArcGIS/rest/services/Reference/World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}",
    attribution: "Tiles © Esri",
    min_zoom: 0,
    max_zoom: 19,
    available: true,
    capabilities: { supports_labels: true, supports_roads: true, supports_places: true, supports_opacity: true },
  },
  {
    type: "streets",
    name: "Streets",
    description: "Clean street map with toggleable labels.",
    tile_url: "https://basemaps.cartocdn.com/rastertiles/voyager_nolabels/{z}/{x}/{y}.png",
    labels_tile_url: "https://basemaps.cartocdn.com/rastertiles/voyager_only_labels/{z}/{x}/{y}.png",
    places_tile_url: "https://basemaps.cartocdn.com/rastertiles/voyager_only_labels/{z}/{x}/{y}.png",
    attribution: "© OpenStreetMap contributors © CARTO",
    min_zoom: 0,
    max_zoom: 20,
    available: true,
    capabilities: { supports_labels: true, supports_roads: false, supports_places: true, supports_opacity: true },
  },
  {
    type: "terrain",
    name: "Terrain",
    description: "Topographic terrain with elevation contours.",
    tile_url: "https://tile.opentopomap.org/{z}/{x}/{y}.png",
    attribution: "© OpenTopoMap (CC-BY-SA)",
    min_zoom: 0,
    max_zoom: 17,
    available: true,
    capabilities: { supports_labels: false, supports_roads: false, supports_places: false, supports_opacity: true },
  },
  {
    type: "light",
    name: "Light",
    description: "Minimal light basemap for GIS overlays.",
    tile_url: "https://basemaps.cartocdn.com/light_nolabels/{z}/{x}/{y}.png",
    labels_tile_url: "https://basemaps.cartocdn.com/light_only_labels/{z}/{x}/{y}.png",
    places_tile_url: "https://basemaps.cartocdn.com/light_only_labels/{z}/{x}/{y}.png",
    attribution: "© OpenStreetMap contributors © CARTO",
    min_zoom: 0,
    max_zoom: 20,
    available: true,
    capabilities: { supports_labels: true, supports_roads: false, supports_places: true, supports_opacity: true },
  },
  {
    type: "dark",
    name: "Dark",
    description: "Dark technical basemap.",
    tile_url: "https://basemaps.cartocdn.com/dark_nolabels/{z}/{x}/{y}.png",
    labels_tile_url: "https://basemaps.cartocdn.com/dark_only_labels/{z}/{x}/{y}.png",
    places_tile_url: "https://basemaps.cartocdn.com/dark_only_labels/{z}/{x}/{y}.png",
    attribution: "© OpenStreetMap contributors © CARTO",
    min_zoom: 0,
    max_zoom: 20,
    available: true,
    capabilities: { supports_labels: true, supports_roads: false, supports_places: true, supports_opacity: true },
  },
  {
    type: "custom",
    name: "Custom",
    description: "Selected custom base layer.",
    min_zoom: 0,
    max_zoom: 22,
    available: true,
    capabilities: { supports_labels: false, supports_roads: false, supports_places: false, supports_opacity: true },
  },
]

function safeRemoveLayerAndSource(map, layerIds, sourceId) {
  layerIds.forEach((lid) => {
    if (map.getLayer(lid)) {
      try {
        map.removeLayer(lid)
      } catch {}
    }
  })
  if (sourceId && map.getSource(sourceId)) {
    try {
      map.removeSource(sourceId)
    } catch {}
  }
}

export function resolveCustomBaseImageUrl(baseLayer) {
  const rs = baseLayer?.rendering_source || {}
  const dsId = baseLayer?.dataset_id || rs.dataset_id
  const sid = rs.session_id || getActiveSessionId()
  if (rs.image_url) {
    const base = /^https?:\/\//i.test(rs.image_url)
      ? rs.image_url
      : `${API_URL}${rs.image_url.startsWith("/") ? "" : "/"}${rs.image_url}`
    if (sid && !base.includes("session_id=")) {
      return `${base}${base.includes("?") ? "&" : "?"}session_id=${encodeURIComponent(sid)}`
    }
    return base
  }
  if (!dsId) return null
  const q = sid ? `?session_id=${encodeURIComponent(sid)}` : ""
  return `${API_URL}/datasets/${encodeURIComponent(dsId)}/source-image${q}`
}

/**
 * Synchronizes the background Map View (World, Satellite, Streets, Terrain, Light, Dark, Custom)
 * and optional Custom Base Layer strictly BENEATH all workspace layers without calling map.setStyle().
 */
export function syncMapBaseAndView(
  map,
  {
    mapViews = DEFAULT_MAP_VIEWS,
    viewType = "world",
    customBaseLayer = null,
    opacity = 1.0,
    visible = true,
    showLabels = true,
    showRoads = true,
    showPlaces = true,
    basemapLayerId = BASEMAP_LAYER_ID,
  } = {}
) {
  if (!map || typeof map.getStyle !== "function" || !map.getStyle()) return

  const safeOpacity = Number.isFinite(Number(opacity)) ? Math.max(0, Math.min(1, Number(opacity))) : 1.0
  const activeViewDef =
    mapViews.find((v) => v.type === viewType) ||
    DEFAULT_MAP_VIEWS.find((v) => v.type === viewType) ||
    DEFAULT_MAP_VIEWS[0]

  const beforeId = findFirstWorkspaceLayerId(map)
  const isCustomMode = viewType === "custom" && customBaseLayer && !customBaseLayer.is_default

  // 1. Update primary raster tile basemap source (when not purely hidden)
  const tileUrl = isCustomMode
    ? (mapViews.find((v) => v.type === "light")?.tile_url || BASEMAP_TILE_URL)
    : (activeViewDef.tile_url || BASEMAP_TILE_URL)
  const maxZoom = activeViewDef.max_zoom || 19
  const attribution = activeViewDef.attribution || BASEMAP_ATTRIBUTION

  const existingBase = map.getSource(BASEMAP_SOURCE_ID)
  if (!existingBase || existingBase._currentTileUrl !== tileUrl) {
    safeRemoveLayerAndSource(map, [basemapLayerId], BASEMAP_SOURCE_ID)
    try {
      map.addSource(BASEMAP_SOURCE_ID, {
        type: "raster",
        tiles: [tileUrl],
        tileSize: 256,
        attribution,
        maxzoom: maxZoom,
      })
      const added = map.getSource(BASEMAP_SOURCE_ID)
      if (added) added._currentTileUrl = tileUrl
      map.addLayer(
        {
          id: basemapLayerId,
          type: "raster",
          source: BASEMAP_SOURCE_ID,
          paint: { "raster-opacity": isCustomMode ? Math.min(0.35, safeOpacity) : safeOpacity },
        },
        beforeId
      )
    } catch {}
  } else if (map.getLayer(basemapLayerId)) {
    try {
      map.setPaintProperty(basemapLayerId, "raster-opacity", isCustomMode ? Math.min(0.35, safeOpacity) : safeOpacity)
    } catch {}
  }

  if (map.getLayer(basemapLayerId)) {
    try {
      map.setLayoutProperty(basemapLayerId, "visibility", visible ? "visible" : "none")
    } catch {}
  }

  // 2. Provider-dependent Roads overlay (e.g. Esri World Transportation on Satellite)
  const roadsUrl = !isCustomMode && activeViewDef.capabilities?.supports_roads ? activeViewDef.roads_tile_url : ""
  if (roadsUrl && visible && showRoads) {
    const existingRoads = map.getSource(BASEMAP_ROADS_SOURCE_ID)
    if (!existingRoads || existingRoads._currentTileUrl !== roadsUrl) {
      safeRemoveLayerAndSource(map, [BASEMAP_ROADS_LAYER_ID], BASEMAP_ROADS_SOURCE_ID)
      try {
        map.addSource(BASEMAP_ROADS_SOURCE_ID, {
          type: "raster",
          tiles: [roadsUrl],
          tileSize: 256,
          maxzoom: maxZoom,
        })
        const added = map.getSource(BASEMAP_ROADS_SOURCE_ID)
        if (added) added._currentTileUrl = roadsUrl
        map.addLayer(
          {
            id: BASEMAP_ROADS_LAYER_ID,
            type: "raster",
            source: BASEMAP_ROADS_SOURCE_ID,
            paint: { "raster-opacity": safeOpacity },
          },
          beforeId
        )
      } catch {}
    } else if (map.getLayer(BASEMAP_ROADS_LAYER_ID)) {
      try {
        map.setLayoutProperty(BASEMAP_ROADS_LAYER_ID, "visibility", "visible")
        map.setPaintProperty(BASEMAP_ROADS_LAYER_ID, "raster-opacity", safeOpacity)
      } catch {}
    }
  } else if (map.getLayer(BASEMAP_ROADS_LAYER_ID)) {
    try {
      map.setLayoutProperty(BASEMAP_ROADS_LAYER_ID, "visibility", "none")
    } catch {}
  }

  // 3. Provider-dependent Labels / Place Names overlay
  const labelsSupported = activeViewDef.capabilities?.supports_labels || activeViewDef.capabilities?.supports_places
  const labelsUrl = !isCustomMode && labelsSupported ? (activeViewDef.labels_tile_url || activeViewDef.places_tile_url) : ""
  const wantLabels = Boolean(labelsUrl && visible && (showLabels || showPlaces))
  if (wantLabels) {
    const existingLabels = map.getSource(BASEMAP_LABELS_SOURCE_ID)
    if (!existingLabels || existingLabels._currentTileUrl !== labelsUrl) {
      safeRemoveLayerAndSource(map, [BASEMAP_LABELS_LAYER_ID], BASEMAP_LABELS_SOURCE_ID)
      try {
        map.addSource(BASEMAP_LABELS_SOURCE_ID, {
          type: "raster",
          tiles: [labelsUrl],
          tileSize: 256,
          maxzoom: maxZoom,
        })
        const added = map.getSource(BASEMAP_LABELS_SOURCE_ID)
        if (added) added._currentTileUrl = labelsUrl
        map.addLayer(
          {
            id: BASEMAP_LABELS_LAYER_ID,
            type: "raster",
            source: BASEMAP_LABELS_SOURCE_ID,
            paint: { "raster-opacity": safeOpacity },
          },
          beforeId
        )
      } catch {}
    } else if (map.getLayer(BASEMAP_LABELS_LAYER_ID)) {
      try {
        map.setLayoutProperty(BASEMAP_LABELS_LAYER_ID, "visibility", "visible")
        map.setPaintProperty(BASEMAP_LABELS_LAYER_ID, "raster-opacity", safeOpacity)
      } catch {}
    }
  } else if (map.getLayer(BASEMAP_LABELS_LAYER_ID)) {
    try {
      map.setLayoutProperty(BASEMAP_LABELS_LAYER_ID, "visibility", "none")
    } catch {}
  }

  // 4. Custom Base Layer (rendered strictly below workspace layers!)
  if (isCustomMode && customBaseLayer) {
    const rs = customBaseLayer.rendering_source || {}
    const mode = rs.mode || "image"
    const customBeforeId = findFirstWorkspaceLayerId(map)

    if (mode === "tiles" && rs.tile_url_template) {
      const dsId = customBaseLayer.dataset_id || rs.dataset_id
      const resolvedTileUrl = datasetTileUrl(dsId, rs.tile_url_template)
      const existingCustom = map.getSource(CUSTOM_BASE_SOURCE_ID)
      if (!existingCustom || existingCustom._currentKey !== `tiles:${resolvedTileUrl}`) {
        safeRemoveLayerAndSource(
          map,
          [CUSTOM_BASE_RASTER_LAYER_ID, CUSTOM_BASE_VECTOR_FILL_ID, CUSTOM_BASE_VECTOR_LINE_ID],
          CUSTOM_BASE_SOURCE_ID
        )
        try {
          const opts = {
            type: "raster",
            tiles: [resolvedTileUrl],
            tileSize: rs.tile_size || 256,
            minzoom: rs.tile_min_zoom ?? 0,
            maxzoom: rs.tile_max_zoom ?? 22,
          }
          if (Array.isArray(customBaseLayer.extent) && customBaseLayer.extent.length === 4) {
            opts.bounds = customBaseLayer.extent.map(Number)
          }
          map.addSource(CUSTOM_BASE_SOURCE_ID, opts)
          const added = map.getSource(CUSTOM_BASE_SOURCE_ID)
          if (added) added._currentKey = `tiles:${resolvedTileUrl}`
          map.addLayer(
            {
              id: CUSTOM_BASE_RASTER_LAYER_ID,
              type: "raster",
              source: CUSTOM_BASE_SOURCE_ID,
              paint: { "raster-opacity": safeOpacity },
            },
            customBeforeId
          )
        } catch {}
      }
      if (map.getLayer(CUSTOM_BASE_RASTER_LAYER_ID)) {
        try {
          map.setPaintProperty(CUSTOM_BASE_RASTER_LAYER_ID, "raster-opacity", safeOpacity)
          map.setLayoutProperty(CUSTOM_BASE_RASTER_LAYER_ID, "visibility", visible ? "visible" : "none")
        } catch {}
      }
    } else if (mode === "vector" && rs.geojson_url) {
      const geoUrl = /^https?:\/\//i.test(rs.geojson_url)
        ? rs.geojson_url
        : `${API_URL}${rs.geojson_url.startsWith("/") ? "" : "/"}${rs.geojson_url}`
      const existingCustom = map.getSource(CUSTOM_BASE_SOURCE_ID)
      if (!existingCustom || existingCustom._currentKey !== `vector:${geoUrl}`) {
        safeRemoveLayerAndSource(
          map,
          [CUSTOM_BASE_RASTER_LAYER_ID, CUSTOM_BASE_VECTOR_FILL_ID, CUSTOM_BASE_VECTOR_LINE_ID],
          CUSTOM_BASE_SOURCE_ID
        )
        try {
          map.addSource(CUSTOM_BASE_SOURCE_ID, { type: "geojson", data: geoUrl })
          const added = map.getSource(CUSTOM_BASE_SOURCE_ID)
          if (added) added._currentKey = `vector:${geoUrl}`
          map.addLayer(
            {
              id: CUSTOM_BASE_VECTOR_FILL_ID,
              type: "fill",
              source: CUSTOM_BASE_SOURCE_ID,
              paint: { "fill-color": "#3ecf8e", "fill-opacity": safeOpacity * 0.25 },
            },
            customBeforeId
          )
          map.addLayer(
            {
              id: CUSTOM_BASE_VECTOR_LINE_ID,
              type: "line",
              source: CUSTOM_BASE_SOURCE_ID,
              paint: { "line-color": "#24b47e", "line-width": 1.8, "line-opacity": safeOpacity },
            },
            customBeforeId
          )
        } catch {}
      }
      ;[CUSTOM_BASE_VECTOR_FILL_ID, CUSTOM_BASE_VECTOR_LINE_ID].forEach((lid) => {
        if (map.getLayer(lid)) {
          try {
            map.setLayoutProperty(lid, "visibility", visible ? "visible" : "none")
          } catch {}
        }
      })
    } else {
      // Georeferenced raster image mode
      const coords = resolveBaseLayerCoordinates(customBaseLayer)
      const imgUrl = resolveCustomBaseImageUrl(customBaseLayer)
      if (coords && imgUrl) {
        const key = `image:${imgUrl}:${coords.map((c) => c.join(",")).join(";")}`
        const existingCustom = map.getSource(CUSTOM_BASE_SOURCE_ID)
        if (!existingCustom || existingCustom._currentKey !== key) {
          safeRemoveLayerAndSource(
            map,
            [CUSTOM_BASE_RASTER_LAYER_ID, CUSTOM_BASE_VECTOR_FILL_ID, CUSTOM_BASE_VECTOR_LINE_ID],
            CUSTOM_BASE_SOURCE_ID
          )
          try {
            map.addSource(CUSTOM_BASE_SOURCE_ID, {
              type: "image",
              url: imgUrl,
              coordinates: coords,
            })
            const added = map.getSource(CUSTOM_BASE_SOURCE_ID)
            if (added) added._currentKey = key
            map.addLayer(
              {
                id: CUSTOM_BASE_RASTER_LAYER_ID,
                type: "raster",
                source: CUSTOM_BASE_SOURCE_ID,
                paint: { "raster-opacity": safeOpacity },
              },
              customBeforeId
            )
          } catch {}
        }
        if (map.getLayer(CUSTOM_BASE_RASTER_LAYER_ID)) {
          try {
            map.setPaintProperty(CUSTOM_BASE_RASTER_LAYER_ID, "raster-opacity", safeOpacity)
            map.setLayoutProperty(CUSTOM_BASE_RASTER_LAYER_ID, "visibility", visible ? "visible" : "none")
          } catch {}
        }
      }
    }
  } else {
    // Hide/remove custom base layer when switched to a built-in map view (World, Satellite, etc.)
    ;[CUSTOM_BASE_RASTER_LAYER_ID, CUSTOM_BASE_VECTOR_FILL_ID, CUSTOM_BASE_VECTOR_LINE_ID].forEach((lid) => {
      if (map.getLayer(lid)) {
        try {
          map.setLayoutProperty(lid, "visibility", "none")
        } catch {}
      }
    })
  }
}
