import React, { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"

import {
  createControlPoint,
  datasetSourceImageUrl,
  georeferenceDataset,
  getDataset,
  getGeoreferenceConfig,
  listControlPoints,
  readableApiError,
  removeControlPoint,
  updateControlPoint,
} from "../api"
import ControlPointList from "../components/ControlPointList"
import ReferenceMap from "../components/ReferenceMap"
import SourceImagePane from "../components/SourceImagePane"
import {
  buildDirectCoordinateControlPoints,
  parseGoogleMapsCoordinates,
} from "../lib/coordinateParser"
import { safeWorkspaceReturn } from "../lib/workspaceNavigation"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ArrowLeft, MapPin, X } from "lucide-react"
import EndSessionDialog from "../components/EndSessionDialog"

function overlayCoordinates(parameters, imageSize) {
  if (!parameters) return null
  const width = Number(imageSize?.width)
  const height = Number(imageSize?.height)
  if (!width || !height) return null

  let a, b, c, d, e, f

  // 1. Array format [a, -b, c, d, -e, f] (from transform or affine matrix)
  const transformArray = Array.isArray(parameters)
    ? parameters
    : Array.isArray(parameters?.transform)
      ? parameters.transform
      : null

  if (transformArray && transformArray.length === 6) {
    const [t0, t1, t2, t3, t4, t5] = transformArray.map(Number)
    if ([t0, t1, t2, t3, t4, t5].every(Number.isFinite)) {
      a = t0
      b = -t1
      c = t2
      d = t3
      e = -t4
      f = t5
    }
  }

  // 2. Object format { a, b, c, d, e, f }
  if (a === undefined) {
    const params = parameters?.parameters || parameters?.transformation_parameters || parameters
    if (params && typeof params === "object") {
      const values = [params.a, params.b, params.c, params.d, params.e, params.f].map(Number)
      if (values.every(Number.isFinite)) {
        ;[a, b, c, d, e, f] = values
      }
    }
  }

  if (a === undefined || !Number.isFinite(a) || !Number.isFinite(d)) return null

  const project = (imageX, imageY) => [
    a * imageX + b * -imageY + c,
    d * imageX + e * -imageY + f,
  ]

  // MapLibre GL image source expects 4 coordinates in clockwise order:
  // [top-left, top-right, bottom-right, bottom-left]
  return [project(0, 0), project(width, 0), project(width, height), project(0, height)]
}

function recommendation(imageSize) {
  const width = Number(imageSize?.width) || 0
  const height = Number(imageSize?.height) || 0
  const longestEdge = Math.max(width, height)
  const ratio = Math.max(width, height) / Math.max(1, Math.min(width, height))
  if (longestEdge >= 6000 || ratio > 2.2) return "7–10 points for a large or irregular image"
  if (longestEdge >= 3000) return "4–6 points for this image"
  return "3–4 points for this image"
}

function titleText(value) {
  return String(value || "").replaceAll("_", " ").replace(/\b\w/g, (character) => character.toUpperCase())
}

function SavedConfiguration({ configuration }) {
  if (!configuration) return <p className="text-[13px] text-muted">No saved configuration.</p>
  const rows = [
    ["Status", titleText(configuration.status)],
    ["Transformation", titleText(configuration.transformation)],
    ["Control points", configuration.control_point_count],
    ["Target CRS", configuration.target_crs],
    ["Source size", `${configuration.source_width} × ${configuration.source_height}`],
    ["Output", configuration.output_format],
  ]
  return (
    <dl className="grid gap-2 text-[12px]">
      {rows.map(([label, value]) => (
        <div key={label} className="grid grid-cols-[100px_1fr] gap-3">
          <dt className="text-muted">{label}</dt>
          <dd className="text-right text-ink">{value}</dd>
        </div>
      ))}
    </dl>
  )
}

export default function GeoreferencePage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const returnTo = safeWorkspaceReturn(searchParams.get("returnTo"), id)
  const pointsRef = useRef([])
  const pairingRef = useRef(false)
  const [dataset, setDataset] = useState(null)
  const [points, setPoints] = useState([])
  const [savedConfig, setSavedConfig] = useState(null)
  const [calculation, setCalculation] = useState(null)
  const [mode, setMode] = useState("edit")
  const [controlPointPlacement, setControlPointPlacement] = useState(false)
  const [draft, setDraft] = useState(null)
  const [draftBannerCoord, setDraftBannerCoord] = useState("")
  const [selectedId, setSelectedId] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [notice, setNotice] = useState(null)
  const [savingPoint, setSavingPoint] = useState(false)
  const [calculating, setCalculating] = useState(false)
  const [savingConfig, setSavingConfig] = useState(false)
  const [applyingDirect, setApplyingDirect] = useState(false)
  const [overlayEnabled, setOverlayEnabled] = useState(false)
  const [overlayOpacity, setOverlayOpacity] = useState(0.65)
  const [focusTarget, setFocusTarget] = useState(null)

  // Direct Google Maps Lat/Lng placement state
  const [directPanelOpen, setDirectPanelOpen] = useState(false)
  const [directMode, setDirectMode] = useState("center") // "center" | "corners"
  const [directCenterInput, setDirectCenterInput] = useState("")
  const [directWidthMeters, setDirectWidthMeters] = useState("100")
  const [directRotationDeg, setDirectRotationDeg] = useState("0")
  const [directTopLeftInput, setDirectTopLeftInput] = useState("")
  const [directBottomRightInput, setDirectBottomRightInput] = useState("")
  const [directError, setDirectError] = useState(null)

  useEffect(() => {
    pointsRef.current = points
  }, [points])

  useEffect(() => {
    if (!draft) {
      setDraftBannerCoord("")
    }
  }, [draft])

  useEffect(() => {
    const controller = new AbortController()
    let active = true
    setLoading(true)
    Promise.all([
      getDataset(id, controller.signal),
      listControlPoints(id, controller.signal),
      getGeoreferenceConfig(id, controller.signal),
    ])
      .then(([nextDataset, nextPoints, configuration]) => {
        if (!active) return
        setDataset(nextDataset)
        setPoints(nextPoints)
        setSavedConfig(configuration)
        setMode(configuration ? "view" : "edit")
        if (configuration?.reusable) {
          setCalculation(configuration)
          setOverlayEnabled(true)
        }
        setSelectedId(nextPoints[0]?.id || null)
      })
      .catch((requestError) => {
        if (active && requestError.code !== "ERR_CANCELED") {
          setError(readableApiError(requestError, "Unable to load the visual georeferencer."))
        }
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
      controller.abort()
    }
  }, [id])

  const [naturalDimensions, setNaturalDimensions] = useState(null)
  const imageSize = useMemo(() => {
    const meta = dataset?.extracted_metadata || dataset?.metadata
    const w = Number(meta?.width) || Number(naturalDimensions?.width) || 0
    const h = Number(meta?.height) || Number(naturalDimensions?.height) || 0
    return w > 0 && h > 0 ? { width: w, height: h } : null
  }, [dataset, naturalDimensions])

  const aspectMatchedHeightMeters = useMemo(() => {
    const wMeters = Number(directWidthMeters)
    if (!imageSize || !Number.isFinite(wMeters) || wMeters <= 0) return null
    return ((wMeters / imageSize.width) * imageSize.height).toFixed(1)
  }, [directWidthMeters, imageSize])

  const sourceUrl = useMemo(() => datasetSourceImageUrl(id), [id])
  const previewCoordinates = useMemo(
    () =>
      overlayCoordinates(
        calculation?.parameters ||
          calculation?.transformation_parameters ||
          calculation?.transform ||
          calculation,
        imageSize,
      ),
    [calculation, imageSize],
  )
  const transformLabel = points.length < 2
    ? "Add 2 points"
    : points.length === 2
      ? "Similarity transform"
      : "Affine transform"
  const editing = mode === "edit"
  const busy = savingPoint || calculating || savingConfig || applyingDirect
  const canCalculate = editing && points.length >= 2 && !draft && !busy
  const canSave = editing && (calculation?.status === "calculated" || calculation?.is_georeferenced || Boolean(previewCoordinates)) && !draft && !busy

  function invalidateCalculation() {
    setCalculation(null)
    setOverlayEnabled(false)
    if (savedConfig) {
      setSavedConfig((current) => current ? { ...current, status: "pending", reusable: false } : current)
    }
  }

  function selectPoint(pointId) {
    setSelectedId(pointId)
    setNotice(null)
  }

  function handleSourceCreate(coordinates) {
    if (!editing) return
    setError(null)
    setNotice(null)
    if (points.length >= 10) {
      setError({ message: "Too many control points were added.", details: "The visual georeferencer supports a maximum of 10 control points." })
      return
    }
    if (!controlPointPlacement) {
      setNotice("Select Add Control Point before choosing a source location.")
      return
    }
    setDraft(coordinates)
  }

  async function pairDraftLocation(location, { flyToMap = false } = {}) {
    if (!editing || pairingRef.current) return
    setError(null)
    if (!draft) {
      setNotice("Click the corresponding location on the source image first.")
      return
    }
    pairingRef.current = true
    setSavingPoint(true)
    try {
      const created = await createControlPoint(id, {
        ...draft,
        longitude: location.lng,
        latitude: location.lat,
      })
      setPoints((current) => [...current, created])
      setSelectedId(created.id)
      setDraft(null)
      invalidateCalculation()
      if (flyToMap) {
        setFocusTarget({
          latitude: created.latitude,
          longitude: created.longitude,
          zoom: 17,
          revision: Date.now(),
        })
      }
      setNotice(`Point ${points.length + 1} paired at Lat ${created.latitude.toFixed(6)}, Lng ${created.longitude.toFixed(6)}. Add another point or calculate.`)
    } catch (requestError) {
      setError(readableApiError(requestError, "Unable to save the control point."))
    } finally {
      pairingRef.current = false
      setSavingPoint(false)
    }
  }

  function handleMapClick(location) {
    return pairDraftLocation(location, { flyToMap: false })
  }

  function handlePairDraftByInput(location) {
    return pairDraftLocation(location, { flyToMap: true })
  }

  function submitDraftBannerCoordinates(event) {
    event.preventDefault()
    if (!draft || busy) return
    const parsed = parseGoogleMapsCoordinates(draftBannerCoord, { order: "lat-lng" })
    if (!parsed.ok) {
      setError({ message: "Invalid Google Maps coordinates", details: parsed.error })
      return
    }
    pairDraftLocation({ lat: parsed.latitude, lng: parsed.longitude }, { flyToMap: true })
  }

  function cancelControlPoint() {
    setDraft(null)
    setControlPointPlacement(false)
    setNotice("Control point placement cancelled.")
  }

  function applyPointChange(pointId, patch) {
    setPoints((current) => current.map((point) => (point.id === pointId ? { ...point, ...patch } : point)))
  }

  async function persistPointChange(pointId, patch) {
    if (!editing) return
    const previous = pointsRef.current.find((point) => point.id === pointId)
    if (!previous) return
    const nextPoint = { ...previous, ...patch }
    applyPointChange(pointId, patch)
    setError(null)
    try {
      const saved = await updateControlPoint(pointId, {
        image_x: nextPoint.image_x,
        image_y: nextPoint.image_y,
        longitude: nextPoint.longitude,
        latitude: nextPoint.latitude,
      })
      setPoints((current) => current.map((point) => (point.id === pointId ? saved : point)))
      invalidateCalculation()
      if (patch.latitude !== undefined || patch.longitude !== undefined) {
        setFocusTarget({
          latitude: saved.latitude,
          longitude: saved.longitude,
          zoom: 17,
          revision: Date.now(),
        })
      }
    } catch (requestError) {
      applyPointChange(pointId, previous)
      setError(readableApiError(requestError, "Unable to update the control point."))
    }
  }

  function handleSourceMove(pointId, coordinates) {
    if (editing) applyPointChange(pointId, coordinates)
  }

  async function handleDelete(pointId) {
    if (!editing) return
    const previous = pointsRef.current
    const removed = previous.find((point) => point.id === pointId)
    if (!removed) return
    setPoints(previous.filter((point) => point.id !== pointId))
    if (selectedId === pointId) setSelectedId(previous.find((point) => point.id !== pointId)?.id || null)
    try {
      await removeControlPoint(pointId)
      invalidateCalculation()
    } catch (requestError) {
      setPoints(previous)
      setError(readableApiError(requestError, "Unable to delete the control point."))
    }
  }

  async function handleUndo() {
    if (!editing) return
    if (draft) {
      setDraft(null)
      return
    }
    const lastPoint = pointsRef.current.at(-1)
    if (lastPoint) await handleDelete(lastPoint.id)
  }

  async function handleReset() {
    if (!editing || (!points.length && !draft)) return
    if (!window.confirm("Clear all control points for this dataset?")) return
    const previous = pointsRef.current
    setDraft(null)
    setPoints([])
    setSelectedId(null)
    try {
      await Promise.all(previous.map((point) => removeControlPoint(point.id)))
      invalidateCalculation()
    } catch (requestError) {
      setPoints(previous)
      setError(readableApiError(requestError, "Unable to clear all control points."))
    }
  }

  async function handleCalculate() {
    setCalculating(true)
    setError(null)
    setNotice(null)
    try {
      const result = await georeferenceDataset(id, "calculate")
      setCalculation(result)
      setOverlayEnabled(true)
      if (Array.isArray(result.maplibre_bounds) && result.maplibre_bounds.length === 4) {
        setFocusTarget({
          bounds: result.maplibre_bounds,
          revision: Date.now(),
        })
      }
      const article = result.transformation === "affine" ? "an" : "a"
      const count = result.control_point_count || points.length
      setNotice(`${count} points calculated with ${article} ${result.transformation} transform. Review the preview, then save.`)
    } catch (requestError) {
      setError(readableApiError(requestError, "Unable to calculate the georeference."))
    } finally {
      setCalculating(false)
    }
  }

  async function handleDirectCoordinateSubmit(event) {
    event.preventDefault()
    if (busy) return
    setDirectError(null)
    setError(null)
    setNotice(null)

    if (!imageSize) {
      setDirectError("Source image dimensions are still loading. Wait a moment and try again.")
      return
    }

    let buildResult
    if (directMode === "corners") {
      const parsedTL = parseGoogleMapsCoordinates(directTopLeftInput, { order: "lat-lng" })
      if (!parsedTL.ok) {
        setDirectError(`Top-Left: ${parsedTL.error}`)
        return
      }
      const parsedBR = parseGoogleMapsCoordinates(directBottomRightInput, { order: "lat-lng" })
      if (!parsedBR.ok) {
        setDirectError(`Bottom-Right: ${parsedBR.error}`)
        return
      }
      buildResult = buildDirectCoordinateControlPoints({
        mode: "corners",
        width: imageSize.width,
        height: imageSize.height,
        topLeft: { latitude: parsedTL.latitude, longitude: parsedTL.longitude },
        bottomRight: { latitude: parsedBR.latitude, longitude: parsedBR.longitude },
      })
    } else {
      const parsedCenter = parseGoogleMapsCoordinates(directCenterInput, { order: "lat-lng" })
      if (!parsedCenter.ok) {
        setDirectError(parsedCenter.error)
        return
      }
      buildResult = buildDirectCoordinateControlPoints({
        mode: "center",
        width: imageSize.width,
        height: imageSize.height,
        center: { latitude: parsedCenter.latitude, longitude: parsedCenter.longitude },
        widthMeters: Number(directWidthMeters),
        rotationDegrees: Number(directRotationDeg) || 0,
      })
    }

    if (!buildResult.ok) {
      setDirectError(buildResult.error)
      return
    }

    setApplyingDirect(true)
    setMode("edit")
    setDraft(null)
    setControlPointPlacement(false)

    try {
      const existing = pointsRef.current
      if (existing.length > 0) {
        await Promise.all(existing.map((pt) => removeControlPoint(pt.id)))
      }

      const createdPoints = []
      for (const candidate of buildResult.points) {
        const created = await createControlPoint(id, candidate)
        createdPoints.push(created)
      }

      setPoints(createdPoints)
      setSelectedId(createdPoints[0]?.id || null)

      const calcResult = await georeferenceDataset(id, "calculate")
      setCalculation(calcResult)
      setOverlayEnabled(true)

      if (Array.isArray(calcResult.maplibre_bounds) && calcResult.maplibre_bounds.length === 4) {
        setFocusTarget({
          bounds: calcResult.maplibre_bounds,
          revision: Date.now(),
        })
      } else {
        setFocusTarget({
          latitude: buildResult.center.latitude,
          longitude: buildResult.center.longitude,
          zoom: 17,
          revision: Date.now(),
        })
      }

      setNotice(
        `Map positioned at Google Maps Lat ${buildResult.center.latitude.toFixed(6)}, Lng ${buildResult.center.longitude.toFixed(6)}. Review the preview overlay, then click Save Georeference.`,
      )
    } catch (requestError) {
      try {
        const synced = await listControlPoints(id)
        setPoints(synced)
      } catch {
        // Keep current list if refresh fails
      }
      setError(readableApiError(requestError, "Unable to apply Google Maps coordinates."))
    } finally {
      setApplyingDirect(false)
    }
  }

  async function handleSave() {
    setSavingConfig(true)
    setError(null)
    try {
      setNotice("Saving georeference configuration...")
      const result = await georeferenceDataset(id, "save")
      setCalculation(result)
      setSavedConfig(result)
      setMode("view")
      setDirectPanelOpen(false)
      setNotice("Georeference saved and published to the main map.")
      navigate(returnTo, { replace: true })
    } catch (requestError) {
      setError(readableApiError(requestError, "Unable to save and publish the georeference."))
    } finally {
      setSavingConfig(false)
    }
  }

  function handleReuse() {
    if (!savedConfig?.reusable) return
    setCalculation(savedConfig)
    setOverlayEnabled(true)
    setMode("view")
    setError(null)
    setNotice("Saved control points and transformation parameters restored for inspection.")
  }

  if (loading) return <p className="page-message">Loading visual georeferencer…</p>
  if (!dataset) {
    return (
      <main className="min-h-[calc(100vh-44px)] bg-parchment px-5 py-20">
        <div className="error-panel mx-auto max-w-2xl" role="alert">
          <p className="font-semibold">{error?.message || "Dataset unavailable"}</p>
          <p className="mt-2 text-[14px] text-muted">{error?.details}</p>
          <div className="mt-5 flex items-center gap-4">
            {returnTo && <Link className="text-action" to={returnTo}>Back to workspace</Link>}
            <Link className="text-action" to="/">Back to datasets</Link>
          </div>
        </div>
      </main>
    )
  }

  return (
    <main className="min-h-[calc(100vh-44px)] bg-parchment p-3 phone:p-5">
      <div className="mx-auto flex max-w-canvas flex-col gap-3">
        <header className="flex min-h-[72px] flex-wrap items-center gap-2.5 rounded-[18px] border border-[#e0e0e0] bg-[#ffffff] px-4 py-3 sm:px-6 shadow-xs">
          <Link
            className="inline-flex items-center gap-1.5 text-xs text-[#7a7a7a] hover:text-[#0066cc] font-medium transition-colors shrink-0 mr-2 group"
            to={returnTo}
          >
            <ArrowLeft className="size-3.5 group-hover:-translate-x-0.5 transition-transform" />
            <span className="hidden sm:inline">Back to workspace</span>
            <span className="sm:hidden">Back</span>
          </Link>
          <h1 className="min-w-[160px] flex-1 text-xl sm:text-2xl font-semibold tracking-tight text-[#1d1d1f]">
            Visual Georeferencer
          </h1>
          <div className="min-w-[150px] text-right text-xs text-[#7a7a7a]">
            <p className="font-semibold text-[#1d1d1f]">
              {points.length} {points.length === 1 ? "point" : "points"} · {transformLabel}
            </p>
            <p className="mt-0.5 text-[11px]">Suggested: {recommendation(imageSize)}</p>
          </div>
          {savedConfig && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="shrink-0 rounded-full border-[#e0e0e0] text-xs"
              disabled={!savedConfig.reusable || busy}
              onClick={handleReuse}
            >
              Reuse Config
            </Button>
          )}
          {savedConfig && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="shrink-0 rounded-full border-[#e0e0e0] text-xs"
              disabled={editing || busy}
              onClick={() => {
                setMode("edit")
                setNotice("Editing enabled. Recalculate after changing control points.")
              }}
            >
              Edit Georeferencing
            </Button>
          )}
          <Button
            type="button"
            variant="outline"
            size="sm"
            className={`shrink-0 rounded-full border-[#e0e0e0] text-xs ${
              directPanelOpen ? "border-[#0066cc] bg-[#eaf3ff] text-[#0052a3]" : ""
            }`}
            disabled={busy}
            onClick={() => {
              if (!editing) setMode("edit")
              setDirectPanelOpen((prev) => !prev)
              setDirectError(null)
            }}
          >
            <MapPin className="mr-1 size-3.5 text-[#0066cc]" />
            Place by Lat, Lng
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="shrink-0 rounded-full border-[#e0e0e0] text-xs"
            disabled={!editing || busy || Boolean(draft)}
            onClick={() => {
              setControlPointPlacement(true)
              setNotice("Click the source image, then click the Reference Map or enter Google Maps Lat, Lng.")
            }}
          >
            {controlPointPlacement ? "Adding Point…" : "Add Control Point"}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="shrink-0 rounded-full border-[#e0e0e0] text-xs"
            disabled={!canCalculate}
            onClick={handleCalculate}
          >
            {calculating ? "Calculating…" : "Calculate"}
          </Button>
          <Button
            type="button"
            variant="pill"
            size="sm"
            className="shrink-0 bg-[#0066cc] hover:bg-[#0071e3] text-white text-xs font-normal"
            disabled={!canSave}
            onClick={handleSave}
          >
            {savingConfig ? "Saving…" : "Save Georeference"}
          </Button>
          <EndSessionDialog />
        </header>

        {directPanelOpen && (
          <section
            aria-label="Place map directly by Google Maps coordinates"
            className="rounded-[18px] border border-[#cfe2ff] bg-[#f8fbff] p-4 shadow-xs"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h2 className="text-[13px] font-semibold tracking-[0.04em] text-[#1d1d1f]">
                  PLACE BY GOOGLE MAPS COORDINATES (LAT, LNG)
                </h2>
                <p className="mt-0.5 text-[12px] text-[#555]">
                  Paste coordinates directly from Google Maps (decimal{" "}
                  <code className="rounded bg-white px-1 py-0.5 text-[11px]">28.6139, 77.2090</code>, DMS{" "}
                  <code className="rounded bg-white px-1 py-0.5 text-[11px]">28°36&apos;50.2&quot;N 77°12&apos;32.5&quot;E</code>, full Plus Code, or Google Maps URL).
                </p>
              </div>
              <div className="flex items-center gap-2">
                <div className="inline-flex rounded-full border border-[#d6e4f7] bg-white p-0.5 text-[12px]">
                  <button
                    type="button"
                    onClick={() => {
                      setDirectMode("center")
                      setDirectError(null)
                    }}
                    className={`rounded-full px-3 py-1 transition-colors ${
                      directMode === "center"
                        ? "bg-[#0066cc] font-medium text-white"
                        : "text-[#555] hover:text-[#1d1d1f]"
                    }`}
                  >
                    Center Lat, Lng + Width
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setDirectMode("corners")
                      setDirectError(null)
                    }}
                    className={`rounded-full px-3 py-1 transition-colors ${
                      directMode === "corners"
                        ? "bg-[#0066cc] font-medium text-white"
                        : "text-[#555] hover:text-[#1d1d1f]"
                    }`}
                  >
                    Two Corners (NW &amp; SE)
                  </button>
                </div>
                <button
                  type="button"
                  aria-label="Close direct coordinate panel"
                  className="inline-flex size-7 items-center justify-center rounded-full text-[#7a7a7a] hover:bg-white hover:text-[#1d1d1f]"
                  onClick={() => setDirectPanelOpen(false)}
                >
                  <X className="size-4" />
                </button>
              </div>
            </div>

            <form className="mt-3 grid gap-3" onSubmit={handleDirectCoordinateSubmit}>
              {directMode === "center" ? (
                <div className="grid gap-3 tablet:grid-cols-[minmax(0,1.8fr)_minmax(140px,0.7fr)_minmax(130px,0.6fr)_auto] tablet:items-end">
                  <label className="flex flex-col gap-1 text-[12px] font-medium text-[#1d1d1f]">
                    <span>Center Latitude, Longitude (Google Maps)</span>
                    <Input
                      value={directCenterInput}
                      placeholder="e.g. 28.613939, 77.209021"
                      aria-label="Center Google Maps latitude and longitude"
                      className="h-9 rounded-lg bg-white text-[13px]"
                      onChange={(event) => {
                        setDirectCenterInput(event.target.value)
                        if (directError) setDirectError(null)
                      }}
                    />
                  </label>
                  <label className="flex flex-col gap-1 text-[12px] font-medium text-[#1d1d1f]">
                    <span>
                      Ground Width (m)
                      {aspectMatchedHeightMeters ? (
                        <span className="ml-1 font-normal text-[#7a7a7a]">
                          · H ≈ {aspectMatchedHeightMeters}m
                        </span>
                      ) : null}
                    </span>
                    <Input
                      type="number"
                      min="1"
                      step="any"
                      value={directWidthMeters}
                      aria-label="Map ground width in meters"
                      className="h-9 rounded-lg bg-white text-[13px]"
                      onChange={(event) => setDirectWidthMeters(event.target.value)}
                    />
                  </label>
                  <label className="flex flex-col gap-1 text-[12px] font-medium text-[#1d1d1f]">
                    <span>Rotation (° CW)</span>
                    <Input
                      type="number"
                      step="any"
                      value={directRotationDeg}
                      aria-label="Map clockwise rotation in degrees"
                      className="h-9 rounded-lg bg-white text-[13px]"
                      onChange={(event) => setDirectRotationDeg(event.target.value)}
                    />
                  </label>
                  <Button
                    type="submit"
                    disabled={busy}
                    className="h-9 rounded-full bg-[#0066cc] px-5 text-xs font-medium text-white hover:bg-[#0071e3]"
                  >
                    {applyingDirect ? "Positioning…" : "Apply & Preview"}
                  </Button>
                </div>
              ) : (
                <div className="grid gap-3 tablet:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] tablet:items-end">
                  <label className="flex flex-col gap-1 text-[12px] font-medium text-[#1d1d1f]">
                    <span>Top-Left (North-West) Lat, Lng</span>
                    <Input
                      value={directTopLeftInput}
                      placeholder="e.g. 28.614500, 77.208200"
                      aria-label="Top-left Google Maps latitude and longitude"
                      className="h-9 rounded-lg bg-white text-[13px]"
                      onChange={(event) => {
                        setDirectTopLeftInput(event.target.value)
                        if (directError) setDirectError(null)
                      }}
                    />
                  </label>
                  <label className="flex flex-col gap-1 text-[12px] font-medium text-[#1d1d1f]">
                    <span>Bottom-Right (South-East) Lat, Lng</span>
                    <Input
                      value={directBottomRightInput}
                      placeholder="e.g. 28.613300, 77.209800"
                      aria-label="Bottom-right Google Maps latitude and longitude"
                      className="h-9 rounded-lg bg-white text-[13px]"
                      onChange={(event) => {
                        setDirectBottomRightInput(event.target.value)
                        if (directError) setDirectError(null)
                      }}
                    />
                  </label>
                  <Button
                    type="submit"
                    disabled={busy}
                    className="h-9 rounded-full bg-[#0066cc] px-5 text-xs font-medium text-white hover:bg-[#0071e3]"
                  >
                    {applyingDirect ? "Positioning…" : "Apply & Preview"}
                  </Button>
                </div>
              )}
              {directError && (
                <p role="alert" className="text-[12px] font-medium text-[#b42318]">
                  {directError}
                </p>
              )}
            </form>
          </section>
        )}

        {savedConfig?.reuse_warning && <div className="rounded-utility border border-[#ebc36a] bg-[#fff8e6] px-4 py-3 text-[13px] text-ink" role="alert">{savedConfig.reuse_warning}</div>}
        {(error || notice || draft) && (
          <div className={`rounded-utility border px-4 py-3 text-[14px] ${error ? "border-[#e9b4b4] bg-[#fff6f6]" : "border-hairline bg-white"}`} role={error ? "alert" : "status"}>
            <p className="font-medium text-ink">{error?.message || (draft ? "Point pending map reference" : notice)}</p>
            <p className="mt-1 text-[12px] text-muted">
              {error?.details ||
                (draft
                  ? `Source pixel (${Math.round(draft.image_x)}, ${Math.round(draft.image_y)}) selected. Click the matching spot on the Reference Map or paste Google Maps Lat, Lng below:`
                  : "")}
            </p>
            {draft && (
              <form
                className="mt-3 flex flex-wrap items-center gap-2"
                onSubmit={submitDraftBannerCoordinates}
              >
                <Input
                  value={draftBannerCoord}
                  placeholder="Paste Google Maps Lat, Lng (e.g. 28.6139, 77.2090)"
                  aria-label="Google Maps latitude and longitude for pending point"
                  className="h-9 max-w-sm flex-1 rounded-full px-3.5 text-[12px]"
                  onChange={(event) => setDraftBannerCoord(event.target.value)}
                />
                <Button
                  type="submit"
                  size="sm"
                  disabled={busy}
                  className="h-9 rounded-full bg-[#0066cc] px-4 text-xs font-normal text-white hover:bg-[#0071e3]"
                >
                  Pair Point
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-9 rounded-full px-4 text-xs"
                  onClick={cancelControlPoint}
                >
                  Cancel pending point
                </Button>
              </form>
            )}
          </div>
        )}

        <section className="grid min-h-[500px] gap-3 tablet-landscape:grid-cols-2" aria-label="Visual georeferencing workspace">
          <ReferenceMap
            points={points}
            draft={draft}
            selectedId={selectedId}
            onSelect={selectPoint}
            onMapClick={handleMapClick}
            onMoveEnd={persistPointChange}
            focusTarget={focusTarget}
            readOnly={!editing}
            placementMode={controlPointPlacement}
            overlay={{
              visible: overlayEnabled && Boolean(previewCoordinates),
              imageUrl: sourceUrl,
              coordinates: previewCoordinates,
              opacity: overlayOpacity,
            }}
          />
          <SourceImagePane
            sourceUrl={sourceUrl}
            imageSize={imageSize}
            points={points}
            draft={draft}
            selectedId={selectedId}
            onSelect={selectPoint}
            onCreate={handleSourceCreate}
            onMove={handleSourceMove}
            onMoveEnd={persistPointChange}
            readOnly={!editing}
            placementMode={controlPointPlacement}
            onImageDimensions={setNaturalDimensions}
          />
        </section>

        <section className="grid gap-3 desktop:grid-cols-[minmax(0,1fr)_330px]">
          <ControlPointList
            points={points}
            draft={draft}
            selectedId={selectedId}
            onSelect={selectPoint}
            onUpdate={persistPointChange}
            onDelete={handleDelete}
            onPairDraft={handlePairDraftByInput}
            readOnly={!editing}
            busy={busy}
          />
          <aside className="flex flex-col gap-5 rounded-utility border border-hairline bg-white p-4">
            <div><p className="mb-3 text-[12px] font-semibold tracking-[0.08em] text-ink">SAVED CONFIGURATION</p><SavedConfiguration configuration={savedConfig} /></div>
            <div className="grid grid-cols-3 gap-2"><Button type="button" variant="outline" onClick={handleUndo} disabled={!editing || (!points.length && !draft)}>Undo</Button><Button type="button" variant="outline" onClick={() => selectedId && handleDelete(selectedId)} disabled={!editing || !selectedId}>Delete</Button><Button type="button" variant="outline" onClick={handleReset} disabled={!editing || (!points.length && !draft)}>Reset</Button></div>
            <div className="border-t border-hairline pt-4"><div className="flex items-center justify-between gap-4"><label className="flex min-h-11 items-center gap-3 text-[14px] text-ink"><input type="checkbox" className="h-5 w-5 accent-primary" checked={overlayEnabled} disabled={!previewCoordinates} onChange={(event) => setOverlayEnabled(event.target.checked)} />Preview overlay</label><span className="text-[12px] text-muted">{previewCoordinates ? "Ready" : "Calculate first"}</span></div><div className="mt-3 flex items-center gap-3"><label htmlFor="georeference-opacity" className="shrink-0 text-[14px] text-muted">Opacity</label><input id="georeference-opacity" className="h-11 min-w-0 flex-1" type="range" min="0" max="100" disabled={!overlayEnabled || !previewCoordinates} value={Math.round(overlayOpacity * 100)} onChange={(event) => setOverlayOpacity(Number(event.target.value) / 100)} /><output className="w-9 text-right text-[13px] tabular-nums">{Math.round(overlayOpacity * 100)}%</output></div></div>
            <p className="text-[12px] leading-5 text-muted">Google Maps convention: Lat, Lng (WGS84). Source convention: x = image_x, y = -image_y. Two points use conformal similarity; three to ten use affine.</p>
          </aside>
        </section>
      </div>
    </main>
  )
}
