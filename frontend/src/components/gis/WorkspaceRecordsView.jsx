import React, { useEffect, useState, useCallback } from "react"
import {
  Database,
  Layers,
  Image as ImageIcon,
  Crosshair,
  MapPin,
  Search,
  Plus,
  Download,
  Columns3,
  Eye,
  EyeOff,
  Navigation,
  Trash2,
  Edit2,
  Info,
  ChevronLeft,
  ChevronRight,
  ArrowUpDown,
  X,
  Check,
  Loader2,
} from "lucide-react"
import {
  listWorkspaceRecords,
  getWorkspaceRecordSchema,
  updateWorkspaceRecord,
  deleteWorkspaceRecord,
  exportWorkspaceRecords,
  updateWorkspaceLayer,
  readableApiError,
} from "../../api"

const TABS = [
  { id: "all", label: "All Records", icon: Database, countKey: "all" },
  { id: "vector", label: "Vectors", icon: Layers, countKey: "vectors" },
  { id: "raster", label: "Rasters", icon: ImageIcon, countKey: "rasters" },
  { id: "control_point", label: "Control Points", icon: Crosshair, countKey: "control_points" },
  { id: "user_icon", label: "User Icons", icon: MapPin, countKey: "user_icons" },
]

function errorText(err, fallback) {
  const res = readableApiError(err, fallback)
  if (typeof res === "string") return res
  if (res && typeof res === "object") {
    return res.details ? `${res.message} ${res.details}` : res.message || fallback
  }
  return fallback
}

function formatCellValue(record, col) {
  const key = col.key
  if (key.startsWith("attr:")) {
    const attrKey = key.slice(5)
    const v = record.attributes?.[attrKey]
    return v === undefined || v === null || v === "" ? "—" : String(v)
  }
  switch (key) {
    case "coordinates":
      if (Array.isArray(record.coordinates) && record.coordinates.length >= 2) {
        return `${Number(record.coordinates[1]).toFixed(5)}, ${Number(record.coordinates[0]).toFixed(5)}`
      }
      if (Array.isArray(record.bounds) && record.bounds.length === 4) {
        return `[${record.bounds.map((v) => Number(v).toFixed(3)).join(", ")}]`
      }
      return "—"
    case "bounds":
      if (Array.isArray(record.bounds) && record.bounds.length === 4) {
        return `[${record.bounds.map((v) => Number(v).toFixed(4)).join(", ")}]`
      }
      return "—"
    case "dimensions":
      return record.width && record.height ? `${record.width} × ${record.height} px` : "—"
    case "image_x":
      return record.image_x != null ? Number(record.image_x).toFixed(1) : "—"
    case "image_y":
      return record.image_y != null ? Number(record.image_y).toFixed(1) : "—"
    case "geographic_x":
      return record.geographic_x != null ? Number(record.geographic_x).toFixed(6) : "—"
    case "geographic_y":
      return record.geographic_y != null ? Number(record.geographic_y).toFixed(6) : "—"
    case "visibility":
      return record.visibility ? "Visible" : "Hidden"
    default: {
      const val = record[key]
      return val === undefined || val === null || val === "" ? "—" : String(val)
    }
  }
}

export default function WorkspaceRecordsView({
  workspaceId,
  gisLayers = [],
  refreshKey = 0,
  selectedFeatureIds = [],
  onSelectFeatureIds,
  onOpenAddDataset,
  onLocateRecord,
  onLayerVisibilityChanged,
  onRecordsMutated,
  notify,
}) {
  const [activeTab, setActiveTab] = useState("all")
  const [records, setRecords] = useState([])
  const [counts, setCounts] = useState({ all: 0, vectors: 0, rasters: 0, control_points: 0, user_icons: 0 })
  const [columns, setColumns] = useState([])
  const [visibleCols, setVisibleCols] = useState({})
  const [columnsMenuOpen, setColumnsMenuOpen] = useState(false)

  const [search, setSearch] = useState("")
  const [layerFilter, setLayerFilter] = useState("")
  const [statusFilter, setStatusFilter] = useState("")
  const [sortBy, setSortBy] = useState("display_name")
  const [sortDirection, setSortDirection] = useState("asc")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(25)
  const [total, setTotal] = useState(0)
  const [totalPages, setTotalPages] = useState(1)

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [selectedIds, setSelectedIds] = useState([])
  const [detailRecord, setDetailRecord] = useState(null)
  const [editingRecord, setEditingRecord] = useState(null)
  const [editDraft, setEditDraft] = useState({})
  const [savingEdit, setSavingEdit] = useState(false)

  const loadSchema = useCallback(async () => {
    if (!workspaceId) return
    try {
      const res = await getWorkspaceRecordSchema(workspaceId, {
        record_type: activeTab,
        layer_id: layerFilter,
      })
      const cols = res?.columns || []
      setColumns(cols)
      setVisibleCols((prev) => {
        const next = {}
        cols.forEach((c) => {
          next[c.key] = prev[c.key] !== undefined ? prev[c.key] : Boolean(c.default_show)
        })
        return next
      })
    } catch {}
  }, [workspaceId, activeTab, layerFilter])

  const loadRecords = useCallback(async () => {
    if (!workspaceId) return
    setLoading(true)
    setError("")
    try {
      const res = await listWorkspaceRecords(workspaceId, {
        record_type: activeTab,
        layer_id: layerFilter || undefined,
        search: search.trim() || undefined,
        status: statusFilter || undefined,
        page,
        page_size: pageSize,
        sort_by: sortBy,
        sort_direction: sortDirection,
      })
      setRecords(res?.records || [])
      setTotal(res?.total || 0)
      setTotalPages(res?.total_pages || 1)
      if (res?.counts) setCounts(res.counts)
    } catch (err) {
      setError(errorText(err, "Unable to load workspace records."))
    } finally {
      setLoading(false)
    }
  }, [workspaceId, activeTab, layerFilter, search, statusFilter, page, pageSize, sortBy, sortDirection])

  useEffect(() => {
    loadSchema()
  }, [loadSchema, refreshKey])

  useEffect(() => {
    loadRecords()
  }, [loadRecords, refreshKey])

  const handleSort = (colKey) => {
    const cleanKey = colKey.startsWith("attr:") ? colKey.slice(5) : colKey
    if (sortBy === cleanKey) {
      setSortDirection((d) => (d === "asc" ? "desc" : "asc"))
    } else {
      setSortBy(cleanKey)
      setSortDirection("asc")
    }
    setPage(1)
  }

  const toggleSelectAll = () => {
    if (selectedIds.length === records.length) {
      setSelectedIds([])
    } else {
      setSelectedIds(records.map((r) => r.id))
    }
  }

  const toggleSelectOne = (id) => {
    setSelectedIds((prev) => {
      const next = prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
      if (onSelectFeatureIds) {
        const fids = records
          .filter((r) => next.includes(r.id) && r.source_feature_id)
          .map((r) => r.source_feature_id)
        onSelectFeatureIds(fids)
      }
      return next
    })
  }

  // Sync external MapLibre feature selection into table row selection
  useEffect(() => {
    if (!Array.isArray(selectedFeatureIds) || records.length === 0) return
    const fidSet = new Set(selectedFeatureIds)
    const matched = records
      .filter((r) => r.source_feature_id && fidSet.has(r.source_feature_id))
      .map((r) => r.id)
    if (matched.length > 0) {
      setSelectedIds((prev) => {
        const same = prev.length === matched.length && matched.every((id) => prev.includes(id))
        return same ? prev : matched
      })
    }
  }, [selectedFeatureIds, records])

  const handleExport = async (format = "csv") => {
    try {
      const blob = await exportWorkspaceRecords(workspaceId, {
        format,
        record_type: activeTab,
        layer_id: layerFilter || undefined,
        record_ids: selectedIds.length > 0 ? selectedIds : undefined,
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = `workspace_records_${activeTab}.${format}`
      a.click()
      URL.revokeObjectURL(url)
      notify?.(`Exported ${selectedIds.length > 0 ? selectedIds.length : total} record(s) as ${format.toUpperCase()}`)
    } catch (err) {
      notify?.(errorText(err, "Export failed."), "error")
    }
  }

  const handleToggleLayerVis = async (rec) => {
    if (!rec.workspace_layer_id) return
    try {
      await updateWorkspaceLayer(workspaceId, rec.workspace_layer_id, {
        visibility: !rec.visibility,
      })
      onLayerVisibilityChanged?.()
      await loadRecords()
    } catch (err) {
      notify?.(errorText(err, "Failed toggling visibility."), "error")
    }
  }

  const handleDeleteRecord = async (rec) => {
    if (!window.confirm(`Delete record "${rec.display_name}"?`)) return
    try {
      await deleteWorkspaceRecord(workspaceId, rec.id)
      setSelectedIds((prev) => prev.filter((x) => x !== rec.id))
      if (detailRecord?.id === rec.id) setDetailRecord(null)
      notify?.(`Deleted "${rec.display_name}"`)
      onRecordsMutated?.()
      await loadRecords()
    } catch (err) {
      notify?.(errorText(err, "Unable to delete record."), "error")
    }
  }

  const openInlineEdit = (rec) => {
    setEditingRecord(rec)
    setEditDraft({
      display_name: rec.display_name || "",
      ...(rec.attributes || {}),
    })
  }

  const handleSaveInlineEdit = async (e) => {
    e.preventDefault()
    if (!editingRecord) return
    setSavingEdit(true)
    try {
      const { display_name, ...attrs } = editDraft
      await updateWorkspaceRecord(workspaceId, editingRecord.id, {
        display_name,
        editable_attributes: attrs,
      })
      setEditingRecord(null)
      notify?.("Record updated")
      onRecordsMutated?.()
      await loadRecords()
    } catch (err) {
      notify?.(errorText(err, "Failed saving record changes."), "error")
    } finally {
      setSavingEdit(false)
    }
  }

  const activeColumns = columns.filter((c) => visibleCols[c.key])

  return (
    <div className="flex flex-col h-full w-full bg-[#fafafa] dark:bg-[#16181d] text-neutral-900 dark:text-neutral-100 overflow-hidden">
      {/* Top Tabs & Primary Actions Bar */}
      <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#2a2d34] bg-white dark:bg-[#1c1f24] flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-1 overflow-x-auto">
          {TABS.map((tab) => {
            const Icon = tab.icon
            const active = activeTab === tab.id
            return (
              <button
                key={tab.id}
                type="button"
                onClick={() => {
                  setActiveTab(tab.id)
                  setPage(1)
                  setSelectedIds([])
                }}
                className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[7px] text-xs font-semibold transition-colors cursor-pointer shrink-0 ${
                  active
                    ? "bg-[#3ecf8e] text-[#171717] shadow-xs"
                    : "text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2a2d34]"
                }`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{tab.label}</span>
                <span
                  className={`px-1.5 py-0.5 rounded-full text-[10px] font-bold ${
                    active ? "bg-black/15 text-[#171717]" : "bg-neutral-200 dark:bg-[#2a2e37] text-neutral-600 dark:text-neutral-300"
                  }`}
                >
                  {counts[tab.countKey] ?? 0}
                </span>
              </button>
            )
          })}
        </div>

        <div className="flex items-center gap-2">
          <span className="text-[11px] font-mono text-neutral-500 dark:text-neutral-400">
            Total: <strong className="text-neutral-900 dark:text-white">{total}</strong>
          </span>
          {onOpenAddDataset && (
            <button
              type="button"
              onClick={onOpenAddDataset}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-bold cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              Add Dataset
            </button>
          )}
          <button
            type="button"
            onClick={() => handleExport("csv")}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#343842] text-xs font-semibold text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#2a2d34] cursor-pointer"
            title="Export selected or filtered records as CSV"
          >
            <Download className="w-3.5 h-3.5" />
            Export CSV {selectedIds.length > 0 ? `(${selectedIds.length})` : ""}
          </button>
          <button
            type="button"
            onClick={() => handleExport("geojson")}
            className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#343842] text-xs font-semibold text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#2a2d34] cursor-pointer"
            title="Export selected or filtered records as GeoJSON"
          >
            GeoJSON
          </button>
        </div>
      </div>

      {/* Filter & Column Controls Row */}
      <div className="px-4 py-2.5 border-b border-neutral-200 dark:border-[#2a2d34] bg-neutral-50/70 dark:bg-[#17191e] flex flex-wrap items-center gap-2.5">
        <div className="relative flex-1 min-w-[200px] max-w-sm">
          <Search className="w-3.5 h-3.5 text-neutral-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              setPage(1)
            }}
            placeholder="Search by name, layer, attribute, CRS…"
            className="w-full pl-8 pr-3 py-1.5 rounded-[6px] text-xs bg-white dark:bg-[#1e2026] border border-neutral-200 dark:border-[#2e323a] text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
          />
        </div>

        <select
          value={layerFilter}
          onChange={(e) => {
            setLayerFilter(e.target.value)
            setPage(1)
          }}
          className="px-2.5 py-1.5 rounded-[6px] text-xs bg-white dark:bg-[#1e2026] border border-neutral-200 dark:border-[#2e323a] text-neutral-800 dark:text-neutral-200"
        >
          <option value="">All Layers</option>
          {gisLayers.map((l) => (
            <option key={l.layer_id} value={l.layer_name}>
              {l.layer_name}
            </option>
          ))}
        </select>

        <select
          value={statusFilter}
          onChange={(e) => {
            setStatusFilter(e.target.value)
            setPage(1)
          }}
          className="px-2.5 py-1.5 rounded-[6px] text-xs bg-white dark:bg-[#1e2026] border border-neutral-200 dark:border-[#2e323a] text-neutral-800 dark:text-neutral-200"
        >
          <option value="">All Statuses</option>
          <option value="active">Active</option>
          <option value="georeferenced">Georeferenced</option>
          <option value="unreferenced">Unreferenced</option>
          <option value="paired">Paired</option>
        </select>

        {/* Dynamic Columns Picker */}
        <div className="relative ml-auto">
          <button
            type="button"
            onClick={() => setColumnsMenuOpen((p) => !p)}
            className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-[6px] border border-neutral-200 dark:border-[#2e323a] bg-white dark:bg-[#1e2026] text-xs font-semibold text-neutral-700 dark:text-neutral-200 hover:border-[#3ecf8e] cursor-pointer"
          >
            <Columns3 className="w-3.5 h-3.5 text-[#3ecf8e]" />
            Columns ({activeColumns.length}/{columns.length})
          </button>
          {columnsMenuOpen && (
            <div className="absolute right-0 top-full mt-1 z-40 w-60 max-h-72 overflow-y-auto rounded-[9px] border border-neutral-200 dark:border-[#2e323a] bg-white dark:bg-[#1c1f24] p-2 shadow-2xl space-y-1">
              <div className="px-2 py-1 text-[10px] font-bold uppercase text-neutral-400">
                Toggle Visible Columns
              </div>
              {columns.map((c) => (
                <label
                  key={c.key}
                  className="flex items-center gap-2 px-2 py-1 rounded hover:bg-neutral-100 dark:hover:bg-[#2a2d34] text-xs cursor-pointer"
                >
                  <input
                    type="checkbox"
                    checked={Boolean(visibleCols[c.key])}
                    onChange={(e) =>
                      setVisibleCols((prev) => ({ ...prev, [c.key]: e.target.checked }))
                    }
                    className="accent-[#3ecf8e]"
                  />
                  <span className="truncate flex-1">{c.label}</span>
                  {c.dynamic && (
                    <span className="text-[9px] px-1 rounded bg-emerald-500/15 text-emerald-500">
                      Field
                    </span>
                  )}
                </label>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Main Table + Optional Details Drawer */}
      <div className="flex-1 flex min-h-0 overflow-hidden">
        <div className="flex-1 overflow-auto">
          {error ? (
            <div className="m-6 p-4 rounded-[9px] bg-red-500/10 border border-red-500/30 text-xs text-red-500">
              {error}
            </div>
          ) : loading ? (
            <div className="py-20 flex flex-col items-center justify-center text-xs text-neutral-500 gap-2">
              <Loader2 className="w-6 h-6 animate-spin text-[#3ecf8e]" />
              <span>Loading workspace records…</span>
            </div>
          ) : records.length === 0 ? (
            <div className="py-20 px-6 text-center max-w-md mx-auto">
              <Database className="w-10 h-10 text-neutral-400 mx-auto mb-3 opacity-60" />
              <h4 className="text-sm font-bold text-neutral-900 dark:text-white">
                No records found in this view
              </h4>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 mb-4">
                Upload a GeoPackage, Shapefile, GeoJSON, raster image, or draw features on the map to populate Workspace Records automatically.
              </p>
              {onOpenAddDataset && (
                <button
                  type="button"
                  onClick={onOpenAddDataset}
                  className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-[6px] bg-[#3ecf8e] text-[#171717] text-xs font-bold cursor-pointer"
                >
                  <Plus className="w-3.5 h-3.5" />
                  Add Dataset
                </button>
              )}
            </div>
          ) : (
            <table className="w-full text-left border-collapse text-xs">
              <thead className="sticky top-0 z-10 bg-neutral-100 dark:bg-[#1e2128] border-b border-neutral-200 dark:border-[#2e323a] text-[11px] font-semibold text-neutral-600 dark:text-neutral-300">
                <tr>
                  <th className="w-9 px-3 py-2.5">
                    <input
                      type="checkbox"
                      checked={records.length > 0 && selectedIds.length === records.length}
                      onChange={toggleSelectAll}
                      className="accent-[#3ecf8e] cursor-pointer"
                      aria-label="Select all rows"
                    />
                  </th>
                  {activeColumns.map((col) => (
                    <th key={col.key} className="px-3 py-2.5 whitespace-nowrap">
                      <button
                        type="button"
                        onClick={() => handleSort(col.key)}
                        title={col.tooltip || `Sort by ${col.label}`}
                        className="inline-flex items-center gap-1 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
                      >
                        <span>{col.label}</span>
                        <ArrowUpDown className="w-3 h-3 opacity-50" />
                      </button>
                    </th>
                  ))}
                  <th className="px-3 py-2.5 text-right whitespace-nowrap">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-200/70 dark:divide-[#252830]">
                {records.map((rec) => {
                  const selected = selectedIds.includes(rec.id)
                  return (
                    <tr
                      key={rec.id}
                      className={`transition-colors ${
                        selected
                          ? "bg-[#3ecf8e]/10"
                          : "hover:bg-neutral-100/70 dark:hover:bg-[#1f232b]"
                      }`}
                    >
                      <td className="px-3 py-2">
                        <input
                          type="checkbox"
                          checked={selected}
                          onChange={() => toggleSelectOne(rec.id)}
                          className="accent-[#3ecf8e] cursor-pointer"
                        />
                      </td>
                      {activeColumns.map((col) => (
                        <td
                          key={col.key}
                          className="px-3 py-2 max-w-[220px] truncate text-neutral-800 dark:text-neutral-200"
                          title={formatCellValue(rec, col)}
                        >
                          {col.key === "visibility" ? (
                            <button
                              type="button"
                              onClick={() => handleToggleLayerVis(rec)}
                              className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold cursor-pointer ${
                                rec.visibility
                                  ? "bg-emerald-500/15 text-emerald-500"
                                  : "bg-neutral-200 dark:bg-[#2a2e37] text-neutral-500"
                              }`}
                            >
                              {rec.visibility ? <Eye className="w-3 h-3" /> : <EyeOff className="w-3 h-3" />}
                              {rec.visibility ? "Visible" : "Hidden"}
                            </button>
                          ) : col.key === "status" ? (
                            <span className="px-1.5 py-0.5 rounded bg-neutral-200/70 dark:bg-[#2a2e37] text-[10px] font-semibold uppercase">
                              {rec.status}
                            </span>
                          ) : (
                            formatCellValue(rec, col)
                          )}
                        </td>
                      ))}
                      <td className="px-3 py-2 whitespace-nowrap text-right">
                        <div className="inline-flex items-center gap-1">
                          <button
                            type="button"
                            onClick={() => onLocateRecord?.(rec)}
                            className="p-1 rounded text-emerald-500 hover:bg-emerald-500/15 cursor-pointer"
                            title="Show & zoom to this record on the map"
                          >
                            <Navigation className="w-3.5 h-3.5" />
                          </button>
                          <button
                            type="button"
                            onClick={() => setDetailRecord(rec)}
                            className="p-1 rounded text-neutral-500 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-200 dark:hover:bg-[#2a2d34] cursor-pointer"
                            title="Inspect full attributes and geometry metadata"
                          >
                            <Info className="w-3.5 h-3.5" />
                          </button>
                          {(rec.record_type === "vector" || rec.record_type === "raster") && (
                            <button
                              type="button"
                              onClick={() => openInlineEdit(rec)}
                              className="p-1 rounded text-neutral-500 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-200 dark:hover:bg-[#2a2d34] cursor-pointer"
                              title="Edit record properties"
                            >
                              <Edit2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                          {rec.record_type !== "raster" && (
                            <button
                              type="button"
                              onClick={() => handleDeleteRecord(rec)}
                              className="p-1 rounded text-red-500 hover:bg-red-500/15 cursor-pointer"
                              title="Delete record"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>

        {/* Record Details Side Drawer */}
        {detailRecord && (
          <aside className="w-80 shrink-0 border-l border-neutral-200 dark:border-[#2a2d34] bg-white dark:bg-[#1b1d23] flex flex-col overflow-hidden">
            <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#2a2d34] flex items-center justify-between">
              <div className="min-w-0">
                <h4 className="text-xs font-bold text-neutral-900 dark:text-white truncate">
                  {detailRecord.display_name}
                </h4>
                <p className="text-[10px] text-neutral-500">
                  {detailRecord.layer_name} • {detailRecord.geometry_type}
                </p>
              </div>
              <button
                type="button"
                onClick={() => setDetailRecord(null)}
                className="p-1 text-neutral-400 hover:text-white cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-4 space-y-3 text-xs">
              <dl className="grid grid-cols-2 gap-2 text-[11px]">
                <div>
                  <dt className="text-neutral-400 text-[10px]">Record Type</dt>
                  <dd className="font-semibold uppercase">{detailRecord.record_type}</dd>
                </div>
                <div>
                  <dt className="text-neutral-400 text-[10px]">CRS</dt>
                  <dd className="font-mono">{detailRecord.crs || "—"}</dd>
                </div>
                <div className="col-span-2">
                  <dt className="text-neutral-400 text-[10px]">Coordinates / Bounds</dt>
                  <dd className="font-mono text-[10px] break-all">
                    {formatCellValue(detailRecord, { key: "coordinates" })}
                  </dd>
                </div>
              </dl>

              {detailRecord.attributes && Object.keys(detailRecord.attributes).length > 0 && (
                <div className="pt-2 border-t border-neutral-200 dark:border-[#2a2d34] space-y-1.5">
                  <div className="text-[10px] font-bold uppercase text-neutral-400">
                    Dynamic Attributes
                  </div>
                  {Object.entries(detailRecord.attributes).map(([k, v]) => (
                    <div key={k} className="flex justify-between gap-2 text-[11px] py-0.5 border-b border-neutral-100 dark:border-[#23262e]">
                      <span className="text-neutral-500 truncate">{k}</span>
                      <span className="font-medium text-right break-all">{v === null || v === undefined ? "—" : String(v)}</span>
                    </div>
                  ))}
                </div>
              )}

              <button
                type="button"
                onClick={() => onLocateRecord?.(detailRecord)}
                className="w-full inline-flex items-center justify-center gap-1.5 px-3 py-2 rounded-[6px] bg-[#3ecf8e] text-[#171717] font-bold text-xs cursor-pointer"
              >
                <Navigation className="w-3.5 h-3.5" />
                Zoom to Feature on Map
              </button>
            </div>
          </aside>
        )}
      </div>

      {/* Server-side Pagination Footer */}
      <div className="px-4 py-2.5 border-t border-neutral-200 dark:border-[#2a2d34] bg-white dark:bg-[#1c1f24] flex flex-wrap items-center justify-between gap-3 text-xs">
        <div className="flex items-center gap-2 text-neutral-500">
          <span>
            Page <strong className="text-neutral-900 dark:text-white">{page}</strong> of{" "}
            <strong className="text-neutral-900 dark:text-white">{totalPages}</strong> ({total} records)
          </span>
          <select
            value={pageSize}
            onChange={(e) => {
              setPageSize(Number(e.target.value))
              setPage(1)
            }}
            className="px-2 py-1 rounded bg-neutral-100 dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-[11px]"
          >
            {[25, 50, 100].map((sz) => (
              <option key={sz} value={sz}>
                {sz} / page
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-1.5">
          <button
            type="button"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded border border-neutral-200 dark:border-[#2e323a] disabled:opacity-40 cursor-pointer"
          >
            <ChevronLeft className="w-3.5 h-3.5" /> Prev
          </button>
          <button
            type="button"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded border border-neutral-200 dark:border-[#2e323a] disabled:opacity-40 cursor-pointer"
          >
            Next <ChevronRight className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Inline Record Edit Modal */}
      {editingRecord && (
        <div className="fixed inset-0 z-[1100] bg-black/60 backdrop-blur-xs flex items-center justify-center p-4">
          <form
            onSubmit={handleSaveInlineEdit}
            className="w-full max-w-md rounded-[12px] bg-white dark:bg-[#1c1f24] border border-neutral-200 dark:border-[#2e323a] shadow-2xl overflow-hidden"
          >
            <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#2a2d34] flex items-center justify-between">
              <h4 className="text-sm font-bold">Edit Record Properties</h4>
              <button
                type="button"
                onClick={() => setEditingRecord(null)}
                className="p-1 text-neutral-400 hover:text-white cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <div className="p-4 space-y-3 max-h-[60vh] overflow-y-auto text-xs">
              {Object.entries(editDraft).map(([k, v]) => (
                <div key={k}>
                  <label className="block text-[11px] font-semibold text-neutral-500 mb-1">
                    {k}
                  </label>
                  <input
                    type="text"
                    value={v ?? ""}
                    onChange={(e) => setEditDraft((prev) => ({ ...prev, [k]: e.target.value }))}
                    className="w-full px-3 py-1.5 rounded-[6px] bg-neutral-50 dark:bg-[#14161a] border border-neutral-200 dark:border-[#2e323a] text-neutral-900 dark:text-white"
                  />
                </div>
              ))}
            </div>
            <div className="px-4 py-3 border-t border-neutral-200 dark:border-[#2a2d34] flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setEditingRecord(null)}
                className="px-3 py-1.5 rounded-[6px] text-xs text-neutral-500 cursor-pointer"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={savingEdit}
                className="inline-flex items-center gap-1 px-4 py-1.5 rounded-[6px] bg-[#3ecf8e] text-[#171717] font-bold text-xs cursor-pointer"
              >
                <Check className="w-3.5 h-3.5" />
                {savingEdit ? "Saving…" : "Save Changes"}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  )
}
