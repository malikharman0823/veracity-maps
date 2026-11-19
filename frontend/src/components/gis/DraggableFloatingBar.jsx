import React, { useEffect, useRef, useState } from "react"
import { GripVertical, Lock, Unlock, ChevronLeft, ChevronRight } from "lucide-react"

/**
 * DraggableFloatingBar wraps any floating studio toolbar so users can freely drag it
 * anywhere within the workspace canvas, lock/unlock its position, and collapse/expand it.
 */
export default function DraggableFloatingBar({
  storageKey,
  label = "Tools",
  defaultStyle = { top: 12, left: 12 },
  collapsible = true,
  defaultCollapsed = false,
  className = "",
  children,
}) {
  const barRef = useRef(null)

  const [locked, setLocked] = useState(() => {
    try {
      const saved = localStorage.getItem(`veracity_bar_lock_${storageKey}`)
      return saved === "true"
    } catch {
      return false
    }
  })

  const [collapsed, setCollapsed] = useState(() => {
    try {
      const saved = localStorage.getItem(`veracity_bar_collapsed_${storageKey}`)
      if (saved === "true") return true
      if (saved === "false") return false
      return defaultCollapsed
    } catch {
      return defaultCollapsed
    }
  })

  const [pos, setPos] = useState(() => {
    try {
      const raw = localStorage.getItem(`veracity_bar_pos_${storageKey}`)
      if (raw) {
        const parsed = JSON.parse(raw)
        if (typeof parsed?.x === "number" && typeof parsed?.y === "number") {
          return parsed
        }
      }
    } catch {}
    return null
  })

  useEffect(() => {
    try {
      localStorage.setItem(`veracity_bar_lock_${storageKey}`, String(locked))
    } catch {}
  }, [locked, storageKey])

  useEffect(() => {
    try {
      localStorage.setItem(`veracity_bar_collapsed_${storageKey}`, String(collapsed))
    } catch {}
  }, [collapsed, storageKey])

  useEffect(() => {
    if (!pos) return
    try {
      localStorage.setItem(`veracity_bar_pos_${storageKey}`, JSON.stringify(pos))
    } catch {}
  }, [pos, storageKey])

  const handlePointerDown = (e) => {
    if (locked) return
    e.preventDefault()
    e.stopPropagation()

    const barEl = barRef.current
    const parentEl = barEl?.offsetParent
    if (!barEl || !parentEl) return

    const parentRect = parentEl.getBoundingClientRect()
    const barRect = barEl.getBoundingClientRect()

    const startClientX = e.clientX
    const startClientY = e.clientY
    const startLeft = barRect.left - parentRect.left
    const startTop = barRect.top - parentRect.top

    const onMove = (mv) => {
      const dx = mv.clientX - startClientX
      const dy = mv.clientY - startClientY
      const maxLeft = Math.max(8, parentRect.width - barRect.width - 8)
      const maxTop = Math.max(8, parentRect.height - barRect.height - 8)
      const nextX = Math.max(8, Math.min(maxLeft, startLeft + dx))
      const nextY = Math.max(8, Math.min(maxTop, startTop + dy))
      setPos({ x: Math.round(nextX), y: Math.round(nextY) })
    }

    const onUp = () => {
      window.removeEventListener("pointermove", onMove)
      window.removeEventListener("pointerup", onUp)
    }

    window.addEventListener("pointermove", onMove)
    window.addEventListener("pointerup", onUp)
  }

  const positionStyle = pos
    ? { top: pos.y, left: pos.x }
    : defaultStyle

  return (
    <div
      ref={barRef}
      style={positionStyle}
      className={`absolute z-30 pointer-events-auto flex items-center gap-1 ${className}`}
    >
      {/* Drag Grip + Lock + Collapse Controls */}
      <div className="flex items-center rounded-[7px] bg-white/95 dark:bg-[#222222]/95 backdrop-blur-md border border-neutral-200 dark:border-[#343434] shadow-lg p-1 gap-0.5">
        <button
          type="button"
          onPointerDown={handlePointerDown}
          onDoubleClick={() => {
            if (!locked) setPos(null)
          }}
          disabled={locked}
          title={
            locked
              ? "Bar is locked in place. Click the lock icon to unlock and move."
              : "Drag to move this bar anywhere (double-click to reset position)"
          }
          className={`p-1 rounded-[4px] transition-colors ${
            locked
              ? "text-neutral-400/50 cursor-not-allowed"
              : "text-neutral-500 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-white cursor-grab active:cursor-grabbing"
          }`}
          aria-label={`Drag ${label} toolbar`}
        >
          <GripVertical className="size-3.5" />
        </button>

        <button
          type="button"
          onClick={() => setLocked((prev) => !prev)}
          title={
            locked
              ? "Locked to this position — click to unlock and move freely"
              : "Unlocked — click to lock this bar in its current position"
          }
          className={`p-1 rounded-[4px] transition-colors cursor-pointer ${
            locked
              ? "bg-[#3ecf8e]/20 text-[#24b47e] dark:text-[#3ecf8e]"
              : "text-neutral-400 hover:text-neutral-800 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#303030]"
          }`}
          aria-label={locked ? `Unlock ${label} toolbar position` : `Lock ${label} toolbar position`}
        >
          {locked ? <Lock className="size-3.5" /> : <Unlock className="size-3.5" />}
        </button>

        {collapsible && (
          <button
            type="button"
            onClick={() => setCollapsed((prev) => !prev)}
            title={collapsed ? `Expand ${label} menu` : `Collapse ${label} menu`}
            className={`inline-flex items-center gap-1 rounded-[4px] transition-colors cursor-pointer ${
              collapsed
                ? "px-1.5 py-1 text-[11px] font-semibold text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#303030]"
                : "p-1 text-neutral-400 hover:text-neutral-800 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-[#303030]"
            }`}
            aria-expanded={!collapsed}
            aria-label={collapsed ? `Expand ${label} menu` : `Collapse ${label} menu`}
          >
            {collapsed ? (
              <>
                <span className="text-[#3ecf8e]">{label}</span>
                <ChevronRight className="size-3.5 text-neutral-400" />
              </>
            ) : (
              <ChevronLeft className="size-3.5" />
            )}
          </button>
        )}

        {!collapsed && (
          <>
            <div className="w-[1px] h-4 bg-neutral-200 dark:bg-[#363636] mx-0.5" />
            {children}
          </>
        )}
      </div>
    </div>
  )
}
