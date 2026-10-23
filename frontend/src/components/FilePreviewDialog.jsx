import React, { useEffect, useState } from "react"
import { File, Image as ImageIcon, X } from "lucide-react"

import { fileSize } from "./DatasetCard"
import { getFilePreviewKind, getFileTypeLabel } from "../lib/filePreview"

export default function FilePreviewDialog({ file, open, onCancel, onConfirm }) {
  const [objectUrl, setObjectUrl] = useState("")
  const previewKind = getFilePreviewKind(file)

  useEffect(() => {
    if (!open || !file || previewKind !== "image") {
      setObjectUrl("")
      return undefined
    }

    const nextUrl = URL.createObjectURL(file)
    setObjectUrl(nextUrl)
    return () => URL.revokeObjectURL(nextUrl)
  }, [file, open, previewKind])

  useEffect(() => {
    if (!open) return undefined
    const handleKeyDown = (event) => {
      if (event.key === "Escape") onCancel()
    }
    window.addEventListener("keydown", handleKeyDown)
    return () => window.removeEventListener("keydown", handleKeyDown)
  }, [onCancel, open])

  if (!open || !file) return null

  return (
    <div className="workspace-dialog-backdrop" role="presentation" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onCancel()
    }}>
      <section
        className="relative w-full max-w-xl overflow-hidden rounded-2xl border border-white/80 bg-white shadow-2xl"
        role="dialog"
        aria-modal="true"
        aria-labelledby="file-preview-title"
      >
        <header className="flex items-start justify-between gap-4 border-b border-[#e0e0e0] px-5 py-4">
          <div className="min-w-0">
            <p className="text-[11px] font-semibold uppercase tracking-wider text-[#7a7a7a]">Selected file</p>
            <h2 id="file-preview-title" className="mt-1 truncate text-base font-semibold text-[#1d1d1f]" title={file.name}>
              {file.name}
            </h2>
          </div>
          <button type="button" onClick={onCancel} className="flex size-8 shrink-0 items-center justify-center rounded-lg text-[#7a7a7a] hover:bg-[#f5f5f7] hover:text-[#1d1d1f]" aria-label="Close file preview">
            <X className="size-4" />
          </button>
        </header>

        <div className="p-5">
          <div className="flex min-h-72 items-center justify-center overflow-hidden rounded-xl border border-[#e0e0e0] bg-[#f5f5f7]">
            {previewKind === "image" && objectUrl ? (
              <img src={objectUrl} alt={`Preview of ${file.name}`} className="max-h-[55vh] w-full object-contain" />
            ) : (
              <div className="flex max-w-sm flex-col items-center px-8 py-12 text-center">
                <div className="mb-4 flex size-16 items-center justify-center rounded-2xl border border-[#dce8f5] bg-[#eaf3fc] text-[#0066cc]">
                  <File className="size-8" />
                </div>
                <p className="break-all text-sm font-semibold text-[#1d1d1f]">{file.name}</p>
                <p className="mt-2 text-xs text-[#7a7a7a]">This format has no browser image preview. Its live file details are shown below.</p>
              </div>
            )}
          </div>

          <dl className="mt-4 grid grid-cols-1 gap-2 text-xs sm:grid-cols-2">
            <div className="rounded-lg border border-[#e0e0e0] bg-[#fafafc] px-3 py-2">
              <dt className="text-[#7a7a7a]">Type</dt>
              <dd className="mt-0.5 truncate font-mono text-[#1d1d1f]" title={getFileTypeLabel(file)}>{getFileTypeLabel(file)}</dd>
            </div>
            <div className="rounded-lg border border-[#e0e0e0] bg-[#fafafc] px-3 py-2">
              <dt className="text-[#7a7a7a]">Size</dt>
              <dd className="mt-0.5 font-mono text-[#1d1d1f]">{fileSize(file.size)}</dd>
            </div>
          </dl>
        </div>

        <footer className="flex items-center justify-end gap-2 border-t border-[#e0e0e0] bg-[#fafafc] px-5 py-4">
          <button type="button" onClick={onCancel} className="rounded-full border border-[#e0e0e0] bg-white px-4 py-2 text-xs font-medium text-[#1d1d1f] hover:bg-[#f5f5f7]">
            Cancel
          </button>
          <button type="button" onClick={onConfirm} className="inline-flex items-center gap-1.5 rounded-full bg-[#0066cc] px-4 py-2 text-xs font-medium text-white hover:bg-[#0071e3]">
            {previewKind === "image" ? <ImageIcon className="size-3.5" /> : <File className="size-3.5" />}
            Use this file
          </button>
        </footer>
      </section>
    </div>
  )
}
