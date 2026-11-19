/**
 * Centralized Keyboard Shortcut Registry for Veracity Maps Studio Sidebar & Tools.
 * Single source of truth used by both keyboard event listeners and hover tooltips.
 */

export const SHORTCUT_STORAGE_KEY = "veracity_maps_custom_shortcuts_v1"

export const DEFAULT_SHORTCUTS = [
  {
    tool_id: "view_map",
    label: "Map Canvas View",
    description: "Switch to the interactive map canvas.",
    shortcut: "1",
    category: "Views",
    enabled: true,
  },
  {
    tool_id: "view_records",
    label: "Workspace Records",
    description: "Open the centralized Workspace Records data table.",
    shortcut: "2",
    category: "Views",
    enabled: true,
  },
  {
    tool_id: "select",
    label: "Select Tool",
    description: "Select points, lines, and polygons on the map.",
    shortcut: "V",
    category: "Map Tools",
    enabled: true,
  },
  {
    tool_id: "identify",
    label: "Identify Tool",
    description: "Click any map feature to inspect its source layer and attributes.",
    shortcut: "I",
    category: "Map Tools",
    enabled: true,
  },
  {
    tool_id: "point",
    label: "Draw Point",
    description: "Place a new point feature on the active vector layer.",
    shortcut: "P",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "line",
    label: "Draw Line",
    description: "Draw roads, paths, or boundaries on the active vector layer.",
    shortcut: "L",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "polygon",
    label: "Draw Polygon",
    description: "Outline areas, parcels, buildings, or zones.",
    shortcut: "G",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "move",
    label: "Move Feature",
    description: "Drag a geometric feature to reposition it on the map.",
    shortcut: "M",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "vertex",
    label: "Edit Vertices",
    description: "Drag vertices to reshape or resize geometries (hold Shift to scale proportionally).",
    shortcut: "E",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "snap",
    label: "Toggle Snap",
    description: "Magnetize cursor to nearby vertices and edges.",
    shortcut: "S",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "undo",
    label: "Undo",
    description: "Undo the last geometry edit.",
    shortcut: "Ctrl+Z",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "redo",
    label: "Redo",
    description: "Redo the previously undone geometry edit.",
    shortcut: "Ctrl+Shift+Z",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "delete_selected",
    label: "Delete Selected Feature",
    description: "Remove currently selected vector feature(s).",
    shortcut: "Delete",
    category: "Drawing & Editing",
    enabled: true,
  },
  {
    tool_id: "cancel_action",
    label: "Cancel Current Tool / Action",
    description: "Close open dialogs, cancel active drawing, or return to Select.",
    shortcut: "Escape",
    category: "General",
    enabled: true,
  },
  {
    tool_id: "attribute_table",
    label: "Open Attribute Table",
    description: "Inspect and edit attributes for the active vector layer.",
    shortcut: "F6",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "layer_manager",
    label: "Open Layer Manager",
    description: "Toggle or focus the Layers sidebar panel.",
    shortcut: "Ctrl+Shift+L",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "base_layer",
    label: "Base Layer / Map View",
    description: "Open the Base Layer & Map View selector.",
    shortcut: "Ctrl+Shift+B",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "zoom_full",
    label: "Zoom to Full Extent",
    description: "Fit the map camera to all visible layers.",
    shortcut: "Ctrl+Shift+F",
    category: "Navigation",
    enabled: true,
  },
  {
    tool_id: "symbology",
    label: "Symbology & Styling",
    description: "Customize colors, outlines, point sizes, and labels.",
    shortcut: "Ctrl+Shift+Y",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "crs_manager",
    label: "Coordinate System (CRS)",
    description: "Inspect or reproject layer coordinate reference systems.",
    shortcut: "Ctrl+Shift+C",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "dataset_browser",
    label: "Dataset Browser",
    description: "Browse uploaded GeoPackages, Shapefiles, and GeoTIFFs.",
    shortcut: "Ctrl+Shift+D",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "processing_toolbox",
    label: "GIS Processing Toolbox",
    description: "Open spatial analysis tools (Buffer, Clip, Overlay, Geometry).",
    shortcut: "Ctrl+Shift+P",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "add_dataset",
    label: "Add Dataset",
    description: "Open the Unified Upload modal to add Vector, Raster, or Base layers.",
    shortcut: "Ctrl+Shift+A",
    category: "Panels & Modals",
    enabled: true,
  },
  {
    tool_id: "shortcuts_help",
    label: "Keyboard Shortcuts",
    description: "View and customize all workspace keyboard shortcuts.",
    shortcut: "?",
    category: "General",
    enabled: true,
  },
]

export function normalizeShortcutCombo(combo) {
  if (!combo || typeof combo !== "string") return ""
  const parts = combo
    .split("+")
    .map((p) => p.trim())
    .filter(Boolean)
  const mods = []
  let key = ""
  for (const p of parts) {
    const low = p.toLowerCase()
    if (low === "ctrl" || low === "control" || low === "cmd" || low === "meta") {
      if (!mods.includes("Ctrl")) mods.push("Ctrl")
    } else if (low === "shift") {
      if (!mods.includes("Shift")) mods.push("Shift")
    } else if (low === "alt" || low === "option") {
      if (!mods.includes("Alt")) mods.push("Alt")
    } else {
      if (low === "esc" || low === "escape") key = "Escape"
      else if (low === "del" || low === "delete") key = "Delete"
      else if (/^f\d{1,2}$/i.test(p)) key = p.toUpperCase()
      else if (p.length === 1) key = p.toUpperCase()
      else key = p
    }
  }
  return [...mods, key].filter(Boolean).join("+")
}

export function formatShortcutForOS(combo, platformStr) {
  const norm = normalizeShortcutCombo(combo)
  if (!norm) return ""
  const plat =
    platformStr ??
    (typeof navigator !== "undefined" ? navigator.platform || navigator.userAgent : "")
  const isMac = /mac|iphone|ipad/i.test(plat)
  if (!isMac) return norm
  return norm.replaceAll("Ctrl+", "⌘").replaceAll("Shift+", "⇧").replaceAll("Alt+", "⌥")
}

export function isTextInputTarget(target) {
  if (!target || typeof target !== "object") return false
  const tag = String(target.tagName || "").toUpperCase()
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") {
    const type = String(target.type || "").toLowerCase()
    if (tag === "INPUT" && (type === "checkbox" || type === "radio" || type === "button" || type === "submit")) {
      return false
    }
    return true
  }
  if (target.isContentEditable) return true
  if (typeof target.closest === "function" && target.closest('[contenteditable="true"]')) {
    return true
  }
  return false
}

export function eventToShortcutCombo(event) {
  if (!event || !event.key) return ""
  const k = event.key
  if (k === "Control" || k === "Shift" || k === "Alt" || k === "Meta") return ""
  const mods = []
  if (event.ctrlKey || event.metaKey) mods.push("Ctrl")
  if (event.shiftKey && k !== "?") mods.push("Shift")
  if (event.altKey) mods.push("Alt")

  let keyPart = k
  if (k === "Escape" || k === "Esc") keyPart = "Escape"
  else if (k === "Delete") keyPart = "Delete"
  else if (/^F\d{1,2}$/i.test(k)) keyPart = k.toUpperCase()
  else if (k.length === 1) keyPart = k.toUpperCase()

  return [...mods, keyPart].join("+")
}

export function matchesShortcutEvent(event, combo) {
  if (!event || !combo) return false
  const expected = normalizeShortcutCombo(combo)
  const actual = eventToShortcutCombo(event)
  return Boolean(expected && actual && expected === actual)
}

export function loadUserShortcutOverrides() {
  try {
    const raw = localStorage.getItem(SHORTCUT_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    return parsed && typeof parsed === "object" ? parsed : {}
  } catch {
    return {}
  }
}

export function saveUserShortcutOverrides(overrides) {
  try {
    localStorage.setItem(SHORTCUT_STORAGE_KEY, JSON.stringify(overrides || {}))
  } catch {}
}

export function getEffectiveShortcuts(overrides = {}) {
  return DEFAULT_SHORTCUTS.map((def) => {
    const custom = overrides?.[def.tool_id]
    return {
      ...def,
      shortcut: custom ? normalizeShortcutCombo(custom) : def.shortcut,
    }
  })
}

export function validateShortcutAssignment(effectiveShortcuts, toolId, newCombo) {
  const norm = normalizeShortcutCombo(newCombo)
  if (!norm) {
    return { valid: false, error: "Please press a valid key combination." }
  }
  // Avoid common OS/browser conflicts
  const reserved = new Set(["Ctrl+W", "Ctrl+T", "Ctrl+N", "Ctrl+R", "Ctrl+Q", "Ctrl+C", "Ctrl+V", "Ctrl+X"])
  if (reserved.has(norm)) {
    return { valid: false, error: `${norm} is reserved by your browser/OS.` }
  }
  const conflict = effectiveShortcuts.find(
    (s) => s.tool_id !== toolId && normalizeShortcutCombo(s.shortcut) === norm
  )
  if (conflict) {
    return { valid: false, error: `${norm} is already assigned to "${conflict.label}".` }
  }
  return { valid: true, shortcut: norm }
}
