import React, { useEffect, useState } from "react"
import {
  Cpu,
  Search,
  Play,
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Loader2,
  Bookmark,
  Trash2,
  Terminal,
  History,
  Wrench,
  Layers,
  Table2,
  X,
} from "lucide-react"

import {
  listProcessingAlgorithms,
  validateProcessingRequest,
  executeProcessingJob,
  listProcessingJobs,
  getProcessingJob,
  cancelProcessingJob,
  getProcessingJobLogs,
  getTopologyErrors,
  fixTopologyErrors,
} from "../../api"
import AlgorithmPreviewCard, { getAlgorithmGuide } from "./AlgorithmPreviewCard"

const CATEGORIES = [
  "All",
  "Geometry Operations",
  "Overlay Analysis",
  "Spatial Statistics",
  "Raster Processing",
  "Data Conversion",
  "Geometry Validation",
]

const CORE_ALGORITHM_IDS = new Set([
  "buffer",
  "centroid",
  "calculate_geometry",
  "clip",
  "intersection",
  "union",
  "dissolve",
  "difference",
  "spatial_join",
  "topology",
  "raster_clip",
  "raster_reproject",
  "raster_resample",
  "raster_statistics",
  "raster_calculator",
  "raster_hillshade",
  "raster_contours",
])

export default function ProcessingToolboxModal({
  open,
  onClose,
  mapId,
  layers = [],
  datasets = [],
  activeLayer,
  onJobCompleted,
  onOpenAttributeTable,
  notify,
}) {
  const [algorithms, setAlgorithms] = useState([])
  const [search, setSearch] = useState("")
  const [selectedCategory, setSelectedCategory] = useState("All")
  const [selectedAlg, setSelectedAlg] = useState(null)
  const [params, setParams] = useState({})
  const [running, setRunning] = useState(false)
  const [activeJob, setActiveJob] = useState(null)
  const [jobLogs, setJobLogs] = useState([])
  const [jobHistory, setJobHistory] = useState([])
  const [topologyErrors, setTopologyErrors] = useState([])
  const [presets, setPresets] = useState([])
  const [presetName, setPresetName] = useState("")
  const [bottomTab, setBottomTab] = useState("logs") // "logs" | "history" | "topology"

  const vectorLayers = layers.filter((l) => l.layer_type === "vector" || !l.layer_type)

  useEffect(() => {
    if (!open) return
    listProcessingAlgorithms()
      .then((res) => {
        const list = (res?.algorithms || []).filter((a) => CORE_ALGORITHM_IDS.has(a.algorithm_id))
        setAlgorithms(list)
        if (list.length > 0 && !selectedAlg) {
          selectAlgorithm(list[0])
        }
      })
      .catch(() => {})
    refreshHistory()
  }, [open, mapId])

  const refreshHistory = () => {
    listProcessingJobs(mapId ? { map_id: mapId, limit: 25 } : { limit: 25 })
      .then((res) => setJobHistory(res?.jobs || []))
      .catch(() => {})
  }

  const selectAlgorithm = (alg) => {
    setSelectedAlg(alg)
    setTopologyErrors([])
    const initial = {}
    const defaultLayerId = activeLayer?.layer_id || vectorLayers[0]?.layer_id || layers[0]?.layer_id || ""
    const secondLayerId = vectorLayers[1]?.layer_id || defaultLayerId
    const defaultDatasetId = datasets[0]?.dataset_id || datasets[0]?.id || ""

    for (const p of alg.parameters || []) {
      if (p.default_value !== undefined) {
        initial[p.name] = p.default_value
      } else if (p.type === "layer") {
        if (p.name.endsWith("_b") || p.name === "source_layer_id" || p.name === "target_layer_id" || p.name === "clipping_layer_id") {
          initial[p.name] = secondLayerId
        } else {
          initial[p.name] = defaultLayerId
        }
      } else if (p.type === "dataset") {
        initial[p.name] = defaultDatasetId
      } else {
        initial[p.name] = ""
      }
    }
    setParams(initial)
    listProcessingPresets(alg.algorithm_id)
      .then((res) => setPresets(res?.presets || []))
      .catch(() => setPresets([]))
  }

  const handleParamChange = (name, value) => {
    setParams((prev) => ({ ...prev, [name]: value }))
  }

  const buildPayload = () => {
    if (!selectedAlg) return null
    const formattedParams = { ...params }
    for (const p of selectedAlg.parameters || []) {
      if (p.type === "number" && formattedParams[p.name] !== "") {
        formattedParams[p.name] = Number(formattedParams[p.name])
      }
      if (p.type === "fields" && typeof formattedParams[p.name] === "string") {
        formattedParams[p.name] = formattedParams[p.name]
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean)
      }
      if (p.name === "input_rasters" || p.name === "raster_dataset_ids") {
        const raw = formattedParams[p.name]
        if (typeof raw === "string") {
          formattedParams[p.name] = raw
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean)
        }
      }
    }
    return {
      map_id: mapId,
      algorithm_name: selectedAlg.algorithm_id,
      parameters: formattedParams,
      output_name: formattedParams.output_name || `${selectedAlg.name} Output`,
    }
  }

  const handleValidate = async () => {
    const payload = buildPayload()
    if (!payload) return
    try {
      await validateProcessingRequest(payload)
      notify?.(`Parameters for "${selectedAlg.name}" are valid.`)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Parameter validation failed.")
    }
  }

  const handleExecute = async () => {
    const payload = buildPayload()
    if (!payload) return
    setRunning(true)
    setBottomTab("logs")
    try {
      const job = await executeProcessingJob(payload)
      setActiveJob(job)
      if (job?.job_id) {
        const logsRes = await getProcessingJobLogs(job.job_id).catch(() => ({ logs: [] }))
        setJobLogs(logsRes?.logs || [])
        if (selectedAlg.algorithm_id === "topology") {
          const errRes = await getTopologyErrors(job.job_id).catch(() => ({ errors: [] }))
          setTopologyErrors(errRes?.errors || [])
          setBottomTab("topology")
        }
      }
      refreshHistory()
      onJobCompleted?.(job)
      notify?.(`${selectedAlg.name} completed (${job?.status || "done"}).`)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Processing execution failed.")
    } finally {
      setRunning(false)
    }
  }

  const handleCancelJob = async () => {
    if (!activeJob?.job_id) return
    try {
      const updated = await cancelProcessingJob(activeJob.job_id)
      setActiveJob(updated)
      refreshHistory()
      notify?.("Processing job cancelled.")
    } catch {
      notify?.("Unable to cancel job.")
    }
  }

  const handleFixTopology = async () => {
    if (!activeJob?.job_id) return
    try {
      const res = await fixTopologyErrors(activeJob.job_id, { fix_all: true })
      const errRes = await getTopologyErrors(activeJob.job_id).catch(() => ({ errors: [] }))
      setTopologyErrors(errRes?.errors || [])
      onJobCompleted?.(activeJob)
      notify?.(`Resolved ${res?.resolved_count || 0} topology errors.`)
    } catch {
      notify?.("Failed to auto-fix topology errors.")
    }
  }

  const handleSavePreset = async () => {
    if (!selectedAlg || !presetName.trim()) return
    try {
      await saveProcessingPreset({
        map_id: mapId,
        algorithm_name: selectedAlg.algorithm_id,
        preset_name: presetName.trim(),
        parameters: params,
      })
      setPresetName("")
      const res = await listProcessingPresets(selectedAlg.algorithm_id)
      setPresets(res?.presets || [])
      notify?.("Processing preset saved.")
    } catch {
      notify?.("Failed to save preset.")
    }
  }

  const handleDeletePreset = async (presetId) => {
    try {
      await deleteProcessingPreset(presetId)
      setPresets((prev) => prev.filter((p) => p.preset_id !== presetId))
    } catch {
      notify?.("Failed to delete preset.")
    }
  }

  const handleInspectHistoryJob = async (job) => {
    setActiveJob(job)
    if (job?.job_id) {
      const logsRes = await getProcessingJobLogs(job.job_id).catch(() => ({ logs: [] }))
      setJobLogs(logsRes?.logs || [])
      if (job.algorithm_name === "topology") {
        const errRes = await getTopologyErrors(job.job_id).catch(() => ({ errors: [] }))
        setTopologyErrors(errRes?.errors || [])
        setBottomTab("topology")
      } else {
        setBottomTab("logs")
      }
    }
  }

  if (!open) return null

  const filteredAlgorithms = algorithms.filter((alg) => {
    if (selectedCategory !== "All" && alg.category !== selectedCategory) return false
    if (
      search &&
      !alg.name.toLowerCase().includes(search.toLowerCase()) &&
      !alg.description.toLowerCase().includes(search.toLowerCase()) &&
      !alg.algorithm_id.toLowerCase().includes(search.toLowerCase())
    ) {
      return false
    }
    return true
  })

  const outputLayerId = activeJob?.output_config?.output_layer_id || activeJob?.output_config?.error_layer_id

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 backdrop-blur-xs p-4">
      <div className="w-full max-w-6xl h-[86vh] rounded-xl bg-white dark:bg-[#1a1a1a] border border-neutral-200 dark:border-[#2e2e2e] shadow-2xl flex flex-col overflow-hidden">
        {/* Top Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#202020]">
          <div className="flex items-center gap-2.5">
            <div className="size-8 rounded-lg bg-[#3ecf8e]/15 border border-[#3ecf8e]/30 flex items-center justify-center text-[#3ecf8e]">
              <Cpu className="size-4" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-neutral-900 dark:text-white">
                GIS Processing Toolbox
              </h3>
              <p className="text-[11px] text-neutral-500">
                Asynchronous GEOS, PROJ & GDAL Spatial Analysis Engine ({algorithms.length} Algorithms)
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="text-neutral-400 hover:text-neutral-700 dark:hover:text-white p-1"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Main Body: Left Sidebar (Categories + Search) + Right Workspace (Form + Logs/History) */}
        <div className="flex-1 flex min-h-0 overflow-hidden">
          {/* Left Panel: Algorithm Catalog */}
          <div className="w-80 border-r border-neutral-200 dark:border-[#2e2e2e] flex flex-col bg-neutral-50/50 dark:bg-[#171717]">
            <div className="p-2.5 border-b border-neutral-200 dark:border-[#2a2a2a] space-y-2">
              <div className="relative">
                <Search className="size-3.5 text-neutral-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search GIS algorithms..."
                  className="w-full pl-8 pr-2.5 py-1.5 text-xs rounded bg-white dark:bg-[#222] border border-neutral-200 dark:border-[#333] text-neutral-900 dark:text-white outline-none focus:border-[#3ecf8e]"
                />
              </div>
              <div className="flex flex-wrap gap-1">
                {CATEGORIES.map((cat) => (
                  <button
                    key={cat}
                    type="button"
                    onClick={() => setSelectedCategory(cat)}
                    className={`px-2 py-0.5 rounded text-[10px] font-semibold transition-colors ${
                      selectedCategory === cat
                        ? "bg-[#3ecf8e] text-[#171717]"
                        : "bg-neutral-200/70 dark:bg-[#262626] text-neutral-600 dark:text-neutral-300 hover:bg-neutral-300 dark:hover:bg-[#303030]"
                    }`}
                  >
                    {cat === "All" ? "All" : cat.split(" ")[0]}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-2 space-y-1">
              {filteredAlgorithms.map((alg) => {
                const isSelected = selectedAlg?.algorithm_id === alg.algorithm_id
                return (
                  <button
                    key={alg.algorithm_id}
                    type="button"
                    onClick={() => selectAlgorithm(alg)}
                    className={`w-full text-left p-2.5 rounded-lg border transition-all ${
                      isSelected
                        ? "border-[#3ecf8e] bg-[#3ecf8e]/10"
                        : "border-transparent hover:bg-neutral-100 dark:hover:bg-[#222]"
                    }`}
                  >
                    <div className="flex items-center justify-between gap-1">
                      <span className="text-xs font-bold text-neutral-900 dark:text-white truncate">
                        {alg.name}
                      </span>
                      <span className="text-[9px] font-mono px-1.5 py-0.5 rounded bg-neutral-200 dark:bg-[#2a2a2a] text-neutral-600 dark:text-neutral-300 shrink-0">
                        {alg.library}
                      </span>
                    </div>
                    <div className="text-[10px] text-[#24b47e] dark:text-[#3ecf8e] font-medium mt-0.5">
                      {alg.category}
                    </div>
                    <p className="text-[10px] text-neutral-500 line-clamp-2 mt-0.5">
                      {alg.description}
                    </p>
                  </button>
                )
              })}
            </div>
          </div>

          {/* Center + Bottom Panel */}
          <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
            {/* Parameter Form Area */}
            {selectedAlg ? (
              <div className="flex-1 overflow-y-auto p-4 space-y-4">
                <div className="flex items-start justify-between gap-4 pb-3 border-b border-neutral-200 dark:border-[#2a2a2a]">
                  <div>
                    <div className="flex items-center gap-2">
                      <h4 className="text-base font-bold text-neutral-900 dark:text-white">
                        {selectedAlg.name}
                      </h4>
                      <span className="text-[10px] px-2 py-0.5 rounded bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] font-mono font-semibold">
                        {selectedAlg.library}
                      </span>
                    </div>
                    <p className="text-xs text-neutral-500 mt-1">{selectedAlg.description}</p>
                  </div>
                </div>

                <AlgorithmPreviewCard algorithm={selectedAlg} />

                {/* Dynamic Parameter Inputs */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3.5">
                  {(selectedAlg.parameters || []).map((param) => {
                    const val = params[param.name] ?? ""
                    return (
                      <div key={param.name} className="space-y-1">
                        <label className="text-xs font-semibold text-neutral-700 dark:text-neutral-300 flex items-center gap-1">
                          <span>{param.label}</span>
                          {param.required && <span className="text-red-500">*</span>}
                        </label>

                        {param.type === "layer" ? (
                          <select
                            value={val}
                            onChange={(e) => handleParamChange(param.name, e.target.value)}
                            className="w-full text-xs px-2.5 py-2 rounded bg-neutral-50 dark:bg-[#222] border border-neutral-200 dark:border-[#333] text-neutral-900 dark:text-white"
                          >
                            <option value="">-- Select Map Layer --</option>
                            {layers.map((l) => (
                              <option key={l.layer_id} value={l.layer_id}>
                                {l.layer_name} ({l.geometry_type || l.layer_type})
                              </option>
                            ))}
                          </select>
                        ) : param.type === "dataset" ? (
                          <input
                            type="text"
                            value={Array.isArray(val) ? val.join(", ") : val}
                            onChange={(e) => handleParamChange(param.name, e.target.value)}
                            placeholder={
                              datasets.length > 0
                                ? `e.g. ${datasets.map((d) => d.dataset_id || d.id).join(", ")}`
                                : "Enter raster dataset ID(s)"
                            }
                            className="w-full text-xs px-2.5 py-2 rounded bg-neutral-50 dark:bg-[#222] border border-neutral-200 dark:border-[#333] text-neutral-900 dark:text-white"
                          />
                        ) : param.type === "select" ? (
                          <select
                            value={val}
                            onChange={(e) => handleParamChange(param.name, e.target.value)}
                            className="w-full text-xs px-2.5 py-2 rounded bg-neutral-50 dark:bg-[#222] border border-neutral-200 dark:border-[#333] text-neutral-900 dark:text-white"
                          >
                            {(param.options || []).map((opt) => (
                              <option key={opt} value={opt}>
                                {opt}
                              </option>
                            ))}
                          </select>
                        ) : param.type === "boolean" ? (
                          <label className="flex items-center gap-2 text-xs text-neutral-700 dark:text-neutral-300 pt-1 cursor-pointer">
                            <input
                              type="checkbox"
                              checked={Boolean(val)}
                              onChange={(e) => handleParamChange(param.name, e.target.checked)}
                              className="rounded border-neutral-300"
                            />
                            <span>Enable {param.label}</span>
                          </label>
                        ) : param.type === "number" ? (
                          <input
                            type="number"
                            step="any"
                            min={param.min}
                            max={param.max}
                            value={val}
                            onChange={(e) => handleParamChange(param.name, e.target.value)}
                            className="w-full text-xs px-2.5 py-2 rounded bg-neutral-50 dark:bg-[#222] border border-neutral-200 dark:border-[#333] text-neutral-900 dark:text-white"
                          />
                        ) : (
                          <input
                            type="text"
                            value={Array.isArray(val) ? val.join(", ") : val}
                            onChange={(e) => handleParamChange(param.name, e.target.value)}
                            placeholder={param.label}
                            className="w-full text-xs px-2.5 py-2 rounded bg-neutral-50 dark:bg-[#222] border border-neutral-200 dark:border-[#333] text-neutral-900 dark:text-white"
                          />
                        )}
                      </div>
                    )
                  })}
                </div>

                {/* Action Bar */}
                <div className="flex items-center justify-between pt-3 border-t border-neutral-200 dark:border-[#2a2a2a]">
                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={handleValidate}
                      className="px-3 py-2 rounded-lg border border-neutral-200 dark:border-[#333] hover:bg-neutral-100 dark:hover:bg-[#252525] text-xs font-semibold text-neutral-700 dark:text-neutral-200"
                    >
                      Validate Parameters
                    </button>
                    <button
                      type="button"
                      onClick={handleExecute}
                      disabled={running}
                      className="px-4 py-2 rounded-lg bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-bold text-xs flex items-center gap-1.5 disabled:opacity-50"
                    >
                      {running ? <Loader2 className="size-3.5 animate-spin" /> : <Play className="size-3.5" />}
                      <span>{running ? "Executing Job..." : `Run ${selectedAlg.name}`}</span>
                    </button>
                  </div>

                  {/* Quick Result Actions when Output Layer is Ready */}
                  {outputLayerId && (
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-[#24b47e] dark:text-[#3ecf8e] font-semibold flex items-center gap-1">
                        <CheckCircle2 className="size-3.5" /> Output Layer Registered
                      </span>
                      {onOpenAttributeTable && (
                        <button
                          type="button"
                          onClick={() => {
                            const found = layers.find((l) => l.layer_id === outputLayerId) || {
                              layer_id: outputLayerId,
                              layer_name: activeJob?.output_config?.output_layer_name || "Output Layer",
                              layer_type: "vector",
                            }
                            onOpenAttributeTable(found)
                            onClose()
                          }}
                          className="px-2.5 py-1.5 rounded bg-neutral-100 dark:bg-[#262626] hover:bg-neutral-200 text-xs font-semibold text-neutral-800 dark:text-neutral-200 flex items-center gap-1"
                        >
                          <Table2 className="size-3.5 text-[#3ecf8e]" /> Open Output Attributes
                        </button>
                      )}
                    </div>
                  )}
                </div>
              </div>
            ) : null}

            {/* Bottom Console: Progress, Logs, History, and Topology Inspector */}
            <div className="h-60 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#151515] flex flex-col">
              <div className="flex items-center justify-between px-3 py-1.5 border-b border-neutral-200 dark:border-[#262626]">
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => setBottomTab("logs")}
                    className={`px-2.5 py-1 rounded text-xs font-semibold flex items-center gap-1 ${
                      bottomTab === "logs"
                        ? "bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]"
                        : "text-neutral-500 hover:text-neutral-800 dark:hover:text-white"
                    }`}
                  >
                    <Terminal className="size-3.5" /> Execution Logs ({jobLogs.length})
                  </button>
                  <button
                    type="button"
                    onClick={() => setBottomTab("history")}
                    className={`px-2.5 py-1 rounded text-xs font-semibold flex items-center gap-1 ${
                      bottomTab === "history"
                        ? "bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e]"
                        : "text-neutral-500 hover:text-neutral-800 dark:hover:text-white"
                    }`}
                  >
                    <History className="size-3.5" /> Job History ({jobHistory.length})
                  </button>
                  {topologyErrors.length > 0 && (
                    <button
                      type="button"
                      onClick={() => setBottomTab("topology")}
                      className={`px-2.5 py-1 rounded text-xs font-semibold flex items-center gap-1 ${
                        bottomTab === "topology"
                          ? "bg-amber-500/20 text-amber-500"
                          : "text-amber-500 hover:bg-amber-500/10"
                      }`}
                    >
                      <AlertTriangle className="size-3.5" /> Topology Errors ({topologyErrors.length})
                    </button>
                  )}
                </div>

                {activeJob && (
                  <div className="flex items-center gap-3 text-xs">
                    <span className="font-mono text-[11px] text-neutral-500">
                      Job: {activeJob.job_id} • {activeJob.stage} ({activeJob.progress}%)
                    </span>
                    {activeJob.status === "running" || activeJob.status === "queued" ? (
                      <button
                        type="button"
                        onClick={handleCancelJob}
                        className="px-2 py-0.5 rounded bg-red-500/15 text-red-500 text-[11px] font-semibold"
                      >
                        Cancel
                      </button>
                    ) : null}
                  </div>
                )}
              </div>

              <div className="flex-1 overflow-y-auto p-3 text-xs font-mono">
                {bottomTab === "logs" && (
                  <div className="space-y-1">
                    {jobLogs.length === 0 ? (
                      <div className="text-neutral-400">
                        Select an algorithm and click Run to view real-time execution stages and GDAL/GEOS logs.
                      </div>
                    ) : (
                      jobLogs.map((l) => (
                        <div key={l.log_id} className="flex items-start gap-2">
                          <span className="text-neutral-400 shrink-0">[{l.stage}]</span>
                          <span
                            className={
                              l.log_level === "ERROR"
                                ? "text-red-400"
                                : l.log_level === "WARN"
                                ? "text-amber-400"
                                : "text-[#3ecf8e]"
                            }
                          >
                            {l.log_level}:
                          </span>
                          <span className="text-neutral-800 dark:text-neutral-200">{l.message}</span>
                        </div>
                      ))
                    )}
                  </div>
                )}

                {bottomTab === "history" && (
                  <div className="space-y-1.5 font-sans">
                    {jobHistory.map((j) => (
                      <div
                        key={j.job_id}
                        onClick={() => handleInspectHistoryJob(j)}
                        className="flex items-center justify-between p-2 rounded bg-white dark:bg-[#1e1e1e] border border-neutral-200 dark:border-[#2a2a2a] cursor-pointer hover:border-[#3ecf8e]"
                      >
                        <div className="flex items-center gap-2">
                          {j.status === "completed" ? (
                            <CheckCircle2 className="size-3.5 text-[#3ecf8e]" />
                          ) : j.status === "failed" ? (
                            <XCircle className="size-3.5 text-red-500" />
                          ) : (
                            <Loader2 className="size-3.5 text-amber-500" />
                          )}
                          <span className="font-semibold text-neutral-900 dark:text-white uppercase text-[11px]">
                            {j.algorithm_name}
                          </span>
                          <span className="text-[11px] text-neutral-400 font-mono">{j.job_id}</span>
                        </div>
                        <span className="text-[11px] text-neutral-500">
                          {j.status} ({j.progress}%)
                        </span>
                      </div>
                    ))}
                  </div>
                )}

                {bottomTab === "topology" && (
                  <div className="space-y-2 font-sans">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-amber-500">
                        Detected {topologyErrors.length} topological error(s)
                      </span>
                      <button
                        type="button"
                        onClick={handleFixTopology}
                        className="px-3 py-1 rounded bg-[#3ecf8e] text-[#171717] font-bold text-xs flex items-center gap-1"
                      >
                        <Wrench className="size-3.5" /> Auto-Fix All Topology Errors
                      </button>
                    </div>
                    {topologyErrors.map((err) => (
                      <div
                        key={err.error_id}
                        className="p-2 rounded bg-white dark:bg-[#1e1e1e] border border-neutral-200 dark:border-[#2a2a2a] flex items-center justify-between"
                      >
                        <div>
                          <span className="text-xs font-bold text-amber-500 uppercase mr-2">
                            {err.error_type}
                          </span>
                          <span className="text-xs text-neutral-800 dark:text-neutral-200">
                            {err.error_message}
                          </span>
                          {err.suggestion && (
                            <p className="text-[11px] text-neutral-400 mt-0.5">{err.suggestion}</p>
                          )}
                        </div>
                        <span
                          className={`text-[10px] px-2 py-0.5 rounded font-semibold ${
                            err.is_resolved
                              ? "bg-[#3ecf8e]/20 text-[#3ecf8e]"
                              : "bg-amber-500/20 text-amber-400"
                          }`}
                        >
                          {err.is_resolved ? "Resolved" : "Open"}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
