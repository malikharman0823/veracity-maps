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
} from "lucide-react"

import {
  importDatasetZip,
  importMapProject,
  downloadProjectExport,
  uploadDataset,
  listIcons,
  inspectArchive,
  readableApiError,
  createSavedConfig,
  listBuildings,
  listDatasets,
} from "../api"
import WorkspaceTable from "../components/WorkspaceTable"
import WorkspaceLayerPanel from "../components/WorkspaceLayerPanel"
import WorkspaceMap from "../components/WorkspaceMap"
import ErrorBoundary from "../components/ErrorBoundary"
import useWorkspace from "../hooks/useWorkspace"
import EndSessionDialog from "../components/EndSessionDialog"
import ZipImportReview from "../components/ZipImportReview"
import ExportProjectModal from "../components/ExportProjectModal"
import ImportPdfModal from "../components/ImportPdfModal"
import { applyFloorSelection } from "../lib/floorSelection"
import { savedConfigMemberIds } from "../lib/sessionLoading"

function AddDatasetDialog({ open, onClose, onUploaded, existingMembers = [] }) {
  const inputRef = useRef(null)
  const [state, setState] = useState("confirm")
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState(null)
  const [archiveInspection, setArchiveInspection] = useState(null)
  const [pdfInspection, setPdfInspection] = useState(null)

  useEffect(() => {
    if (open) {
      setState("confirm")
      setProgress(0)
      setError(null)
      setArchiveInspection(null)
      setPdfInspection(null)
    }
  }, [open])

  if (!open) return null

  async function selectFile(event) {
    const files = Array.from(event.target.files || [])
    event.target.value = ""
    if (!files.length) return
    const isSupported = (name) => /\.(png|jpe?g|tif|tiff|webp|bmp|pdf|dxf|dwg|svf|zip)$/i.test(name || "")
    const invalid = files.find((f) => !isSupported(f.name))
    if (invalid) {
      setError({
        message: "Unsupported file format.",
        details: `File "${invalid.name}" is not supported. Supported formats are PNG, JPG, TIFF, WebP, PDF, CAD, SVF, and ZIP.`,
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
        })

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
      <section className="workspace-dialog" role="dialog" aria-modal="true" aria-labelledby="add-dataset-title">
        <button type="button" className="workspace-dialog-close" onClick={onClose} aria-label="Close"><X /></button>
        <div className="workspace-dialog-icon"><Layers /></div>
        <h2 id="add-dataset-title">Add another dataset?</h2>
        <p>
          The new file will remain an independent dataset and will be attached to this workspace as another layer.
          If it is not georeferenced, you will be taken to the georeference page first.
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
              <Upload /> Select File(s)
            </button>
          </div>
        )}
        <input
          ref={inputRef}
          type="file"
          multiple
          accept=".png,.jpg,.jpeg,.tif,.tiff,.webp,.bmp,.pdf,.dxf,.dwg,.svf,.zip,image/png,image/jpeg,image/tiff,application/pdf,application/zip"
          className="sr-only"
          onChange={selectFile}
        />
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
  const { id } = useParams()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedView = searchParams.get("view")
  const [layersOpen, setLayersOpen] = useState(false)
  const [addOpen, setAddOpen] = useState(false)
  const [icons, setIcons] = useState([])
  const [dirty, setDirty] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [reopening, setReopening] = useState(false)
  const reopenInputRef = useRef(null)
  const [transformingMember, setTransformingMember] = useState(null)
  const [activeFloorLevel, setActiveFloorLevel] = useState(0)
  const [activeBuildingName, setActiveBuildingName] = useState("")
  const [isolateActiveFloor, setIsolateActiveFloor] = useState(true)
  const initializedFloorRef = useRef(false)
  const workspace = useWorkspace(id)
  const view = requestedView === "database" ? "database" : "map"

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

  function handleSelectFloor(level, buildingName) {
    applyFloorSelection({
      level,
      buildingName,
      members: workspace.manifest?.members,
      setFloorLevel: setActiveFloorLevel,
      setBuildingName: setActiveBuildingName,
      updateMember: (datasetId, patch) => workspace.updateMember(datasetId, patch).catch(() => undefined),
    })
  }

  async function handleReopenExportedZip(event) {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) return
    if (!confirmDiscard()) return
    setReopening(true)
    try {
      let targetId = null
      try {
        const res = await importMapProject(file)
        targetId = res.data?.id || res.data?.dataset_id || res.dataset_id
      } catch (projErr) {
        const res = await importDatasetZip(file)
        targetId = res.id || res.data?.id
      }
      if (targetId) {
        navigate(`/datasets/${targetId}/workspace?view=map`)
      }
    } catch (err) {
      workspace.setError({
        message: "Failed to reopen exported project package.",
        details: err?.response?.data?.detail || err?.message || "Invalid package.",
      })
    } finally {
      setReopening(false)
    }
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
      const members = savedConfigMemberIds(workspace.manifest?.members)
      const payload = {
        name,
        primary_dataset_id: id,
        members,
        configuration: {
          active_view: view,
          camera: workspace.manifest?.camera || null,
        },
      }
      await createSavedConfig(null, payload)
      setSaveConfigModalOpen(false)
      setConfigSavedRecently(true)
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

  async function handleAddedDataset(dataset) {
    await workspace.attachDataset(dataset.id).catch(() => undefined)
    if (dataset.imported_datasets && Array.isArray(dataset.imported_datasets)) {
      for (const extra of dataset.imported_datasets) {
        if (extra.id && extra.id !== dataset.id) {
          await workspace.attachDataset(extra.id).catch(() => undefined)
        }
      }
    }
    await workspace.refreshManifest().catch(() => undefined)
    setAddOpen(false)
    if (!dataset.is_georeferenced) {
      const returnTo = `/datasets/${id}/workspace?view=map`
      navigate(`/datasets/${dataset.id}/georeference?returnTo=${encodeURIComponent(returnTo)}`)
      return
    }
    setView("map")
  }

  function handleDetach(member) {
    if (!window.confirm(`Detach “${member.name}” from this workspace? The dataset and its files will remain in the library.`)) return
    workspace.detachDataset(member.dataset_id).catch(() => undefined)
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

  if (workspace.loading) {
    return <div className="workspace-loading"><Loader2 className="animate-spin" /> Loading workspace…</div>
  }

  if (!workspace.manifest) {
    return (
      <div className="workspace-loading workspace-load-error">
        <AlertCircle className="size-8 text-amber-500 mb-2" />
        <strong>{workspace.error?.message || "Workspace not found in current session"}</strong>
        <span className="max-w-md text-center text-slate-500 mb-4">
          {workspace.error?.details || "The requested dataset belongs to a different or previous session that was cleared. Return to the library to view available datasets or upload new floor plans."}
        </span>
        <Link to="/" className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-xs font-semibold rounded-lg shadow-sm transition-colors">
          Return to library
        </Link>
      </div>
    )
  }


  return (
    <div className="dataset-workspace-shell">
      <header className="workspace-topbar">
        <div className="workspace-topbar-left">
          <Link to="/" onClick={(event) => { if (!confirmDiscard()) event.preventDefault() }} aria-label="Back to library"><ArrowLeft /></Link>
          <button type="button" className="workspace-mobile-layers" onClick={() => setLayersOpen(true)} aria-label="Open layers"><Menu /></button>
          <div>
            <span className="workspace-eyebrow">Dataset workspace</span>
            <h1>{workspace.manifest.name}</h1>
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
            disabled={savingConfig}
            title="Save this map configuration to the dashboard"
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
          <input
            ref={reopenInputRef}
            type="file"
            accept=".zip,application/zip"
            className="sr-only"
            onChange={handleReopenExportedZip}
            disabled={reopening}
          />
          <button
            type="button"
            className="workspace-secondary-button"
            onClick={() => reopenInputRef.current?.click()}
            disabled={reopening}
            title="Reopen an exported Map Studio project package"
          >
            {reopening ? <Loader2 className="animate-spin" /> : <FolderOpen />}
            <span>{reopening ? "Opening…" : "Reopen Map (.zip)"}</span>
          </button>
          <EndSessionDialog />
        </div>
      </header>

      {workspace.error && (
        <div className="workspace-error-banner">
          <AlertCircle />
          <span><strong>{workspace.error.message}</strong> {workspace.error.details}</span>
          <button type="button" onClick={() => workspace.setError(null)} aria-label="Dismiss error"><X /></button>
        </div>
      )}

      <div className="workspace-body">
        <div className="workspace-desktop-layers">
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
              <Bookmark className="text-blue-600 size-6" />
            </div>
            <h2 id="save-config-title">Save Map Configuration</h2>
            <p>Save this configuration to the dashboard so you can reopen all layers, camera position, and active datasets anytime during this session.</p>
            <form onSubmit={handleSaveConfigSubmit} className="mt-4 flex flex-col gap-4 text-left">
              <div>
                <label className="text-xs font-semibold text-slate-700 block mb-1">Configuration Name</label>
                <input
                  type="text"
                  value={saveConfigName}
                  onChange={(e) => setSaveConfigName(e.target.value)}
                  placeholder="e.g. Master Floorplan Review"
                  className="w-full px-3 py-2 text-sm border border-slate-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                  autoFocus
                />
              </div>
              <div className="text-xs text-slate-500 bg-slate-50 p-3 rounded-lg border border-slate-200">
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
    </div>
  )
}
