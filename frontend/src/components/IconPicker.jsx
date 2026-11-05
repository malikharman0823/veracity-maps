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
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-xs">
      <div
        className="bg-white dark:bg-[#202020] rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] w-full max-w-2xl flex flex-col max-h-[85vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150 text-neutral-900 dark:text-white"
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3.5 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50/80 dark:bg-[#171717]/80">
          <div className="flex items-center gap-2">
            <div className="p-1.5 bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e] rounded-[6px] border border-[#3ecf8e]/30">
              <Grid className="size-4" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">{title}</h3>
              <p className="text-xs text-neutral-500 dark:text-neutral-400">
                {icons.length} geographic icons available
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px] transition-colors cursor-pointer"
            aria-label="Close icon picker"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Search & Category Filter */}
        <div className="p-4 border-b border-neutral-200 dark:border-[#2e2e2e] space-y-3 bg-white dark:bg-[#202020]">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-neutral-400 pointer-events-none" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search icons by name or tag…"
              className="w-full pl-9 pr-8 py-2 text-xs bg-neutral-50 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] focus:outline-none focus:border-[#3ecf8e] transition-all text-neutral-900 dark:text-white placeholder:text-neutral-400"
              autoFocus
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => setSearchQuery("")}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 p-0.5 cursor-pointer"
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
              className={`px-3 py-1 rounded-[6px] text-xs font-medium whitespace-nowrap transition-colors cursor-pointer ${
                selectedCategory === "All"
                  ? "bg-[#3ecf8e] text-[#171717]"
                  : "bg-neutral-100 dark:bg-[#282828] text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232]"
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
                  className={`px-3 py-1 rounded-[6px] text-xs font-medium whitespace-nowrap transition-colors cursor-pointer ${
                    selectedCategory === cat
                      ? "bg-[#3ecf8e] text-[#171717]"
                      : "bg-neutral-100 dark:bg-[#282828] text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#323232]"
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
          <div className="px-4 py-2 bg-neutral-50/80 dark:bg-[#171717]/80 border-b border-neutral-200 dark:border-[#2e2e2e] flex items-center gap-2">
            <div className="flex items-center gap-1 text-[11px] font-medium text-neutral-500 dark:text-neutral-400 shrink-0">
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
                    className={`p-1 rounded-[6px] border flex items-center justify-center transition-all cursor-pointer ${
                      isSelected
                        ? "border-[#3ecf8e] bg-[#3ecf8e]/10 ring-1 ring-[#3ecf8e]"
                        : "border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] hover:border-neutral-400 dark:hover:border-neutral-500"
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
            <div className="h-48 flex items-center justify-center text-xs text-neutral-400">
              Loading icon catalog…
            </div>
          ) : filteredIcons.length === 0 ? (
            <div className="h-48 flex flex-col items-center justify-center text-center text-neutral-400 gap-1">
              <p className="text-xs font-medium text-neutral-700 dark:text-neutral-300">No icons found</p>
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
                    className={`group relative flex flex-col items-center justify-center p-2 rounded-[8px] border text-center transition-all cursor-pointer ${
                      isSelected
                        ? "border-[#3ecf8e] bg-[#3ecf8e]/10 shadow-xs ring-1 ring-[#3ecf8e]"
                        : "border-neutral-200 dark:border-[#2e2e2e] hover:border-neutral-300 dark:hover:border-neutral-500 hover:bg-neutral-50 dark:hover:bg-[#282828] bg-white dark:bg-[#171717]"
                    }`}
                  >
                    {/* Icon container */}
                    <div className="size-11 rounded-[6px] flex items-center justify-center p-1 relative overflow-hidden bg-neutral-100/50 dark:bg-[#202020]">
                      <img
                        src={getIconUrl(icon.id)}
                        alt={icon.name}
                        className="max-size-full max-h-8 max-w-8 object-contain transition-transform group-hover:scale-110 drop-shadow-xs"
                        loading="lazy"
                      />
                    </div>
                    <span className="mt-1 text-[10px] text-neutral-700 dark:text-neutral-300 font-medium truncate w-full text-center group-hover:text-neutral-900 dark:group-hover:text-white">
                      {icon.name}
                    </span>
                    {isSelected && (
                      <span className="absolute top-1 right-1 size-4 bg-[#3ecf8e] text-[#171717] rounded-full flex items-center justify-center shadow-xs">
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
        <div className="flex items-center justify-between px-5 py-3 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50/80 dark:bg-[#171717]/80">
          <div className="text-xs text-neutral-500 dark:text-neutral-400">
            {selectedIconId ? (
              <span className="flex items-center gap-1.5">
                Selected: <strong className="text-neutral-900 dark:text-white">{selectedIconId}</strong>
              </span>
            ) : (
              <span>Click an icon to select it</span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3.5 py-1.5 text-xs font-medium text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200/60 dark:hover:bg-[#282828] rounded-[6px] transition-colors border border-neutral-200 dark:border-[#2e2e2e] cursor-pointer"
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
                className="px-4 py-1.5 text-xs font-semibold text-[#171717] bg-[#3ecf8e] hover:bg-[#24b47e] disabled:opacity-50 disabled:pointer-events-none rounded-[6px] transition-colors shadow-xs cursor-pointer"
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
