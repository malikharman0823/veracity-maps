import React, { useEffect, useState } from "react"
import { Globe, Search, RefreshCw, AlertTriangle, Check, X } from "lucide-react"
import { listSupportedCRS, reprojectDatasetOrLayer, setDatasetSourceCRS } from "../../api"

export default function CRSManagerModal({
  open,
  onClose,
  layers = [],
  datasets = [],
  activeLayer,
  onReprojected,
  notify,
}) {
  const [search, setSearch] = useState("")
  const [crsList, setCrsList] = useState([])
  const [selectedTargetType, setSelectedTargetType] = useState("layer") // "layer" | "dataset"
  const [selectedLayerId, setSelectedLayerId] = useState(activeLayer?.layer_id || "")
  const [selectedDatasetId, setSelectedDatasetId] = useState(datasets[0]?.dataset_id || datasets[0]?.id || "")
  const [sourceEPSG, setSourceEPSG] = useState("EPSG:4326")
  const [targetEPSG, setTargetEPSG] = useState("EPSG:3857")
  const [outputName, setOutputName] = useState("")
  const [processing, setProcessing] = useState(false)

  useEffect(() => {
    if (!open) return
    listSupportedCRS(search)
      .then((res) => setCrsList(res?.crs_list || []))
      .catch(() => {})
  }, [open, search])

  useEffect(() => {
    if (activeLayer?.layer_id) {
      setSelectedLayerId(activeLayer.layer_id)
      setSourceEPSG(activeLayer.crs || "EPSG:4326")
    }
  }, [activeLayer?.layer_id])

  if (!open) return null

  const currentDataset = datasets.find((d) => (d.dataset_id || d.id) === selectedDatasetId)
  const datasetCRS =
    currentDataset?.dataset?.extracted_metadata?.crs ||
    currentDataset?.extracted_metadata?.crs ||
    ""

  const handleAssignMissingCRS = async () => {
    if (!selectedDatasetId || !sourceEPSG) return
    try {
      await setDatasetSourceCRS(selectedDatasetId, sourceEPSG)
      notify?.(`Assigned source CRS ${sourceEPSG} to dataset`)
      onReprojected?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed setting source CRS", "error")
    }
  }

  const handleReproject = async (e) => {
    e.preventDefault()
    const targetId = selectedTargetType === "layer" ? selectedLayerId : selectedDatasetId
    if (!targetId) {
      notify?.("Select a layer or dataset to transform", "error")
      return
    }
    setProcessing(true)
    try {
      const res = await reprojectDatasetOrLayer(targetId, {
        layer_id: selectedTargetType === "layer" ? selectedLayerId : "",
        source_epsg: sourceEPSG,
        target_epsg: targetEPSG,
        output_name: outputName.trim(),
      })
      notify?.(`Transformed from ${sourceEPSG} to ${targetEPSG} (original preserved)`)
      setOutputName("")
      onReprojected?.(res)
      onClose?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "CRS reprojection failed", "error")
    } finally {
      setProcessing(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex items-center justify-center p-4">
      <div className="w-full max-w-xl rounded-[12px] bg-white dark:bg-[#1a1a1a] border border-neutral-200 dark:border-[#2e2e2e] shadow-2xl overflow-hidden text-xs">
        <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#2a2a2a] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Globe className="size-4 text-[#3ecf8e]" />
            <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
              CRS Management &amp; PROJ Coordinate Reprojection
            </h3>
          </div>
          <button type="button" onClick={onClose} className="p-1 rounded text-neutral-400 hover:text-white">
            <X className="size-4" />
          </button>
        </div>

        <form onSubmit={handleReproject} className="p-4 space-y-4 max-h-[80vh] overflow-y-auto">
          <div className="grid grid-cols-2 gap-3 p-3 rounded-[8px] bg-neutral-50 dark:bg-[#141414] border border-neutral-200 dark:border-[#282828]">
            <div>
              <div className="text-[10px] uppercase text-neutral-400 font-semibold">Project Display CRS</div>
              <div className="font-mono font-bold text-[#3ecf8e] mt-0.5">EPSG:3857 / WGS84 (EPSG:4326)</div>
            </div>
            <div>
              <div className="text-[10px] uppercase text-neutral-400 font-semibold">Active Layer CRS</div>
              <div className="font-mono font-bold text-neutral-900 dark:text-white mt-0.5">
                {activeLayer?.crs || "EPSG:4326"}
              </div>
            </div>
          </div>

          {/* CRS Visual Preview & Plain-Language Explanation */}
          <div className="rounded-lg bg-[#15181b] border border-[#2c333a] p-3 flex items-center gap-3">
            <svg viewBox="0 0 140 60" className="w-32 h-14 rounded border border-[#2a2f35] shrink-0 bg-[#161a1d]">
              <circle cx="34" cy="30" r="18" fill="#1e293b" stroke="#38bdf8" strokeWidth="1.4" />
              <ellipse cx="34" cy="30" rx="8" ry="18" fill="none" stroke="#38bdf8" strokeWidth="0.9" opacity="0.7" />
              <line x1="16" y1="30" x2="52" y2="30" stroke="#38bdf8" strokeWidth="0.9" opacity="0.7" />
              <path d="M58 30 L80 30 M75 26 L81 30 L75 34" stroke="#9ca3af" strokeWidth="1.5" fill="none" />
              <rect x="88" y="14" width="38" height="32" fill="#3ecf8e" fillOpacity="0.18" stroke="#3ecf8e" strokeWidth="1.4" />
              <line x1="100" y1="14" x2="100" y2="46" stroke="#3ecf8e" strokeWidth="0.7" opacity="0.5" />
              <line x1="113" y1="14" x2="113" y2="46" stroke="#3ecf8e" strokeWidth="0.7" opacity="0.5" />
              <line x1="88" y1="30" x2="126" y2="30" stroke="#3ecf8e" strokeWidth="0.7" opacity="0.5" />
            </svg>
            <div className="min-w-0">
              <div className="text-[11px] font-bold text-[#3ecf8e] mb-0.5">
                {sourceEPSG} → {targetEPSG}
              </div>
              <p className="text-[11px] text-neutral-300 leading-snug">
                Converts your map coordinates between global latitude/longitude and flat metric grids so distances and shapes line up accurately.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setSelectedTargetType("layer")}
              className={`flex-1 py-1.5 rounded-[6px] font-semibold border ${
                selectedTargetType === "layer"
                  ? "border-[#3ecf8e] bg-[#3ecf8e]/15 text-[#3ecf8e]"
                  : "border-neutral-200 dark:border-[#303030] text-neutral-500"
              }`}
            >
              Transform Vector Layer ({layers.length})
            </button>
            <button
              type="button"
              onClick={() => setSelectedTargetType("dataset")}
              className={`flex-1 py-1.5 rounded-[6px] font-semibold border ${
                selectedTargetType === "dataset"
                  ? "border-[#3ecf8e] bg-[#3ecf8e]/15 text-[#3ecf8e]"
                  : "border-neutral-200 dark:border-[#303030] text-neutral-500"
              }`}
            >
              Transform Raster Dataset ({datasets.length})
            </button>
          </div>

          {selectedTargetType === "layer" ? (
            <div>
              <label className="block text-[11px] font-semibold text-neutral-500 mb-1">Select Vector Layer</label>
              <select
                value={selectedLayerId}
                onChange={(e) => {
                  setSelectedLayerId(e.target.value)
                  const found = layers.find((l) => l.layer_id === e.target.value)
                  if (found?.crs) setSourceEPSG(found.crs)
                }}
                className="w-full px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333]"
              >
                <option value="">Select a layer…</option>
                {layers.map((l) => (
                  <option key={l.layer_id} value={l.layer_id}>
                    {l.layer_name} ({l.crs || "EPSG:4326"})
                  </option>
                ))}
              </select>
            </div>
          ) : (
            <div className="space-y-2">
              <label className="block text-[11px] font-semibold text-neutral-500 mb-1">Select Dataset</label>
              <select
                value={selectedDatasetId}
                onChange={(e) => setSelectedDatasetId(e.target.value)}
                className="w-full px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333]"
              >
                <option value="">Select a dataset…</option>
                {datasets.map((d) => {
                  const id = d.dataset_id || d.id
                  const name = d.alias || d.dataset?.filename || d.filename || id
                  return (
                    <option key={id} value={id}>
                      {name}
                    </option>
                  )
                })}
              </select>

              {!datasetCRS && selectedDatasetId && (
                <div className="p-2.5 rounded-[6px] bg-amber-500/10 border border-amber-500/30 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2 text-amber-400">
                    <AlertTriangle className="size-4 shrink-0" />
                    <span>Missing CRS detected on this dataset. Confirm source CRS first:</span>
                  </div>
                  <button
                    type="button"
                    onClick={handleAssignMissingCRS}
                    className="px-2.5 py-1 rounded bg-amber-500 text-black font-semibold shrink-0"
                  >
                    Set {sourceEPSG}
                  </button>
                </div>
              )}
            </div>
          )}

          {/* EPSG Search Catalog */}
          <div>
            <label className="block text-[11px] font-semibold text-neutral-500 mb-1">
              Search Supported EPSG Registry
            </label>
            <div className="flex items-center gap-2 px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333] mb-2">
              <Search className="size-3.5 text-neutral-400" />
              <input
                type="text"
                placeholder="Search by EPSG code (e.g. 4326, 3857, 32643, UTM, British)…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="bg-transparent outline-none w-full"
              />
            </div>
            <div className="max-h-36 overflow-y-auto border border-neutral-200 dark:border-[#2c2c2c] rounded-[6px] divide-y divide-neutral-200 dark:divide-[#262626]">
              {crsList.map((crs) => (
                <div
                  key={crs.epsg_code}
                  onClick={() => setTargetEPSG(crs.epsg_code)}
                  className={`p-2 flex items-center justify-between cursor-pointer ${
                    targetEPSG === crs.epsg_code
                      ? "bg-[#3ecf8e]/15 text-[#3ecf8e]"
                      : "hover:bg-neutral-50 dark:hover:bg-[#222]"
                  }`}
                >
                  <div>
                    <div className="font-mono font-bold">{crs.epsg_code} — {crs.name}</div>
                    <div className="text-[10px] text-neutral-400">
                      Type: {crs.type} • Units: {crs.units} • Area: {crs.area_of_use}
                    </div>
                  </div>
                  {targetEPSG === crs.epsg_code && <Check className="size-4 text-[#3ecf8e]" />}
                </div>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <label className="flex flex-col gap-1">
              <span className="text-[11px] font-semibold text-neutral-500">Source CRS</span>
              <input
                type="text"
                value={sourceEPSG}
                onChange={(e) => setSourceEPSG(e.target.value)}
                className="px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333] font-mono"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-[11px] font-semibold text-neutral-500">Target CRS</span>
              <input
                type="text"
                value={targetEPSG}
                onChange={(e) => setTargetEPSG(e.target.value)}
                className="px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333] font-mono"
              />
            </label>
          </div>

          <label className="block">
            <span className="text-[11px] font-semibold text-neutral-500">New Output Layer / Dataset Name</span>
            <input
              type="text"
              placeholder="Optional name for reprojected copy (original is preserved)"
              value={outputName}
              onChange={(e) => setOutputName(e.target.value)}
              className="w-full mt-1 px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333]"
            />
          </label>

          <div className="flex justify-end gap-2 pt-2 border-t border-neutral-200 dark:border-[#2a2a2a]">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 rounded-[6px] border border-neutral-300 dark:border-[#333]"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={processing}
              className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold cursor-pointer"
            >
              <RefreshCw className={`size-3.5 ${processing ? "animate-spin" : ""}`} />
              <span>{processing ? "Reprojecting…" : "Run Reprojection"}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
