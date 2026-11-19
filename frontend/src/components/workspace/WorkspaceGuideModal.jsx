import React from "react"
import { BookOpen, Layers, Cpu, Database, Compass, X } from "lucide-react"

export default function WorkspaceGuideModal({ open, workspaceName, crs, vectorCount, rasterCount, onClose }) {
  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-[1150] bg-black/70 backdrop-blur-xs flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Veracity Maps Workspace Guide"
    >
      <div className="w-full max-w-xl rounded-[10px] border border-neutral-300 dark:border-[#343434] bg-white dark:bg-[#1c1c1c] shadow-2xl overflow-hidden text-neutral-800 dark:text-neutral-200">
        <div className="px-4 py-3 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#232323] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <BookOpen className="size-4 text-[#3ecf8e]" />
            <h2 className="text-sm font-bold text-neutral-900 dark:text-white">
              Workspace Properties &amp; GIS Guide
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded hover:bg-neutral-200 dark:hover:bg-[#323232] cursor-pointer"
            aria-label="Close guide"
          >
            <X className="size-4" />
          </button>
        </div>

        <div className="p-4 space-y-3 text-xs">
          <div className="p-3 rounded-[6px] bg-neutral-50 dark:bg-[#222222] border border-neutral-200 dark:border-[#303030] grid grid-cols-2 gap-2">
            <div>
              <span className="text-neutral-400 block text-[10px] uppercase">Workspace</span>
              <strong className="text-neutral-900 dark:text-white">{workspaceName}</strong>
            </div>
            <div>
              <span className="text-neutral-400 block text-[10px] uppercase">Active CRS</span>
              <strong className="font-mono text-[#24b47e] dark:text-[#3ecf8e]">{crs || "EPSG:4326"}</strong>
            </div>
            <div>
              <span className="text-neutral-400 block text-[10px] uppercase">Vector Layers</span>
              <strong>{vectorCount}</strong>
            </div>
            <div>
              <span className="text-neutral-400 block text-[10px] uppercase">Raster Layers</span>
              <strong>{rasterCount}</strong>
            </div>
          </div>

          <div className="space-y-2.5">
            <div className="flex items-start gap-2.5">
              <Layers className="size-4 text-[#3ecf8e] mt-0.5 shrink-0" />
              <div>
                <div className="font-bold text-neutral-900 dark:text-white">Three-Group Layer Hierarchy</div>
                <p className="text-neutral-500 dark:text-neutral-400">
                  Layers are permanently organized as <strong>Vector (Top) → Raster (Middle) → Base Layer (Bottom)</strong>. Reorder within groups by dragging, or right-click any layer for Zoom, Symbology, Attributes, Rename, or Remove.
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2.5">
              <Compass className="size-4 text-amber-500 mt-0.5 shrink-0" />
              <div>
                <div className="font-bold text-neutral-900 dark:text-white">Browser &amp; Dynamic GeoPackage Discovery</div>
                <p className="text-neutral-500 dark:text-neutral-400">
                  Expand any uploaded <code>.gpkg</code> in the left Browser panel to inspect every vector layer dynamically and add selected layers directly onto the Viewscape.
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2.5">
              <Cpu className="size-4 text-sky-500 mt-0.5 shrink-0" />
              <div>
                <div className="font-bold text-neutral-900 dark:text-white">Right Processing Toolbox</div>
                <p className="text-neutral-500 dark:text-neutral-400">
                  Run Vector Analysis, Overlay, Geometry, Table, Raster Analysis, Raster Tools, and Terrain algorithms directly from the collapsible right dock or the Vector/Raster top menus.
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2.5">
              <Database className="size-4 text-purple-400 mt-0.5 shrink-0" />
              <div>
                <div className="font-bold text-neutral-900 dark:text-white">Dockable Workspace Records (F6)</div>
                <p className="text-neutral-500 dark:text-neutral-400">
                  Press <kbd className="px-1 py-0.5 rounded bg-neutral-200 dark:bg-[#303030] font-mono text-[10px]">F6</kbd> to dock the Workspace Records table at the bottom without leaving the map canvas. Selecting or locating a record highlights and zooms on the map.
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
