import React, { useEffect, useState } from "react"
import { Palette, Plus, Trash2, Check, X } from "lucide-react"
import { getLayerStyle, updateLayerStyle, validateLayerStyle } from "../../api"

export default function StyleEditorPanel({
  layer,
  onStyleSaved,
  onClose,
  notify,
}) {
  const [styleType, setStyleType] = useState("single")
  const [fillColor, setFillColor] = useState("#3ecf8e")
  const [strokeColor, setStrokeColor] = useState("#24b47e")
  const [pointColor, setPointColor] = useState("#3ecf8e")
  const [strokeWidth, setStrokeWidth] = useState(2.0)
  const [pointRadius, setPointRadius] = useState(6.0)
  const [opacity, setOpacity] = useState(0.85)
  const [labelField, setLabelField] = useState("")
  const [labelSize, setLabelSize] = useState(12)
  const [labelColor, setLabelColor] = useState("#ffffff")

  // Categorized / Graduated / Rule-based fields
  const [field, setField] = useState("zone")
  const [categories, setCategories] = useState([
    { value: "commercial", color: "#f59e0b" },
    { value: "residential", color: "#3b82f6" },
  ])
  const [breaks, setBreaks] = useState([
    { stop: 100, color: "#60a5fa" },
    { stop: 250, color: "#1d4ed8" },
  ])
  const [rules, setRules] = useState([
    { field: "value", operator: ">", value: 150, color: "#ec4899" },
  ])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!layer?.layer_id) return
    getLayerStyle(layer.layer_id)
      .then((st) => {
        if (!st) return
        setStyleType(st.style_type || "single")
        const cfg = st.style_config || {}
        if (cfg.fill_color) setFillColor(cfg.fill_color)
        if (cfg.stroke_color) setStrokeColor(cfg.stroke_color)
        if (cfg.point_color) setPointColor(cfg.point_color)
        if (cfg.stroke_width !== undefined) setStrokeWidth(cfg.stroke_width)
        if (cfg.point_radius !== undefined) setPointRadius(cfg.point_radius)
        if (cfg.opacity !== undefined) setOpacity(cfg.opacity)
        if (cfg.label_field !== undefined) setLabelField(cfg.label_field)
        if (cfg.label_size !== undefined) setLabelSize(cfg.label_size)
        if (cfg.label_color) setLabelColor(cfg.label_color)
        if (cfg.field) setField(cfg.field)
        if (Array.isArray(cfg.categories) && cfg.categories.length > 0) setCategories(cfg.categories)
        if (Array.isArray(cfg.breaks) && cfg.breaks.length > 0) setBreaks(cfg.breaks)
        if (Array.isArray(cfg.rules) && cfg.rules.length > 0) setRules(cfg.rules)
      })
      .catch(() => {})
  }, [layer?.layer_id])

  if (!layer) return null

  const buildPayload = () => {
    const styleConfig = {
      fill_color: fillColor,
      stroke_color: strokeColor,
      point_color: pointColor,
      stroke_width: Number(strokeWidth),
      point_radius: Number(pointRadius),
      opacity: Number(opacity),
      label_field: labelField.trim(),
      label_size: Number(labelSize),
      label_color: labelColor,
    }
    if (styleType === "categorized") {
      styleConfig.field = field.trim()
      styleConfig.categories = categories
    } else if (styleType === "graduated") {
      styleConfig.field = field.trim()
      styleConfig.breaks = breaks.map((b) => ({ stop: Number(b.stop), color: b.color }))
    } else if (styleType === "rule_based") {
      styleConfig.rules = rules
    }
    return {
      style_type: styleType,
      style_config: styleConfig,
    }
  }

  const handleSave = async (e) => {
    e.preventDefault()
    setSaving(true)
    try {
      const payload = buildPayload()
      const validation = await validateLayerStyle(layer.layer_id, payload)
      if (validation && validation.is_valid === false) {
        notify?.(validation.error || "Invalid style rules", "error")
        setSaving(false)
        return
      }
      const saved = await updateLayerStyle(layer.layer_id, payload)
      notify?.(`Saved symbology for "${layer.layer_name}"`)
      onStyleSaved?.(saved)
    } catch (err) {
      notify?.(err?.response?.data?.error?.message || "Failed saving style", "error")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="w-80 bg-white dark:bg-[#181818] border-l border-neutral-200 dark:border-[#2c2c2c] flex flex-col h-full z-30 shadow-xl text-xs">
      <div className="px-3.5 py-2.5 border-b border-neutral-200 dark:border-[#282828] flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Palette className="size-4 text-[#3ecf8e]" />
          <strong className="text-neutral-900 dark:text-white truncate">
            Style: {layer.layer_name}
          </strong>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="p-1 rounded hover:bg-neutral-100 dark:hover:bg-[#282828] text-neutral-400"
        >
          <X className="size-4" />
        </button>
      </div>

      <form onSubmit={handleSave} className="flex-1 overflow-y-auto p-3.5 space-y-3.5">
        <div>
          <label className="block text-[11px] font-semibold text-neutral-500 dark:text-neutral-400 mb-1">
            Symbology Mode
          </label>
          <select
            value={styleType}
            onChange={(e) => setStyleType(e.target.value)}
            className="w-full px-2.5 py-1.5 rounded-[6px] bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333] text-neutral-900 dark:text-white font-medium"
          >
            <option value="single">Single Symbol</option>
            <option value="categorized">Categorized (By Attribute)</option>
            <option value="graduated">Graduated (Numeric Stops)</option>
          </select>
        </div>

        {/* Base Symbol Colors & Dimensions */}
        <div className="grid grid-cols-3 gap-2">
          <label className="flex flex-col gap-1">
            <span className="text-[10px] text-neutral-400">Fill Color</span>
            <input
              type="color"
              value={fillColor}
              onChange={(e) => setFillColor(e.target.value)}
              className="h-8 w-full rounded cursor-pointer bg-transparent"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-[10px] text-neutral-400">Stroke Color</span>
            <input
              type="color"
              value={strokeColor}
              onChange={(e) => setStrokeColor(e.target.value)}
              className="h-8 w-full rounded cursor-pointer bg-transparent"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-[10px] text-neutral-400">Point Color</span>
            <input
              type="color"
              value={pointColor}
              onChange={(e) => setPointColor(e.target.value)}
              className="h-8 w-full rounded cursor-pointer bg-transparent"
            />
          </label>
        </div>

        <div className="grid grid-cols-3 gap-2">
          <label className="flex flex-col gap-1">
            <span className="text-[10px] text-neutral-400">Line Width</span>
            <input
              type="number"
              min={0}
              max={30}
              step={0.5}
              value={strokeWidth}
              onChange={(e) => setStrokeWidth(e.target.value)}
              className="px-2 py-1 rounded bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333]"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-[10px] text-neutral-400">Point Radius</span>
            <input
              type="number"
              min={1}
              max={50}
              step={1}
              value={pointRadius}
              onChange={(e) => setPointRadius(e.target.value)}
              className="px-2 py-1 rounded bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333]"
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-[10px] text-neutral-400">Opacity</span>
            <input
              type="number"
              min={0}
              max={1}
              step={0.05}
              value={opacity}
              onChange={(e) => setOpacity(e.target.value)}
              className="px-2 py-1 rounded bg-neutral-100 dark:bg-[#242424] border border-neutral-300 dark:border-[#333]"
            />
          </label>
        </div>

        {/* Categorized Editor */}
        {styleType === "categorized" && (
          <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] space-y-2">
            <label className="block">
              <span className="text-[10px] text-neutral-400">Attribute Field</span>
              <input
                type="text"
                value={field}
                onChange={(e) => setField(e.target.value)}
                placeholder="e.g. zone"
                className="w-full mt-0.5 px-2 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
              />
            </label>
            <div className="space-y-1.5">
              {categories.map((cat, idx) => (
                <div key={idx} className="flex items-center gap-1.5">
                  <input
                    type="text"
                    value={cat.value}
                    onChange={(e) => {
                      const next = [...categories]
                      next[idx] = { ...cat, value: e.target.value }
                      setCategories(next)
                    }}
                    placeholder="Value"
                    className="flex-1 px-2 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
                  />
                  <input
                    type="color"
                    value={cat.color}
                    onChange={(e) => {
                      const next = [...categories]
                      next[idx] = { ...cat, color: e.target.value }
                      setCategories(next)
                    }}
                    className="h-6 w-8 rounded cursor-pointer"
                  />
                  <button
                    type="button"
                    onClick={() => setCategories((prev) => prev.filter((_, i) => i !== idx))}
                    className="text-neutral-400 hover:text-red-500"
                  >
                    <Trash2 className="size-3.5" />
                  </button>
                </div>
              ))}
              <button
                type="button"
                onClick={() => setCategories((prev) => [...prev, { value: "new", color: "#10b981" }])}
                className="inline-flex items-center gap-1 text-[11px] text-[#3ecf8e] font-semibold"
              >
                <Plus className="size-3" /> Add Category
              </button>
            </div>
          </div>
        )}

        {/* Graduated Editor */}
        {styleType === "graduated" && (
          <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] space-y-2">
            <label className="block">
              <span className="text-[10px] text-neutral-400">Numeric Field</span>
              <input
                type="text"
                value={field}
                onChange={(e) => setField(e.target.value)}
                placeholder="e.g. value"
                className="w-full mt-0.5 px-2 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
              />
            </label>
            <div className="space-y-1.5">
              {breaks.map((br, idx) => (
                <div key={idx} className="flex items-center gap-1.5">
                  <input
                    type="number"
                    value={br.stop}
                    onChange={(e) => {
                      const next = [...breaks]
                      next[idx] = { ...br, stop: Number(e.target.value) }
                      setBreaks(next)
                    }}
                    className="flex-1 px-2 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
                  />
                  <input
                    type="color"
                    value={br.color}
                    onChange={(e) => {
                      const next = [...breaks]
                      next[idx] = { ...br, color: e.target.value }
                      setBreaks(next)
                    }}
                    className="h-6 w-8 rounded cursor-pointer"
                  />
                  <button
                    type="button"
                    onClick={() => setBreaks((prev) => prev.filter((_, i) => i !== idx))}
                    className="text-neutral-400 hover:text-red-500"
                  >
                    <Trash2 className="size-3.5" />
                  </button>
                </div>
              ))}
              <button
                type="button"
                onClick={() => setBreaks((prev) => [...prev, { stop: 500, color: "#1e40af" }])}
                className="inline-flex items-center gap-1 text-[11px] text-[#3ecf8e] font-semibold"
              >
                <Plus className="size-3" /> Add Class Break
              </button>
            </div>
          </div>
        )}

        {/* Rule-Based Editor */}
        {styleType === "rule_based" && (
          <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] space-y-2">
            <div className="text-[10px] font-semibold text-neutral-400 uppercase">Rules</div>
            {rules.map((rl, idx) => (
              <div key={idx} className="space-y-1 border-b border-neutral-200 dark:border-[#303030] pb-1.5">
                <div className="flex items-center gap-1">
                  <input
                    type="text"
                    value={rl.field}
                    onChange={(e) => {
                      const next = [...rules]
                      next[idx] = { ...rl, field: e.target.value }
                      setRules(next)
                    }}
                    placeholder="field"
                    className="w-20 px-1.5 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
                  />
                  <select
                    value={rl.operator}
                    onChange={(e) => {
                      const next = [...rules]
                      next[idx] = { ...rl, operator: e.target.value }
                      setRules(next)
                    }}
                    className="px-1 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
                  >
                    <option value="=">=</option>
                    <option value="!=">!=</option>
                    <option value=">">&gt;</option>
                    <option value="<">&lt;</option>
                  </select>
                  <input
                    type="text"
                    value={rl.value}
                    onChange={(e) => {
                      const next = [...rules]
                      next[idx] = { ...rl, value: e.target.value }
                      setRules(next)
                    }}
                    placeholder="val"
                    className="flex-1 px-1.5 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
                  />
                  <input
                    type="color"
                    value={rl.color}
                    onChange={(e) => {
                      const next = [...rules]
                      next[idx] = { ...rl, color: e.target.value }
                      setRules(next)
                    }}
                    className="h-6 w-7 rounded"
                  />
                </div>
              </div>
            ))}
            <button
              type="button"
              onClick={() => setRules((prev) => [...prev, { field: "name", operator: "=", value: "A", color: "#10b981" }])}
              className="inline-flex items-center gap-1 text-[11px] text-[#3ecf8e] font-semibold"
            >
              <Plus className="size-3" /> Add Rule
            </button>
          </div>
        )}

        {/* Data-Driven Labels */}
        <div className="p-2.5 rounded-[6px] bg-neutral-50 dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] space-y-2">
          <div className="text-[11px] font-semibold text-neutral-700 dark:text-neutral-300">
            Data-Driven Labels
          </div>
          <input
            type="text"
            placeholder="Label attribute field (e.g. name, leave blank to hide)"
            value={labelField}
            onChange={(e) => setLabelField(e.target.value)}
            className="w-full px-2 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
          />
          <div className="grid grid-cols-2 gap-2">
            <label className="flex flex-col gap-0.5">
              <span className="text-[10px] text-neutral-400">Font Size</span>
              <input
                type="number"
                min={8}
                max={32}
                value={labelSize}
                onChange={(e) => setLabelSize(e.target.value)}
                className="px-2 py-1 rounded bg-white dark:bg-[#161616] border border-neutral-300 dark:border-[#333]"
              />
            </label>
            <label className="flex flex-col gap-0.5">
              <span className="text-[10px] text-neutral-400">Text Color</span>
              <input
                type="color"
                value={labelColor}
                onChange={(e) => setLabelColor(e.target.value)}
                className="h-7 w-full rounded cursor-pointer"
              />
            </label>
          </div>
        </div>

        <button
          type="submit"
          disabled={saving}
          className="w-full py-2 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold flex items-center justify-center gap-1.5 cursor-pointer disabled:opacity-50"
        >
          <Check className="size-4" />
          <span>{saving ? "Applying & Saving Style…" : "Save & Apply Symbology"}</span>
        </button>
      </form>
    </div>
  )
}
