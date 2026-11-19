import React, { useEffect, useMemo, useRef, useState } from "react"
import {
  ChevronDown,
  ChevronRight,
  Search,
  Play,
  ArrowLeft,
  CheckCircle2,
  AlertTriangle,
  Loader2,
  History,
  Terminal,
  Wrench,
  Table2,
  X,
  Cpu,
  Palette,
} from "lucide-react"
import {
  listProcessingAlgorithms,
  validateProcessingRequest,
  executeProcessingJob,
  listProcessingJobs,
  getProcessingJobLogs,
  getTopologyErrors,
  fixTopologyErrors,
} from "../../api"
import StyleEditorPanel from "../gis/StyleEditorPanel"

const CATEGORY_BUCKETS = [
  { id: "vector_analysis", label: "Vector Analysis", ids: ["spatial_join", "centroid"] },
  { id: "vector_overlay", label: "Vector Overlay", ids: ["clip", "intersection", "union", "difference"] },
  { id: "vector_geometry", label: "Vector Geometry", ids: ["buffer", "dissolve", "topology"] },
  { id: "vector_table", label: "Vector Table", ids: ["calculate_geometry"] },
  { id: "raster_analysis", label: "Raster Analysis", ids: ["raster_calculator", "raster_statistics"] },
  { id: "raster_tools", label: "Raster Tools", ids: ["raster_clip", "raster_reproject", "raster_resample"] },
  { id: "raster_terrain", label: "Raster Terrain", ids: ["raster_hillshade", "raster_contours"] },
]

const ALLOWED_ALG_IDS = new Set(CATEGORY_BUCKETS.flatMap((b) => b.ids))

export default function DockedProcessingToolbox({
  mapId,
  layers = [],
  datasets = [],
  activeLayer,
  activeDockTab = "toolbox", // "toolbox" | "history" | "symbology"
  onChangeDockTab,
  requestedAlgorithmId = null,
  requestedAlgorithmTick = 0,
  focusSearchTick = 0,
  styleEditorLayer = null,
  onStyleSaved,
  onJobCompleted,
  onOpenAttributeTable,
  onCollapse,
  notify,
}) {
  const [algorithms, setAlgorithms] = useState([])
  const [loadingAlgs, setLoadingAlgs] = useState(false)
  const [search, setSearch] = useState("")
  const [expandedCats, setExpandedCats] = useState(() =>
    Object.fromEntries(CATEGORY_BUCKETS.map((c) => [c.id, true]))
  )
  const [selectedAlg, setSelectedAlg] = useState(null)
  const [params, setParams] = useState({})
  const [running, setRunning] = useState(false)
  const [activeJob, setActiveJob] = useState(null)
  const [jobLogs, setJobLogs] = useState([])
  const [jobHistory, setJobHistory] = useState([])
  const [topologyErrors, setTopologyErrors] = useState([])
  const searchRef = useRef(null)

  const vectorLayers = useMemo(
    () => layers.filter((l) => l.layer_type === "vector" || !l.layer_type),
    [layers]
  )

  const refreshHistory = () => {
    listProcessingJobs(mapId ? { map_id: mapId, limit: 25 } : { limit: 25 })
      .then((res) => setJobHistory(res?.jobs || []))
      .catch(() => {})
  }

  useEffect(() => {
    setLoadingAlgs(true)
    listProcessingAlgorithms()
      .then((res) => {
        const list = (res?.algorithms || []).filter((a) => ALLOWED_ALG_IDS.has(a.algorithm_id))
        setAlgorithms(list)
      })
      .catch(() => {})
      .finally(() => setLoadingAlgs(false))
    refreshHistory()
  }, [mapId])

  useEffect(() => {
    if (focusSearchTick > 0) {
      setSelectedAlg(null)
      setTimeout(() => searchRef.current?.focus(), 60)
    }
  }, [focusSearchTick])

  const selectAlgorithm = (alg) => {
    setSelectedAlg(alg)
    setTopologyErrors([])
    setActiveJob(null)
    setJobLogs([])
    const initial = {}
    const defaultLayerId = activeLayer?.layer_id || vectorLayers[0]?.layer_id || layers[0]?.layer_id || ""
    const secondLayerId = vectorLayers[1]?.layer_id || defaultLayerId
    const defaultDatasetId = datasets[0]?.dataset_id || datasets[0]?.id || ""

    for (const p of alg.parameters || []) {
      if (p.default_value !== undefined) {
        initial[p.name] = p.default_value
      } else if (p.type === "layer") {
        if (
          p.name.endsWith("_b") ||
          p.name === "source_layer_id" ||
          p.name === "target_layer_id" ||
          p.name === "clipping_layer_id"
        ) {
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
    initial.output_name = `${alg.name} Output`
    setParams(initial)
  }

  useEffect(() => {
    if (!requestedAlgorithmId || algorithms.length === 0) return
    const match = algorithms.find((a) => a.algorithm_id === requestedAlgorithmId)
    if (match) {
      selectAlgorithm(match)
    }
  }, [requestedAlgorithmId, requestedAlgorithmTick, algorithms])

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
      notify?.(`Parameters valid for "${selectedAlg.name}"`)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Parameter validation failed")
    }
  }

  const handleRun = async () => {
    const payload = buildPayload()
    if (!payload) return
    setRunning(true)
    try {
      const job = await executeProcessingJob(payload)
      setActiveJob(job)
      if (job?.job_id) {
        const logsRes = await getProcessingJobLogs(job.job_id).catch(() => ({ logs: [] }))
        setJobLogs(logsRes?.logs || [])
        if (selectedAlg.algorithm_id === "topology") {
          const errRes = await getTopologyErrors(job.job_id).catch(() => ({ errors: [] }))
          setTopologyErrors(errRes?.errors || [])
        }
      }
      refreshHistory()
      onJobCompleted?.(job)
      notify?.(`${selectedAlg.name} finished (${job?.status || "done"})`)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Processing failed")
    } finally {
      setRunning(false)
    }
  }

  const handleFixTopology = async () => {
    if (!activeJob?.job_id) return
    try {
      const res = await fixTopologyErrors(activeJob.job_id, { fix_all: true })
      const errRes = await getTopologyErrors(activeJob.job_id).catch(() => ({ errors: [] }))
      setTopologyErrors(errRes?.errors || [])
      onJobCompleted?.(activeJob)
      notify?.(`Auto-fixed ${res?.resolved_count || 0} topology errors`)
    } catch {
      notify?.("Topology auto-fix failed")
    }
  }

  const q = search.trim().toLowerCase()
  const algById = useMemo(
    () => new Map(algorithms.map((a) => [a.algorithm_id, a])),
    [algorithms]
  )

  const outputLayerId =
    activeJob?.output_config?.output_layer_id || activeJob?.output_config?.error_layer_id

  return (
    <div className="flex flex-col h-full min-h-0 bg-white dark:bg-[#1e1e1e] text-neutral-800 dark:text-neutral-200">
      {/* Dock Header Tabs */}
      <div className="h-9 px-2.5 border-b border-neutral-200 dark:border-[#303030] bg-neutral-100 dark:bg-[#252525] flex items-center justify-between gap-2 shrink-0">
        <div className="flex items-center gap-1 min-w-0">
          <button
            type="button"
            onClick={() => onChangeDockTab?.("toolbox")}
            className={`px-2.5 py-1 rounded-[5px] inline-flex items-center gap-1.5 transition-colors cursor-pointer ${
              activeDockTab === "toolbox"
                ? "bg-white dark:bg-[#1a1a1a] text-[#24b47e] dark:text-[#3ecf8e] shadow-2xs"
                : "text-neutral-500 hover:text-neutral-900 dark:hover:text-white"
            }`}
          >
            <Cpu className="size-3.5 shrink-0" />
            <span className="text-[11px] font-semibold tracking-wide">Toolbox</span>
          </button>
          <button
            type="button"
            onClick={() => {
              onChangeDockTab?.("history")
              refreshHistory()
            }}
            className={`px-2.5 py-1 rounded-[5px] inline-flex items-center gap-1.5 transition-colors cursor-pointer ${
              activeDockTab === "history"
                ? "bg-white dark:bg-[#1a1a1a] text-[#24b47e] dark:text-[#3ecf8e] shadow-2xs"
                : "text-neutral-500 hover:text-neutral-900 dark:hover:text-white"
            }`}
          >
            <History className="size-3.5 shrink-0" />
            <span className="text-[11px] font-semibold tracking-wide">History</span>
          </button>
          <button
            type="button"
            onClick={() => onChangeDockTab?.("symbology")}
            className={`px-2.5 py-1 rounded-[5px] inline-flex items-center gap-1.5 transition-colors cursor-pointer ${
              activeDockTab === "symbology"
                ? "bg-white dark:bg-[#1a1a1a] text-[#24b47e] dark:text-[#3ecf8e] shadow-2xs"
                : "text-neutral-500 hover:text-neutral-900 dark:hover:text-white"
            }`}
          >
            <Palette className="size-3.5 shrink-0" />
            <span className="text-[11px] font-semibold tracking-wide">Style</span>
          </button>
        </div>

        {onCollapse && (
          <button
            type="button"
            onClick={onCollapse}
            className="p-1.5 rounded-[5px] hover:bg-neutral-200 dark:hover:bg-[#353535] text-neutral-500 dark:text-neutral-300 cursor-pointer shrink-0"
            title="Collapse Processing Toolbox"
          >
            <X className="size-3.5" />
          </button>
        )}
      </div>

      {/* TAB 1: PROCESSING TOOLBOX */}
      {activeDockTab === "toolbox" && (
        <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
          {!selectedAlg ? (
            <>
              <div className="p-2.5 border-b border-neutral-200 dark:border-[#2c2c2c] shrink-0">
                <div className="relative">
                  <Search className="size-3.5 text-neutral-400 absolute left-2.5 top-1/2 -translate-y-1/2" />
                  <input
                    ref={searchRef}
                    type="text"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Search GIS algorithms…"
                    className="w-full h-8 pl-8 pr-3 text-[12px] rounded-[6px] bg-neutral-100 dark:bg-[#161616] border border-neutral-200 dark:border-[#333333] text-neutral-800 dark:text-neutral-100 placeholder:text-neutral-500 outline-none focus:border-[#3ecf8e]"
                  />
                </div>
              </div>

              <div className="flex-1 overflow-y-auto py-2 px-1.5 space-y-1 min-h-0">
                {loadingAlgs ? (
                  <div className="p-6 flex items-center justify-center gap-2 text-xs text-neutral-400">
                    <Loader2 className="size-4 animate-spin text-[#3ecf8e]" /> Loading algorithms…
                  </div>
                ) : (
                  CATEGORY_BUCKETS.map((cat) => {
                    const items = cat.ids
                      .map((id) => algById.get(id))
                      .filter(Boolean)
                      .filter((alg) => {
                        if (!q) return true
                        return (
                          alg.name.toLowerCase().includes(q) ||
                          alg.description.toLowerCase().includes(q) ||
                          alg.algorithm_id.toLowerCase().includes(q)
                        )
                      })
                    if (q && items.length === 0) return null
                    const isOpen = q ? true : Boolean(expandedCats[cat.id])
                    return (
                      <div key={cat.id} className="pb-0.5">
                        <button
                          type="button"
                          onClick={() =>
                            setExpandedCats((prev) => ({ ...prev, [cat.id]: !prev[cat.id] }))
                          }
                          className="w-full flex items-center gap-2 px-2.5 py-2 rounded-[5px] hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] transition-colors cursor-pointer"
                        >
                          {isOpen ? (
                            <ChevronDown className="size-3.5 text-neutral-400 shrink-0" />
                          ) : (
                            <ChevronRight className="size-3.5 text-neutral-400 shrink-0" />
                          )}
                          <span className="truncate flex-1 text-left text-[12px] font-semibold text-neutral-800 dark:text-neutral-100">
                            {cat.label}
                          </span>
                          <span className="px-1.5 py-0.5 rounded bg-neutral-200/70 dark:bg-[#2d2d2d] text-[10px] font-mono text-neutral-500 dark:text-neutral-400">
                            {items.length}
                          </span>
                        </button>
                        {isOpen && (
                          <div className="pl-4 pr-1 py-1 space-y-1">
                            {items.map((alg) => (
                              <button
                                key={alg.algorithm_id}
                                type="button"
                                onClick={() => selectAlgorithm(alg)}
                                className="w-full text-left px-2.5 py-2 rounded-[6px] hover:bg-[#3ecf8e]/12 transition-colors cursor-pointer group space-y-1"
                              >
                                <div className="flex items-center justify-between gap-2">
                                  <span className="text-[12px] font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-[#24b47e] dark:group-hover:text-[#3ecf8e] truncate">
                                    {alg.name}
                                  </span>
                                  <span className="text-[9.5px] font-mono uppercase px-1.5 py-0.5 rounded bg-neutral-200 dark:bg-[#2e2e2e] text-neutral-500 dark:text-neutral-400 shrink-0">
                                    {alg.engine || "GIS"}
                                  </span>
                                </div>
                                <div className="text-[11px] leading-relaxed text-neutral-500 dark:text-neutral-400 line-clamp-2">
                                  {alg.description}
                                </div>
                              </button>
                            ))}
                          </div>
                        )}
                      </div>
                    )
                  })
                )}
              </div>
            </>
          ) : (
            /* Selected Algorithm Parameter Runner */
            <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
              <div className="px-2.5 py-2 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#232323] shrink-0">
                <button
                  type="button"
                  onClick={() => setSelectedAlg(null)}
                  className="inline-flex items-center gap-1 text-[10.5px] text-neutral-500 hover:text-[#3ecf8e] mb-1 cursor-pointer"
                >
                  <ArrowLeft className="size-3" /> All Algorithms
                </button>
                <div className="text-xs font-bold text-neutral-900 dark:text-white">
                  {selectedAlg.name}
                </div>
                <p className="text-[10.5px] text-neutral-500 dark:text-neutral-400 mt-0.5 leading-snug">
                  {selectedAlg.description}
                </p>
              </div>

              <div className="flex-1 overflow-y-auto p-2.5 space-y-2.5 min-h-0 text-[11px]">
                {(selectedAlg.parameters || []).map((p) => (
                  <div key={p.name}>
                    <label className="block text-[10.5px] font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                      {p.label || p.name}
                      {p.required && <span className="text-red-500 ml-0.5">*</span>}
                    </label>
                    {p.type === "layer" ? (
                      <select
                        value={params[p.name] ?? ""}
                        onChange={(e) => setParams((prev) => ({ ...prev, [p.name]: e.target.value }))}
                        className="w-full h-7 px-2 rounded-[4px] bg-neutral-50 dark:bg-[#161616] border border-neutral-300 dark:border-[#363636] text-[11px]"
                      >
                        <option value="">Select vector layer…</option>
                        {vectorLayers.map((l) => (
                          <option key={l.layer_id} value={l.layer_id}>
                            {l.layer_name} ({l.geometry_type || "Vector"})
                          </option>
                        ))}
                      </select>
                    ) : p.type === "dataset" ? (
                      <select
                        value={params[p.name] ?? ""}
                        onChange={(e) => setParams((prev) => ({ ...prev, [p.name]: e.target.value }))}
                        className="w-full h-7 px-2 rounded-[4px] bg-neutral-50 dark:bg-[#161616] border border-neutral-300 dark:border-[#363636] text-[11px]"
                      >
                        <option value="">Select raster dataset…</option>
                        {datasets.map((d) => (
                          <option key={d.dataset_id || d.id} value={d.dataset_id || d.id}>
                            {d.name || d.dataset?.filename || d.dataset_id}
                          </option>
                        ))}
                      </select>
                    ) : p.type === "select" && Array.isArray(p.options) ? (
                      <select
                        value={params[p.name] ?? ""}
                        onChange={(e) => setParams((prev) => ({ ...prev, [p.name]: e.target.value }))}
                        className="w-full h-7 px-2 rounded-[4px] bg-neutral-50 dark:bg-[#161616] border border-neutral-300 dark:border-[#363636] text-[11px]"
                      >
                        {p.options.map((opt) => (
                          <option key={opt} value={opt}>
                            {opt}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <input
                        type={p.type === "number" ? "number" : "text"}
                        step="any"
                        value={params[p.name] ?? ""}
                        placeholder={p.description || ""}
                        onChange={(e) => setParams((prev) => ({ ...prev, [p.name]: e.target.value }))}
                        className="w-full h-7 px-2 rounded-[4px] bg-neutral-50 dark:bg-[#161616] border border-neutral-300 dark:border-[#363636] text-[11px] outline-none focus:border-[#3ecf8e]"
                      />
                    )}
                  </div>
                ))}

                <div>
                  <label className="block text-[10.5px] font-semibold text-neutral-700 dark:text-neutral-300 mb-1">
                    Output Layer Name
                  </label>
                  <input
                    type="text"
                    value={params.output_name ?? ""}
                    onChange={(e) => setParams((prev) => ({ ...prev, output_name: e.target.value }))}
                    className="w-full h-7 px-2 rounded-[4px] bg-neutral-50 dark:bg-[#161616] border border-neutral-300 dark:border-[#363636] text-[11px] outline-none focus:border-[#3ecf8e]"
                  />
                </div>

                {/* Action Buttons */}
                <div className="flex items-center gap-1.5 pt-1">
                  <button
                    type="button"
                    onClick={handleValidate}
                    disabled={running}
                    className="px-2.5 h-7 rounded-[4px] border border-neutral-300 dark:border-[#383838] text-[11px] font-medium hover:bg-neutral-100 dark:hover:bg-[#2c2c2c] cursor-pointer"
                  >
                    Validate
                  </button>
                  <button
                    type="button"
                    onClick={handleRun}
                    disabled={running}
                    className="flex-1 h-7 px-3 rounded-[4px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#141414] text-[11px] font-bold inline-flex items-center justify-center gap-1.5 disabled:opacity-50 cursor-pointer"
                  >
                    {running ? <Loader2 className="size-3.5 animate-spin" /> : <Play className="size-3.5" />}
                    <span>{running ? "Running…" : "Run"}</span>
                  </button>
                </div>

                {/* Status & Execution Logs */}
                {activeJob && (
                  <div className="rounded-[5px] border border-neutral-200 dark:border-[#323232] bg-neutral-50 dark:bg-[#161616] p-2 space-y-1.5">
                    <div className="flex items-center justify-between text-[10.5px]">
                      <span className="font-bold inline-flex items-center gap-1">
                        {activeJob.status === "completed" ? (
                          <CheckCircle2 className="size-3.5 text-[#3ecf8e]" />
                        ) : (
                          <AlertTriangle className="size-3.5 text-amber-500" />
                        )}
                        Status: {activeJob.status}
                      </span>
                      {outputLayerId && onOpenAttributeTable && (
                        <button
                          type="button"
                          onClick={() => {
                            const outLyr = layers.find((l) => l.layer_id === outputLayerId)
                            if (outLyr) onOpenAttributeTable(outLyr)
                          }}
                          className="text-[10px] text-[#24b47e] dark:text-[#3ecf8e] hover:underline inline-flex items-center gap-0.5 font-semibold cursor-pointer"
                        >
                          <Table2 className="size-3" /> Attributes
                        </button>
                      )}
                    </div>

                    {jobLogs.length > 0 && (
                      <div className="max-h-36 overflow-y-auto space-y-1 font-mono text-[10px] text-neutral-600 dark:text-neutral-300 border-t border-neutral-200 dark:border-[#2a2a2a] pt-1.5">
                        <div className="flex items-center gap-1 text-[9.5px] text-neutral-400 uppercase">
                          <Terminal className="size-3" /> Execution Log
                        </div>
                        {jobLogs.map((l, i) => (
                          <div key={i} className="leading-snug">
                            <span className="text-[#3ecf8e]">[{l.stage || l.level || "INFO"}]</span>{" "}
                            {l.message}
                          </div>
                        ))}
                      </div>
                    )}

                    {topologyErrors.length > 0 && (
                      <div className="border-t border-neutral-200 dark:border-[#2a2a2a] pt-1.5 space-y-1">
                        <div className="flex items-center justify-between text-[10px]">
                          <span className="text-amber-500 font-semibold">
                            {topologyErrors.length} topology issue(s)
                          </span>
                          <button
                            type="button"
                            onClick={handleFixTopology}
                            className="px-2 py-0.5 rounded bg-amber-500 text-[#141414] font-bold inline-flex items-center gap-1 cursor-pointer"
                          >
                            <Wrench className="size-2.5" /> Auto-Fix
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {/* TAB 2: RECENT OPERATIONS HISTORY */}
      {activeDockTab === "history" && (
        <div className="flex-1 overflow-y-auto p-2 space-y-1.5 min-h-0 text-[11px]">
          {jobHistory.length === 0 ? (
            <div className="py-8 text-center text-neutral-400 text-[11px]">
              No processing jobs run in this workspace yet.
            </div>
          ) : (
            jobHistory.map((job) => (
              <div
                key={job.job_id}
                className="p-2 rounded-[5px] border border-neutral-200 dark:border-[#303030] bg-neutral-50 dark:bg-[#191919] space-y-0.5"
              >
                <div className="flex items-center justify-between gap-1">
                  <span className="font-semibold text-neutral-800 dark:text-neutral-100 truncate">
                    {job.algorithm_name}
                  </span>
                  <span
                    className={`px-1.5 py-0.2 rounded text-[9.5px] font-semibold uppercase ${
                      job.status === "completed"
                        ? "bg-emerald-500/15 text-emerald-500"
                        : "bg-amber-500/15 text-amber-500"
                    }`}
                  >
                    {job.status}
                  </span>
                </div>
                <div className="text-[10px] font-mono text-neutral-400 truncate">
                  ID: {job.job_id}
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {/* TAB 3: SYMBOLOGY / LAYER PROPERTIES */}
      {activeDockTab === "symbology" && (
        <div className="flex-1 overflow-y-auto min-h-0">
          {styleEditorLayer || activeLayer || vectorLayers[0] ? (
            <StyleEditorPanel
              layer={styleEditorLayer || activeLayer || vectorLayers[0]}
              onStyleSaved={onStyleSaved}
              onClose={() => onChangeDockTab?.("toolbox")}
              notify={notify}
            />
          ) : (
            <div className="p-5 text-center text-[11px] text-neutral-400">
              Select a vector layer in the Layers panel to edit its symbology, colors, outlines, and labels.
            </div>
          )}
        </div>
      )}
    </div>
  )
}
