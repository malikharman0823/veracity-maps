import React from "react"
import { Globe, Map as MapIcon, CheckCircle2, AlertTriangle, Compass } from "lucide-react"
import { baseLayerPreviewUrl } from "../../api"

export default function BaseLayerPreview({ layer, title = "Selected Base Layer Preview" }) {
  if (!layer) {
    return (
      <div className="rounded-[10px] border border-dashed border-neutral-300 dark:border-[#2e2e2e] p-6 text-center text-xs text-neutral-500 dark:text-neutral-400">
        Select a base layer to inspect its geographic reference details and coverage.
      </div>
    )
  }

  const isDefault = Boolean(layer.is_default || layer.id === "world_map")
  const previewSrc = !isDefault && layer.preview_path ? baseLayerPreviewUrl(layer.preview_path) : null
  const ext = Array.isArray(layer.extent) && layer.extent.length === 4 ? layer.extent.map(Number) : null
  const isReady = layer.status === "ready" || isDefault

  return (
    <div className="rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#17191d] overflow-hidden flex flex-col">
      <div className="px-3.5 py-2 border-b border-neutral-200 dark:border-[#2a2d34] flex items-center justify-between">
        <span className="text-[11px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
          {title}
        </span>
        <span
          className={`inline-flex items-center gap-1 text-[10px] font-semibold px-2 py-0.5 rounded-full ${
            isReady
              ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
              : "bg-amber-500/15 text-amber-600 dark:text-amber-400"
          }`}
        >
          {isReady ? <CheckCircle2 className="w-3 h-3" /> : <AlertTriangle className="w-3 h-3" />}
          {isReady ? "Ready" : layer.status}
        </span>
      </div>

      {/* Visual preview area */}
      <div className="relative h-44 w-full bg-[#111317] flex items-center justify-center overflow-hidden">
        {isDefault ? (
          <div className="w-full h-full bg-gradient-to-br from-[#0d2233] via-[#13384a] to-[#1a4b38] flex flex-col items-center justify-center px-4 text-center">
            <Globe className="w-12 h-12 text-[#3ecf8e] mb-2" />
            <div className="text-sm font-bold text-white">Default World Map</div>
            <div className="text-[11px] text-emerald-200/80 mt-0.5">
              OpenStreetMap global geographic reference (EPSG:3857)
            </div>
          </div>
        ) : previewSrc ? (
          <img
            src={previewSrc}
            alt={layer.name}
            loading="lazy"
            className="max-h-full max-w-full object-contain"
            onError={(e) => {
              e.currentTarget.style.display = "none"
            }}
          />
        ) : (
          <div className="flex flex-col items-center text-neutral-400">
            <MapIcon className="w-10 h-10 mb-1" />
            <span className="text-[11px]">Geographic reference preview</span>
          </div>
        )}
      </div>

      {/* Structured geographic metadata */}
      <div className="p-3.5 space-y-2 text-xs">
        <div>
          <div className="font-bold text-neutral-900 dark:text-white">{layer.name}</div>
          {layer.description && (
            <div className="text-[11px] text-neutral-500 dark:text-neutral-400 mt-0.5">
              {layer.description}
            </div>
          )}
        </div>

        <dl className="grid grid-cols-2 gap-2 pt-2 border-t border-neutral-200 dark:border-[#2a2d34] text-[11px]">
          <div>
            <dt className="text-[10px] uppercase text-neutral-400">Source CRS</dt>
            <dd className="font-mono font-semibold text-neutral-800 dark:text-neutral-200">
              {layer.source_crs || "Unassigned"}
            </dd>
          </div>
          <div>
            <dt className="text-[10px] uppercase text-neutral-400">Source Type</dt>
            <dd className="font-semibold uppercase text-neutral-800 dark:text-neutral-200">
              {isDefault ? "World Map" : (layer.source_type || "raster")}
            </dd>
          </div>
          {!isDefault && (layer.dataset_name || layer.dataset_id) && (
            <div className="col-span-2">
              <dt className="text-[10px] uppercase text-neutral-400">Source Dataset</dt>
              <dd className="font-medium text-neutral-800 dark:text-neutral-200 truncate">
                {layer.dataset_name || layer.dataset_id}
              </dd>
            </div>
          )}
          <div className="col-span-2">
            <dt className="text-[10px] uppercase text-neutral-400 flex items-center gap-1">
              <Compass className="w-3 h-3 text-[#3ecf8e]" />
              Geographic Extent (W, S, E, N)
            </dt>
            <dd className="font-mono text-[10px] text-neutral-700 dark:text-neutral-300 mt-0.5 break-all">
              {ext
                ? `[${ext.map((v) => v.toFixed(5)).join(", ")}]`
                : "Not available — georeferencing required"}
            </dd>
          </div>
        </dl>
      </div>
    </div>
  )
}
