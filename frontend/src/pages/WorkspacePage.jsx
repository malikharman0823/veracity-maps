import React, { useEffect, useRef, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import {
  AlertCircle,
  ArrowLeft,
  Check,
  Database,
  Download,
  Layers,
  Loader2,
  Map as MapIcon,
  Menu,
  Plus,
  RefreshCw,
  Upload,
  FolderOpen,
  Bookmark,
  BookmarkCheck,
  X,
  Share2,
  Copy,
  LogOut,
} from "lucide-react"

import {
  importDatasetZip,
  downloadProjectExport,
  uploadDataset,
    listIcons,
  inspectArchive,
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
  listUserFiles,
  ensureActiveSession,
} from "../api"
import WorkspaceTable from "../components/WorkspaceTable"
import WorkspaceLayerPanel from "../components/WorkspaceLayerPanel"
import { SidebarTrigger } from "@/components/ui/sidebar"
import WorkspaceMap from "../components/WorkspaceMap"
import ErrorBoundary from "../components/ErrorBoundary"
import useWorkspace from "../hooks/useWorkspace"
import ThemeToggle from "../components/ThemeToggle"
import ZipImportReview from "../components/ZipImportReview"
import ExportProjectModal from "../components/ExportProjectModal"
import ImportPdfModal from "../components/ImportPdfModal"
import EnhancementModal from "../components/EnhancementModal"
import AnnotationReviewPanel from "../components/AnnotationReviewPanel"
import ReopenProjectModal from "../components/ReopenProjectModal"
import ShareMapModal from "../components/ShareMapModal"
import { useAuth } from "../context/AuthContext"
import { applyFloorSelection } from "../lib/floorSelection"
import { lockHeldByAnotherUser, lockHolderNameFromError, lockHolderNameFromLock } from "../lib/mapLock"
import { loadMapWorkspace, mapConfigurationMemberIds } from "../lib/sessionLoading"
import { persistMapConfiguration } from "../lib/mapConfigPersistence"

function AddDatasetDialog({ open, onClose, onUploaded, existingMembers = [], projectId, mapId }) {
  const inputRef = useRef(null)
  const [state, setState] = useState("confirm")
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState(null)
  const [archiveInspection, setArchiveInspection] = useState(null)
  const [pdfInspection, setPdfInspection] = useState(null)
  const [userFiles, setUserFiles] = useState([])
  const [loadingUserFiles, setLoadingUserFiles] = useState(false)

  useEffect(() => {
    if (open) {
      setState("confirm")
      setProgress(0)
      setError(null)
      setArchiveInspection(null)
      setPdfInspection(null)
      setLoadingUserFiles(true)
      listUserFiles()
        .then((res) => {
          const files = Array.isArray(res) ? res : res?.files || []
          setUserFiles(files)
        })
        .catch(() => setUserFiles([]))
        .finally(() => setLoadingUserFiles(false))
    }
  }, [open])

  if (!open) return null

  async function selectFile(event) {
    const files = Array.from(event.target.files || [])
    event.target.value = ""
    if (!files.length) return
    const isSupported = (name) => /\.(png|jpe?g|tif|tiff|webp|bmp|pdf|dxf|dwg|svf|mbtiles|zip)$/i.test(name || "")
    const invalid = files.find((f) => !isSupported(f.name))
    if (invalid) {
      setError({
        message: "Unsupported file format.",
        details: `File "${invalid.name}" is not supported. Supported formats are PNG, JPG, TIFF, WebP, PDF, CAD, SVF, MBTiles, and ZIP.`,
      })
      return
    }

    // If single zip file, try staged inspection first
    if (files.length === 1 && (files[0].name || "").toLowerCase().endsWith(".zip")) {
      setState("uploading")
      setError(null)
      try {
        const inspection = await inspectArchive(files[0], {
          onUploadProgress: (ev) => {
            if (ev.total) setProgress(Math.round((ev.loaded / ev.total) * 100))
          },
        })
        if (inspection && inspection.datasets?.length > 0) {
          setArchiveInspection(inspection)
          setState("confirm")
          return
        }
      } catch (inspectErr) {
        console.warn("inspectArchive fallback to legacy zip:", inspectErr)
      }
    }

    setState("uploading")
    setError(null)
    try {
      for (let i = 0; i < files.length; i++) {
        const file = files[i]
        const isZip = (file.name || "").toLowerCase().endsWith(".zip")
        const uploadFn = isZip ? importDatasetZip : uploadDataset
        const dataset = await uploadFn(file, (uploadEvent) => {
          if (uploadEvent.total) {
            const currentPct = (uploadEvent.loaded / uploadEvent.total) * 100
            const totalPct = Math.round((i * 100 + currentPct) / files.length)
            setProgress(totalPct)
          }
        }, projectId, mapId)

        // Check if PDF pending inspection
        if (dataset && (dataset.type === "pdf_pending" || dataset.status === "pdf_pending")) {
          setPdfInspection(dataset)
          setState("confirm")
          return
        }

        const isAlreadyMember = existingMembers.some(
          (m) => m.dataset_id === dataset.id
        )
        if (isAlreadyMember) {
          setError({
            message: "Dataset is already attached.",
            details: `"${file.name}" is already a layer in this workspace. To add a 2nd raster layer, please select a different image file.`,
          })
          setState("confirm")
          return
        }
        await onUploaded(dataset)
      }
    } catch (requestError) {
      setError(readableApiError(requestError, "Unable to add the dataset."))
      setState("confirm")
    }
  }

  return (
    <div className="workspace-dialog-backdrop" role="presentation">
      <section className="workspace-dialog max-w-lg" role="dialog" aria-modal="true" aria-labelledby="add-dataset-title">
        <button type="button" className="workspace-dialog-close" onClick={onClose} aria-label="Close"><X /></button>
        <div className="workspace-dialog-icon"><Layers /></div>
        <h2 id="add-dataset-title">
          {existingMembers.length > 0 ? "Add layer to map" : "Attach a layer to map"}
        </h2>
        <p>
          {existingMembers.length > 0
            ? "Upload a new floor plan, raster, or vector file, or select a previously uploaded file from your personal storage to attach as another layer."
            : "Upload a floor plan, GeoTIFF, or CAD file, or select one from your personal storage to begin editing this map."}
        </p>
        {error && <div className="workspace-dialog-error"><AlertCircle /><div><strong>{error.message}</strong><span>{error.details}</span></div></div>}
        {state === "uploading" ? (
          <div className="workspace-dialog-progress">
            <Loader2 className="animate-spin" />
            <span>Uploading and preparing dataset…</span>
            <strong>{progress}%</strong>
          </div>
        ) : (
          <div className="workspace-dialog-actions">
            <button type="button" className="workspace-secondary-button" onClick={onClose}>Cancel</button>
            <button type="button" className="workspace-primary-button" onClick={() => inputRef.current?.click()}>
              <Upload /> Select New File(s)
            </button>
          </div>
        )}
        <input
          ref={inputRef}
          type="file"
          multiple
          accept=".png,.jpg,.jpeg,.tif,.tiff,.webp,.bmp,.pdf,.dxf,.dwg,.svf,.mbtiles,.zip,image/png,image/jpeg,image/tiff,application/pdf,application/zip"
          className="sr-only"
          onChange={selectFile}
        />

        {userFiles.length > 0 && state !== "uploading" && (
          <div className="mt-4 pt-4 border-t border-neutral-200 dark:border-[#2e2e2e] text-left">
            <div className="text-xs font-semibold text-neutral-600 dark:text-neutral-400 mb-2 flex items-center justify-between">
              <span>Or choose from your personal storage ({userFiles.length})</span>
              {loadingUserFiles && <Loader2 className="animate-spin size-3" />}
            </div>
            <div className="max-h-48 overflow-y-auto flex flex-col gap-1.5 pr-1">
              {userFiles.map((f) => {
                const isAlreadyMember = existingMembers.some((m) => m.dataset_id === f.id)
                return (
                  <div
                    key={f.id}
                    className="flex items-center justify-between p-2 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#1a1a1a] hover:bg-neutral-100 dark:hover:bg-[#222222] transition-colors"
                  >
                    <div className="min-w-0 pr-2">
                      <div className="text-xs font-medium text-neutral-900 dark:text-white truncate">
                        {f.original_filename}
                      </div>
                      <div className="text-[10px] text-neutral-500 dark:text-neutral-400">
                        {(f.file_size / (1024 * 1024)).toFixed(2)} MB • {f.file_type || "File"}
                      </div>
                    </div>
                    {isAlreadyMember ? (
                      <span className="text-[10px] px-2 py-0.5 rounded-[4px] bg-neutral-200 dark:bg-neutral-800 text-neutral-500 font-medium">
                        Added
                      </span>
                    ) : (
                      <button
                        type="button"
                        onClick={() => onUploaded({ id: f.id, name: f.original_filename, is_georeferenced: true })}
                        className="px-2.5 py-1 text-xs font-semibold rounded-[4px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] transition-colors cursor-pointer"
                      >
                        Attach
                      </button>
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </section>

      {archiveInspection && (
        <ZipImportReview
          inspectionResult={archiveInspection}
          onClose={() => setArchiveInspection(null)}
          onImportComplete={async (importRes) => {
            setArchiveInspection(null)
            onClose()
            for (const r of importRes.results || []) {
              if ((r.status === "imported" || r.status === "success") && r.dataset_id) {
                await onUploaded({ id: r.dataset_id, name: r.name, is_georeferenced: true })
              }
            }
          }}
        />
      )}

      {pdfInspection && (
        <ImportPdfModal
          pdfInspection={pdfInspection}
          onClose={() => setPdfInspection(null)}
          onImportSuccess={async (importRes) => {
            setPdfInspection(null)
            onClose()
            for (const ds of importRes.datasets || []) {
              await onUploaded(ds)
            }
          }}
        />
      )}
    </div>
  )
}

export default function WorkspacePage() {
  const { id: paramDatasetId, mapspaceId, projectId: paramProjectId, mapId } = useParams()
  const { user } = useAuth()
  const activeContainerId = mapId || mapspaceId || paramProjectId || ""
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedView = searchParams.get("view")
  const [layersOpen, setLayersOpen] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
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

  // Mapspace & Workspace Map state
  const [currentMap, setCurrentMap] = useState(null)
  const [mapLoading, setMapLoading] = useState(Boolean(mapId))
  const [mapNotFound, setMapNotFound] = useState(false)
  const [userLibraryFiles, setUserLibraryFiles] = useState([])
  const [activeDatasetId, setActiveDatasetId] = useState(paramDatasetId || null)
  const [workspaceToast, setWorkspaceToast] = useState("")
  const [shareModalOpen, setShareModalOpen] = useState(false)

  // Lock State
  const [lockHeld, setLockHeld] = useState(false)
  const [isLockedByOther, setIsLockedByOther] = useState(false)
  const [lockHolderName, setLockHolderName] = useState("")

  // Single-editor lock lifecycle (only after map is verified to exist)
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
          releaseMapLock(mapId).catch(() => { })
          return
        }

        setLockHeld(true)
        setIsLockedByOther(false)
        setLockHolderName("")
        renewInterval = setInterval(() => {
          renewMapLock(mapId).catch((err) => {
            console.warn("Failed renewing map lock:", err)
            const isConflict = err.response?.status === 409 || err.response?.data?.error?.code === "LOCK_HELD" || err.response?.data?.error?.code === "LOCK_LOST"
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
        const isConflict = err.response?.status === 409 || err.response?.data?.error?.code === "LOCK_HELD"
        if (isConflict) {
          setLockHeld(false)
          setIsLockedByOther(true)
          setLockHolderName(lockHolderNameFromError(err))
        }
      }
    }

    initializeLock()

    const handleBeforeUnload = () => {
      if (acquired) releaseMapLock(mapId).catch(() => { })
    }
    window.addEventListener("beforeunload", handleBeforeUnload)

    return () => {
      isMounted = false
      if (renewInterval) clearInterval(renewInterval)
      window.removeEventListener("beforeunload", handleBeforeUnload)
      if (acquired) releaseMapLock(mapId).catch(() => { })
    }
  }, [mapId, user?.id, currentMap?.id])

  useEffect(() => {
    if (mapId) {
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
                    if (mId && !configuredMemberIds.includes(mId)) {
                      configuredMemberIds.push(mId)
                    }
                  }
                }
                if (!savedPrimaryId && configuredMemberIds.length > 0) {
                  savedPrimaryId = configuredMemberIds[0]
                }
              } catch {}

              if (Array.isArray(m.assets)) {
                for (const asset of m.assets) {
                  if (asset?.id && !configuredMemberIds.includes(asset.id)) {
                    configuredMemberIds.push(asset.id)
                  }
                }
              }
              pendingMapMembersRef.current = configuredMemberIds

              if (savedPrimaryId) {
                setActiveDatasetId(savedPrimaryId)
              } else if (m.assets?.length > 0) {
                setActiveDatasetId(m.assets[0].id)
              } else {
                // New or empty map: do not auto-attach random files from other maps or sessions
                setActiveDatasetId(null)
                try {
                  const res = await listUserFiles()
                  const files = Array.isArray(res) ? res : res?.files || []
                  setUserLibraryFiles(files)
                } catch (e) {
                  console.warn("Failed checking user library files:", e)
                }
              }
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
        .finally(() => {
          setMapLoading(false)
        })
    }
  }, [mapId, paramDatasetId, searchParams, navigate])

  const effectiveId = activeDatasetId || paramDatasetId
  const workspace = useWorkspace(effectiveId)
  const id = effectiveId
  const view = requestedView === "database" ? "database" : "map"

  useEffect(() => {
    initializedFloorRef.current = false
  }, [id])

  useEffect(() => {
    if (!initializedFloorRef.current && workspace.manifest?.members?.length > 0) {
      const primary = workspace.manifest.members.find((m) => m.dataset_id === id)
      const targetLevel = (primary && typeof primary.floor_level === "number")
        ? primary.floor_level
        : (workspace.manifest.members.find((m) => m.floor_level !== undefined)?.floor_level ?? 0)
      const targetBuilding = primary?.building_name || workspace.manifest.members.find((m) => m.building_name)?.building_name || ""
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
    } catch {
      // Non-blocking fallback
    }
  }

  async function handlePerformExport(payload) {
    if (exporting || workspace.saveState === "saving") return
    setExporting(true)
    setExportError(null)
    try {
      await downloadProjectExport(id, workspace.manifest?.name, payload)
      setExportModalOpen(false)
    } catch (requestError) {
      const err = readableApiError(
        requestError,
        "Unable to export the Map Studio project package.",
      )
      setExportError(err)
      workspace.setError(err)
    } finally {
      setExporting(false)
    }
  }

  async function handleExportMap() {
    if (exporting || workspace.saveState === "saving") return
    setExporting(true)
    workspace.setError(null)
    try {
      await downloadProjectExport(id, workspace.manifest?.name)
    } catch (requestError) {
      workspace.setError(readableApiError(
        requestError,
        "Unable to export the Map Studio package.",
      ))
    } finally {
      setExporting(false)
    }
  }

  const [savingConfig, setSavingConfig] = useState(false)
  const [configSavedRecently, setConfigSavedRecently] = useState(false)
  const [saveConfigModalOpen, setSaveConfigModalOpen] = useState(false)
  const [saveConfigName, setSaveConfigName] = useState("")

  function openSaveConfigDialog() {
    const defaultName = workspace.manifest?.name || "Workspace Configuration"
    setSaveConfigName(defaultName)
    setSaveConfigModalOpen(true)
  }

  async function handleSaveConfigSubmit(e) {
    if (e) e.preventDefault()
    if (savingConfig) return
    const name = (saveConfigName || "").trim() || workspace.manifest?.name || "Saved Configuration"
    setSavingConfig(true)
    try {
      const members = mapConfigurationMemberIds(workspace.manifest?.members)
      const payload = {
        name,
        primary_dataset_id: id,
        members,
        configuration: {
          active_view: view,
          camera: workspace.manifest?.camera || null,
        },
      }
	  const persisted = await persistMapConfiguration({
		mapId,
		payload,
		name,
		expectedRevision: currentMap?.revision_number || 0,
		activeView: view,
	  }, {
		saveMapConfig,
		createMapFromConfiguration,
	  })
	  if (persisted?.map) setCurrentMap(persisted.map)
      setSaveConfigModalOpen(false)
      setConfigSavedRecently(true)
	  if (persisted.navigateTo) {
		navigate(persisted.navigateTo, { replace: true })
	  }
      setTimeout(() => setConfigSavedRecently(false), 3000)
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

  useEffect(() => {
    if (!requestedView && workspace.manifest?.active_view) {
      setSearchParams({ view: workspace.manifest.active_view }, { replace: true })
    }
  }, [requestedView, setSearchParams, workspace.manifest?.active_view])

  useEffect(() => {
    const warn = (event) => {
      if (!dirty) return
      event.preventDefault()
      event.returnValue = ""
    }
    window.addEventListener("beforeunload", warn)
    return () => window.removeEventListener("beforeunload", warn)
  }, [dirty])

  function confirmDiscard() {
    return !dirty || window.confirm("Discard the unsaved record changes?")
  }

  function setView(nextView) {
    if (!confirmDiscard()) return
    setDirty(false)
    setSearchParams({ view: nextView }, { replace: true })
    workspace.updateState({ active_view: nextView }).catch(() => undefined)
  }

  function selectLayer(datasetId, kind) {
    workspace.setFilters({ datasetId, kind })
    setLayersOpen(false)
    if (kind && kind !== "raster" && view !== "database") {
      setView("database")
    }
  }

  const syncedReturnDatasetRef = useRef(null)

  useEffect(() => {
    const returnDatasetId = searchParams.get("datasetId") || searchParams.get("dataset")
    if (!mapId || !id || !returnDatasetId || !workspace.manifest?.members) return
    const syncKey = `${mapId}:${returnDatasetId}:${workspace.manifest.members.length}`
    if (syncedReturnDatasetRef.current === syncKey) return
    syncedReturnDatasetRef.current = syncKey
    const memberIds = workspace.manifest.members.map((m) => m.dataset_id).filter(Boolean)
    autoSaveActiveMapConfig(id, memberIds, view)
  }, [mapId, id, searchParams, workspace.manifest?.members, view])

  async function autoSaveActiveMapConfig(primaryId, memberIds, activeViewName = "map") {
    if (!mapId || !primaryId) return
    const safeMemberIds = Array.isArray(memberIds) ? memberIds : []
    const mapName = currentMap?.name || workspace.manifest?.name || "Map Configuration"
    const payload = {
      name: mapName,
      primary_dataset_id: primaryId,
      members: Array.from(new Set([primaryId, ...safeMemberIds].filter(Boolean))),
      configuration: {
        active_view: activeViewName,
        camera: workspace.manifest?.camera || null,
      },
    }
    try {
      await saveMapConfig(mapId, payload, mapName, 0)
      const refreshed = await getMap(mapId).catch(() => null)
      if (refreshed?.map || refreshed) {
        setCurrentMap(refreshed?.map || refreshed)
      }
    } catch (err) {
      console.warn("Auto-persisting map configuration failed:", err)
    }
  }

  async function handleAddedDataset(dataset) {
    const extraIds = Array.isArray(dataset.imported_datasets)
      ? dataset.imported_datasets.map((d) => d.id).filter(Boolean)
      : []

    if (!id || id === "undefined" || id === "null") {
      setActiveDatasetId(dataset.id)
      setAddOpen(false)
      if (extraIds.length > 0) {
        pendingMapMembersRef.current = Array.from(new Set([...pendingMapMembersRef.current, ...extraIds]))
      }
      await autoSaveActiveMapConfig(dataset.id, [dataset.id, ...extraIds], "map")
      if (!dataset.is_georeferenced) {
        const returnTo = mapId
          ? `/maps/${mapId}/edit?datasetId=${dataset.id}&view=map`
          : `/datasets/${dataset.id}/workspace?view=map`
        navigate(`/datasets/${dataset.id}/georeference?returnTo=${encodeURIComponent(returnTo)}`)
        return
      }
      setView("map")
      return
    }

    const currentMemberIds = new Set((workspace.manifest?.members || []).map((m) => m.dataset_id))
    if (!currentMemberIds.has(dataset.id)) {
      await workspace.attachDataset(dataset.id).catch(() => undefined)
      currentMemberIds.add(dataset.id)
    }
    for (const extraId of extraIds) {
      if (extraId !== dataset.id && !currentMemberIds.has(extraId)) {
        await workspace.attachDataset(extraId).catch(() => undefined)
        currentMemberIds.add(extraId)
      }
    }
    await workspace.refreshManifest().catch(() => undefined)
    await autoSaveActiveMapConfig(id, Array.from(currentMemberIds), "map")
    setAddOpen(false)
    if (!dataset.is_georeferenced) {
      const returnTo = mapId
        ? `/maps/${mapId}/edit?datasetId=${id}&view=map`
        : `/datasets/${id}/workspace?view=map`
      navigate(`/datasets/${dataset.id}/georeference?returnTo=${encodeURIComponent(returnTo)}`)
      return
    }
    setView("map")
  }

  function handleDetach(member) {
    if (!window.confirm(`Detach “${member.name}” from this workspace? The dataset and its files will remain in the library.`)) return
    workspace.detachDataset(member.dataset_id)
      .then(() => {
        if (mapId && id) {
          const remainingIds = (workspace.manifest?.members || [])
            .map((m) => m.dataset_id)
            .filter((mId) => mId && mId !== member.dataset_id)
          autoSaveActiveMapConfig(id, remainingIds, view)
        }
      })
      .catch(() => undefined)
  }

  function selectMapRecord(recordId) {
    workspace.setSelectedRecordId(recordId)
    if (recordId) {
      const record = workspace.mapRecords.find((item) => item.id === recordId)
      if (record) workspace.setFilters({ datasetId: record.dataset_id, kind: record.kind })
      setView("database")
    }
  }

  function handleSwitchToMap(recordId) {
    workspace.setSelectedRecordId(recordId)
    setView("map")
  }

  if (mapNotFound) {
    return (
      <div className="dataset-workspace-shell flex flex-col items-center justify-center p-6 text-center bg-neutral-50 dark:bg-[#171717]">
        <div className="max-w-md w-full p-8 rounded-2xl bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] shadow-xl">
          <div className="size-14 mx-auto mb-4 rounded-2xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center text-amber-600 dark:text-amber-400">
            <AlertCircle className="size-7" />
          </div>
          <h2 className="text-lg font-bold text-neutral-900 dark:text-white">Map Not Found</h2>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-2 mb-6">
            This map does not exist, has been deleted, or is no longer accessible. Please select an active map from your personal dashboard.
          </p>
          <Link
            to="/dashboard"
            className="inline-flex items-center justify-center gap-2 px-5 py-2.5 rounded-lg bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] font-semibold text-xs transition-colors shadow-sm"
          >
            <ArrowLeft className="size-4" /> Return to Dashboard
          </Link>
        </div>
      </div>
    )
  }

  if (workspace.loading || mapLoading) {
    return <div className="workspace-loading"><Loader2 className="animate-spin" /> Loading workspace…</div>
  }

  if (!workspace.manifest) {
    if (mapId || (mapspaceId && mapId)) {
      return (
        <div className="dataset-workspace-shell">
          <header className="workspace-topbar">
            <div className="workspace-topbar-left">
              <SidebarTrigger className="text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px] transition-colors" />
              <Link to="/dashboard" aria-label="Back">
                <ArrowLeft />
              </Link>
              <div>
                <span className="workspace-eyebrow">
                  {currentMap ? `Workspace / ${currentMap.name}` : "Workspace Map"}
                </span>
                <div className="flex items-center gap-2">
                  <h1 className="text-sm font-bold text-neutral-900 dark:text-white">
                    {currentMap?.name || "Untitled Map"}
                  </h1>
                  <span className="text-[10px] px-1.5 py-0.2 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold uppercase">
                    Rev #{currentMap?.revision_number || currentMap?.revision_count || 1}
                  </span>
                  {lockHeld && (
                    <span
                      className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 text-[10px] font-semibold tracking-wide"
                      title="Exclusive Editing Lock Active • Auto-renewing heartbeat"
                    >
                      <span className="w-1.5 h-1.5 rounded-full bg-[#3ecf8e] animate-pulse"></span>
                      <span>Locked (You)</span>
                    </span>
                  )}
                  {isLockedByOther && (
                    <span
                      className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/30 text-[10px] font-semibold"
                      title={`Locked for editing by ${lockHolderName}. Viewing in read-only mode.`}
                    >
                      <span>🔒 Read-Only ({lockHolderName})</span>
                    </span>
                  )}
                </div>
              </div>
            </div>
            <div className="workspace-topbar-actions">
              <button
                type="button"
                className="workspace-primary-button"
                onClick={() => setAddOpen(true)}
              >
                <Plus /> Add layer
              </button>
            </div>
          </header>

          <main className="workspace-main flex items-center justify-center p-8 bg-neutral-950/20 min-h-[calc(100vh-60px)]">
            <div className="max-w-lg w-full p-8 rounded-2xl bg-white dark:bg-[#1f1f1f] border border-neutral-200 dark:border-[#2e2e2e] text-center shadow-lg">
              <div className="size-16 mx-auto mb-4 rounded-2xl bg-[#3ecf8e]/10 border border-[#3ecf8e]/20 flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e]">
                <Layers className="size-8" />
              </div>
              <h2 className="text-lg font-bold text-neutral-900 dark:text-white">Empty Map Canvas</h2>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-2 mb-6">
                This map has no layers active in this session yet. Upload your floor plans, GeoTIFFs, or CAD files to begin editing.
              </p>

              {userLibraryFiles.length > 0 && (
                <div className="mb-6 text-left">
                  <div className="text-xs font-semibold text-neutral-700 dark:text-neutral-300 mb-2">
                    Available files from your storage ({userLibraryFiles.length}):
                  </div>
                  <div className="max-h-44 overflow-y-auto flex flex-col gap-2 p-1.5 rounded-lg border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717]">
                    {userLibraryFiles.map((file) => (
                      <div key={file.id} className="flex items-center justify-between p-2 rounded bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2a2a2a] shadow-xs">
                        <div className="min-w-0 pr-2">
                          <span className="text-xs font-semibold text-neutral-900 dark:text-white block truncate">
                            {file.original_filename}
                          </span>
                          <span className="text-[10px] text-neutral-500 dark:text-neutral-400">
                            {(file.file_size / (1024 * 1024)).toFixed(2)} MB • {file.file_type || "Layer"}
                          </span>
                        </div>
                        <button
                          type="button"
                          onClick={() => handleAddedDataset({ id: file.id, name: file.original_filename, is_georeferenced: true })}
                          className="px-3 py-1.5 rounded-[4px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] text-xs font-semibold cursor-pointer shrink-0 transition-colors"
                        >
                          Attach to Map
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <div className="flex items-center justify-center gap-3 flex-wrap">
                <button
                  type="button"
                  onClick={() => setAddOpen(true)}
                  className="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] font-semibold text-xs transition-colors shadow-sm"
                >
                  <Plus className="size-4" /> Upload or Select Layer
                </button>
              </div>
            </div>
          </main>

          <AddDatasetDialog
            open={addOpen}
            onClose={() => setAddOpen(false)}
            onUploaded={handleAddedDataset}
            existingMembers={[]}
            projectId={activeContainerId}
            mapId={mapId}
          />
        </div>
      )
    }

    return (
      <div className="workspace-loading workspace-load-error">
        <AlertCircle className="size-8 text-amber-500 mb-2" />
        <strong>{workspace.error?.message || "Workspace not found in current session"}</strong>
        <span className="max-w-md text-center text-slate-500 mb-4">
          {workspace.error?.details || "The requested dataset belongs to a different or previous session that was cleared. Return to the dashboard to view available maps or upload new floor plans."}
        </span>
        <Link to="/dashboard" className="px-4 py-2 bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-semibold rounded-[6px] shadow-xs transition-colors">
          Return to dashboard
        </Link>
      </div>
    )
  }


  return (
    <div className="dataset-workspace-shell">
      <header className="workspace-topbar">
        <div className="workspace-topbar-left">
          <SidebarTrigger className="text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px] transition-colors" />
          <Link to="/dashboard" onClick={(event) => { if (!confirmDiscard()) event.preventDefault() }} aria-label="Back to dashboard"><ArrowLeft /></Link>
          <button type="button" className="workspace-mobile-layers" onClick={() => setLayersOpen(true)} aria-label="Open layers"><Menu /></button>
          <div>
            <span className="workspace-eyebrow">
              {currentMap ? `Workspace / ${currentMap.name}` : "Workspace / Map Studio"}
            </span>
            <div className="flex items-center gap-2">
              <h1 className="text-sm font-bold text-neutral-900 dark:text-white">
                {currentMap?.name || workspace.manifest.name}
              </h1>
              {currentMap && (
                <span className="text-[10px] px-1.5 py-0.2 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 font-semibold uppercase">
                  Rev #{currentMap.revision_count || currentMap.revision_number || 1}
                </span>
              )}
              {lockHeld && (
                <span
                  className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30 text-[10px] font-semibold tracking-wide"
                  title="Exclusive Editing Lock Active • Auto-renewing heartbeat"
                >
                  <span className="w-1.5 h-1.5 rounded-full bg-[#3ecf8e] animate-pulse"></span>
                  <span>Locked (You)</span>
                </span>
              )}
              {isLockedByOther && (
                <span
                  className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/30 text-[10px] font-semibold"
                  title={`Locked for editing by ${lockHolderName}. Viewing in read-only mode.`}
                >
                  <span>🔒 Read-Only ({lockHolderName})</span>
                </span>
              )}
            </div>
          </div>
        </div>
        <div className="workspace-topbar-actions">
          <div className={`workspace-save-state save-${workspace.saveState}`}>
            {workspace.saveState === "saving" ? <Loader2 className="animate-spin" /> : workspace.saveState === "failed" ? <AlertCircle /> : <Check />}
            <span>{workspace.saveState === "saving" ? "Saving" : workspace.saveState === "failed" ? "Save failed" : "Saved"}</span>
            {workspace.saveState === "failed" && <button type="button" onClick={workspace.retrySave}><RefreshCw /> Retry</button>}
          </div>
          <button
            type="button"
            className={`workspace-secondary-button ${configSavedRecently ? "border-green-500 text-green-600 bg-green-50" : ""}`}
            onClick={openSaveConfigDialog}
            disabled={savingConfig || isLockedByOther}
            title={isLockedByOther ? `Locked for editing by ${lockHolderName}` : "Save this map configuration to the dashboard"}
          >
            {savingConfig ? (
              <Loader2 className="animate-spin" />
            ) : configSavedRecently ? (
              <BookmarkCheck className="text-green-600" />
            ) : (
              <Bookmark />
            )}
            <span>{configSavedRecently ? "Saved to Dashboard" : "Save Config"}</span>
          </button>
          <button type="button" className="workspace-primary-button" onClick={() => setAddOpen(true)}><Plus /> Add dataset</button>
          <button
            type="button"
            className="workspace-secondary-button"
            onClick={openExportModal}
            disabled={exporting || workspace.saveState === "saving"}
            title="Export this Map Studio project as a ZIP package"
          >
            {exporting ? <Loader2 className="animate-spin" /> : <Download />}
            <span>{exporting ? "Exporting…" : "Export"}</span>
          </button>
          <button
            type="button"
            className="workspace-secondary-button"
            onClick={() => {
              if (!confirmDiscard()) return
              setReopenModalOpen(true)
            }}
            title="Reopen a standalone .mbtiles database or Veracity Maps .zip project package"
          >
            <FolderOpen />
            <span>Reopen (.mbtiles / .zip)</span>
          </button>
          {currentMap?.id && (
            <button
              type="button"
              className="workspace-secondary-button"
              onClick={() => setShareModalOpen(true)}
              title="Invite collaborators via email or copy share link"
            >
              <Share2 className="w-4 h-4 text-[#3ecf8e]" />
              <span>Share</span>
            </button>
          )}
          <ThemeToggle />
          <Link
            to="/dashboard"
            className="h-8 px-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-700 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-50 dark:hover:bg-[#2a2a2a] text-xs font-semibold flex items-center gap-1.5 transition-colors cursor-pointer"
            title="Exit Map Studio and return to personal dashboard"
          >
            <LogOut className="w-3.5 h-3.5" />
            <span>Exit Studio</span>
          </Link>
        </div>
      </header>

      {workspaceToast && (
        <div className="fixed top-14 right-6 z-50 flex items-center gap-2 px-4 py-2 rounded-lg bg-neutral-900 text-white dark:bg-white dark:text-neutral-900 text-xs font-semibold shadow-xl border border-neutral-700 dark:border-neutral-200 animate-in fade-in slide-in-from-top-2">
          <Check className="w-4 h-4 text-[#3ecf8e]" />
          <span>{workspaceToast}</span>
        </div>
      )}

      {workspace.error && (
        <div className="workspace-error-banner">
          <AlertCircle />
          <span><strong>{workspace.error.message}</strong> {workspace.error.details}</span>
          <button type="button" onClick={() => workspace.setError(null)} aria-label="Dismiss error"><X /></button>
        </div>
      )}

      <div className="workspace-body bg-white dark:bg-[#1c1c1c]">
        <div className="workspace-desktop-layers bg-white dark:bg-[#202020] border-r border-neutral-200 dark:border-[#2e2e2e]">
          <WorkspaceLayerPanel
            members={workspace.manifest.members}
            records={workspace.mapRecords}
            selectedDatasetId={workspace.filters.datasetId}
            selectedKind={workspace.filters.kind}
            onSelect={selectLayer}
            onUpdate={(datasetId, patch) => workspace.updateMember(datasetId, patch).catch(() => undefined)}
            onReorder={(ids) => workspace.reorderMembers(ids).catch(() => undefined)}
            onDetach={handleDetach}
            onLayerChanged={() => workspace.refreshManifest().catch(() => undefined)}
            onOpenTransform={(member) => {
              setTransformingMember(member)
              if (view !== "map") setView("map")
            }}
            onOpenEnhancement={(member) =>
              setEnhancementTarget({
                datasetId: member.dataset_id,
                name: member.name || member.dataset?.filename,
              })
            }
            onOpenAnnotations={(member) =>
              setAnnotationTarget({
                datasetId: member.dataset_id,
                name: member.name || member.dataset?.filename,
              })
            }
            activeFloorLevel={activeFloorLevel}
            activeBuildingName={activeBuildingName}
            onSelectFloor={handleSelectFloor}
            buildings={workspace.manifest.buildings}
            isolateActiveFloor={isolateActiveFloor}
            onToggleIsolateActiveFloor={setIsolateActiveFloor}
          />
        </div>
        {layersOpen && (
          <div className="workspace-mobile-layer-sheet">
            <div className="workspace-sheet-backdrop" onClick={() => setLayersOpen(false)} />
            <WorkspaceLayerPanel
              members={workspace.manifest.members}
              records={workspace.mapRecords}
              selectedDatasetId={workspace.filters.datasetId}
              selectedKind={workspace.filters.kind}
              onSelect={selectLayer}
              onUpdate={(datasetId, patch) => workspace.updateMember(datasetId, patch).catch(() => undefined)}
              onReorder={(ids) => workspace.reorderMembers(ids).catch(() => undefined)}
              onDetach={handleDetach}
              onClose={() => setLayersOpen(false)}
              onLayerChanged={() => workspace.refreshManifest().catch(() => undefined)}
              onOpenTransform={(member) => {
                setTransformingMember(member)
                setLayersOpen(false)
                if (view !== "map") setView("map")
              }}
              onOpenEnhancement={(member) => {
                setLayersOpen(false)
                setEnhancementTarget({
                  datasetId: member.dataset_id,
                  name: member.name || member.dataset?.filename,
                })
              }}
              onOpenAnnotations={(member) => {
                setLayersOpen(false)
                setAnnotationTarget({
                  datasetId: member.dataset_id,
                  name: member.name || member.dataset?.filename,
                })
              }}
              activeFloorLevel={activeFloorLevel}
              activeBuildingName={activeBuildingName}
              onSelectFloor={handleSelectFloor}
              buildings={workspace.manifest.buildings}
              isolateActiveFloor={isolateActiveFloor}
              onToggleIsolateActiveFloor={setIsolateActiveFloor}
            />
          </div>
        )}

        <main className="workspace-canvas">
          {view === "map" ? (
            <ErrorBoundary title="Map Canvas Error">
              <WorkspaceMap
                key={id}
                manifest={workspace.manifest}
                overlayRefreshKey={overlayRefreshKey}
                members={workspace.manifest.members}
                records={workspace.mapRecords}
                selectedRecordId={workspace.selectedRecordId}
                onSelectRecord={selectMapRecord}
                savedCamera={workspace.manifest.camera}
                onCameraChange={(camera) => workspace.updateCamera(camera)}
                transformMember={transformingMember}
                onEndTransform={() => setTransformingMember(null)}
                onTransformApplied={() => {
                  workspace.refreshManifest().catch(() => undefined)
                }}
                activeFloorLevel={activeFloorLevel}
                activeBuildingName={activeBuildingName}
                onActiveFloorChange={(lvl, bName) => handleSelectFloor(lvl, bName)}
                onRecordsRefresh={() => Promise.all([workspace.refreshRecords(), workspace.refreshMapRecords(), workspace.refreshManifest()])}
                primaryDatasetId={id}
                onMemberUpdate={(datasetId, patch) => workspace.updateMember(datasetId, patch).catch(() => undefined)}
                buildings={workspace.manifest.buildings}
                isolateActiveFloor={isolateActiveFloor}
                onToggleIsolateActiveFloor={setIsolateActiveFloor}
              />
            </ErrorBoundary>
          ) : (
            <WorkspaceTable
              recordsData={workspace.records}
              filters={workspace.filters}
              onFiltersChange={workspace.setFilters}
              loading={workspace.recordsLoading}
              members={workspace.manifest.members}
              icons={icons}
              selectedRecordId={workspace.selectedRecordId}
              onSelectRecord={workspace.setSelectedRecordId}
              onMemberUpdate={workspace.updateMember}
              onRecordsRefresh={() => Promise.all([workspace.refreshRecords(), workspace.refreshMapRecords(), workspace.refreshManifest()])}
              onDirtyChange={setDirty}
              workspaceId={id}
              onAddDataset={() => setAddOpen(true)}
              onSwitchToMap={handleSwitchToMap}
            />
          )}
        </main>
      </div>

      <nav className="workspace-bottom-dock" aria-label="Workspace views">
        <div className="workspace-bottom-dock-inner">
          <button
            type="button"
            className={`workspace-dock-item ${view === "map" ? "is-active" : ""}`}
            onClick={() => setView("map")}
            aria-label="Interactive Map view"
          >
            <MapIcon className="size-4" />
            <span>Map</span>
            {workspace.manifest.members.length > 0 && (
              <span className="workspace-dock-badge">
                {workspace.manifest.members.filter((m) => m.visible).length}
              </span>
            )}
          </button>
          <button
            type="button"
            className={`workspace-dock-item ${view === "database" ? "is-active" : ""}`}
            onClick={() => setView("database")}
            aria-label="Database Table view"
          >
            <Database className="size-4" />
            <span>Database</span>
            {workspace.records.total > 0 && (
              <span className="workspace-dock-badge">{workspace.records.total}</span>
            )}
          </button>
          <div className="workspace-dock-divider" />
          <button
            type="button"
            className="workspace-dock-item workspace-dock-action"
            onClick={() => setAddOpen(true)}
            aria-label="Add or merge another dataset"
            title="Add another dataset to workspace"
          >
            <Plus className="size-4" />
            <span>Add Dataset</span>
          </button>
        </div>
      </nav>

      <AddDatasetDialog
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onUploaded={handleAddedDataset}
        existingMembers={workspace.manifest.members}
        projectId={activeContainerId}
        mapId={mapId}
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
            <h2 id="save-config-title">Save Map Configuration</h2>
            <p>Save this configuration to your map library so its layers, camera position, and revision history remain available after this session ends or you log out.</p>
            <form onSubmit={handleSaveConfigSubmit} className="mt-4 flex flex-col gap-4 text-left">
              <div>
                <label className="text-xs font-semibold text-neutral-700 dark:text-neutral-300 block mb-1">Configuration Name</label>
                <input
                  type="text"
                  value={saveConfigName}
                  onChange={(e) => setSaveConfigName(e.target.value)}
                  placeholder="e.g. Master Floorplan Review"
                  className="w-full px-3 py-2 text-sm border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#171717] text-neutral-900 dark:text-white rounded-[6px] focus:outline-none focus:ring-1 focus:ring-[#3ecf8e]"
                  autoFocus
                />
              </div>
              <div className="text-xs text-neutral-500 dark:text-neutral-400 bg-neutral-50 dark:bg-[#171717] p-3 rounded-[6px] border border-neutral-200 dark:border-[#2e2e2e]">
                <strong>Included datasets:</strong> {workspace.manifest?.members?.length || 1} layer(s)
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
                <button
                  type="submit"
                  className="workspace-primary-button"
                  disabled={savingConfig}
                >
                  {savingConfig ? <Loader2 className="animate-spin size-4" /> : <Bookmark className="size-4" />}
                  <span>{savingConfig ? "Saving…" : "Save to Dashboard"}</span>
                </button>
              </div>
            </form>
          </section>
        </div>
      )}
      {/* Export Map Project Selective Modal */}
      <ExportProjectModal
        open={exportModalOpen}
        onClose={() => setExportModalOpen(false)}
        manifest={workspace.manifest}
        buildings={exportModalBuildings}
        datasets={exportModalDatasets}
        primaryDatasetId={id}
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
        onOpenAnnotations={(dsId, dsName) => {
          setAnnotationTarget({ datasetId: dsId, name: dsName })
        }}
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
          if (restored?.id) {
            navigate(`/datasets/${restored.id}/workspace?view=map`)
          }
        }}
      />

      <ShareMapModal
        isOpen={shareModalOpen}
        onClose={() => setShareModalOpen(false)}
        map={currentMap}
      />
    </div>
  )
}
