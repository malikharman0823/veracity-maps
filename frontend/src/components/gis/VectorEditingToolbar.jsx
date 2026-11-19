import React from "react"
import {
  MousePointer,
  MapPin,
  Spline,
  Pentagon,
  Move,
  Edit3,
  Scissors,
  Combine,
  Trash2,
  Undo2,
  Redo2,
  Magnet,
  Check,
  X,
  Copy,
  AlertTriangle,
} from "lucide-react"

const TOOLS = [
  { id: "select", label: "Select", icon: MousePointer, shortcut: "V" },
  { id: "point", label: "Point", icon: MapPin, shortcut: "P" },
  { id: "line", label: "Line", icon: Spline, shortcut: "L" },
  { id: "polygon", label: "Polygon", icon: Pentagon, shortcut: "G" },
  { id: "move", label: "Move", icon: Move, shortcut: "M" },
  { id: "vertex", label: "Vertex Editor", icon: Edit3, shortcut: "E" },
  { id: "split", label: "Split", icon: Scissors, shortcut: "S" },
]

export default function VectorEditingToolbar({
  activeLayer,
  activeTool = "select",
  onSelectTool,
  snapEnabled = false,
  onToggleSnap,
  selectedFeatureIds = [],
  draftCoordinates = [],
  validationResult = null,
  canUndo = false,
  canRedo = false,
  onUndo,
  onRedo,
  onConfirmSaveDraft,
  onCancelDraft,
  onDeleteSelected,
  onDuplicateSelected,
  onMergeSelected,
}) {
  if (!activeLayer || activeLayer.layer_type !== "vector") {
    return null
  }

  const isLocked = Boolean(activeLayer.is_locked)
  const hasDraft = draftCoordinates.length > 0

  return (
    <div className="flex flex-col items-start gap-1.5 z-30 pointer-events-auto">
      <div className="flex flex-nowrap items-center gap-1 text-xs whitespace-nowrap">
        <div className="px-2 py-0.5 rounded bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] font-semibold text-[11px] mr-0.5 flex items-center gap-1.5">
          <span>Editing: {activeLayer.layer_name}</span>
          {isLocked && <span className="text-amber-400 font-bold">(LOCKED)</span>}
        </div>

        {TOOLS.map((tool) => {
          const Icon = tool.icon
          const active = activeTool === tool.id
          return (
            <button
              key={tool.id}
              type="button"
              disabled={isLocked && tool.id !== "select"}
              onClick={() => onSelectTool?.(tool.id)}
              className={`inline-flex items-center gap-1 px-2 py-1 rounded-[6px] font-medium transition-colors cursor-pointer disabled:opacity-40 ${
                active
                  ? "bg-[#3ecf8e] text-[#171717] font-semibold shadow-2xs"
                  : "text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#262626]"
              }`}
              title={`${tool.label} (${tool.shortcut})`}
            >
              <Icon className="size-3.5" />
              <span className="hidden md:inline text-[11px]">{tool.label}</span>
            </button>
          )
        })}

        <div className="h-4 w-[1px] bg-neutral-200 dark:bg-[#303030] mx-0.5" />

        <button
          type="button"
          disabled={isLocked || selectedFeatureIds.length < 2}
          onClick={onMergeSelected}
          className="inline-flex items-center gap-1 px-2 py-1 rounded-[6px] text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#262626] disabled:opacity-40 cursor-pointer"
          title="Merge Selected Features (select 2+ compatible features)"
        >
          <Combine className="size-3.5" />
          <span className="hidden lg:inline text-[11px]">Merge</span>
        </button>

        <button
          type="button"
          disabled={isLocked || selectedFeatureIds.length === 0}
          onClick={onDuplicateSelected}
          className="inline-flex items-center gap-1 px-2 py-1 rounded-[6px] text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#262626] disabled:opacity-40 cursor-pointer"
          title="Duplicate Selected Feature"
        >
          <Copy className="size-3.5" />
        </button>

        <button
          type="button"
          disabled={isLocked || selectedFeatureIds.length === 0}
          onClick={onDeleteSelected}
          className="inline-flex items-center gap-1 px-2 py-1 rounded-[6px] text-red-500 hover:bg-red-500/10 disabled:opacity-40 cursor-pointer"
          title="Delete Selected Feature(s)"
        >
          <Trash2 className="size-3.5" />
        </button>

        <div className="h-4 w-[1px] bg-neutral-200 dark:bg-[#303030] mx-0.5" />

        <button
          type="button"
          disabled={!canUndo || isLocked}
          onClick={onUndo}
          className="p-1.5 rounded-[6px] text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#262626] disabled:opacity-40 cursor-pointer"
          title="Undo Edit"
        >
          <Undo2 className="size-3.5" />
        </button>
        <button
          type="button"
          disabled={!canRedo || isLocked}
          onClick={onRedo}
          className="p-1.5 rounded-[6px] text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#262626] disabled:opacity-40 cursor-pointer"
          title="Redo Edit"
        >
          <Redo2 className="size-3.5" />
        </button>

        <button
          type="button"
          onClick={onToggleSnap}
          className={`inline-flex items-center gap-1 px-2 py-1 rounded-[6px] font-medium transition-colors cursor-pointer ${
            snapEnabled
              ? "bg-sky-500/20 text-sky-400 border border-sky-500/40"
              : "text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-[#262626]"
          }`}
          title="Snap vertices to nearby geometry"
        >
          <Magnet className="size-3.5" />
          <span className="text-[11px]">Snap</span>
        </button>
      </div>

      {hasDraft && (
        <div className="flex flex-wrap items-center gap-2 px-3 py-1.5 rounded-[8px] bg-neutral-900/95 text-white border border-[#3ecf8e]/40 shadow-lg text-xs">
          <span className="font-mono text-[11px] text-neutral-300">
            Vertices: <strong>{draftCoordinates.length}</strong>
          </span>
          {activeTool === "vertex" && (
            <span className="text-[11px] text-emerald-300/90">
              Drag vertices to resize • Hold Shift to scale proportionally • Drag midpoints to add • Alt+Click to remove
            </span>
          )}
          {validationResult && !validationResult.is_valid && (
            <span className="inline-flex items-center gap-1 text-amber-400 text-[11px]">
              <AlertTriangle className="size-3.5" />
              {validationResult.reason}
            </span>
          )}
          <button
            type="button"
            onClick={onConfirmSaveDraft}
            className="inline-flex items-center gap-1 px-2.5 py-1 rounded bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-[11px] cursor-pointer"
          >
            <Check className="size-3.5" />
            <span>{activeTool === "vertex" ? "Done Editing" : "Confirm & Save Feature"}</span>
          </button>
          <button
            type="button"
            onClick={onCancelDraft}
            className="inline-flex items-center gap-1 px-2 py-1 rounded bg-neutral-800 hover:bg-neutral-700 text-neutral-300 text-[11px] cursor-pointer"
          >
            <X className="size-3.5" />
            <span>Cancel</span>
          </button>
        </div>
      )}
    </div>
  )
}
