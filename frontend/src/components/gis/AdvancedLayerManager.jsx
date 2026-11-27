import React, { useState } from "react"
import {
  DndContext,
  PointerSensor,
  KeyboardSensor,
  closestCenter,
  useSensor,
  useSensors,
} from "@dnd-kit/core"
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable"
import { CSS } from "@dnd-kit/utilities"
import {
  ChevronDown,
  ChevronRight,
  Copy,
  Eye,
  EyeOff,
  FolderPlus,
  Folder,
  GripVertical,
  Image,
  Layers,
  Lock,
  Unlock,
  MoreVertical,
  Palette,
  Plus,
  Shapes,
  Table2,
  Trash2,
  Edit2,
  Check,
  X,
  FileCode,
} from "lucide-react"

import {
  createMapLayer,
  updateGISLayer,
  deleteGISLayer,
  reorderMapLayers,
  updateGISLayerVisibility,
  updateGISLayerOpacity,
  duplicateGISLayer,
  createLayerGroup,
  updateLayerGroup,
  deleteLayerGroup,
} from "../../api"

function LayerTypeIcon({ type }) {
  const t = String(type || "vector").toLowerCase()
  if (t === "raster") return <Image className="size-3.5 text-emerald-400 shrink-0" />
  if (t === "cad") return <FileCode className="size-3.5 text-amber-400 shrink-0" />
  return <Shapes className="size-3.5 text-sky-400 shrink-0" />
}

function SortableGISLayerRow({
  layer,
  isActive,
  groups,
  onSelectLayer,
  onRefresh,
  onOpenAttributeTable,
  onOpenStyleEditor,
  onLayerUpdatedImmediate,
  notify,
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: layer.layer_id,
  })
  const [menuOpen, setMenuOpen] = useState(false)
  const [editingName, setEditingName] = useState(false)
  const [draftName, setDraftName] = useState(layer.layer_name)
  const [expandedSettings, setExpandedSettings] = useState(false)
  const [minZoom, setMinZoom] = useState(layer.min_zoom ?? 0)
  const [maxZoom, setMaxZoom] = useState(layer.max_zoom ?? 24)

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.55 : 1,
  }

  const handleToggleVisibility = async (e) => {
    e.stopPropagation()
    const nextVis = !layer.is_visible
    onLayerUpdatedImmediate?.(layer.layer_id, { is_visible: nextVis })
    try {
      await updateGISLayerVisibility(layer.layer_id, nextVis)
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to update visibility", "error")
    }
  }

  const handleToggleLock = async (e) => {
    e.stopPropagation()
    const nextLock = !layer.is_locked
    try {
      await updateGISLayer(layer.layer_id, { is_locked: nextLock })
      notify?.(nextLock ? `Layer "${layer.layer_name}" locked` : `Layer "${layer.layer_name}" unlocked`)
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to toggle layer lock", "error")
    }
  }

  const handleOpacityChange = async (val) => {
    if (layer.is_locked) {
      notify?.("Unlock layer before changing opacity", "error")
      return
    }
    const num = Math.max(0, Math.min(1, Number(val) / 100))
    onLayerUpdatedImmediate?.(layer.layer_id, { opacity: num })
    try {
      await updateGISLayerOpacity(layer.layer_id, num)
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to update opacity", "error")
    }
  }

  const handleSaveRename = async () => {
    if (!draftName.trim() || draftName.trim() === layer.layer_name) {
      setEditingName(false)
      return
    }
    try {
      await updateGISLayer(layer.layer_id, { layer_name: draftName.trim() })
      setEditingName(false)
      notify?.("Layer renamed")
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to rename layer", "error")
    }
  }

  const handleSaveZoomRange = async () => {
    try {
      await updateGISLayer(layer.layer_id, {
        min_zoom: Number(minZoom),
        max_zoom: Number(maxZoom),
      })
      notify?.("Zoom visibility range saved")
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Invalid zoom range", "error")
    }
  }

  const handleDuplicate = async () => {
    setMenuOpen(false)
    try {
      await duplicateGISLayer(layer.layer_id)
      notify?.(`Duplicated "${layer.layer_name}"`)
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to duplicate layer", "error")
    }
  }

  const handleDelete = async () => {
    setMenuOpen(false)
    if (layer.is_locked) {
      notify?.("Layer is locked and cannot be deleted", "error")
      return
    }
    try {
      await deleteGISLayer(layer.layer_id)
      notify?.(`Deleted layer "${layer.layer_name}"`)
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to delete layer", "error")
    }
  }

  const handleMoveToGroup = async (groupId) => {
    setMenuOpen(false)
    try {
      await updateGISLayer(layer.layer_id, { parent_group_id: groupId })
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to move layer", "error")
    }
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`rounded-[6px] border transition-colors ${
        isActive
          ? "border-[#3ecf8e] bg-[#3ecf8e]/10"
          : "border-neutral-200 dark:border-[#2a2a2a] bg-white dark:bg-[#1b1b1b] hover:border-neutral-300 dark:hover:border-[#383838]"
      }`}
    >
      <div
        className="flex items-center gap-1.5 px-2 py-1.5 cursor-pointer select-none"
        onClick={() => onSelectLayer?.(layer)}
      >
        <button
          type="button"
          className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-grab active:cursor-grabbing"
          title="Drag to reorder layer"
          {...attributes}
          {...listeners}
          onClick={(e) => e.stopPropagation()}
        >
          <GripVertical className="size-3.5" />
        </button>

        <button
          type="button"
          onClick={handleToggleVisibility}
          className="text-neutral-500 hover:text-neutral-900 dark:hover:text-white"
          title={layer.is_visible ? "Hide layer" : "Show layer"}
        >
          {layer.is_visible ? <Eye className="size-3.5 text-[#3ecf8e]" /> : <EyeOff className="size-3.5 opacity-50" />}
        </button>

        <LayerTypeIcon type={layer.layer_type} />

        <div className="flex-1 min-w-0">
          {editingName ? (
            <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
              <input
                type="text"
                value={draftName}
                onChange={(e) => setDraftName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleSaveRename()
                  if (e.key === "Escape") setEditingName(false)
                }}
                className="w-full text-xs px-1.5 py-0.5 rounded bg-neutral-100 dark:bg-[#262626] text-neutral-900 dark:text-white border border-[#3ecf8e] outline-none"
                autoFocus
              />
              <button type="button" onClick={handleSaveRename} className="text-[#3ecf8e]">
                <Check className="size-3.5" />
              </button>
              <button type="button" onClick={() => setEditingName(false)} className="text-neutral-400">
                <X className="size-3.5" />
              </button>
            </div>
          ) : (
            <div className="flex items-center gap-1.5 min-w-0">
              <span className="text-xs font-medium text-neutral-900 dark:text-neutral-100 truncate">
                {layer.layer_name}
              </span>
              {layer.layer_type === "vector" && (
                <span className="text-[10px] font-mono px-1 py-0.2 rounded bg-neutral-100 dark:bg-[#262626] text-neutral-500 dark:text-neutral-400 shrink-0">
                  {layer.feature_count ?? 0}
                </span>
              )}
            </div>
          )}
        </div>

        <button
          type="button"
          onClick={handleToggleLock}
          className={`p-1 rounded hover:bg-neutral-100 dark:hover:bg-[#282828] ${
            layer.is_locked ? "text-amber-400" : "text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200"
          }`}
          title={layer.is_locked ? "Layer is locked (click to unlock)" : "Lock layer"}
        >
          {layer.is_locked ? <Lock className="size-3" /> : <Unlock className="size-3" />}
        </button>

        {layer.layer_type === "vector" && (
          <>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                onOpenAttributeTable?.(layer)
              }}
              className="p-1 rounded text-neutral-400 hover:text-[#3ecf8e] hover:bg-neutral-100 dark:hover:bg-[#282828]"
              title="Open Attribute Table"
            >
              <Table2 className="size-3.5" />
            </button>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                onOpenStyleEditor?.(layer)
              }}
              className="p-1 rounded text-neutral-400 hover:text-[#3ecf8e] hover:bg-neutral-100 dark:hover:bg-[#282828]"
              title="Open Symbology & Style Editor"
            >
              <Palette className="size-3.5" />
            </button>
          </>
        )}

        <div className="relative" onClick={(e) => e.stopPropagation()}>
          <button
            type="button"
            onClick={() => setMenuOpen((prev) => !prev)}
            className="p-1 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#282828]"
            title="Layer options"
          >
            <MoreVertical className="size-3.5" />
          </button>

          {menuOpen && (
            <div className="absolute right-0 top-7 z-50 w-48 rounded-[8px] border border-neutral-200 dark:border-[#303030] bg-white dark:bg-[#1f1f1f] shadow-xl py-1 text-xs">
              <button
                type="button"
                onClick={() => {
                  setMenuOpen(false)
                  setExpandedSettings((prev) => !prev)
                }}
                className="w-full px-3 py-1.5 text-left flex items-center gap-2 hover:bg-neutral-100 dark:hover:bg-[#2b2b2b]"
              >
                <Layers className="size-3.5 text-[#3ecf8e]" />
                <span>Opacity &amp; Zoom Range</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  setMenuOpen(false)
                  setDraftName(layer.layer_name)
                  setEditingName(true)
                }}
                disabled={layer.is_locked}
                className="w-full px-3 py-1.5 text-left flex items-center gap-2 hover:bg-neutral-100 dark:hover:bg-[#2b2b2b] disabled:opacity-40"
              >
                <Edit2 className="size-3.5" />
                <span>Rename Layer</span>
              </button>
              <button
                type="button"
                onClick={handleDuplicate}
                className="w-full px-3 py-1.5 text-left flex items-center gap-2 hover:bg-neutral-100 dark:hover:bg-[#2b2b2b]"
              >
                <Copy className="size-3.5" />
                <span>Duplicate Layer</span>
              </button>
              {groups.length > 0 && (
                <div className="border-t border-neutral-200 dark:border-[#2e2e2e] my-1 pt-1">
                  <div className="px-3 py-0.5 text-[10px] uppercase tracking-wider text-neutral-400">Move to Group</div>
                  <button
                    type="button"
                    onClick={() => handleMoveToGroup("")}
                    className="w-full px-3 py-1 text-left text-[11px] hover:bg-neutral-100 dark:hover:bg-[#2b2b2b]"
                  >
                    (Root / Ungrouped)
                  </button>
                  {groups.map((g) => (
                    <button
                      key={g.group_id}
                      type="button"
                      onClick={() => handleMoveToGroup(g.group_id)}
                      className="w-full px-3 py-1 text-left text-[11px] truncate hover:bg-neutral-100 dark:hover:bg-[#2b2b2b]"
                    >
                      {g.group_name}
                    </button>
                  ))}
                </div>
              )}
              <div className="border-t border-neutral-200 dark:border-[#2e2e2e] my-1 pt-1">
                <button
                  type="button"
                  onClick={handleDelete}
                  disabled={layer.is_locked}
                  className="w-full px-3 py-1.5 text-left flex items-center gap-2 text-red-500 hover:bg-red-500/10 disabled:opacity-40"
                >
                  <Trash2 className="size-3.5" />
                  <span>Delete Layer</span>
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {expandedSettings && (
        <div
          className="px-3 pb-2.5 pt-1 border-t border-neutral-200 dark:border-[#282828] space-y-2 text-xs"
          onClick={(e) => e.stopPropagation()}
        >
          <div>
            <div className="flex justify-between text-[11px] text-neutral-400 mb-1">
              <span>Opacity</span>
              <span className="font-mono text-neutral-900 dark:text-white">{Math.round((layer.opacity ?? 1) * 100)}%</span>
            </div>
            <input
              type="range"
              min={0}
              max={100}
              value={Math.round((layer.opacity ?? 1) * 100)}
              onChange={(e) => handleOpacityChange(e.target.value)}
              disabled={layer.is_locked}
              className="w-full accent-[#3ecf8e] cursor-pointer"
            />
          </div>

          <div className="grid grid-cols-2 gap-2">
            <label className="flex flex-col gap-0.5 text-[10px] text-neutral-400">
              <span>Min Zoom (0–24)</span>
              <input
                type="number"
                min={0}
                max={24}
                step={0.5}
                value={minZoom}
                onChange={(e) => setMinZoom(e.target.value)}
                className="px-2 py-1 rounded bg-neutral-100 dark:bg-[#252525] text-neutral-900 dark:text-white border border-neutral-300 dark:border-[#333]"
              />
            </label>
            <label className="flex flex-col gap-0.5 text-[10px] text-neutral-400">
              <span>Max Zoom (0–24)</span>
              <input
                type="number"
                min={0}
                max={24}
                step={0.5}
                value={maxZoom}
                onChange={(e) => setMaxZoom(e.target.value)}
                className="px-2 py-1 rounded bg-neutral-100 dark:bg-[#252525] text-neutral-900 dark:text-white border border-neutral-300 dark:border-[#333]"
              />
            </label>
          </div>
          <div className="flex items-center justify-between pt-1">
            <span className="text-[10px] font-mono text-neutral-400">CRS: {layer.crs || "EPSG:4326"}</span>
            <button
              type="button"
              onClick={handleSaveZoomRange}
              className="px-2.5 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold text-[11px] hover:bg-[#24b47e]"
            >
              Apply Zoom
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export default function AdvancedLayerManager({
  mapId,
  layers = [],
  groups = [],
  activeLayerId,
  onSelectLayer,
  onRefresh,
  onOpenAttributeTable,
  onOpenStyleEditor,
  onLayerUpdatedImmediate,
  notify,
}) {
  const [creatingLayer, setCreatingLayer] = useState(false)
  const [newLayerName, setNewLayerName] = useState("")
  const [newLayerType, setNewLayerType] = useState("vector")
  const [newGeomType, setNewGeomType] = useState("Polygon")
  const [newLayerGroupId, setNewLayerGroupId] = useState("")

  const [creatingGroup, setCreatingGroup] = useState(false)
  const [newGroupName, setNewGroupName] = useState("")
  const [newParentGroupId, setNewParentGroupId] = useState("")

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )

  const handleDragEnd = async (event) => {
    const { active, over } = event
    if (!over || active.id === over.id || !mapId) return
    const oldIndex = layers.findIndex((l) => l.layer_id === active.id)
    const newIndex = layers.findIndex((l) => l.layer_id === over.id)
    if (oldIndex < 0 || newIndex < 0) return

    const reordered = arrayMove(layers, oldIndex, newIndex)
    const orderedIds = reordered.map((l) => l.layer_id)
    try {
      await reorderMapLayers(mapId, orderedIds)
      onRefresh?.()
    } catch (err) {
      notify?.("Failed to reorder layers", "error")
    }
  }

  const handleCreateLayer = async (e) => {
    e.preventDefault()
    if (!newLayerName.trim() || !mapId) return
    try {
      const created = await createMapLayer(mapId, {
        layer_name: newLayerName.trim(),
        layer_type: newLayerType,
        geometry_type: newLayerType === "vector" ? newGeomType : "",
        parent_group_id: newLayerGroupId || "",
        crs: "EPSG:4326",
      })
      setNewLayerName("")
      setCreatingLayer(false)
      notify?.(`Created layer "${created.layer_name}"`)
      await onRefresh?.()
      onSelectLayer?.(created)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to create layer", "error")
    }
  }

  const handleCreateGroup = async (e) => {
    e.preventDefault()
    if (!newGroupName.trim() || !mapId) return
    try {
      await createLayerGroup(mapId, {
        group_name: newGroupName.trim(),
        parent_group_id: newParentGroupId || "",
      })
      setNewGroupName("")
      setCreatingGroup(false)
      notify?.("Created layer group")
      onRefresh?.()
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed to create group", "error")
    }
  }

  const handleToggleGroupExpand = async (grp) => {
    try {
      await updateLayerGroup(grp.group_id, { is_expanded: !grp.is_expanded })
      onRefresh?.()
    } catch {}
  }

  const handleDeleteGroup = async (groupId) => {
    try {
      await deleteLayerGroup(groupId)
      notify?.("Deleted layer group")
      onRefresh?.()
    } catch (err) {
      notify?.("Failed to delete group", "error")
    }
  }

  const ungroupedLayers = layers.filter((l) => !l.parent_group_id)

  const renderGroupNode = (grp, depth = 0) => {
    const childGroups = groups.filter((g) => g.parent_group_id === grp.group_id)
    const groupLayers = layers.filter((l) => l.parent_group_id === grp.group_id)

    return (
      <div
        key={grp.group_id}
        className="rounded-[6px] border border-neutral-200 dark:border-[#2a2a2a] bg-neutral-50/60 dark:bg-[#161616] p-1.5 space-y-1.5"
        style={{ marginLeft: depth * 10 }}
      >
        <div className="flex items-center justify-between gap-1 px-1">
          <button
            type="button"
            onClick={() => handleToggleGroupExpand(grp)}
            className="flex items-center gap-1.5 text-xs font-semibold text-neutral-800 dark:text-neutral-200 min-w-0"
          >
            {grp.is_expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
            <Folder className="size-3.5 text-[#3ecf8e] shrink-0" />
            <span className="truncate">{grp.group_name}</span>
            <span className="text-[10px] text-neutral-400 font-normal">({groupLayers.length})</span>
          </button>
          <button
            type="button"
            onClick={() => handleDeleteGroup(grp.group_id)}
            className="p-1 text-neutral-400 hover:text-red-500 rounded"
            title="Delete group (layers will be ungrouped)"
          >
            <Trash2 className="size-3" />
          </button>
        </div>

        {grp.is_expanded && (
          <div className="space-y-1 pl-2">
            {childGroups.map((sub) => renderGroupNode(sub, depth + 1))}
            {groupLayers.map((layer) => (
              <SortableGISLayerRow
                key={layer.layer_id}
                layer={layer}
                isActive={activeLayerId === layer.layer_id}
                groups={groups}
                onSelectLayer={onSelectLayer}
                onRefresh={onRefresh}
                onOpenAttributeTable={onOpenAttributeTable}
                onOpenStyleEditor={onOpenStyleEditor}
                onLayerUpdatedImmediate={onLayerUpdatedImmediate}
                notify={notify}
              />
            ))}
          </div>
        )}
      </div>
    )
  }

  const rootGroups = groups.filter((g) => !g.parent_group_id)

  return (
    <div className="space-y-2.5 p-2 border-t border-neutral-200 dark:border-[#2b2b2b]">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-1.5">
          <Layers className="size-3.5 text-[#3ecf8e]" />
          <span className="text-xs font-bold uppercase tracking-wider text-neutral-700 dark:text-neutral-300">
            GIS Layers ({layers.length})
          </span>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => {
              setCreatingGroup((p) => !p)
              setCreatingLayer(false)
            }}
            className="p-1 rounded hover:bg-neutral-200 dark:hover:bg-[#2a2a2a] text-neutral-600 dark:text-neutral-300"
            title="New Layer Group"
          >
            <FolderPlus className="size-3.5" />
          </button>
          <button
            type="button"
            onClick={() => {
              setCreatingLayer((p) => !p)
              setCreatingGroup(false)
            }}
            className="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-[11px]"
            title="Create New GIS Layer"
          >
            <Plus className="size-3" />
            <span>Layer</span>
          </button>
        </div>
      </div>

      {creatingGroup && (
        <form onSubmit={handleCreateGroup} className="p-2 rounded-[6px] bg-neutral-100 dark:bg-[#222] border border-neutral-200 dark:border-[#333] space-y-2 text-xs">
          <div className="font-semibold text-neutral-800 dark:text-neutral-200">Create Layer Group</div>
          <input
            type="text"
            placeholder="Group name (e.g. Base Infrastructure)"
            value={newGroupName}
            onChange={(e) => setNewGroupName(e.target.value)}
            className="w-full px-2 py-1 rounded bg-white dark:bg-[#181818] border border-neutral-300 dark:border-[#333] text-neutral-900 dark:text-white"
            required
          />
          {groups.length > 0 && (
            <select
              value={newParentGroupId}
              onChange={(e) => setNewParentGroupId(e.target.value)}
              className="w-full px-2 py-1 rounded bg-white dark:bg-[#181818] border border-neutral-300 dark:border-[#333] text-neutral-900 dark:text-white"
            >
              <option value="">Root Group (No Parent)</option>
              {groups.map((g) => (
                <option key={g.group_id} value={g.group_id}>
                  Subgroup of: {g.group_name}
                </option>
              ))}
            </select>
          )}
          <div className="flex justify-end gap-1.5">
            <button type="button" onClick={() => setCreatingGroup(false)} className="px-2 py-1 rounded text-neutral-500">
              Cancel
            </button>
            <button type="submit" className="px-2.5 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold">
              Create Group
            </button>
          </div>
        </form>
      )}

      {creatingLayer && (
        <form onSubmit={handleCreateLayer} className="p-2 rounded-[6px] bg-neutral-100 dark:bg-[#222] border border-neutral-200 dark:border-[#333] space-y-2 text-xs">
          <div className="font-semibold text-neutral-800 dark:text-neutral-200">Create GIS Layer</div>
          <input
            type="text"
            placeholder="Layer name (e.g. Zoning Polygons)"
            value={newLayerName}
            onChange={(e) => setNewLayerName(e.target.value)}
            className="w-full px-2 py-1 rounded bg-white dark:bg-[#181818] border border-neutral-300 dark:border-[#333] text-neutral-900 dark:text-white"
            required
          />
          <div className="grid grid-cols-2 gap-1.5">
            <select
              value={newLayerType}
              onChange={(e) => setNewLayerType(e.target.value)}
              className="px-2 py-1 rounded bg-white dark:bg-[#181818] border border-neutral-300 dark:border-[#333] text-neutral-900 dark:text-white"
            >
              <option value="vector">Vector Layer</option>
              <option value="raster">Raster Layer</option>
              <option value="cad">CAD Layer</option>
            </select>
            {newLayerType === "vector" && (
              <select
                value={newGeomType}
                onChange={(e) => setNewGeomType(e.target.value)}
                className="px-2 py-1 rounded bg-white dark:bg-[#181818] border border-neutral-300 dark:border-[#333] text-neutral-900 dark:text-white"
              >
                <option value="Polygon">Polygon</option>
                <option value="LineString">LineString</option>
                <option value="Point">Point</option>
              </select>
            )}
          </div>
          {groups.length > 0 && (
            <select
              value={newLayerGroupId}
              onChange={(e) => setNewLayerGroupId(e.target.value)}
              className="w-full px-2 py-1 rounded bg-white dark:bg-[#181818] border border-neutral-300 dark:border-[#333] text-neutral-900 dark:text-white"
            >
              <option value="">Ungrouped</option>
              {groups.map((g) => (
                <option key={g.group_id} value={g.group_id}>
                  Group: {g.group_name}
                </option>
              ))}
            </select>
          )}
          <div className="flex justify-end gap-1.5">
            <button type="button" onClick={() => setCreatingLayer(false)} className="px-2 py-1 rounded text-neutral-500">
              Cancel
            </button>
            <button type="submit" className="px-2.5 py-1 rounded bg-[#3ecf8e] text-[#171717] font-semibold">
              Add Layer
            </button>
          </div>
        </form>
      )}

      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={layers.map((l) => l.layer_id)} strategy={verticalListSortingStrategy}>
          <div className="space-y-1.5">
            {rootGroups.map((grp) => renderGroupNode(grp, 0))}
            {ungroupedLayers.map((layer) => (
              <SortableGISLayerRow
                key={layer.layer_id}
                layer={layer}
                isActive={activeLayerId === layer.layer_id}
                groups={groups}
                onSelectLayer={onSelectLayer}
                onRefresh={onRefresh}
                onOpenAttributeTable={onOpenAttributeTable}
                onOpenStyleEditor={onOpenStyleEditor}
                onLayerUpdatedImmediate={onLayerUpdatedImmediate}
                notify={notify}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>

      {layers.length === 0 && groups.length === 0 && !creatingLayer && (
        <div className="text-[11px] text-neutral-400 text-center py-3 border border-dashed border-neutral-300 dark:border-[#2e2e2e] rounded-[6px]">
          No custom GIS vector/CAD layers yet. Click <strong>+ Layer</strong> to start drawing or analyzing vector features.
        </div>
      )}
    </div>
  )
}
