import React, { useEffect, useMemo, useRef, useState } from "react"
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
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
  Archive,
  Building2,
  ChevronDown,
  ChevronRight,
  Database,
  FileText,
  GripVertical,
  Image,
  MapPin,
  MoreHorizontal,
  Move,
  ScanText,
  Shapes,
  Sparkles,
  Trash2,
  Wand2,
  X,
} from "lucide-react"

import BackgroundRemovalDialog from "./BackgroundRemovalDialog"
import WakeSlider from "./WakeSlider"
import { propagateBuildingGeoreference, datasetSourceImageUrl, pdfThumbnailUrl } from "../api"
import { nextExpandedLayerId } from "../lib/layerPanelState"


const SUBLAYER_ICONS = {
  raster: Image,
  control_point: MapPin,
  icon_point: Database,
  vector: Shapes,
  annotation: ScanText,
}

const VISIBILITY_FIELDS = {
  raster: "raster_visible",
  control_point: "control_points_visible",
  icon_point: "icon_points_visible",
  vector: "vectors_visible",
  annotation: "annotations_visible",
}

function LayerCheckbox({ checked, label, onChange }) {
  return (
    <label
      className="workspace-layer-checkbox"
      title={`${checked ? "Hide" : "Show"} ${label}`}
      onClick={(e) => e.stopPropagation()}
    >
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span aria-hidden="true" />
    </label>
  )
}

function LayerOpacitySlider({ datasetId, opacity, onUpdate }) {
  const safeOpacity = typeof opacity === "number" && Number.isFinite(opacity) ? opacity : 1
  const [localPercent, setLocalPercent] = useState(() => Math.round(safeOpacity * 100))
  const debounceTimerRef = useRef(null)

  useEffect(() => {
    setLocalPercent(Math.round(safeOpacity * 100))
  }, [safeOpacity])

  const commitUpdate = (val) => {
    const floatVal = Math.max(0, Math.min(1, val / 100))
    onUpdate(datasetId, { raster_opacity: floatVal })
  }

  const handleSliderChange = (val) => {
    setLocalPercent(val)
    const floatVal = Math.max(0, Math.min(1, val / 100))

    const setOpacity = typeof window !== "undefined" ? (window.__veracity_setLayerOpacity || window.__maptiler_setLayerOpacity) : null
    if (setOpacity) {
      setOpacity(datasetId, floatVal)
    }

    clearTimeout(debounceTimerRef.current)
    debounceTimerRef.current = setTimeout(() => {
      commitUpdate(val)
    }, 150)
  }

  return (
    <div className="workspace-layer-opacity-control p-1" onClick={(e) => e.stopPropagation()}>
      <div className="flex items-center justify-between text-xs mb-1.5 select-none">
        <span className="font-medium text-neutral-400">Raster opacity</span>
        <strong className="font-mono text-neutral-800 dark:text-neutral-100">{localPercent}%</strong>
      </div>
      <WakeSlider
        value={localPercent}
        onChange={handleSliderChange}
        min={0}
        max={100}
        step={1}
        bars={26}
        height={24}
        restHeight={6}
        gap={2.5}
        fillColor="#3ecf8e"
        trackColor="#262626"
        crestColor="#3ecf8e"
        sensitivity={1.2}
        reach={5}
        glide={0.25}
        ariaLabel="Raster opacity"
      />
    </div>
  )
}

function FloorLevelRow({
  floorObj,
  isActiveFloor,
  buildingName,
  onSelectFloor,
  onSelect,
  onUpdate,
  onDetach,
  onOpenBgRemoval,
  onOpenTransform,
  onOpenEnhancement,
  onOpenAnnotations,
}) {
  const [expanded, setExpanded] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef(null)
  const { level, label, members } = floorObj

  const isMultiLayer = members.length > 1
  const allVisible = members.every((m) => m.visible && m.raster_visible)
  const primaryMember = members[0]

  const badgeLabel = level === -1 ? "B1" : level === 0 ? "G" : `${level}`
  const badgeClass = level < 0 ? "badge-basement" : level === 0 ? "badge-ground" : "badge-upper"

  useEffect(() => {
    if (!menuOpen) return
    const handleClickOutside = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener("pointerdown", handleClickOutside)
    return () => {
      document.removeEventListener("pointerdown", handleClickOutside)
    }
  }, [menuOpen])

  function toggleFloorVisibility(checked) {
    members.forEach((m) => {
      onUpdate(m.dataset_id, { visible: checked, raster_visible: checked })
    })
  }

  return (
    <div className={`workspace-floor-card ${menuOpen ? "has-menu-open" : ""}`}>
      <div
        className={`workspace-floor-item ${isActiveFloor ? "is-active" : ""}`}
        onClick={() => {
          if (onSelectFloor) onSelectFloor(level, buildingName)
          if (primaryMember) onSelect(primaryMember.dataset_id, "raster")
          if (!allVisible) {
            members.forEach((m) => {
              onUpdate(m.dataset_id, { visible: true, raster_visible: true })
            })
          }
        }}
      >
        {isMultiLayer ? (
          <button
            type="button"
            className="workspace-layer-disclosure"
            aria-label={`${expanded ? "Collapse" : "Expand"} ${label} layers`}
            onClick={(e) => {
              e.stopPropagation()
              setExpanded(!expanded)
            }}
          >
            {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
          </button>
        ) : (
          <LayerCheckbox
            checked={allVisible}
            label={label}
            onChange={toggleFloorVisibility}
          />
        )}

        <span className={`workspace-floor-badge ${badgeClass}`}>{badgeLabel}</span>

        <div className="workspace-floor-info">
          <div className="flex items-center gap-1.5 min-w-0">
            <span className="workspace-floor-name" title={label}>
              {label}
            </span>
            {isMultiLayer && (
              <span className="workspace-floor-layer-pill">
                {members.length} layers
              </span>
            )}
          </div>
          {isActiveFloor && <span className="workspace-floor-active-tag">Active Floor</span>}
        </div>

        <div className="workspace-floor-actions" onClick={(e) => e.stopPropagation()}>
          {isMultiLayer && (
            <LayerCheckbox
              checked={allVisible}
              label={label}
              onChange={toggleFloorVisibility}
            />
          )}

          {!isMultiLayer && primaryMember && (
            <button
              type="button"
              className="workspace-floor-tool-btn"
              title="Transform raster placement"
              onClick={(e) => {
                e.stopPropagation()
                e.preventDefault()
                if (onOpenTransform) onOpenTransform(primaryMember)
              }}
            >
              <Move />
            </button>
          )}

          <div className="workspace-layer-menu-wrap" ref={menuRef}>
            <button
              type="button"
              className="workspace-floor-tool-btn"
              title="Floor actions"
              onClick={(e) => {
                e.stopPropagation()
                e.preventDefault()
                setMenuOpen((prev) => !prev)
              }}
            >
              <MoreHorizontal />
            </button>
            {menuOpen && (
              <div className="workspace-layer-popover" onClick={(e) => e.stopPropagation()}>
                {primaryMember && (
                  <LayerOpacitySlider
                    datasetId={primaryMember.dataset_id}
                    opacity={primaryMember.raster_opacity}
                    onUpdate={(dId, patch) => {
                      members.forEach((m) => onUpdate(m.dataset_id, patch))
                    }}
                  />
                )}
                {primaryMember && (
                  <button
                    type="button"
                    className="workspace-layer-feature-btn"
                    onClick={() => {
                      if (onOpenTransform) onOpenTransform(primaryMember)
                      setMenuOpen(false)
                    }}
                  >
                    <Move />
                    <span>Transform Raster</span>
                  </button>
                )}
                {primaryMember && onOpenEnhancement && (
                  <button
                    type="button"
                    className="workspace-layer-feature-btn"
                    onClick={() => {
                      onOpenEnhancement(primaryMember)
                      setMenuOpen(false)
                    }}
                  >
                    <Wand2 />
                    <span>Enhance &amp; Upscale</span>
                  </button>
                )}
                {primaryMember && onOpenAnnotations && (
                  <button
                    type="button"
                    className="workspace-layer-feature-btn"
                    onClick={() => {
                      onOpenAnnotations(primaryMember)
                      setMenuOpen(false)
                    }}
                  >
                    <ScanText />
                    <span>OCR &amp; Vector Labels</span>
                  </button>
                )}
                {primaryMember && (
                  <button
                    type="button"
                    className="workspace-layer-feature-btn"
                    onClick={() => {
                      if (onOpenBgRemoval) onOpenBgRemoval(primaryMember)
                      setMenuOpen(false)
                    }}
                  >
                    <Sparkles />
                    <span>Remove Background</span>
                  </button>
                )}
                {primaryMember && primaryMember.dataset?.georeference_status !== "completed" && (
                  <button
                    type="button"
                    className="workspace-layer-feature-btn text-amber-600"
                    onClick={() => {
                      const returnTo = window.location.pathname + window.location.search
                      window.location.href = `/datasets/${primaryMember.dataset_id}/georeference?returnTo=${encodeURIComponent(returnTo)}`
                    }}
                  >
                    <MapPin />
                    <span>Georeference Floor</span>
                  </button>
                )}
                {isMultiLayer && (
                  <button
                    type="button"
                    className="workspace-layer-feature-btn text-[#24b47e] dark:text-[#3ecf8e]"
                    onClick={() => {
                      setExpanded(!expanded)
                      setMenuOpen(false)
                    }}
                  >
                    <span>{expanded ? "Collapse Layers" : `Show all ${members.length} layers`}</span>
                  </button>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Render child layers when expanded */}
      {isMultiLayer && expanded && (
        <div className="workspace-floor-child-layers">
          {members.map((member, idx) => {
            const rawFilename = member.name || member.dataset?.filename || `Layer ${idx + 1}`
            const displayName = rawFilename.length > 28 ? rawFilename.slice(0, 25) + "..." : rawFilename

            return (
              <div key={member.dataset_id} className="workspace-floor-child-item">
                <div className="workspace-floor-child-title">
                  <LayerCheckbox
                    checked={member.visible && member.raster_visible}
                    label={rawFilename}
                    onChange={(checked) => {
                      onUpdate(member.dataset_id, { visible: checked, raster_visible: checked })
                    }}
                  />
                  <Image className="size-3 text-slate-400 shrink-0" />
                  <span title={rawFilename}>{displayName}</span>
                </div>
                <div className="flex items-center gap-1">
                  {onOpenEnhancement && (
                    <button
                      type="button"
                      className="workspace-floor-tool-btn"
                      title="Inspection, OpenCV enhancement & AI upscaling"
                      onClick={() => onOpenEnhancement(member)}
                    >
                      <Wand2 />
                    </button>
                  )}
                  <button
                    type="button"
                    className="workspace-floor-tool-btn"
                    title="Transform layer"
                    onClick={() => {
                      if (onOpenTransform) onOpenTransform(member)
                    }}
                  >
                    <Move />
                  </button>
                  <button
                    type="button"
                    className="workspace-floor-tool-btn"
                    title="Remove background"
                    onClick={() => {
                      if (onOpenBgRemoval) onOpenBgRemoval(member)
                    }}
                  >
                    <Sparkles />
                  </button>
                  {!member.is_primary && (
                    <button
                      type="button"
                      className="workspace-floor-tool-btn hover:text-red-600"
                      title="Detach dataset"
                      onClick={() => onDetach(member)}
                    >
                      <Trash2 />
                    </button>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function BuildingGroup({
  buildingName,
  floors,
  activeFloorLevel,
  activeBuildingName = "",
  onSelectFloor,
  onSelect,
  onUpdate,
  onDetach,
  onOpenBgRemoval,
  onOpenTransform,
  onOpenEnhancement,
  onOpenAnnotations,
  buildings = [],
  onLayerChanged,
  hasAnyGeorefInWorkspace = false,
  floorFilter = "all",
}) {
  const [expanded, setExpanded] = useState(true)
  const [isPropagating, setIsPropagating] = useState(false)
  const isGeoreferenced = floors.some(
    (f) => f.dataset?.georeference_status === "completed" || f.dataset?.map_layer
  )
  const allVisible = floors.some((f) => f.visible)
  const primaryFloor = floors.find((f) => f.floor_level === 0) || floors[0]

  const isBuildingActive = !activeBuildingName || (buildingName.trim().toLowerCase() === activeBuildingName.trim().toLowerCase())

  // Group members into distinct floor levels
  const floorLevels = useMemo(() => {
    const levelMap = new Map()
    floors.forEach((m) => {
      const lvl = m.floor_level ?? 0
      if (!levelMap.has(lvl)) {
        levelMap.set(lvl, {
          level: lvl,
          label: m.floor_label || (lvl === -1 ? "Basement" : lvl === 0 ? "Ground Floor" : `Floor ${lvl}`),
          members: [],
        })
      }
      const entry = levelMap.get(lvl)
      entry.members.push(m)
      if (m.floor_label && m.floor_label !== m.name && entry.label === `Floor ${lvl}`) {
        entry.label = m.floor_label
      }
    })

    const sorted = Array.from(levelMap.values()).sort((a, b) => b.level - a.level)
    if (floorFilter === "active") {
      if (isBuildingActive) {
        return sorted.filter((fl) => fl.level === activeFloorLevel)
      }
      // In active floor only mode, other buildings show their matching floor level if available or all floors
      return sorted.filter((fl) => fl.level === activeFloorLevel)
    }
    return sorted
  }, [floors, floorFilter, activeFloorLevel, isBuildingActive])

  const distinctFloorCount = useMemo(() => {
    const s = new Set()
    floors.forEach((f) => s.add(f.floor_level ?? 0))
    return s.size
  }, [floors])

  function toggleMasterVisibility(checked) {
    floors.forEach((f) => {
      onUpdate(f.dataset_id, { visible: checked })
    })
  }

  async function handleInheritFootprint() {
    setIsPropagating(true)
    try {
      const bldObj = buildings?.find(
        (b) => b.name?.toLowerCase() === buildingName?.toLowerCase()
      )
      const bId = bldObj?.id || floors[0]?.building_id || floors[0]?.dataset?.extracted_metadata?.building_id
      if (bId) {
        await propagateBuildingGeoreference(bId)
      } else {
        const refId = primaryFloor?.dataset_id || floors[0]?.dataset_id
        if (refId) {
          await propagateBuildingGeoreference(refId)
        }
      }
      if (onLayerChanged) onLayerChanged()
    } catch (err) {
      console.warn("Failed to propagate building footprint:", err)
    } finally {
      setIsPropagating(false)
    }
  }

  return (
    <div className="workspace-building-container">
      <div className="workspace-building-header">
        <div className="workspace-building-header-main">
          <button
            type="button"
            className="workspace-layer-disclosure"
            aria-label={`${expanded ? "Collapse" : "Expand"} ${buildingName}`}
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? <ChevronDown /> : <ChevronRight />}
          </button>
          <Building2 className="size-4 text-[#24b47e] dark:text-[#3ecf8e] shrink-0" />
          <LayerCheckbox
            checked={allVisible}
            label={buildingName}
            onChange={toggleMasterVisibility}
          />
          <div className="workspace-building-title" title={buildingName}>
            <span>{buildingName}</span>
            <span className="workspace-building-badge">
              {distinctFloorCount} {distinctFloorCount === 1 ? "Floor" : "Floors"}
            </span>
          </div>
        </div>
        <div className="workspace-building-actions">
          {!isGeoreferenced ? (
            <>
              {hasAnyGeorefInWorkspace && (
                <button
                  type="button"
                  disabled={isPropagating}
                  className="text-[10px] text-[#24b47e] dark:text-[#3ecf8e] font-semibold bg-[#3ecf8e]/10 px-2 py-0.5 rounded-full border border-[#3ecf8e]/30 hover:bg-[#3ecf8e]/20 transition-colors disabled:opacity-50 whitespace-nowrap"
                  title="Inherit footprint and georeference coordinates from matching floor"
                  onClick={handleInheritFootprint}
                >
                  {isPropagating ? "Syncing..." : "Inherit Footprint"}
                </button>
              )}
              <button
                type="button"
                className="text-[11px] text-amber-600 font-semibold hover:underline whitespace-nowrap"
                onClick={() => {
                  const targetId = primaryFloor?.dataset_id || floors[0]?.dataset_id
                  const returnTo = window.location.pathname + window.location.search
                  window.location.href = `/datasets/${targetId}/georeference?returnTo=${encodeURIComponent(returnTo)}`
                }}
              >
                Georeference →
              </button>
            </>
          ) : (
            <span className="text-[10px] text-emerald-600 font-semibold bg-emerald-50 px-2 py-0.5 rounded-full border border-emerald-200 whitespace-nowrap">
              Georeferenced
            </span>
          )}
        </div>
      </div>

      {expanded && (
        <div className="workspace-building-floors">
          {floorLevels.map((floorObj) => (
            <FloorLevelRow
              key={floorObj.level}
              floorObj={floorObj}
              isActiveFloor={isBuildingActive && activeFloorLevel === floorObj.level}
              buildingName={buildingName}
              onSelectFloor={onSelectFloor}
              onSelect={onSelect}
              onUpdate={onUpdate}
              onDetach={onDetach}
              onOpenBgRemoval={onOpenBgRemoval}
              onOpenTransform={onOpenTransform}
              onOpenEnhancement={onOpenEnhancement}
              onOpenAnnotations={onOpenAnnotations}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function FileDocumentGroup({
  group,
  selectedDatasetId,
  onSelect,
  onUpdate,
  onDetach,
  onOpenBgRemoval,
  onOpenTransform,
  onOpenEnhancement,
  onOpenAnnotations,
}) {
  const [expanded, setExpanded] = useState(true)
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef(null)

  const { title, type, members } = group
  const isPdf = type === "pdf"
  const allVisible = members.length > 0 && members.every((m) => m.visible !== false && m.raster_visible !== false)
  const primaryMember = members[0]

  useEffect(() => {
    if (!menuOpen) return
    const handleClickOutside = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener("pointerdown", handleClickOutside)
    return () => {
      document.removeEventListener("pointerdown", handleClickOutside)
    }
  }, [menuOpen])

  function toggleGroupVisibility(checked) {
    members.forEach((m) => {
      onUpdate(m.dataset_id, { visible: checked, raster_visible: checked })
    })
  }

  const badgeClass = isPdf ? "badge-pdf" : "badge-zip"
  const badgeText = isPdf ? "PDF" : "ZIP"
  const GroupIcon = isPdf ? FileText : Archive

  return (
    <div className={`workspace-doc-container bg-white dark:bg-[#1c1c1c] border border-slate-200 dark:border-[#2e2e2e] ${expanded ? "" : "is-collapsed"}`}>
      {/* File Document Layer Header */}
      <div className="workspace-doc-header bg-slate-50 dark:bg-[#202020] border-b border-slate-200 dark:border-[#2e2e2e]">
        <div className="workspace-doc-header-main">
          <button
            type="button"
            className="workspace-layer-disclosure"
            aria-label={`${expanded ? "Collapse" : "Expand"} ${title}`}
            onClick={() => setExpanded((prev) => !prev)}
          >
            {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
          </button>

          <LayerCheckbox
            checked={allVisible}
            label={title}
            onChange={toggleGroupVisibility}
          />

          <div className="workspace-doc-title">
            <GroupIcon className={`size-4 ${isPdf ? "text-red-500" : "text-emerald-600"} shrink-0`} />
            <span title={title}>{title}</span>
            <span className={`workspace-doc-badge ${badgeClass}`}>{badgeText}</span>
          </div>
        </div>

        <div className="workspace-doc-actions" onClick={(e) => e.stopPropagation()}>
          <span className="text-[10px] text-slate-400 font-medium mr-1">
            {members.length} {isPdf ? (members.length === 1 ? "page" : "pages") : (members.length === 1 ? "raster" : "rasters")}
          </span>

          <div className="workspace-layer-menu-wrap" ref={menuRef}>
            <button
              type="button"
              className="workspace-floor-tool-btn"
              title="File actions"
              onClick={(e) => {
                e.stopPropagation()
                e.preventDefault()
                setMenuOpen((prev) => !prev)
              }}
            >
              <MoreHorizontal />
            </button>
            {menuOpen && (
              <div className="workspace-layer-popover" onClick={(e) => e.stopPropagation()}>
                {primaryMember && (
                  <LayerOpacitySlider
                    datasetId={primaryMember.dataset_id}
                    opacity={primaryMember.raster_opacity}
                    onUpdate={(dId, patch) => {
                      members.forEach((m) => onUpdate(m.dataset_id, patch))
                    }}
                  />
                )}
                <button
                  type="button"
                  className="workspace-layer-feature-btn text-[#24b47e] dark:text-[#3ecf8e]"
                  onClick={() => {
                    setExpanded((prev) => !prev)
                    setMenuOpen(false)
                  }}
                >
                  <span>{expanded ? "Collapse sublayers" : `Show all ${members.length} sublayers`}</span>
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Child Sublayers List */}
      {expanded && (
        <div className="workspace-doc-sublayers">
          {members.map((member, idx) => {
            const pageNum = member.dataset?.extracted_metadata?.pdf_page_number ?? member.pdf_page_number
            const docId = member.dataset?.extracted_metadata?.source_document_id ?? member.source_document_id

            let sublayerLabel = member.name || member.dataset?.filename || `Sublayer ${idx + 1}`
            if (isPdf && pageNum) {
              sublayerLabel = `Page ${pageNum}`
            } else if (!isPdf) {
              const entry = member.dataset?.extracted_metadata?.archive_entry || member.archive_entry
              if (entry) {
                const parts = entry.split("/")
                sublayerLabel = parts[parts.length - 1] || entry
              }
            }

            const isSelected = selectedDatasetId === member.dataset_id
            const isVisible = member.visible !== false && member.raster_visible !== false
            const isGeoreferenced = member.dataset?.georeference_status === "completed" || Boolean(member.dataset?.map_layer)

            // Dynamic thumbnail URL
            const thumbUrl = (isPdf && docId && pageNum)
              ? pdfThumbnailUrl(docId, pageNum)
              : datasetSourceImageUrl(member.dataset_id)

            return (
              <div
                key={member.dataset_id}
                className={`workspace-doc-sublayer bg-white dark:bg-[#202020] border border-slate-200 dark:border-[#2e2e2e] text-neutral-800 dark:text-neutral-200 ${isSelected ? "is-selected dark:bg-[#3ecf8e]/10" : ""}`}
              >
                <div
                  className="workspace-doc-sublayer-info"
                  onClick={() => {
                    if (!isGeoreferenced) {
                      const returnTo = window.location.pathname + window.location.search
                      window.location.href = `/datasets/${member.dataset_id}/georeference?returnTo=${encodeURIComponent(returnTo)}`
                    } else {
                      onSelect(member.dataset_id, "raster")
                    }
                  }}
                >
                  <LayerCheckbox
                    checked={isVisible}
                    label={sublayerLabel}
                    onChange={(checked) => {
                      onUpdate(member.dataset_id, { visible: checked, raster_visible: checked })
                    }}
                  />

                  {/* Dynamic Thumbnail Preview */}
                  <div className="workspace-doc-thumb bg-slate-100 dark:bg-[#171717] border border-slate-200 dark:border-[#2e2e2e]" title={sublayerLabel}>
                    <img
                      src={thumbUrl}
                      alt=""
                      onError={(e) => {
                        e.target.style.display = "none"
                      }}
                    />
                    <Image className="size-3 text-slate-400 absolute" style={{ zIndex: 0 }} />
                  </div>

                  <div className="workspace-doc-sublayer-text">
                    <span className="workspace-doc-sublayer-name text-neutral-900 dark:text-white" title={sublayerLabel}>
                      {sublayerLabel}
                    </span>
                    <span className="workspace-doc-sublayer-meta text-neutral-500 dark:text-neutral-400">
                      {isGeoreferenced ? (
                        <span className="text-emerald-600 font-medium">Georeferenced</span>
                      ) : (
                        <span className="text-amber-600 font-medium">Needs georeferencing →</span>
                      )}
                    </span>
                  </div>
                </div>

                <div className="workspace-doc-sublayer-actions" onClick={(e) => e.stopPropagation()}>
                  {onOpenEnhancement && (
                    <button
                      type="button"
                      className="workspace-floor-tool-btn"
                      title="Inspection, OpenCV enhancement & AI upscaling"
                      onClick={() => onOpenEnhancement(member)}
                    >
                      <Wand2 />
                    </button>
                  )}
                  <button
                    type="button"
                    className="workspace-floor-tool-btn"
                    title="Transform placement"
                    onClick={() => {
                      if (onOpenTransform) onOpenTransform(member)
                    }}
                  >
                    <Move />
                  </button>
                  <button
                    type="button"
                    className="workspace-floor-tool-btn"
                    title="Remove background"
                    onClick={() => {
                      if (onOpenBgRemoval) onOpenBgRemoval(member)
                    }}
                  >
                    <Sparkles />
                  </button>
                  {!member.is_primary && (
                    <button
                      type="button"
                      className="workspace-floor-tool-btn hover:text-red-600"
                      title="Detach dataset"
                      onClick={() => onDetach(member)}
                    >
                      <Trash2 />
                    </button>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function SortableMember({
  member,
  records = [],
  selectedDatasetId,
  selectedKind,
  settingsOpen,
  onToggleSettings,
  onSelect,
  onUpdate,
  onDetach,
  onOpenBgRemoval,
  onOpenTransform,
  onOpenEnhancement,
  onOpenAnnotations,
}) {
  const [expanded, setExpanded] = useState(false)
  const sortable = useSortable({ id: member.dataset_id })
  const style = {
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
  }

  return (
    <div
      ref={sortable.setNodeRef}
      style={style}
      className={`workspace-layer-group${sortable.isDragging ? " is-dragging" : ""}`}
    >
      <div className={`workspace-layer-parent${selectedDatasetId === member.dataset_id && !selectedKind ? " is-selected" : ""}`}>
        <button
          type="button"
          className="workspace-layer-disclosure"
          aria-label={`${expanded ? "Collapse" : "Expand"} ${member.name}`}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? <ChevronDown /> : <ChevronRight />}
        </button>
        <button
          type="button"
          className="workspace-layer-drag"
          aria-label={`Reorder ${member.name}`}
          ref={sortable.setActivatorNodeRef}
          {...sortable.attributes}
          {...sortable.listeners}
        >
          <GripVertical />
        </button>
        <LayerCheckbox
          checked={member.visible}
          label={member.name}
          onChange={(visible) => onUpdate(member.dataset_id, { visible })}
        />
        <div className="workspace-doc-thumb" title={member.name}>
          <img
            src={datasetSourceImageUrl(member.dataset_id)}
            alt=""
            onError={(e) => {
              e.target.style.display = "none"
            }}
          />
          <Image className="size-3 text-slate-400 absolute" style={{ zIndex: 0 }} />
        </div>
        <button
          type="button"
          className="workspace-layer-title"
          onClick={() => {
            if (member.dataset?.georeference_status !== "completed") {
              const returnTo = window.location.pathname + window.location.search
              window.location.href = `/datasets/${member.dataset_id}/georeference?returnTo=${encodeURIComponent(returnTo)}`
            } else {
              onSelect(member.dataset_id, "")
            }
          }}
        >
          <span>{member.name}</span>
          <small className={member.dataset?.georeference_status !== "completed" ? "text-amber-600 font-medium cursor-pointer" : ""}>
            {member.dataset?.georeference_status === "completed" ? "Georeferenced" : "Needs georeferencing →"}
          </small>
        </button>
        <div className="workspace-layer-menu-wrap">
          <button
            type="button"
            className={`workspace-layer-menu${settingsOpen ? " is-active" : ""}`}
            aria-label={`${settingsOpen ? "Collapse" : "Expand"} settings for ${member.name}`}
            aria-expanded={settingsOpen}
            onClick={onToggleSettings}
          >
            <MoreHorizontal />
          </button>
        </div>
      </div>

      {settingsOpen && (
        <section className="workspace-layer-settings" aria-label={`Settings for ${member.name}`}>
          <div className="workspace-layer-settings-header">
            <span>Layer settings</span>
            <button type="button" onClick={onToggleSettings}>
              <ChevronDown /> Collapse
            </button>
          </div>
          <LayerOpacitySlider
            datasetId={member.dataset_id}
            opacity={member.raster_opacity}
            onUpdate={onUpdate}
          />
          <div className="workspace-layer-settings-actions">
            {member.dataset?.georeference_status !== "completed" && (
              <button
                type="button"
                className="workspace-layer-feature-btn text-amber-600"
                title="Georeference layer"
                onClick={() => {
                  const returnTo = window.location.pathname + window.location.search
                  window.location.href = `/datasets/${member.dataset_id}/georeference?returnTo=${encodeURIComponent(returnTo)}`
                }}
              >
                <MapPin />
                <span>Georeference</span>
              </button>
            )}
            {onOpenEnhancement && (
              <button
                type="button"
                className="workspace-layer-feature-btn"
                title="Enhance and upscale"
                onClick={() => {
                  onOpenEnhancement(member)
                  onToggleSettings()
                }}
              >
                <Wand2 />
                <span>Enhance</span>
              </button>
            )}
            {onOpenAnnotations && (
              <button
                type="button"
                className="workspace-layer-feature-btn"
                title="OCR and vector labels"
                onClick={() => {
                  onOpenAnnotations(member)
                  onToggleSettings()
                }}
              >
                <ScanText />
                <span>OCR labels</span>
              </button>
            )}
            <button
              type="button"
              className="workspace-layer-feature-btn"
              title="Transform raster"
              onClick={() => {
                if (onOpenTransform) onOpenTransform(member)
                onToggleSettings()
              }}
            >
              <Move />
              <span>Transform</span>
            </button>
            <button
              type="button"
              className="workspace-layer-feature-btn"
              title="Remove raster background"
              onClick={() => {
                if (onOpenBgRemoval) onOpenBgRemoval(member)
                onToggleSettings()
              }}
            >
              <Sparkles />
              <span>Background</span>
            </button>
          </div>
          {!member.is_primary && (
            <button
              type="button"
              className="workspace-danger-action workspace-layer-settings-danger"
              onClick={() => {
                onToggleSettings()
                onDetach(member)
              }}
            >
              <Trash2 /> Detach dataset
            </button>
          )}
        </section>
      )}

      {expanded && (
        <div className="workspace-sublayers">
          {(member.sublayers || [
            { kind: "raster", label: "Raster Overlay", count: 1 },
            { kind: "control_point", label: "Control Points", count: 0 },
            { kind: "icon_point", label: "Icon Points", count: 0 },
          ]).map((sublayer) => {
            const Icon = SUBLAYER_ICONS[sublayer.kind] || Image
            const field = VISIBILITY_FIELDS[sublayer.kind]
            const selected = selectedDatasetId === member.dataset_id && selectedKind === sublayer.kind
            const liveCount = (records && records.length > 0 && sublayer.kind !== "raster")
              ? records.filter((r) => r.kind === sublayer.kind && (r.dataset_id === member.dataset_id || (!r.dataset_id && member.is_primary))).length
              : (sublayer.count ?? 0)
            return (
              <div key={sublayer.kind} className={`workspace-sublayer${selected ? " is-selected" : ""}`}>
                <LayerCheckbox
                  checked={member[field]}
                  label={`${member.name} ${sublayer.label}`}
                  onChange={(visible) => onUpdate(member.dataset_id, { [field]: visible })}
                />
                <button type="button" onClick={() => onSelect(member.dataset_id, sublayer.kind)}>
                  <Icon />
                  <span>{sublayer.label}</span>
                  {sublayer.kind !== "raster" && <small>{liveCount}</small>}
                </button>
                {sublayer.kind === "raster" && (
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      className="workspace-sublayer-tool"
                      title="Transform raster placement (Move, Scale, Rotate, Free Transform)"
                      onClick={(e) => {
                        e.stopPropagation()
                        if (onOpenTransform) onOpenTransform(member)
                      }}
                    >
                      <Move />
                    </button>
                    <button
                      type="button"
                      className="workspace-sublayer-tool"
                      title="Remove raster background"
                      onClick={(e) => {
                        e.stopPropagation()
                        if (onOpenBgRemoval) onOpenBgRemoval(member)
                      }}
                    >
                      <Sparkles />
                    </button>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

export default function WorkspaceLayerPanel({
  members = [],
  records = [],
  selectedDatasetId,
  selectedKind,
  onSelect,
  onUpdate,
  onReorder,
  onDetach,
  onClose,
  onLayerChanged,
  onOpenTransform,
  onOpenEnhancement,
  onOpenAnnotations,
  activeFloorLevel = 0,
  activeBuildingName = "",
  onSelectFloor,
  buildings = [],
  isolateActiveFloor = true,
  onToggleIsolateActiveFloor,
}) {
  const [bgRemovalMember, setBgRemovalMember] = useState(null)
  const [openSettingsId, setOpenSettingsId] = useState(null)
  const [localFloorFilter, setLocalFloorFilter] = useState("active")
  const floorFilter = onToggleIsolateActiveFloor
    ? (isolateActiveFloor ? "active" : "all")
    : localFloorFilter

  const handleFilterChange = (mode) => {
    if (onToggleIsolateActiveFloor) {
      onToggleIsolateActiveFloor(mode === "active")
    } else {
      setLocalFloorFilter(mode)
    }
  }

  const sortedMembers = useMemo(
    () => [...members].sort((a, b) => a.sort_order - b.sort_order),
    [members],
  )

  const hasAnyGeorefInWorkspace = useMemo(() => {
    return members.some(
      (m) => m.dataset?.georeference_status === "completed" || m.dataset?.map_layer || m.map_layer
    )
  }, [members])

  const { fileGroups, buildingGroups, standaloneMembers } = useMemo(() => {
    const fileGroupMap = new Map()
    const groups = new Map()
    const standalone = []

    const isFloorToken = (name) =>
      /^(floor|flr|level|lvl|basement|ground)\b/i.test((name || "").trim())

    const realBuildingNames = []
    sortedMembers.forEach((m) => {
      const bn = (m.building_name || "").trim()
      if (bn && !isFloorToken(bn) && !realBuildingNames.includes(bn)) {
        realBuildingNames.push(bn)
      }
    })
    if (buildings && Array.isArray(buildings)) {
      buildings.forEach((b) => {
        const bn = (b.name || "").trim()
        if (bn && !isFloorToken(bn) && !realBuildingNames.includes(bn)) {
          realBuildingNames.push(bn)
        }
      })
    }

    sortedMembers.forEach((member) => {
      // 1. Check if member belongs to an uploaded file container (.pdf or .zip)
      const docType = member.dataset?.extracted_metadata?.document_type || member.document_type || ""
      const srcDocId = member.dataset?.extracted_metadata?.source_document_id || member.source_document_id || ""
      const srcFn = member.dataset?.extracted_metadata?.source_filename || member.source_filename || ""
      const archFn = member.dataset?.extracted_metadata?.archive_filename || member.archive_filename || ""
      const mName = (member.name || member.dataset?.filename || "").trim()

      const isArchiveName = (str) => {
        const s = (str || "").toLowerCase()
        return s.endsWith(".zip") || s.endsWith(".tar") || s.endsWith(".tar.gz") || s.endsWith(".tgz")
      }
      const isPdfMember = docType === "pdf" || srcFn.toLowerCase().endsWith(".pdf") || mName.toLowerCase().endsWith(".pdf")
      const isZipMember = isArchiveName(archFn) || isArchiveName(srcFn) || isArchiveName(mName)

      if (isPdfMember) {
        const pdfKey = `pdf_${srcDocId || srcFn || "document.pdf"}`
        const pdfTitle = srcFn || mName.replace(/\s*-\s*Page\s+\d+.*$/i, ".pdf") || "Document.pdf"
        if (!fileGroupMap.has(pdfKey)) {
          fileGroupMap.set(pdfKey, {
            id: pdfKey,
            title: pdfTitle,
            type: "pdf",
            members: [],
          })
        }
        fileGroupMap.get(pdfKey).members.push(member)
        return
      }

      if (isZipMember) {
        const zipKey = `zip_${archFn || srcFn || "archive.zip"}`
        const zipTitle = archFn || srcFn || "Archive.zip"
        if (!fileGroupMap.has(zipKey)) {
          fileGroupMap.set(zipKey, {
            id: zipKey,
            title: zipTitle,
            type: "zip",
            members: [],
          })
        }
        fileGroupMap.get(zipKey).members.push(member)
        return
      }

      // 2. Check if member belongs to a multi-floor building
      let bName = (member.building_name || "").trim()
      if (!bName || isFloorToken(bName)) {
        const matchedReal = realBuildingNames.find((rn) =>
          mName.toLowerCase().includes(rn.toLowerCase().replace(/\s+/g, "_")) ||
          mName.toLowerCase().includes(rn.toLowerCase()) ||
          (member.dataset?.storage_path && member.dataset.storage_path.toLowerCase().includes(rn.toLowerCase().replace(/\s+/g, "_")))
        )
        if (matchedReal) {
          bName = matchedReal
        } else if (member.is_building_floor && realBuildingNames.length > 0) {
          bName = realBuildingNames[0]
        }
      }

      if (bName) {
        if (!groups.has(bName)) groups.set(bName, [])
        groups.get(bName).push(member)
      } else {
        standalone.push(member)
      }
    })

    return {
      fileGroups: Array.from(fileGroupMap.values()),
      buildingGroups: groups,
      standaloneMembers: standalone,
    }
  }, [sortedMembers, buildings])

  const totalDistinctFloors = useMemo(() => {
    let count = 0
    buildingGroups.forEach((floors) => {
      const s = new Set()
      floors.forEach((f) => s.add(f.floor_level ?? 0))
      count += s.size
    })
    return count
  }, [buildingGroups])

  const [orderedIds, setOrderedIds] = useState(standaloneMembers.map((member) => member.dataset_id))
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  useEffect(() => {
    setOrderedIds(standaloneMembers.map((member) => member.dataset_id))
  }, [standaloneMembers])

  function finishDrag(event) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIndex = orderedIds.indexOf(active.id)
    const newIndex = orderedIds.indexOf(over.id)
    const next = arrayMove(orderedIds, oldIndex, newIndex)
    setOrderedIds(next)
    onReorder(next)
  }

  const memberById = new Map(standaloneMembers.map((member) => [member.dataset_id, member]))

  return (
    <aside className="workspace-layer-panel bg-white dark:bg-[#202020] text-neutral-900 dark:text-white border-r border-neutral-200 dark:border-[#2e2e2e]" aria-label="Workspace layers">
      <div className="workspace-panel-header border-b border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020]">
        <div>
          <span className="workspace-eyebrow">Layer stack</span>
          <h2>Datasets</h2>
        </div>
        {onClose && (
          <button type="button" onClick={onClose} aria-label="Close layers">
            <X />
          </button>
        )}
      </div>

      {buildingGroups.size > 0 && totalDistinctFloors > 1 && (
        <div className="workspace-layer-filter-bar">
          <button
            type="button"
            className={`workspace-filter-chip ${floorFilter === "all" ? "is-active" : ""}`}
            onClick={() => handleFilterChange("all")}
          >
            All Floors ({totalDistinctFloors})
          </button>
          <button
            type="button"
            className={`workspace-filter-chip ${floorFilter === "active" ? "is-active" : ""}`}
            onClick={() => handleFilterChange("active")}
          >
            Active Floor Only
          </button>
        </div>
      )}

      <div className="workspace-layer-scroll">
        {/* Render file document groups (e.g. .pdf and .zip uploads) */}
        {fileGroups.map((group) => (
          <FileDocumentGroup
            key={group.id}
            group={group}
            selectedDatasetId={selectedDatasetId}
            onSelect={onSelect}
            onUpdate={onUpdate}
            onDetach={onDetach}
            onOpenBgRemoval={setBgRemovalMember}
            onOpenTransform={onOpenTransform}
            onOpenEnhancement={onOpenEnhancement}
            onOpenAnnotations={onOpenAnnotations}
          />
        ))}

        {/* Render building groups */}
        {Array.from(buildingGroups.entries()).map(([bName, floors]) => (
          <BuildingGroup
            key={bName}
            buildingName={bName}
            floors={floors}
            activeFloorLevel={activeFloorLevel}
            activeBuildingName={activeBuildingName}
            onSelectFloor={onSelectFloor}
            onSelect={onSelect}
            onUpdate={onUpdate}
            onDetach={onDetach}
            onOpenBgRemoval={setBgRemovalMember}
            onOpenTransform={onOpenTransform}
            onOpenEnhancement={onOpenEnhancement}
            onOpenAnnotations={onOpenAnnotations}
            buildings={buildings}
            onLayerChanged={onLayerChanged}
            hasAnyGeorefInWorkspace={hasAnyGeorefInWorkspace}
            floorFilter={floorFilter}
          />
        ))}

        {/* Render standalone datasets (e.g. 2nd dataset) */}
        {standaloneMembers.length > 0 && (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={finishDrag}>
            <SortableContext items={orderedIds} strategy={verticalListSortingStrategy}>
              {orderedIds.map((id) => {
                const member = memberById.get(id)
                return member ? (
                  <SortableMember
                    key={id}
                    member={member}
                    records={records}
                    selectedDatasetId={selectedDatasetId}
                    selectedKind={selectedKind}
                    settingsOpen={openSettingsId === member.dataset_id}
                    onToggleSettings={() => {
                      setOpenSettingsId((currentId) => nextExpandedLayerId(currentId, member.dataset_id))
                    }}
                    onSelect={onSelect}
                    onUpdate={onUpdate}
                    onDetach={onDetach}
                    onOpenBgRemoval={setBgRemovalMember}
                    onOpenTransform={onOpenTransform}
                    onOpenEnhancement={onOpenEnhancement}
                    onOpenAnnotations={onOpenAnnotations}
                  />
                ) : null
              })}
            </SortableContext>
          </DndContext>
        )}
      </div>

      {bgRemovalMember && (
        <BackgroundRemovalDialog
          open={Boolean(bgRemovalMember)}
          datasetId={bgRemovalMember.dataset_id}
          datasetName={bgRemovalMember.name}
          isGeoreferenced={bgRemovalMember.dataset?.georeference_status === "completed" || Boolean(bgRemovalMember.dataset?.map_layer)}
          onClose={() => setBgRemovalMember(null)}
          onApplied={() => {
            if (onLayerChanged) onLayerChanged()
          }}
        />
      )}
    </aside>
  )
}

