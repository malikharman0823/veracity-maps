import React, { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { parseGoogleMapsCoordinates } from "../lib/coordinateParser"

function coordinateText(value, digits = 6) {
  const number = Number(value)
  return Number.isFinite(number) ? number.toFixed(digits) : ""
}

function EditableCoordinateField({
  label,
  value,
  digits = 6,
  min,
  max,
  readOnly = false,
  onCommitSingle,
  onCommitPair,
  ariaLabel,
}) {
  const formatted = coordinateText(value, digits)
  const [text, setText] = useState(formatted)
  const [focused, setFocused] = useState(false)
  const [invalid, setInvalid] = useState(false)

  useEffect(() => {
    if (!focused) {
      setText(formatted)
      setInvalid(false)
    }
  }, [formatted, focused])

  function tryApply(rawValue) {
    const trimmed = String(rawValue || "").trim()
    if (!trimmed) {
      setText(formatted)
      setInvalid(false)
      return
    }

    // If user pasted or typed a full Google Maps "Lat, Lng" / DMS / URL / Plus Code into lat or lon
    if (onCommitPair && (trimmed.includes(",") || trimmed.includes(" ") || /[NSEW°+]/i.test(trimmed) || /^https?:\/\//i.test(trimmed))) {
      const pair = parseGoogleMapsCoordinates(trimmed, { order: "lat-lng" })
      if (pair.ok) {
        setInvalid(false)
        onCommitPair({ latitude: pair.latitude, longitude: pair.longitude })
        return
      }
    }

    const num = Number(trimmed)
    if (!Number.isFinite(num) || (min !== undefined && num < min) || (max !== undefined && num > max)) {
      setInvalid(true)
      return
    }

    setInvalid(false)
    if (Math.abs(num - Number(value)) > 1e-8) {
      onCommitSingle(num)
    } else {
      setText(coordinateText(num, digits))
    }
  }

  if (readOnly) {
    return (
      <label className="grid min-w-0 grid-cols-[auto_1fr] items-center gap-2 text-[12px] text-muted">
        <span>{label}</span>
        <span className="h-9 min-w-0 truncate rounded-lg border border-hairline bg-pearl px-2 py-2 text-right tabular-nums text-ink">
          {formatted || "—"}
        </span>
      </label>
    )
  }

  return (
    <label className="grid min-w-0 grid-cols-[auto_1fr] items-center gap-1.5 text-[12px] text-muted">
      <span>{label}</span>
      <input
        type="text"
        inputMode="decimal"
        aria-label={ariaLabel || label}
        value={text}
        onFocus={() => setFocused(true)}
        onChange={(event) => {
          setText(event.target.value)
          if (invalid) setInvalid(false)
        }}
        onPaste={(event) => {
          if (!onCommitPair) return
          const pasted = event.clipboardData?.getData("text") || ""
          const pair = parseGoogleMapsCoordinates(pasted, { order: "lat-lng" })
          if (pair.ok) {
            event.preventDefault()
            setInvalid(false)
            onCommitPair({ latitude: pair.latitude, longitude: pair.longitude })
          }
        }}
        onBlur={() => {
          setFocused(false)
          tryApply(text)
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault()
            event.currentTarget.blur()
          } else if (event.key === "Escape") {
            setText(formatted)
            setInvalid(false)
            event.currentTarget.blur()
          }
        }}
        className={`h-9 min-w-0 w-full rounded-lg border px-2 py-1.5 text-right text-[12px] tabular-nums text-ink outline-none transition-colors ${
          invalid
            ? "border-[#e03126] bg-[#fff6f6]"
            : "border-hairline bg-white hover:border-[#c8c8cc] focus:border-[#0066cc]"
        }`}
      />
    </label>
  )
}

export default function ControlPointList({
  points,
  draft,
  selectedId,
  onSelect,
  onUpdate,
  onDelete,
  onPairDraft,
  readOnly = false,
  busy = false,
}) {
  const [draftCoordInput, setDraftCoordInput] = useState("")
  const [draftError, setDraftError] = useState(null)

  useEffect(() => {
    if (!draft) {
      setDraftCoordInput("")
      setDraftError(null)
    }
  }, [draft])

  function submitDraftCoordinates(event) {
    event.preventDefault()
    if (!draft || !onPairDraft || busy) return
    const parsed = parseGoogleMapsCoordinates(draftCoordInput, { order: "lat-lng" })
    if (!parsed.ok) {
      setDraftError(parsed.error)
      return
    }
    setDraftError(null)
    onPairDraft({ lat: parsed.latitude, lng: parsed.longitude })
  }

  return (
    <section className="overflow-hidden rounded-utility border border-hairline bg-white">
      <header className="flex min-h-14 items-center justify-between border-b border-hairline px-5">
        <div>
          <h2 className="text-[13px] font-semibold tracking-[0.08em] text-ink">CONTROL POINTS</h2>
          <p className="mt-0.5 text-[12px] text-muted">
            {readOnly
              ? "Inspect paired pixel and Google Maps Lat, Lng coordinates"
              : "Click or paste Google Maps Lat, Lng directly into any point"}
          </p>
        </div>
        <span className="text-[13px] tabular-nums text-muted">{points.length} / 10</span>
      </header>
      <div className="divide-y divide-hairline">
        {points.length === 0 && !draft && (
          <p className="px-5 py-6 text-[14px] text-muted">
            Click <strong>Place by Lat, Lng</strong> above to position by Google Maps coordinates, or click{" "}
            <strong>Add Control Point</strong> to pair points visually.
          </p>
        )}
        {points.map((point, index) => (
          <article
            key={point.id}
            className={`grid cursor-pointer gap-3 px-4 py-3 transition-colors tablet:grid-cols-[36px_1fr_1.2fr_auto] tablet:items-center${
              selectedId === point.id ? " bg-[#eaf3ff]" : " hover:bg-pearl"
            }`}
            onClick={() => onSelect(point.id)}
          >
            <div className="flex items-center gap-2 text-[14px] font-semibold tabular-nums">
              <span
                className={`h-2.5 w-2.5 rounded-full${
                  selectedId === point.id ? " bg-[#004c99]" : " bg-primary"
                }`}
              />
              {index + 1}
            </div>
            <div className="grid grid-cols-2 gap-2">
              <EditableCoordinateField
                label="x"
                digits={1}
                min={0}
                value={point.image_x}
                readOnly={readOnly || !onUpdate}
                ariaLabel={`Point ${index + 1} image X`}
                onCommitSingle={(image_x) => onUpdate?.(point.id, { image_x })}
              />
              <EditableCoordinateField
                label="y"
                digits={1}
                min={0}
                value={point.image_y}
                readOnly={readOnly || !onUpdate}
                ariaLabel={`Point ${index + 1} image Y`}
                onCommitSingle={(image_y) => onUpdate?.(point.id, { image_y })}
              />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <EditableCoordinateField
                label="lat"
                digits={6}
                min={-90}
                max={90}
                value={point.latitude}
                readOnly={readOnly || !onUpdate}
                ariaLabel={`Point ${index + 1} latitude`}
                onCommitSingle={(latitude) => onUpdate?.(point.id, { latitude })}
                onCommitPair={(coords) => onUpdate?.(point.id, coords)}
              />
              <EditableCoordinateField
                label="lng"
                digits={6}
                min={-180}
                max={180}
                value={point.longitude}
                readOnly={readOnly || !onUpdate}
                ariaLabel={`Point ${index + 1} longitude`}
                onCommitSingle={(longitude) => onUpdate?.(point.id, { longitude })}
                onCommitPair={(coords) => onUpdate?.(point.id, coords)}
              />
            </div>
            <Button
              type="button"
              variant="ghost"
              className="min-h-10 rounded-full px-3 text-[13px] text-muted hover:bg-[#f5f5f7] hover:text-ink"
              disabled={readOnly}
              onClick={(event) => {
                event.stopPropagation()
                onDelete(point.id)
              }}
            >
              Delete
            </Button>
          </article>
        ))}
        {draft && (
          <article className="flex flex-col gap-2.5 bg-[#fff8e6] px-4 py-3 text-[13px]">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <span className="font-semibold tabular-nums text-ink">#{points.length + 1}</span>
                <span className="text-muted">
                  Source pixel ({Math.round(draft.image_x)}, {Math.round(draft.image_y)}) · Click Reference Map or enter Google Maps Lat, Lng:
                </span>
              </div>
            </div>
            {!readOnly && onPairDraft && (
              <form className="flex flex-wrap items-center gap-2" onSubmit={submitDraftCoordinates}>
                <Input
                  value={draftCoordInput}
                  placeholder="Paste Google Maps Lat, Lng (e.g. 28.6139, 77.2090)"
                  aria-label="Enter Google Maps latitude and longitude for pending control point"
                  className="h-9 min-w-[240px] flex-1 rounded-full bg-white px-3.5 text-[12px]"
                  onChange={(event) => {
                    setDraftCoordInput(event.target.value)
                    if (draftError) setDraftError(null)
                  }}
                />
                <Button
                  type="submit"
                  size="sm"
                  disabled={busy}
                  className="h-9 rounded-full bg-[#0066cc] px-4 text-xs font-normal text-white hover:bg-[#0071e3]"
                >
                  Pair Point
                </Button>
              </form>
            )}
            {draftError && (
              <p role="alert" className="text-[12px] text-[#b42318]">
                {draftError}
              </p>
            )}
          </article>
        )}
      </div>
    </section>
  )
}
