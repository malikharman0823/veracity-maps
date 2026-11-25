import React, { useMemo, useState } from "react"
import {
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
  ArrowUp,
  ArrowDown,
  Lock,
  MapPin,
  Minus,
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

// Color-coded layer-type visual badges (Photoshop-style layer tags)
function getLayerTypeBadge(item) {
  if (item.kind === "raster") {
    return {
      label: "RASTER",
      accent: "#0ea5e9",
      pillClass: "bg-sky-500/15 text-sky-600 dark:text-sky-400 border-sky-500/30",
      Icon: ImageIcon,
    }
  }
  const gt = String(item.geometryType || "").toLowerCase()
  if (gt.includes("point")) {
    return {
      label: "POINT",
      accent: "#f59e0b",
      pillClass: "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30",
      Icon: MapPin,
    }
  }
  if (gt.includes("line")) {
    return {
      label: "LINE",
      accent: "#8b5cf6",
      pillClass: "bg-violet-500/15 text-violet-600 dark:text-violet-400 border-violet-500/30",
      Icon: Minus,
    }
  }
  return {
    label: "POLYGON",
    accent: "#3ecf8e",
    pillClass: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30",
    Icon: Shapes,
  }
}

export default function ThreeGroupLayersPanel({
  mapId,
  gisLayers = [],
  members = [],
  activeGISLayer,
  activeBaseLayer,
  mapViewPrefs = {},
  layerStack = [],
  onReorderLayerStack,
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
  const [renamingId, setRenamingId] = useState(null)
  const [renameDraft, setRenameDraft] = useState("")
  const [opacityOpenId, setOpacityOpenId] = useState(null)
  const [contextMenu, setContextMenu] = useState(null) // { kind: "vector"|"raster", item, x, y }
  const [creatingVector, setCreatingVector] = useState(false)
  const [newVectorName, setNewVectorName] = useState("")
  const [newVectorGeom, setNewVectorGeom] = useState("Polygon")
  const [dragKey, setDragKey] = useState(null)
  const [dropOverKey, setDropOverKey] = useState(null)

  const vectorLayers = useMemo(
    () =>
      [...gisLayers]
        .filter((l) => l.layer_type === "vector" || !l.layer_type)
        .sort((a, b) => (a.display_order ?? 0) - (b.display_order ?? 0)),
    [gisLayers]
  )

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

  // Build unified flat Photoshop-style stack (top-to-bottom), ordered by layerStack
  const unifiedItems = useMemo(() => {
    const byKey = new Map()
    vectorLayers.forEach((layer) => {
      const key = `vector:${layer.layer_id}`
      const swatchColor =
        layer.style?.maplibre_style?.fill?.["fill-color"] ||
        layer.style?.maplibre_style?.circle?.["circle-color"] ||
        layer.style?.maplibre_style?.line?.["line-color"] ||
        "#3ecf8e"
      byKey.set(key, {
        key,
        kind: "vector",
        id: layer.layer_id,
        name: layer.layer_name,
        visible: layer.is_visible !== false,
        opacity: typeof layer.opacity === "number" ? layer.opacity : 1.0,
        geometryType: layer.geometry_type || "Polygon",
        featureCount: layer.feature_count ?? 0,
        hasMVT: Boolean(layer.metadata?.vector_tiles || layer.rendering_source?.vector_tiles),
        swatchColor,
        layer,
      })
    })
    rasterEntries.forEach((entry) => {
      const key = `raster:${entry.id}`
      byKey.set(key, {
        key,
        kind: "raster",
        id: entry.id,
        name: entry.name,
        visible: entry.visible,
        opacity: entry.opacity,
        isGeoreferenced: entry.isGeoreferenced,
        entry,
      })
    })

    const ordered = []
    const used = new Set()
    if (Array.isArray(layerStack) && layerStack.length > 0) {
      layerStack.forEach((s) => {
        const k = s.key || `${s.kind}:${s.id}`
        if (byKey.has(k) && !used.has(k)) {
          ordered.push(byKey.get(k))
          used.add(k)
        }
      })
    }
    byKey.forEach((val, k) => {
      if (!used.has(k)) ordered.push(val)
    })
    return ordered
  }, [vectorLayers, rasterEntries, layerStack])

  const baseVisible = mapViewPrefs?.base_layer_visible !== false
  const baseOpacity = typeof mapViewPrefs?.base_layer_opacity === "number" ? mapViewPrefs.base_layer_opacity : 1.0

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

  // Commit unified Photoshop-style reorder and sync backend vector/member relative orders
  const commitUnifiedReorder = async (nextKeys) => {
    onReorderLayerStack?.(nextKeys)

    // Sync relative GIS vector order to backend
    const nextVectorIds = nextKeys
      .filter((k) => k.startsWith("vector:"))
      .map((k) => k.slice("vector:".length))
    if (nextVectorIds.length > 1 && mapId) {
      const rasterGisIds = gisLayers.filter((l) => l.layer_type === "raster").map((l) => l.layer_id)
      const combined = [...nextVectorIds, ...rasterGisIds]
      onLayerUpdatedImmediate?.(null, { reorderIds: combined })
      reorderMapLayers(mapId, combined).catch(() => {})
    }

    // Sync relative workspace raster member order
    const memberIdSet = new Set(rasterEntries.filter((r) => r.entryKind === "member").map((r) => r.id))
    const nextMemberIds = nextKeys
      .filter((k) => k.startsWith("raster:"))
      .map((k) => k.slice("raster:".length))
      .filter((id) => memberIdSet.has(id))
    if (nextMemberIds.length > 1) {
      onReorderMembers?.(nextMemberIds)
    }
  }

  const handleDropOnItem = (targetKey) => {
    setDropOverKey(null)
    if (!dragKey || dragKey === targetKey) {
      setDragKey(null)
      return
    }
    const keys = unifiedItems.map((it) => it.key)
    const from = keys.indexOf(dragKey)
    const to = keys.indexOf(targetKey)
    setDragKey(null)
    if (from === -1 || to === -1) return
    const next = [...keys]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    commitUnifiedReorder(next)
  }

  const handleNudge = (key, delta, e) => {
    e?.stopPropagation()
    const keys = unifiedItems.map((it) => it.key)
    const idx = keys.indexOf(key)
    const targetIdx = idx + delta
    if (idx === -1 || targetIdx < 0 || targetIdx >= keys.length) return
    const next = [...keys]
    const [moved] = next.splice(idx, 1)
    next.splice(targetIdx, 0, moved)
    commitUnifiedReorder(next)
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
          <span className="text-[10px] font-mono text-neutral-400" title="Vector / Raster / Base count">
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

      {/* Quick Action Bar + Layer-Type Color Legend */}
      <div className="px-2 py-1.5 border-b border-neutral-200 dark:border-[#2d2d2d] bg-neutral-50/80 dark:bg-[#212121] space-y-1.5 shrink-0">
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={onOpenAddVector}
            className="flex-1 inline-flex items-center justify-center gap-1 py-1 px-1.5 rounded-[5px] border border-emerald-500/30 bg-emerald-500/10 hover:bg-emerald-500/20 text-[10.5px] font-semibold text-emerald-700 dark:text-emerald-300 cursor-pointer transition-colors"
            title="Upload Vector Dataset"
          >
            <Plus className="size-3" /> Vector
          </button>
          <button
            type="button"
            onClick={onOpenAddRaster}
            className="flex-1 inline-flex items-center justify-center gap-1 py-1 px-1.5 rounded-[5px] border border-sky-500/30 bg-sky-500/10 hover:bg-sky-500/20 text-[10.5px] font-semibold text-sky-700 dark:text-sky-300 cursor-pointer transition-colors"
            title="Upload Raster Image"
          >
            <Plus className="size-3" /> Raster
          </button>
          <button
            type="button"
            onClick={() => setCreatingVector((p) => !p)}
            className="p-1.5 rounded-[5px] border border-neutral-200 dark:border-[#343434] hover:bg-neutral-200 dark:hover:bg-[#323232] text-neutral-600 dark:text-neutral-300 cursor-pointer"
            title="Create New Empty Vector Layer"
          >
            <FolderPlus className="size-3.5" />
          </button>
        </div>

        {/* Color legend */}
        <div className="flex items-center justify-between text-[9px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400 px-0.5 select-none">
          <span className="inline-flex items-center gap-1">
            <span className="size-2 rounded-full bg-[#0ea5e9]" /> Raster
          </span>
          <span className="inline-flex items-center gap-1">
            <span className="size-2 rounded-full bg-[#3ecf8e]" /> Poly
          </span>
          <span className="inline-flex items-center gap-1">
            <span className="size-2 rounded-full bg-[#8b5cf6]" /> Line
          </span>
          <span className="inline-flex items-center gap-1">
            <span className="size-2 rounded-full bg-[#f59e0b]" /> Point
          </span>
        </div>
      </div>

      {/* Unified Photoshop-Style Draggable Layer Stack (Top = Front, Bottom = Back) */}
      <div className="flex-1 overflow-y-auto min-h-0 p-1.5 space-y-1">
        {creatingVector && (
          <form
            onSubmit={handleCreateVectorLayer}
            className="p-1.5 rounded bg-neutral-50 dark:bg-[#171717] border border-[#3ecf8e]/40 space-y-1.5 mb-1.5"
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

        {unifiedItems.length === 0 ? (
          <div className="px-3 py-6 text-center text-[11px] text-neutral-400 space-y-1">
            <div>No layers yet.</div>
            <div className="text-[10px]">Add a Vector or Raster layer above, then drag any layer to adjust stacking order.</div>
          </div>
        ) : (
          unifiedItems.map((item, idx) => {
            const badge = getLayerTypeBadge(item)
            const BadgeIcon = badge.Icon
            const isVector = item.kind === "vector"
            const isActive = isVector && activeGISLayer?.layer_id === item.id
            const renameKey = isVector ? `v:${item.id}` : `r:${item.id}`
            const isRenaming = renamingId === renameKey
            const showOp = opacityOpenId === item.key
            const isDropTarget = dropOverKey === item.key && dragKey && dragKey !== item.key

            return (
              <div
                key={item.key}
                draggable
                onDragStart={() => setDragKey(item.key)}
                onDragEnd={() => {
                  setDragKey(null)
                  setDropOverKey(null)
                }}
                onDragOver={(e) => {
                  e.preventDefault()
                  if (dragKey && dragKey !== item.key) setDropOverKey(item.key)
                }}
                onDragLeave={() => {
                  if (dropOverKey === item.key) setDropOverKey(null)
                }}
                onDrop={(e) => {
                  e.preventDefault()
                  handleDropOnItem(item.key)
                }}
                onContextMenu={(e) => openCtx(e, item.kind, isVector ? item.layer : item.entry)}
                style={{ borderLeftColor: badge.accent }}
                className={`group rounded-[6px] border border-l-[3px] transition-all ${
                  isDropTarget
                    ? "border-t-2 border-t-[#3ecf8e] bg-[#3ecf8e]/10"
                    : isActive
                      ? "border-neutral-300 dark:border-[#3e3e3e] bg-neutral-100 dark:bg-[#2a2a2a]"
                      : "border-neutral-200/80 dark:border-[#2c2c2c] bg-white dark:bg-[#1b1b1b] hover:bg-neutral-50 dark:hover:bg-[#242424]"
                } ${dragKey === item.key ? "opacity-45" : ""}`}
              >
                <div
                  onClick={() => {
                    if (isVector) onSelectGISLayer?.(item.layer)
                  }}
                  className="flex items-center gap-1.5 px-1.5 py-1.5 text-[11px] cursor-pointer"
                >
                  <GripVertical
                    className="size-3 text-neutral-400 cursor-grab shrink-0"
                    title="Drag to reorder layer"
                  />

                  {/* Visibility eye */}
                  <button
                    type="button"
                    onClick={(e) =>
                      isVector ? handleToggleVectorVis(item.layer, e) : handleToggleRasterVis(item.entry, e)
                    }
                    className="text-neutral-500 hover:text-neutral-900 dark:hover:text-white cursor-pointer shrink-0"
                    title={item.visible ? "Hide layer" : "Show layer"}
                  >
                    {item.visible ? (
                      <Eye className="size-3.5" style={{ color: badge.accent }} />
                    ) : (
                      <EyeOff className="size-3.5 opacity-40" />
                    )}
                  </button>

                  {/* Type icon / swatch */}
                  <span
                    className={`inline-flex items-center justify-center size-5 rounded-[4px] border shrink-0 ${badge.pillClass}`}
                    title={badge.label}
                  >
                    <BadgeIcon className="size-3" />
                  </span>

                  {/* Layer name */}
                  <div className="flex-1 min-w-0">
                    {isRenaming ? (
                      <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
                        <input
                          type="text"
                          value={renameDraft}
                          onChange={(e) => setRenameDraft(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") commitRename(item.kind, isVector ? item.layer : item.entry)
                            if (e.key === "Escape") setRenamingId(null)
                          }}
                          className="w-full h-5 px-1 text-[11px] rounded bg-white dark:bg-[#141414] border border-[#3ecf8e] outline-none"
                          autoFocus
                        />
                        <button
                          type="button"
                          onClick={() => commitRename(item.kind, isVector ? item.layer : item.entry)}
                          className="text-[#3ecf8e] cursor-pointer"
                        >
                          <Check className="size-3" />
                        </button>
                      </div>
                    ) : (
                      <div className="flex items-center gap-1 min-w-0">
                        <span className="truncate font-medium text-neutral-800 dark:text-neutral-100">
                          {item.name}
                        </span>
                      </div>
                    )}
                    <div className="flex items-center gap-1.5 text-[9px] text-neutral-400">
                      <span style={{ color: badge.accent }} className="font-bold">
                        {badge.label}
                      </span>
                      {isVector ? (
                        <span className="font-mono">{item.featureCount} feat</span>
                      ) : (
                        <span>{item.isGeoreferenced ? "Georef" : "Unplaced"}</span>
                      )}
                      {item.opacity < 0.99 && (
                        <span className="font-mono">{Math.round(item.opacity * 100)}%</span>
                      )}
                    </div>
                  </div>

                  {/* Quick Up / Down nudge buttons */}
                  <div className="flex items-center gap-0.5 shrink-0">
                    <button
                      type="button"
                      disabled={idx === 0}
                      onClick={(e) => handleNudge(item.key, -1, e)}
                      className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white disabled:opacity-20 cursor-pointer disabled:cursor-default"
                      title="Move layer up (bring forward)"
                    >
                      <ArrowUp className="size-3" />
                    </button>
                    <button
                      type="button"
                      disabled={idx === unifiedItems.length - 1}
                      onClick={(e) => handleNudge(item.key, 1, e)}
                      className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white disabled:opacity-20 cursor-pointer disabled:cursor-default"
                      title="Move layer down (send backward)"
                    >
                      <ArrowDown className="size-3" />
                    </button>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        setOpacityOpenId(showOp ? null : item.key)
                      }}
                      className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
                      title="Opacity"
                    >
                      <SlidersHorizontal className="size-3" />
                    </button>
                    <button
                      type="button"
                      onClick={(e) => openCtx(e, item.kind, isVector ? item.layer : item.entry)}
                      className="p-0.5 rounded text-neutral-400 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
                      title="Layer actions"
                    >
                      <MoreHorizontal className="size-3.5" />
                    </button>
                  </div>
                </div>

                {showOp && (
                  <div className="px-6 pb-2 flex items-center gap-2 text-[10px] text-neutral-500">
                    <span>Opacity</span>
                    <input
                      type="range"
                      min="0"
                      max="100"
                      value={Math.round(item.opacity * 100)}
                      onChange={(e) =>
                        isVector
                          ? handleVectorOpacity(item.layer, e.target.value)
                          : handleRasterOpacity(item.entry, e.target.value)
                      }
                      className="flex-1 h-1"
                      style={{ accentColor: badge.accent }}
                    />
                    <span className="font-mono w-7 text-right">{Math.round(item.opacity * 100)}%</span>
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>

      {/* Background Base Layer (Locked at bottom, like Photoshop Background) */}
      <div className="p-1.5 border-t border-neutral-200 dark:border-[#303030] bg-neutral-50 dark:bg-[#191919] shrink-0">
        <div
          style={{ borderLeftColor: "#64748b" }}
          className="rounded-[6px] border border-l-[3px] border-neutral-200 dark:border-[#303030] bg-white dark:bg-[#1e1e1e] px-2 py-1.5 text-[11px]"
        >
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={() => onChangeBasePrefs?.({ base_layer_visible: !baseVisible })}
              className="text-neutral-500 hover:text-neutral-900 dark:hover:text-white cursor-pointer shrink-0"
              title={baseVisible ? "Hide base layer" : "Show base layer"}
            >
              {baseVisible ? (
                <Eye className="size-3.5 text-slate-500 dark:text-slate-300" />
              ) : (
                <EyeOff className="size-3.5 opacity-45" />
              )}
            </button>
            <Globe className="size-3.5 text-slate-500 dark:text-slate-400 shrink-0" />
            <div className="min-w-0 flex-1">
              <div className="truncate font-semibold text-neutral-800 dark:text-neutral-100">
                {activeBaseLayer?.name || "World Base Map (OSM)"}
              </div>
              <div className="text-[9px] font-bold uppercase text-slate-500">BASE • BACKGROUND</div>
            </div>
            <Lock className="size-3 text-neutral-400 shrink-0" title="Locked at bottom of stack" />
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
              className="flex-1 accent-slate-500 h-1"
            />
            <span className="font-mono w-7 text-right">{Math.round(baseOpacity * 100)}%</span>
          </div>
        </div>
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
