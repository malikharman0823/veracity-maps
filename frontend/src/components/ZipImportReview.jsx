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
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-xs">
      <div
        className="bg-white dark:bg-[#202020] rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] w-full max-w-5xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150 text-neutral-900 dark:text-white"
        role="dialog"
        aria-modal="true"
        aria-label="Multi-Dataset ZIP Inspection & Import"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50/80 dark:bg-[#171717]/80">
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] rounded-[8px] border border-[#3ecf8e]/30">
              <Archive className="size-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-base font-semibold text-neutral-900 dark:text-white">
                  Review & Import Archive Datasets
                </h2>
                <span className="px-2 py-0.5 rounded-[4px] text-[11px] font-mono bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/20 font-medium">
                  {rawDatasets.length} Discovered
                </span>
              </div>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                Source ZIP: <span className="font-mono text-neutral-800 dark:text-neutral-200">{archiveName}</span>
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-200/60 dark:hover:bg-[#282828] rounded-[6px] transition-colors cursor-pointer"
            aria-label="Close review dialog"
          >
            <X className="size-5" />
          </button>
        </div>

        {/* Import Results View */}
        {importResult ? (
          <div className="p-8 flex flex-col items-center justify-center text-center space-y-4">
            <div className="size-14 rounded-full bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] flex items-center justify-center shadow-xs border border-[#3ecf8e]/30">
              <CheckCircle2 className="size-8" />
            </div>
            <div className="space-y-1">
              <h3 className="text-lg font-semibold text-neutral-900 dark:text-white">
                Import Completed Successfully
              </h3>
              <p className="text-xs text-neutral-500 dark:text-neutral-400 max-w-md">
                Successfully imported <strong>{importResult.imported_count}</strong> dataset(s) into your session.
                {importResult.failed_count > 0 && (
                  <span className="text-amber-500 ml-1">
                    ({importResult.failed_count} failed)
                  </span>
                )}
              </p>
            </div>

            <div className="w-full max-w-lg bg-neutral-50 dark:bg-[#171717] rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] p-4 divide-y divide-neutral-200 dark:divide-[#2e2e2e] text-left max-h-60 overflow-y-auto text-xs">
              {importResult.results?.map((res, i) => (
                <div key={i} className="py-2 first:pt-0 last:pb-0 flex items-center justify-between">
                  <div className="flex items-center gap-2 truncate pr-2">
                    <FileImage className="size-4 text-neutral-400 shrink-0" />
                    <span className="font-medium text-neutral-800 dark:text-neutral-200 truncate">{res.name}</span>
                  </div>
                  <span
                    className={`px-2 py-0.5 rounded-[4px] text-[10px] font-medium shrink-0 ${
                      res.status === "success" || res.status === "imported"
                        ? "bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] border border-[#3ecf8e]/30"
                        : "bg-red-500/15 text-red-600 dark:text-red-400 border border-red-500/30"
                    }`}
                  >
                    {res.status === "success" || res.status === "imported" ? "Imported" : "Failed"}
                  </span>
                </div>
              ))}
            </div>

            <button
              type="button"
              onClick={() => {
                if (onImportComplete) onImportComplete(importResult)
              }}
              className="px-6 py-2.5 bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs rounded-[6px] shadow-xs transition-colors flex items-center gap-2 cursor-pointer"
            >
              <span>Open in Workspace</span>
              <ArrowRight className="size-4" />
            </button>
          </div>
        ) : (
          <>
            {/* Action Bar */}
            <div className="px-6 py-3 border-b border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] flex flex-wrap items-center justify-between gap-3 text-xs">
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => toggleAll(true)}
                  className="px-2.5 py-1 text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-[#282828] hover:bg-neutral-200 dark:hover:bg-[#323232] rounded-[6px] font-medium transition-colors cursor-pointer border border-neutral-200 dark:border-[#2e2e2e]"
                >
                  Select All
                </button>
                <button
                  type="button"
                  onClick={() => toggleAll(false)}
                  className="px-2.5 py-1 text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-[#282828] hover:bg-neutral-200 dark:hover:bg-[#323232] rounded-[6px] font-medium transition-colors cursor-pointer border border-neutral-200 dark:border-[#2e2e2e]"
                >
                  Deselect All
                </button>
                <span className="text-neutral-400">|</span>
                <span className="text-neutral-600 dark:text-neutral-400">
                  Selected: <strong className="text-neutral-900 dark:text-white">{selectedCount}</strong> of {selections.length}
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
                      className="px-2 py-1 bg-white dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] text-neutral-900 dark:text-white rounded-[6px] text-xs focus:outline-none focus:border-[#3ecf8e]"
                      autoFocus
                    />
                    <button
                      type="button"
                      onClick={handleAddBuildingSubmit}
                      className="px-2.5 py-1 bg-[#3ecf8e] text-[#171717] rounded-[6px] font-semibold text-xs hover:bg-[#24b47e] cursor-pointer"
                    >
                      Assign
                    </button>
                    <button
                      type="button"
                      onClick={() => setShowAddBuilding(false)}
                      className="p-1 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
                    >
                      <X className="size-3.5" />
                    </button>
                  </div>
                ) : (
                  <button
                    type="button"
                    onClick={() => setShowAddBuilding(true)}
                    className="px-2.5 py-1 text-[#24b47e] dark:text-[#3ecf8e] bg-[#3ecf8e]/10 hover:bg-[#3ecf8e]/20 rounded-[6px] font-medium transition-colors flex items-center gap-1 border border-[#3ecf8e]/30 cursor-pointer"
                  >
                    <Plus className="size-3.5" />
                    <span>New Building</span>
                  </button>
                )}
              </div>
            </div>

            {error && (
              <div className="mx-6 mt-3 px-3.5 py-2.5 bg-red-500/10 border border-red-500/30 rounded-[8px] text-xs text-red-600 dark:text-red-400 flex items-center gap-2">
                <AlertTriangle className="size-4 shrink-0 text-red-500" />
                <span>{error}</span>
              </div>
            )}

            {/* Datasets Table */}
            <div className="flex-1 overflow-y-auto p-6 min-h-[300px]">
              <div className="border border-neutral-200 dark:border-[#2e2e2e] rounded-[8px] overflow-hidden shadow-2xs">
                <table className="w-full text-left text-xs divide-y divide-neutral-200 dark:divide-[#2e2e2e]">
                  <thead className="bg-neutral-50 dark:bg-[#171717] text-neutral-600 dark:text-neutral-400 font-semibold uppercase text-[10px] tracking-wider">
                    <tr>
                      <th className="py-2.5 pl-4 pr-2 w-10 text-center">
                        <input
                          type="checkbox"
                          checked={selectedCount === selections.length && selections.length > 0}
                          onChange={(e) => toggleAll(e.target.checked)}
                          className="rounded-[4px] border-neutral-300 dark:border-[#2e2e2e] text-[#3ecf8e] focus:ring-[#3ecf8e] size-3.5 accent-[#3ecf8e] cursor-pointer"
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
                  <tbody className="divide-y divide-neutral-200 dark:divide-[#2e2e2e] bg-white dark:bg-[#202020]">
                    {selections.map((row, idx) => (
                      <tr
                        key={row.entry_path}
                        className={`transition-colors hover:bg-neutral-50 dark:hover:bg-[#282828] ${
                          row.selected ? "bg-[#3ecf8e]/5" : "opacity-60"
                        }`}
                      >
                        <td className="py-3 pl-4 pr-2 text-center">
                          <input
                            type="checkbox"
                            checked={row.selected}
                            onChange={(e) => updateRow(idx, { selected: e.target.checked })}
                            className="rounded-[4px] border-neutral-300 dark:border-[#2e2e2e] text-[#3ecf8e] focus:ring-[#3ecf8e] size-3.5 accent-[#3ecf8e] cursor-pointer"
                          />
                        </td>

                        {/* Editable Name */}
                        <td className="py-3 px-3">
                          <input
                            type="text"
                            value={row.name}
                            onChange={(e) => updateRow(idx, { name: e.target.value })}
                            className="w-full px-2 py-1 bg-white dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] rounded-[6px] font-medium text-neutral-900 dark:text-white focus:outline-none focus:border-[#3ecf8e]"
                          />
                        </td>

                        {/* Archive Entry Path */}
                        <td className="py-3 px-3 text-neutral-500 dark:text-neutral-400 font-mono text-[11px] truncate max-w-[180px]" title={row.entry_path}>
                          {row.entry_path}
                        </td>

                        {/* Format & Dimensions */}
                        <td className="py-3 px-3 text-neutral-600 dark:text-neutral-300">
                          <span className="font-semibold uppercase text-neutral-800 dark:text-neutral-200">
                            {row.format}
                          </span>
                          {row.width > 0 && row.height > 0 && (
                            <span className="text-neutral-400 dark:text-neutral-500 ml-1 text-[11px]">
                              {row.width}×{row.height}
                            </span>
                          )}
                        </td>

                        {/* Georeference status */}
                        <td className="py-3 px-3">
                          {row.is_georeferenced ? (
                            <div className="flex items-center gap-1.5 text-[#24b47e] dark:text-[#3ecf8e] bg-[#3ecf8e]/10 px-2 py-0.5 rounded-[4px] border border-[#3ecf8e]/30 text-[10px] font-semibold w-fit">
                              <Check className="size-3" />
                              <span>Georeferenced</span>
                            </div>
                          ) : (
                            <span className="text-neutral-500 dark:text-neutral-400 bg-neutral-100 dark:bg-[#171717] px-2 py-0.5 rounded-[4px] border border-neutral-200 dark:border-[#2e2e2e] text-[10px] font-medium">
                              Not referenced
                            </span>
                          )}
                          {row.sidecars?.length > 0 && (
                            <div className="text-[10px] text-neutral-400 mt-0.5">
                              Sidecar: {row.sidecars.join(", ")}
                            </div>
                          )}
                        </td>

                        {/* Building Assignment */}
                        <td className="py-3 px-3">
                          <div className="flex items-center gap-1.5">
                            <Building className="size-3.5 text-neutral-400 shrink-0" />
                            <select
                              value={row.building_name}
                              onChange={(e) => updateRow(idx, { building_name: e.target.value })}
                              className="px-2 py-1 bg-white dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] rounded-[6px] text-xs text-neutral-800 dark:text-neutral-200 focus:outline-none focus:border-[#3ecf8e]"
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
                            <Layers className="size-3.5 text-neutral-400 shrink-0" />
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
                              className="px-2 py-1 bg-white dark:bg-[#171717] border border-neutral-300 dark:border-[#2e2e2e] rounded-[6px] text-xs text-neutral-800 dark:text-neutral-200 focus:outline-none focus:border-[#3ecf8e]"
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
            <div className="flex items-center justify-between px-6 py-4 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50/80 dark:bg-[#171717]/80">
              <span className="text-xs text-neutral-500 dark:text-neutral-400">
                Each imported dataset becomes an independent layer in the session workspace.
              </span>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={onClose}
                  disabled={importing}
                  className="px-4 py-2 text-xs font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200/60 dark:hover:bg-[#282828] rounded-[6px] transition-colors border border-neutral-200 dark:border-[#2e2e2e] disabled:opacity-50 cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={selectedCount === 0 || importing}
                  onClick={handleImport}
                  className="px-5 py-2 text-xs font-semibold text-[#171717] bg-[#3ecf8e] hover:bg-[#24b47e] disabled:opacity-50 disabled:pointer-events-none rounded-[6px] transition-colors shadow-xs flex items-center gap-2 cursor-pointer"
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
