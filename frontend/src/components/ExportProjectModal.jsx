import React, { useEffect, useState } from "react"
import {
  X,
  Globe,
  Copy,
  Check,
  Trash2,
  Loader2,
  AlertCircle,
} from "lucide-react"
import {
  publishWebTileMap,
  listPublishedMaps,
  revokePublishedMap,
  API_URL,
} from "../api"

function toAbsoluteUrl(pathOrUrl, baseOrigin = API_URL, token = "") {
  if (!pathOrUrl) return ""
  let url = /^https?:\/\//i.test(pathOrUrl)
    ? pathOrUrl
    : `${(baseOrigin || window.location.origin).replace(/\/+$/, "")}${pathOrUrl.startsWith("/") ? "" : "/"}${pathOrUrl}`
  if (token && !/[?&]token=/.test(url)) {
    url += `${url.includes("?") ? "&" : "?"}token=${encodeURIComponent(token)}`
  }
  return url
}

export default function ExportProjectModal({
  open,
  onClose,
  manifest,
  datasets = [],
  gisLayers = [],
  primaryDatasetId,
  workspaceId = null,
  error = null,
}) {
  const [publishedMaps, setPublishedMaps] = useState([])
  const [lanOrigin, setLanOrigin] = useState("")
  const [loadingPublished, setLoadingPublished] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [publishError, setPublishError] = useState("")
  const [copiedKey, setCopiedKey] = useState("")

  // Load active published Custom Web Tile URLs when modal opens
  useEffect(() => {
    if (!open) return
    let active = true
    setPublishError("")
    setLoadingPublished(true)
    listPublishedMaps(workspaceId || primaryDatasetId || "")
      .then((res) => {
        if (!active) return
        setPublishedMaps(res?.published_maps || [])
        if (res?.lan_origin) setLanOrigin(res.lan_origin)
      })
      .catch(() => {
        if (active) setPublishedMaps([])
      })
      .finally(() => {
        if (active) setLoadingPublished(false)
      })
    return () => {
      active = false
    }
  }, [open, workspaceId, primaryDatasetId])

  if (!open) return null

  const primaryDs = datasets.find((d) => d.id === primaryDatasetId)
  const projectName = manifest?.name || primaryDs?.name || "Veracity Project"

  const copyUrl = (key, text) => {
    if (!text) return
    if (navigator?.clipboard?.writeText) {
      navigator.clipboard.writeText(text).catch(() => undefined)
    }
    setCopiedKey(key)
    setTimeout(() => setCopiedKey(""), 2500)
  }

  const handlePublishWebTile = async () => {
    if (publishing) return
    setPublishing(true)
    setPublishError("")
    try {
      const created = await publishWebTileMap({
        workspace_id: workspaceId || primaryDatasetId,
        project_id: workspaceId || primaryDatasetId,
        dataset_id: primaryDatasetId,
        name: projectName,
        mode: "combined",
        is_public: true,
      })
      const pubId = created?.map_id || created?.id
      const effectiveOrigin = created?.lan_origin || lanOrigin || API_URL
      if (created?.lan_origin) setLanOrigin(created.lan_origin)
      const compositeUrl =
        created?.composite_tile_url ||
        toAbsoluteUrl(`/tiles/${pubId}/composite/{0}/{1}/{2}.png`, effectiveOrigin)
      if (compositeUrl) {
        copyUrl(`${pubId}-composite`, compositeUrl)
      }
      setPublishedMaps((prev) => [created, ...prev.filter((p) => (p.map_id || p.id) !== pubId)])
    } catch (err) {
      setPublishError(
        err?.response?.data?.error?.details ||
          err?.response?.data?.error?.message ||
          "Failed to generate Custom Web Tile URL."
      )
    } finally {
      setPublishing(false)
    }
  }

  const handleRevoke = async (pubId) => {
    try {
      setPublishError("")
      await revokePublishedMap(pubId)
      setPublishedMaps((prev) => prev.filter((p) => (p.map_id || p.id) !== pubId))
    } catch (err) {
      setPublishError(err?.response?.data?.error?.message || "Failed to revoke URL.")
    }
  }

  const hasContent =
    Boolean(primaryDatasetId) ||
    (manifest?.members?.length || 0) > 0 ||
    datasets.length > 0 ||
    gisLayers.length > 0

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-xs animate-in fade-in duration-150">
      <div
        className="relative w-full max-w-lg flex flex-col bg-white dark:bg-[#1a1a1a] text-neutral-900 dark:text-white rounded-[12px] shadow-2xl border border-neutral-200 dark:border-[#2e2e2e] overflow-hidden"
        role="dialog"
        aria-modal="true"
        aria-labelledby="export-modal-title"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-neutral-200 dark:border-[#2a2a2a] bg-neutral-50 dark:bg-[#151515]">
          <div className="flex items-center gap-2.5">
            <div className="flex items-center justify-center size-9 rounded-[8px] bg-sky-500/10 text-sky-500 border border-sky-500/20">
              <Globe className="size-4" />
            </div>
            <div>
              <h2 id="export-modal-title" className="text-sm font-bold text-neutral-900 dark:text-white">
                Export Custom Web Tile
              </h2>
              <p className="text-[11px] text-neutral-500 dark:text-neutral-400">
                Publish all rasters and vectors as a Custom Web Tile URL
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#252525] rounded-[6px] transition-colors cursor-pointer"
            aria-label="Close export modal"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Body */}
        <div className="p-5 space-y-4 max-h-[75vh] overflow-y-auto">
          {(error || publishError) && (
            <div className="p-3 rounded-[8px] bg-red-500/10 border border-red-500/30 text-xs text-red-600 dark:text-red-400 flex items-center gap-2">
              <AlertCircle className="size-4 shrink-0" />
              <span>{publishError || error}</span>
            </div>
          )}

          <div className="p-4 rounded-[10px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50/70 dark:bg-[#202020] space-y-3">
            <div className="flex items-center gap-2">
              <Globe className="size-4 text-sky-500 shrink-0" />
              <h3 className="text-xs font-bold text-neutral-900 dark:text-white">
                Custom Web Tile
              </h3>
            </div>
            <p className="text-[11px] text-neutral-600 dark:text-neutral-400 leading-relaxed">
              Generate a shareable Custom Web Tile URL (<span className="font-mono">{"{0}/{1}/{2}.png"}</span>) combining all raster and vector layers in this project.
            </p>

            <button
              type="button"
              disabled={publishing || !hasContent}
              onClick={handlePublishWebTile}
              className="w-full flex items-center justify-center gap-2 py-2 px-3 rounded-[6px] text-xs font-bold bg-[#3ecf8e] hover:bg-[#34b77c] text-[#141414] disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed transition-colors"
            >
              {publishing ? (
                <>
                  <Loader2 className="size-3.5 animate-spin" />
                  <span>Generating URL…</span>
                </>
              ) : (
                <>
                  <Globe className="size-3.5" />
                  <span>Generate Custom Web Tile URL</span>
                </>
              )}
            </button>

            {loadingPublished && (
              <div className="text-[11px] text-neutral-400 flex items-center gap-1.5">
                <Loader2 className="size-3 animate-spin" /> Loading published URLs…
              </div>
            )}

            {!loadingPublished && publishedMaps.length > 0 && (
              <div className="space-y-2 pt-1">
                {publishedMaps.map((pub) => {
                  const pubId = pub.map_id || pub.id
                  const accessToken = !pub.is_public ? pub.access_token || "" : ""
                  const effectiveOrigin = pub.lan_origin || lanOrigin || API_URL
                  const compositeUrl = toAbsoluteUrl(
                    pub.composite_tile_url || `/tiles/${pubId}/composite/{0}/{1}/{2}.png`,
                    effectiveOrigin,
                    accessToken
                  )

                  return (
                    <div
                      key={pubId}
                      className="flex items-center justify-between gap-2 px-2.5 py-2 rounded-[6px] bg-white dark:bg-[#161616] border border-neutral-200 dark:border-[#2c2c2c] text-[11px]"
                    >
                      <span className="font-mono text-neutral-900 dark:text-neutral-200 truncate select-all">
                        {compositeUrl}
                      </span>
                      <div className="flex items-center gap-1 shrink-0">
                        <button
                          type="button"
                          onClick={() => copyUrl(`${pubId}-composite`, compositeUrl)}
                          className="px-2 py-1 rounded bg-[#3ecf8e]/15 hover:bg-[#3ecf8e]/25 text-[#14532d] dark:text-[#3ecf8e] text-[10px] font-semibold flex items-center gap-1 cursor-pointer"
                        >
                          {copiedKey === `${pubId}-composite` ? <Check className="size-3" /> : <Copy className="size-3" />}
                          <span>{copiedKey === `${pubId}-composite` ? "Copied" : "Copy"}</span>
                        </button>
                        <button
                          type="button"
                          onClick={() => handleRevoke(pubId)}
                          className="p-1 rounded bg-red-500/10 hover:bg-red-500/20 text-red-500 cursor-pointer"
                          title="Revoke URL"
                        >
                          <Trash2 className="size-3" />
                        </button>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </div>

        {/* Footer */}
        <div className="flex justify-end px-5 py-3 border-t border-neutral-200 dark:border-[#2a2a2a] bg-neutral-50 dark:bg-[#151515]">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-1.5 text-xs font-semibold text-neutral-700 dark:text-neutral-300 hover:bg-neutral-200 dark:hover:bg-[#252525] rounded-[6px] transition-colors cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
