import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import {
  AlertCircle,
  ArrowLeft,
  Bookmark,
  Check,
  Database,
  Loader2,
  X,
} from "lucide-react"

import {
  downloadProjectExport,
  listIcons,
  readableApiError,
  createMapFromConfiguration,
  listBuildings,
  listDatasets,
  getMap,
  saveMapConfig,
  acquireMapLock,
  getMapLock,
  renewMapLock,
  releaseMapLock,
  ensureActiveSession,
  recordRecentMap,
  getMapLayers,
  deleteGISLayer,
  listBaseLayers,
  listMapViews,
  getWorkspaceBaseLayer,
  selectWorkspaceBaseLayer,
  getWorkspaceMapView,
  updateWorkspaceMapView,
} from "../api"
import WorkspaceMap from "../components/WorkspaceMap"
import ErrorBoundary from "../components/ErrorBoundary"
import useWorkspace from "../hooks/useWorkspace"
import ExportProjectModal from "../components/ExportProjectModal"
import EnhancementModal from "../components/EnhancementModal"
import AnnotationReviewPanel from "../components/AnnotationReviewPanel"
import ReopenProjectModal from "../components/ReopenProjectModal"
import ShareMapModal from "../components/ShareMapModal"
import AttributeTablePanel from "../components/gis/AttributeTablePanel"
import CRSManagerModal from "../components/gis/CRSManagerModal"
import DatasetBrowserModal from "../components/gis/DatasetBrowserModal"
import MapLibreGISController from "../components/gis/MapLibreGISController"
import BaseLayerSelectionModal from "../components/baselayers/BaseLayerSelectionModal"
import WorkspaceMenuBar from "../components/workspace/WorkspaceMenuBar"
import WorkspaceToolbar from "../components/workspace/WorkspaceToolbar"
import ThreeGroupLayersPanel from "../components/workspace/ThreeGroupLayersPanel"
import WorkspaceStatusBar from "../components/workspace/WorkspaceStatusBar"
import MapMeasurementOverlay from "../components/workspace/MapMeasurementOverlay"
import WorkspaceGuideModal from "../components/workspace/WorkspaceGuideModal"
import { DEFAULT_MAP_VIEWS } from "../lib/baseLayerSync"
import { useAuth } from "../context/AuthContext"
import { applyFloorSelection } from "../lib/floorSelection"
import { lockHeldByAnotherUser, lockHolderNameFromError, lockHolderNameFromLock } from "../lib/mapLock"
import { loadMapWorkspace, mapConfigurationMemberIds } from "../lib/sessionLoading"
import { persistMapConfiguration } from "../lib/mapConfigPersistence"
import {
  getEffectiveShortcuts,
  loadUserShortcutOverrides,
  saveUserShortcutOverrides,
  isTextInputTarget,
  matchesShortcutEvent,
} from "../lib/shortcutRegistry"
import {
  LAYOUT_LIMITS,
  clamp,
  loadWorkspaceLayout,
  saveWorkspaceLayout,
  resetWorkspaceLayout,
  loadSpatialBookmarks,
  saveSpatialBookmarks,
} from "../lib/workspaceLayout"

const WorkspaceBrowserPanel = lazy(() => import("../components/workspace/WorkspaceBrowserPanel"))
const DockedProcessingToolbox = lazy(() => import("../components/workspace/DockedProcessingToolbox"))
const UnifiedDatasetUploadPanel = lazy(() => import("../components/gis/UnifiedDatasetUploadPanel"))
const WorkspaceRecordsView = lazy(() => import("../components/gis/WorkspaceRecordsView"))
const KeyboardShortcutsModal = lazy(() => import("../components/gis/KeyboardShortcutsModal"))

export default function WorkspacePage() {
  const { id: paramDatasetId, mapspaceId, projectId: paramProjectId, mapId } = useParams()
  const { user } = useAuth()
  const activeContainerId = mapId || mapspaceId || paramProjectId || ""
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedView = searchParams.get("view")

  // QGIS-Inspired Resizable Dock Layout (persisted per user)
  const [layout, setLayout] = useState(() => loadWorkspaceLayout(user?.id))
  useEffect(() => {
    setLayout(loadWorkspaceLayout(user?.id))
  }, [user?.id])

  const updateLayout = useCallback(
    (updater) => {
      setLayout((prev) => {
        const next = typeof updater === "function" ? updater(prev) : { ...prev, ...updater }
        saveWorkspaceLayout(user?.id, next)
        return next
      })
    },
    [user?.id]
  )

  const handleResetLayout = useCallback(() => {
    const fresh = resetWorkspaceLayout(user?.id)
    setLayout(fresh)
    setWorkspaceToast("Workspace layout reset to default")
    setTimeout(() => setWorkspaceToast(""), 2500)
  }, [user?.id])

  // Open records dock automatically if ?view=database was requested via URL
  useEffect(() => {
    if (requestedView === "database") {
      updateLayout((prev) => (prev.recordsOpen ? prev : { ...prev, recordsOpen: true }))
    }
  }, [requestedView, updateLayout])

  // Modals & workspace UI state
  const [addOpen, setAddOpen] = useState(false)
  const [addFocusSection, setAddFocusSection] = useState(null)
  const [recordsRefreshKey, setRecordsRefreshKey] = useState(0)
  const [shortcutsModalOpen, setShortcutsModalOpen] = useState(false)
  const [guideModalOpen, setGuideModalOpen] = useState(false)
  const [shortcutOverrides, setShortcutOverrides] = useState(() => loadUserShortcutOverrides())
  const effectiveShortcuts = useMemo(
    () => getEffectiveShortcuts(shortcutOverrides),
    [shortcutOverrides]
  )

  // Measurement tool state
  const [measureMode, setMeasureMode] = useState(null) // null | "distance" | "area"

  // Right dock tabs & menu-driven algorithm runner triggers
  const [rightDockTab, setRightDockTab] = useState("toolbox") // "toolbox" | "history" | "symbology"
  const [requestedAlgId, setRequestedAlgId] = useState(null)
  const [requestedAlgTick, setRequestedAlgTick] = useState(0)
  const [focusSearchTick, setFocusSearchTick] = useState(0)

  // Existing workspace state
  const [icons, setIcons] = useState([])
  const [dirty, setDirty] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [reopenModalOpen, setReopenModalOpen] = useState(false)
  const [enhancementTarget, setEnhancementTarget] = useState(null)
  const [annotationTarget, setAnnotationTarget] = useState(null)
  const [overlayRefreshKey, setOverlayRefreshKey] = useState(0)
  const [transformingMember, setTransformingMember] = useState(null)
  const [activeFloorLevel, setActiveFloorLevel] = useState(0)
  const [activeBuildingName, setActiveBuildingName] = useState("")
  const [isolateActiveFloor, setIsolateActiveFloor] = useState(true)
  const initializedFloorRef = useRef(false)
  const pendingMapMembersRef = useRef([])
  const leftDockRef = useRef(null)

  // Mapspace & Workspace Map state
  const [currentMap, setCurrentMap] = useState(null)
  const [mapLoading, setMapLoading] = useState(Boolean(mapId))
  const [mapNotFound, setMapNotFound] = useState(false)
  const [activeDatasetId, setActiveDatasetId] = useState(paramDatasetId || null)
  const [workspaceToast, setWorkspaceToast] = useState("")
  const [shareModalOpen, setShareModalOpen] = useState(false)

  // Lock State
  const [lockHeld, setLockHeld] = useState(false)
  const [isLockedByOther, setIsLockedByOther] = useState(false)
  const [lockHolderName, setLockHolderName] = useState("")

  // GIS Layers & Editing State
  const [gisLayers, setGisLayers] = useState([])
  const [activeGISLayer, setActiveGISLayer] = useState(null)
  const [selectedFeatureIds, setSelectedFeatureIds] = useState([])
  const [gisFeaturesRefreshKey, setGisFeaturesRefreshKey] = useState(0)
  const [attributeTableLayer, setAttributeTableLayer] = useState(null)
  const [styleEditorLayer, setStyleEditorLayer] = useState(null)
  const [crsModalOpen, setCrsModalOpen] = useState(false)
  const [datasetBrowserOpen, setDatasetBrowserOpen] = useState(false)
  const [mapInstance, setMapInstance] = useState(null)
  const [activeVectorTool, setActiveVectorTool] = useState("select")
  const [snapEnabled, setSnapEnabled] = useState(false)
  const [controllerActions, setControllerActions] = useState({ canUndo: false, canRedo: false })
  const [statusLoadingLabel, setStatusLoadingLabel] = useState("")
  const [layerOrder, setLayerOrder] = useState(["vector", "raster", "base"])

  const handleToggleLayerOrder = useCallback(() => {
    setLayerOrder((prev) =>
      prev[0] === "raster" ? ["vector", "raster", "base"] : ["raster", "vector", "base"]
    )
  }, [])

  const notifyGIS = useCallback((msg) => {
    if (!msg) return
    const text =
      typeof msg === "string"
        ? msg
        : typeof msg === "object" && msg.message
          ? msg.details
            ? `${msg.message} ${msg.details}`
            : msg.message
          : String(msg)
    setWorkspaceToast(text)
    setTimeout(() => setWorkspaceToast(""), 3500)
  }, [])

  const openUnifiedAddModal = useCallback((section = null) => {
    setAddFocusSection(section)
    setAddOpen(true)
  }, [])

  const handleAssignShortcut = useCallback((toolId, combo) => {
    setShortcutOverrides((prev) => {
      const next = { ...prev, [toolId]: combo }
      saveUserShortcutOverrides(next)
      return next
    })
  }, [])

  const handleResetShortcuts = useCallback(() => {
    setShortcutOverrides({})
    saveUserShortcutOverrides({})
  }, [])

  // Single-editor lock lifecycle
  useEffect(() => {
    if (!mapId || !user?.id || currentMap?.id !== mapId) return

    let renewInterval = null
    let isMounted = true
    let acquired = false

    async function initializeLock() {
      try {
        const status = await getMapLock(mapId)
        if (!isMounted) return

        const existingLock = lockHeldByAnotherUser(status, user.id)
        if (existingLock) {
          setLockHeld(false)
          setIsLockedByOther(true)
          setLockHolderName(lockHolderNameFromLock(existingLock))
          return
        }

        await acquireMapLock(mapId)
        acquired = true
        if (!isMounted) {
          acquired = false
          releaseMapLock(mapId).catch(() => {})
          return
        }

        setLockHeld(true)
        setIsLockedByOther(false)
        setLockHolderName("")
        renewInterval = setInterval(() => {
          renewMapLock(mapId).catch((err) => {
            const isConflict =
              err.response?.status === 409 ||
              err.response?.data?.error?.code === "LOCK_HELD" ||
              err.response?.data?.error?.code === "LOCK_LOST"
            if (isConflict) {
              acquired = false
              setLockHeld(false)
              setIsLockedByOther(true)
              setLockHolderName(lockHolderNameFromError(err))
              if (renewInterval) clearInterval(renewInterval)
            }
          })
        }, 25000)
      } catch (err) {
        if (!isMounted) return
        const isConflict =
          err.response?.status === 409 || err.response?.data?.error?.code === "LOCK_HELD"
        if (isConflict) {
          setLockHeld(false)
          setIsLockedByOther(true)
          setLockHolderName(lockHolderNameFromError(err))
        }
      }
    }

    initializeLock()
    const handleBeforeUnload = () => {
      if (acquired) releaseMapLock(mapId).catch(() => {})
    }
    window.addEventListener("beforeunload", handleBeforeUnload)

    return () => {
      isMounted = false
      if (renewInterval) clearInterval(renewInterval)
      window.removeEventListener("beforeunload", handleBeforeUnload)
      if (acquired) releaseMapLock(mapId).catch(() => {})
    }
  }, [mapId, user?.id, currentMap?.id])

  // Load Map Configuration
  useEffect(() => {
    if (!mapId) return
    setMapLoading(true)
    setMapNotFound(false)
    loadMapWorkspace({
      ensureSession: ensureActiveSession,
      loadMap: () => getMap(mapId),
    })
      .then(async (data) => {
        if (data?.map || data) {
          const m = data?.map || data
          setCurrentMap(m)
          recordRecentMap(mapId, "opened", "Opened in GIS Workspace").catch(() => {})
          const queryDs = searchParams.get("dataset") || searchParams.get("datasetId")
          if (queryDs) {
            setActiveDatasetId(queryDs)
          } else if (!paramDatasetId) {
            let savedPrimaryId = null
            const configuredMemberIds = []
            try {
              const conf = typeof m.configuration === "string" ? JSON.parse(m.configuration) : m.configuration
              savedPrimaryId = conf?.primary_dataset_id || conf?.primaryDatasetId || null
              const members = conf?.members || conf?.layers || conf?.datasets || []
              if (Array.isArray(members)) {
                for (const item of members) {
                  const mId = item?.dataset_id || item?.id || (typeof item === "string" ? item : null)
                  if (mId && !configuredMemberIds.includes(mId)) configuredMemberIds.push(mId)
                }
              }
              if (!savedPrimaryId && configuredMemberIds.length > 0) savedPrimaryId = configuredMemberIds[0]
            } catch {}

            if (Array.isArray(m.assets)) {
              for (const asset of m.assets) {
                if (asset?.id && !configuredMemberIds.includes(asset.id)) configuredMemberIds.push(asset.id)
              }
            }
            pendingMapMembersRef.current = configuredMemberIds

            if (savedPrimaryId) setActiveDatasetId(savedPrimaryId)
            else if (m.assets?.length > 0) setActiveDatasetId(m.assets[0].id)
            else setActiveDatasetId(null)
          }
        }
      })
      .catch((err) => {
        if (err.response?.status === 404) {
          setMapNotFound(true)
          navigate("/dashboard", { replace: true })
          return
        }
        console.warn("Failed to load map details:", err)
      })
      .finally(() => setMapLoading(false))
  }, [mapId, paramDatasetId, searchParams, navigate])

  const effectiveId = activeDatasetId || paramDatasetId
  const workspace = useWorkspace(effectiveId)
  const id = effectiveId
  const gisMapId = mapId || activeContainerId || id

  // Spatial Bookmarks per workspace
  const [bookmarks, setBookmarks] = useState(() => loadSpatialBookmarks(gisMapId))
  useEffect(() => {
    setBookmarks(loadSpatialBookmarks(gisMapId))
  }, [gisMapId])

  const handleAddBookmark = useCallback(() => {
    if (!mapInstance) return
    const center = mapInstance.getCenter()
    const zoom = mapInstance.getZoom()
    const bearing = mapInstance.getBearing()
    const name = window.prompt(
      "Bookmark name:",
      `View (${center.lat.toFixed(3)}, ${center.lng.toFixed(3)})`
    )
    if (!name || !name.trim()) return
    const next = [
      {
        id: `bm_${Date.now()}`,
        name: name.trim(),
        center: [center.lng, center.lat],
        zoom,
        bearing,
      },
      ...bookmarks,
    ]
    setBookmarks(next)
    saveSpatialBookmarks(gisMapId, next)
    notifyGIS(`Saved spatial bookmark "${name.trim()}"`)
  }, [mapInstance, bookmarks, gisMapId, notifyGIS])

  const handleSelectBookmark = useCallback(
    (bm) => {
      if (!mapInstance || !bm?.center) return
      mapInstance.flyTo({
        center: bm.center,
        zoom: bm.zoom ?? 12,
        bearing: bm.bearing ?? 0,
        duration: 600,
      })
    },
    [mapInstance]
  )

  const handleDeleteBookmark = useCallback(
    (bmId) => {
      const next = bookmarks.filter((b) => b.id !== bmId)
      setBookmarks(next)
      saveSpatialBookmarks(gisMapId, next)
    },
    [bookmarks, gisMapId]
  )

  const refreshGISLayers = useCallback(async () => {
    if (!gisMapId) return
    setStatusLoadingLabel("Syncing layers…")
    try {
      const res = await getMapLayers(gisMapId)
      const nextLayers = res?.layers || []
      setGisLayers(nextLayers)
      setActiveGISLayer((prev) => {
        if (prev) {
          const updated = nextLayers.find((l) => l.layer_id === prev.layer_id)
          if (updated) return updated
        }
        return nextLayers.find((l) => l.layer_type === "vector") || nextLayers[0] || null
      })
    } catch {
      // Non-blocking fallback
    } finally {
      setStatusLoadingLabel("")
    }
  }, [gisMapId])

  useEffect(() => {
    refreshGISLayers()
  }, [refreshGISLayers])

  // Custom Base Layer & Multiple Map Views State
  const [mapViews, setMapViews] = useState(DEFAULT_MAP_VIEWS)
  const [customBaseLayers, setCustomBaseLayers] = useState([])
  const [activeBaseLayer, setActiveBaseLayer] = useState(null)
  const [mapViewPrefs, setMapViewPrefs] = useState({
    view_type: "world",
    show_labels: true,
    show_roads: true,
    show_places: true,
    base_layer_opacity: 1.0,
    base_layer_visible: true,
  })
  const [baseLayerModalOpen, setBaseLayerModalOpen] = useState(false)
  const [baseLayerModalMode, setBaseLayerModalMode] = useState("select")
  const [baseLayerFitTrigger, setBaseLayerFitTrigger] = useState(0)

  const refreshBaseLayerState = useCallback(
    async (wsKey) => {
      if (!wsKey) return
      try {
        const [viewsRes, libRes, wsBlRes, mvRes] = await Promise.all([
          listMapViews().catch(() => null),
          listBaseLayers().catch(() => null),
          getWorkspaceBaseLayer(wsKey).catch(() => null),
          getWorkspaceMapView(wsKey).catch(() => null),
        ])
        if (viewsRes?.map_views?.length) setMapViews(viewsRes.map_views)
        if (libRes?.base_layers) setCustomBaseLayers(libRes.base_layers)
        const wbl = wsBlRes?.workspace_base_layer
        if (wbl) {
          setActiveBaseLayer(wbl.base_layer || null)
          if (wbl.previous_unavailable) {
            setBaseLayerModalOpen(true)
          }
        }
        const mv = mvRes?.map_view
        if (mv) {
          setMapViewPrefs({
            view_type: mv.view_type || (wbl?.base_layer_type === "custom" ? "custom" : "world"),
            show_labels: mv.show_labels !== false,
            show_roads: mv.show_roads !== false,
            show_places: mv.show_places !== false,
            base_layer_opacity: typeof mv.base_layer_opacity === "number" ? mv.base_layer_opacity : 1.0,
            base_layer_visible: mv.base_layer_visible !== false,
          })
        }
      } catch {}
    },
    []
  )

  useEffect(() => {
    refreshBaseLayerState(gisMapId)
  }, [gisMapId, refreshBaseLayerState])

  const handleSelectMapView = async (viewType) => {
    setMapViewPrefs((prev) => ({ ...prev, view_type: viewType }))
    if (viewType === "custom" && activeBaseLayer && !activeBaseLayer.is_default) {
      setBaseLayerFitTrigger((n) => n + 1)
    }
    if (!gisMapId) return
    try {
      await updateWorkspaceMapView(gisMapId, { view_type: viewType })
    } catch {}
  }

  const handleChangeMapViewPrefs = async (patch) => {
    setMapViewPrefs((prev) => ({
      ...prev,
      ...(patch.show_labels !== undefined ? { show_labels: patch.show_labels } : {}),
      ...(patch.show_roads !== undefined ? { show_roads: patch.show_roads } : {}),
      ...(patch.show_places !== undefined ? { show_places: patch.show_places } : {}),
      ...(patch.opacity !== undefined ? { base_layer_opacity: patch.opacity } : {}),
      ...(patch.base_layer_visible !== undefined ? { base_layer_visible: patch.base_layer_visible } : {}),
    }))
    if (!gisMapId) return
    try {
      await updateWorkspaceMapView(gisMapId, patch)
    } catch {}
  }

  const handleSelectCustomBaseLayer = async (bl) => {
    if (!bl) return
    setActiveBaseLayer(bl)
    setMapViewPrefs((prev) => ({ ...prev, view_type: "custom", base_layer_visible: true }))
    setBaseLayerFitTrigger((n) => n + 1)
    if (!gisMapId) return
    try {
      await selectWorkspaceBaseLayer(gisMapId, {
        base_layer_type: "custom",
        base_layer_id: bl.id,
        selected_view: "custom",
      })
      notifyGIS(`Switched base layer to "${bl.name}"`)
    } catch {}
  }

  const handleGISLayerImmediateUpdate = (layerId, patch) => {
    setGisLayers((prev) => {
      if (patch?.reorderIds) {
        const byId = new Map(prev.map((l) => [l.layer_id, l]))
        return patch.reorderIds
          .map((lid, idx) => (byId.has(lid) ? { ...byId.get(lid), display_order: idx } : null))
          .filter(Boolean)
      }
      return prev.map((l) => (l.layer_id === layerId ? { ...l, ...patch } : l))
    })
  }

  // Camera navigation helpers
  const fitMapBounds = useCallback(
    (bounds) => {
      if (!mapInstance || !Array.isArray(bounds) || bounds.length !== 4) return false
      const [w, s, e, n] = bounds.map(Number)
      if (![w, s, e, n].every(Number.isFinite)) return false
      if (Math.abs(w) > 180 || Math.abs(e) > 180 || Math.abs(s) > 90 || Math.abs(n) > 90) return false
      if (Math.abs(e - w) < 1e-6 && Math.abs(n - s) < 1e-6) {
        mapInstance.flyTo({ center: [w, s], zoom: 16, duration: 600 })
      } else {
        mapInstance.fitBounds(
          [
            [w, s],
            [e, n],
          ],
          { padding: 64, maxZoom: 18, duration: 600 }
        )
      }
      return true
    },
    [mapInstance]
  )

  const handleZoomToGISFeature = (feature) => {
    if (!mapInstance || !feature?.geometry) return
    const coords = []
    const collect = (arr) => {
      if (!Array.isArray(arr)) return
      if (typeof arr[0] === "number" && typeof arr[1] === "number") coords.push([arr[0], arr[1]])
      else arr.forEach(collect)
    }
    collect(feature.geometry.coordinates)
    if (coords.length === 1) {
      mapInstance.flyTo({ center: coords[0], zoom: 17, duration: 600 })
    } else if (coords.length > 1) {
      let [minX, minY, maxX, maxY] = [coords[0][0], coords[0][1], coords[0][0], coords[0][1]]
      coords.forEach(([x, y]) => {
        if (x < minX) minX = x
        if (x > maxX) maxX = x
        if (y < minY) minY = y
        if (y > maxY) maxY = y
      })
      fitMapBounds([minX, minY, maxX, maxY])
    }
  }

  const handleZoomToLayer = useCallback(
    (layerOrEntry) => {
      if (!layerOrEntry) return
      const b = layerOrEntry.bounds || layerOrEntry.raw?.bounds || layerOrEntry.raw?.dataset?.extracted_metadata?.bounds
      if (fitMapBounds(b)) return
      notifyGIS("No geographic extent recorded for this layer yet.")
    },
    [fitMapBounds, notifyGIS]
  )

  const handleLocateRecord = useCallback(
    (record) => {
      if (!record) return
      if (record.source_feature_id) {
        setSelectedFeatureIds([record.source_feature_id])
      }
      if (fitMapBounds(record.bounds)) return
      if (Array.isArray(record.coordinates) && record.coordinates.length >= 2 && mapInstance) {
        const [lng, lat] = record.coordinates.map(Number)
        if (Number.isFinite(lng) && Number.isFinite(lat) && Math.abs(lng) <= 180 && Math.abs(lat) <= 90) {
          mapInstance.flyTo({ center: [lng, lat], zoom: 16, duration: 600 })
        }
      }
    },
    [fitMapBounds, mapInstance]
  )

  const handleZoomFullExtent = useCallback(() => {
    if (!mapInstance) return
    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity
    const include = (b) => {
      if (!Array.isArray(b) || b.length !== 4) return
      const [w, s, e, n] = b.map(Number)
      if (![w, s, e, n].every(Number.isFinite)) return
      if (Math.abs(w) > 180 || Math.abs(e) > 180 || Math.abs(s) > 90 || Math.abs(n) > 90) return
      if (w < minX) minX = w
      if (s < minY) minY = s
      if (e > maxX) maxX = e
      if (n > maxY) maxY = n
    }
    ;(gisLayers || []).forEach((l) => {
      if (l.is_visible !== false) include(l.bounds)
    })
    ;(workspace.manifest?.members || []).forEach((m) => {
      if (m.visible !== false) include(m.bounds)
    })
    if (activeBaseLayer && !activeBaseLayer.is_default && activeBaseLayer.extent) {
      const ex = activeBaseLayer.extent
      include(
        Array.isArray(ex)
          ? ex
          : [ex.west ?? ex.min_x, ex.south ?? ex.min_y, ex.east ?? ex.max_x, ex.north ?? ex.max_y]
      )
    }
    if (Number.isFinite(minX) && Number.isFinite(minY) && Number.isFinite(maxX) && Number.isFinite(maxY)) {
      fitMapBounds([minX, minY, maxX, maxY])
      notifyGIS("Fitted Viewscape to full extent")
    } else {
      mapInstance.flyTo({ center: [0, 20], zoom: 2, duration: 600 })
      notifyGIS("Reset camera to World Map extent")
    }
  }, [mapInstance, gisLayers, workspace.manifest?.members, activeBaseLayer, fitMapBounds, notifyGIS])

  // Floor & multi-member initialization
  useEffect(() => {
    initializedFloorRef.current = false
  }, [id])

  useEffect(() => {
    if (!initializedFloorRef.current && workspace.manifest?.members?.length > 0) {
      const primary = workspace.manifest.members.find((m) => m.dataset_id === id)
      const targetLevel =
        primary && typeof primary.floor_level === "number"
          ? primary.floor_level
          : workspace.manifest.members.find((m) => m.floor_level !== undefined)?.floor_level ?? 0
      const targetBuilding =
        primary?.building_name || workspace.manifest.members.find((m) => m.building_name)?.building_name || ""
      handleSelectFloor(targetLevel, targetBuilding)
      initializedFloorRef.current = true
    }
  }, [id, workspace.manifest?.members])

  useEffect(() => {
    if (!mapId || !id || !workspace.manifest?.members || pendingMapMembersRef.current.length === 0) return
    const existingIds = new Set(workspace.manifest.members.map((m) => m.dataset_id))
    const toAttach = pendingMapMembersRef.current.filter((mId) => mId && mId !== id && !existingIds.has(mId))
    pendingMapMembersRef.current = []
    if (toAttach.length > 0) {
      ;(async () => {
        for (const extraId of toAttach) {
          await workspace.attachDataset(extraId).catch(() => undefined)
        }
        await workspace.refreshManifest().catch(() => undefined)
      })()
    }
  }, [mapId, id, workspace.manifest?.members])

  function handleSelectFloor(level, buildingName) {
    if (!id || id === "undefined" || id === "null" || !workspace.manifest?.members) return
    applyFloorSelection({
      level,
      buildingName,
      members: workspace.manifest?.members,
      setFloorLevel: setActiveFloorLevel,
      setBuildingName: setActiveBuildingName,
      updateMember: (datasetId, patch) => workspace.updateMember(datasetId, patch).catch(() => undefined),
    })
  }

  // Export & Save Config state
  const [exportModalOpen, setExportModalOpen] = useState(false)
  const [exportModalBuildings, setExportModalBuildings] = useState([])
  const [exportModalDatasets, setExportModalDatasets] = useState([])
  const [exportError, setExportError] = useState(null)

  async function openExportModal() {
    setExportError(null)
    setExportModalOpen(true)
    try {
      const [blds, dss] = await Promise.all([
        listBuildings().catch(() => []),
        listDatasets().catch(() => []),
      ])
      const memberIds = new Set((workspace.manifest?.members || []).map((m) => m.dataset_id))
      const relevantBuildings = (blds || []).filter((b) =>
        (b.floors || []).some((fl) => (fl.dataset_ids || []).some((dId) => memberIds.has(dId)))
      )
      setExportModalBuildings(relevantBuildings)
      setExportModalDatasets(dss || [])
    } catch {}
  }

  const resolvedExportDatasetId =
    id ||
    workspace.manifest?.members?.[0]?.dataset_id ||
    gisLayers.find((l) => l?.dataset_id)?.dataset_id ||
    exportModalDatasets.find((d) => d?.id)?.id ||
    null

  async function handlePerformExport(payload) {
    if (exporting || workspace.saveState === "saving") return
    const firstLayerId = payload?.layer_ids?.[0]
    const matchedGisDatasetId = firstLayerId
      ? gisLayers.find((l) => l.layer_id === firstLayerId || l.dataset_id === firstLayerId)?.dataset_id
      : null
    const matchedExportDatasetId = firstLayerId
      ? exportModalDatasets.find((d) => d.id === firstLayerId)?.id
      : null
    const targetDatasetId =
      id || matchedExportDatasetId || matchedGisDatasetId || resolvedExportDatasetId

    if (!targetDatasetId) {
      const msg = "Upload or add at least one raster or vector dataset before exporting a Viewscape ZIP."
      setExportError(msg)
      workspace.setError(msg)
      return
    }

    setExporting(true)
    setExportError(null)
    try {
      await downloadProjectExport(targetDatasetId, workspace.manifest?.name || currentMap?.name, payload)
      setExportModalOpen(false)
    } catch (requestError) {
      const err = readableApiError(requestError, "Unable to export workspace package.")
      setExportError(err)
      workspace.setError(err)
    } finally {
      setExporting(false)
    }
  }

  const [savingConfig, setSavingConfig] = useState(false)
  const [saveConfigModalOpen, setSaveConfigModalOpen] = useState(false)
  const [saveConfigName, setSaveConfigName] = useState("")

  function openSaveConfigDialog() {
    setSaveConfigName(currentMap?.name || workspace.manifest?.name || "Workspace Configuration")
    setSaveConfigModalOpen(true)
  }

  async function handleSaveConfigSubmit(e) {
    if (e) e.preventDefault()
    if (savingConfig) return
    const name = (saveConfigName || "").trim() || currentMap?.name || "Saved Configuration"
    setSavingConfig(true)
    try {
      const members = mapConfigurationMemberIds(workspace.manifest?.members)
      const payload = {
        name,
        primary_dataset_id: id,
        members,
        configuration: {
          active_view: "map",
          camera: workspace.manifest?.camera || null,
        },
      }
      const persisted = await persistMapConfiguration(
        {
          mapId,
          payload,
          name,
          expectedRevision: currentMap?.revision_number || 0,
          activeView: "map",
        },
        { saveMapConfig, createMapFromConfiguration }
      )
      if (persisted?.map) setCurrentMap(persisted.map)
      setSaveConfigModalOpen(false)
      notifyGIS(`Saved "${name}"`)
      if (persisted.navigateTo) navigate(persisted.navigateTo, { replace: true })
    } catch (err) {
      workspace.setError({
        message: "Failed to save configuration.",
        details: err?.response?.data?.error?.message || err?.message || "Unknown error",
      })
    } finally {
      setSavingConfig(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    listIcons(controller.signal).then(setIcons).catch(() => setIcons([]))
    return () => controller.abort()
  }, [])

  function confirmDiscard() {
    return !dirty || window.confirm("Discard unsaved record changes?")
  }

  const handleUnifiedAddComplete = useCallback(
    async (result) => {
      setAddOpen(false)
      setAddFocusSection(null)
      if (result?.baseLayer) {
        setActiveBaseLayer(result.baseLayer)
        setMapViewPrefs((prev) => ({
          ...prev,
          view_type: result.baseLayerType === "custom" ? "custom" : "world",
          base_layer_visible: true,
        }))
        if (result.baseLayerType === "custom") setBaseLayerFitTrigger((n) => n + 1)
        listBaseLayers()
          .then((res) => {
            if (res?.base_layers) setCustomBaseLayers(res.base_layers)
          })
          .catch(() => {})
      }
      if (result?.rasterItems?.length > 0) {
        for (const r of result.rasterItems) {
          if (!activeDatasetId && r.isGeoreferenced) {
            setActiveDatasetId(r.datasetId)
          } else if (activeDatasetId && r.datasetId && r.datasetId !== activeDatasetId) {
            await workspace.attachDataset(r.datasetId).catch(() => undefined)
          }
        }
        await workspace.refreshManifest().catch(() => undefined)
      }
      await refreshGISLayers()
      setGisFeaturesRefreshKey((k) => k + 1)
      setRecordsRefreshKey((k) => k + 1)

      const addedVec = result?.addedVectorLayers || []
      const addedRas = result?.addedRasterLayers || []
      if (addedVec.length > 0 || addedRas.length > 0) {
        notifyGIS(`Added ${addedVec.length} vector and ${addedRas.length} raster layer(s)`)
        const layerWithBounds = addedVec.find(
          (l) =>
            (Array.isArray(l.extent) && l.extent.length === 4) ||
            (Array.isArray(l.bounds) && l.bounds.length === 4)
        )
        const firstBounds = layerWithBounds?.extent || layerWithBounds?.bounds
        if (firstBounds) setTimeout(() => fitMapBounds(firstBounds), 250)
      }
    },
    [activeDatasetId, workspace, refreshGISLayers, notifyGIS, fitMapBounds]
  )

  function handleDetach(member) {
    if (!window.confirm(`Remove "${member.name}" from this workspace? Uploaded files are preserved.`)) return
    workspace
      .detachDataset(member.dataset_id)
      .then(() => {
        setRecordsRefreshKey((k) => k + 1)
      })
      .catch(() => undefined)
  }

  const handleGeoreferenceTarget = useCallback(
    (entry) => {
      const dsId = entry?.entryKind === "member" ? entry.id : entry?.raw?.dataset_id || entry?.dataset_id
      const fallbackMember = workspace.manifest?.members?.[0]?.dataset_id
      const fallbackRaster = gisLayers.find((l) => l.layer_type === "raster" && l.dataset_id)?.dataset_id
      const targetId = dsId || fallbackMember || fallbackRaster
      if (!targetId) {
        notifyGIS("Add a raster dataset first to open the Georeferencer.")
        return
      }
      const returnTo = mapId
        ? `/maps/${mapId}/edit?datasetId=${encodeURIComponent(targetId)}`
        : `/datasets/${encodeURIComponent(targetId)}/workspace`
      navigate(`/datasets/${encodeURIComponent(targetId)}/georeference?returnTo=${encodeURIComponent(returnTo)}`)
    },
    [workspace.manifest?.members, gisLayers, mapId, navigate, notifyGIS]
  )

  const handleSelectTool = useCallback(
    (toolId) => {
      setMeasureMode(null)
      if (toolId === "select" || toolId === "identify") {
        setActiveVectorTool(toolId)
        return
      }
      if (!activeGISLayer && gisLayers.length > 0) {
        setActiveGISLayer(gisLayers.find((l) => l.layer_type === "vector") || gisLayers[0])
      } else if (!activeGISLayer) {
        notifyGIS("Create or import a Vector Layer in the Layers panel to draw or edit.")
        updateLayout((prev) => ({ ...prev, leftOpen: true, layersOpen: true }))
        return
      }
      setActiveVectorTool(toolId)
    },
    [activeGISLayer, gisLayers, notifyGIS, updateLayout]
  )

  const handleTogglePanel = useCallback(
    (which) => {
      updateLayout((prev) => {
        if (which === "browser") {
          const nextBrowser = !(prev.leftOpen && prev.browserOpen)
          return { ...prev, leftOpen: nextBrowser || prev.layersOpen, browserOpen: nextBrowser }
        }
        if (which === "layers") {
          const nextLayers = !(prev.leftOpen && prev.layersOpen)
          return { ...prev, leftOpen: nextLayers || prev.browserOpen, layersOpen: nextLayers }
        }
        if (which === "left") {
          const nextLeft = !prev.leftOpen
          return {
            ...prev,
            leftOpen: nextLeft,
            browserOpen: nextLeft ? prev.browserOpen || !prev.layersOpen : prev.browserOpen,
            layersOpen: nextLeft ? prev.layersOpen || !prev.browserOpen : prev.layersOpen,
          }
        }
        if (which === "right") return { ...prev, rightOpen: !prev.rightOpen }
        if (which === "records") return { ...prev, recordsOpen: !prev.recordsOpen }
        return prev
      })
    },
    [updateLayout]
  )

  const handleRunMenuAlgorithm = useCallback(
    (algId) => {
      setRightDockTab("toolbox")
      setRequestedAlgId(algId)
      setRequestedAlgTick((t) => t + 1)
      updateLayout((prev) => ({ ...prev, rightOpen: true }))
    },
    [updateLayout]
  )

  // Resizable Drag Handles (pointer events — never reinitializes MapLibre!)
  const startDrag = useCallback(
    (kind, startEvent) => {
      startEvent.preventDefault()
      const startX = startEvent.clientX
      const startY = startEvent.clientY
      const initLeft = layout.leftWidth
      const initRight = layout.rightWidth
      const initRecords = layout.recordsHeight
      const initSplit = layout.browserSplitRatio
      const leftRect = leftDockRef.current?.getBoundingClientRect()

      const onMove = (ev) => {
        if (kind === "left") {
          const next = clamp(initLeft + (ev.clientX - startX), LAYOUT_LIMITS.leftMin, LAYOUT_LIMITS.leftMax)
          updateLayout((p) => ({ ...p, leftWidth: next }))
        } else if (kind === "right") {
          const next = clamp(initRight - (ev.clientX - startX), LAYOUT_LIMITS.rightMin, LAYOUT_LIMITS.rightMax)
          updateLayout((p) => ({ ...p, rightWidth: next }))
        } else if (kind === "records") {
          const next = clamp(
            initRecords - (ev.clientY - startY),
            LAYOUT_LIMITS.recordsMin,
            LAYOUT_LIMITS.recordsMax
          )
          updateLayout((p) => ({ ...p, recordsHeight: next }))
        } else if (kind === "left-split" && leftRect && leftRect.height > 100) {
          const ratio = clamp(
            initSplit + (ev.clientY - startY) / leftRect.height,
            LAYOUT_LIMITS.splitMin,
            LAYOUT_LIMITS.splitMax
          )
          updateLayout((p) => ({ ...p, browserSplitRatio: ratio }))
        }
      }

      const onUp = () => {
        window.removeEventListener("mousemove", onMove)
        window.removeEventListener("mouseup", onUp)
        document.body.style.cursor = ""
        document.body.style.userSelect = ""
      }

      document.body.style.cursor = kind === "records" || kind === "left-split" ? "row-resize" : "col-resize"
      document.body.style.userSelect = "none"
      window.addEventListener("mousemove", onMove)
      window.addEventListener("mouseup", onUp)
    },
    [layout, updateLayout]
  )

  // Centralized Keyboard Shortcut Listener — single source of truth via shortcutRegistry.js
  useEffect(() => {
    const anyModalOpen =
      addOpen ||
      crsModalOpen ||
      datasetBrowserOpen ||
      exportModalOpen ||
      reopenModalOpen ||
      shareModalOpen ||
      baseLayerModalOpen ||
      saveConfigModalOpen ||
      shortcutsModalOpen ||
      guideModalOpen ||
      Boolean(enhancementTarget) ||
      Boolean(annotationTarget)

    const handleKeyDown = (event) => {
      if (isTextInputTarget(event.target)) return
      const matched = effectiveShortcuts.find(
        (entry) => entry.enabled !== false && matchesShortcutEvent(event, entry.shortcut)
      )
      if (!matched) return

      if (matched.tool_id === "cancel_action") {
        event.preventDefault()
        if (shortcutsModalOpen) return setShortcutsModalOpen(false)
        if (guideModalOpen) return setGuideModalOpen(false)
        if (addOpen) return setAddOpen(false)
        if (baseLayerModalOpen) return setBaseLayerModalOpen(false)
        if (crsModalOpen) return setCrsModalOpen(false)
        if (datasetBrowserOpen) return setDatasetBrowserOpen(false)
        if (attributeTableLayer) return setAttributeTableLayer(null)
        if (measureMode) return setMeasureMode(null)
        controllerActions.clearDraft?.()
        setActiveVectorTool("select")
        setSelectedFeatureIds([])
        return
      }

      if (anyModalOpen) return

      event.preventDefault()
      switch (matched.tool_id) {
        case "view_map":
          updateLayout((p) => ({ ...p, recordsOpen: false }))
          break
        case "view_records":
        case "attribute_table":
          handleTogglePanel("records")
          break
        case "select":
        case "identify":
        case "point":
        case "line":
        case "polygon":
        case "move":
        case "vertex":
          handleSelectTool(matched.tool_id)
          break
        case "snap":
          setSnapEnabled((p) => !p)
          notifyGIS(snapEnabled ? "Vertex snapping off" : "Vertex snapping on")
          break
        case "undo":
          controllerActions.undo?.()
          break
        case "redo":
          controllerActions.redo?.()
          break
        case "delete_selected":
          controllerActions.deleteSelected?.()
          break
        case "layer_manager":
          handleTogglePanel("layers")
          break
        case "base_layer":
          setBaseLayerModalMode("select")
          setBaseLayerModalOpen(true)
          break
        case "zoom_full":
          handleZoomFullExtent()
          break
        case "symbology": {
          const target = activeGISLayer || gisLayers[0]
          if (target) {
            setStyleEditorLayer(target)
            setRightDockTab("symbology")
            updateLayout((p) => ({ ...p, rightOpen: true }))
          } else {
            notifyGIS("Select a vector layer first to edit Symbology.")
          }
          break
        }
        case "crs_manager":
          setCrsModalOpen(true)
          break
        case "dataset_browser":
          handleTogglePanel("browser")
          break
        case "processing_toolbox":
          setRightDockTab("toolbox")
          handleTogglePanel("right")
          break
        case "add_dataset":
          openUnifiedAddModal(null)
          break
        case "shortcuts_help":
          setShortcutsModalOpen(true)
          break
        default:
          break
      }
    }

    window.addEventListener("keydown", handleKeyDown)
    return () => window.removeEventListener("keydown", handleKeyDown)
  }, [
    effectiveShortcuts,
    addOpen,
    crsModalOpen,
    datasetBrowserOpen,
    exportModalOpen,
    reopenModalOpen,
    shareModalOpen,
    baseLayerModalOpen,
    saveConfigModalOpen,
    shortcutsModalOpen,
    guideModalOpen,
    enhancementTarget,
    annotationTarget,
    attributeTableLayer,
    measureMode,
    activeGISLayer,
    gisLayers,
    snapEnabled,
    controllerActions,
    handleTogglePanel,
    handleSelectTool,
    handleZoomFullExtent,
    openUnifiedAddModal,
    updateLayout,
    notifyGIS,
  ])

  const effectiveManifest =
    workspace.manifest ||
    (mapId || mapspaceId
      ? {
          id: gisMapId,
          name: currentMap?.name || "Workspace",
          active_view: "map",
          members: [],
          buildings: [],
          camera: null,
        }
      : null)

  if (mapNotFound) {
    return (
      <div className="h-screen w-full flex flex-col items-center justify-center p-6 text-center bg-neutral-50 dark:bg-[#171717]">
        <div className="max-w-md w-full p-8 rounded-xl bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-xl">
          <AlertCircle className="size-8 mx-auto mb-3 text-amber-500" />
          <h2 className="text-base font-bold text-neutral-900 dark:text-white">Map Not Found</h2>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1.5 mb-5">
            This map does not exist or is no longer accessible.
          </p>
          <Link
            to="/dashboard"
            className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] font-semibold text-xs"
          >
            <ArrowLeft className="size-3.5" /> Return to Dashboard
          </Link>
        </div>
      </div>
    )
  }

  if (workspace.loading || mapLoading) {
    return (
      <div className="h-screen w-full flex items-center justify-center gap-2 bg-neutral-50 dark:bg-[#181818] text-xs text-neutral-500">
        <Loader2 className="size-4 animate-spin text-[#3ecf8e]" /> Loading GIS Workspace…
      </div>
    )
  }

  if (!effectiveManifest) {
    return (
      <div className="h-screen w-full flex flex-col items-center justify-center gap-3 p-6 text-center bg-neutral-50 dark:bg-[#171717]">
        <AlertCircle className="size-8 text-amber-500" />
        <strong className="text-sm text-neutral-900 dark:text-white">
          {workspace.error?.message || "Workspace not found in current session"}
        </strong>
        <Link
          to="/dashboard"
          className="px-4 py-2 bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-semibold rounded-[6px]"
        >
          Return to Dashboard
        </Link>
      </div>
    )
  }

  const workspaceDisplayName = currentMap?.name || effectiveManifest.name || "Untitled Workspace"
  const activeCRS = activeGISLayer?.crs || activeBaseLayer?.crs || "EPSG:4326"
  const showLeftDock = layout.leftOpen && (layout.browserOpen || layout.layersOpen)
  const hasGeorefRaster =
    (effectiveManifest.members?.length || 0) > 0 ||
    gisLayers.some((l) => l.layer_type === "raster" && l.dataset_id)

  return (
    <div className="qgis-workspace-root h-screen w-full flex flex-col overflow-hidden bg-neutral-200 dark:bg-[#141414] text-neutral-900 dark:text-neutral-100">
      {/* 1. Desktop GIS Title Strip & Top Menu Bar */}
      <WorkspaceMenuBar
        workspaceName={workspaceDisplayName}
        revisionNumber={currentMap?.revision_count || currentMap?.revision_number}
        lockHeld={lockHeld}
        isLockedByOther={isLockedByOther}
        lockHolderName={lockHolderName}
        saveState={workspace.saveState}
        onRetrySave={workspace.retrySave}
        user={user}
        layout={layout}
        canUndo={controllerActions.canUndo}
        canRedo={controllerActions.canRedo}
        hasSelection={selectedFeatureIds.length > 0}
        hasActiveLayer={Boolean(activeGISLayer)}
        snapEnabled={snapEnabled}
        mapViewPrefs={mapViewPrefs}
        shortcuts={effectiveShortcuts}
        onSave={openSaveConfigDialog}
        onSaveAs={openSaveConfigDialog}
        onOpenProperties={() => setGuideModalOpen(true)}
        onExport={openExportModal}
        onReopen={() => {
          if (confirmDiscard()) setReopenModalOpen(true)
        }}
        onShare={currentMap?.id ? () => setShareModalOpen(true) : null}
        onCloseWorkspace={() => {
          if (confirmDiscard()) navigate("/dashboard")
        }}
        onUndo={() => controllerActions.undo?.()}
        onRedo={() => controllerActions.redo?.()}
        onSelectTool={handleSelectTool}
        onToggleSnap={() => setSnapEnabled((p) => !p)}
        onDeleteSelected={() => controllerActions.deleteSelected?.()}
        onZoomIn={() => mapInstance?.zoomIn({ duration: 250 })}
        onZoomOut={() => mapInstance?.zoomOut({ duration: 250 })}
        onZoomFullExtent={handleZoomFullExtent}
        onZoomToActiveLayer={() => handleZoomToLayer(activeGISLayer)}
        onTogglePanel={handleTogglePanel}
        onResetLayout={handleResetLayout}
        onAddVector={() => openUnifiedAddModal("vector")}
        onAddRaster={() => openUnifiedAddModal("raster")}
        onChangeBaseLayer={() => {
          setBaseLayerModalMode("select")
          setBaseLayerModalOpen(true)
        }}
        onOpenLayerProperties={() => {
          const target = activeGISLayer || gisLayers[0]
          if (target) {
            setStyleEditorLayer(target)
            setRightDockTab("symbology")
            updateLayout((p) => ({ ...p, rightOpen: true }))
          }
        }}
        onOpenAttributeTable={() => {
          const target = activeGISLayer || gisLayers[0]
          if (target) setAttributeTableLayer(target)
        }}
        onRemoveActiveLayer={async () => {
          if (!activeGISLayer) return
          if (!window.confirm(`Remove layer "${activeGISLayer.layer_name}" from workspace?`)) return
          try {
            await deleteGISLayer(activeGISLayer.layer_id)
            notifyGIS(`Removed "${activeGISLayer.layer_name}"`)
            refreshGISLayers()
            setRecordsRefreshKey((k) => k + 1)
          } catch {
            notifyGIS("Unable to remove layer")
          }
        }}
        onRunAlgorithm={handleRunMenuAlgorithm}
        onOpenProcessingToolbox={() => {
          setRightDockTab("toolbox")
          updateLayout((p) => ({ ...p, rightOpen: true }))
        }}
        onSearchProcessing={() => {
          setRightDockTab("toolbox")
          updateLayout((p) => ({ ...p, rightOpen: true }))
          setFocusSearchTick((t) => t + 1)
        }}
        onOpenProcessingHistory={() => {
          setRightDockTab("history")
          updateLayout((p) => ({ ...p, rightOpen: true }))
        }}
        onChangeMapViewPrefs={handleChangeMapViewPrefs}
        onOpenCRS={() => setCrsModalOpen(true)}
        onOpenShortcuts={() => setShortcutsModalOpen(true)}
        onOpenGuide={() => setGuideModalOpen(true)}
      />

      {/* 2. Compact Desktop GIS Icon Toolbars */}
      <WorkspaceToolbar
        shortcuts={effectiveShortcuts}
        activeTool={activeVectorTool}
        measureMode={measureMode}
        snapEnabled={snapEnabled}
        canUndo={controllerActions.canUndo}
        canRedo={controllerActions.canRedo}
        hasSelection={selectedFeatureIds.length > 0}
        hasActiveLayer={Boolean(activeGISLayer)}
        hasGeoreferenceableRaster={hasGeorefRaster}
        isLockedByOther={isLockedByOther}
        layout={layout}
        mapViews={mapViews}
        mapViewPrefs={mapViewPrefs}
        activeBaseLayer={activeBaseLayer}
        onSave={openSaveConfigDialog}
        onAddDataset={() => openUnifiedAddModal(null)}
        onExport={openExportModal}
        onSelectTool={handleSelectTool}
        onZoomIn={() => mapInstance?.zoomIn({ duration: 250 })}
        onZoomOut={() => mapInstance?.zoomOut({ duration: 250 })}
        onZoomFullExtent={handleZoomFullExtent}
        onZoomToActiveLayer={() => handleZoomToLayer(activeGISLayer)}
        onAddBookmark={handleAddBookmark}
        onToggleRecords={() => handleTogglePanel("records")}
        onOpenAttributeTable={() => {
          const target = activeGISLayer || gisLayers[0]
          if (target) setAttributeTableLayer((p) => (p ? null : target))
        }}
        onToggleSnap={() => setSnapEnabled((p) => !p)}
        onUndo={() => controllerActions.undo?.()}
        onRedo={() => controllerActions.redo?.()}
        onDeleteSelected={() => controllerActions.deleteSelected?.()}
        onToggleMeasure={(mode) => setMeasureMode((prev) => (prev === mode ? null : mode))}
        onGeoreferenceActive={() => handleGeoreferenceTarget(null)}
        onSelectMapView={handleSelectMapView}
        onOpenBaseLayerModal={() => {
          setBaseLayerModalMode("select")
          setBaseLayerModalOpen(true)
        }}
        onOpenCRSModal={() => setCrsModalOpen(true)}
        onToggleLeftDock={() => handleTogglePanel("left")}
        onToggleRightDock={() => handleTogglePanel("right")}
        onOpenShortcuts={() => setShortcutsModalOpen(true)}
      />

      {/* Error & Toast Notifications */}
      {workspaceToast && (
        <div className="fixed top-20 right-4 z-[1000] flex items-center gap-2 px-3 py-1.5 rounded-[6px] bg-neutral-900 text-white dark:bg-white dark:text-neutral-900 text-[11px] font-semibold shadow-xl border border-neutral-700 dark:border-neutral-200">
          <Check className="size-3.5 text-[#3ecf8e]" />
          <span>{workspaceToast}</span>
        </div>
      )}
      {workspace.error && (
        <div className="px-3 py-1 bg-red-500/15 border-b border-red-500/30 text-red-600 dark:text-red-300 text-[11px] flex items-center justify-between gap-2 shrink-0">
          <span className="truncate">
            <strong>{workspace.error.message}</strong> {workspace.error.details}
          </span>
          <button type="button" onClick={() => workspace.setError(null)} className="cursor-pointer">
            <X className="size-3.5" />
          </button>
        </div>
      )}

      {/* 3. Main Three-Column Dockable Body + Bottom Dockable Records Panel */}
      <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
        {/* Upper Workspace Area: Left Dock | Central Viewscape | Right Dock */}
        <div className="flex-1 flex min-h-0 overflow-hidden">
          {/* LEFT DOCK: Browser (Top) + Splitter + Three-Group Layers (Bottom) */}
          {showLeftDock && (
            <>
              <aside
                ref={leftDockRef}
                aria-label="Left GIS Dock (Browser and Layers)"
                style={{ width: layout.leftWidth }}
                className="shrink-0 flex flex-col min-h-0 border-r border-neutral-300 dark:border-[#323232] bg-white dark:bg-[#1e1e1e] overflow-hidden"
              >
                {layout.browserOpen && (
                  <div
                    style={{
                      height: layout.layersOpen ? `${Math.round(layout.browserSplitRatio * 100)}%` : "100%",
                    }}
                    className="min-h-0 overflow-hidden flex flex-col"
                  >
                    <Suspense
                      fallback={
                        <div className="p-3 text-[11px] text-neutral-400 flex items-center gap-1.5">
                          <Loader2 className="size-3 animate-spin" /> Loading Browser…
                        </div>
                      }
                    >
                      <WorkspaceBrowserPanel
                        workspaceId={gisMapId}
                        userId={user?.id}
                        customBaseLayers={customBaseLayers}
                        mapViews={mapViews}
                        bookmarks={bookmarks}
                        onAddBookmark={handleAddBookmark}
                        onSelectBookmark={handleSelectBookmark}
                        onDeleteBookmark={handleDeleteBookmark}
                        onSelectMapView={handleSelectMapView}
                        onSelectCustomBaseLayer={handleSelectCustomBaseLayer}
                        onOpenSavedMap={(m) => {
                          if (m?.id && confirmDiscard()) navigate(`/maps/${m.id}/edit`)
                        }}
                        onLayersAdded={(addedLayers) => {
                          refreshGISLayers()
                          setGisFeaturesRefreshKey((k) => k + 1)
                          setRecordsRefreshKey((k) => k + 1)
                          const b = addedLayers?.find((l) => Array.isArray(l.bounds) && l.bounds.length === 4)?.bounds
                          if (b) setTimeout(() => fitMapBounds(b), 250)
                        }}
                        onOpenUpload={() => openUnifiedAddModal(null)}
                        onCollapse={() => handleTogglePanel("browser")}
                        notify={notifyGIS}
                      />
                    </Suspense>
                  </div>
                )}

                {layout.browserOpen && layout.layersOpen && (
                  <div
                    role="separator"
                    aria-orientation="horizontal"
                    aria-label="Resize Browser and Layers split"
                    onMouseDown={(e) => startDrag("left-split", e)}
                    className="h-1.5 shrink-0 cursor-row-resize bg-neutral-200 dark:bg-[#2c2c2c] hover:bg-[#3ecf8e]/60 transition-colors"
                  />
                )}

                {layout.layersOpen && (
                  <div className="flex-1 min-h-0 overflow-hidden flex flex-col">
                    <ThreeGroupLayersPanel
                      mapId={gisMapId}
                      gisLayers={gisLayers}
                      members={effectiveManifest.members}
                      activeGISLayer={activeGISLayer}
                      activeBaseLayer={activeBaseLayer}
                      mapViewPrefs={mapViewPrefs}
                      layerOrder={layerOrder}
                      onToggleLayerOrder={handleToggleLayerOrder}
                      onSelectGISLayer={setActiveGISLayer}
                      onLayerUpdatedImmediate={handleGISLayerImmediateUpdate}
                      onRefreshGISLayers={() => {
                        refreshGISLayers()
                        setRecordsRefreshKey((k) => k + 1)
                      }}
                      onOpenAttributeTable={(lyr) => {
                        setActiveGISLayer(lyr)
                        setAttributeTableLayer(lyr)
                      }}
                      onOpenStyleEditor={(lyr) => {
                        setActiveGISLayer(lyr)
                        setStyleEditorLayer(lyr)
                        setRightDockTab("symbology")
                        updateLayout((p) => ({ ...p, rightOpen: true }))
                      }}
                      onZoomToLayer={handleZoomToLayer}
                      onUpdateMember={(dsId, patch) => workspace.updateMember(dsId, patch).catch(() => undefined)}
                      onReorderMembers={(ids) => workspace.reorderMembers(ids).catch(() => undefined)}
                      onDetachMember={handleDetach}
                      onGeoreferenceMember={handleGeoreferenceTarget}
                      onOpenTransformMember={(member) => setTransformingMember(member)}
                      onOpenEnhanceMember={(member) =>
                        setEnhancementTarget({
                          datasetId: member.dataset_id,
                          name: member.name || member.dataset?.filename,
                        })
                      }
                      onOpenAddVector={() => openUnifiedAddModal("vector")}
                      onOpenAddRaster={() => openUnifiedAddModal("raster")}
                      onOpenChangeBaseLayer={() => {
                        setBaseLayerModalMode("select")
                        setBaseLayerModalOpen(true)
                      }}
                      onChangeBasePrefs={handleChangeMapViewPrefs}
                      onCollapse={() => handleTogglePanel("layers")}
                      notify={notifyGIS}
                    />
                  </div>
                )}
              </aside>

              {/* Left Dock Resize Handle */}
              <div
                role="separator"
                aria-orientation="vertical"
                aria-label="Resize left dock"
                onMouseDown={(e) => startDrag("left", e)}
                className="w-1 shrink-0 cursor-col-resize bg-neutral-300/60 dark:bg-[#2a2a2a] hover:bg-[#3ecf8e] transition-colors"
              />
            </>
          )}

          {/* CENTRAL VIEWSCAPE — MapLibre GL JS stays permanently mounted! */}
          <main className="flex-1 relative min-w-0 min-h-0 overflow-hidden bg-[#0f1318]">
            <ErrorBoundary title="Map Canvas Error">
              <WorkspaceMap
                key={id || gisMapId}
                qgisMode
                manifest={effectiveManifest}
                overlayRefreshKey={overlayRefreshKey}
                members={effectiveManifest.members}
                records={workspace.mapRecords}
                selectedRecordId={workspace.selectedRecordId}
                onSelectRecord={(recId) => {
                  workspace.setSelectedRecordId(recId)
                  if (recId) updateLayout((p) => ({ ...p, recordsOpen: true }))
                }}
                savedCamera={effectiveManifest.camera}
                onCameraChange={(camera) => {
                  if (workspace.manifest) workspace.updateCamera(camera)
                }}
                transformMember={transformingMember}
                onEndTransform={() => setTransformingMember(null)}
                onTransformApplied={() => workspace.refreshManifest().catch(() => undefined)}
                activeFloorLevel={activeFloorLevel}
                activeBuildingName={activeBuildingName}
                onActiveFloorChange={(lvl, bName) => handleSelectFloor(lvl, bName)}
                onRecordsRefresh={() => {
                  setRecordsRefreshKey((k) => k + 1)
                  return Promise.all([
                    workspace.refreshRecords(),
                    workspace.refreshMapRecords(),
                    workspace.refreshManifest(),
                  ])
                }}
                primaryDatasetId={id}
                onMemberUpdate={(dsId, patch) => workspace.updateMember(dsId, patch).catch(() => undefined)}
                buildings={effectiveManifest.buildings}
                isolateActiveFloor={isolateActiveFloor}
                onToggleIsolateActiveFloor={setIsolateActiveFloor}
                onMapReady={setMapInstance}
                hasGISLayers
                layerOrder={layerOrder}
                mapViews={mapViews}
                mapViewPrefs={mapViewPrefs}
                customBaseLayer={activeBaseLayer}
                customBaseLayers={customBaseLayers}
                baseLayerFitTrigger={baseLayerFitTrigger}
                onSelectMapView={handleSelectMapView}
                onSelectCustomBaseLayer={handleSelectCustomBaseLayer}
                onChangeMapViewPrefs={handleChangeMapViewPrefs}
                onOpenBaseLayerModal={() => {
                  setBaseLayerModalMode("select")
                  setBaseLayerModalOpen(true)
                }}
                onOpenUploadBaseLayer={() => {
                  setBaseLayerModalMode("upload")
                  setBaseLayerModalOpen(true)
                }}
              />

              {mapInstance && (
                <MapLibreGISController
                  map={mapInstance}
                  gisLayers={gisLayers}
                  layerOrder={layerOrder}
                  activeLayer={activeGISLayer}
                  activeTool={measureMode ? "select" : activeVectorTool}
                  onSelectTool={handleSelectTool}
                  snapEnabled={snapEnabled}
                  onToggleSnap={setSnapEnabled}
                  selectedFeatureIds={selectedFeatureIds}
                  onSelectFeatures={setSelectedFeatureIds}
                  featuresRefreshKey={gisFeaturesRefreshKey}
                  onFeaturesMutated={() => {
                    setGisFeaturesRefreshKey((k) => k + 1)
                    setRecordsRefreshKey((k) => k + 1)
                    refreshGISLayers()
                  }}
                  onControllerActionsReady={setControllerActions}
                  onOpenInRecords={() => updateLayout((p) => ({ ...p, recordsOpen: true }))}
                  notify={notifyGIS}
                />
              )}

              {mapInstance && (
                <MapMeasurementOverlay
                  map={mapInstance}
                  mode={measureMode}
                  onClose={() => setMeasureMode(null)}
                />
              )}

              {attributeTableLayer && (
                <AttributeTablePanel
                  layer={attributeTableLayer}
                  selectedFeatureIds={selectedFeatureIds}
                  onSelectFeatures={setSelectedFeatureIds}
                  onZoomToFeature={handleZoomToGISFeature}
                  onFeaturesMutated={() => {
                    setGisFeaturesRefreshKey((k) => k + 1)
                    setRecordsRefreshKey((k) => k + 1)
                    refreshGISLayers()
                  }}
                  onClose={() => setAttributeTableLayer(null)}
                  notify={notifyGIS}
                />
              )}
            </ErrorBoundary>
          </main>

          {/* RIGHT DOCK: Collapsible Processing Toolbox (+ History + Layer Style) */}
          {layout.rightOpen && (
            <>
              <div
                role="separator"
                aria-orientation="vertical"
                aria-label="Resize Processing Toolbox"
                onMouseDown={(e) => startDrag("right", e)}
                className="w-1 shrink-0 cursor-col-resize bg-neutral-300/60 dark:bg-[#2a2a2a] hover:bg-[#3ecf8e] transition-colors"
              />
              <aside
                aria-label="Processing Toolbox Dock"
                style={{ width: layout.rightWidth }}
                className="shrink-0 flex flex-col min-h-0 border-l border-neutral-300 dark:border-[#323232] bg-white dark:bg-[#1e1e1e] overflow-hidden"
              >
                <Suspense
                  fallback={
                    <div className="p-3 text-[11px] text-neutral-400 flex items-center gap-1.5">
                      <Loader2 className="size-3 animate-spin" /> Loading Processing Toolbox…
                    </div>
                  }
                >
                  <DockedProcessingToolbox
                    mapId={gisMapId}
                    layers={gisLayers}
                    datasets={effectiveManifest.members || []}
                    activeLayer={activeGISLayer}
                    activeDockTab={rightDockTab}
                    onChangeDockTab={setRightDockTab}
                    requestedAlgorithmId={requestedAlgId}
                    requestedAlgorithmTick={requestedAlgTick}
                    focusSearchTick={focusSearchTick}
                    styleEditorLayer={styleEditorLayer}
                    onStyleSaved={() => {
                      setGisFeaturesRefreshKey((k) => k + 1)
                      refreshGISLayers()
                    }}
                    onJobCompleted={() => {
                      setGisFeaturesRefreshKey((k) => k + 1)
                      setRecordsRefreshKey((k) => k + 1)
                      refreshGISLayers()
                      workspace.refreshManifest().catch(() => undefined)
                    }}
                    onOpenAttributeTable={(lyr) => {
                      setActiveGISLayer(lyr)
                      setAttributeTableLayer(lyr)
                    }}
                    onCollapse={() => handleTogglePanel("right")}
                    notify={notifyGIS}
                  />
                </Suspense>
              </aside>
            </>
          )}
        </div>

        {/* BOTTOM DOCKABLE PANEL: Workspace Records (F6 / View menu) */}
        {layout.recordsOpen && (
          <>
            <div
              role="separator"
              aria-orientation="horizontal"
              aria-label="Resize Workspace Records panel"
              onMouseDown={(e) => startDrag("records", e)}
              className="h-1.5 shrink-0 cursor-row-resize bg-neutral-300/70 dark:bg-[#2c2c2c] hover:bg-[#3ecf8e] transition-colors"
            />
            <section
              aria-label="Dockable Workspace Records Panel"
              style={{ height: layout.recordsHeight }}
              className="shrink-0 flex flex-col min-h-0 border-t border-neutral-300 dark:border-[#343434] bg-white dark:bg-[#181a1f] overflow-hidden"
            >
              <div className="h-6 px-2.5 bg-neutral-100 dark:bg-[#232323] border-b border-neutral-200 dark:border-[#303030] flex items-center justify-between text-[10.5px] shrink-0 select-none">
                <div className="flex items-center gap-1.5 font-bold uppercase tracking-wider text-neutral-700 dark:text-neutral-200">
                  <Database className="size-3 text-[#3ecf8e]" />
                  <span>Workspace Records</span>
                  <span className="font-mono text-neutral-400 font-normal">(F6)</span>
                </div>
                <button
                  type="button"
                  onClick={() => updateLayout((p) => ({ ...p, recordsOpen: false }))}
                  className="p-0.5 rounded hover:bg-neutral-200 dark:hover:bg-[#343434] text-neutral-500 cursor-pointer"
                  title="Close Workspace Records Dock"
                >
                  <X className="size-3" />
                </button>
              </div>
              <div className="flex-1 min-h-0 overflow-hidden">
                <Suspense
                  fallback={
                    <div className="h-full flex items-center justify-center text-xs text-neutral-400">
                      <Loader2 className="size-4 animate-spin mr-2" /> Loading Workspace Records…
                    </div>
                  }
                >
                  <WorkspaceRecordsView
                    workspaceId={gisMapId}
                    gisLayers={gisLayers}
                    refreshKey={recordsRefreshKey}
                    selectedFeatureIds={selectedFeatureIds}
                    onSelectFeatureIds={setSelectedFeatureIds}
                    onOpenAddDataset={() => openUnifiedAddModal(null)}
                    onLocateRecord={handleLocateRecord}
                    onLayerVisibilityChanged={() => {
                      refreshGISLayers()
                      setGisFeaturesRefreshKey((k) => k + 1)
                    }}
                    onRecordsMutated={() => {
                      refreshGISLayers()
                      setGisFeaturesRefreshKey((k) => k + 1)
                      workspace.refreshManifest().catch(() => undefined)
                    }}
                    notify={notifyGIS}
                  />
                </Suspense>
              </div>
            </section>
          </>
        )}
      </div>

      {/* 4. Bottom Status Bar */}
      <WorkspaceStatusBar
        map={mapInstance}
        activeCRS={activeCRS}
        loadingStatus={statusLoadingLabel}
        activeLayerName={activeGISLayer?.layer_name || ""}
        onOpenCRS={() => setCrsModalOpen(true)}
        onResetNorth={() => mapInstance?.easeTo({ bearing: 0, pitch: 0, duration: 350 })}
      />

      {/* Supporting Modals (Preserving all existing GIS & workspace workflows) */}
      <CRSManagerModal
        open={crsModalOpen}
        onClose={() => setCrsModalOpen(false)}
        layers={gisLayers}
        datasets={effectiveManifest.members || []}
        activeLayer={activeGISLayer}
        onReprojected={() => {
          setGisFeaturesRefreshKey((k) => k + 1)
          refreshGISLayers()
          workspace.refreshManifest().catch(() => undefined)
        }}
        notify={notifyGIS}
      />

      <DatasetBrowserModal
        open={datasetBrowserOpen}
        onClose={() => setDatasetBrowserOpen(false)}
        mapId={gisMapId}
        onDatasetAttached={() => {
          refreshGISLayers()
          setRecordsRefreshKey((k) => k + 1)
          workspace.refreshManifest().catch(() => undefined)
        }}
        onDatasetsChanged={() => {
          refreshGISLayers()
          setRecordsRefreshKey((k) => k + 1)
          workspace.refreshManifest().catch(() => undefined)
        }}
        notify={notifyGIS}
      />

      {addOpen && (
        <div
          className="fixed inset-0 z-[1050] bg-black/70 backdrop-blur-xs flex items-center justify-center p-4"
          role="dialog"
          aria-modal="true"
          aria-label="Add Dataset to Workspace"
        >
          <div className="w-full max-w-4xl max-h-[90vh] rounded-[12px] bg-[#f8f9fb] dark:bg-[#131519] border border-neutral-200 dark:border-[#2e323a] shadow-2xl flex flex-col overflow-hidden">
            <div className="px-5 py-3 border-b border-neutral-200 dark:border-[#2a2d34] bg-white dark:bg-[#191b1f] flex items-center justify-between">
              <div>
                <h2 className="text-sm font-bold text-neutral-900 dark:text-white">
                  Add Datasets &amp; Configure Layers
                </h2>
                <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
                  Add optional Vector Layers, Raster Layers, or configure your Base Layer.
                </p>
              </div>
              <button
                type="button"
                onClick={() => {
                  setAddOpen(false)
                  setAddFocusSection(null)
                }}
                className="p-1.5 rounded-[6px] text-neutral-500 hover:bg-neutral-100 dark:hover:bg-[#2a2d34] cursor-pointer"
                aria-label="Close Add Dataset Modal"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <div className="p-5 overflow-y-auto flex-1">
              <Suspense
                fallback={
                  <div className="py-12 flex items-center justify-center text-xs text-neutral-500">
                    <Loader2 className="animate-spin size-4 mr-2" /> Loading Unified Upload Panel…
                  </div>
                }
              >
                <UnifiedDatasetUploadPanel
                  workspaceId={gisMapId}
                  workspaceName={workspaceDisplayName}
                  initialFocusSection={addFocusSection}
                  primaryActionLabel="Apply to Workspace"
                  showBackButton
                  onBack={() => {
                    setAddOpen(false)
                    setAddFocusSection(null)
                  }}
                  onComplete={handleUnifiedAddComplete}
                />
              </Suspense>
            </div>
          </div>
        </div>
      )}

      <Suspense fallback={null}>
        <KeyboardShortcutsModal
          open={shortcutsModalOpen}
          shortcuts={effectiveShortcuts}
          onClose={() => setShortcutsModalOpen(false)}
          onAssignShortcut={handleAssignShortcut}
          onResetDefaults={handleResetShortcuts}
        />
      </Suspense>

      <WorkspaceGuideModal
        open={guideModalOpen}
        workspaceName={workspaceDisplayName}
        crs={activeCRS}
        vectorCount={gisLayers.filter((l) => l.layer_type === "vector" || !l.layer_type).length}
        rasterCount={
          (effectiveManifest.members?.length || 0) +
          gisLayers.filter((l) => l.layer_type === "raster").length
        }
        onClose={() => setGuideModalOpen(false)}
      />

      {saveConfigModalOpen && (
        <div className="workspace-dialog-backdrop" role="presentation">
          <section className="workspace-dialog" role="dialog" aria-modal="true" aria-labelledby="save-config-title">
            <button
              type="button"
              className="workspace-dialog-close"
              onClick={() => setSaveConfigModalOpen(false)}
              aria-label="Close"
            >
              <X />
            </button>
            <div className="workspace-dialog-icon">
              <Bookmark className="text-[#24b47e] dark:text-[#3ecf8e] size-6" />
            </div>
            <h2 id="save-config-title">Save Workspace Configuration</h2>
            <p>Save this workspace&apos;s layers, camera position, and revision history to your library.</p>
            <form onSubmit={handleSaveConfigSubmit} className="mt-4 flex flex-col gap-4 text-left">
              <div>
                <label className="text-xs font-semibold text-neutral-700 dark:text-neutral-300 block mb-1">
                  Workspace Name
                </label>
                <input
                  type="text"
                  value={saveConfigName}
                  onChange={(e) => setSaveConfigName(e.target.value)}
                  className="w-full px-3 py-2 text-sm border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] text-neutral-900 dark:text-white rounded-[6px] focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
                  autoFocus
                />
              </div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400 bg-neutral-50 dark:bg-[#171717] p-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e]">
                <strong>Active layers:</strong> {(effectiveManifest.members?.length || 0) + gisLayers.length} layer(s)
              </div>
              <div className="workspace-dialog-actions mt-2">
                <button
                  type="button"
                  className="workspace-secondary-button"
                  onClick={() => setSaveConfigModalOpen(false)}
                  disabled={savingConfig}
                >
                  Cancel
                </button>
                <button type="submit" className="workspace-primary-button" disabled={savingConfig}>
                  {savingConfig ? <Loader2 className="animate-spin size-4" /> : <Bookmark className="size-4" />}
                  <span>{savingConfig ? "Saving…" : "Save Workspace"}</span>
                </button>
              </div>
            </form>
          </section>
        </div>
      )}

      <ExportProjectModal
        open={exportModalOpen}
        onClose={() => setExportModalOpen(false)}
        manifest={effectiveManifest}
        buildings={exportModalBuildings}
        datasets={exportModalDatasets}
        gisLayers={gisLayers}
        primaryDatasetId={resolvedExportDatasetId}
        workspaceId={gisMapId}
        layerOrder={layerOrder}
        onToggleLayerOrder={handleToggleLayerOrder}
        activeBaseLayer={activeBaseLayer}
        onExport={handlePerformExport}
        exporting={exporting}
        error={exportError}
      />

      <EnhancementModal
        open={Boolean(enhancementTarget)}
        datasetId={enhancementTarget?.datasetId}
        datasetName={enhancementTarget?.name}
        onClose={() => setEnhancementTarget(null)}
        onUpdated={() => {
          workspace.refreshManifest().catch(() => undefined)
          setOverlayRefreshKey((k) => k + 1)
        }}
        onOpenAnnotations={(dsId, dsName) => setAnnotationTarget({ datasetId: dsId, name: dsName })}
      />

      <AnnotationReviewPanel
        open={Boolean(annotationTarget)}
        datasetId={annotationTarget?.datasetId}
        datasetName={annotationTarget?.name}
        onClose={() => setAnnotationTarget(null)}
        onUpdated={() => {
          workspace.refreshManifest().catch(() => undefined)
          setOverlayRefreshKey((k) => k + 1)
        }}
      />

      <ReopenProjectModal
        open={reopenModalOpen}
        onClose={() => setReopenModalOpen(false)}
        onReopened={(restored) => {
          setReopenModalOpen(false)
          if (restored?.id) navigate(`/datasets/${restored.id}/workspace`)
        }}
      />

      <ShareMapModal
        isOpen={shareModalOpen}
        onClose={() => setShareModalOpen(false)}
        map={currentMap}
        layerOrder={layerOrder}
      />

      <BaseLayerSelectionModal
        open={baseLayerModalOpen}
        initialMode={baseLayerModalMode}
        workspaceId={gisMapId}
        workspaceName={workspaceDisplayName}
        onCancel={() => setBaseLayerModalOpen(false)}
        onConfirmed={({ baseLayer, baseLayerType }) => {
          setBaseLayerModalOpen(false)
          setActiveBaseLayer(baseLayer)
          setMapViewPrefs((prev) => ({
            ...prev,
            view_type: baseLayerType === "custom" ? "custom" : "world",
            base_layer_visible: true,
          }))
          if (baseLayerType === "custom") setBaseLayerFitTrigger((n) => n + 1)
          listBaseLayers()
            .then((res) => {
              if (res?.base_layers) setCustomBaseLayers(res.base_layers)
            })
            .catch(() => {})
          notifyGIS(
            baseLayerType === "custom"
              ? `Base layer set to "${baseLayer?.name}"`
              : "Base layer set to Default World Map"
          )
        }}
      />
    </div>
  )
}
