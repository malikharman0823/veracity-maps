import React, { useEffect, useState } from "react"
import {
  Database,
  Search,
  Layers,
  Trash2,
  Edit2,
  Plus,
  Check,
  X,
  Image,
  Shapes,
  FileCode,
} from "lucide-react"

import {
  listDatasets,
  deleteDataset,
  getGISDatasetMetadata,
  inspectGISDataset,
  renameGISDataset,
  createMapLayer,
} from "../../api"

export default function DatasetBrowserModal({
  open,
  onClose,
  mapId,
  onDatasetAttached,
  onDatasetsChanged,
  notify,
}) {
  const [datasets, setDatasets] = useState([])
  const [search, setSearch] = useState("")
  const [typeFilter, setTypeFilter] = useState("all") // "all" | "raster" | "vector" | "cad"
  const [selectedDs, setSelectedDs] = useState(null)
  const [details, setDetails] = useState(null)
  const [loading, setLoading] = useState(false)
  const [renamingId, setRenamingId] = useState(null)
  const [draftName, setDraftName] = useState("")

  const loadList = async () => {
    setLoading(true)
    try {
      const list = await listDatasets()
      setDatasets(Array.isArray(list) ? list : [])
    } catch {
      setDatasets([])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (open) {
      loadList()
    }
  }, [open])

  const loadDetails = async (ds) => {
    setSelectedDs(ds)
    try {
      const meta = await getGISDatasetMetadata(ds.id)
      setDetails(meta)
    } catch {
      setDetails(null)
    }
  }

  if (!open) return null

  const classifyDataset = (ds) => {
    const ext = (ds.filename || "").split(".").pop()?.toLowerCase() || ""
    if (["gpkg", "shp", "geojson", "json", "kml"].includes(ext)) return "vector"
    if (["dxf", "dwg", "svf"].includes(ext)) return "cad"
    return "raster"
  }

  const filtered = datasets.filter((ds) => {
    const matchesSearch = !search.trim() || (ds.filename || "").toLowerCase().includes(search.toLowerCase())
    const kind = classifyDataset(ds)
    const matchesType = typeFilter === "all" || kind === typeFilter
    return matchesSearch && matchesType
  })

  const handleRename = async (dsId) => {
    if (!draftName.trim()) {
      setRenamingId(null)
      return
    }
    try {
      await renameGISDataset(dsId, draftName.trim())
      setRenamingId(null)
      notify?.("Dataset renamed")
      loadList()
      if (selectedDs?.id === dsId) {
        loadDetails({ ...selectedDs, filename: draftName.trim() })
      }
      onDatasetsChanged?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to rename dataset", "error")
    }
  }

  const handleDelete = async (ds) => {
    try {
      await deleteDataset(ds.id)
      notify?.(`Deleted dataset "${ds.filename}"`)
      if (selectedDs?.id === ds.id) {
        setSelectedDs(null)
        setDetails(null)
      }
      loadList()
      onDatasetsChanged?.()
    } catch (err) {
      const code = err?.response?.data?.error?.code
      if (code === "DATASET_IN_USE" || err?.response?.status === 409) {
        notify?.(
          err?.response?.data?.error?.message ||
            "Cannot delete dataset because it is referenced by active map layers.",
          "error"
        )
      } else {
        notify?.(err?.response?.data?.error?.message || "Failed to delete dataset", "error")
      }
    }
  }

  const handleReuseInMap = async (ds, subLayer = null) => {
    if (!mapId) {
      onDatasetAttached?.(ds)
      onClose?.()
      return
    }
    try {
      const kind = classifyDataset(ds)
      const created = await createMapLayer(mapId, {
        dataset_id: ds.id,
        dataset_layer_id: subLayer?.dataset_layer_id || "",
        layer_name: subLayer?.layer_name || ds.filename,
        layer_type: kind,
        geometry_type: subLayer?.geometry_type || (kind === "vector" ? "Polygon" : ""),
        crs: subLayer?.crs || ds.extracted_metadata?.crs || "EPSG:4326",
      })
      notify?.(`Attached "${created.layer_name}" to map without duplicating storage`)
      onDatasetAttached?.(ds, created)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed attaching dataset to map", "error")
    }
  }

  const handleDeepInspect = async () => {
    if (!selectedDs) return
    try {
      const res = await inspectGISDataset(selectedDs.id)
      setDetails((prev) => ({ ...prev, ...res }))
      notify?.("GDAL/OGR dataset inspection complete")
    } catch (err) {
      notify?.("Dataset inspection failed", "error")
    }
  }

  return (
    <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex items-center justify-center p-4">
      <div className="w-full max-w-4xl h-[75vh] rounded-[12px] bg-white dark:bg-[#181818] border border-neutral-200 dark:border-[#2e2e2e] shadow-2xl flex flex-col overflow-hidden text-xs">
        <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#282828] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Database className="size-4 text-[#3ecf8e]" />
            <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
              GIS Dataset Browser &amp; Multi-Layer Inspector
            </h3>
          </div>
          <button type="button" onClick={onClose} className="p-1 rounded text-neutral-400 hover:text-white">
            <X className="size-4" />
          </button>
        </div>

        <div className="flex-1 flex min-h-0">
          {/* Left Column: Dataset Catalog */}
          <div className="w-1/2 border-r border-neutral-200 dark:border-[#282828] flex flex-col">
            <div className="p-3 border-b border-neutral-200 dark:border-[#282828] space-y-2">
              <div className="flex items-center gap-2 px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#222] border border-neutral-300 dark:border-[#333]">
                <Search className="size-3.5 text-neutral-400" />
                <input
                  type="text"
                  placeholder="Search datasets by filename…"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="bg-transparent outline-none w-full"
                />
              </div>
              <div className="flex items-center gap-1">
                {["all", "raster", "vector", "cad"].map((t) => (
                  <button
                    key={t}
                    type="button"
                    onClick={() => setTypeFilter(t)}
                    className={`px-2.5 py-1 rounded-[5px] capitalize font-medium ${
                      typeFilter === t
                        ? "bg-[#3ecf8e] text-[#171717] font-semibold"
                        : "bg-neutral-100 dark:bg-[#222] text-neutral-500"
                    }`}
                  >
                    {t}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex-1 overflow-y-auto divide-y divide-neutral-200 dark:divide-[#242424]">
              {filtered.map((ds) => {
                const kind = classifyDataset(ds)
                const isSelected = selectedDs?.id === ds.id
                return (
                  <div
                    key={ds.id}
                    onClick={() => loadDetails(ds)}
                    className={`p-3 flex items-center justify-between gap-2 cursor-pointer ${
                      isSelected ? "bg-[#3ecf8e]/10" : "hover:bg-neutral-50 dark:hover:bg-[#202020]"
                    }`}
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      {kind === "raster" ? (
                        <Image className="size-4 text-emerald-400 shrink-0" />
                      ) : kind === "cad" ? (
                        <FileCode className="size-4 text-amber-400 shrink-0" />
                      ) : (
                        <Shapes className="size-4 text-sky-400 shrink-0" />
                      )}
                      <div className="min-w-0">
                        {renamingId === ds.id ? (
                          <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
                            <input
                              type="text"
                              value={draftName}
                              onChange={(e) => setDraftName(e.target.value)}
                              className="px-1.5 py-0.5 rounded bg-white dark:bg-[#262626] border border-[#3ecf8e]"
                              autoFocus
                            />
                            <button type="button" onClick={() => handleRename(ds.id)} className="text-[#3ecf8e]">
                              <Check className="size-3.5" />
                            </button>
                          </div>
                        ) : (
                          <div className="font-semibold text-neutral-900 dark:text-white truncate">
                            {ds.filename}
                          </div>
                        )}
                        <div className="text-[10px] text-neutral-400 font-mono">
                          {((ds.file_size || 0) / 1024).toFixed(1)} KB • {kind.toUpperCase()} •{" "}
                          {ds.extracted_metadata?.crs || "Unreferenced"}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
                      <button
                        type="button"
                        onClick={() => handleReuseInMap(ds)}
                        className="px-2 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold text-[11px]"
                        title="Reuse dataset in current map"
                      >
                        + Use
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setRenamingId(ds.id)
                          setDraftName(ds.filename)
                        }}
                        className="p-1 text-neutral-400 hover:text-white"
                        title="Rename dataset"
                      >
                        <Edit2 className="size-3.5" />
                      </button>
                      <button
                        type="button"
                        onClick={() => handleDelete(ds)}
                        className="p-1 text-neutral-400 hover:text-red-500"
                        title="Delete dataset (checked for active map references)"
                      >
                        <Trash2 className="size-3.5" />
                      </button>
                    </div>
                  </div>
                )
              })}
              {filtered.length === 0 && !loading && (
                <div className="p-8 text-center text-neutral-400">
                  No matching datasets in project.
                </div>
              )}
            </div>
          </div>

          {/* Right Column: Metadata & Internal GeoPackage/CAD Layers */}
          <div className="w-1/2 p-4 overflow-y-auto space-y-4">
            {selectedDs ? (
              <>
                <div className="flex items-center justify-between">
                  <div>
                    <h4 className="text-sm font-bold text-neutral-900 dark:text-white">{selectedDs.filename}</h4>
                    <div className="text-[11px] font-mono text-neutral-400">ID: {selectedDs.id}</div>
                  </div>
                  <button
                    type="button"
                    onClick={handleDeepInspect}
                    className="px-2.5 py-1 rounded border border-[#3ecf8e] text-[#3ecf8e] font-semibold text-[11px]"
                  >
                    Re-Inspect (GDAL/OGR)
                  </button>
                </div>

                {/* Dataset Visual Preview Card */}
                <div className="rounded-lg bg-[#15181b] border border-[#2c333a] p-2.5 flex items-center gap-3">
                  <svg viewBox="0 0 120 60" className="w-28 h-14 rounded border border-[#2a2f35] shrink-0 bg-[#161a1d]">
                    <rect width="120" height="60" fill="#161a1d" />
                    <path d="M0 30 H120 M60 0 V60" stroke="#232a30" strokeWidth="1" />
                    <rect x="26" y="14" width="68" height="32" fill="#3ecf8e" fillOpacity="0.22" stroke="#3ecf8e" strokeWidth="1.5" strokeDasharray="3 2" />
                    <circle cx="26" cy="14" r="2.5" fill="#3ecf8e" />
                    <circle cx="94" cy="14" r="2.5" fill="#3ecf8e" />
                    <circle cx="94" cy="46" r="2.5" fill="#3ecf8e" />
                    <circle cx="26" cy="46" r="2.5" fill="#3ecf8e" />
                  </svg>
                  <div className="min-w-0">
                    <div className="text-[11px] font-bold text-[#3ecf8e]">
                      Dataset Footprint Preview
                    </div>
                    <p className="text-[11px] text-neutral-300 leading-snug">
                      Inspect the file format, CRS, and internal layers below, then click <strong>+ Use</strong> to add it to your map without duplicating files.
                    </p>
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-2.5 p-3 rounded-[8px] bg-neutral-50 dark:bg-[#141414] border border-neutral-200 dark:border-[#282828]">
                  <div>
                    <span className="text-[10px] text-neutral-400 block">CRS</span>
                    <strong className="font-mono">
                      {details?.extracted_metadata?.crs || selectedDs.extracted_metadata?.crs || "None"}
                    </strong>
                  </div>
                  <div>
                    <span className="text-[10px] text-neutral-400 block">Dimensions / Bands</span>
                    <strong className="font-mono">
                      {selectedDs.extracted_metadata?.width || "—"} × {selectedDs.extracted_metadata?.height || "—"} (
                      {selectedDs.extracted_metadata?.bands || 1} bands)
                    </strong>
                  </div>
                  <div>
                    <span className="text-[10px] text-neutral-400 block">Active Map References</span>
                    <strong className="font-mono text-[#3ecf8e]">{details?.referenced_layers ?? 0} layer(s)</strong>
                  </div>
                  <div>
                    <span className="text-[10px] text-neutral-400 block">Uploaded</span>
                    <strong className="font-mono">
                      {selectedDs.created_at ? new Date(selectedDs.created_at).toLocaleDateString() : "—"}
                    </strong>
                  </div>
                </div>

                {/* Internal Layers (GeoPackage / Shapefile / DXF / Raster) */}
                <div>
                  <div className="text-[11px] font-bold uppercase tracking-wider text-neutral-500 mb-2 flex items-center gap-1.5">
                    <Layers className="size-3.5 text-[#3ecf8e]" />
                    <span>Internal Dataset Layers ({details?.layers?.length || 0})</span>
                  </div>
                  <div className="space-y-1.5">
                    {(details?.layers || []).map((l) => (
                      <div
                        key={l.dataset_layer_id || l.layer_name}
                        className="p-2.5 rounded-[6px] border border-neutral-200 dark:border-[#2c2c2c] bg-neutral-50 dark:bg-[#1f1f1f] flex items-center justify-between"
                      >
                        <div>
                          <div className="font-semibold text-neutral-900 dark:text-white">{l.layer_name}</div>
                          <div className="text-[10px] text-neutral-400 font-mono">
                            Geometry: {l.geometry_type} • Features: {l.feature_count} • CRS: {l.crs || "Default"}
                          </div>
                        </div>
                        <button
                          type="button"
                          onClick={() => handleReuseInMap(selectedDs, l)}
                          className="inline-flex items-center gap-1 px-2.5 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold text-[11px]"
                        >
                          <Plus className="size-3" /> Add Layer
                        </button>
                      </div>
                    ))}
                  </div>
                </div>
              </>
            ) : (
              <div className="h-full flex items-center justify-center text-neutral-400 text-center">
                Select a dataset on the left to inspect its GDAL/OGR metadata, CRS, bounding box, and internal GeoPackage/CAD layers.
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
