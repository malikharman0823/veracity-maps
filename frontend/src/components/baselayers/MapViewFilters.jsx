import React from "react"
import { Eye, EyeOff, Layers, SlidersHorizontal, Tag, Route, MapPin } from "lucide-react"

export default function MapViewFilters({
  activeViewDef,
  customBaseLayer,
  prefs,
  onChangePrefs,
  onOpenBaseLayerModal,
}) {
  const caps = activeViewDef?.capabilities || {
    supports_labels: false,
    supports_roads: false,
    supports_places: false,
    supports_opacity: true,
  }
  const opacity = typeof prefs?.base_layer_opacity === "number" ? prefs.base_layer_opacity : 1.0
  const visible = prefs?.base_layer_visible !== false
  const hasAnyProviderFilter = caps.supports_labels || caps.supports_roads || caps.supports_places

  return (
    <div className="w-64 rounded-[10px] border border-neutral-700/80 bg-[#181a1f]/95 p-3 text-xs text-slate-200 shadow-2xl backdrop-blur-md space-y-3">
      <div className="flex items-center justify-between border-b border-slate-700/70 pb-2">
        <div className="flex items-center gap-1.5 font-semibold text-white">
          <SlidersHorizontal className="w-3.5 h-3.5 text-[#3ecf8e]" />
          <span>Map View &amp; Base Controls</span>
        </div>
        <button
          type="button"
          onClick={() => onChangePrefs?.({ base_layer_visible: !visible })}
          className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-semibold cursor-pointer ${
            visible
              ? "bg-[#3ecf8e]/20 text-[#3ecf8e]"
              : "bg-slate-800 text-slate-400"
          }`}
          title={visible ? "Hide background base layer" : "Show background base layer"}
        >
          {visible ? <Eye className="w-3 h-3" /> : <EyeOff className="w-3 h-3" />}
          {visible ? "Visible" : "Hidden"}
        </button>
      </div>

      {/* Base Layer Opacity */}
      <div className="space-y-1">
        <div className="flex items-center justify-between text-[11px]">
          <span className="text-slate-300 font-medium">Base Layer Opacity</span>
          <span className="font-mono text-[10px] text-[#3ecf8e]">
            {Math.round(opacity * 100)}%
          </span>
        </div>
        <input
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={opacity}
          onChange={(e) => onChangePrefs?.({ opacity: parseFloat(e.target.value) })}
          className="w-full accent-[#3ecf8e] cursor-pointer h-1.5 bg-slate-700 rounded-lg"
        />
      </div>

      {/* Provider-dependent View Filters */}
      <div className="space-y-1.5 pt-1 border-t border-slate-700/70">
        <div className="text-[10px] uppercase tracking-wide text-slate-400 font-semibold">
          Background Map Layers ({activeViewDef?.name || "World"})
        </div>

        {hasAnyProviderFilter ? (
          <div className="space-y-1.5">
            {caps.supports_labels && (
              <label className="flex items-center justify-between py-0.5 cursor-pointer">
                <span className="inline-flex items-center gap-1.5 text-[11px] text-slate-200">
                  <Tag className="w-3.5 h-3.5 text-slate-400" />
                  Show Labels
                </span>
                <input
                  type="checkbox"
                  checked={prefs?.show_labels !== false}
                  onChange={(e) => onChangePrefs?.({ show_labels: e.target.checked })}
                  className="accent-[#3ecf8e] cursor-pointer"
                />
              </label>
            )}
            {caps.supports_roads && (
              <label className="flex items-center justify-between py-0.5 cursor-pointer">
                <span className="inline-flex items-center gap-1.5 text-[11px] text-slate-200">
                  <Route className="w-3.5 h-3.5 text-slate-400" />
                  Show Roads
                </span>
                <input
                  type="checkbox"
                  checked={prefs?.show_roads !== false}
                  onChange={(e) => onChangePrefs?.({ show_roads: e.target.checked })}
                  className="accent-[#3ecf8e] cursor-pointer"
                />
              </label>
            )}
            {caps.supports_places && (
              <label className="flex items-center justify-between py-0.5 cursor-pointer">
                <span className="inline-flex items-center gap-1.5 text-[11px] text-slate-200">
                  <MapPin className="w-3.5 h-3.5 text-slate-400" />
                  Show Place Names
                </span>
                <input
                  type="checkbox"
                  checked={prefs?.show_places !== false}
                  onChange={(e) => onChangePrefs?.({ show_places: e.target.checked })}
                  className="accent-[#3ecf8e] cursor-pointer"
                />
              </label>
            )}
          </div>
        ) : (
          <p className="text-[10px] text-slate-400 leading-relaxed">
            This map style uses baked imagery tiles; label/road sub-layer filters are available in Satellite, Streets, Light, and Dark views.
          </p>
        )}
      </div>

      {/* Active Custom Base Layer Info & Switcher */}
      <div className="pt-2 border-t border-slate-700/70 space-y-1.5">
        <div className="flex items-center justify-between text-[10px] text-slate-400">
          <span>Geographic Reference</span>
          <span className="font-mono text-slate-300">
            {customBaseLayer && !customBaseLayer.is_default ? customBaseLayer.source_crs : "EPSG:3857"}
          </span>
        </div>
        <div className="text-[11px] font-medium text-white truncate">
          {customBaseLayer && !customBaseLayer.is_default ? customBaseLayer.name : "Default World Map"}
        </div>
        {onOpenBaseLayerModal && (
          <button
            type="button"
            onClick={onOpenBaseLayerModal}
            className="w-full inline-flex items-center justify-center gap-1.5 px-2.5 py-1.5 rounded-[6px] bg-slate-800 hover:bg-slate-700 text-[11px] font-semibold text-[#3ecf8e] cursor-pointer transition-colors"
          >
            <Layers className="w-3.5 h-3.5" />
            Change Base Layer…
          </button>
        )}
      </div>
    </div>
  )
}
