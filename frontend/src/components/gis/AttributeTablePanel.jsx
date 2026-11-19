import React, { useEffect, useState } from "react"
import {
  Table2,
  Search,
  Filter,
  Plus,
  Trash2,
  Calculator,
  Download,
  ChevronLeft,
  ChevronRight,
  ArrowUpDown,
  X,
  Check,
  Navigation,
} from "lucide-react"

import {
  filterLayerAttributes,
  updateFeatureAttributes,
  addLayerField,
  deleteLayerField,
  calculateLayerField,
  exportLayerAttributes,
} from "../../api"

const OPERATORS = ["=", "!=", ">", "<", ">=", "<=", "LIKE", "IN", "IS NULL", "IS NOT NULL"]

export default function AttributeTablePanel({
  layer,
  selectedFeatureIds = [],
  onSelectFeatures,
  onZoomToFeature,
  onFeaturesMutated,
  onClose,
  notify,
}) {
  const [height, setHeight] = useState(290)
  const [fields, setFields] = useState([])
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(25)
  const [search, setSearch] = useState("")
  const [sortBy, setSortBy] = useState("feature_id")
  const [sortOrder, setSortOrder] = useState("asc")
  const [loading, setLoading] = useState(false)

  // Filter builder state
  const [showFilters, setShowFilters] = useState(false)
  const [filterField, setFilterField] = useState("")
  const [filterOp, setFilterOp] = useState("=")
  const [filterVal, setFilterVal] = useState("")
  const [activeFilters, setActiveFilters] = useState([])

  // Add field modal/row state
  const [showAddField, setShowAddField] = useState(false)
  const [newFieldName, setNewFieldName] = useState("")
  const [newFieldType, setNewFieldType] = useState("string")

  // Field calculator state
  const [showCalculator, setShowCalculator] = useState(false)
  const [calcTargetField, setCalcTargetField] = useState("")
  const [calcExpression, setCalcExpression] = useState("")

  // Inline cell editing
  const [editingCell, setEditingCell] = useState(null) // { featureId, fieldName }
  const [editingValue, setEditingValue] = useState("")

  const loadData = async () => {
    if (!layer?.layer_id) return
    setLoading(true)
    try {
      const data = await filterLayerAttributes(layer.layer_id, {
        filters: activeFilters,
        search,
        page,
        page_size: pageSize,
        sort_by: sortBy,
        sort_order: sortOrder,
      })
      setFields(data?.fields || [])
      setRows(data?.rows || [])
      setTotal(data?.total || 0)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed loading attribute table", "error")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [layer?.layer_id, page, pageSize, sortBy, sortOrder, activeFilters])

  if (!layer) return null

  const allFieldNames = Array.from(
    new Set([
      ...fields.map((f) => f.field_name),
      ...rows.flatMap((r) => Object.keys(r.properties || {})),
    ])
  )

  const handleSort = (col) => {
    if (sortBy === col) {
      setSortOrder((prev) => (prev === "asc" ? "desc" : "asc"))
    } else {
      setSortBy(col)
      setSortOrder("asc")
    }
  }

  const handleSaveInlineCell = async (featureId, fieldName) => {
    setEditingCell(null)
    try {
      let parsedVal = editingValue
      const fldDef = fields.find((f) => f.field_name === fieldName)
      if (fldDef?.field_type === "integer") {
        parsedVal = parseInt(editingValue, 10)
        if (Number.isNaN(parsedVal)) parsedVal = 0
      } else if (fldDef?.field_type === "float") {
        parsedVal = parseFloat(editingValue)
        if (Number.isNaN(parsedVal)) parsedVal = 0.0
      } else if (fldDef?.field_type === "boolean") {
        parsedVal = editingValue === "true" || editingValue === "1"
      }
      await updateFeatureAttributes(featureId, { [fieldName]: parsedVal })
      loadData()
      onFeaturesMutated?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed updating attribute", "error")
    }
  }

  const handleAddField = async (e) => {
    e.preventDefault()
    if (!newFieldName.trim()) return
    try {
      await addLayerField(layer.layer_id, {
        field_name: newFieldName.trim(),
        field_type: newFieldType,
      })
      setNewFieldName("")
      setShowAddField(false)
      notify?.(`Added field "${newFieldName.trim()}"`)
      loadData()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to add field", "error")
    }
  }

  const handleDeleteField = async (fld) => {
    try {
      await deleteLayerField(layer.layer_id, fld.field_id || fld.field_name)
      notify?.(`Deleted field "${fld.field_name}"`)
      loadData()
      onFeaturesMutated?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to delete field", "error")
    }
  }

  const handleRunCalculator = async (e) => {
    e.preventDefault()
    if (!calcTargetField.trim() || !calcExpression.trim()) return
    try {
      const res = await calculateLayerField(layer.layer_id, {
        target_field: calcTargetField.trim(),
        expression: calcExpression.trim(),
        feature_ids: selectedFeatureIds,
      })
      notify?.(`Calculated "${calcTargetField}" across ${res.updated_count} feature(s)`)
      setShowCalculator(false)
      loadData()
      onFeaturesMutated?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Field calculation failed", "error")
    }
  }

  const handleExport = async (format) => {
    try {
      const blob = await exportLayerAttributes(layer.layer_id, {
        format,
        featureIds: selectedFeatureIds,
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = `${layer.layer_name.replace(/\s+/g, "_")}_attributes.${format}`
      a.click()
      URL.revokeObjectURL(url)
      notify?.(`Exported attributes as ${format.toUpperCase()}`)
    } catch (err) {
      notify?.("Failed to export attributes", "error")
    }
  }

  const toggleRowSelection = (featureId) => {
    const exists = selectedFeatureIds.includes(featureId)
    const next = exists
      ? selectedFeatureIds.filter((id) => id !== featureId)
      : [...selectedFeatureIds, featureId]
    onSelectFeatures?.(next)
  }

  const startResize = (e) => {
    e.preventDefault()
    const startY = e.clientY
    const startH = height
    const onMove = (mv) => {
      const delta = startY - mv.clientY
      setHeight(Math.max(180, Math.min(620, startH + delta)))
    }
    const onUp = () => {
      window.removeEventListener("pointermove", onMove)
      window.removeEventListener("pointerup", onUp)
    }
    window.addEventListener("pointermove", onMove)
    window.addEventListener("pointerup", onUp)
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div
      style={{ height }}
      className="absolute bottom-0 left-0 right-0 w-full bg-white dark:bg-[#161616] border-t border-neutral-200 dark:border-[#2c2c2c] flex flex-col z-40 shadow-2xl"
    >
      {/* Drag Resize Handle */}
      <div
        onPointerDown={startResize}
        className="h-1.5 w-full bg-neutral-200/60 dark:bg-[#242424] hover:bg-[#3ecf8e] cursor-ns-resize transition-colors shrink-0"
        title="Drag to resize Attribute Table"
      />

      {/* Toolbar Header */}
      <div className="px-3 py-1.5 border-b border-neutral-200 dark:border-[#282828] flex flex-wrap items-center justify-between gap-2 text-xs shrink-0">
        <div className="flex items-center gap-2">
          <Table2 className="size-4 text-[#3ecf8e]" />
          <strong className="text-neutral-900 dark:text-white">{layer.layer_name} — Attributes</strong>
          <span className="text-[11px] text-neutral-500 font-mono">
            ({total} records{selectedFeatureIds.length > 0 ? `, ${selectedFeatureIds.length} selected` : ""})
          </span>
        </div>

        <div className="flex flex-wrap items-center gap-1.5">
          <div className="flex items-center bg-neutral-100 dark:bg-[#222] rounded-[6px] px-2 py-1 border border-neutral-200 dark:border-[#303030]">
            <Search className="size-3.5 text-neutral-400 mr-1.5" />
            <input
              type="text"
              placeholder="Search records…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && loadData()}
              className="bg-transparent outline-none text-xs w-36 text-neutral-900 dark:text-white"
            />
          </div>

          <button
            type="button"
            onClick={() => setShowFilters((p) => !p)}
            className={`inline-flex items-center gap-1 px-2 py-1 rounded-[6px] border text-[11px] font-medium ${
              activeFilters.length > 0
                ? "border-[#3ecf8e] bg-[#3ecf8e]/15 text-[#3ecf8e]"
                : "border-neutral-200 dark:border-[#303030] text-neutral-700 dark:text-neutral-300"
            }`}
          >
            <Filter className="size-3" />
            <span>Filter {activeFilters.length > 0 ? `(${activeFilters.length})` : ""}</span>
          </button>

          <button
            type="button"
            onClick={() => {
              setShowAddField((p) => !p)
              setShowCalculator(false)
            }}
            className="inline-flex items-center gap-1 px-2 py-1 rounded-[6px] border border-neutral-200 dark:border-[#303030] text-[11px] font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#252525]"
          >
            <Plus className="size-3" />
            <span>Add Field</span>
          </button>

          <button
            type="button"
            onClick={() => {
              setShowCalculator((p) => !p)
              setShowAddField(false)
            }}
            className="inline-flex items-center gap-1 px-2 py-1 rounded-[6px] border border-neutral-200 dark:border-[#303030] text-[11px] font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#252525]"
          >
            <Calculator className="size-3" />
            <span>Calculate</span>
          </button>

          <button
            type="button"
            onClick={() => handleExport("csv")}
            className="inline-flex items-center gap-1 px-2 py-1 rounded-[6px] border border-neutral-200 dark:border-[#303030] text-[11px] font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#252525]"
            title="Export CSV"
          >
            <Download className="size-3" />
            <span>CSV</span>
          </button>

          <button
            type="button"
            onClick={() => handleExport("geojson")}
            className="inline-flex items-center gap-1 px-2 py-1 rounded-[6px] border border-neutral-200 dark:border-[#303030] text-[11px] font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#252525]"
            title="Export GeoJSON"
          >
            <Download className="size-3" />
            <span>GeoJSON</span>
          </button>

          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-neutral-100 dark:hover:bg-[#282828] text-neutral-400 hover:text-white"
            title="Close Attribute Table"
          >
            <X className="size-4" />
          </button>
        </div>
      </div>

      {/* Sub-bars for Filter / Add Field / Field Calculator */}
      {showFilters && (
        <div className="px-3 py-1.5 bg-neutral-50 dark:bg-[#1c1c1c] border-b border-neutral-200 dark:border-[#282828] flex flex-wrap items-center gap-2 text-xs">
          <select
            value={filterField}
            onChange={(e) => setFilterField(e.target.value)}
            className="px-2 py-1 rounded bg-white dark:bg-[#141414] border border-neutral-300 dark:border-[#333]"
          >
            <option value="">Select field…</option>
            {allFieldNames.map((fn) => (
              <option key={fn} value={fn}>{fn}</option>
            ))}
          </select>
          <select
            value={filterOp}
            onChange={(e) => setFilterOp(e.target.value)}
            className="px-2 py-1 rounded bg-white dark:bg-[#141414] border border-neutral-300 dark:border-[#333]"
          >
            {OPERATORS.map((op) => (
              <option key={op} value={op}>{op}</option>
            ))}
          </select>
          {filterOp !== "IS NULL" && filterOp !== "IS NOT NULL" && (
            <input
              type="text"
              placeholder="Value…"
              value={filterVal}
              onChange={(e) => setFilterVal(e.target.value)}
              className="px-2 py-1 rounded bg-white dark:bg-[#141414] border border-neutral-300 dark:border-[#333]"
            />
          )}
          <button
            type="button"
            onClick={() => {
              if (!filterField) return
              setActiveFilters((prev) => [...prev, { field: filterField, operator: filterOp, value: filterVal }])
              setFilterVal("")
            }}
            className="px-2.5 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold"
          >
            Add Filter
          </button>
          {activeFilters.map((af, idx) => (
            <span
              key={idx}
              className="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-[#3ecf8e]/15 text-[#3ecf8e] font-mono text-[11px]"
            >
              {af.field} {af.operator} {String(af.value ?? "")}
              <button
                type="button"
                onClick={() => setActiveFilters((prev) => prev.filter((_, i) => i !== idx))}
              >
                <X className="size-3" />
              </button>
            </span>
          ))}
        </div>
      )}

      {showAddField && (
        <form onSubmit={handleAddField} className="px-3 py-1.5 bg-neutral-50 dark:bg-[#1c1c1c] border-b border-neutral-200 dark:border-[#282828] flex items-center gap-2 text-xs">
          <span className="font-semibold">New Field:</span>
          <input
            type="text"
            placeholder="field_name (alphanumeric)"
            value={newFieldName}
            onChange={(e) => setNewFieldName(e.target.value)}
            className="px-2 py-1 rounded bg-white dark:bg-[#141414] border border-neutral-300 dark:border-[#333]"
            required
          />
          <select
            value={newFieldType}
            onChange={(e) => setNewFieldType(e.target.value)}
            className="px-2 py-1 rounded bg-white dark:bg-[#141414] border border-neutral-300 dark:border-[#333]"
          >
            <option value="string">String</option>
            <option value="integer">Integer</option>
            <option value="float">Float</option>
            <option value="boolean">Boolean</option>
            <option value="date">Date</option>
          </select>
          <button type="submit" className="px-2.5 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold">
            Create Field
          </button>
        </form>
      )}

      {showCalculator && (
        <form onSubmit={handleRunCalculator} className="px-3 py-1.5 bg-neutral-50 dark:bg-[#1c1c1c] border-b border-neutral-200 dark:border-[#282828] flex flex-wrap items-center gap-2 text-xs">
          <span className="font-semibold">Field Calculator:</span>
          <input
            type="text"
            placeholder="Target field (e.g. area_calc)"
            value={calcTargetField}
            onChange={(e) => setCalcTargetField(e.target.value)}
            className="px-2 py-1 rounded bg-white dark:bg-[#141414] border border-neutral-300 dark:border-[#333] w-36"
            required
          />
          <span>=</span>
          <input
            type="text"
            placeholder="Expression (e.g. $area, $length, value * 2, upper(name))"
            value={calcExpression}
            onChange={(e) => setCalcExpression(e.target.value)}
            className="px-2 py-1 rounded bg-white dark:bg-[#141414] border border-neutral-300 dark:border-[#333] w-72 font-mono"
            required
          />
          <button type="submit" className="px-2.5 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold">
            Run Calculation
          </button>
        </form>
      )}

      {/* Spreadsheet Grid */}
      <div className="flex-1 overflow-auto">
        <table className="w-full border-collapse text-xs">
          <thead className="sticky top-0 bg-neutral-100 dark:bg-[#202020] text-neutral-600 dark:text-neutral-300 border-b border-neutral-200 dark:border-[#2e2e2e] z-10">
            <tr>
              <th className="p-2 w-8 text-center">
                <input
                  type="checkbox"
                  checked={rows.length > 0 && rows.every((r) => selectedFeatureIds.includes(r.feature_id))}
                  onChange={(e) => {
                    if (e.target.checked) {
                      onSelectFeatures?.(rows.map((r) => r.feature_id))
                    } else {
                      onSelectFeatures?.([])
                    }
                  }}
                />
              </th>
              <th className="p-2 text-left font-mono text-[11px] cursor-pointer" onClick={() => handleSort("feature_id")}>
                <div className="inline-flex items-center gap-1">
                  <span>Feature ID</span>
                  <ArrowUpDown className="size-3 opacity-60" />
                </div>
              </th>
              <th className="p-2 text-left font-mono text-[11px]">Geometry</th>
              {allFieldNames.map((fn) => {
                const fldObj = fields.find((f) => f.field_name === fn)
                return (
                  <th key={fn} className="p-2 text-left font-mono text-[11px]">
                    <div className="flex items-center justify-between gap-1">
                      <button
                        type="button"
                        onClick={() => handleSort(fn)}
                        className="inline-flex items-center gap-1 hover:text-[#3ecf8e]"
                      >
                        <span>{fn}</span>
                        <ArrowUpDown className="size-3 opacity-60" />
                      </button>
                      {fldObj && (
                        <button
                          type="button"
                          onClick={() => handleDeleteField(fldObj)}
                          className="text-neutral-400 hover:text-red-500"
                          title={`Delete field ${fn}`}
                        >
                          <Trash2 className="size-3" />
                        </button>
                      )}
                    </div>
                  </th>
                )
              })}
              <th className="p-2 text-right">Locate</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-200 dark:divide-[#262626]">
            {rows.map((row) => {
              const isSelected = selectedFeatureIds.includes(row.feature_id)
              return (
                <tr
                  key={row.feature_id}
                  onClick={() => toggleRowSelection(row.feature_id)}
                  className={`cursor-pointer transition-colors ${
                    isSelected
                      ? "bg-[#3ecf8e]/15 dark:bg-[#3ecf8e]/15"
                      : "hover:bg-neutral-50 dark:hover:bg-[#1e1e1e]"
                  }`}
                >
                  <td className="p-2 text-center" onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => toggleRowSelection(row.feature_id)}
                    />
                  </td>
                  <td className="p-2 font-mono text-[11px] text-neutral-500">{row.feature_id}</td>
                  <td className="p-2 font-mono text-[11px] text-sky-400">{row.geometry_type}</td>
                  {allFieldNames.map((fn) => {
                    const isEditing = editingCell?.featureId === row.feature_id && editingCell?.fieldName === fn
                    const cellVal = row.properties?.[fn]
                    return (
                      <td
                        key={fn}
                        className="p-2 text-neutral-900 dark:text-neutral-100"
                        onDoubleClick={(e) => {
                          e.stopPropagation()
                          setEditingCell({ featureId: row.feature_id, fieldName: fn })
                          setEditingValue(cellVal === null || cellVal === undefined ? "" : String(cellVal))
                        }}
                      >
                        {isEditing ? (
                          <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
                            <input
                              type="text"
                              value={editingValue}
                              onChange={(e) => setEditingValue(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") handleSaveInlineCell(row.feature_id, fn)
                                if (e.key === "Escape") setEditingCell(null)
                              }}
                              className="px-1.5 py-0.5 rounded bg-white dark:bg-[#262626] border border-[#3ecf8e] text-xs outline-none w-28"
                              autoFocus
                            />
                            <button
                              type="button"
                              onClick={() => handleSaveInlineCell(row.feature_id, fn)}
                              className="text-[#3ecf8e]"
                            >
                              <Check className="size-3.5" />
                            </button>
                          </div>
                        ) : (
                          <span title="Double-click to edit cell">
                            {cellVal === null || cellVal === undefined ? "—" : String(cellVal)}
                          </span>
                        )}
                      </td>
                    )
                  })}
                  <td className="p-2 text-right" onClick={(e) => e.stopPropagation()}>
                    <button
                      type="button"
                      onClick={() => onZoomToFeature?.(row)}
                      className="p-1 rounded hover:bg-neutral-200 dark:hover:bg-[#2c2c2c] text-[#3ecf8e]"
                      title="Zoom to feature on map"
                    >
                      <Navigation className="size-3.5" />
                    </button>
                  </td>
                </tr>
              )
            })}
            {rows.length === 0 && !loading && (
              <tr>
                <td colSpan={allFieldNames.length + 4} className="p-6 text-center text-neutral-400">
                  No features found in this layer. Use the Vector Editing toolbar on the map to draw points, lines, or polygons.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination Footer */}
      <div className="px-3 py-1.5 border-t border-neutral-200 dark:border-[#282828] flex items-center justify-between text-xs text-neutral-500 shrink-0">
        <div>
          Page <strong>{page}</strong> of <strong>{totalPages}</strong> • Double-click any cell to edit inline
        </div>
        <div className="flex items-center gap-2">
          <select
            value={pageSize}
            onChange={(e) => {
              setPageSize(Number(e.target.value))
              setPage(1)
            }}
            className="px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-[#222] border border-neutral-200 dark:border-[#333]"
          >
            <option value={10}>10 / page</option>
            <option value={25}>25 / page</option>
            <option value={50}>50 / page</option>
            <option value={100}>100 / page</option>
          </select>
          <button
            type="button"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className="p-1 rounded border border-neutral-200 dark:border-[#333] disabled:opacity-40"
          >
            <ChevronLeft className="size-3.5" />
          </button>
          <button
            type="button"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            className="p-1 rounded border border-neutral-200 dark:border-[#333] disabled:opacity-40"
          >
            <ChevronRight className="size-3.5" />
          </button>
        </div>
      </div>
    </div>
  )
}
