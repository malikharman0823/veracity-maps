export const BASEMAP_SOURCE_ID = "basemap"
export const BASEMAP_LAYER_ID = "workspace-basemap"
export const BASEMAP_ROADS_SOURCE_ID = "basemap-roads"
export const BASEMAP_ROADS_LAYER_ID = "workspace-basemap-roads"
export const BASEMAP_LABELS_SOURCE_ID = "basemap-labels"
export const BASEMAP_LABELS_LAYER_ID = "workspace-basemap-labels"
export const CUSTOM_BASE_SOURCE_ID = "workspace-custom-baselayer-source"
export const CUSTOM_BASE_RASTER_LAYER_ID = "workspace-custom-baselayer-raster"
export const CUSTOM_BASE_VECTOR_FILL_ID = "workspace-custom-baselayer-vector-fill"
export const CUSTOM_BASE_VECTOR_LINE_ID = "workspace-custom-baselayer-vector-line"

export function isBackgroundLayerId(layerId) {
  return (
    layerId === BASEMAP_LAYER_ID ||
    layerId === "basemap" ||
    layerId === BASEMAP_ROADS_LAYER_ID ||
    layerId === BASEMAP_LABELS_LAYER_ID ||
    layerId === CUSTOM_BASE_RASTER_LAYER_ID ||
    layerId === CUSTOM_BASE_VECTOR_FILL_ID ||
    layerId === CUSTOM_BASE_VECTOR_LINE_ID
  )
}

export function findFirstWorkspaceLayerId(map) {
  if (!map || typeof map.getStyle !== "function") return undefined
  const style = map.getStyle()
  const layers = style?.layers || []
  const firstForeground = layers.find((l) => !isBackgroundLayerId(l.id))
  return firstForeground?.id
}

export function resolveBaseLayerCoordinates(baseLayer) {
  if (!baseLayer) return null
  const rs = baseLayer.rendering_source || {}
  if (Array.isArray(rs.coordinates) && rs.coordinates.length === 4) {
    const valid = rs.coordinates.every(
      (pt) => Array.isArray(pt) && pt.length >= 2 && Number.isFinite(Number(pt[0])) && Number.isFinite(Number(pt[1]))
    )
    if (valid) return rs.coordinates.map(([x, y]) => [Number(x), Number(y)])
  }
  const ext = Array.isArray(baseLayer.extent) && baseLayer.extent.length === 4
    ? baseLayer.extent.map(Number)
    : Array.isArray(rs.bounds) && rs.bounds.length === 4
      ? rs.bounds.map(Number)
      : null
  if (ext && ext.every(Number.isFinite) && ext[0] < ext[2] && ext[1] < ext[3]) {
    const [w, s, e, n] = ext
    return [
      [w, n],
      [e, n],
      [e, s],
      [w, s],
    ]
  }
  return null
}
