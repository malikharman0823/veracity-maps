import React, { useEffect, useState } from "react"
import { MapPin, Image, Trash2, Move, X } from "lucide-react"
import { getIconUrl } from "../api"
import IconPicker from "./IconPicker"

function coordinateText(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number.toFixed(6) : "—"
}

export default function IconPointForm({
  coordinate,
  initialName = "",
  initialDescription = "",
  initialIconKey = "cinema",
  initialScope = "floor",
  initialFloorLevel = 0,
  floors = [],
  submitLabel = "Save Point",
  title = "Geographic Icon Point",
  isEditing = false,
  onCancel,
  onSubmit,
  onDelete,
  onStartMove,
}) {
  const [name, setName] = useState(initialName || "")
  const [description, setDescription] = useState(initialDescription || "")
  const [iconId, setIconId] = useState(initialIconKey || "cinema")
  const [scope, setScope] = useState(initialScope || "floor")
  const [floorLevel, setFloorLevel] = useState(initialFloorLevel ?? 0)
  const [showPicker, setShowPicker] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    setName(initialName || "")
    setDescription(initialDescription || "")
    setIconId(initialIconKey || "cinema")
    setScope(initialScope || "floor")
    setFloorLevel(initialFloorLevel ?? 0)
  }, [initialName, initialDescription, initialIconKey, initialScope, initialFloorLevel, coordinate])

  async function handleSubmit(e) {
    e.preventDefault()
    if (!coordinate && !isEditing) return
    setSubmitting(true)
    try {
      await onSubmit({
        name: name.trim() || "Point",
        description: description.trim() || null,
        icon_id: iconId || "cinema",
        icon_key: iconId || "cinema",
        scope,
        floor_level: Number(floorLevel),
        longitude: coordinate?.longitude,
        latitude: coordinate?.latitude,
      })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="bg-white dark:bg-[#202020] rounded-[12px] shadow-xl border border-neutral-200 dark:border-[#2e2e2e] p-4 w-80 space-y-3.5 text-xs text-neutral-900 dark:text-white">
      <div className="flex items-center justify-between pb-2 border-b border-neutral-100 dark:border-[#2e2e2e]">
        <div className="flex items-center gap-1.5 font-semibold text-neutral-900 dark:text-white">
          <MapPin className="size-4 text-[#24b47e] dark:text-[#3ecf8e]" />
          <span>{title}</span>
        </div>
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 p-0.5 rounded-[4px] cursor-pointer"
          >
            <X className="size-4" />
          </button>
        )}
      </div>

      {coordinate && (
        <div className="flex items-center justify-between px-2.5 py-1.5 bg-neutral-50 dark:bg-[#171717] rounded-[6px] text-[11px] text-neutral-500 dark:text-neutral-400 font-mono border border-neutral-200 dark:border-[#2e2e2e]">
          <span>Lat: <strong className="text-neutral-800 dark:text-neutral-200">{coordinateText(coordinate.latitude)}</strong></span>
          <span>Lng: <strong className="text-neutral-800 dark:text-neutral-200">{coordinateText(coordinate.longitude)}</strong></span>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-3">
        {/* Icon Preview & Picker Trigger */}
        <div>
          <label className="block text-[11px] font-medium text-neutral-600 dark:text-neutral-400 mb-1">
            Icon Asset
          </label>
          <button
            type="button"
            onClick={() => setShowPicker(true)}
            className="w-full flex items-center justify-between p-2 bg-neutral-50 dark:bg-[#171717] hover:bg-neutral-100 dark:hover:bg-[#282828] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] transition-colors group cursor-pointer"
          >
            <div className="flex items-center gap-2">
              <div className="size-8 rounded-[4px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center p-1">
                <img
                  src={getIconUrl(iconId)}
                  alt={iconId}
                  className="max-size-full max-h-6 max-w-6 object-contain"
                />
              </div>
              <span className="font-medium text-neutral-800 dark:text-neutral-200 truncate max-w-[140px]">
                {iconId}
              </span>
            </div>
            <span className="text-[11px] text-[#24b47e] dark:text-[#3ecf8e] font-medium group-hover:underline">
              Change…
            </span>
          </button>
        </div>

        {/* Point Name */}
        <div>
          <label className="block text-[11px] font-medium text-neutral-600 dark:text-neutral-400 mb-1">
            Point Name
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Main Entrance, Elevator, Fire Extinguisher"
            required
            className="w-full px-2.5 py-1.5 bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] text-xs text-neutral-800 dark:text-neutral-200 focus:outline-none focus:border-[#3ecf8e]"
          />
        </div>

        {/* Scope and Floor Assignment */}
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="block text-[11px] font-medium text-neutral-600 dark:text-neutral-400 mb-1">
              Scope
            </label>
            <select
              value={scope}
              onChange={(e) => setScope(e.target.value)}
              className="w-full px-2 py-1.5 bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] text-xs text-neutral-800 dark:text-neutral-200 focus:outline-none focus:border-[#3ecf8e]"
            >
              <option value="floor">Floor Specific</option>
              <option value="building">Building-wide</option>
              <option value="dataset">Dataset Only</option>
            </select>
          </div>

          <div>
            <label className="block text-[11px] font-medium text-neutral-600 dark:text-neutral-400 mb-1">
              Floor
            </label>
            <select
              value={floorLevel}
              disabled={scope === "building"}
              onChange={(e) => setFloorLevel(Number(e.target.value))}
              className="w-full px-2 py-1.5 bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] text-xs text-neutral-800 dark:text-neutral-200 focus:outline-none focus:border-[#3ecf8e] disabled:opacity-50 disabled:bg-neutral-100 dark:disabled:bg-[#1c1c1c]"
            >
              {floors.length > 0 ? (
                floors.map((fl) => (
                  <option key={fl.dataset_id || fl.floor_level} value={fl.floor_level}>
                    {fl.floor_label || `Floor ${fl.floor_level}`}
                  </option>
                ))
              ) : (
                <>
                  <option value={-2}>Basement 2 (-2)</option>
                  <option value={-1}>Basement 1 (-1)</option>
                  <option value={0}>Ground Floor (0)</option>
                  <option value={1}>1st Floor (1)</option>
                  <option value={2}>2nd Floor (2)</option>
                  <option value={3}>3rd Floor (3)</option>
                </>
              )}
            </select>
          </div>
        </div>

        {/* Point Description */}
        <div>
          <label className="block text-[11px] font-medium text-neutral-600 dark:text-neutral-400 mb-1">
            Description / Notes (Optional)
          </label>
          <textarea
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Additional geographic notes or operational details…"
            className="w-full px-2.5 py-1.5 bg-white dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] rounded-[6px] text-xs text-neutral-800 dark:text-neutral-200 focus:outline-none focus:border-[#3ecf8e] resize-none"
          />
        </div>

        {/* Action Buttons */}
        <div className="flex items-center gap-2 pt-2 border-t border-neutral-100 dark:border-[#2e2e2e]">
          {isEditing && onDelete && (
            <button
              type="button"
              onClick={onDelete}
              className="p-1.5 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 rounded-[6px] border border-red-200 dark:border-red-900/40 transition-colors cursor-pointer"
              title="Delete this point"
            >
              <Trash2 className="size-3.5" />
            </button>
          )}

          {isEditing && onStartMove && (
            <button
              type="button"
              onClick={onStartMove}
              className="px-2.5 py-1.5 text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-[#282828] hover:bg-neutral-200 dark:hover:bg-[#323232] rounded-[6px] transition-colors flex items-center gap-1 font-medium text-[11px] cursor-pointer"
              title="Move point on map"
            >
              <Move className="size-3" />
              <span>Move</span>
            </button>
          )}

          <div className="flex-1" />

          {onCancel && (
            <button
              type="button"
              onClick={onCancel}
              className="px-3 py-1.5 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-100 dark:hover:bg-[#282828] rounded-[6px] font-medium transition-colors cursor-pointer"
            >
              Cancel
            </button>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-1.5 bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold rounded-[6px] shadow-xs transition-colors disabled:opacity-50 cursor-pointer"
          >
            {submitting ? "Saving…" : submitLabel}
          </button>
        </div>
      </form>

      {/* Modal Icon Picker */}
      {showPicker && (
        <IconPicker
          selectedIconId={iconId}
          onSelectIcon={(icon) => {
            setIconId(icon.id)
            setShowPicker(false)
          }}
          onClose={() => setShowPicker(false)}
        />
      )}
    </div>
  )
}
