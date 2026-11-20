import React, { useEffect, useMemo, useState } from "react"
import {
  ChevronDown,
  ChevronRight,
  Star,
  Clock,
  FolderOpen,
  Package,
  Shapes,
  Image as ImageIcon,
  FileCode,
  FileText,
  Globe,
  Map as MapIcon,
  Radio,
  Bookmark,
  BookmarkPlus,
  Search,
  RefreshCw,
  Plus,
  Loader2,
  Trash2,
  X,
  Eye,
} from "lucide-react"
import {
  listUserFiles,
  listMaps,
  inspectUploadedDataset,
  addWorkspaceDatasetLayers,
  readableApiError,
} from "../../api"
import {
  loadBrowserFavorites,
  saveBrowserFavorites,
} from "../../lib/workspaceLayout"

const VECTOR_EXT = /\.(shp|geojson|json|kml|osm|zip)$/i
const GPKG_EXT = /\.gpkg$/i
const RASTER_EXT = /\.(tif|tiff|png|jpg|jpeg|webp|bmp|mbtiles)$/i
const CAD_EXT = /\.(dxf|dwg|svf)$/i
const PDF_EXT = /\.pdf$/i

function fileExt(name = "") {
  const m = /\.([^.]+)$/.exec(String(name))
  return m ? m[1].toLowerCase() : ""
}

export default function WorkspaceBrowserPanel({
  workspaceId,
  userId,
  customBaseLayers = [],
  mapViews = [],
  bookmarks = [],
  onAddBookmark,
  onSelectBookmark,
  onDeleteBookmark,
  onSelectMapView,
  onSelectCustomBaseLayer,
  onOpenSavedMap,
  onLayersAdded,
  onOpenUpload,
  onCollapse,
  notify,
}) {
  const [files, setFiles] = useState([])
  const [savedMaps, setSavedMaps] = useState([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState("")
  const [expandedNodes, setExpandedNodes] = useState({
    geopackages: true,
    vectors: true,
    rasters: true,
    bookmarks: true,
  })
  const [favorites, setFavorites] = useState(() => loadBrowserFavorites(userId))

  // Dynamically inspected GeoPackage / multi-layer dataset metadata cache: { [datasetId]: { loading, data, error } }
  const [inspectedById, setInspectedById] = useState({})
  const [addingKey, setAddingKey] = useState("")
  const [previewItem, setPreviewItem] = useState(null)

  const refreshBrowser = async () => {
    setLoading(true)
    try {
      const [filesRes, mapsRes] = await Promise.all([
        listUserFiles().catch(() => []),
        listMaps().catch(() => ({ my_maps: [], saved_maps: [] })),
      ])
      const list = Array.isArray(filesRes) ? filesRes : filesRes?.files || []
      setFiles(list)
      const combined = [
        ...(mapsRes?.saved_maps || []),
        ...(mapsRes?.my_maps || []),
      ]
      const uniqueMaps = Array.from(new Map(combined.map((m) => [m.id, m])).values())
      setSavedMaps(uniqueMaps)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refreshBrowser()
  }, [workspaceId])

  const toggleNode = (key) => {
    setExpandedNodes((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  const toggleFavorite = (fileId) => {
    setFavorites((prev) => {
      const next = prev.includes(fileId)
        ? prev.filter((x) => x !== fileId)
        : [fileId, ...prev]
      saveBrowserFavorites(userId, next)
      return next
    })
  }

  const inspectFile = async (file) => {
    const id = file.id
    if (inspectedById[id]?.data || inspectedById[id]?.loading) {
      setExpandedNodes((prev) => ({ ...prev, [`ds:${id}`]: !prev[`ds:${id}`] }))
      if (inspectedById[id]?.data) {
        setPreviewItem({ kind: "dataset", file, inspection: inspectedById[id].data })
      }
      return
    }
    setExpandedNodes((prev) => ({ ...prev, [`ds:${id}`]: true }))
    setInspectedById((prev) => ({ ...prev, [id]: { loading: true, data: null, error: "" } }))
    try {
      const res = await inspectUploadedDataset({ file_id: id, workspace_id: workspaceId || "" })
      setInspectedById((prev) => ({ ...prev, [id]: { loading: false, data: res, error: "" } }))
      setPreviewItem({ kind: "dataset", file, inspection: res })
    } catch (err) {
      const msg = readableApiError(err, "Unable to inspect dataset.").message
      setInspectedById((prev) => ({ ...prev, [id]: { loading: false, data: null, error: msg } }))
    }
  }

  const handleAddDatasetLayer = async (datasetId, layerNames = null, label = "dataset") => {
    if (!workspaceId || !datasetId) return
    const key = `${datasetId}:${layerNames ? layerNames.join(",") : "all"}`
    setAddingKey(key)
    try {
      const payload = {
        dataset_id: datasetId,
        workspace_id: workspaceId,
      }
      if (Array.isArray(layerNames) && layerNames.length > 0) {
        payload.selected_layer_names = layerNames
      }
      const res = await addWorkspaceDatasetLayers(workspaceId, payload)
      const count = res?.layers?.length || 1
      notify?.(`Added ${count} layer${count === 1 ? "" : "s"} from ${label}`)
      onLayersAdded?.(res?.layers || [], datasetId)
    } catch (err) {
      notify?.(readableApiError(err, `Failed to add ${label}`).message)
    } finally {
      setAddingKey("")
    }
  }

  const q = search.trim().toLowerCase()
  const filteredFiles = useMemo(() => {
    if (!q) return files
    return files.filter((f) =>
      String(f.original_filename || f.name || "").toLowerCase().includes(q)
    )
  }, [files, q])

  const grouped = useMemo(() => {
    const favoriteFiles = filteredFiles.filter((f) => favorites.includes(f.id))
    const recentFiles = [...filteredFiles]
      .sort((a, b) => new Date(b.created_at || b.updated_at || 0) - new Date(a.created_at || a.updated_at || 0))
      .slice(0, 8)
    const geopackages = filteredFiles.filter((f) => GPKG_EXT.test(f.original_filename || ""))
    const vectors = filteredFiles.filter(
      (f) => VECTOR_EXT.test(f.original_filename || "") && !GPKG_EXT.test(f.original_filename || "")
    )
    const rasters = filteredFiles.filter((f) => RASTER_EXT.test(f.original_filename || ""))
    const cad = filteredFiles.filter((f) => CAD_EXT.test(f.original_filename || ""))
    const pdfs = filteredFiles.filter((f) => PDF_EXT.test(f.original_filename || ""))
    return { favoriteFiles, recentFiles, geopackages, vectors, rasters, cad, pdfs }
  }, [filteredFiles, favorites])

  const renderFileRow = (file, allowExpandLayers = false) => {
    const fname = file.original_filename || file.name || file.id
    const isFav = favorites.includes(file.id)
    const expanded = Boolean(expandedNodes[`ds:${file.id}`])
    const insp = inspectedById[file.id]
    const discoveredLayers = insp?.data?.layers || []

    return (
      <div key={file.id} className="select-none">
        <div
          className="group flex items-center gap-1 px-1.5 py-1 rounded-[3px] hover:bg-neutral-200/70 dark:hover:bg-[#303030] text-[11px] cursor-pointer"
          onClick={() => inspectFile(file)}
          title={`${fname} — Click to inspect`}
        >
          {allowExpandLayers || GPKG_EXT.test(fname) ? (
            <span className="text-neutral-400 shrink-0">
              {expanded ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
            </span>
          ) : (
            <span className="w-3 shrink-0" />
          )}

          <span className="truncate flex-1 text-neutral-800 dark:text-neutral-200 font-medium">
            {fname}
          </span>

          <span className="text-[9px] font-mono uppercase text-neutral-400 shrink-0">
            {fileExt(fname)}
          </span>

          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              toggleFavorite(file.id)
            }}
            className={`p-0.5 rounded opacity-60 group-hover:opacity-100 cursor-pointer ${
              isFav ? "text-amber-400 opacity-100" : "text-neutral-400 hover:text-amber-400"
            }`}
            title={isFav ? "Remove from Favorites" : "Add to Favorites"}
          >
            <Star className="size-3" fill={isFav ? "currentColor" : "none"} />
          </button>

          <button
            type="button"
            disabled={addingKey === `${file.id}:all`}
            onClick={(e) => {
              e.stopPropagation()
              handleAddDatasetLayer(file.id, null, fname)
            }}
            className="p-0.5 rounded text-[#24b47e] dark:text-[#3ecf8e] hover:bg-[#3ecf8e]/20 cursor-pointer shrink-0"
            title={`Add ${fname} to workspace`}
          >
            {addingKey === `${file.id}:all` ? (
              <Loader2 className="size-3 animate-spin" />
            ) : (
              <Plus className="size-3" />
            )}
          </button>
        </div>

        {/* Dynamically discovered GeoPackage / multi-layer child rows */}
        {expanded && (
          <div className="ml-4 pl-2 border-l border-neutral-300 dark:border-[#353535] my-0.5 space-y-0.5">
            {insp?.loading && (
              <div className="flex items-center gap-1.5 py-1 text-[10px] text-neutral-400">
                <Loader2 className="size-3 animate-spin text-[#3ecf8e]" /> Inspecting layers…
              </div>
            )}
            {insp?.error && (
              <div className="py-1 text-[10px] text-amber-500">{insp.error}</div>
            )}
            {discoveredLayers.length > 0 && (
              <>
                {discoveredLayers.map((lyr) => {
                  const lyrName = lyr.layer_name || lyr.name || "Layer"
                  const lKey = `${file.id}:${lyrName}`
                  return (
                    <div
                      key={lyrName}
                      onClick={() => setPreviewItem({ kind: "layer", file, layer: lyr })}
                      className="flex items-center justify-between gap-1 px-1.5 py-0.5 rounded-[3px] hover:bg-neutral-200/70 dark:hover:bg-[#303030] text-[10.5px] cursor-pointer"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-neutral-800 dark:text-neutral-200 font-medium">
                          {lyrName}
                        </div>
                        <div className="text-[9.5px] text-neutral-400 font-mono truncate">
                          {lyr.geometry_type || lyr.layer_type || "Layer"} • {lyr.feature_count ?? 0} • {lyr.crs || "—"}
                        </div>
                      </div>
                      <button
                        type="button"
                        disabled={addingKey === lKey}
                        onClick={(e) => {
                          e.stopPropagation()
                          handleAddDatasetLayer(file.id, [lyrName], lyrName)
                        }}
                        className="px-1.5 py-0.5 rounded bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] hover:bg-[#3ecf8e] hover:text-[#141414] text-[9.5px] font-bold cursor-pointer shrink-0"
                        title={`Add layer "${lyrName}"`}
                      >
                        {addingKey === lKey ? "…" : "+ Add"}
                      </button>
                    </div>
                  )
                })}
              </>
            )}
            {!insp?.loading && !insp?.error && discoveredLayers.length === 0 && insp?.data && (
              <div className="py-0.5 text-[10px] text-neutral-400">
                Single dataset ({insp.data.crs || "CRS not set"})
              </div>
            )}
          </div>
        )}
      </div>
    )
  }

  const TreeSection = ({ id, label, icon: Icon, count, action, children }) => {
    const isOpen = Boolean(expandedNodes[id])
    return (
      <div>
        <div
          onClick={() => toggleNode(id)}
          className="flex items-center gap-2 px-2 py-1.5 rounded-[5px] hover:bg-neutral-200/70 dark:hover:bg-[#2a2a2a] cursor-pointer text-[12px] font-semibold text-neutral-800 dark:text-neutral-100"
        >
          {isOpen ? (
            <ChevronDown className="size-3.5 text-neutral-400 shrink-0" />
          ) : (
            <ChevronRight className="size-3.5 text-neutral-400 shrink-0" />
          )}
          <Icon className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
          <span className="truncate flex-1">{label}</span>
          {count !== undefined && (
            <span className="px-1.5 py-0.5 rounded bg-neutral-200/70 dark:bg-[#2d2d2d] text-[10px] font-mono text-neutral-500 dark:text-neutral-400">
              {count}
            </span>
          )}
          {action}
        </div>
        {isOpen && <div className="pl-3.5 pr-1 py-1 space-y-1">{children}</div>}
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full min-h-0 bg-white dark:bg-[#1e1e1e] text-neutral-800 dark:text-neutral-200">
      {/* Dock Header */}
      <div className="h-9 px-3 border-b border-neutral-200 dark:border-[#303030] bg-neutral-100 dark:bg-[#252525] flex items-center justify-between gap-2 shrink-0">
        <span className="text-[11px] font-bold uppercase tracking-wider text-neutral-700 dark:text-neutral-200">
          Browser
        </span>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={refreshBrowser}
            className="p-1.5 rounded-[5px] hover:bg-neutral-200 dark:hover:bg-[#353535] text-neutral-500 dark:text-neutral-300 cursor-pointer"
            title="Refresh Browser"
          >
            <RefreshCw className={`size-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          <button
            type="button"
            onClick={onOpenUpload}
            className="p-1.5 rounded-[5px] hover:bg-neutral-200 dark:hover:bg-[#353535] text-[#24b47e] dark:text-[#3ecf8e] cursor-pointer"
            title="Upload New Dataset"
          >
            <Plus className="size-3.5" />
          </button>
          {onCollapse && (
            <button
              type="button"
              onClick={onCollapse}
              className="p-1.5 rounded-[5px] hover:bg-neutral-200 dark:hover:bg-[#353535] text-neutral-500 dark:text-neutral-300 cursor-pointer"
              title="Hide Browser Panel"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Filter Bar */}
      <div className="p-2.5 border-b border-neutral-200 dark:border-[#2c2c2c] shrink-0">
        <div className="relative">
          <Search className="size-3.5 text-neutral-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter datasets, layers…"
            className="w-full h-8 pl-8 pr-3 text-[12px] rounded-[6px] bg-neutral-100 dark:bg-[#161616] border border-neutral-200 dark:border-[#333333] text-neutral-800 dark:text-neutral-100 placeholder:text-neutral-500 outline-none focus:border-[#3ecf8e]"
          />
        </div>
      </div>

      {/* Tree Content */}
      <div className="flex-1 overflow-y-auto py-1.5 px-1 space-y-0.5 min-h-0">
        <TreeSection id="favorites" label="Favorites" icon={Star} count={grouped.favoriteFiles.length}>
          {grouped.favoriteFiles.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">Star any upload to pin it here.</div>
          ) : (
            grouped.favoriteFiles.map((f) => renderFileRow(f, true))
          )}
        </TreeSection>

        <TreeSection id="recent" label="Recent Datasets" icon={Clock} count={grouped.recentFiles.length}>
          {grouped.recentFiles.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No recent uploads yet.</div>
          ) : (
            grouped.recentFiles.map((f) => renderFileRow(f, GPKG_EXT.test(f.original_filename || "")))
          )}
        </TreeSection>

        <TreeSection id="uploads" label="My Uploads" icon={FolderOpen} count={filteredFiles.length}>
          {filteredFiles.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No uploaded files found.</div>
          ) : (
            filteredFiles.map((f) => renderFileRow(f, true))
          )}
        </TreeSection>

        <TreeSection id="geopackages" label="GeoPackages" icon={Package} count={grouped.geopackages.length}>
          {grouped.geopackages.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No .gpkg files uploaded.</div>
          ) : (
            grouped.geopackages.map((f) => renderFileRow(f, true))
          )}
        </TreeSection>

        <TreeSection id="vectors" label="Vector Files" icon={Shapes} count={grouped.vectors.length}>
          {grouped.vectors.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No Shapefile/GeoJSON/KML/OSM files.</div>
          ) : (
            grouped.vectors.map((f) => renderFileRow(f, true))
          )}
        </TreeSection>

        <TreeSection id="rasters" label="Raster Files" icon={ImageIcon} count={grouped.rasters.length}>
          {grouped.rasters.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No GeoTIFF/PNG/JPG rasters.</div>
          ) : (
            grouped.rasters.map((f) => renderFileRow(f, false))
          )}
        </TreeSection>

        <TreeSection id="cad" label="CAD Files" icon={FileCode} count={grouped.cad.length}>
          {grouped.cad.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No DXF/DWG files.</div>
          ) : (
            grouped.cad.map((f) => renderFileRow(f, true))
          )}
        </TreeSection>

        <TreeSection id="pdfs" label="PDF Files" icon={FileText} count={grouped.pdfs.length}>
          {grouped.pdfs.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No PDF files.</div>
          ) : (
            grouped.pdfs.map((f) => renderFileRow(f, false))
          )}
        </TreeSection>

        <TreeSection id="custom_bases" label="Custom Base Layers" icon={Globe} count={customBaseLayers.length}>
          {customBaseLayers.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No custom base layers registered.</div>
          ) : (
            customBaseLayers.map((bl) => (
              <div
                key={bl.id}
                onClick={() => setPreviewItem({ kind: "base", baseLayer: bl })}
                className="flex items-center justify-between gap-1 px-1.5 py-1 rounded-[3px] hover:bg-neutral-200/70 dark:hover:bg-[#303030] text-[11px] cursor-pointer"
              >
                <span className="truncate flex-1">{bl.name}</span>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    onSelectCustomBaseLayer?.(bl)
                  }}
                  className="px-1.5 py-0.5 rounded bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] hover:bg-[#3ecf8e] hover:text-[#141414] text-[9.5px] font-bold cursor-pointer"
                >
                  Use
                </button>
              </div>
            ))
          )}
        </TreeSection>

        <TreeSection id="saved_maps" label="Saved Maps" icon={MapIcon} count={savedMaps.length}>
          {savedMaps.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">No saved maps.</div>
          ) : (
            savedMaps.map((m) => (
              <button
                key={m.id}
                type="button"
                onClick={() => onOpenSavedMap?.(m)}
                className="w-full text-left flex items-center justify-between gap-1 px-1.5 py-1 rounded-[3px] hover:bg-neutral-200/70 dark:hover:bg-[#303030] text-[11px] cursor-pointer"
              >
                <span className="truncate flex-1">{m.name}</span>
                <span className="text-[9.5px] font-mono text-neutral-400">v{m.revision_number || 1}</span>
              </button>
            ))
          )}
        </TreeSection>

        <TreeSection id="map_services" label="Map Services" icon={Radio} count={mapViews.length || 6}>
          {(mapViews.length > 0
            ? mapViews.filter((v) => (v.type || v.view_type) !== "custom")
            : [
                { type: "world", name: "World Map (OSM)" },
                { type: "satellite", name: "Satellite Imagery" },
                { type: "streets", name: "Streets" },
                { type: "terrain", name: "Terrain" },
                { type: "light", name: "Light Basemap" },
                { type: "dark", name: "Dark Basemap" },
              ]
          ).map((svc, idx) => {
            const vType = svc.type || svc.view_type || `svc_${idx}`
            return (
              <button
                key={vType}
                type="button"
                onClick={() => onSelectMapView?.(vType)}
                className="w-full text-left flex items-center justify-between gap-1 px-1.5 py-1 rounded-[3px] hover:bg-neutral-200/70 dark:hover:bg-[#303030] text-[11px] cursor-pointer"
              >
                <span className="truncate">{svc.name}</span>
                <Eye className="size-3 text-neutral-400 shrink-0" />
              </button>
            )
          })}
        </TreeSection>

        <TreeSection
          id="bookmarks"
          label="Spatial Bookmarks"
          icon={Bookmark}
          count={bookmarks.length}
          action={
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                onAddBookmark?.()
              }}
              className="p-0.5 rounded hover:bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e] cursor-pointer"
              title="Bookmark current map view"
            >
              <BookmarkPlus className="size-3" />
            </button>
          }
        >
          {bookmarks.length === 0 ? (
            <div className="px-2 py-1 text-[10px] text-neutral-400">
              Click + to bookmark the current camera extent.
            </div>
          ) : (
            bookmarks.map((bm) => (
              <div
                key={bm.id}
                onClick={() => onSelectBookmark?.(bm)}
                className="group flex items-center justify-between gap-1 px-1.5 py-1 rounded-[3px] hover:bg-neutral-200/70 dark:hover:bg-[#303030] text-[11px] cursor-pointer"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium">{bm.name}</div>
                  <div className="text-[9.5px] font-mono text-neutral-400">
                    Z{Number(bm.zoom || 0).toFixed(1)} • {Number(bm.center?.[1] || 0).toFixed(3)},{" "}
                    {Number(bm.center?.[0] || 0).toFixed(3)}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    onDeleteBookmark?.(bm.id)
                  }}
                  className="p-0.5 rounded opacity-0 group-hover:opacity-100 text-neutral-400 hover:text-red-500 cursor-pointer"
                  title="Delete bookmark"
                >
                  <Trash2 className="size-3" />
                </button>
              </div>
            ))
          )}
        </TreeSection>
      </div>

      {/* Compact Dataset / Layer Preview Drawer */}
      {previewItem && (
        <div className="border-t border-neutral-300 dark:border-[#343434] bg-neutral-50 dark:bg-[#181818] p-2 text-[10.5px] shrink-0">
          <div className="flex items-center justify-between gap-1 mb-1">
            <span className="font-bold text-neutral-900 dark:text-white truncate">
              {previewItem.kind === "layer"
                ? previewItem.layer.name
                : previewItem.kind === "base"
                  ? previewItem.baseLayer.name
                  : previewItem.file.original_filename}
            </span>
            <button
              type="button"
              onClick={() => setPreviewItem(null)}
              className="p-0.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-white cursor-pointer"
            >
              <X className="size-3" />
            </button>
          </div>
          {previewItem.kind === "layer" && (
            <div className="space-y-1 text-neutral-500 dark:text-neutral-400">
              <div className="font-mono">
                {previewItem.layer.geometry_type || "Vector"} • {previewItem.layer.feature_count ?? 0} features •{" "}
                {previewItem.layer.crs || "CRS unknown"}
              </div>
              <button
                type="button"
                onClick={() =>
                  handleAddDatasetLayer(previewItem.file.id, [previewItem.layer.name], previewItem.layer.name)
                }
                className="w-full py-1 rounded bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] font-bold text-[10.5px] cursor-pointer"
              >
                Add Layer to Workspace
              </button>
            </div>
          )}
          {previewItem.kind === "dataset" && (
            <div className="space-y-1 text-neutral-500 dark:text-neutral-400">
              <div className="font-mono">
                {previewItem.inspection?.layers?.length
                  ? `${previewItem.inspection.layers.length} layer(s) detected`
                  : previewItem.inspection?.driver || previewItem.file.file_type || "Dataset"}{" "}
                • {previewItem.inspection?.crs || "Inspect CRS"}
              </div>
              <button
                type="button"
                onClick={() =>
                  handleAddDatasetLayer(previewItem.file.id, null, previewItem.file.original_filename)
                }
                className="w-full py-1 rounded bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] font-bold text-[10.5px] cursor-pointer"
              >
                Add All to Workspace
              </button>
            </div>
          )}
          {previewItem.kind === "base" && (
            <div className="space-y-1 text-neutral-500 dark:text-neutral-400">
              <div className="font-mono">
                {previewItem.baseLayer.crs || "EPSG:4326"} • {previewItem.baseLayer.source_format || "Base Layer"}
              </div>
              <button
                type="button"
                onClick={() => onSelectCustomBaseLayer?.(previewItem.baseLayer)}
                className="w-full py-1 rounded bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] font-bold text-[10.5px] cursor-pointer"
              >
                Set as Active Base Layer
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
