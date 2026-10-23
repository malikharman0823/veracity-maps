import React, { useState } from "react"
import {
  FileText,
  CheckSquare,
  Square,
  Layers,
  ArrowRight,
  X,
  Loader2,
  AlertCircle,
  Building,
  RotateCw,
  Maximize2,
} from "lucide-react"
import { importPdfPages, pdfThumbnailUrl, readableApiError } from "../api"
import { Button } from "@/components/ui/button"

export default function ImportPdfModal({
  pdfInspection,
  onClose,
  onImportSuccess,
}) {
  const documentId = pdfInspection?.document_id
  const filename = pdfInspection?.filename || "document.pdf"
  const pageCount = pdfInspection?.page_count || 0
  const pages = pdfInspection?.pages || []

  // Default selection: select all pages
  const [selectedPages, setSelectedPages] = useState(() =>
    pages.map((p) => p.page_number)
  )
  const [georefMode, setGeorefMode] = useState("shared") // "shared" | "independent"
  const [buildingName, setBuildingName] = useState(
    pdfInspection?.suggested_building || "Building 1"
  )
  const [startingFloor, setStartingFloor] = useState(0)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState(null)

  const selectedCount = selectedPages.length

  function togglePage(pageNum) {
    setSelectedPages((prev) =>
      prev.includes(pageNum)
        ? prev.filter((p) => p !== pageNum)
        : [...prev, pageNum].sort((a, b) => a - b)
    )
  }

  function selectAll() {
    setSelectedPages(pages.map((p) => p.page_number))
  }

  function deselectAll() {
    setSelectedPages([])
  }

  async function handleContinue() {
    if (selectedPages.length === 0) {
      setError("Please select at least one page to import.")
      return
    }

    setImporting(true)
    setError(null)
    try {
      const result = await importPdfPages({
        document_id: documentId,
        selected_pages: selectedPages,
        georeferencing_mode: georefMode,
        building_name: buildingName.trim() || "Building 1",
        starting_floor: parseInt(startingFloor, 10) || 0,
      })
      onImportSuccess(result)
    } catch (err) {
      setError(readableApiError(err, "Failed to import selected PDF pages."))
      setImporting(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 bg-black/50 backdrop-blur-xs flex items-center justify-center p-4 sm:p-6"
      role="dialog"
      aria-modal="true"
      aria-labelledby="import-pdf-title"
    >
      <div className="bg-[#ffffff] text-[#1d1d1f] w-full max-w-4xl max-h-[90vh] rounded-2xl shadow-2xl border border-[#e0e0e0] flex flex-col overflow-hidden animate-in fade-in zoom-in-95 duration-200">
        {/* Header */}
        <div className="px-6 py-4 border-b border-[#e0e0e0] flex items-center justify-between bg-[#fafafc]">
          <div className="flex items-center gap-3">
            <div className="size-10 rounded-xl bg-[#0066cc]/10 border border-[#0066cc]/20 flex items-center justify-center text-[#0066cc]">
              <FileText className="size-5" />
            </div>
            <div>
              <h2 id="import-pdf-title" className="text-lg font-semibold text-[#1d1d1f] tracking-tight">
                Import PDF
              </h2>
              <p className="text-xs text-[#7a7a7a] flex items-center gap-2 mt-0.5">
                <span className="font-medium text-[#1d1d1f]">{filename}</span>
                <span>•</span>
                <span>{pageCount} {pageCount === 1 ? "page" : "pages"} detected</span>
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={importing}
            className="size-8 rounded-full flex items-center justify-center text-[#7a7a7a] hover:text-[#1d1d1f] hover:bg-[#e8e8ed] transition-colors disabled:opacity-50"
            aria-label="Close dialog"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6">
          {error && (
            <div className="p-3.5 rounded-xl bg-red-50 border border-red-200 text-red-700 text-xs flex items-start gap-2.5">
              <AlertCircle className="size-4 shrink-0 mt-0.5" />
              <div>
                <strong>Import failed:</strong> {error}
              </div>
            </div>
          )}

          {/* Page Selection Controls */}
          <div className="flex flex-wrap items-center justify-between gap-3 pb-2 border-b border-[#f0f0f0]">
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold text-[#1d1d1f]">
                Select Pages to Display:
              </span>
              <span className="text-xs px-2 py-0.5 rounded-full bg-[#f0f4ff] text-[#0066cc] font-medium border border-[#0066cc]/20">
                {selectedCount} of {pageCount} selected
              </span>
            </div>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={selectAll}
                disabled={importing}
                className="h-7 text-xs px-2.5 rounded-lg border-[#e0e0e0] text-[#1d1d1f] hover:bg-[#fafafc]"
              >
                <CheckSquare className="size-3.5 mr-1 text-[#0066cc]" />
                Select All
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={deselectAll}
                disabled={importing}
                className="h-7 text-xs px-2.5 rounded-lg border-[#e0e0e0] text-[#7a7a7a] hover:bg-[#fafafc]"
              >
                <Square className="size-3.5 mr-1" />
                Deselect All
              </Button>
            </div>
          </div>

          {/* Page Thumbnail Grid */}
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-4 max-h-72 overflow-y-auto p-1 border border-[#e0e0e0] rounded-xl bg-[#fafafc]">
            {pages.map((page) => {
              const isSelected = selectedPages.includes(page.page_number)
              return (
                <div
                  key={page.page_number}
                  onClick={() => !importing && togglePage(page.page_number)}
                  className={`group relative rounded-xl border-2 transition-all p-2 flex flex-col cursor-pointer bg-[#ffffff] select-none ${
                    isSelected
                      ? "border-[#0066cc] ring-2 ring-[#0066cc]/20 shadow-xs"
                      : "border-[#e0e0e0] opacity-60 hover:opacity-100 hover:border-[#b0b0b0]"
                  }`}
                >
                  <div className="relative aspect-[3/4] bg-[#f5f5f7] rounded-lg overflow-hidden flex items-center justify-center border border-[#e8e8ed]">
                    <img
                      src={pdfThumbnailUrl(documentId, page.page_number)}
                      alt={`Page ${page.page_number}`}
                      className="w-full h-full object-contain pointer-events-none transition-opacity duration-200"
                      loading="eager"
                      onError={(e) => {
                        e.currentTarget.style.display = "none"
                        const fallback = e.currentTarget.parentElement?.querySelector(".pdf-fallback-icon")
                        if (fallback) fallback.style.display = "flex"
                      }}
                    />
                    <div className="pdf-fallback-icon hidden flex-col items-center justify-center p-3 text-center text-[#7a7a7a]">
                      <FileText className="size-8 text-[#0066cc]/60 mb-1" />
                      <span className="text-[11px] font-medium text-[#1d1d1f]">Page {page.page_number}</span>
                      <span className="text-[10px] text-[#7a7a7a]">{page.width} × {page.height} px</span>
                    </div>
                    <div className="absolute top-2 left-2 z-10">
                      <div
                        className={`size-5 rounded flex items-center justify-center transition-colors shadow-xs ${
                          isSelected
                            ? "bg-[#0066cc] text-white"
                            : "bg-white/80 border border-[#b0b0b0] text-transparent"
                        }`}
                      >
                        ✓
                      </div>
                    </div>
                  </div>

                  <div className="mt-2 flex items-center justify-between text-[11px] text-[#7a7a7a]">
                    <span className="font-semibold text-[#1d1d1f]">
                      Page {page.page_number}
                    </span>
                    <span>
                      {page.width || page.width_pt ? `${page.width || Math.round(page.width_pt)} × ${page.height || Math.round(page.height_pt)}` : `Page ${page.page_number}`}
                    </span>
                  </div>
                  {page.rotation > 0 && (
                    <div className="text-[10px] text-[#0066cc] flex items-center gap-1 mt-0.5">
                      <RotateCw className="size-2.5" />
                      <span>{page.rotation}° rotation</span>
                    </div>
                  )}
                </div>
              )
            })}
          </div>

          {/* Georeferencing Mode Selection */}
          <div className="space-y-3">
            <h3 className="text-xs font-semibold text-[#1d1d1f] uppercase tracking-wider text-[#7a7a7a]">
              Georeferencing Mode
            </h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {/* Option A: Shared */}
              <label
                className={`p-4 rounded-xl border-2 transition-all cursor-pointer flex flex-col gap-1.5 ${
                  georefMode === "shared"
                    ? "border-[#0066cc] bg-[#f0f4ff]/40 ring-2 ring-[#0066cc]/10"
                    : "border-[#e0e0e0] hover:border-[#b0b0b0] bg-[#ffffff]"
                }`}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <input
                      type="radio"
                      name="georefMode"
                      value="shared"
                      checked={georefMode === "shared"}
                      onChange={() => setGeorefMode("shared")}
                      className="text-[#0066cc] focus:ring-[#0066cc]"
                    />
                    <span className="text-sm font-semibold text-[#1d1d1f]">
                      Georeference together (one-time)
                    </span>
                  </div>
                  <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-[#34c759]/10 text-[#248a3d] font-semibold border border-[#34c759]/20">
                    Recommended
                  </span>
                </div>
                <p className="text-xs text-[#7a7a7a] pl-5">
                  Align selected pages using one shared building coordinate system. Editing shared control points aligns all floor pages simultaneously.
                </p>
              </label>

              {/* Option B: Independent */}
              <label
                className={`p-4 rounded-xl border-2 transition-all cursor-pointer flex flex-col gap-1.5 ${
                  georefMode === "independent"
                    ? "border-[#0066cc] bg-[#f0f4ff]/40 ring-2 ring-[#0066cc]/10"
                    : "border-[#e0e0e0] hover:border-[#b0b0b0] bg-[#ffffff]"
                }`}
              >
                <div className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="georefMode"
                    value="independent"
                    checked={georefMode === "independent"}
                    onChange={() => setGeorefMode("independent")}
                    className="text-[#0066cc] focus:ring-[#0066cc]"
                  />
                  <span className="text-sm font-semibold text-[#1d1d1f]">
                    Georeference separately
                  </span>
                </div>
                <p className="text-xs text-[#7a7a7a] pl-5">
                  Align each selected page independently. Each page maintains its own individual control points, rotation, and bounding coordinates.
                </p>
              </label>
            </div>
          </div>

          {/* Building and Multi-Floor Assignment */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 p-4 rounded-xl bg-[#fafafc] border border-[#e0e0e0]">
            <div>
              <label className="text-xs font-medium text-[#1d1d1f] flex items-center gap-1.5 mb-1.5">
                <Building className="size-3.5 text-[#7a7a7a]" />
                Building Name
              </label>
              <input
                type="text"
                value={buildingName}
                onChange={(e) => setBuildingName(e.target.value)}
                disabled={importing}
                placeholder="e.g. Building A"
                className="w-full text-xs px-3 py-2 rounded-lg border border-[#e0e0e0] bg-[#ffffff] focus:outline-none focus:border-[#0066cc] focus:ring-1 focus:ring-[#0066cc]"
              />
              <p className="text-[11px] text-[#7a7a7a] mt-1">
                Pages will be grouped under this building structure.
              </p>
            </div>

            <div>
              <label className="text-xs font-medium text-[#1d1d1f] flex items-center gap-1.5 mb-1.5">
                <Layers className="size-3.5 text-[#7a7a7a]" />
                Starting Floor Level
              </label>
              <input
                type="number"
                value={startingFloor}
                onChange={(e) => setStartingFloor(e.target.value)}
                disabled={importing}
                placeholder="0"
                className="w-full text-xs px-3 py-2 rounded-lg border border-[#e0e0e0] bg-[#ffffff] focus:outline-none focus:border-[#0066cc] focus:ring-1 focus:ring-[#0066cc]"
              />
              <p className="text-[11px] text-[#7a7a7a] mt-1">
                Floor 0 = Ground Floor. Subsequent pages increment floor levels.
              </p>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="px-6 py-4 border-t border-[#e0e0e0] bg-[#fafafc] flex items-center justify-between">
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
            disabled={importing}
            className="rounded-full text-xs border-[#e0e0e0] text-[#1d1d1f] hover:bg-[#ffffff]"
          >
            Cancel
          </Button>

          <Button
            type="button"
            onClick={handleContinue}
            disabled={importing || selectedCount === 0}
            className="rounded-full text-xs bg-[#0066cc] hover:bg-[#0055b3] text-white px-5 shadow-xs"
          >
            {importing ? (
              <>
                <Loader2 className="size-3.5 animate-spin mr-1.5" />
                Rasterizing {selectedCount} {selectedCount === 1 ? "page" : "pages"}...
              </>
            ) : (
              <>
                Import {selectedCount} {selectedCount === 1 ? "Page" : "Pages"}
                <ArrowRight className="size-3.5 ml-1.5" />
              </>
            )}
          </Button>
        </div>
      </div>
    </div>
  )
}
