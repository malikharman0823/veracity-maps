/**
 * Workspace panel layout persistence, spatial bookmarks, favorites,
 * map scale calculations, and geodesic measurement helpers for the
 * QGIS-inspired Veracity Maps GIS Workspace.
 */

export const DEFAULT_WORKSPACE_LAYOUT = {
  leftOpen: true,
  leftWidth: 276,
  browserOpen: true,
  layersOpen: true,
  browserSplitRatio: 0.42, // Fraction of left dock height allocated to Browser when both open
  rightOpen: true,
  rightWidth: 312,
  recordsOpen: false,
  recordsHeight: 270,
}

export const LAYOUT_LIMITS = {
  leftMin: 210,
  leftMax: 460,
  rightMin: 250,
  rightMax: 520,
  recordsMin: 170,
  recordsMax: 520,
  splitMin: 0.2,
  splitMax: 0.78,
}

export function clamp(value, min, max) {
  const num = Number(value)
  if (!Number.isFinite(num)) return min
  return Math.max(min, Math.min(max, num))
}

function layoutStorageKey(userId) {
  return `veracity_workspace_layout_v1:${userId || "anon"}`
}

function bookmarksStorageKey(workspaceId) {
  return `veracity_spatial_bookmarks_v1:${workspaceId || "default"}`
}

function favoritesStorageKey(userId) {
  return `veracity_browser_favorites_v1:${userId || "anon"}`
}

export function sanitizeWorkspaceLayout(raw) {
  const src = raw && typeof raw === "object" ? raw : {}
  return {
    leftOpen: typeof src.leftOpen === "boolean" ? src.leftOpen : DEFAULT_WORKSPACE_LAYOUT.leftOpen,
    leftWidth: clamp(
      src.leftWidth ?? DEFAULT_WORKSPACE_LAYOUT.leftWidth,
      LAYOUT_LIMITS.leftMin,
      LAYOUT_LIMITS.leftMax
    ),
    browserOpen:
      typeof src.browserOpen === "boolean" ? src.browserOpen : DEFAULT_WORKSPACE_LAYOUT.browserOpen,
    layersOpen:
      typeof src.layersOpen === "boolean" ? src.layersOpen : DEFAULT_WORKSPACE_LAYOUT.layersOpen,
    browserSplitRatio: clamp(
      src.browserSplitRatio ?? DEFAULT_WORKSPACE_LAYOUT.browserSplitRatio,
      LAYOUT_LIMITS.splitMin,
      LAYOUT_LIMITS.splitMax
    ),
    rightOpen:
      typeof src.rightOpen === "boolean" ? src.rightOpen : DEFAULT_WORKSPACE_LAYOUT.rightOpen,
    rightWidth: clamp(
      src.rightWidth ?? DEFAULT_WORKSPACE_LAYOUT.rightWidth,
      LAYOUT_LIMITS.rightMin,
      LAYOUT_LIMITS.rightMax
    ),
    recordsOpen:
      typeof src.recordsOpen === "boolean" ? src.recordsOpen : DEFAULT_WORKSPACE_LAYOUT.recordsOpen,
    recordsHeight: clamp(
      src.recordsHeight ?? DEFAULT_WORKSPACE_LAYOUT.recordsHeight,
      LAYOUT_LIMITS.recordsMin,
      LAYOUT_LIMITS.recordsMax
    ),
  }
}

export function loadWorkspaceLayout(userId) {
  try {
    const raw = localStorage.getItem(layoutStorageKey(userId))
    if (!raw) return { ...DEFAULT_WORKSPACE_LAYOUT }
    return sanitizeWorkspaceLayout(JSON.parse(raw))
  } catch {
    return { ...DEFAULT_WORKSPACE_LAYOUT }
  }
}

export function saveWorkspaceLayout(userId, layout) {
  try {
    localStorage.setItem(layoutStorageKey(userId), JSON.stringify(sanitizeWorkspaceLayout(layout)))
  } catch {}
}

export function resetWorkspaceLayout(userId) {
  const fresh = { ...DEFAULT_WORKSPACE_LAYOUT }
  saveWorkspaceLayout(userId, fresh)
  return fresh
}

export function loadSpatialBookmarks(workspaceId) {
  try {
    const raw = localStorage.getItem(bookmarksStorageKey(workspaceId))
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

export function saveSpatialBookmarks(workspaceId, bookmarks) {
  try {
    localStorage.setItem(bookmarksStorageKey(workspaceId), JSON.stringify(bookmarks || []))
  } catch {}
}

export function loadBrowserFavorites(userId) {
  try {
    const raw = localStorage.getItem(favoritesStorageKey(userId))
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

export function saveBrowserFavorites(userId, favorites) {
  try {
    localStorage.setItem(favoritesStorageKey(userId), JSON.stringify(favorites || []))
  } catch {}
}

/**
 * Compute approximate cartographic scale denominator (e.g. 1:25,000) from MapLibre zoom & latitude.
 * Uses standard 96 DPI (0.000264583 m/px) and Web Mercator 512px tile earth resolution.
 */
export function computeMapScaleDenominator(zoom, latitude = 0) {
  const z = Number(zoom)
  const lat = Number(latitude)
  if (!Number.isFinite(z)) return null
  const clampedLat = clamp(Number.isFinite(lat) ? lat : 0, -85, 85)
  const metersPerPixel = (156543.03392 * Math.cos((clampedLat * Math.PI) / 180)) / Math.pow(2, z)
  const denom = Math.max(1, Math.round(metersPerPixel / 0.0002645833))
  return denom
}

export function formatMapScale(zoom, latitude = 0) {
  const denom = computeMapScaleDenominator(zoom, latitude)
  if (!denom) return "1:—"
  return `1:${denom.toLocaleString("en-US")}`
}

export function computeZoomPercent(zoom) {
  const z = Number(zoom)
  if (!Number.isFinite(z)) return 100
  return Math.max(1, Math.round((z / 12) * 100))
}

const EARTH_RADIUS_M = 6371008.8

export function haversineDistanceMeters(a, b) {
  if (!Array.isArray(a) || !Array.isArray(b) || a.length < 2 || b.length < 2) return 0
  const [lng1, lat1] = a.map(Number)
  const [lng2, lat2] = b.map(Number)
  if (![lng1, lat1, lng2, lat2].every(Number.isFinite)) return 0
  const toRad = (deg) => (deg * Math.PI) / 180
  const dLat = toRad(lat2 - lat1)
  const dLng = toRad(lng2 - lng1)
  const sinLat = Math.sin(dLat / 2)
  const sinLng = Math.sin(dLng / 2)
  const h = sinLat * sinLat + Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * sinLng * sinLng
  return 2 * EARTH_RADIUS_M * Math.asin(Math.min(1, Math.sqrt(h)))
}

export function polylineDistanceMeters(coords) {
  if (!Array.isArray(coords) || coords.length < 2) return 0
  let total = 0
  for (let i = 1; i < coords.length; i += 1) {
    total += haversineDistanceMeters(coords[i - 1], coords[i])
  }
  return total
}

/**
 * Spherical excess polygon area in square meters for a [lng, lat] ring.
 */
export function sphericalPolygonAreaSqMeters(coords) {
  if (!Array.isArray(coords) || coords.length < 3) return 0
  const toRad = (deg) => (deg * Math.PI) / 180
  const ring = [...coords, coords[0]]
  let sum = 0
  for (let i = 0; i < ring.length - 1; i += 1) {
    const [lng1, lat1] = ring[i]
    const [lng2, lat2] = ring[i + 1]
    sum += (toRad(lng2) - toRad(lng1)) * (2 + Math.sin(toRad(lat1)) + Math.sin(toRad(lat2)))
  }
  return Math.abs((sum * EARTH_RADIUS_M * EARTH_RADIUS_M) / 2)
}

export function formatDistance(meters) {
  const m = Number(meters)
  if (!Number.isFinite(m) || m < 0) return "0 m"
  if (m >= 1000) return `${(m / 1000).toFixed(3)} km`
  return `${m.toFixed(1)} m`
}

export function formatArea(sqMeters) {
  const a = Number(sqMeters)
  if (!Number.isFinite(a) || a < 0) return "0 m²"
  if (a >= 1_000_000) return `${(a / 1_000_000).toFixed(3)} km²`
  if (a >= 10_000) return `${(a / 10_000).toFixed(2)} ha`
  return `${a.toFixed(1)} m²`
}
