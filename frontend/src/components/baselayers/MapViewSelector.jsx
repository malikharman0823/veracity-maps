import React, { useState, useRef, useEffect } from "react"
import { SlidersHorizontal, ChevronDown, ChevronLeft, ChevronRight, Layers, Upload } from "lucide-react"
import MapViewFilters from "./MapViewFilters"
import { DEFAULT_MAP_VIEWS } from "../../lib/baseLayerSync"

export default function MapViewSelector({
  mapViews = DEFAULT_MAP_VIEWS,
  activeView = "world",
  customBaseLayer = null,
  customBaseLayers = [],
  prefs = {},
  onSelectView,
  onSelectCustomBaseLayer,
  onChangePrefs,
  onOpenBaseLayerModal,
  onOpenUploadBaseLayer,
  compact = false,
  embedded = false,
  collapsible = false,
}) {
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [customMenuOpen, setCustomMenuOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const containerRef = useRef(null)

  useEffect(() => {
    const handleOutsideClick = (e) => {
      if (containerRef.current && !containerRef.current.contains(e.target)) {
        setFiltersOpen(false)
        setCustomMenuOpen(false)
      }
    }
    document.addEventListener("mousedown", handleOutsideClick)
    return () => document.removeEventListener("mousedown", handleOutsideClick)
  }, [])

  const viewsList = mapViews && mapViews.length > 0 ? mapViews : DEFAULT_MAP_VIEWS
  const activeViewDef = viewsList.find((v) => v.type === activeView) || viewsList[0]
  const hasCustom = Boolean(customBaseLayer && !customBaseLayer.is_default)

  return (
    <div ref={containerRef} className="relative inline-flex items-center gap-1">
      <div
        className={
          embedded
            ? "inline-flex items-center gap-0.5"
            : "inline-flex items-center rounded-[8px] border border-neutral-700/80 bg-[#181a1f]/92 p-0.5 shadow-lg backdrop-blur-md"
        }
        role="group"
        aria-label="Map View Selector"
      >
        {collapsible && (
          <button
            type="button"
            onClick={() => {
              setFiltersOpen(false)
              setCustomMenuOpen(false)
              setCollapsed((prev) => !prev)
            }}
            title={collapsed ? "Expand Map Views" : "Collapse Map Views"}
            aria-expanded={!collapsed}
            className={`inline-flex items-center gap-1 rounded-[6px] font-semibold transition-colors cursor-pointer ${
              collapsed
                ? "px-2 py-1 text-[10px] text-[#3ecf8e] hover:bg-white/10"
                : "p-1 text-slate-400 hover:text-white hover:bg-white/10"
            }`}
          >
            {collapsed ? (
              <>
                <span>{activeViewDef?.name || "Views"}</span>
                <ChevronLeft className="w-3 h-3 text-slate-400" />
              </>
            ) : (
              <ChevronRight className="w-3 h-3" />
            )}
          </button>
        )}

        {!collapsed && (
          <>
            {viewsList.map((view) => {
          const isSelected = activeView === view.type
          const isCustomBtn = view.type === "custom"
          const isDisabled = !view.available
          return (
            <button
              key={view.type}
              type="button"
              disabled={isDisabled}
              title={
                isCustomBtn
                  ? hasCustom
                    ? `Custom Base Layer: ${customBaseLayer.name}`
                    : "Select a Custom Base Layer"
                  : view.description
              }
              onClick={() => {
                if (isCustomBtn) {
                  setFiltersOpen(false)
                  if (customBaseLayers.length === 0 && !hasCustom) {
                    setCustomMenuOpen(false)
                    ;(onOpenUploadBaseLayer || onOpenBaseLayerModal)?.()
                    return
                  }
                  if (!hasCustom && customBaseLayers.length > 0) {
                    onSelectCustomBaseLayer?.(customBaseLayers[0])
                    setCustomMenuOpen(true)
                    return
                  }
                  if (!isSelected) {
                    onSelectView?.("custom")
                    setCustomMenuOpen(true)
                    return
                  }
                  setCustomMenuOpen((p) => !p)
                  return
                }
                setCustomMenuOpen(false)
                onSelectView?.(view.type)
              }}
              className={`inline-flex items-center gap-1 rounded-[6px] font-semibold transition-colors cursor-pointer ${
                compact ? "px-2 py-1 text-[10px]" : "px-2.5 py-1 text-[11px]"
              } ${
                isSelected
                  ? "bg-[#3ecf8e] text-[#14161a] shadow-xs"
                  : "text-slate-300 hover:text-white hover:bg-white/10"
              } ${isDisabled ? "opacity-40 cursor-not-allowed" : ""}`}
            >
              <span>{view.name}</span>
              {isCustomBtn && <ChevronDown className="w-3 h-3 opacity-80" />}
            </button>
          )
        })}

        <div className="h-4 w-px bg-slate-700/80 mx-0.5" />

        <button
          type="button"
          onClick={() => {
            setCustomMenuOpen(false)
            setFiltersOpen((p) => !p)
          }}
          title="Map view filters, base layer opacity & visibility"
          aria-expanded={filtersOpen}
          className={`inline-flex items-center gap-1 rounded-[6px] px-2 py-1 text-[11px] font-medium transition-colors cursor-pointer ${
            filtersOpen
              ? "bg-slate-700 text-[#3ecf8e]"
              : "text-slate-300 hover:text-white hover:bg-white/10"
          }`}
        >
          <SlidersHorizontal className="w-3.5 h-3.5" />
        </button>
          </>
        )}
      </div>

      {/* Custom Base Layer Quick-Switcher Dropdown */}
      {customMenuOpen && customBaseLayers.length > 0 && (
        <div className="absolute right-0 top-full mt-1.5 z-[960] w-60 rounded-[10px] border border-neutral-700/80 bg-[#181a1f]/95 p-1.5 shadow-2xl backdrop-blur-md">
          <div className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-slate-400">
            My Custom Base Layers
          </div>
          <div className="max-h-52 overflow-y-auto space-y-0.5">
            {customBaseLayers.map((bl) => {
              const active = customBaseLayer?.id === bl.id
              return (
                <button
                  key={bl.id}
                  type="button"
                  onClick={() => {
                    setCustomMenuOpen(false)
                    onSelectCustomBaseLayer?.(bl)
                  }}
                  className={`w-full text-left px-2.5 py-1.5 rounded-[6px] text-[11px] flex items-center justify-between gap-2 cursor-pointer ${
                    active
                      ? "bg-[#3ecf8e]/20 text-[#3ecf8e] font-semibold"
                      : "text-slate-200 hover:bg-white/10"
                  }`}
                >
                  <span className="truncate">{bl.name}</span>
                  <span className="shrink-0 font-mono text-[9px] text-slate-400">
                    {bl.source_crs}
                  </span>
                </button>
              )
            })}
          </div>
          <div className="mt-1 pt-1 border-t border-slate-700/70 space-y-0.5">
            {(onOpenUploadBaseLayer || onOpenBaseLayerModal) && (
              <button
                type="button"
                onClick={() => {
                  setCustomMenuOpen(false)
                  ;(onOpenUploadBaseLayer || onOpenBaseLayerModal)?.()
                }}
                className="w-full inline-flex items-center justify-center gap-1.5 px-2 py-1.5 rounded-[6px] bg-[#3ecf8e]/15 text-[11px] font-semibold text-[#3ecf8e] hover:bg-[#3ecf8e]/25 cursor-pointer"
              >
                <Upload className="w-3.5 h-3.5" />
                Upload Map File as Base Layer
              </button>
            )}
            {onOpenBaseLayerModal && (
              <button
                type="button"
                onClick={() => {
                  setCustomMenuOpen(false)
                  onOpenBaseLayerModal()
                }}
                className="w-full inline-flex items-center justify-center gap-1.5 px-2 py-1.5 rounded-[6px] text-[11px] font-medium text-slate-300 hover:bg-white/10 cursor-pointer"
              >
                <Layers className="w-3.5 h-3.5" />
                Browse / Select Base Layer…
              </button>
            )}
          </div>
        </div>
      )}

      {/* Map View Filters Popover */}
      {filtersOpen && (
        <div className="absolute right-0 top-full mt-1.5 z-[960]">
          <MapViewFilters
            activeViewDef={activeViewDef}
            customBaseLayer={customBaseLayer}
            prefs={prefs}
            onChangePrefs={onChangePrefs}
            onOpenBaseLayerModal={() => {
              setFiltersOpen(false)
              onOpenBaseLayerModal?.()
            }}
          />
        </div>
      )}
    </div>
  )
}
