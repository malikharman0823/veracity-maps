import React from "react"
import { Globe, Map as MapIcon, Check, AlertTriangle, Edit2, Trash2, Eye } from "lucide-react"
import { baseLayerPreviewUrl } from "../../api"

function formatExtentShort(extent) {
  if (!Array.isArray(extent) || extent.length !== 4) return "Extent unavailable"
  const [w, s, e, n] = extent.map(Number)
  if (![w, s, e, n].every(Number.isFinite)) return "Extent unavailable"
  if (w <= -179 && e >= 179) return "Global Coverage"
  return `${s.toFixed(2)}°–${n.toFixed(2)}°N, ${w.toFixed(2)}°–${e.toFixed(2)}°E`
}

export default function BaseLayerCard({
  layer,
  selected = false,
  onSelect,
  onPreview,
  onEdit,
  onDelete,
  showManagementActions = false,
}) {
  if (!layer) return null
  const isDefault = Boolean(layer.is_default || layer.id === "world_map")
  const previewSrc = !isDefault && layer.preview_path ? baseLayerPreviewUrl(layer.preview_path) : null
  const isReady = layer.status === "ready" || isDefault

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSelect?.(layer)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault()
          onSelect?.(layer)
        }
      }}
      className={`group relative flex flex-col rounded-[10px] border transition-all cursor-pointer overflow-hidden ${
        selected
          ? "border-[#3ecf8e] bg-[#3ecf8e]/8 ring-1 ring-[#3ecf8e]/50 shadow-sm"
          : "border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#1e1e1e] hover:border-[#3ecf8e]/50"
      }`}
    >
      {/* Thumbnail */}
      <div className="relative h-28 w-full overflow-hidden bg-neutral-100 dark:bg-[#15171b] flex items-center justify-center">
        {isDefault ? (
          <div className="relative w-full h-full bg-gradient-to-br from-[#0f293a] via-[#173b4f] to-[#1e4d3a] flex flex-col items-center justify-center text-white/90">
            <Globe className="w-9 h-9 text-[#3ecf8e] mb-1 drop-shadow" />
            <span className="text-[10px] font-semibold tracking-wider uppercase text-emerald-200/90">
              Global OpenStreetMap
            </span>
          </div>
        ) : previewSrc ? (
          <img
            src={previewSrc}
            alt={layer.name}
            loading="lazy"
            className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
            onError={(e) => {
              e.currentTarget.style.display = "none"
            }}
          />
        ) : (
          <MapIcon className="w-8 h-8 text-neutral-400" />
        )}

        {/* Top badges */}
        <div className="absolute top-2 left-2 flex items-center gap-1">
          <span className="px-1.5 py-0.5 rounded bg-black/70 text-[10px] font-semibold text-white backdrop-blur-xs">
            {layer.source_crs || "CRS N/A"}
          </span>
          {!isReady && (
            <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-amber-500/90 text-[10px] font-semibold text-black">
              <AlertTriangle className="w-3 h-3" />
              {layer.status}
            </span>
          )}
        </div>

        {selected && (
          <div className="absolute top-2 right-2 w-6 h-6 rounded-full bg-[#3ecf8e] text-[#171717] flex items-center justify-center shadow">
            <Check className="w-3.5 h-3.5 stroke-[3]" />
          </div>
        )}
      </div>

      {/* Metadata Body */}
      <div className="p-3 flex-1 flex flex-col justify-between gap-2">
        <div>
          <div className="flex items-center justify-between gap-2">
            <h4 className="text-xs font-bold text-neutral-900 dark:text-white truncate" title={layer.name}>
              {layer.name}
            </h4>
            <span className="shrink-0 text-[9px] font-semibold uppercase px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-[#2a2a2a] text-neutral-600 dark:text-neutral-300">
              {isDefault ? "Default" : (layer.source_type || "raster")}
            </span>
          </div>
          {layer.description && (
            <p className="text-[11px] text-neutral-500 dark:text-neutral-400 line-clamp-2 mt-0.5">
              {layer.description}
            </p>
          )}
        </div>

        <div className="space-y-1 pt-1.5 border-t border-neutral-100 dark:border-[#2a2a2a] text-[10px] text-neutral-500 dark:text-neutral-400">
          {!isDefault && (layer.dataset_name || layer.dataset_id) && (
            <div className="truncate">
              <span className="text-neutral-400">Dataset: </span>
              <span className="text-neutral-700 dark:text-neutral-200 font-medium">
                {layer.dataset_name || layer.dataset_id}
              </span>
            </div>
          )}
          <div className="truncate font-mono text-[10px]">{formatExtentShort(layer.extent)}</div>
        </div>

        {showManagementActions && (
          <div
            className="flex items-center justify-end gap-1 pt-1.5 border-t border-neutral-100 dark:border-[#2a2a2a]"
            onClick={(e) => e.stopPropagation()}
          >
            {onPreview && (
              <button
                type="button"
                onClick={() => onPreview(layer)}
                className="inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] cursor-pointer"
                title="Preview base layer"
              >
                <Eye className="w-3.5 h-3.5" />
                Preview
              </button>
            )}
            {!isDefault && onEdit && (
              <button
                type="button"
                onClick={() => onEdit(layer)}
                className="inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] font-medium text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] cursor-pointer"
                title="Rename or edit description"
              >
                <Edit2 className="w-3.5 h-3.5" />
                Edit
              </button>
            )}
            {!isDefault && onDelete && (
              <button
                type="button"
                onClick={() => onDelete(layer)}
                className="inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] font-medium text-red-500 hover:bg-red-500/10 cursor-pointer"
                title="Remove base layer registration"
              >
                <Trash2 className="w-3.5 h-3.5" />
                Remove
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
