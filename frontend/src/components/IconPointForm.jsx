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
    <div className="bg-white rounded-xl shadow-xl border border-gray-200 p-4 w-80 space-y-3.5 text-xs">
      <div className="flex items-center justify-between pb-2 border-b border-gray-100">
        <div className="flex items-center gap-1.5 font-semibold text-gray-900">
          <MapPin className="size-4 text-blue-600" />
          <span>{title}</span>
        </div>
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="text-gray-400 hover:text-gray-600 p-0.5 rounded"
          >
            <X className="size-4" />
          </button>
        )}
      </div>

      {coordinate && (
        <div className="flex items-center justify-between px-2.5 py-1.5 bg-gray-50 rounded-lg text-[11px] text-gray-500 font-mono">
          <span>Lat: <strong className="text-gray-800">{coordinateText(coordinate.latitude)}</strong></span>
          <span>Lng: <strong className="text-gray-800">{coordinateText(coordinate.longitude)}</strong></span>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-3">
        {/* Icon Preview & Picker Trigger */}
        <div>
          <label className="block text-[11px] font-medium text-gray-600 mb-1">
            Icon Asset
          </label>
          <button
            type="button"
            onClick={() => setShowPicker(true)}
            className="w-full flex items-center justify-between p-2 bg-gray-50 hover:bg-gray-100/80 border border-gray-200 rounded-lg transition-colors group"
          >
            <div className="flex items-center gap-2">
              <div className="size-8 rounded-md bg-white border border-gray-200 flex items-center justify-center p-1 bg-[radial-gradient(#e5e7eb_1px,transparent_1px)] [background-size:4px_4px]">
                <img
                  src={getIconUrl(iconId)}
                  alt={iconId}
                  className="max-size-full max-h-6 max-w-6 object-contain"
                />
              </div>
              <span className="font-medium text-gray-800 truncate max-w-[140px]">
                {iconId}
              </span>
            </div>
            <span className="text-[11px] text-blue-600 font-medium group-hover:underline">
              Change…
            </span>
          </button>
        </div>

        {/* Point Name */}
        <div>
          <label className="block text-[11px] font-medium text-gray-600 mb-1">
            Point Name
          </label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Main Entrance, Elevator, Fire Extinguisher"
            required
            className="w-full px-2.5 py-1.5 bg-white border border-gray-200 rounded-lg text-xs text-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500"
          />
        </div>

        {/* Scope and Floor Assignment */}
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="block text-[11px] font-medium text-gray-600 mb-1">
              Scope
            </label>
            <select
              value={scope}
              onChange={(e) => setScope(e.target.value)}
              className="w-full px-2 py-1.5 bg-white border border-gray-200 rounded-lg text-xs text-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500"
            >
              <option value="floor">Floor Specific</option>
              <option value="building">Building-wide</option>
              <option value="dataset">Dataset Only</option>
            </select>
          </div>

          <div>
            <label className="block text-[11px] font-medium text-gray-600 mb-1">
              Floor
            </label>
            <select
              value={floorLevel}
              disabled={scope === "building"}
              onChange={(e) => setFloorLevel(Number(e.target.value))}
              className="w-full px-2 py-1.5 bg-white border border-gray-200 rounded-lg text-xs text-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 disabled:opacity-50 disabled:bg-gray-100"
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
          <label className="block text-[11px] font-medium text-gray-600 mb-1">
            Description / Notes (Optional)
          </label>
          <textarea
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Additional geographic notes or operational details…"
            className="w-full px-2.5 py-1.5 bg-white border border-gray-200 rounded-lg text-xs text-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500 resize-none"
          />
        </div>

        {/* Action Buttons */}
        <div className="flex items-center gap-2 pt-1 border-t border-gray-100">
          {isEditing && onDelete && (
            <button
              type="button"
              onClick={onDelete}
              className="p-1.5 text-red-600 hover:bg-red-50 rounded-lg border border-red-200 transition-colors"
              title="Delete this point"
            >
              <Trash2 className="size-3.5" />
            </button>
          )}

          {isEditing && onStartMove && (
            <button
              type="button"
              onClick={onStartMove}
              className="px-2.5 py-1.5 text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-lg transition-colors flex items-center gap-1 font-medium text-[11px]"
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
              className="px-3 py-1.5 text-gray-600 hover:bg-gray-100 rounded-lg font-medium transition-colors"
            >
              Cancel
            </button>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="px-4 py-1.5 bg-blue-600 hover:bg-blue-700 text-white font-medium rounded-lg shadow-xs transition-colors disabled:opacity-50"
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
