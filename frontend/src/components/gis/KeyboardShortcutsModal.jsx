import React, { useEffect, useState } from "react"
import { Keyboard, RotateCcw, X, AlertTriangle, Check } from "lucide-react"
import {
  eventToShortcutCombo,
  formatShortcutForOS,
  validateShortcutAssignment,
} from "../../lib/shortcutRegistry"

export default function KeyboardShortcutsModal({
  open,
  shortcuts = [],
  onClose,
  onAssignShortcut,
  onResetDefaults,
}) {
  const [capturingId, setCapturingId] = useState(null)
  const [captureError, setCaptureError] = useState("")

  useEffect(() => {
    if (!open || !capturingId) return undefined
    const handleKeyDown = (e) => {
      e.preventDefault()
      e.stopPropagation()
      if (e.key === "Escape") {
        setCapturingId(null)
        setCaptureError("")
        return
      }
      const combo = eventToShortcutCombo(e)
      if (!combo) return
      const res = validateShortcutAssignment(shortcuts, capturingId, combo)
      if (!res.valid) {
        setCaptureError(res.error)
        return
      }
      onAssignShortcut?.(capturingId, res.shortcut)
      setCapturingId(null)
      setCaptureError("")
    }
    window.addEventListener("keydown", handleKeyDown, true)
    return () => window.removeEventListener("keydown", handleKeyDown, true)
  }, [open, capturingId, shortcuts, onAssignShortcut])

  if (!open) return null

  const categories = Array.from(new Set(shortcuts.map((s) => s.category || "General")))

  return (
    <div
      className="fixed inset-0 z-[1100] bg-black/70 backdrop-blur-xs flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="keyboard-shortcuts-title"
    >
      <div className="w-full max-w-2xl max-h-[85vh] rounded-[14px] bg-white dark:bg-[#191b1f] border border-neutral-200 dark:border-[#2e323a] shadow-2xl flex flex-col overflow-hidden">
        <div className="px-5 py-3.5 border-b border-neutral-200 dark:border-[#2a2d34] flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-[7px] bg-[#3ecf8e]/15 text-[#3ecf8e] flex items-center justify-center">
              <Keyboard className="w-4 h-4" />
            </div>
            <div>
              <h2 id="keyboard-shortcuts-title" className="text-sm font-bold text-neutral-900 dark:text-white">
                Keyboard Shortcuts
              </h2>
              <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
                Click any shortcut badge to customize it. Tooltips update automatically.
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => {
                setCapturingId(null)
                setCaptureError("")
                onResetDefaults?.()
              }}
              className="inline-flex items-center gap-1 px-2.5 py-1 rounded-[6px] border border-neutral-200 dark:border-[#2e323a] text-[11px] font-semibold text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-[#2a2d34] cursor-pointer"
            >
              <RotateCcw className="w-3 h-3" />
              Reset Defaults
            </button>
            <button
              type="button"
              onClick={onClose}
              className="p-1.5 rounded-[6px] text-neutral-400 hover:text-neutral-900 dark:hover:text-white cursor-pointer"
              aria-label="Close keyboard shortcuts"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-5 space-y-5">
          {captureError && (
            <div className="flex items-center gap-2 p-2.5 rounded-[7px] bg-amber-500/10 border border-amber-500/30 text-xs text-amber-600 dark:text-amber-300">
              <AlertTriangle className="w-4 h-4 shrink-0" />
              <span>{captureError}</span>
            </div>
          )}

          {categories.map((cat) => (
            <div key={cat}>
              <h3 className="text-[11px] font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-400 mb-2">
                {cat}
              </h3>
              <div className="divide-y divide-neutral-100 dark:divide-[#262930] rounded-[9px] border border-neutral-200 dark:border-[#2a2d34] overflow-hidden">
                {shortcuts
                  .filter((s) => (s.category || "General") === cat)
                  .map((item) => {
                    const isCapturing = capturingId === item.tool_id
                    return (
                      <div
                        key={item.tool_id}
                        className="flex items-center justify-between gap-4 px-3.5 py-2.5 bg-white dark:bg-[#1e2026] hover:bg-neutral-50 dark:hover:bg-[#24272f]"
                      >
                        <div className="min-w-0">
                          <div className="text-xs font-semibold text-neutral-900 dark:text-white">
                            {item.label}
                          </div>
                          <div className="text-[11px] text-neutral-500 dark:text-neutral-400 truncate">
                            {item.description}
                          </div>
                        </div>
                        <button
                          type="button"
                          onClick={() => {
                            setCaptureError("")
                            setCapturingId(isCapturing ? null : item.tool_id)
                          }}
                          className={`shrink-0 px-2.5 py-1 rounded-[5px] font-mono text-[11px] font-semibold border transition-colors cursor-pointer ${
                            isCapturing
                              ? "border-[#3ecf8e] bg-[#3ecf8e]/20 text-[#3ecf8e] animate-pulse"
                              : "border-neutral-300 dark:border-[#363b45] bg-neutral-100 dark:bg-[#14161a] text-neutral-800 dark:text-neutral-200 hover:border-[#3ecf8e]"
                          }`}
                          title="Click and press a new key combination"
                        >
                          {isCapturing ? "Press keys… (Esc cancels)" : formatShortcutForOS(item.shortcut)}
                        </button>
                      </div>
                    )
                  })}
              </div>
            </div>
          ))}
        </div>

        <div className="px-5 py-3 border-t border-neutral-200 dark:border-[#2a2d34] bg-neutral-50 dark:bg-[#15171b] flex items-center justify-between text-[11px] text-neutral-500 dark:text-neutral-400">
          <span>Shortcuts automatically pause while typing inside text fields or forms.</span>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex items-center gap-1 px-3 py-1.5 rounded-[6px] bg-[#3ecf8e] text-[#171717] font-semibold cursor-pointer"
          >
            <Check className="w-3.5 h-3.5" />
            Done
          </button>
        </div>
      </div>
    </div>
  )
}
