import React, { useEffect, useMemo, useState } from "react"
import { Search, X, Check, Clock, Grid } from "lucide-react"
import { listIcons, getIconUrl } from "../api"

const RECENT_ICONS_KEY = "veracity_recent_icon_ids"

export default function IconPicker({
  selectedIconId,
  onSelectIcon,
  onClose,
  title = "Select Map Icon",
  placementModeActive = false,
  onStartPlacement,
}) {
  const [icons, setIcons] = useState([])
  const [categories, setCategories] = useState([])
  const [selectedCategory, setSelectedCategory] = useState("All")
  const [searchQuery, setSearchQuery] = useState("")
  const [loading, setLoading] = useState(true)
  const [recentIconIds, setRecentIconIds] = useState(() => {
    try {
      const saved = localStorage.getItem(RECENT_ICONS_KEY)
      return saved ? JSON.parse(saved) : []
    } catch {
      return []
    }
  })

  useEffect(() => {
    let mounted = true
    listIcons()
      .then((data) => {
        if (!mounted) return
        const catalog = data?.catalog || []
        const cats = data?.categories || []
        setIcons(catalog)
        setCategories(cats)
      })
      .catch((err) => {
        console.warn("Could not load icon catalog:", err)
      })
      .finally(() => {
        if (mounted) setLoading(false)
      })
    return () => {
      mounted = false
    }
  }, [])

  const filteredIcons = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    return icons.filter((icon) => {
      const matchesCategory =
        selectedCategory === "All" || icon.category === selectedCategory
      const matchesSearch =
        !query ||
        icon.name?.toLowerCase().includes(query) ||
        icon.id?.toLowerCase().includes(query) ||
        icon.category?.toLowerCase().includes(query)
      return matchesCategory && matchesSearch
    })
  }, [icons, selectedCategory, searchQuery])

  const recentIcons = useMemo(() => {
    if (!recentIconIds.length || !icons.length) return []
    return recentIconIds
      .map((id) => icons.find((ic) => ic.id === id))
      .filter(Boolean)
      .slice(0, 6)
  }, [recentIconIds, icons])

  function handleChooseIcon(icon) {
    onSelectIcon(icon)
    try {
      const updated = [icon.id, ...recentIconIds.filter((id) => id !== icon.id)].slice(0, 10)
      setRecentIconIds(updated)
      localStorage.setItem(RECENT_ICONS_KEY, JSON.stringify(updated))
    } catch {
      // storage error fallback
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40 backdrop-blur-xs">
      <div
        className="bg-white rounded-xl shadow-2xl border border-gray-200 w-full max-w-2xl flex flex-col max-h-[85vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3.5 border-b border-gray-100 bg-gray-50/50">
          <div className="flex items-center gap-2">
            <div className="p-1.5 bg-blue-50 text-blue-600 rounded-lg">
              <Grid className="size-4" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
              <p className="text-xs text-gray-500">
                {icons.length} geographic icons available
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-gray-400 hover:text-gray-700 hover:bg-gray-100 rounded-lg transition-colors"
            aria-label="Close icon picker"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Search & Category Filter */}
        <div className="p-4 border-b border-gray-100 space-y-3 bg-white">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-gray-400 pointer-events-none" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search icons by name or tag…"
              className="w-full pl-9 pr-8 py-2 text-xs bg-gray-50 border border-gray-200 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 transition-all text-gray-800 placeholder:text-gray-400"
              autoFocus
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => setSearchQuery("")}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 p-0.5"
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>

          {/* Categories Horizontal Scroll */}
          <div className="flex items-center gap-1.5 overflow-x-auto pb-1 no-scrollbar text-xs">
            <button
              type="button"
              onClick={() => setSelectedCategory("All")}
              className={`px-3 py-1 rounded-full text-xs font-medium whitespace-nowrap transition-colors ${
                selectedCategory === "All"
                  ? "bg-gray-900 text-white"
                  : "bg-gray-100 text-gray-600 hover:bg-gray-200"
              }`}
            >
              All ({icons.length})
            </button>
            {categories.map((cat) => {
              const count = icons.filter((i) => i.category === cat).length
              return (
                <button
                  key={cat}
                  type="button"
                  onClick={() => setSelectedCategory(cat)}
                  className={`px-3 py-1 rounded-full text-xs font-medium whitespace-nowrap transition-colors ${
                    selectedCategory === cat
                      ? "bg-gray-900 text-white"
                      : "bg-gray-100 text-gray-600 hover:bg-gray-200"
                  }`}
                >
                  {cat} ({count})
                </button>
              )
            })}
          </div>
        </div>

        {/* Recently Used Bar */}
        {recentIcons.length > 0 && !searchQuery && selectedCategory === "All" && (
          <div className="px-4 py-2 bg-gray-50/70 border-b border-gray-100 flex items-center gap-2">
            <div className="flex items-center gap-1 text-[11px] font-medium text-gray-500 shrink-0">
              <Clock className="size-3" />
              <span>Recent:</span>
            </div>
            <div className="flex items-center gap-2 overflow-x-auto">
              {recentIcons.map((rec) => {
                const isSelected = selectedIconId === rec.id
                return (
                  <button
                    key={rec.id}
                    type="button"
                    onClick={() => handleChooseIcon(rec)}
                    title={rec.name}
                    className={`p-1 rounded-md border flex items-center justify-center transition-all ${
                      isSelected
                        ? "border-blue-600 bg-blue-50 ring-1 ring-blue-600"
                        : "border-gray-200 bg-white hover:border-gray-400"
                    }`}
                  >
                    <img
                      src={getIconUrl(rec.id)}
                      alt={rec.name}
                      className="size-5 object-contain"
                      loading="lazy"
                    />
                  </button>
                )
              })}
            </div>
          </div>
        )}

        {/* Icons Grid Gallery */}
        <div className="flex-1 overflow-y-auto p-4 min-h-[260px]">
          {loading ? (
            <div className="h-48 flex items-center justify-center text-xs text-gray-400">
              Loading icon catalog…
            </div>
          ) : filteredIcons.length === 0 ? (
            <div className="h-48 flex flex-col items-center justify-center text-center text-gray-400 gap-1">
              <p className="text-xs font-medium text-gray-600">No icons found</p>
              <p className="text-[11px]">Try adjusting your search or category filter.</p>
            </div>
          ) : (
            <div className="grid grid-cols-4 sm:grid-cols-6 md:grid-cols-8 gap-2.5">
              {filteredIcons.map((icon) => {
                const isSelected = selectedIconId === icon.id
                return (
                  <button
                    key={icon.id}
                    type="button"
                    onClick={() => handleChooseIcon(icon)}
                    title={`${icon.name} (${icon.category})`}
                    className={`group relative flex flex-col items-center justify-center p-2 rounded-xl border text-center transition-all ${
                      isSelected
                        ? "border-blue-600 bg-blue-50/70 shadow-xs ring-2 ring-blue-600/20"
                        : "border-gray-100 hover:border-gray-300 hover:bg-gray-50/80 bg-white"
                    }`}
                  >
                    {/* Checkerboard transparency preview container */}
                    <div className="size-11 rounded-lg flex items-center justify-center p-1 relative overflow-hidden bg-[radial-gradient(#e5e7eb_1px,transparent_1px)] [background-size:6px_6px]">
                      <img
                        src={getIconUrl(icon.id)}
                        alt={icon.name}
                        className="max-size-full max-h-8 max-w-8 object-contain transition-transform group-hover:scale-110 drop-shadow-xs"
                        loading="lazy"
                      />
                    </div>
                    <span className="mt-1 text-[10px] text-gray-700 font-medium truncate w-full text-center group-hover:text-gray-900">
                      {icon.name}
                    </span>
                    {isSelected && (
                      <span className="absolute top-1 right-1 size-4 bg-blue-600 text-white rounded-full flex items-center justify-center shadow-xs">
                        <Check className="size-2.5 stroke-[3]" />
                      </span>
                    )}
                  </button>
                )
              })}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-5 py-3 border-t border-gray-100 bg-gray-50/70">
          <div className="text-xs text-gray-500">
            {selectedIconId ? (
              <span className="flex items-center gap-1.5">
                Selected: <strong className="text-gray-900">{selectedIconId}</strong>
              </span>
            ) : (
              <span>Click an icon to select it</span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3.5 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-200/60 rounded-lg transition-colors border border-gray-200"
            >
              Done
            </button>
            {onStartPlacement && (
              <button
                type="button"
                disabled={!selectedIconId}
                onClick={() => {
                  onStartPlacement()
                  onClose()
                }}
                className="px-4 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:pointer-events-none rounded-lg transition-colors shadow-xs"
              >
                {placementModeActive ? "Placement Active" : "Place Point on Map →"}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
