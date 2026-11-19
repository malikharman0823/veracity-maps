import React from "react"

const ALGORITHM_GUIDE = {
  buffer: {
    simpleText:
      "Draws a protective zone or ring around points, lines, or shapes at the exact distance you choose (for example, 500 meters around every hospital).",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <circle cx="52" cy="38" r="5" fill="#3ecf8e" />
        <path d="M92 38 L120 38 M115 33 L121 38 L115 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <circle cx="164" cy="38" r="22" fill="#3ecf8e" fillOpacity="0.22" stroke="#3ecf8e" strokeWidth="1.6" strokeDasharray="3 2" />
        <circle cx="164" cy="38" r="5" fill="#3ecf8e" />
      </svg>
    ),
  },
  centroid: {
    simpleText:
      "Finds the exact center point of each polygon or line and turns it into a clean Point layer while keeping all its table info.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <polygon points="28,18 76,16 82,56 32,58" fill="#3ecf8e" fillOpacity="0.2" stroke="#3ecf8e" strokeWidth="1.5" />
        <path d="M96 38 L124 38 M119 33 L125 38 L119 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <polygon points="140,18 188,16 194,56 144,58" fill="none" stroke="#4b5563" strokeWidth="1.2" strokeDasharray="3 2" />
        <circle cx="166" cy="37" r="5" fill="#3ecf8e" stroke="#fff" strokeWidth="1.2" />
      </svg>
    ),
  },
  calculate_geometry: {
    simpleText:
      "Automatically measures the area, perimeter, length, and center coordinates of every shape and saves them as columns.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="24" y="18" width="54" height="38" fill="#3ecf8e" fillOpacity="0.2" stroke="#3ecf8e" strokeWidth="1.5" />
        <path d="M94 38 L120 38 M115 33 L121 38 L115 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <rect x="132" y="16" width="70" height="44" rx="4" fill="#20262b" stroke="#334155" />
        <text x="140" y="33" fill="#3ecf8e" fontSize="9" fontFamily="monospace">area: 1,420</text>
        <text x="140" y="48" fill="#38bdf8" fontSize="9" fontFamily="monospace">len:  152m</text>
      </svg>
    ),
  },
  intersection: {
    simpleText:
      "Keeps ONLY the overlapping area where Layer A and Layer B cross each other, and combines their attributes.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <circle cx="46" cy="38" r="20" fill="#38bdf8" fillOpacity="0.22" stroke="#38bdf8" strokeWidth="1.4" />
        <circle cx="68" cy="38" r="20" fill="#f59e0b" fillOpacity="0.22" stroke="#f59e0b" strokeWidth="1.4" />
        <path d="M100 38 L124 38 M119 33 L125 38 L119 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <circle cx="154" cy="38" r="20" fill="none" stroke="#4b5563" strokeWidth="1" strokeDasharray="2 2" />
        <circle cx="176" cy="38" r="20" fill="none" stroke="#4b5563" strokeWidth="1" strokeDasharray="2 2" />
        <path d="M165,21 A20,20 0 0,1 165,55 A20,20 0 0,1 165,21 Z" fill="#3ecf8e" fillOpacity="0.7" stroke="#fff" strokeWidth="1.2" />
      </svg>
    ),
  },
  union: {
    simpleText:
      "Combines all shapes from Layer A and Layer B together into one layer while keeping all boundaries and attributes.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <circle cx="46" cy="38" r="19" fill="#38bdf8" fillOpacity="0.25" stroke="#38bdf8" strokeWidth="1.4" />
        <circle cx="68" cy="38" r="19" fill="#f59e0b" fillOpacity="0.25" stroke="#f59e0b" strokeWidth="1.4" />
        <path d="M100 38 L124 38 M119 33 L125 38 L119 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <circle cx="154" cy="38" r="19" fill="#3ecf8e" fillOpacity="0.35" stroke="#3ecf8e" strokeWidth="1.5" />
        <circle cx="176" cy="38" r="19" fill="#3ecf8e" fillOpacity="0.35" stroke="#3ecf8e" strokeWidth="1.5" />
      </svg>
    ),
  },
  difference: {
    simpleText:
      "Cuts out and removes the parts of Layer A that are covered by Layer B — just like using a cookie cutter.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="24" y="18" width="38" height="38" fill="#3ecf8e" fillOpacity="0.28" stroke="#3ecf8e" strokeWidth="1.4" />
        <circle cx="62" cy="37" r="18" fill="#ef4444" fillOpacity="0.28" stroke="#ef4444" strokeWidth="1.4" />
        <path d="M94 38 L120 38 M115 33 L121 38 L115 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <path d="M138,18 H176 V22 A18,18 0 0,0 176,52 V56 H138 Z" fill="#3ecf8e" fillOpacity="0.55" stroke="#3ecf8e" strokeWidth="1.5" />
      </svg>
    ),
  },
  clip: {
    simpleText:
      "Trims Layer A so that only the features inside the boundary of Layer B are kept.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <line x1="22" y1="54" x2="82" y2="20" stroke="#3ecf8e" strokeWidth="2" />
        <rect x="34" y="18" width="36" height="38" fill="none" stroke="#f59e0b" strokeWidth="1.5" strokeDasharray="3 2" />
        <path d="M96 38 L122 38 M117 33 L123 38 L117 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <rect x="142" y="18" width="36" height="38" fill="none" stroke="#4b5563" strokeWidth="1" strokeDasharray="3 2" />
        <line x1="142" y1="43" x2="178" y2="22" stroke="#3ecf8e" strokeWidth="2.5" />
      </svg>
    ),
  },
  dissolve: {
    simpleText:
      "Merges neighboring shapes into one combined shape (either all together or grouped by a shared column like City or Zone).",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="24" y="20" width="26" height="36" fill="#3ecf8e" fillOpacity="0.25" stroke="#3ecf8e" strokeWidth="1.4" />
        <rect x="50" y="20" width="26" height="36" fill="#3ecf8e" fillOpacity="0.25" stroke="#3ecf8e" strokeWidth="1.4" />
        <path d="M94 38 L120 38 M115 33 L121 38 L115 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <rect x="138" y="20" width="52" height="36" fill="#3ecf8e" fillOpacity="0.4" stroke="#3ecf8e" strokeWidth="1.8" />
      </svg>
    ),
  },
  spatial_join: {
    simpleText:
      "Copies or counts data from one layer into another based on where they overlap (for example, counting hospitals inside each ward).",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="24" y="16" width="56" height="44" fill="#38bdf8" fillOpacity="0.18" stroke="#38bdf8" strokeWidth="1.5" />
        <circle cx="42" cy="32" r="3.5" fill="#f59e0b" />
        <circle cx="62" cy="44" r="3.5" fill="#f59e0b" />
        <path d="M96 38 L122 38 M117 33 L123 38 L117 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <rect x="136" y="16" width="60" height="44" fill="#3ecf8e" fillOpacity="0.28" stroke="#3ecf8e" strokeWidth="1.5" />
        <text x="166" y="42" textAnchor="middle" fill="#fff" fontSize="11" fontWeight="bold">count: 2</text>
      </svg>
    ),
  },
  topology: {
    simpleText:
      "Scans your layer for geometry mistakes like overlapping polygons, gaps, or self-intersecting lines and lets you fix them.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <polygon points="30,52 72,20 72,52 30,20" fill="#f59e0b" fillOpacity="0.25" stroke="#f59e0b" strokeWidth="1.5" />
        <circle cx="51" cy="36" r="4" fill="#ef4444" />
        <path d="M94 38 L120 38 M115 33 L121 38 L115 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <polygon points="140,20 186,20 186,54 140,54" fill="#3ecf8e" fillOpacity="0.28" stroke="#3ecf8e" strokeWidth="1.6" />
      </svg>
    ),
  },
  raster_clip: {
    simpleText:
      "Crops a raster image or GeoTIFF to match the exact shape of a polygon boundary layer.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="24" y="16" width="54" height="44" fill="#334155" stroke="#64748b" />
        <polygon points="36,24 68,28 60,52 30,48" fill="none" stroke="#3ecf8e" strokeWidth="1.5" strokeDasharray="3 2" />
        <path d="M96 38 L122 38 M117 33 L123 38 L117 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <polygon points="146,24 178,28 170,52 140,48" fill="#3ecf8e" fillOpacity="0.35" stroke="#3ecf8e" strokeWidth="1.5" />
      </svg>
    ),
  },
  raster_reproject: {
    simpleText:
      "Converts a raster image from one Coordinate Reference System (CRS) to another, such as EPSG:4326 to EPSG:3857.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="26" y="18" width="46" height="40" fill="#38bdf8" fillOpacity="0.2" stroke="#38bdf8" strokeWidth="1.4" />
        <path d="M94 38 L120 38 M115 33 L121 38 L115 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <polygon points="142,16 192,22 184,58 136,52" fill="#3ecf8e" fillOpacity="0.25" stroke="#3ecf8e" strokeWidth="1.5" />
      </svg>
    ),
  },
  raster_resample: {
    simpleText:
      "Resizes a raster's pixel resolution (make it sharper or lighter) using nearest, bilinear, or cubic interpolation.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="28" y="20" width="20" height="18" fill="#38bdf8" fillOpacity="0.35" stroke="#38bdf8" />
        <rect x="48" y="20" width="20" height="18" fill="#38bdf8" fillOpacity="0.15" stroke="#38bdf8" />
        <rect x="28" y="38" width="20" height="18" fill="#38bdf8" fillOpacity="0.15" stroke="#38bdf8" />
        <rect x="48" y="38" width="20" height="18" fill="#38bdf8" fillOpacity="0.35" stroke="#38bdf8" />
        <path d="M94 38 L120 38 M115 33 L121 38 L115 43" stroke="#9ca3af" strokeWidth="1.6" fill="none" />
        <rect x="138" y="20" width="44" height="36" fill="#3ecf8e" fillOpacity="0.28" stroke="#3ecf8e" strokeWidth="1.4" />
      </svg>
    ),
  },
  raster_statistics: {
    simpleText:
      "Calculates the minimum, maximum, and average pixel values across a raster dataset.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="36" y="42" width="14" height="18" fill="#38bdf8" />
        <rect x="56" y="26" width="14" height="34" fill="#3ecf8e" />
        <rect x="76" y="18" width="14" height="42" fill="#f59e0b" />
        <text x="148" y="32" textAnchor="middle" fill="#9ca3af" fontSize="9">MIN / MAX / MEAN</text>
        <text x="148" y="48" textAnchor="middle" fill="#3ecf8e" fontSize="11" fontWeight="bold">0 – 255</text>
      </svg>
    ),
  },
  raster_calculator: {
    simpleText:
      "Runs safe math formulas on raster bands (for example, A * 1.5 or (A - B) / (A + B)) to create a new raster.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <rect x="26" y="18" width="168" height="40" rx="6" fill="#20262b" stroke="#3ecf8e" strokeWidth="1.2" />
        <text x="110" y="42" textAnchor="middle" fill="#3ecf8e" fontSize="13" fontFamily="monospace" fontWeight="bold">
          (A - B) / (A + B)
        </text>
      </svg>
    ),
  },
  raster_hillshade: {
    simpleText:
      "Creates a 3D shaded relief image from elevation or terrain intensity using a simulated sun angle.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <circle cx="42" cy="22" r="7" fill="#fbbf24" />
        <path d="M24 58 L70 28 L108 48 L152 22 L196 58 Z" fill="#475569" stroke="#94a3b8" strokeWidth="1.2" />
        <path d="M70 28 L108 48 L70 58 Z" fill="#1e293b" opacity="0.7" />
      </svg>
    ),
  },
  raster_contours: {
    simpleText:
      "Draws elevation contour lines at regular intervals from a raster dataset so you can view terrain heights as vectors.",
    svg: (
      <svg viewBox="0 0 220 76" className="w-full h-full">
        <rect width="220" height="76" fill="#161a1d" />
        <ellipse cx="110" cy="38" rx="72" ry="26" fill="none" stroke="#3ecf8e" strokeWidth="1.2" opacity="0.5" />
        <ellipse cx="110" cy="38" rx="48" ry="17" fill="none" stroke="#3ecf8e" strokeWidth="1.4" opacity="0.75" />
        <ellipse cx="110" cy="38" rx="22" ry="8" fill="none" stroke="#3ecf8e" strokeWidth="1.6" />
      </svg>
    ),
  },
}

export function getAlgorithmGuide(algId) {
  return ALGORITHM_GUIDE[algId] || null
}

export default function AlgorithmPreviewCard({ algorithm }) {
  if (!algorithm) return null
  const guide = ALGORITHM_GUIDE[algorithm.algorithm_id]

  return (
    <div className="rounded-lg bg-[#15181b] border border-[#2c333a] p-3 flex flex-col sm:flex-row items-center gap-3">
      <div className="w-full sm:w-52 h-[76px] rounded border border-[#2a2f35] overflow-hidden shrink-0 bg-[#161a1d]">
        {guide?.svg || (
          <svg viewBox="0 0 220 76" className="w-full h-full">
            <rect width="220" height="76" fill="#161a1d" />
            <circle cx="110" cy="38" r="18" fill="#3ecf8e" fillOpacity="0.25" stroke="#3ecf8e" strokeWidth="1.5" />
          </svg>
        )}
      </div>
      <div className="flex-1 min-w-0">
        <div className="text-[11px] font-bold uppercase tracking-wider text-[#3ecf8e] mb-1">
          Visual Preview &amp; How It Works
        </div>
        <p className="text-xs text-neutral-300 leading-relaxed">
          {guide?.simpleText || algorithm.description}
        </p>
      </div>
    </div>
  )
}
