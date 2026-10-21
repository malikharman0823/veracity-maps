import React, { useState } from "react"
import {
  Archive,
  Building,
  Check,
  CheckCircle2,
  FileImage,
  Layers,
  Loader2,
  Plus,
  AlertTriangle,
  X,
  ArrowRight,
} from "lucide-react"
import { importArchiveSelected } from "../api"

export default function ZipImportReview({
  inspectionResult,
  onClose,
  onImportComplete,
}) {
  const token = inspectionResult?.archive_token
  const archiveName = inspectionResult?.archive_name || "archive.zip"
  const rawDatasets = inspectionResult?.datasets || []

  const [selections, setSelections] = useState(() =>
    rawDatasets.map((ds, idx) => ({
      selected: ds.status === "ready" || ds.status === "valid" || ds.status === "ambiguous" || !ds.status,
      entry_path: ds.entry_path,
      name: ds.name || `Dataset ${idx + 1}`,
      format: ds.format,
      width: ds.width,
      height: ds.height,
      is_georeferenced: ds.is_georeferenced,
      sidecars: ds.sidecars || [],
      building_name: ds.suggested_building || "Building 1",
      floor_level: ds.suggested_floor ?? 0,
      floor_label: ds.suggested_floor_label || (ds.suggested_floor === 0 ? "Ground Floor" : `Floor ${ds.suggested_floor ?? 0}`),
      warnings: ds.warnings || [],
    }))
  )

  const [importing, setImporting] = useState(false)
  const [importResult, setImportResult] = useState(null)
  const [customBuildingInput, setCustomBuildingInput] = useState("")
  const [showAddBuilding, setShowAddBuilding] = useState(false)
  const [error, setError] = useState(null)

  const selectedCount = selections.filter((s) => s.selected).length

  // Unique building names from current rows
  const existingBuildings = Array.from(
    new Set(selections.map((s) => s.building_name).filter(Boolean))
  )

  function updateRow(index, patch) {
    setSelections((prev) => {
      const next = [...prev]
      next[index] = { ...next[index], ...patch }
      return next
    })
  }

  function toggleAll(selectAll) {
    setSelections((prev) => prev.map((s) => ({ ...s, selected: selectAll })))
  }

  function handleAddBuildingSubmit() {
    if (!customBuildingInput.trim()) return
    const newBld = customBuildingInput.trim()
    setShowAddBuilding(false)
    setCustomBuildingInput("")
    // assign to first selected row that doesn't have it
    setSelections((prev) =>
      prev.map((s) => (s.selected ? { ...s, building_name: newBld } : s))
    )
  }

  async function handleImport() {
    if (!selectedCount || !token) return
    setImporting(true)
    setError(null)
    try {
      const itemsToImport = selections
        .filter((s) => s.selected)
        .map((s) => ({
          entry_path: s.entry_path,
          name: (s.name || "").trim(),
          building_name: (s.building_name || "").trim(),
          floor_level: Number(s.floor_level || 0),
          floor_label: (s.floor_label || "").trim(),
        }))

      const result = await importArchiveSelected(token, itemsToImport)
      setImportResult(result)
    } catch (err) {
      console.error("Multi-dataset import error:", err)
      setError(
        err.response?.data?.error?.message ||
          err.message ||
          "Failed to import selected datasets."
      )
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-xs">
      <div
        className="bg-white rounded-2xl shadow-2xl border border-gray-200 w-full max-w-5xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
        role="dialog"
        aria-modal="true"
        aria-label="Multi-Dataset ZIP Inspection & Import"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 bg-gray-50/70">
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-blue-50 text-blue-600 rounded-xl">
              <Archive className="size-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-base font-semibold text-gray-900">
                  Review & Import Archive Datasets
                </h2>
                <span className="px-2 py-0.5 rounded-full text-[11px] font-mono bg-blue-100 text-blue-700 font-medium">
                  {rawDatasets.length} Discovered
                </span>
              </div>
              <p className="text-xs text-gray-500 mt-0.5">
                Source ZIP: <span className="font-mono text-gray-700">{archiveName}</span>
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-gray-400 hover:text-gray-700 hover:bg-gray-200/60 rounded-lg transition-colors"
            aria-label="Close review dialog"
          >
            <X className="size-5" />
          </button>
        </div>

        {/* Import Results View */}
        {importResult ? (
          <div className="p-8 flex flex-col items-center justify-center text-center space-y-4">
            <div className="size-14 rounded-full bg-emerald-50 text-emerald-600 flex items-center justify-center shadow-xs">
              <CheckCircle2 className="size-8" />
            </div>
            <div className="space-y-1">
              <h3 className="text-lg font-semibold text-gray-900">
                Import Completed Successfully
              </h3>
              <p className="text-xs text-gray-500 max-w-md">
                Successfully imported <strong>{importResult.imported_count}</strong> dataset(s) into your session.
                {importResult.failed_count > 0 && (
                  <span className="text-amber-600 ml-1">
                    ({importResult.failed_count} failed)
                  </span>
                )}
              </p>
            </div>

            <div className="w-full max-w-lg bg-gray-50 rounded-xl border border-gray-200 p-4 divide-y divide-gray-100 text-left max-h-60 overflow-y-auto text-xs">
              {importResult.results?.map((res, i) => (
                <div key={i} className="py-2 first:pt-0 last:pb-0 flex items-center justify-between">
                  <div className="flex items-center gap-2 truncate pr-2">
                    <FileImage className="size-4 text-gray-400 shrink-0" />
                    <span className="font-medium text-gray-800 truncate">{res.name}</span>
                  </div>
                  <span
                    className={`px-2 py-0.5 rounded-full text-[10px] font-medium shrink-0 ${
                      res.status === "success"
                        ? "bg-emerald-100 text-emerald-800"
                        : "bg-red-100 text-red-800"
                    }`}
                  >
                    {res.status === "success" ? "Imported" : "Failed"}
                  </span>
                </div>
              ))}
            </div>

            <button
              type="button"
              onClick={() => {
                if (onImportComplete) onImportComplete(importResult)
              }}
              className="px-6 py-2.5 bg-blue-600 hover:bg-blue-700 text-white font-medium text-xs rounded-xl shadow-xs transition-colors flex items-center gap-2"
            >
              <span>Open in Workspace</span>
              <ArrowRight className="size-4" />
            </button>
          </div>
        ) : (
          <>
            {/* Action Bar */}
            <div className="px-6 py-3 border-b border-gray-100 bg-white flex flex-wrap items-center justify-between gap-3 text-xs">
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => toggleAll(true)}
                  className="px-2.5 py-1 text-gray-600 bg-gray-100 hover:bg-gray-200 rounded-md font-medium transition-colors"
                >
                  Select All
                </button>
                <button
                  type="button"
                  onClick={() => toggleAll(false)}
                  className="px-2.5 py-1 text-gray-600 bg-gray-100 hover:bg-gray-200 rounded-md font-medium transition-colors"
                >
                  Deselect All
                </button>
                <span className="text-gray-400">|</span>
                <span className="text-gray-600">
                  Selected: <strong className="text-gray-900">{selectedCount}</strong> of {selections.length}
                </span>
              </div>

              <div className="flex items-center gap-2">
                {showAddBuilding ? (
                  <div className="flex items-center gap-1.5 animate-in fade-in duration-100">
                    <input
                      type="text"
                      value={customBuildingInput}
                      onChange={(e) => setCustomBuildingInput(e.target.value)}
                      placeholder="Building name…"
                      className="px-2 py-1 bg-gray-50 border border-gray-300 rounded-md text-xs focus:outline-none focus:ring-1 focus:ring-blue-500"
                      autoFocus
                    />
                    <button
                      type="button"
                      onClick={handleAddBuildingSubmit}
                      className="px-2 py-1 bg-blue-600 text-white rounded-md font-medium text-xs hover:bg-blue-700"
                    >
                      Assign
                    </button>
                    <button
                      type="button"
                      onClick={() => setShowAddBuilding(false)}
                      className="p-1 text-gray-400 hover:text-gray-600"
                    >
                      <X className="size-3.5" />
                    </button>
                  </div>
                ) : (
                  <button
                    type="button"
                    onClick={() => setShowAddBuilding(true)}
                    className="px-2.5 py-1 text-blue-600 bg-blue-50 hover:bg-blue-100 rounded-md font-medium transition-colors flex items-center gap-1"
                  >
                    <Plus className="size-3.5" />
                    <span>New Building</span>
                  </button>
                )}
              </div>
            </div>

            {error && (
              <div className="mx-6 mt-3 px-3.5 py-2.5 bg-red-50 border border-red-200 rounded-xl text-xs text-red-700 flex items-center gap-2">
                <AlertTriangle className="size-4 shrink-0 text-red-500" />
                <span>{error}</span>
              </div>
            )}

            {/* Datasets Table */}
            <div className="flex-1 overflow-y-auto p-6 min-h-[300px]">
              <div className="border border-gray-200 rounded-xl overflow-hidden shadow-2xs">
                <table className="w-full text-left text-xs divide-y divide-gray-200">
                  <thead className="bg-gray-50/80 text-gray-600 font-semibold uppercase text-[10px] tracking-wider">
                    <tr>
                      <th className="py-2.5 pl-4 pr-2 w-10 text-center">
                        <input
                          type="checkbox"
                          checked={selectedCount === selections.length && selections.length > 0}
                          onChange={(e) => toggleAll(e.target.checked)}
                          className="rounded border-gray-300 text-blue-600 focus:ring-blue-500 size-3.5"
                        />
                      </th>
                      <th className="py-2.5 px-3">Dataset Name</th>
                      <th className="py-2.5 px-3">Archive Path</th>
                      <th className="py-2.5 px-3">Format / Size</th>
                      <th className="py-2.5 px-3">Georeferencing</th>
                      <th className="py-2.5 px-3">Building</th>
                      <th className="py-2.5 px-3">Floor Plan</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100 bg-white">
                    {selections.map((row, idx) => (
                      <tr
                        key={row.entry_path}
                        className={`transition-colors hover:bg-gray-50/70 ${
                          row.selected ? "bg-blue-50/20" : "opacity-60"
                        }`}
                      >
                        <td className="py-3 pl-4 pr-2 text-center">
                          <input
                            type="checkbox"
                            checked={row.selected}
                            onChange={(e) => updateRow(idx, { selected: e.target.checked })}
                            className="rounded border-gray-300 text-blue-600 focus:ring-blue-500 size-3.5 cursor-pointer"
                          />
                        </td>

                        {/* Editable Name */}
                        <td className="py-3 px-3">
                          <input
                            type="text"
                            value={row.name}
                            onChange={(e) => updateRow(idx, { name: e.target.value })}
                            className="w-full px-2 py-1 bg-white border border-gray-200 rounded-md font-medium text-gray-900 focus:outline-none focus:ring-1 focus:ring-blue-500"
                          />
                        </td>

                        {/* Archive Entry Path */}
                        <td className="py-3 px-3 text-gray-500 font-mono text-[11px] truncate max-w-[180px]" title={row.entry_path}>
                          {row.entry_path}
                        </td>

                        {/* Format & Dimensions */}
                        <td className="py-3 px-3 text-gray-600">
                          <span className="font-semibold uppercase text-gray-800">
                            {row.format}
                          </span>
                          {row.width > 0 && row.height > 0 && (
                            <span className="text-gray-400 ml-1 text-[11px]">
                              {row.width}×{row.height}
                            </span>
                          )}
                        </td>

                        {/* Georeference status */}
                        <td className="py-3 px-3">
                          {row.is_georeferenced ? (
                            <div className="flex items-center gap-1.5 text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded-full border border-emerald-200 text-[10px] font-semibold w-fit">
                              <Check className="size-3" />
                              <span>Georeferenced</span>
                            </div>
                          ) : (
                            <span className="text-gray-500 bg-gray-100 px-2 py-0.5 rounded-full text-[10px] font-medium">
                              Not referenced
                            </span>
                          )}
                          {row.sidecars?.length > 0 && (
                            <div className="text-[10px] text-gray-400 mt-0.5">
                              Sidecar: {row.sidecars.join(", ")}
                            </div>
                          )}
                        </td>

                        {/* Building Assignment */}
                        <td className="py-3 px-3">
                          <div className="flex items-center gap-1.5">
                            <Building className="size-3.5 text-gray-400 shrink-0" />
                            <select
                              value={row.building_name}
                              onChange={(e) => updateRow(idx, { building_name: e.target.value })}
                              className="px-2 py-1 bg-white border border-gray-200 rounded-md text-xs text-gray-800 focus:outline-none focus:ring-1 focus:ring-blue-500"
                            >
                              {existingBuildings.map((bld) => (
                                <option key={bld} value={bld}>
                                  {bld}
                                </option>
                              ))}
                              {!existingBuildings.includes(row.building_name) && (
                                <option value={row.building_name}>{row.building_name}</option>
                              )}
                            </select>
                          </div>
                        </td>

                        {/* Floor Assignment */}
                        <td className="py-3 px-3">
                          <div className="flex items-center gap-1.5">
                            <Layers className="size-3.5 text-gray-400 shrink-0" />
                            <select
                              value={row.floor_level}
                              onChange={(e) => {
                                const lvl = Number(e.target.value)
                                const label =
                                  lvl === -2
                                    ? "Basement 2"
                                    : lvl === -1
                                    ? "Basement 1"
                                    : lvl === 0
                                    ? "Ground Floor"
                                    : `Floor ${lvl}`
                                updateRow(idx, { floor_level: lvl, floor_label: label })
                              }}
                              className="px-2 py-1 bg-white border border-gray-200 rounded-md text-xs text-gray-800 focus:outline-none focus:ring-1 focus:ring-blue-500"
                            >
                              <option value={-2}>Basement 2 (-2)</option>
                              <option value={-1}>Basement 1 (-1)</option>
                              <option value={0}>Ground Floor (0)</option>
                              <option value={1}>Floor 1 (1)</option>
                              <option value={2}>Floor 2 (2)</option>
                              <option value={3}>Floor 3 (3)</option>
                              <option value={4}>Floor 4 (4)</option>
                            </select>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between px-6 py-4 border-t border-gray-100 bg-gray-50/70">
              <span className="text-xs text-gray-500">
                Each imported dataset becomes an independent layer in the session workspace.
              </span>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={onClose}
                  disabled={importing}
                  className="px-4 py-2 text-xs font-medium text-gray-700 hover:bg-gray-200/60 rounded-xl transition-colors border border-gray-200 disabled:opacity-50"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={selectedCount === 0 || importing}
                  onClick={handleImport}
                  className="px-5 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:pointer-events-none rounded-xl transition-colors shadow-xs flex items-center gap-2"
                >
                  {importing ? (
                    <>
                      <Loader2 className="size-3.5 animate-spin" />
                      <span>Importing {selectedCount} Dataset(s)…</span>
                    </>
                  ) : (
                    <>
                      <span>Import {selectedCount} Selected Dataset(s)</span>
                      <ArrowRight className="size-3.5" />
                    </>
                  )}
                </button>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
