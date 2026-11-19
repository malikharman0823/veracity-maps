import React, { useState, useRef } from "react"

// Visual SVG mini-previews rendered inside the hover popup menu
function ToolPreviewGraphic({ previewKey }) {
  switch (previewKey) {
    case "map":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <path d="M0 45 Q45 30 90 42 T180 35" stroke="#2c353d" strokeWidth="2" fill="none" />
          <path d="M30 0 L50 72 M125 0 L110 72" stroke="#232a30" strokeWidth="1.5" />
          <polygon points="55,20 115,16 125,52 62,56" fill="#3ecf8e" fillOpacity="0.22" stroke="#3ecf8e" strokeWidth="1.5" />
          <circle cx="90" cy="34" r="4" fill="#3ecf8e" />
        </svg>
      )
    case "database":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <rect x="14" y="10" width="152" height="13" rx="2" fill="#263138" />
          <rect x="14" y="27" width="152" height="10" rx="1.5" fill="#1e2429" />
          <rect x="14" y="40" width="152" height="10" rx="1.5" fill="#3ecf8e" fillOpacity="0.25" stroke="#3ecf8e" strokeWidth="0.8" />
          <rect x="14" y="53" width="152" height="10" rx="1.5" fill="#1e2429" />
          <line x1="60" y1="10" x2="60" y2="63" stroke="#2c353d" strokeWidth="1" />
          <line x1="115" y1="10" x2="115" y2="63" stroke="#2c353d" strokeWidth="1" />
        </svg>
      )
    case "select":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <polygon points="52,18 118,14 128,50 60,55" fill="#3ecf8e" fillOpacity="0.28" stroke="#3ecf8e" strokeWidth="1.8" strokeDasharray="3 2" />
          <path d="M96 36 L96 56 L102 51 L107 60 L111 58 L106 49 L113 48 Z" fill="#ffffff" stroke="#111" strokeWidth="1" />
        </svg>
      )
    case "point":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <circle cx="52" cy="40" r="4" fill="#3ecf8e" opacity="0.6" />
          <circle cx="130" cy="24" r="4" fill="#3ecf8e" opacity="0.6" />
          <circle cx="90" cy="36" r="14" fill="#3ecf8e" fillOpacity="0.16" />
          <path d="M90 16 C82 16 77 22 77 29 C77 38 90 50 90 50 C90 50 103 38 103 29 C103 22 98 16 90 16 Z" fill="#3ecf8e" />
          <circle cx="90" cy="28" r="4" fill="#161a1d" />
        </svg>
      )
    case "line":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <polyline points="26,52 68,22 112,46 154,18" fill="none" stroke="#3ecf8e" strokeWidth="2.5" />
          <circle cx="26" cy="52" r="4" fill="#fff" stroke="#3ecf8e" strokeWidth="1.8" />
          <circle cx="68" cy="22" r="4" fill="#fff" stroke="#3ecf8e" strokeWidth="1.8" />
          <circle cx="112" cy="46" r="4" fill="#fff" stroke="#3ecf8e" strokeWidth="1.8" />
          <circle cx="154" cy="18" r="4" fill="#3ecf8e" stroke="#fff" strokeWidth="1.5" />
        </svg>
      )
    case "polygon":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <polygon points="44,16 132,14 146,52 60,58 34,38" fill="#3ecf8e" fillOpacity="0.28" stroke="#3ecf8e" strokeWidth="2" />
          <rect x="41" y="13" width="6" height="6" fill="#fff" stroke="#3ecf8e" strokeWidth="1.2" />
          <rect x="129" y="11" width="6" height="6" fill="#fff" stroke="#3ecf8e" strokeWidth="1.2" />
          <rect x="143" y="49" width="6" height="6" fill="#fff" stroke="#3ecf8e" strokeWidth="1.2" />
          <rect x="57" y="55" width="6" height="6" fill="#fff" stroke="#3ecf8e" strokeWidth="1.2" />
          <rect x="31" y="35" width="6" height="6" fill="#fff" stroke="#3ecf8e" strokeWidth="1.2" />
        </svg>
      )
    case "move":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <rect x="26" y="18" width="46" height="34" fill="none" stroke="#4b5563" strokeWidth="1.5" strokeDasharray="3 3" />
          <rect x="96" y="18" width="46" height="34" fill="#3ecf8e" fillOpacity="0.25" stroke="#3ecf8e" strokeWidth="1.8" />
          <path d="M74 35 L92 35 M87 30 L93 35 L87 40" stroke="#fff" strokeWidth="1.8" fill="none" />
        </svg>
      )
    case "vertex":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <polygon points="42,54 88,16 138,54" fill="#3ecf8e" fillOpacity="0.22" stroke="#3ecf8e" strokeWidth="1.8" />
          <circle cx="42" cy="54" r="4" fill="#fff" stroke="#3ecf8e" strokeWidth="1.5" />
          <circle cx="88" cy="16" r="5.5" fill="#f59e0b" stroke="#fff" strokeWidth="1.5" />
          <circle cx="138" cy="54" r="4" fill="#fff" stroke="#3ecf8e" strokeWidth="1.5" />
        </svg>
      )
    case "snap":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <line x1="25" y1="54" x2="88" y2="36" stroke="#64748b" strokeWidth="2" />
          <line x1="92" y1="36" x2="155" y2="20" stroke="#3ecf8e" strokeWidth="2" />
          <circle cx="90" cy="36" r="14" fill="#38bdf8" fillOpacity="0.18" stroke="#38bdf8" strokeWidth="1" strokeDasharray="3 2" />
          <circle cx="90" cy="36" r="4.5" fill="#38bdf8" />
        </svg>
      )
    case "undo":
    case "redo":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <path
            d={previewKey === "undo" ? "M125 46 C125 22 72 22 56 36 M64 28 L54 37 L66 42" : "M55 46 C55 22 108 22 124 36 M116 28 L126 37 L114 42"}
            stroke="#3ecf8e"
            strokeWidth="2.5"
            fill="none"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )
    case "attributes":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <rect x="14" y="12" width="60" height="48" rx="3" fill="#20262b" stroke="#2f3840" />
          <polygon points="28,26 58,22 62,48 32,50" fill="#3ecf8e" fillOpacity="0.35" stroke="#3ecf8e" strokeWidth="1.5" />
          <rect x="84" y="12" width="82" height="12" rx="2" fill="#2b333a" />
          <rect x="84" y="28" width="82" height="11" rx="2" fill="#3ecf8e" fillOpacity="0.28" stroke="#3ecf8e" strokeWidth="1" />
          <rect x="84" y="43" width="82" height="11" rx="2" fill="#20262b" />
        </svg>
      )
    case "symbology":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <rect x="20" y="16" width="38" height="40" rx="4" fill="#3ecf8e" fillOpacity="0.7" stroke="#fff" strokeWidth="1.2" />
          <rect x="70" y="16" width="38" height="40" rx="4" fill="#3b82f6" fillOpacity="0.7" stroke="#fff" strokeWidth="1.2" />
          <rect x="120" y="16" width="38" height="40" rx="4" fill="#f59e0b" fillOpacity="0.7" stroke="#fff" strokeWidth="1.2" />
        </svg>
      )
    case "crs":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <circle cx="90" cy="36" r="24" fill="#1e293b" stroke="#3ecf8e" strokeWidth="1.6" />
          <ellipse cx="90" cy="36" rx="11" ry="24" fill="none" stroke="#3ecf8e" strokeWidth="1" opacity="0.7" />
          <line x1="66" y1="36" x2="114" y2="36" stroke="#3ecf8e" strokeWidth="1" opacity="0.7" />
          <line x1="70" y1="24" x2="110" y2="24" stroke="#3ecf8e" strokeWidth="0.8" opacity="0.5" />
          <line x1="70" y1="48" x2="110" y2="48" stroke="#3ecf8e" strokeWidth="0.8" opacity="0.5" />
        </svg>
      )
    case "datasets":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <rect x="20" y="14" width="40" height="44" rx="4" fill="#232b32" stroke="#3ecf8e" strokeWidth="1.2" />
          <rect x="70" y="14" width="40" height="44" rx="4" fill="#232b32" stroke="#38bdf8" strokeWidth="1.2" />
          <rect x="120" y="14" width="40" height="44" rx="4" fill="#232b32" stroke="#a78bfa" strokeWidth="1.2" />
          <text x="40" y="40" textAnchor="middle" fill="#3ecf8e" fontSize="9" fontWeight="bold">TIF</text>
          <text x="90" y="40" textAnchor="middle" fill="#38bdf8" fontSize="9" fontWeight="bold">SHP</text>
          <text x="140" y="40" textAnchor="middle" fill="#a78bfa" fontSize="9" fontWeight="bold">GPKG</text>
        </svg>
      )
    case "processing":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <circle cx="72" cy="36" r="22" fill="#3ecf8e" fillOpacity="0.2" stroke="#3ecf8e" strokeWidth="1.5" strokeDasharray="3 2" />
          <circle cx="72" cy="36" r="5" fill="#3ecf8e" />
          <circle cx="112" cy="36" r="18" fill="#38bdf8" fillOpacity="0.2" stroke="#38bdf8" strokeWidth="1.5" />
        </svg>
      )
    case "place_icon":
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <rect x="34" y="14" width="112" height="44" fill="#22282d" stroke="#38424b" strokeWidth="1.2" />
          <line x1="88" y1="14" x2="88" y2="58" stroke="#38424b" strokeWidth="1" />
          <circle cx="62" cy="36" r="7" fill="#3ecf8e" />
          <circle cx="118" cy="36" r="7" fill="#f59e0b" />
        </svg>
      )
    default:
      return (
        <svg viewBox="0 0 180 72" className="w-full h-full">
          <rect width="180" height="72" fill="#161a1d" />
          <rect x="28" y="16" width="124" height="40" rx="6" fill="#222a30" stroke="#3ecf8e" strokeWidth="1.4" />
          <circle cx="52" cy="36" r="8" fill="#3ecf8e" fillOpacity="0.3" stroke="#3ecf8e" strokeWidth="1.5" />
          <rect x="70" y="29" width="66" height="5" rx="2" fill="#9ca3af" />
          <rect x="70" y="39" width="44" height="4" rx="2" fill="#4b5563" />
        </svg>
      )
  }
}

export default function ToolHoverPreview({
  title,
  description,
  previewKey = "default",
  shortcut,
  side = "right", // "right" | "bottom" | "top" | "left"
  children,
}) {
  const [open, setOpen] = useState(false)
  const timerRef = useRef(null)

  const handleEnter = () => {
    clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setOpen(true), 220)
  }

  const handleLeave = () => {
    clearTimeout(timerRef.current)
    setOpen(false)
  }

  const posClass =
    side === "right"
      ? "left-full top-1/2 -translate-y-1/2 ml-2.5"
      : side === "left"
      ? "right-full top-1/2 -translate-y-1/2 mr-2.5"
      : side === "top"
      ? "bottom-full left-1/2 -translate-x-1/2 mb-2.5"
      : "top-full left-1/2 -translate-x-1/2 mt-2.5"

  return (
    <div
      className="relative inline-flex"
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
    >
      {children}
      {open && (
        <div
          role="tooltip"
          className={`pointer-events-none absolute ${posClass} z-[120] w-60 rounded-lg bg-[#232323] text-white border border-[#3a3a3a] shadow-2xl p-2.5 animate-in fade-in zoom-in-95 duration-100`}
        >
          <div className="flex items-center justify-between gap-2 mb-1.5">
            <span className="text-[11px] font-bold text-white tracking-wide">{title}</span>
            {shortcut && (
              <kbd className="px-1.5 py-0.5 rounded bg-[#161616] border border-[#3a3a3a] text-[9px] font-mono text-[#3ecf8e]">
                {shortcut}
              </kbd>
            )}
          </div>
          <div className="w-full h-[72px] rounded border border-[#323232] overflow-hidden mb-2 bg-[#161a1d]">
            <ToolPreviewGraphic previewKey={previewKey} />
          </div>
          <p className="text-[11px] leading-snug text-neutral-300">
            {description}
          </p>
        </div>
      )}
    </div>
  )
}
