import axios from "axios"
import { findDatasetSessionId, isBackendSessionId } from "./lib/sessionLoading"
import { syncUploadSession } from "./lib/uploadSession"

function requiredSetting(name) {
  const value = import.meta.env[name]
  if (!value) throw new Error(`Missing required frontend setting: ${name}`)
  return value
}

export const API_URL = requiredSetting("VITE_API_URL").replace(/\/$/, "")
export const BASEMAP_TILE_URL = requiredSetting("VITE_BASEMAP_TILE_URL")
export const BASEMAP_ATTRIBUTION = requiredSetting("VITE_BASEMAP_ATTRIBUTION")

export const SESSION_STORAGE_KEY = "veracity_maps_active_session_id"
export const AUTH_TOKEN_KEY = "veracity_maps_auth_token"

export function getAuthToken() {
  try {
    return localStorage.getItem(AUTH_TOKEN_KEY) || sessionStorage.getItem(AUTH_TOKEN_KEY) || null
  } catch {
    return null
  }
}

export function setAuthToken(token) {
  try {
    if (token) {
      localStorage.setItem(AUTH_TOKEN_KEY, token)
      sessionStorage.setItem(AUTH_TOKEN_KEY, token)
    } else {
      localStorage.removeItem(AUTH_TOKEN_KEY)
      sessionStorage.removeItem(AUTH_TOKEN_KEY)
    }
  } catch {
    // fallback
  }
}

export function clearAuthToken() {
  setAuthToken(null)
}

export function getActiveSessionId() {
  try {
    return (
      sessionStorage.getItem(SESSION_STORAGE_KEY) ||
      localStorage.getItem(SESSION_STORAGE_KEY) ||
      null
    )
  } catch {
    return null
  }
}

export function setActiveSessionId(sessionId) {
  try {
    if (sessionId) {
      sessionStorage.setItem(SESSION_STORAGE_KEY, sessionId)
      localStorage.setItem(SESSION_STORAGE_KEY, sessionId)
    } else {
      sessionStorage.removeItem(SESSION_STORAGE_KEY)
      localStorage.removeItem(SESSION_STORAGE_KEY)
    }
  } catch {
    // Local storage access error fallback
  }
}

export function clearActiveSessionId() {
  lastVerifiedSessionId = null
  lastVerifiedAt = 0
  setActiveSessionId(null)
}

const client = axios.create({ baseURL: API_URL, timeout: 30_000, withCredentials: true })

client.interceptors.request.use((config) => {
  const sid = getActiveSessionId()
  if (sid) {
    config.headers["X-Session-ID"] = sid
  }
  const token = getAuthToken()
  if (token) {
    config.headers["Authorization"] = `Bearer ${token}`
  }
  return config
})

client.interceptors.response.use(
  (response) => {
    // Note: Never silently overwrite active session from arbitrary response headers,
    // which causes cross-tab contamination when switching maps or workspaces.
    return response
  },
  (error) => {
    const status = error.response?.status
    const code = error.response?.data?.error?.code
    if (status === 404 && code === "SESSION_NOT_FOUND") {
      clearActiveSessionId()
    }
    return Promise.reject(error)
  }
)

// --- Authentication APIs ---

export async function sendOtp(email, purpose, name = "") {
  const payload = { email, purpose }
  if (name) payload.name = name
  const response = await client.post("/api/auth/otp/send", payload)
  return response.data.data
}

export async function verifyOtp(email, code, purpose, name = "") {
  const payload = { email, code, purpose }
  if (name) payload.name = name
  const response = await client.post("/api/auth/otp/verify", payload)
  const data = response.data.data
  if (data?.token) {
    setAuthToken(data.token)
  }
  return data
}

export async function registerUser(name, email, password) {
  const response = await client.post("/api/auth/register", { name, email, password })
  const data = response.data.data
  if (data?.token) {
    setAuthToken(data.token)
  }
  return data
}

export async function loginUser(email, password) {
  const response = await client.post("/api/auth/login", { email, password })
  const data = response.data.data
  if (data?.token) {
    setAuthToken(data.token)
  }
  return data
}

export async function testLogin(email, name = "") {
  const response = await client.post("/api/auth/test-login", { email, name })
  const data = response.data.data
  if (data?.token) {
    setAuthToken(data.token)
  }
  return data
}

export async function logoutUser() {
  try {
    await client.post("/api/auth/logout")
  } finally {
    clearAuthToken()
  }
  return true
}

export async function getCurrentUser() {
  const token = getAuthToken()
  if (!token) return null
  const response = await client.get("/api/auth/me")
  return response.data.data
}

// --- Project Ownership & Dashboard APIs ---

export async function listUserProjects() {
  const response = await client.get("/api/projects")
  return response.data.data?.projects || []
}

export async function createUserProject(name, description = "") {
  const response = await client.post("/api/projects", { name, description })
  return response.data.data
}

export async function getUserProject(projectId) {
  const response = await client.get(`/api/projects/${encodeURIComponent(projectId)}`)
  return response.data.data
}

export async function updateUserProject(projectId, { name, description }) {
  const response = await client.patch(`/api/projects/${encodeURIComponent(projectId)}`, { name, description })
  return response.data.data
}

export async function deleteUserProject(projectId) {
  const response = await client.delete(`/api/projects/${encodeURIComponent(projectId)}`)
  return response.data.data
}

// --- Project Sharing & Collaboration APIs ---

export async function listSharedProjects() {
  const response = await client.get("/api/projects/shared")
  return response.data.data?.shared_projects || []
}

export async function leaveSharedProject(projectId) {
  const response = await client.post(`/api/projects/${encodeURIComponent(projectId)}/leave`)
  return response.data.data
}

export async function listUserInvitations() {
  const response = await client.get("/api/projects/invitations")
  return response.data.data?.invitations || []
}

export async function respondToInvitation(invitationId, accept) {
  const response = await client.post(`/api/projects/invitations/${encodeURIComponent(invitationId)}/respond`, {
    accept: !!accept,
  })
  return response.data.data
}

export async function shareProject(projectId, email, role = "viewer") {
  const response = await client.post(`/api/projects/${encodeURIComponent(projectId)}/share`, {
    email,
    role,
  })
  return response.data.data
}

export async function listProjectMembers(projectId) {
  const response = await client.get(`/api/projects/${encodeURIComponent(projectId)}/members`)
  return response.data.data?.members || []
}

export async function updateProjectMemberRole(projectId, memberId, role) {
  const response = await client.patch(
    `/api/projects/${encodeURIComponent(projectId)}/members/${encodeURIComponent(memberId)}`,
    { role }
  )
  return response.data.data
}

export async function removeProjectMember(projectId, memberId) {
  const response = await client.delete(
    `/api/projects/${encodeURIComponent(projectId)}/members/${encodeURIComponent(memberId)}`
  )
  return response.data.data
}

export async function listProjectActivities(projectId) {
  const response = await client.get(`/api/projects/${encodeURIComponent(projectId)}/activities`)
  return response.data.data?.activities || []
}

// --- Unified Invitations API ---

export async function listAllInvitations() {
  const response = await client.get("/api/invitations")
  return response.data.data || { project_invitations: [], total: 0 }
}

export async function acceptInvitation(invitationId) {
  const response = await client.post(`/api/invitations/${encodeURIComponent(invitationId)}/accept`)
  return response.data.data
}

export async function rejectInvitation(invitationId) {
  const response = await client.post(`/api/invitations/${encodeURIComponent(invitationId)}/reject`)
  return response.data.data
}

export async function createSession() {
  const response = await client.post("/sessions")
  const data = response.data.data
  if (data?.session_id) {
    setActiveSessionId(data.session_id)
  }
  return data
}

export async function getSession(sessionId) {
  const sid = sessionId || getActiveSessionId()
  if (!sid) return null
  try {
    const response = await client.get(`/sessions/${encodeURIComponent(sid)}`, {
      validateStatus: (status) => (status >= 200 && status < 300) || status === 404,
    })
    if (response.status === 404) {
      return null
    }
    return response.data.data
  } catch {
    return null
  }
}

export async function listSessions() {
  const response = await client.get("/sessions")
  return response.data.data?.sessions || []
}

export async function ensureDatasetSession(datasetId) {
  const existing = getActiveSessionId()
  if (existing && isBackendSessionId(existing) && lastVerifiedSessionId === existing && Date.now() - lastVerifiedAt < 30_000) {
    return existing
  }
  if (!datasetId) return ensureActiveSession()

  try {
    const sessions = await listSessions()
    const sessionId = findDatasetSessionId(sessions, datasetId)
    if (sessionId && isBackendSessionId(sessionId)) {
      setActiveSessionId(sessionId)
      lastVerifiedSessionId = sessionId
      lastVerifiedAt = Date.now()
      return sessionId
    }
  } catch {
    // Fall back to active session
  }

  return ensureActiveSession()
}

export function switchSession(sessionId, targetDatasetId = null) {
  if (!sessionId) return
  setActiveSessionId(sessionId)
  if (targetDatasetId) {
    window.location.href = `/datasets/${encodeURIComponent(targetDatasetId)}/workspace`
  } else {
    window.location.href = `/workspace`
  }
}

export async function endSession(sessionId) {
  const sid = sessionId || getActiveSessionId()
  if (!sid) return true
  try {
    await client.post(`/sessions/${encodeURIComponent(sid)}/end`)
  } finally {
    if (!sessionId || sessionId === getActiveSessionId()) {
      clearActiveSessionId()
    }
  }
  return true
}

export async function deleteSession(sessionId) {
  const sid = sessionId || getActiveSessionId()
  if (!sid) return true
  try {
    await client.delete(`/sessions/${encodeURIComponent(sid)}`)
  } finally {
    if (!sessionId || sessionId === getActiveSessionId()) {
      clearActiveSessionId()
    }
  }
  return true
}

export async function getSessionMapConfig(sessionId) {
  const sid = sessionId || getActiveSessionId()
  if (!sid) return null
  const response = await client.get(`/sessions/${encodeURIComponent(sid)}/map-config`)
  return response.data.data
}

export async function saveSessionMapConfig(sessionId, configuration) {
  const sid = sessionId || getActiveSessionId()
  if (!sid) return null
  const response = await client.put(`/sessions/${encodeURIComponent(sid)}/map-config`, configuration)
  return response.data.data
}

let ensureSessionPromise = null
let lastVerifiedSessionId = null
let lastVerifiedAt = 0

export async function ensureActiveSession() {
  const current = getActiveSessionId()
  if (current && isBackendSessionId(current) && lastVerifiedSessionId === current && Date.now() - lastVerifiedAt < 30_000) {
    return current
  }

  if (ensureSessionPromise) {
    return ensureSessionPromise
  }

  ensureSessionPromise = (async () => {
    const existing = getActiveSessionId()
    if (existing) {
      if (!isBackendSessionId(existing)) {
        clearActiveSessionId()
      } else {
        try {
          const info = await getSession(existing)
          if (info && (info.session_id || info.id)) {
            const validId = info.session_id || info.id
            lastVerifiedSessionId = validId
            lastVerifiedAt = Date.now()
            return validId
          }
          clearActiveSessionId()
        } catch {
          clearActiveSessionId()
        }
      }
    }
    const created = await createSession()
    if (created?.session_id) {
      lastVerifiedSessionId = created.session_id
      lastVerifiedAt = Date.now()
    }
    return created.session_id
  })().finally(() => {
    ensureSessionPromise = null
  })

  return ensureSessionPromise
}

export async function listDatasets(signal) {
  const response = await client.get("/datasets", { signal })
  return response.data.data.datasets
}

export async function getDataset(datasetId, signal) {
  const response = await client.get(`/datasets/${encodeURIComponent(datasetId)}`, {
    signal,
  })
  return response.data.data
}

export async function uploadDataset(file, onUploadProgress, projectId, mapId) {
  const body = new FormData()
  body.append("file", file)
  if (projectId) body.append("project_id", projectId)
  if (mapId) body.append("map_id", mapId)
  const headers = {}
  if (projectId) headers["X-Project-Id"] = projectId
  if (mapId) headers["X-Map-Id"] = mapId
  const response = await client.post("/upload", body, {
    headers,
    onUploadProgress,
    timeout: 10 * 60_000,
  })
  syncUploadSession(response, setActiveSessionId)
  return response.data.data
}

export async function importDatasetZip(file, onUploadProgress, projectId, mapId) {
  const body = new FormData()
  body.append("file", file)
  if (projectId) body.append("project_id", projectId)
  if (mapId) body.append("map_id", mapId)
  const headers = {}
  if (projectId) headers["X-Project-Id"] = projectId
  if (mapId) headers["X-Map-Id"] = mapId
  const response = await client.post("/datasets/import-zip", body, {
    headers,
    onUploadProgress,
    timeout: 10 * 60_000,
  })
  syncUploadSession(response, setActiveSessionId)
  return response.data.data
}

export async function getPbfPreview(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/pbf-preview`,
    { signal, timeout: 10 * 60_000 },
  )
  return response.data.data
}

export async function getVectorPreview(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/vector-preview`,
    { signal, timeout: 10 * 60_000 },
  )
  return response.data.data
}

export async function deleteDataset(datasetId) {
  const response = await client.delete(
    `/datasets/${encodeURIComponent(datasetId)}`,
  )
  return response.data.data
}

export function datasetTileUrl(datasetId, template) {
  const sid = getActiveSessionId()
  const sessionQuery = sid ? `?session_id=${encodeURIComponent(sid)}` : ""
  if (template) {
    const base = /^https?:\/\//i.test(template)
      ? template
      : `${API_URL}${template.startsWith("/") ? "" : "/"}${template}`
    return sid ? `${base}${base.includes("?") ? "&" : "?"}session_id=${encodeURIComponent(sid)}` : base
  }
  return `${API_URL}/datasets/${encodeURIComponent(datasetId)}/tiles/{z}/{x}/{y}${sessionQuery}`
}

export function mbtilesTileUrl(datasetId) {
  const sid = getActiveSessionId()
  const sessionQuery = sid ? `?session_id=${encodeURIComponent(sid)}` : ""
  return `${API_URL}/datasets/${encodeURIComponent(datasetId)}/tiles/{z}/{x}/{y}${sessionQuery}`
}

export function datasetSourceImageUrl(datasetId, options = {}) {
  const sid = getActiveSessionId()
  const params = new URLSearchParams()
  const bust = options?.bust !== undefined ? options.bust : Date.now()
  params.set("t", String(bust))
  if (options?.variant) {
    params.set("variant", String(options.variant))
  }
  if (sid) {
    params.set("session_id", sid)
  }
  return `${API_URL}/datasets/${encodeURIComponent(datasetId)}/source-image?${params.toString()}`
}

export async function listControlPoints(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/control-points`,
    { signal },
  )
  return response.data.data.points
}

export async function createControlPoint(datasetId, point) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/control-points`,
    { dataset_id: datasetId, ...point },
  )
  return response.data.data
}

export async function updateControlPoint(controlPointId, point) {
  const response = await client.put(
    `/control-points/${encodeURIComponent(controlPointId)}`,
    point,
  )
  return response.data.data
}

export async function removeControlPoint(controlPointId) {
  const response = await client.delete(
    `/control-points/${encodeURIComponent(controlPointId)}`,
  )
  return response.data.data
}

export async function georeferenceDataset(datasetId, action = "calculate") {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/georeference`,
    {
      action,
      transformation: "auto",
      output_format: "tif",
      target_crs: "EPSG:4326",
    },
    { timeout: 10 * 60_000 },
  )
  return response.data.data
}

export async function getGeoreferenceConfig(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/georeference`,
    { signal },
  )
  return response.data.data.configuration
}

export async function listIconPoints(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/icon-points`,
    { signal },
  )
  return response.data.data.points
}

export async function createIconPoint(datasetId, point) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/icon-points`,
    point,
  )
  return response.data.data
}

export async function updateIconPoint(iconPointId, point) {
  const response = await client.put(
    `/icon-points/${encodeURIComponent(iconPointId)}`,
    point,
  )
  return response.data.data
}

export async function removeIconPoint(iconPointId) {
  const response = await client.delete(
    `/icon-points/${encodeURIComponent(iconPointId)}`,
  )
  return response.data.data
}

export const deleteIconPoint = removeIconPoint

export async function getIconPoint(pointId) {
  const response = await client.get(`/icon-points/${encodeURIComponent(pointId)}`)
  return response.data.data
}

export async function getMapProject(datasetId, signal) {
  const response = await client.get(`/datasets/${encodeURIComponent(datasetId)}/project`, { signal })
  return response.data
}

export async function updateMapProject(datasetId, name, configuration) {
  const response = await client.put(
    `/datasets/${encodeURIComponent(datasetId)}/project`,
    { name, configuration }
  )
  return response.data
}

export async function getWorkspace(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/workspace`,
    { signal },
  )
  return response.data.data
}

export async function attachWorkspaceDataset(primaryDatasetId, datasetId) {
  if (!primaryDatasetId || primaryDatasetId === "undefined" || primaryDatasetId === "null") {
    throw new Error("Cannot attach to undefined primary dataset")
  }
  const response = await client.post(
    `/datasets/${encodeURIComponent(primaryDatasetId)}/workspace/members`,
    { dataset_id: datasetId },
  )
  return response.data.data
}

export async function patchWorkspaceMember(primaryDatasetId, datasetId, patch) {
  if (!primaryDatasetId || primaryDatasetId === "undefined" || primaryDatasetId === "null") {
    return null
  }
  const response = await client.patch(
    `/datasets/${encodeURIComponent(primaryDatasetId)}/workspace/members/${encodeURIComponent(datasetId)}`,
    patch,
  )
  return response.data.data
}

export async function detachWorkspaceDataset(primaryDatasetId, datasetId) {
  if (!primaryDatasetId || primaryDatasetId === "undefined" || primaryDatasetId === "null") {
    return null
  }
  const response = await client.delete(
    `/datasets/${encodeURIComponent(primaryDatasetId)}/workspace/members/${encodeURIComponent(datasetId)}`,
  )
  return response.data.data
}

export async function patchWorkspaceState(primaryDatasetId, patch) {
  if (!primaryDatasetId || primaryDatasetId === "undefined" || primaryDatasetId === "null") {
    return null
  }
  const response = await client.patch(
    `/datasets/${encodeURIComponent(primaryDatasetId)}/workspace/state`,
    patch,
  )
  return response.data.data
}

export async function getWorkspaceRecords(primaryDatasetId, options = {}, signal) {
  const params = new URLSearchParams()
  const values = {
    page: options.page || 1,
    page_size: options.pageSize || 25,
    search: options.search,
    member_dataset_id: options.datasetId,
    kind: options.kind,
    status: options.status,
    sort: options.sort || "name",
    order: options.order || "asc",
  }
  Object.entries(values).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") {
      params.set(key, String(value))
    }
  })
  const response = await client.get(
    `/datasets/${encodeURIComponent(primaryDatasetId)}/workspace/records?${params}`,
    { signal },
  )
  return response.data.data
}

export async function importMapProject(file, onUploadProgress) {
  const body = new FormData()
  body.append("file", file)
  const response = await client.post("/map-projects/import", body, {
    onUploadProgress,
    timeout: 10 * 60_000,
  })
  return response.data
}

export async function importProjectFolder(files, onUploadProgress) {
  const body = new FormData()
  for (const file of files) {
    body.append("files", file, file.name)
    body.append("paths", file.webkitRelativePath || file.name)
  }
  const response = await client.post("/map-projects/import-folder", body, {
    onUploadProgress,
    timeout: 10 * 60_000,
  })
  return response.data
}

function safeDownloadStem(value) {
  const stem = String(value || "map_project")
    .replace(/[^A-Za-z0-9._-]+/g, "_")
    .replace(/^[._-]+|[._-]+$/g, "")
    .slice(0, 80)
  return stem || "map_project"
}

function responseFilename(disposition) {
  if (!disposition) return null
  const encoded = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
  if (encoded) {
    try {
      return decodeURIComponent(encoded)
    } catch {
      return encoded
    }
  }
  return disposition.match(/filename="([^"]+)"/i)?.[1]
    || disposition.match(/filename=([^;]+)/i)?.[1]?.trim()
    || null
}

function safeDownloadFilename(value, fallback) {
  const basename = String(value || "").replace(/\\/g, "/").split("/").pop()
  const safe = basename?.replace(/[<>:"/\\|?*\u0000-\u001F]/g, "_").trim()
  return safe && safe !== "." && safe !== ".." ? safe : fallback
}

export async function downloadProjectExport(datasetId, projectName, exportPayload = null) {
  let response
  try {
    if (exportPayload && (exportPayload.layer_ids || exportPayload.options || exportPayload.building_ids || exportPayload.format)) {
      response = await client.post(
        `/datasets/${encodeURIComponent(datasetId)}/export`,
        { ...exportPayload, format: exportPayload.format || "custom_local_tiles", project_name: projectName },
        {
          responseType: "blob",
          timeout: 10 * 60_000,
          headers: { Accept: "application/octet-stream, application/zip, application/x-sqlite3" },
        },
      )
    } else {
      response = await client.get(
        `/datasets/${encodeURIComponent(datasetId)}/export`,
        { responseType: "blob", timeout: 10 * 60_000 },
      )
    }
  } catch (error) {
    if (error.response?.data instanceof Blob) {
      try {
        error.response.data = JSON.parse(await error.response.data.text())
      } catch {
        // Keep the original Blob when the server did not return JSON.
      }
    }
    throw error
  }

  if (!response.data || (response.data instanceof Blob && response.data.size === 0)) {
    throw new Error("The exported file is empty or invalid.")
  }

  let fallback = `${safeDownloadStem(projectName)}.zip`
  if (exportPayload?.format === "mbtiles") {
    fallback = `${safeDownloadStem(projectName)}.mbtiles`
  } else if (exportPayload?.format === "base_layer" || exportPayload?.format === "basemap") {
    fallback = "veracity-base-map.zip"
  }
  const filename = safeDownloadFilename(
    responseFilename(response.headers.get?.("content-disposition") || response.headers["content-disposition"]),
    fallback,
  )
  const objectUrl = URL.createObjectURL(response.data)
  try {
    const link = document.createElement("a")
    link.href = objectUrl
    link.download = filename
    document.body.appendChild(link)
    link.click()
    link.remove()
  } finally {
    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 0)
  }
  return filename
}

export async function getExportProgress(datasetId, signal) {
  const response = await client.get(`/datasets/${encodeURIComponent(datasetId)}/export/progress`, { signal })
  return response.data?.data || response.data
}

export async function cancelExport(datasetId) {
  const response = await client.post(`/datasets/${encodeURIComponent(datasetId)}/export/cancel`)
  return response.data?.data || response.data
}



const DEFAULT_ICONS = [
  "cinema",
  "fingerprint",
  "key",
  "locked-with-key",
  "locked",
  "person-police-officer",
  "security-lock",
  "shield",
  "unlocked",
  "video-call",
]

export async function listIcons(signal) {
  try {
    const response = await client.get(`/icons`, { signal })
    const body = response.data?.data || response.data || {}
    const iconList = body.icons || DEFAULT_ICONS
    const catalog = body.catalog || iconList.map((id) => ({ id, name: id, path: `/icons/${id}.png` }))
    const categories = body.categories || ["General"]
    return {
      icons: iconList,
      catalog,
      categories,
      data: { icons: iconList, catalog, categories },
      [Symbol.iterator]: function* () {
        for (const item of iconList) yield item
      },
      map: (...args) => iconList.map(...args),
      filter: (...args) => iconList.filter(...args),
      forEach: (...args) => iconList.forEach(...args),
      find: (...args) => iconList.find(...args),
      some: (...args) => iconList.some(...args),
      length: iconList.length,
    }
  } catch {
    return {
      icons: DEFAULT_ICONS,
      catalog: DEFAULT_ICONS.map((id) => ({ id, name: id, path: `/icons/${id}.png` })),
      categories: ["Security & Access"],
      data: { icons: DEFAULT_ICONS },
      [Symbol.iterator]: function* () {
        for (const item of DEFAULT_ICONS) yield item
      },
      map: (...args) => DEFAULT_ICONS.map(...args),
      filter: (...args) => DEFAULT_ICONS.filter(...args),
      forEach: (...args) => DEFAULT_ICONS.forEach(...args),
      find: (...args) => DEFAULT_ICONS.find(...args),
      some: (...args) => DEFAULT_ICONS.some(...args),
      length: DEFAULT_ICONS.length,
    }
  }
}

export async function processRasterDataset(datasetId) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/process-raster`,
    { target_crs: "EPSG:3857" },
    { timeout: 10 * 60_000 },
  )
  return response.data.data
}

export async function generateDatasetTiles(datasetId) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/generate-tiles`,
    { min_zoom: 0, max_zoom: 22 },
    { timeout: 10 * 60_000 },
  )
  return response.data.data
}

export async function getBackgroundRemoval(datasetId) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/background-removal`,
  )
  return response.data.data
}

export async function previewBackgroundRemoval(datasetId, { color = "#ffffff", threshold = 30, edgeSoftness = 10 } = {}) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/background-removal`,
    {
      action: "preview",
      color,
      threshold,
      edge_softness: edgeSoftness,
    },
  )
  return response.data.data
}

export async function applyBackgroundRemoval(datasetId, { color = "#ffffff", threshold = 30, edgeSoftness = 10, applyToAllFloors = false } = {}) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/background-removal`,
    {
      action: "apply",
      color,
      threshold,
      edge_softness: edgeSoftness,
      apply_to_all_floors: applyToAllFloors,
    },
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

export async function resetBackgroundRemoval(datasetId) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/background-removal`,
    {
      action: "reset",
    },
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

export async function getRasterTransform(datasetId) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/transform`,
  )
  return response.data.data
}

export async function previewRasterTransform(datasetId, { corners, mode = "free", rotationAngle = null, lockAspectRatio = false } = {}) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/transform`,
    {
      action: "preview",
      mode,
      corners,
      rotation_angle: rotationAngle,
      lock_aspect_ratio: lockAspectRatio,
    },
    { timeout: 2 * 60_000 },
  )
  return response.data.data
}

export async function applyRasterTransform(datasetId, { corners, mode = "free", rotationAngle = null, lockAspectRatio = false, resampling = "bilinear" } = {}) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/transform`,
    {
      action: "apply",
      mode,
      corners,
      rotation_angle: rotationAngle,
      lock_aspect_ratio: lockAspectRatio,
      resampling,
    },
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

export async function undoRasterTransform(datasetId) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/transform`,
    { action: "undo" },
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

export async function redoRasterTransform(datasetId) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/transform`,
    { action: "redo" },
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

export async function resetRasterTransform(datasetId) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/transform`,
    { action: "reset" },
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

export function readableApiError(error, fallbackMessage) {
  const data = error.response?.data
  const apiError = data?.error
  if (typeof apiError === "string") {
    return {
      message: apiError,
      details: data?.message || "Please check the file and try again.",
    }
  }
  if (apiError && typeof apiError === "object") {
    return {
      message: apiError.message || apiError.code || fallbackMessage,
      details: apiError.details || apiError.code || "Please check the file and try again.",
    }
  }
  if (data?.message) {
    return {
      message: data.message,
      details: data.details || "Please try again.",
    }
  }
  if (!error.response) {
    return {
      message: "Unable to reach the server.",
      details: "Check that the backend is running, then try again.",
    }
  }
  return {
    message: fallbackMessage,
    details: `Status ${error.response.status}: ${error.response.statusText || "Bad Request"}`,
  }
}

// -------------------------------------------------------------
// Icon Catalog & Icon Points API
// -------------------------------------------------------------

export function getIconUrl(iconId) {
  if (!iconId) return ""
  if (iconId.startsWith("/") || iconId.startsWith("http")) return iconId
  return `${API_URL}/icons/${encodeURIComponent(iconId)}`
}

// -------------------------------------------------------------
// Building & Floor Management API
// -------------------------------------------------------------

export async function listBuildings() {
  const response = await client.get("/buildings")
  return response.data.data || []
}

export async function createBuilding(data) {
  const response = await client.post("/buildings", data)
  return response.data.data
}

export async function getBuilding(buildingId) {
  const response = await client.get(`/buildings/${encodeURIComponent(buildingId)}`)
  return response.data.data
}

export async function updateBuilding(buildingId, data) {
  const response = await client.put(`/buildings/${encodeURIComponent(buildingId)}`, data)
  return response.data.data
}

export async function deleteBuilding(buildingId) {
  const response = await client.delete(`/buildings/${encodeURIComponent(buildingId)}`)
  return response.data.data
}

export async function addBuildingFloor(buildingId, floorData) {
  const response = await client.post(`/buildings/${encodeURIComponent(buildingId)}/floors`, floorData)
  return response.data.data
}

export async function updateBuildingFloor(buildingId, floorId, floorData) {
  const response = await client.put(`/buildings/${encodeURIComponent(buildingId)}/floors/${encodeURIComponent(floorId)}`, floorData)
  return response.data.data
}

export async function deleteBuildingFloor(buildingId, floorId) {
  const response = await client.delete(`/buildings/${encodeURIComponent(buildingId)}/floors/${encodeURIComponent(floorId)}`)
  return response.data.data
}

export async function propagateBuildingGeoreference(buildingId) {
  const response = await client.post(`/buildings/${encodeURIComponent(buildingId)}/propagate`)
  return response.data.data
}


// -------------------------------------------------------------
// Multi-Dataset ZIP Inspection & Import API
// -------------------------------------------------------------

export async function inspectArchive(file, { onUploadProgress } = {}) {
  const formData = new FormData()
  formData.append("file", file)
  const response = await client.post("/datasets/inspect-archive", formData, {
    onUploadProgress,
    timeout: 10 * 60_000,
  })
  return response.data.data
}

export async function importArchiveSelected(token, selections) {
  const response = await client.post("/datasets/import-archive-selected", {
    archive_token: token,
    token,
    selections,
  })
  return response.data.data
}

// -------------------------------------------------------------
// PDF Multi-Page Import API
// -------------------------------------------------------------

export function pdfThumbnailUrl(documentId, pageNumber) {
  const sid = getActiveSessionId()
  const q = sid ? `?session_id=${encodeURIComponent(sid)}` : ""
  return `${API_URL}/documents/${encodeURIComponent(documentId)}/pdf-thumbnail/${encodeURIComponent(pageNumber)}${q}`
}

export async function importPdfPages(payload) {
  const response = await client.post("/datasets/import-pdf-pages", payload, {
    timeout: 5 * 60_000,
  })
  return response.data.data
}

// -------------------------------------------------------------
// Universal File Inspection, OpenCV/AI Enhancement, OCR & MBTiles Reopening API
// -------------------------------------------------------------

export async function inspectProjectPackage(file, onUploadProgress) {
  const body = new FormData()
  body.append("file", file)
  const response = await client.post("/map-projects/inspect", body, {
    onUploadProgress,
    timeout: 5 * 60_000,
  })
  return response.data.data
}

export async function reopenProjectPackage(file, onUploadProgress) {
  const body = new FormData()
  body.append("file", file)
  const response = await client.post("/map-projects/reopen", body, {
    onUploadProgress,
    timeout: 10 * 60_000,
  })
  return response.data
}

export async function getDatasetInspection(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/inspection`,
    { signal },
  )
  return response.data.data
}

export async function getDatasetEnhancement(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/enhance`,
    { signal },
  )
  return response.data.data
}

export async function enhanceDataset(datasetId, options = {}) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/enhance`,
    options,
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

export async function runDatasetOcr(datasetId, minConfidence = 0.35) {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/ocr`,
    { min_confidence: minConfidence },
    { timeout: 3 * 60_000 },
  )
  return response.data.data
}

export async function listDatasetAnnotations(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/annotations`,
    { signal },
  )
  const d = response.data.data
  if (Array.isArray(d)) return d
  if (Array.isArray(d?.annotations)) return d.annotations
  return []
}

export async function saveDatasetAnnotation(datasetId, annotation) {
  if (annotation?.id) {
    const response = await client.put(
      `/datasets/${encodeURIComponent(datasetId)}/annotations/${encodeURIComponent(annotation.id)}`,
      annotation,
    )
    return response.data.data
  }
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/annotations`,
    annotation,
  )
  return response.data.data
}

export async function deleteDatasetAnnotation(datasetId, annotationId) {
  const response = await client.delete(
    `/datasets/${encodeURIComponent(datasetId)}/annotations/${encodeURIComponent(annotationId)}`,
  )
  return response.data.data
}

export async function getDatasetVectors(datasetId, signal) {
  const response = await client.get(
    `/datasets/${encodeURIComponent(datasetId)}/vectors`,
    { signal },
  )
  return response.data.data
}

export async function generateDatasetMbtiles(datasetId, resampling = "") {
  const response = await client.post(
    `/datasets/${encodeURIComponent(datasetId)}/mbtiles`,
    { resampling },
    { timeout: 5 * 60_000 },
  )
  return response.data.data
}

// -------------------------------------------------------------
// Saved Configurations API
// -------------------------------------------------------------

// -------------------------------------------------------------
// User Profile & Profile Dashboard API
// -------------------------------------------------------------

export async function getProfileDashboard() {
  const response = await client.get("/api/profile/dashboard")
  return response.data.data
}

export async function getProfile() {
  const response = await client.get("/api/profile")
  return response.data.data
}

export async function updateProfileName(fullName) {
  const response = await client.patch("/api/profile", { full_name: fullName })
  return response.data.data
}

export async function listProfileFiles() {
  const response = await client.get("/api/profile/files")
  return response.data.data?.files || []
}

export async function deleteProfileFile(fileId) {
  const response = await client.delete(`/api/profile/files/${encodeURIComponent(fileId)}`)
  return response.data.data
}

// -------------------------------------------------------------
// Personal Maps & Map Sharing API
// Architecture: User -> Personal Dashboard -> My Maps -> Share Map -> Shared With Me
// -------------------------------------------------------------

// Maps CRUD
export async function listMaps() {
  const response = await client.get("/api/maps")
  return response.data.data
}

export async function createMap(name, description = "") {
  const response = await client.post("/api/maps", { name, description })
  return response.data.data
}

export async function getMap(mapId) {
  const response = await client.get(`/api/maps/${encodeURIComponent(mapId)}`)
  return response.data.data
}

export async function updateMap(mapId, name, description = "") {
  const response = await client.put(`/api/maps/${encodeURIComponent(mapId)}`, { name, description })
  return response.data.data
}

export async function deleteMap(mapId) {
  const response = await client.delete(`/api/maps/${encodeURIComponent(mapId)}`)
  return response.data.data
}

// Recent & Saved Maps
export async function recordRecentMap(mapId, action = "opened", workSummary = "") {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/recent`, {
    action,
    work_summary: workSummary,
  })
  return response.data.data
}

export async function saveMapToSaved(mapId, notes = "") {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/saved`, { notes })
  return response.data.data
}

export async function removeSavedMap(mapId) {
  const response = await client.delete(`/api/maps/${encodeURIComponent(mapId)}/saved`)
  return response.data.data
}

// Single-Editor Locks
export async function acquireMapLock(mapId) {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/lock`)
  return response.data.data
}

export async function renewMapLock(mapId) {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/lock/renew`)
  return response.data.data
}

export async function releaseMapLock(mapId) {
  const response = await client.delete(`/api/maps/${encodeURIComponent(mapId)}/lock`)
  return response.data.data
}

export async function getMapLock(mapId) {
  const response = await client.get(`/api/maps/${encodeURIComponent(mapId)}/lock`)
  return response.data.data
}

// Map Configuration & Revisions
export async function createMapFromConfiguration(payload) {
  const response = await client.post("/api/maps/from-configuration", payload)
  return response.data.data
}

export async function saveMapConfig(mapId, config, description = "", expectedRevision = 0) {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/save`, {
    configuration: config,
    description,
    expected_revision: expectedRevision,
  })
  return response.data.data
}

export async function listMapRevisions(mapId) {
  const response = await client.get(`/api/maps/${encodeURIComponent(mapId)}/revisions`)
  return response.data.data?.revisions || []
}

export async function restoreMapRevision(mapId, revisionId) {
  const response = await client.post(
    `/api/maps/${encodeURIComponent(mapId)}/revisions/${encodeURIComponent(revisionId)}/restore`
  )
  return response.data.data
}

// User Files (Personal Storage)
export async function listUserFiles() {
  const response = await client.get("/api/user/files")
  return response.data.data?.files || []
}

export async function deleteUserFile(fileId) {
  const response = await client.delete(`/api/user/files/${encodeURIComponent(fileId)}`)
  return response.data.data
}

// Email-Based Map Sharing
export async function inviteToMap(mapId, email, permission = "editor") {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/invitations`, {
    email,
    permission,
  })
  return response.data.data
}

export async function listMapShares(mapId) {
  const response = await client.get(`/api/maps/${encodeURIComponent(mapId)}/shares`)
  return response.data.data
}

export async function updateMapShare(mapId, shareId, permission) {
  const response = await client.patch(
    `/api/maps/${encodeURIComponent(mapId)}/shares/${encodeURIComponent(shareId)}`,
    { permission }
  )
  return response.data.data
}

export async function revokeMapShare(mapId, shareId) {
  const response = await client.delete(
    `/api/maps/${encodeURIComponent(mapId)}/shares/${encodeURIComponent(shareId)}`
  )
  return response.data.data
}

export async function getMapInvitation(token) {
  const response = await client.get(`/api/map-invitations/${encodeURIComponent(token)}`)
  return response.data.data
}

export async function acceptMapInvitation(token) {
  const response = await client.post(`/api/map-invitations/${encodeURIComponent(token)}/accept`)
  return response.data.data
}

export async function declineMapInvitation(token) {
  const response = await client.post(`/api/map-invitations/${encodeURIComponent(token)}/decline`)
  return response.data.data
}

// Secure Shareable Links
export async function createShareLink(mapId, { permission = "viewer", isPublic = true, expiresInDays = 7 } = {}) {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/share-links`, {
    permission,
    is_public: isPublic,
    expires_in_days: expiresInDays,
  })
  return response.data.data
}

export async function listShareLinks(mapId) {
  const response = await client.get(`/api/maps/${encodeURIComponent(mapId)}/share-links`)
  return response.data.data?.links || []
}

export async function revokeShareLink(mapId, linkId) {
  const response = await client.delete(
    `/api/maps/${encodeURIComponent(mapId)}/share-links/${encodeURIComponent(linkId)}`
  )
  return response.data.data
}

export async function resolveShareLink(token) {
  const response = await client.get(`/api/share/${encodeURIComponent(token)}`)
  return response.data.data
}

// Compatibility Aliases
export const listWorkspaceMaps = listMaps
export const createWorkspaceMap = createMap
export const getWorkspaceMap = getMap
export const updateWorkspaceMap = updateMap
export const deleteWorkspaceMap = deleteMap
export const saveWorkspaceMapConfig = saveMapConfig
export const listWorkspaceMapRevisions = listMapRevisions
export const listWorkspaceFiles = listUserFiles
export const deleteWorkspaceFile = deleteUserFile

// ============================================================================
// PHASE 1 GIS FOUNDATION APIs (MODULES A - G)
// ============================================================================

// Module A — Advanced Layer Manager
export async function getMapLayers(mapId) {
  const response = await client.get(`/api/maps/${encodeURIComponent(mapId)}/layers`)
  return response.data.data
}

export async function createMapLayer(mapId, payload) {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/layers`, payload)
  return response.data.data
}

export async function getGISLayer(layerId) {
  const response = await client.get(`/api/layers/${encodeURIComponent(layerId)}`)
  return response.data.data
}

export async function updateGISLayer(layerId, payload) {
  const response = await client.patch(`/api/layers/${encodeURIComponent(layerId)}`, payload)
  return response.data.data
}

export async function deleteGISLayer(layerId) {
  const response = await client.delete(`/api/layers/${encodeURIComponent(layerId)}`)
  return response.data.data
}

export async function reorderMapLayers(mapId, layerIds) {
  const response = await client.put(`/api/maps/${encodeURIComponent(mapId)}/layers/order`, {
    layer_ids: layerIds,
  })
  return response.data.data
}

export async function updateGISLayerVisibility(layerId, isVisible) {
  const response = await client.patch(`/api/layers/${encodeURIComponent(layerId)}/visibility`, {
    is_visible: isVisible,
  })
  return response.data.data
}

export async function updateGISLayerOpacity(layerId, opacity) {
  const response = await client.patch(`/api/layers/${encodeURIComponent(layerId)}/opacity`, {
    opacity,
  })
  return response.data.data
}

export async function duplicateGISLayer(layerId) {
  const response = await client.post(`/api/layers/${encodeURIComponent(layerId)}/duplicate`)
  return response.data.data
}

export async function createLayerGroup(mapId, payload) {
  const response = await client.post(`/api/maps/${encodeURIComponent(mapId)}/layer-groups`, payload)
  return response.data.data
}

export async function updateLayerGroup(groupId, payload) {
  const response = await client.patch(`/api/layer-groups/${encodeURIComponent(groupId)}`, payload)
  return response.data.data
}

export async function deleteLayerGroup(groupId) {
  const response = await client.delete(`/api/layer-groups/${encodeURIComponent(groupId)}`)
  return response.data.data
}

// Module B — Vector Editing
export async function getLayerFeatures(layerId, { bbox, limit } = {}) {
  const params = {}
  if (Array.isArray(bbox) && bbox.length === 4) {
    params.bbox = bbox.join(",")
  }
  if (limit) {
    params.limit = limit
  }
  const response = await client.get(`/api/layers/${encodeURIComponent(layerId)}/features`, {
    params,
    timeout: 5 * 60_000,
  })
  return response.data.data
}

export async function createLayerFeature(layerId, payload) {
  const response = await client.post(`/api/layers/${encodeURIComponent(layerId)}/features`, payload)
  return response.data.data
}

export async function getVectorFeature(featureId) {
  const response = await client.get(`/api/features/${encodeURIComponent(featureId)}`)
  return response.data.data
}

export async function updateVectorFeature(featureId, payload) {
  const response = await client.patch(`/api/features/${encodeURIComponent(featureId)}`, payload)
  return response.data.data
}

export async function deleteVectorFeature(featureId) {
  const response = await client.delete(`/api/features/${encodeURIComponent(featureId)}`)
  return response.data.data
}

export async function splitVectorFeature(featureId, splitGeometry) {
  const response = await client.post(`/api/features/${encodeURIComponent(featureId)}/split`, {
    split_geometry: splitGeometry,
  })
  return response.data.data
}

export async function mergeVectorFeatures(payload) {
  const response = await client.post("/api/features/merge", payload)
  return response.data.data
}

export async function validateVectorGeometry(geometry, crs = "EPSG:4326") {
  const response = await client.post("/api/features/validate", { geometry, crs })
  return response.data.data
}

export async function snapVectorGeometry(payload) {
  const response = await client.post("/api/features/snap", payload)
  return response.data.data
}

export async function getFeatureHistory(featureId) {
  const response = await client.get(`/api/features/${encodeURIComponent(featureId)}/history`)
  return response.data.data
}

// Module C — Attribute Table
export async function getLayerAttributeTable(layerId, params = {}) {
  const response = await client.get(`/api/layers/${encodeURIComponent(layerId)}/attributes`, { params })
  return response.data.data
}

export async function filterLayerAttributes(layerId, payload) {
  const response = await client.post(`/api/layers/${encodeURIComponent(layerId)}/attributes/filter`, payload)
  return response.data.data
}

export async function updateFeatureAttributes(featureId, values) {
  const response = await client.patch(`/api/features/${encodeURIComponent(featureId)}/attributes`, {
    values,
  })
  return response.data.data
}

export async function addLayerField(layerId, payload) {
  const response = await client.post(`/api/layers/${encodeURIComponent(layerId)}/fields`, payload)
  return response.data.data
}

export async function deleteLayerField(layerId, fieldId) {
  const response = await client.delete(
    `/api/layers/${encodeURIComponent(layerId)}/fields/${encodeURIComponent(fieldId)}`
  )
  return response.data.data
}

export async function calculateLayerField(layerId, payload) {
  const response = await client.post(`/api/layers/${encodeURIComponent(layerId)}/attributes/calculate`, payload)
  return response.data.data
}

export async function exportLayerAttributes(layerId, { format = "csv", featureIds = [] } = {}) {
  const response = await client.post(
    `/api/layers/${encodeURIComponent(layerId)}/attributes/export`,
    { format, feature_ids: featureIds },
    { responseType: "blob" }
  )
  return response.data
}

// Module D — Symbology & Styling Engine
export async function getLayerStyle(layerId) {
  const response = await client.get(`/api/layers/${encodeURIComponent(layerId)}/style`)
  return response.data.data
}

export async function updateLayerStyle(layerId, payload) {
  const response = await client.put(`/api/layers/${encodeURIComponent(layerId)}/style`, payload)
  return response.data.data
}

export async function deleteLayerStyle(layerId) {
  const response = await client.delete(`/api/layers/${encodeURIComponent(layerId)}/style`)
  return response.data.data
}

export async function validateLayerStyle(layerId, payload) {
  const response = await client.post(`/api/layers/${encodeURIComponent(layerId)}/style/validate`, payload)
  return response.data.data
}

// Module E — CRS Management
export async function listSupportedCRS(query = "") {
  const response = await client.get("/api/crs", { params: query ? { q: query } : {} })
  return response.data.data
}

export async function getCRSDetails(epsgCode) {
  const response = await client.get(`/api/crs/${encodeURIComponent(epsgCode)}`)
  return response.data.data
}

export async function reprojectDatasetOrLayer(datasetOrLayerId, payload) {
  const response = await client.post(`/api/datasets/${encodeURIComponent(datasetOrLayerId)}/reproject`, payload)
  return response.data.data
}

export async function setDatasetSourceCRS(datasetId, crs) {
  const response = await client.post(`/api/datasets/${encodeURIComponent(datasetId)}/crs`, { crs })
  return response.data.data
}

// Module F — Dataset Browser
export async function inspectGISDataset(datasetId) {
  const response = await client.post(`/api/datasets/${encodeURIComponent(datasetId)}/inspect`)
  return response.data.data
}

export async function getGISDatasetMetadata(datasetId) {
  const response = await client.get(`/api/datasets/${encodeURIComponent(datasetId)}/metadata`)
  return response.data.data
}

export async function getGISDatasetInternalLayers(datasetId) {
  const response = await client.get(`/api/datasets/${encodeURIComponent(datasetId)}/dataset-layers`)
  return response.data.data
}

export async function renameGISDataset(datasetId, name) {
  const response = await client.patch(`/api/datasets/${encodeURIComponent(datasetId)}`, { name })
  return response.data.data
}

// Module G — Spatial Query Builder
export async function validateSpatialQuery(payload) {
  const response = await client.post("/api/spatial-queries/validate", payload)
  return response.data.data
}

export async function executeSpatialQuery(payload) {
  const response = await client.post("/api/spatial-queries/execute", payload)
  return response.data.data
}

export async function saveSpatialQuery(payload) {
  const response = await client.post("/api/spatial-queries", payload)
  return response.data.data
}

export async function listSavedSpatialQueries(params = {}) {
  const response = await client.get("/api/spatial-queries", { params })
  return response.data.data
}

export async function deleteSavedSpatialQuery(queryId) {
  const response = await client.delete(`/api/spatial-queries/${encodeURIComponent(queryId)}`)
  return response.data.data
}

// =============================================================
// VERACITY MAPS 1.4 — PHASE 2: GIS PROCESSING ENGINE API
// =============================================================

export async function listProcessingAlgorithms(params = {}) {
  const response = await client.get("/api/processing/algorithms", { params })
  return response.data.data
}

export async function getProcessingAlgorithmDetails(algorithmId) {
  const response = await client.get(`/api/processing/algorithms/${encodeURIComponent(algorithmId)}`)
  return response.data.data
}

export async function validateProcessingRequest(payload) {
  const response = await client.post("/api/processing/validate", payload)
  return response.data.data
}

export async function executeProcessingJob(payload) {
  const response = await client.post("/api/processing/execute", payload)
  return response.data.data
}

export async function listProcessingPresets(algorithmName = "") {
  const response = await client.get("/api/processing/presets", {
    params: algorithmName ? { algorithm_name: algorithmName } : {},
  })
  return response.data.data
}

export async function saveProcessingPreset(payload) {
  const response = await client.post("/api/processing/presets", payload)
  return response.data.data
}

export async function deleteProcessingPreset(presetId) {
  const response = await client.delete(`/api/processing/presets/${encodeURIComponent(presetId)}`)
  return response.data.data
}

export async function runBufferAnalysis(payload) {
  const response = await client.post("/api/analysis/buffer", payload)
  return response.data.data
}

export async function runIntersectionAnalysis(payload) {
  const response = await client.post("/api/analysis/intersection", payload)
  return response.data.data
}

export async function runUnionAnalysis(payload) {
  const response = await client.post("/api/analysis/union", payload)
  return response.data.data
}

export async function runDifferenceAnalysis(payload) {
  const response = await client.post("/api/analysis/difference", payload)
  return response.data.data
}

export async function runClipAnalysis(payload) {
  const response = await client.post("/api/analysis/clip", payload)
  return response.data.data
}

export async function runDissolveAnalysis(payload) {
  const response = await client.post("/api/analysis/dissolve", payload)
  return response.data.data
}

export async function runSymmetricalDifferenceAnalysis(payload) {
  const response = await client.post("/api/analysis/symmetrical-difference", payload)
  return response.data.data
}

export async function runSpatialJoinAnalysis(payload) {
  const response = await client.post("/api/analysis/spatial-join", payload)
  return response.data.data
}

export async function runProximityAnalysis(payload) {
  const response = await client.post("/api/analysis/proximity", payload)
  return response.data.data
}

export async function runDistanceMatrixAnalysis(payload) {
  const response = await client.post("/api/analysis/distance-matrix", payload)
  return response.data.data
}

export async function runNearestFacilityAnalysis(payload) {
  const response = await client.post("/api/analysis/nearest-facility", payload)
  return response.data.data
}

export async function runHeatmapAnalysis(payload) {
  const response = await client.post("/api/analysis/heatmap", payload)
  return response.data.data
}

export async function runRasterClip(payload) {
  const response = await client.post("/api/raster/clip", payload)
  return response.data.data
}

export async function runRasterReproject(payload) {
  const response = await client.post("/api/raster/reproject", payload)
  return response.data.data
}

export async function runRasterResample(payload) {
  const response = await client.post("/api/raster/resample", payload)
  return response.data.data
}

export async function runRasterCalculator(payload) {
  const response = await client.post("/api/raster/calculator", payload)
  return response.data.data
}

export async function runRasterAlign(payload) {
  const response = await client.post("/api/raster/align", payload)
  return response.data.data
}

export async function runRasterMosaic(payload) {
  const response = await client.post("/api/raster/mosaic", payload)
  return response.data.data
}

export async function runRasterContours(payload) {
  const response = await client.post("/api/raster/contours", payload)
  return response.data.data
}

export async function runRasterHillshade(payload) {
  const response = await client.post("/api/raster/hillshade", payload)
  return response.data.data
}

export async function runTopologyValidation(payload) {
  const response = await client.post("/api/analysis/topology", payload)
  return response.data.data
}

export async function getTopologyResults(jobId) {
  const response = await client.get(`/api/analysis/topology/${encodeURIComponent(jobId)}/results`)
  return response.data.data
}

export async function getTopologyErrors(jobId) {
  const response = await client.get(`/api/analysis/topology/${encodeURIComponent(jobId)}/errors`)
  return response.data.data
}

export async function fixTopologyErrors(jobId, payload = { fix_all: true }) {
  const response = await client.post(`/api/analysis/topology/${encodeURIComponent(jobId)}/fix`, payload)
  return response.data.data
}

export async function listProcessingJobs(params = {}) {
  const response = await client.get("/api/jobs", { params })
  return response.data.data
}

export async function getProcessingJob(jobId) {
  const response = await client.get(`/api/jobs/${encodeURIComponent(jobId)}`)
  return response.data.data
}

export async function cancelProcessingJob(jobId) {
  const response = await client.post(`/api/jobs/${encodeURIComponent(jobId)}/cancel`)
  return response.data.data
}

export async function getProcessingJobResult(jobId) {
  const response = await client.get(`/api/jobs/${encodeURIComponent(jobId)}/result`)
  return response.data.data
}

export async function getProcessingJobLogs(jobId) {
  const response = await client.get(`/api/jobs/${encodeURIComponent(jobId)}/logs`)
  return response.data.data
}

// ============================================================================
// CUSTOM BASE LAYERS & MULTIPLE MAP VIEWS (Veracity Maps 1.3)
// ============================================================================

export function baseLayerPreviewUrl(previewPath) {
  if (!previewPath) return null
  if (/^https?:\/\//i.test(previewPath)) return previewPath
  return `${API_URL}${previewPath.startsWith("/") ? "" : "/"}${previewPath}`
}

export async function listBaseLayers() {
  const response = await client.get("/api/base-layers")
  return response.data.data
}

export async function registerBaseLayer(payload) {
  const response = await client.post("/api/base-layers", payload)
  return response.data.data
}

export async function getBaseLayer(id) {
  const response = await client.get(`/api/base-layers/${encodeURIComponent(id)}`)
  return response.data.data
}

export async function getBaseLayerPreview(id) {
  const response = await client.get(`/api/base-layers/${encodeURIComponent(id)}/preview`)
  return response.data.data
}

export async function updateBaseLayer(id, payload) {
  const response = await client.put(`/api/base-layers/${encodeURIComponent(id)}`, payload)
  return response.data.data
}

export async function deleteBaseLayer(id) {
  const response = await client.delete(`/api/base-layers/${encodeURIComponent(id)}`)
  return response.data.data
}

export async function validateBaseLayer(id) {
  const response = await client.post(`/api/base-layers/${encodeURIComponent(id)}/validate`)
  return response.data.data
}

export async function getWorkspaceBaseLayer(workspaceId) {
  const response = await client.get(`/api/workspaces/${encodeURIComponent(workspaceId)}/base-layer`)
  return response.data.data
}

export async function selectWorkspaceBaseLayer(workspaceId, payload) {
  const response = await client.put(`/api/workspaces/${encodeURIComponent(workspaceId)}/base-layer`, payload)
  return response.data.data
}

export async function getWorkspaceMapView(workspaceId) {
  const response = await client.get(`/api/workspaces/${encodeURIComponent(workspaceId)}/map-view`)
  return response.data.data
}

export async function updateWorkspaceMapView(workspaceId, payload) {
  const response = await client.put(`/api/workspaces/${encodeURIComponent(workspaceId)}/map-view`, payload)
  return response.data.data
}

export async function listMapViews() {
  const response = await client.get("/api/map-views")
  return response.data.data
}

export async function getMapView(viewType) {
  const response = await client.get(`/api/map-views/${encodeURIComponent(viewType)}`)
  return response.data.data
}

// ============================================================================
// DYNAMIC GEOPACKAGE LAYERS, WORKSPACE RECORDS & RASTER MANAGEMENT (1.3)
// ============================================================================

export async function inspectUploadedDataset(payload) {
  const body =
    typeof payload === "string"
      ? { file_id: payload, dataset_id: payload }
      : payload || {}
  const response = await client.post("/api/datasets/inspect", body)
  return response.data.data
}

export async function addWorkspaceDatasetLayers(workspaceId, payload) {
  const response = await client.post(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers`,
    payload,
    { timeout: 10 * 60_000 }
  )
  return response.data.data
}

export async function listWorkspaceLayers(workspaceId) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers`
  )
  return response.data.data
}

export async function getWorkspaceLayer(workspaceId, layerId) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers/${encodeURIComponent(layerId)}`
  )
  return response.data.data
}

export async function updateWorkspaceLayer(workspaceId, layerId, payload) {
  const response = await client.patch(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers/${encodeURIComponent(layerId)}`,
    payload
  )
  return response.data.data
}

export async function deleteWorkspaceLayer(workspaceId, layerId) {
  const response = await client.delete(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers/${encodeURIComponent(layerId)}`
  )
  return response.data.data
}

export async function reorderWorkspaceLayers(workspaceId, layerIds) {
  const response = await client.patch(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers/order`,
    { layer_ids: layerIds }
  )
  return response.data.data
}

export async function batchUpdateWorkspaceLayersVisibility(workspaceId, layerIds, visibility) {
  const response = await client.patch(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers/visibility`,
    { layer_ids: layerIds, visibility }
  )
  return response.data.data
}

export async function queryWorkspaceLayerFeatures(workspaceId, layerId, params = {}) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/layers/${encodeURIComponent(layerId)}/features`,
    { params }
  )
  return response.data.data
}

export async function listWorkspaceRecords(workspaceId, params = {}) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/records`,
    { params }
  )
  return response.data.data
}

export async function getWorkspaceRecordCounts(workspaceId) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/records/counts`
  )
  return response.data.data
}

export async function getWorkspaceRecordSchema(workspaceId, params = {}) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/records/schema`,
    { params }
  )
  return response.data.data
}

export async function getWorkspaceRecord(workspaceId, recordId) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/records/${encodeURIComponent(recordId)}`
  )
  return response.data.data
}

export async function updateWorkspaceRecord(workspaceId, recordId, payload) {
  const response = await client.patch(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/records/${encodeURIComponent(recordId)}`,
    payload
  )
  return response.data.data
}

export async function deleteWorkspaceRecord(workspaceId, recordId) {
  const response = await client.delete(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/records/${encodeURIComponent(recordId)}`
  )
  return response.data.data
}

export async function exportWorkspaceRecords(workspaceId, payload) {
  const response = await client.post(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/records/export`,
    payload,
    { responseType: "blob" }
  )
  return response.data
}

export async function listWorkspaceRasters(workspaceId) {
  const response = await client.get(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/rasters`
  )
  return response.data.data
}

export async function updateWorkspaceRaster(workspaceId, rasterId, payload) {
  const response = await client.patch(
    `/api/workspaces/${encodeURIComponent(workspaceId)}/rasters/${encodeURIComponent(rasterId)}`,
    payload
  )
  return response.data.data
}





