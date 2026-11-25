import React, { useMemo, useState } from "react"
import {
  ChevronDown,
  ChevronRight,
  Eye,
  EyeOff,
  GripVertical,
  Plus,
  MoreHorizontal,
  Shapes,
  Image as ImageIcon,
  Globe,
  Crosshair,
  Palette,
  Table2,
  Edit2,
  Trash2,
  Copy,
  SlidersHorizontal,
  Compass,
  Check,
  X,
  FolderPlus,
} from "lucide-react"
import {
  createMapLayer,
  updateGISLayer,
  deleteGISLayer,
  reorderMapLayers,
  updateGISLayerVisibility,
  updateGISLayerOpacity,
  duplicateGISLayer,
} from "../../api"

export default function ThreeGroupLayersPanel({
  mapId,
  gisLayers = [],
  members = [],
  activeGISLayer,
  activeBaseLayer,
  mapViewPrefs = {},
  onSelectGISLayer,
  onLayerUpdatedImmediate,
  onRefreshGISLayers,
  onOpenAttributeTable,
  onOpenStyleEditor,
  onZoomToLayer,
  onUpdateMember,
  onReorderMembers,
  onDetachMember,
  onGeoreferenceMember,
  onOpenTransformMember,
  onOpenEnhanceMember,
  onOpenAddVector,
  onOpenAddRaster,
  onOpenChangeBaseLayer,
  onChangeBasePrefs,
  onCollapse,
  notify,
}) {
  const [expandedGroups, setExpandedGroups] = useState({
    vector: true,
    raster: true,
    base: true,
  })
  const [renamingId, setRenamingId] = useState(null)
  const [renameDraft, setRenameDraft] = useState("")
  const [opacityOpenId, setOpacityOpenId] = useState(null)
  const [contextMenu, setContextMenu] = useState(null) // { kind: "vector"|"raster"|"base", item, x, y }
  const [creatingVector, setCreatingVector] = useState(false)
  const [newVectorName, setNewVectorName] = useState("")
  const [newVectorGeom, setNewVectorGeom] = useState("Polygon")
  const [dragState, setDragState] = useState(null) // { group: "vector"|"raster", id }

  const vectorLayers = useMemo(
    () =>
      [...gisLayers]
        .filter((l) => l.layer_type === "vector" || !l.layer_type)
        .sort((a, b) => (a.display_order ?? 0) - (b.display_order ?? 0)),
    [gisLayers]
  )

  // Combine workspace raster members and any standalone raster GIS layers without duplicating by dataset_id
  const rasterEntries = useMemo(() => {
    const sortedMembers = [...members].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))
    const memberIds = new Set(sortedMembers.map((m) => m.dataset_id))
    const list = sortedMembers.map((m) => ({
      entryKind: "member",
      id: m.dataset_id,
      name: m.name || m.dataset?.filename || m.dataset_id,
      visible: m.visible !== false,
      opacity: typeof m.opacity === "number" ? m.opacity : 1.0,
      bounds: m.bounds || m.dataset?.extracted_metadata?.bounds || null,
      isGeoreferenced: Boolean(
        m.map_layer || m.dataset?.map_layer || m.dataset?.georeference_status === "completed" || m.bounds
      ),
      raw: m,
    }))
    gisLayers
      .filter((l) => l.layer_type === "raster" && (!l.dataset_id || !memberIds.has(l.dataset_id)))
      .sort((a, b) => (a.display_order ?? 0) - (b.display_order ?? 0))
      .forEach((l) => {
        list.push({
          entryKind: "gis_raster",
          id: l.layer_id,
          name: l.layer_name,
          visible: l.is_visible !== false,
          opacity: typeof l.opacity === "number" ? l.opacity : 1.0,
          bounds: l.bounds || null,
          isGeoreferenced: Boolean(Array.isArray(l.bounds) && l.bounds.length === 4),
          raw: l,
        })
      })
    return list
  }, [members, gisLayers])

  const allVectorsVisible = vectorLayers.length > 0 && vectorLayers.every((l) => l.is_visible !== false)
  const allRastersVisible = rasterEntries.length > 0 && rasterEntries.every((r) => r.visible)
  const baseVisible = mapViewPrefs?.base_layer_visible !== false
  const baseOpacity = typeof mapViewPrefs?.base_layer_opacity === "number" ? mapViewPrefs.base_layer_opacity : 1.0

  const toggleGroupVisibility = async (group) => {
    if (group === "vector") {
      const targetVis = !allVectorsVisible
      for (const l of vectorLayers) {
        onLayerUpdatedImmediate?.(l.layer_id, { is_visible: targetVis })
        updateGISLayerVisibility(l.layer_id, targetVis).catch(() => {})
      }
      setTimeout(() => onRefreshGISLayers?.(), 150)
    } else if (group === "raster") {
      const targetVis = !allRastersVisible
      for (const r of rasterEntries) {
        if (r.entryKind === "member") {
          onUpdateMember?.(r.id, { visible: targetVis })
        } else {
          onLayerUpdatedImmediate?.(r.id, { is_visible: targetVis })
          updateGISLayerVisibility(r.id, targetVis).catch(() => {})
        }
      }
      setTimeout(() => onRefreshGISLayers?.(), 150)
    } else if (group === "base") {
      onChangeBasePrefs?.({ base_layer_visible: !baseVisible })
    }
  }

  const handleToggleVectorVis = async (layer, e) => {
    e?.stopPropagation()
    const next = layer.is_visible === false
    onLayerUpdatedImmediate?.(layer.layer_id, { is_visible: next })
    try {
      await updateGISLayerVisibility(layer.layer_id, next)
      onRefreshGISLayers?.()
    } catch {
      notify?.("Failed updating layer visibility")
    }
  }

  const handleToggleRasterVis = async (entry, e) => {
    e?.stopPropagation()
    const next = !entry.visible
    if (entry.entryKind === "member") {
      onUpdateMember?.(entry.id, { visible: next })
      // Also sync any matching GIS raster layer
      const matching = gisLayers.find((l) => l.layer_type === "raster" && l.dataset_id === entry.id)
      if (matching) {
        onLayerUpdatedImmediate?.(matching.layer_id, { is_visible: next })
        updateGISLayerVisibility(matching.layer_id, next).catch(() => {})
      }
    } else {
      onLayerUpdatedImmediate?.(entry.id, { is_visible: next })
      await updateGISLayerVisibility(entry.id, next).catch(() => {})
      onRefreshGISLayers?.()
    }
  }

  const handleVectorOpacity = async (layer, pct) => {
    const val = Math.max(0, Math.min(1, Number(pct) / 100))
    onLayerUpdatedImmediate?.(layer.layer_id, { opacity: val })
    try {
      await updateGISLayerOpacity(layer.layer_id, val)
    } catch {}
  }

  const handleRasterOpacity = async (entry, pct) => {
    const val = Math.max(0, Math.min(1, Number(pct) / 100))
    if (entry.entryKind === "member") {
      onUpdateMember?.(entry.id, { opacity: val })
    } else {
      onLayerUpdatedImmediate?.(entry.id, { opacity: val })
      await updateGISLayerOpacity(entry.id, val).catch(() => {})
    }
  }

  const commitRename = async (kind, item) => {
    const clean = renameDraft.trim()
    setRenamingId(null)
    if (!clean) return
    if (kind === "vector" || (kind === "raster" && item.entryKind === "gis_raster")) {
      const lid = kind === "vector" ? item.layer_id : item.id
      if (clean === (item.layer_name || item.name)) return
      try {
        await updateGISLayer(lid, { layer_name: clean })
        notify?.(`Renamed layer to "${clean}"`)
        onRefreshGISLayers?.()
      } catch {
        notify?.("Failed renaming layer")
      }
    } else if (kind === "raster" && item.entryKind === "member") {
      if (clean === item.name) return
      onUpdateMember?.(item.id, { name: clean })
    }
  }

  const handleDeleteVector = async (layer) => {
    setContextMenu(null)
    if (!window.confirm(`Remove layer "${layer.layer_name}" from this workspace? Sibling layers and uploaded files are preserved.`)) {
      return
    }
    try {
      await deleteGISLayer(layer.layer_id)
      notify?.(`Removed "${layer.layer_name}"`)
      onRefreshGISLayers?.()
    } catch {
      notify?.("Failed removing layer")
    }
  }

  const handleDuplicateVector = async (layer) => {
    setContextMenu(null)
    try {
      await duplicateGISLayer(layer.layer_id)
      notify?.(`Duplicated "${layer.layer_name}"`)
      onRefreshGISLayers?.()
    } catch {
      notify?.("Failed duplicating layer")
    }
  }

  const handleCreateVectorLayer = async (e) => {
    e.preventDefault()
    const name = newVectorName.trim()
    if (!name || !mapId) return
    try {
      const created = await createMapLayer(mapId, {
        layer_name: name,
        layer_type: "vector",
        geometry_type: newVectorGeom,
        crs: "EPSG:4326",
      })
      setNewVectorName("")
      setCreatingVector(false)
      notify?.(`Created vector layer "${name}"`)
      await onRefreshGISLayers?.()
      if (created) onSelectGISLayer?.(created)
    } catch {
      notify?.("Failed creating vector layer")
    }
  }

  // Strictly intra-group drag-and-drop reordering (Vector cannot move into Raster or Base!)
  const handleDropWithinGroup = async (group, targetId) => {
    if (!dragState || dragState.group !== group || dragState.id === targetId) {
      setDragState(null)
      return
    }
    if (group === "vector") {
      const ids = vectorLayers.map((l) => l.layer_id)
      const from = ids.indexOf(dragState.id)
      const to = ids.indexOf(targetId)
      if (from !== -1 && to !== -1) {
        const next = [...ids]
        const [moved] = next.splice(from, 1)
        next.splice(to, 0, moved)
        // Preserve raster GIS layers after vector order
        const rasterIds = gisLayers.filter((l) => l.layer_type === "raster").map((l) => l.layer_id)
        const combined = [...next, ...rasterIds]
        onLayerUpdatedImmediate?.(null, { reorderIds: combined })
        try {
          await reorderMapLayers(mapId, combined)
          onRefreshGISLayers?.()
        } catch {}
      }
    } else if (group === "raster") {
      const memberIds = rasterEntries.filter((r) => r.entryKind === "member").map((r) => r.id)
      const from = memberIds.indexOf(dragState.id)
      const to = memberIds.indexOf(targetId)
      if (from !== -1 && to !== -1) {
        const next = [...memberIds]
        const [moved] = next.splice(from, 1)
        next.splice(to, 0, moved)
        onReorderMembers?.(next)
      }
    }
    setDragState(null)
  }

  const openCtx = (e, kind, item) => {
    e.preventDefault()
    e.stopPropagation()
    setContextMenu({
      kind,
      item,
      x: Math.min(e.clientX, window.innerWidth - 210),
      y: Math.min(e.clientY, window.innerHeight - 240),
    })
  }

  const GroupHeader = ({ id, title, icon: Icon, count, visible, onToggleVis, onAdd, extraAction }) => {
    const open = expandedGroups[id]
    return (
      <div className="flex items-center gap-2 px-2.5 py-2 bg-neutral-100/90 dark:bg-[#242424] border-y border-neutral-200 dark:border-[#303030] text-[12px] font-semibold text-neutral-800 dark:text-neutral-100 select-none">
        <button
          type="button"
          onClick={() => setExpandedGroups((p) => ({ ...p, [id]: !p[id] }))}
          className="text-neutral-500 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
          aria-label={`Toggle ${title} group`}
        >
          {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
        </button>
        <button
          type="button"
          onClick={onToggleVis}
          className="text-neutral-500 hover:text-[#3ecf8e] cursor-pointer"
          title={visible ? `Hide all ${title}` : `Show all ${title}`}
        >
          {visible ? <Eye className="size-3.5 text-[#3ecf8e]" /> : <EyeOff className="size-3.5 opacity-45" />}
        </button>
        <Icon className="size-3.5 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
        <span className="truncate flex-1">{title}</span>
        <span className="px-1.5 py-0.5 rounded bg-neutral-200 dark:bg-[#333333] text-[10px] font-mono text-neutral-600 dark:text-neutral-300">
          {count}
        </span>
        {extraAction}
        <button
          type="button"
          onClick={onAdd}
          className="p-1 rounded hover:bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e] cursor-pointer"
          title={`Add to ${title}`}
        >
          <Plus className="size-3.5" />
        </button>
      </div>
    )
  }

  return (
    <div
      className="flex flex-col h-full min-h-0 bg-white dark:bg-[#1e1e1e] text-neutral-800 dark:text-neutral-200 relative"
      onClick={() => contextMenu && setContextMenu(null)}
    >
      {/* Panel Header */}
      <div className="h-9 px-3 border-b border-neutral-200 dark:border-[#303030] bg-neutral-100 dark:bg-[#252525] flex items-center justify-between gap-2 shrink-0">
        <span className="text-[11px] font-bold uppercase tracking-wider text-neutral-700 dark:text-neutral-200">
          Layers
        </span>
        <div className="flex items-center gap-1.5">
          <span className="text-[10px] font-mono text-neutral-400">
            V:{vectorLayers.length} R:{rasterEntries.length} B:1
          </span>
          {onCollapse && (
            <button
              type="button"
              onClick={onCollapse}
              className="p-1.5 rounded-[5px] hover:bg-neutral-200 dark:hover:bg-[#353535] text-neutral-500 dark:text-neutral-300 cursor-pointer"
              title="Hide Layers Panel"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Permanent Three-Group Stack: 1. Vector (Top) -> 2. Raster (Middle) -> 3. Base (Bottom) */}
      <div className="flex-1 overflow-y-auto min-h-0">
        {/* GROUP 1 — VECTOR LAYER (TOP) */}
        <GroupHeader
          id="vector"
          title="Vector Layer"
          icon={Shapes}
          count={vectorLayers.length}
          visible={allVectorsVisible}
          onToggleVis={() => toggleGroupVisibility("vector")}
          onAdd={onOpenAddVector}
          extraAction={
            <button
              type="button"
              onClick={() => setCreatingVector((p) => !p)}
              className="p-0.5 rounded hover:bg-neutral-200 dark:hover:bg-[#353535] text-neutral-500 dark:text-neutral-300 cursor-pointer"
              title="Create New Empty Vector Layer"
            >
              <FolderPlus className="size-3" />
            </button>
          }
        />
        {expandedGroups.vector && (
          <div className="p-1 space-y-0.5">
            {creatingVector && (
              <form
                onSubmit={handleCreateVectorLayer}
                className="p-1.5 rounded bg-neutral-50 dark:bg-[#171717] border border-[#3ecf8e]/40 space-y-1.5 mb-1"
              >
                <input
                  type="text"
                  value={newVectorName}
                  onChange={(e) => setNewVectorName(e.target.value)}
                  placeholder="New vector layer name…"
                  className="w-full h-6 px-2 text-[11px] rounded bg-white dark:bg-[#222222] border border-neutral-300 dark:border-[#383838] outline-none focus:border-[#3ecf8e]"
                  autoFocus
                />
                <div className="flex items-center gap-1">
                  <select
                    value={newVectorGeom}
                    onChange={(e) => setNewVectorGeom(e.target.value)}
                    className="flex-1 h-6 px-1.5 text-[10.5px] rounded bg-white dark:bg-[#222222] border border-neutral-300 dark:border-[#383838]"
                  >
                    <option value="Polygon">Polygon</option>
                    <option value="LineString">LineString</option>
                    <option value="Point">Point</option>
                  </select>
                  <button
                    type="submit"
                    className="h-6 px-2 rounded bg-[#3ecf8e] text-[#141414] text-[10.5px] font-bold cursor-pointer"
                  >
                    Create
                  </button>
                  <button
                    type="button"
                    onClick={() => setCreatingVector(false)}
                    className="h-6 px-1.5 rounded text-[10.5px] text-neutral-500 hover:bg-neutral-200 dark:hover:bg-[#303030] cursor-pointer"
                  >
                    ✕
                  </button>
                </div>
              </form>
            )}

            {vectorLayers.length === 0 ? (
              <div className="px-2 py-2 text-[10.5px] text-neutral-400 flex items-center justify-between">
                <span>No vector layers</span>
                <button
                  type="button"
                  onClick={onOpenAddVector}
                  className="text-[#24b47e] dark:text-[#3ecf8e] hover:underline font-semibold cursor-pointer"
                >
                  + Add Vector
                </button>
              </div>
            ) : (
              vectorLayers.map((layer) => {
                const isActive = activeGISLayer?.layer_id === layer.layer_id
                const isRenaming = renamingId === `v:${layer.layer_id}`
                const showOp = opacityOpenId === `v:${layer.layer_id}`
                const color =
                  layer.style?.maplibre_style?.fill?.["fill-color"] ||
                  layer.style?.maplibre_style?.circle?.["circle-color"] ||
                  layer.style?.maplibre_style?.line?.["line-color"] ||
                  "#3ecf8e"
                return (
                  <div
                    key={layer.layer_id}
                    draggable
                    onDragStart={() => setDragState({ group: "vector", id: layer.layer_id })}
                    onDragOver={(e) => {
                      if (dragState?.group === "vector") e.preventDefault()
                    }}
                    onDrop={() => handleDropWithinGroup("vector", layer.layer_id)}
                    onContextMenu={(e) => openCtx(e, "vector", layer)}
                    className={`rounded-[4px] border transition-colors ${
                      isActive
                        ? "border-[#3ecf8e] bg-[#3ecf8e]/10"
                        : "border-transparent hover:bg-neutral-100 dark:hover:bg-[#2a2a2a]"
                    }`}
                  >
                    <div
                      onClick={() => onSelectGISLayer?.(layer)}
                      className="flex items-center gap-1 px-1.5 py-1 text-[11px] cursor-pointer"
                    >
                      <GripVertical className="size-3 text-neutral-400 cursor-grab shrink-0" />
                      <button
                        type="button"
                        onClick={(e) => handleToggleVectorVis(layer, e)}
                        className="text-neutral-500 hover:text-[#3ecf8e] cursor-pointer shrink-0"
                        title={layer.is_visible !== false ? "Hide layer" : "Show layer"}
                      >
                        {layer.is_visible !== false ? (
                          <Eye className="size-3.5 text-[#3ecf8e]" />
                        ) : (
                          <EyeOff className="size-3.5 opacity-45" />
                        )}
                      </button>
                      <span
                        className="size-2.5 rounded-[2px] border border-black/25 shrink-0"
                        style={{ backgroundColor: color }}
                        title={layer.geometry_type || "Vector"}
                      />

                      <div className="flex-1 min-w-0">
                        {isRenaming ? (
                          <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
                            <input
                              type="text"
                              value={renameDraft}
                              onChange={(e) => setRenameDraft(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") commitRename("vector", layer)
                                if (e.key === "Escape") setRenamingId(null)
                              }}
                              className="w-full h-5 px-1 text-[11px] rounded bg-white dark:bg-[#161616] border border-[#3ecf8e] outline-none"
                              autoFocus
                            />
                            <button
                              type="button"
                              onClick={() => commitRename("vector", layer)}
                              className="text-[#3ecf8e] cursor-pointer"
                            >
                              <Check className="size-3" />
                            </button>
                          </div>
                        ) : (
                          <div className="truncate font-medium text-neutral-800 dark:text-neutral-100">
                            {layer.layer_name}
                          </div>
                        )}
                      </div>

                      {(layer.metadata?.vector_tiles || layer.rendering_source?.vector_tiles) && (
                        <span
                          className="px-1 py-0.2 rounded text-[8.5px] font-mono font-bold bg-[#3ecf8e]/15 text-[#24b47e] dark:text-[#3ecf8e] shrink-0"
                          title="Local XYZ MVT Vector Tiles"
                        >
                          MVT
                        </span>
                      )}

                      <span className="text-[9.5px] font-mono text-neutral-400 shrink-0">
                        {layer.feature_count ?? 0}
                      </span>

                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          setOpacityOpenId(showOp ? null : `v:${layer.layer_id}`)
                        }}
                        className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
                        title="Opacity"
                      >
                        <SlidersHorizontal className="size-3" />
                      </button>
                      <button
                        type="button"
                        onClick={(e) => openCtx(e, "vector", layer)}
                        className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
                        title="Layer actions"
                      >
                        <MoreHorizontal className="size-3.5" />
                      </button>
                    </div>

                    {showOp && (
                      <div className="px-6 pb-1.5 flex items-center gap-2 text-[10px] text-neutral-500">
                        <span>Opacity</span>
                        <input
                          type="range"
                          min="0"
                          max="100"
                          value={Math.round((layer.opacity ?? 1) * 100)}
                          onChange={(e) => handleVectorOpacity(layer, e.target.value)}
                          className="flex-1 accent-[#3ecf8e] h-1"
                        />
                        <span className="font-mono w-7 text-right">
                          {Math.round((layer.opacity ?? 1) * 100)}%
                        </span>
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
        )}

        {/* GROUP 2 — RASTER LAYER (MIDDLE) */}
        <GroupHeader
          id="raster"
          title="Raster Layer"
          icon={ImageIcon}
          count={rasterEntries.length}
          visible={allRastersVisible}
          onToggleVis={() => toggleGroupVisibility("raster")}
          onAdd={onOpenAddRaster}
        />
        {expandedGroups.raster && (
          <div className="p-1 space-y-0.5">
            {rasterEntries.length === 0 ? (
              <div className="px-2 py-2 text-[10.5px] text-neutral-400 flex items-center justify-between">
                <span>No raster layers</span>
                <button
                  type="button"
                  onClick={onOpenAddRaster}
                  className="text-sky-500 dark:text-sky-400 hover:underline font-semibold cursor-pointer"
                >
                  + Add Raster
                </button>
              </div>
            ) : (
              rasterEntries.map((entry) => {
                const isRenaming = renamingId === `r:${entry.id}`
                const showOp = opacityOpenId === `r:${entry.id}`
                return (
                  <div
                    key={entry.id}
                    draggable={entry.entryKind === "member"}
                    onDragStart={() => setDragState({ group: "raster", id: entry.id })}
                    onDragOver={(e) => {
                      if (dragState?.group === "raster") e.preventDefault()
                    }}
                    onDrop={() => handleDropWithinGroup("raster", entry.id)}
                    onContextMenu={(e) => openCtx(e, "raster", entry)}
                    className="rounded-[4px] border border-transparent hover:bg-neutral-100 dark:hover:bg-[#2a2a2a] transition-colors"
                  >
                    <div className="flex items-center gap-1 px-1.5 py-1 text-[11px]">
                      <GripVertical className="size-3 text-neutral-400 cursor-grab shrink-0" />
                      <button
                        type="button"
                        onClick={(e) => handleToggleRasterVis(entry, e)}
                        className="text-neutral-500 hover:text-[#3ecf8e] cursor-pointer shrink-0"
                        title={entry.visible ? "Hide raster" : "Show raster"}
                      >
                        {entry.visible ? (
                          <Eye className="size-3.5 text-sky-500" />
                        ) : (
                          <EyeOff className="size-3.5 opacity-45" />
                        )}
                      </button>
                      <ImageIcon className="size-3 text-sky-500 shrink-0" />

                      <div className="flex-1 min-w-0">
                        {isRenaming ? (
                          <div className="flex items-center gap-1">
                            <input
                              type="text"
                              value={renameDraft}
                              onChange={(e) => setRenameDraft(e.target.value)}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") commitRename("raster", entry)
                                if (e.key === "Escape") setRenamingId(null)
                              }}
                              className="w-full h-5 px-1 text-[11px] rounded bg-white dark:bg-[#161616] border border-[#3ecf8e] outline-none"
                              autoFocus
                            />
                            <button
                              type="button"
                              onClick={() => commitRename("raster", entry)}
                              className="text-[#3ecf8e] cursor-pointer"
                            >
                              <Check className="size-3" />
                            </button>
                          </div>
                        ) : (
                          <div className="truncate font-medium text-neutral-800 dark:text-neutral-100">
                            {entry.name}
                          </div>
                        )}
                      </div>

                      {!entry.isGeoreferenced && (
                        <span
                          className="px-1 py-0.2 rounded text-[9px] font-semibold bg-amber-500/15 text-amber-500 shrink-0"
                          title="Not georeferenced yet"
                        >
                          Unref
                        </span>
                      )}

                      <button
                        type="button"
                        onClick={() => setOpacityOpenId(showOp ? null : `r:${entry.id}`)}
                        className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
                        title="Raster Opacity"
                      >
                        <SlidersHorizontal className="size-3" />
                      </button>
                      <button
                        type="button"
                        onClick={(e) => openCtx(e, "raster", entry)}
                        className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
                        title="Raster actions"
                      >
                        <MoreHorizontal className="size-3.5" />
                      </button>
                    </div>

                    {showOp && (
                      <div className="px-6 pb-1.5 flex items-center gap-2 text-[10px] text-neutral-500">
                        <span>Opacity</span>
                        <input
                          type="range"
                          min="0"
                          max="100"
                          value={Math.round(entry.opacity * 100)}
                          onChange={(e) => handleRasterOpacity(entry, e.target.value)}
                          className="flex-1 accent-sky-500 h-1"
                        />
                        <span className="font-mono w-7 text-right">
                          {Math.round(entry.opacity * 100)}%
                        </span>
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
        )}

        {/* GROUP 3 — BASE LAYER (BOTTOM — PERMANENTLY ANCHORED) */}
        <GroupHeader
          id="base"
          title="Base Layer"
          icon={Globe}
          count={1}
          visible={baseVisible}
          onToggleVis={() => toggleGroupVisibility("base")}
          onAdd={onOpenChangeBaseLayer}
        />
        {expandedGroups.base && (
          <div className="p-1.5 space-y-1">
            <div className="rounded-[4px] border border-neutral-200 dark:border-[#303030] bg-neutral-50 dark:bg-[#191919] px-2 py-1.5 text-[11px]">
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => onChangeBasePrefs?.({ base_layer_visible: !baseVisible })}
                  className="text-neutral-500 hover:text-[#3ecf8e] cursor-pointer shrink-0"
                  title={baseVisible ? "Hide base layer" : "Show base layer"}
                >
                  {baseVisible ? (
                    <Eye className="size-3.5 text-amber-500" />
                  ) : (
                    <EyeOff className="size-3.5 opacity-45" />
                  )}
                </button>
                <Compass className="size-3.5 text-amber-500 shrink-0" />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-semibold text-neutral-900 dark:text-white">
                    {activeBaseLayer && !activeBaseLayer.is_default
                      ? activeBaseLayer.name
                      : "Default World Map"}
                  </div>
                  <div className="text-[9.5px] font-mono text-neutral-400">
                    View: {mapViewPrefs?.view_type || "world"} • {activeBaseLayer?.crs || "EPSG:3857"}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={onOpenChangeBaseLayer}
                  className="px-1.5 py-0.5 rounded bg-neutral-200 dark:bg-[#2c2c2c] hover:bg-[#3ecf8e] hover:text-[#141414] text-[10px] font-semibold cursor-pointer shrink-0"
                >
                  Change
                </button>
              </div>
              <div className="mt-1.5 flex items-center gap-2 text-[10px] text-neutral-500">
                <span>Opacity</span>
                <input
                  type="range"
                  min="0"
                  max="100"
                  value={Math.round(baseOpacity * 100)}
                  onChange={(e) =>
                    onChangeBasePrefs?.({ opacity: Math.max(0, Math.min(1, Number(e.target.value) / 100)) })
                  }
                  className="flex-1 accent-amber-500 h-1"
                />
                <span className="font-mono w-7 text-right">{Math.round(baseOpacity * 100)}%</span>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Layer Context Menu */}
      {contextMenu && (
        <div
          style={{ left: contextMenu.x, top: contextMenu.y }}
          className="fixed z-[1200] w-48 py-1 rounded-[6px] border border-neutral-300 dark:border-[#3c3c3c] bg-white dark:bg-[#242424] shadow-2xl text-[11.5px]"
          onClick={(e) => e.stopPropagation()}
        >
          {contextMenu.kind === "vector" && (
            <>
              <button
                type="button"
                onClick={() => {
                  onZoomToLayer?.(contextMenu.item)
                  setContextMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Crosshair className="size-3.5 text-[#3ecf8e]" /> Zoom to Extent
              </button>
              <button
                type="button"
                onClick={() => {
                  onOpenAttributeTable?.(contextMenu.item)
                  setContextMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Table2 className="size-3.5 text-sky-400" /> Attribute Table
              </button>
              <button
                type="button"
                onClick={() => {
                  onOpenStyleEditor?.(contextMenu.item)
                  setContextMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Palette className="size-3.5 text-purple-400" /> Properties &amp; Symbology
              </button>
              <button
                type="button"
                onClick={() => {
                  setRenamingId(`v:${contextMenu.item.layer_id}`)
                  setRenameDraft(contextMenu.item.layer_name)
                  setContextMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Edit2 className="size-3.5 text-neutral-400" /> Rename Layer
              </button>
              <button
                type="button"
                onClick={() => handleDuplicateVector(contextMenu.item)}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Copy className="size-3.5 text-neutral-400" /> Duplicate Layer
              </button>
              <div className="my-1 border-t border-neutral-200 dark:border-[#343434]" />
              <button
                type="button"
                onClick={() => handleDeleteVector(contextMenu.item)}
                className="w-full px-3 py-1.5 text-left text-red-500 hover:bg-red-500/10 flex items-center gap-2 cursor-pointer"
              >
                <Trash2 className="size-3.5" /> Remove from Workspace
              </button>
            </>
          )}

          {contextMenu.kind === "raster" && (
            <>
              <button
                type="button"
                onClick={() => {
                  onZoomToLayer?.(contextMenu.item)
                  setContextMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Crosshair className="size-3.5 text-sky-400" /> Zoom to Extent
              </button>
              <button
                type="button"
                onClick={() => {
                  onGeoreferenceMember?.(contextMenu.item)
                  setContextMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Compass className="size-3.5 text-[#3ecf8e]" /> Georeference Raster
              </button>
              {contextMenu.item.entryKind === "member" && (
                <>
                  <button
                    type="button"
                    onClick={() => {
                      onOpenTransformMember?.(contextMenu.item.raw)
                      setContextMenu(null)
                    }}
                    className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
                  >
                    <SlidersHorizontal className="size-3.5 text-neutral-400" /> Position &amp; Transform
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      onOpenEnhanceMember?.(contextMenu.item.raw)
                      setContextMenu(null)
                    }}
                    className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
                  >
                    <Palette className="size-3.5 text-purple-400" /> Image Enhancement
                  </button>
                </>
              )}
              <button
                type="button"
                onClick={() => {
                  setRenamingId(`r:${contextMenu.item.id}`)
                  setRenameDraft(contextMenu.item.name)
                  setContextMenu(null)
                }}
                className="w-full px-3 py-1.5 text-left hover:bg-[#3ecf8e]/15 flex items-center gap-2 cursor-pointer"
              >
                <Edit2 className="size-3.5 text-neutral-400" /> Rename Layer
              </button>
              <div className="my-1 border-t border-neutral-200 dark:border-[#343434]" />
              <button
                type="button"
                onClick={async () => {
                  const target = contextMenu.item
                  setContextMenu(null)
                  if (target.entryKind === "member") {
                    onDetachMember?.(target.raw)
                  } else {
                    await deleteGISLayer(target.id).catch(() => {})
                    onRefreshGISLayers?.()
                  }
                }}
                className="w-full px-3 py-1.5 text-left text-red-500 hover:bg-red-500/10 flex items-center gap-2 cursor-pointer"
              >
                <Trash2 className="size-3.5" /> Remove from Workspace
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
