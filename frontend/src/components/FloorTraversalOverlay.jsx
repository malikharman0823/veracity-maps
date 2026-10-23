import React, { useState, useEffect, useMemo, useCallback } from "react"
import {
  Building2,
  ChevronUp,
  ChevronDown,
  Layers,
  Eye,
  EyeOff,
  Sliders,
  CheckCircle2,
  ChevronRight,
  Maximize2,
  Minimize2,
  Settings,
} from "lucide-react"
import { resolveBuildingName } from "../lib/buildingUtils"

export default function FloorTraversalOverlay({
  members = [],
  buildings = [],
  activeFloorLevel = 0,
  onSelectFloor,
  onUpdateMember,
  isolateActiveFloor = false,
  onToggleIsolateActiveFloor,
  onOpenBackgroundRemoval,
}) {
  const [selectedBuilding, setSelectedBuilding] = useState("")

  const [showOpacitySlider, setShowOpacitySlider] = useState(false)
  const [isCollapsed, setIsCollapsed] = useState(false)

  // 1. Group building floor members by building name and distinct floor levels
  const buildingsMap = useMemo(() => {
    const bldMap = new Map()
    members.forEach((m) => {
      const isFloor = Boolean(
        m.is_building_floor ||
        m.building_name ||
        m.floor_level !== undefined ||
        m.dataset?.extracted_metadata?.is_building_floor ||
        m.dataset?.extracted_metadata?.building_name ||
        m.dataset?.extracted_metadata?.floor_level !== undefined
      )
      if (isFloor) {
        let bName = resolveBuildingName(m, members, buildings) || "Building 1"
        if (!bldMap.has(bName)) {
          bldMap.set(bName, new Map())
        }
        const levelMap = bldMap.get(bName)
        const floorLevel = m.floor_level ?? m.dataset?.extracted_metadata?.floor_level ?? 0
        const floorLabel =
          m.floor_label ||
          m.dataset?.extracted_metadata?.floor_label ||
          m.name ||
          m.dataset?.filename ||
          (floorLevel === -1 ? "Basement" : floorLevel === 0 ? "Ground Floor" : `Floor ${floorLevel}`)

        if (!levelMap.has(floorLevel)) {
          levelMap.set(floorLevel, {
            ...m,
            floor_level: floorLevel,
            floor_label: floorLabel,
            members: [m],
          })
        } else {
          const existing = levelMap.get(floorLevel)
          existing.members.push(m)
          if (m.floor_label && m.floor_label !== m.name && existing.floor_label === `Floor ${floorLevel}`) {
            existing.floor_label = m.floor_label
          }
          if (m.dataset?.georeference_status === "completed" || m.dataset?.map_layer || m.map_layer) {
            existing.dataset = m.dataset || existing.dataset
            existing.map_layer = m.map_layer || m.dataset?.map_layer || existing.map_layer
          }
        }
      }
    })

    const finalMap = new Map()
    for (const [bName, levelMap] of bldMap.entries()) {
      const sortedFloors = Array.from(levelMap.values()).sort((a, b) => (b.floor_level ?? 0) - (a.floor_level ?? 0))
      finalMap.set(bName, sortedFloors)
    }
    return finalMap
  }, [members, buildings])

  const buildingNames = useMemo(() => {
    const names = Array.from(buildingsMap.keys())
    if (names.length === 0 && buildings?.length > 0) {
      return buildings.map((b) => b.name)
    }
    return names
  }, [buildingsMap, buildings])

  // Automatically determine or fallback active building
  const activeBuildingName = useMemo(() => {
    if (selectedBuilding && buildingNames.includes(selectedBuilding)) {
      return selectedBuilding
    }
    // Find building of the currently active floor level
    for (const [bName, floors] of buildingsMap.entries()) {
      if (floors.some((f) => Number(f.floor_level ?? 0) === Number(activeFloorLevel ?? 0))) {
        return bName
      }
    }
    return buildingNames[0] || ""
  }, [selectedBuilding, buildingNames, buildingsMap, activeFloorLevel])

  // Get floors belonging to current building, sorted descending (highest floor at top, basements at bottom)
  const currentFloors = useMemo(() => {
    return buildingsMap.get(activeBuildingName) || []
  }, [buildingsMap, activeBuildingName])

  // Active floor member
  const activeFloorMember = useMemo(() => {
    return (
      currentFloors.find((f) => Number(f.floor_level ?? 0) === Number(activeFloorLevel ?? 0)) ||
      currentFloors[0] ||
      null
    )
  }, [currentFloors, activeFloorLevel])

  // Find index of current floor in sorted descending list with closest-match fallback
  const currentIndex = useMemo(() => {
    const idx = currentFloors.findIndex((f) => Number(f.floor_level ?? 0) === Number(activeFloorLevel ?? 0))
    if (idx !== -1) return idx
    if (currentFloors.length === 0) return -1
    let closestIdx = 0
    let minDiff = Infinity
    currentFloors.forEach((f, i) => {
      const diff = Math.abs(Number(f.floor_level ?? 0) - Number(activeFloorLevel ?? 0))
      if (diff < minDiff) {
        minDiff = diff
        closestIdx = i
      }
    })
    return closestIdx
  }, [currentFloors, activeFloorLevel])

  // Traversal climbing functions (descending list: index - 1 is UP, index + 1 is DOWN)
  const canClimbUp = currentIndex > 0
  const canClimbDown = currentIndex >= 0 && currentIndex < currentFloors.length - 1

  const handleSelectFloor = useCallback((level) => {
    const numLevel = Number(level)
    if (onSelectFloor) {
      onSelectFloor(numLevel, activeBuildingName)
    }
    const target = currentFloors.find((f) => Number(f.floor_level ?? 0) === numLevel)
    if (target && onUpdateMember) {
      const targets = target.members || [target]
      targets.forEach((m) => {
        if (m.visible === false || m.raster_visible === false) {
          onUpdateMember(m.dataset_id, { visible: true, raster_visible: true })
        }
      })
    }
  }, [currentFloors, activeBuildingName, onSelectFloor, onUpdateMember])

  const handleClimbUp = useCallback(() => {
    if (!canClimbUp) return
    const nextFloor = currentFloors[currentIndex - 1]
    if (nextFloor) {
      handleSelectFloor(nextFloor.floor_level ?? 0)
    }
  }, [canClimbUp, currentFloors, currentIndex, handleSelectFloor])

  const handleClimbDown = useCallback(() => {
    if (!canClimbDown) return
    const prevFloor = currentFloors[currentIndex + 1]
    if (prevFloor) {
      handleSelectFloor(prevFloor.floor_level ?? 0)
    }
  }, [canClimbDown, currentFloors, currentIndex, handleSelectFloor])

  // Keyboard Navigation: ArrowUp / ArrowDown
  useEffect(() => {
    function handleKeyDown(e) {
      const tag = document.activeElement?.tagName?.toLowerCase()
      if (tag === "input" || tag === "textarea" || tag === "select" || document.activeElement?.isContentEditable) {
        return
      }

      if (e.key === "ArrowUp" || e.key === "PageUp") {
        if (canClimbUp) {
          e.preventDefault()
          handleClimbUp()
        }
      } else if (e.key === "ArrowDown" || e.key === "PageDown") {
        if (canClimbDown) {
          e.preventDefault()
          handleClimbDown()
        }
      }
    }

    window.addEventListener("keydown", handleKeyDown)
    return () => window.removeEventListener("keydown", handleKeyDown)
  }, [canClimbUp, canClimbDown, handleClimbUp, handleClimbDown])

  // Toggle active floor raster visibility
  function toggleRasterVisibility() {
    if (!activeFloorMember || !onUpdateMember) return
    const currentVis = (activeFloorMember.visible !== false) && (activeFloorMember.raster_visible !== false)
    const targets = activeFloorMember.members || [activeFloorMember]
    targets.forEach((m) => onUpdateMember(m.dataset_id, { visible: !currentVis, raster_visible: !currentVis }))
  }

  // Update active floor raster opacity
  function handleOpacityChange(e) {
    if (!activeFloorMember || !onUpdateMember) return
    const opacity = parseFloat(e.target.value)
    const targets = activeFloorMember.members || [activeFloorMember]
    targets.forEach((m) => onUpdateMember(m.dataset_id, { raster_opacity: opacity }))
  }

  if (currentFloors.length === 0) {
    return null
  }

  const activeOpacity = activeFloorMember?.raster_opacity ?? 1.0
  const isRasterVisible = (activeFloorMember?.visible !== false) && (activeFloorMember?.raster_visible !== false)

  function formatBadge(level) {
    if (level <= -2) return `B${Math.abs(level)}`
    if (level === -1) return "B1"
    if (level === 0) return "G"
    return `${level}`
  }

  function formatFloorName(floor) {
    if (floor.floor_label) return floor.floor_label
    const lvl = floor.floor_level ?? 0
    if (lvl === 0) return "Ground Floor"
    if (lvl === -1) return "Basement 1"
    if (lvl < -1) return `Basement ${Math.abs(lvl)}`
    return `Floor ${lvl}`
  }

  return (
    <div
      className="absolute top-4 right-14 z-20 flex flex-col items-end pointer-events-none transition-all duration-200"
      role="region"
      aria-label="Floor Traversal Overlay"
    >
      <div className="pointer-events-auto bg-white/95 backdrop-blur-md border border-slate-200/90 rounded-2xl shadow-xl shadow-slate-900/10 p-3 w-56 flex flex-col gap-2.5 text-slate-800 transition-all select-none">
        {/* Header: Building Identity & Switcher */}
        <div className="flex items-center justify-between pb-2 border-b border-slate-100">
          <div className="flex items-center gap-1.5 min-w-0">
            <div className="size-6 rounded-lg bg-blue-50 text-blue-600 flex items-center justify-center shrink-0">
              <Building2 className="size-3.5" />
            </div>
            {buildingNames.length > 1 ? (
              <select
                value={activeBuildingName}
                onChange={(e) => {
                  setSelectedBuilding(e.target.value)
                  const targetList = buildingsMap.get(e.target.value) || []
                  if (targetList.length > 0 && onSelectFloor) {
                    onSelectFloor(targetList[0].floor_level ?? 0, e.target.value)
                  }
                }}
                className="text-xs font-semibold text-slate-800 bg-transparent truncate border-none focus:outline-hidden cursor-pointer"
                title="Switch Building"
              >
                {buildingNames.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
            ) : (
              <span className="text-xs font-semibold text-slate-800 truncate" title={activeBuildingName}>
                {activeBuildingName || "Building"}
              </span>
            )}
          </div>

          <div className="flex items-center gap-1">
            <span className="text-[10px] font-medium text-slate-400">
              {currentFloors.length} Fl
            </span>
            <button
              type="button"
              onClick={() => setIsCollapsed((v) => !v)}
              className="p-1 rounded-md text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition-colors"
              title={isCollapsed ? "Expand floor controls" : "Collapse floor controls"}
            >
              {isCollapsed ? <Maximize2 className="size-3" /> : <Minimize2 className="size-3" />}
            </button>
          </div>
        </div>

        {/* Traversal Climbing Step Controls */}
        <div className="flex items-center gap-1.5 bg-slate-50 p-1 rounded-xl border border-slate-200/60">
          <button
            type="button"
            onClick={handleClimbUp}
            disabled={!canClimbUp}
            className={`flex-1 py-1 px-2 rounded-lg text-xs font-medium flex items-center justify-center gap-1 transition-all ${
              canClimbUp
                ? "bg-white text-slate-800 shadow-xs hover:bg-blue-50 hover:text-blue-600 active:scale-98"
                : "text-slate-300 cursor-not-allowed"
            }`}
            title="Climb Up one floor (Arrow Up ↑)"
          >
            <ChevronUp className="size-3.5" />
            <span className="text-[11px]">Up</span>
          </button>

          <div className="px-1.5 text-center">
            <span className="text-[11px] font-bold text-blue-600 font-mono">
              {formatBadge(activeFloorLevel)}
            </span>
          </div>

          <button
            type="button"
            onClick={handleClimbDown}
            disabled={!canClimbDown}
            className={`flex-1 py-1 px-2 rounded-lg text-xs font-medium flex items-center justify-center gap-1 transition-all ${
              canClimbDown
                ? "bg-white text-slate-800 shadow-xs hover:bg-blue-50 hover:text-blue-600 active:scale-98"
                : "text-slate-300 cursor-not-allowed"
            }`}
            title="Climb Down one floor (Arrow Down ↓)"
          >
            <ChevronDown className="size-3.5" />
            <span className="text-[11px]">Down</span>
          </button>
        </div>

        {/* Vertical Floor Stack (Expanded View) */}
        {!isCollapsed && (
          <div className="flex flex-col gap-1 max-h-52 overflow-y-auto pr-0.5 custom-scrollbar">
            {currentFloors.map((floor) => {
              const level = floor.floor_level ?? 0
              const isActive = activeFloorLevel === level
              const badge = formatBadge(level)
              const name = formatFloorName(floor)
              const isGeoref = Boolean(
                floor.dataset?.georeference_status === "completed" ||
                floor.dataset?.map_layer ||
                floor.map_layer
              )
              const pointsCount = floor.sublayers?.find((s) => s.kind === "icon_point")?.count || 0

              const isFloorVisible = (floor.visible !== false) && (floor.raster_visible !== false)

              return (
                <div
                  key={`${activeBuildingName}_level_${level}`}
                  onClick={() => handleSelectFloor(level)}
                  className={`w-full flex items-center justify-between px-2.5 py-1.5 rounded-xl text-left text-xs transition-all cursor-pointer ${
                    isActive
                      ? "bg-blue-600 text-white font-medium shadow-sm shadow-blue-500/30"
                      : "text-slate-700 hover:bg-slate-100 hover:text-slate-900"
                  }`}
                  title={`Click to select ${name} (Level ${level})`}
                >
                  <div className="flex items-center gap-2 truncate">
                    <input
                      type="checkbox"
                      checked={isFloorVisible}
                      onChange={(e) => {
                        e.stopPropagation()
                        const targetVis = e.target.checked
                        const targets = floor.members || [floor]
                        if (onUpdateMember) {
                          targets.forEach((m) =>
                            onUpdateMember(m.dataset_id, {
                              visible: targetVis,
                              raster_visible: targetVis,
                            })
                          )
                        }
                      }}
                      className="rounded border-slate-300 text-blue-600 focus:ring-blue-500 cursor-pointer size-3.5"
                      title={isFloorVisible ? "Visible on map (click to hide)" : "Hidden on map (click to show)"}
                    />
                    <span
                      className={`inline-flex items-center justify-center min-w-5 h-4.5 px-1 rounded-md text-[10px] font-bold font-mono ${
                        isActive
                          ? "bg-white/20 text-white"
                          : "bg-slate-200 text-slate-700"
                      }`}
                    >
                      {badge}
                    </span>
                    <span className="truncate">{name}</span>
                  </div>

                  <div className="flex items-center gap-1 shrink-0 ml-1.5">
                    {pointsCount > 0 && (
                      <span
                        className={`text-[9px] px-1 py-0.2 rounded-full font-mono ${
                          isActive
                            ? "bg-blue-700/60 text-blue-100"
                            : "bg-slate-200/80 text-slate-600"
                        }`}
                        title={`${pointsCount} icon points on this floor`}
                      >
                        {pointsCount}
                      </span>
                    )}
                    {isGeoref && (
                      <CheckCircle2
                        className={`size-3 ${
                          isActive ? "text-emerald-300" : "text-emerald-500"
                        }`}
                        title="Georeferenced footprint active"
                      />
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}


        {/* Active Floor Raster Quick Controls */}
        <div className="pt-2 border-t border-slate-100 flex flex-col gap-2">
          <div className="flex items-center justify-between text-xs">
            <span className="text-[11px] font-medium text-slate-500">
              Raster Overlay
            </span>
            <div className="flex items-center gap-1.5">
              {onToggleIsolateActiveFloor && (
                <button
                  type="button"
                  onClick={onToggleIsolateActiveFloor}
                  className={`p-1 rounded-md text-xs transition-colors ${
                    isolateActiveFloor
                      ? "bg-amber-100 text-amber-800"
                      : "text-slate-400 hover:text-slate-600 hover:bg-slate-100"
                  }`}
                  title={isolateActiveFloor ? "Mode: Solo Active Floor (click to show all visible layers)" : "Mode: All Checked Layers (click to solo active floor)"}
                >
                  <Layers className="size-3" />
                </button>
              )}

              <button
                type="button"
                onClick={() => setShowOpacitySlider((v) => !v)}
                className={`p-1 rounded-md text-xs transition-colors ${
                  showOpacitySlider
                    ? "bg-blue-100 text-blue-700"
                    : "text-slate-400 hover:text-slate-600 hover:bg-slate-100"
                }`}
                title="Adjust raster opacity"
              >
                <Sliders className="size-3" />
              </button>

              {onOpenBackgroundRemoval && activeFloorMember && (
                <button
                  type="button"
                  onClick={() => onOpenBackgroundRemoval(activeFloorMember)}
                  className="p-1 rounded-md text-xs text-slate-400 hover:text-blue-600 hover:bg-blue-50 transition-colors"
                  title="Background removal & transparency settings (⚙)"
                >
                  <Settings className="size-3" />
                </button>
              )}

              <button
                type="button"
                onClick={toggleRasterVisibility}
                className={`p-1 rounded-md text-xs transition-colors ${
                  isRasterVisible
                    ? "text-blue-600 hover:bg-blue-50"
                    : "text-slate-400 hover:bg-slate-100"
                }`}
                title={isRasterVisible ? "Hide floor plan image" : "Show floor plan image"}
              >
                {isRasterVisible ? <Eye className="size-3" /> : <EyeOff className="size-3" />}
              </button>
            </div>
          </div>


          {/* Opacity slider popup */}
          {showOpacitySlider && (
            <div className="flex items-center gap-2 px-1 pt-1 animate-in fade-in slide-in-from-top-1 duration-150">
              <input
                type="range"
                min="0"
                max="1"
                step="0.05"
                value={activeOpacity}
                onChange={handleOpacityChange}
                className="w-full h-1.5 bg-slate-200 rounded-lg appearance-none cursor-pointer accent-blue-600"
                title={`Opacity: ${Math.round(activeOpacity * 100)}%`}
              />
              <span className="text-[10px] font-mono font-medium text-slate-500 min-w-[28px] text-right">
                {Math.round(activeOpacity * 100)}%
              </span>
            </div>
          )}
        </div>

        {/* Keyboard Traversal Hint */}
        <div className="text-[9px] text-slate-400 text-center font-medium pt-0.5">
          Tip: Press <kbd className="px-1 py-0.5 bg-slate-100 rounded text-[8px] text-slate-600 border border-slate-200">↑</kbd> or <kbd className="px-1 py-0.5 bg-slate-100 rounded text-[8px] text-slate-600 border border-slate-200">↓</kbd> to climb floors
        </div>
      </div>
    </div>
  )
}
