import React from "react"
import { Link } from "react-router-dom"
import { datasetSourceImageUrl } from "../api"
import { Button } from "@/components/ui/button"
import {
  ArrowRight,
  Clock,
  Layers,
  Map,
  Loader2,
  Trash2,
} from "lucide-react"

function displayName(filename) {
  const withoutExtension = filename.replace(/(\.osm)?\.[^.]+$/i, "")
  return withoutExtension
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function formatName(filename, metadata) {
  if (metadata?.driver) return metadata.driver === "GTiff" ? "TIFF" : metadata.driver
  const lower = filename.toLowerCase()
  if (lower.endsWith(".osm.pbf")) return "OSM PBF"
  const extension = lower.split(".").pop()
  return extension === "tif" || extension === "tiff"
    ? "TIFF"
    : extension.toUpperCase()
}

function fileSize(bytes) {
  if (!Number.isFinite(bytes)) return "Unknown"
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

function getStatusDetails(dataset) {
  let dotColor = "bg-[#7a7a7a]"
  let text = "Uploaded"
  let isPulsing = false

  if (dataset.tile_status === "completed") {
    dotColor = "bg-[#34c759]"
    text = "Ready"
  } else if (
    dataset.tile_status === "failed" ||
    dataset.process_status === "failed" ||
    dataset.georeference_status === "failed"
  ) {
    dotColor = "bg-[#ff3b30]"
    text = "Failed"
  } else if (
    dataset.tile_status === "processing" ||
    dataset.process_status === "processing" ||
    dataset.georeference_status === "processing"
  ) {
    dotColor = "bg-[#ff9500]"
    text = "Processing"
    isPulsing = true
  }

  return { dotColor, text, isPulsing }
}

export default function DatasetCard({ dataset, viewMode = "grid", onDeleteClick }) {
  const status = getStatusDetails(dataset)
  const format = formatName(dataset.filename, dataset.metadata)
  const size = fileSize(dataset.file_size)
  const crs = dataset.metadata?.crs || "Unknown CRS"
  const name = displayName(dataset.filename)
  const dateStr = new Date(dataset.updated_at || dataset.created_at).toLocaleDateString()

  const isRaster = format.includes("TIF") || format.includes("PNG") || format.includes("JPG")

  if (viewMode === "list") {
    return (
      <div className="group rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] hover:border-[#3ecf8e] dark:hover:border-[#3ecf8e] transition-all duration-200 flex items-center p-3 sm:p-4 gap-3 sm:gap-4 shadow-xs">
        {/* Minimal icon/thumbnail */}
        <div className="w-12 h-12 rounded-[6px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center shrink-0 overflow-hidden">
          {isRaster ? (
            <img
              src={datasetSourceImageUrl(dataset.id)}
              alt="Preview"
              className="w-full h-full object-cover"
              onError={(e) => {
                e.target.style.display = "none"
              }}
            />
          ) : (
            <Map className="size-5 text-neutral-400" />
          )}
        </div>

        {/* Info */}
        <div className="flex-1 min-w-0 flex flex-col justify-center">
          <div className="flex items-center gap-2">
            <h3 className="font-semibold text-sm text-neutral-900 dark:text-white truncate group-hover:text-[#24b47e] dark:group-hover:text-[#3ecf8e] transition-colors">
              {name}
            </h3>
            <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#171717] text-[10px] font-medium text-neutral-700 dark:text-neutral-300">
              <span
                className={`w-1.5 h-1.5 rounded-full ${status.dotColor} ${
                  status.isPulsing ? "animate-pulse" : ""
                }`}
              ></span>
              {status.text}
            </span>
          </div>
          <div className="flex items-center gap-2 mt-1 text-[11px] text-neutral-500 dark:text-neutral-400">
            <span className="font-mono truncate">{dataset.filename}</span>
            <span>•</span>
            <span className="font-mono">{format}</span>
            <span>•</span>
            <span className="font-mono">{size}</span>
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-xs text-neutral-500 dark:text-neutral-400 hidden md:block mr-2">
            Updated {dateStr}
          </span>
          {onDeleteClick && (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onDeleteClick(dataset)}
              className="text-neutral-400 hover:text-red-500 hover:bg-red-500/10 rounded-[6px]"
              title="Delete Dataset"
            >
              <Trash2 className="size-4" />
            </Button>
          )}
          <Button asChild size="sm" className="h-8 text-xs bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold rounded-[6px] shadow-xs">
            <Link to={`/datasets/${dataset.id}`}>
              <span>Open</span>
              <ArrowRight className="size-3.5 ml-1" />
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="group rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] hover:border-[#3ecf8e] dark:hover:border-[#3ecf8e] transition-all duration-200 flex flex-col justify-between overflow-hidden shadow-xs hover:shadow-md">
      <div>
        {/* Thumbnail preview area */}
        <div className="relative h-44 bg-neutral-50 dark:bg-[#171717] border-b border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center overflow-hidden p-3">
          {isRaster ? (
            <img
              src={datasetSourceImageUrl(dataset.id)}
              alt="Preview"
              className="w-full h-full object-contain rounded-[6px] transition-transform duration-300 group-hover:scale-[1.02]"
              style={{
                filter: "drop-shadow(0 4px 12px rgba(0,0,0,0.15))",
              }}
              onError={(e) => {
                e.target.style.display = "none"
              }}
            />
          ) : (
            <div className="w-full h-full rounded-[6px] bg-neutral-100 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center">
              <Map className="size-10 text-neutral-400" />
            </div>
          )}

          {/* Badges on image */}
          <div className="absolute top-3 left-3 flex items-center gap-1.5">
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-[4px] border border-neutral-200 dark:border-[#2e2e2e] bg-white/90 dark:bg-[#202020]/90 backdrop-blur-xs text-[11px] font-medium text-neutral-800 dark:text-neutral-200 shadow-xs">
              <span
                className={`w-1.5 h-1.5 rounded-full ${status.dotColor} ${
                  status.isPulsing ? "animate-pulse" : ""
                }`}
              ></span>
              {status.text}
            </span>
          </div>

          {onDeleteClick && (
            <button
              onClick={() => onDeleteClick(dataset)}
              className="absolute top-3 right-3 size-7 rounded-[4px] bg-white/90 dark:bg-[#202020]/90 hover:bg-red-500 text-neutral-500 hover:text-white border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center transition-colors shadow-xs cursor-pointer"
              title="Delete Dataset"
            >
              <Trash2 className="size-3.5" />
            </button>
          )}
        </div>

        {/* Card Header & Metadata */}
        <div className="p-5 pb-3 space-y-3">
          <div>
            <h3
              className="font-semibold text-base text-neutral-900 dark:text-white truncate group-hover:text-[#24b47e] dark:group-hover:text-[#3ecf8e] transition-colors tracking-tight"
              title={name}
            >
              {name}
            </h3>
            <p
              className="font-mono text-xs text-neutral-500 dark:text-neutral-400 truncate mt-0.5"
              title={dataset.filename}
            >
              {dataset.filename}
            </p>
          </div>

          {/* Metadata badges */}
          <div className="flex flex-wrap gap-1.5 pt-1">
            <span className="inline-flex items-center px-2 py-0.5 rounded-[4px] text-[11px] font-mono border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#171717] text-neutral-700 dark:text-neutral-300">
              {format}
            </span>
            <span className="inline-flex items-center px-2 py-0.5 rounded-[4px] text-[11px] font-mono border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#171717] text-neutral-500 dark:text-neutral-400">
              {size}
            </span>
            <span className="inline-flex items-center px-2 py-0.5 rounded-[4px] text-[11px] font-mono border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#171717] text-neutral-500 dark:text-neutral-400">
              {crs}
            </span>
          </div>
        </div>
      </div>

      {/* Card Footer */}
      <div className="p-5 pt-3 border-t border-neutral-200 dark:border-[#2e2e2e] space-y-3 bg-neutral-50/50 dark:bg-[#1c1c1c]/50">
        <div className="flex items-center justify-between text-[11px] text-neutral-500 dark:text-neutral-400">
          <span className="flex items-center gap-1.5">
            <Clock className="size-3.5 text-neutral-400" />
            {dateStr}
          </span>
          <span className="font-mono text-[10px] text-neutral-400">
            {dataset.tile_status === "completed" ? "Tiled" : "Raw"}
          </span>
        </div>

        {status.isPulsing ? (
          <Button
            variant="outline"
            className="w-full h-9 rounded-[6px] text-xs font-normal text-neutral-400 border-neutral-300 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#171717] cursor-not-allowed"
            disabled
          >
            <Loader2 className="size-3.5 animate-spin mr-1.5 text-amber-500" />
            <span>Processing…</span>
          </Button>
        ) : (
          <Button
            asChild
            className="w-full h-9 text-xs font-semibold bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] rounded-[6px] shadow-xs"
          >
            <Link to={`/datasets/${dataset.id}`}>
              <span>Open in Studio</span>
              <ArrowRight className="size-3.5 ml-1.5" />
            </Link>
          </Button>
        )}
      </div>
    </div>
  )
}

export { displayName, fileSize, formatName }
