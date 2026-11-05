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
        className="relative w-full max-w-xl overflow-hidden rounded-[12px] border border-neutral-200 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-900 dark:text-white shadow-2xl transition-colors"
        role="dialog"
        aria-modal="true"
        aria-labelledby="file-preview-title"
      >
        <header className="flex items-start justify-between gap-4 border-b border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-5 py-4">
          <div className="min-w-0">
            <p className="text-[11px] font-semibold uppercase tracking-wider text-neutral-500 dark:text-neutral-400">Selected file</p>
            <h2 id="file-preview-title" className="mt-1 truncate text-base font-semibold text-neutral-900 dark:text-white" title={file.name}>
              {file.name}
            </h2>
          </div>
          <button type="button" onClick={onCancel} className="flex size-8 shrink-0 items-center justify-center rounded-[6px] text-neutral-500 hover:bg-neutral-200/50 dark:hover:bg-[#282828] hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-white cursor-pointer" aria-label="Close file preview">
            <X className="size-4" />
          </button>
        </header>

        <div className="p-5">
          <div className="flex min-h-72 items-center justify-center overflow-hidden rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#171717]">
            {previewKind === "image" && objectUrl ? (
              <img src={objectUrl} alt={`Preview of ${file.name}`} className="max-h-[55vh] w-full object-contain" />
            ) : (
              <div className="flex max-w-sm flex-col items-center px-8 py-12 text-center">
                <div className="mb-4 flex size-16 items-center justify-center rounded-[12px] border border-[#3ecf8e]/30 bg-[#3ecf8e]/10 text-[#24b47e] dark:text-[#3ecf8e]">
                  <File className="size-8" />
                </div>
                <p className="break-all text-sm font-semibold text-neutral-900 dark:text-white">{file.name}</p>
                <p className="mt-2 text-xs text-neutral-500 dark:text-neutral-400">This format has no browser image preview. Its live file details are shown below.</p>
              </div>
            )}
          </div>

          <dl className="mt-4 grid grid-cols-1 gap-2 text-xs sm:grid-cols-2">
            <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-3 py-2">
              <dt className="text-neutral-500 dark:text-neutral-400">Type</dt>
              <dd className="mt-0.5 truncate font-mono text-neutral-900 dark:text-white" title={getFileTypeLabel(file)}>{getFileTypeLabel(file)}</dd>
            </div>
            <div className="rounded-[8px] border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-3 py-2">
              <dt className="text-neutral-500 dark:text-neutral-400">Size</dt>
              <dd className="mt-0.5 font-mono text-neutral-900 dark:text-white">{fileSize(file.size)}</dd>
            </div>
          </dl>
        </div>

        <footer className="flex items-center justify-end gap-2 border-t border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] px-5 py-4">
          <button type="button" onClick={onCancel} className="rounded-[6px] border border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] px-4 py-2 text-xs font-medium text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-[#282828] cursor-pointer">
            Cancel
          </button>
          <button type="button" onClick={onConfirm} className="inline-flex items-center gap-1.5 rounded-[6px] bg-[#3ecf8e] hover:bg-[#24b47e] px-4 py-2 text-xs font-medium text-[#171717] shadow-xs cursor-pointer">
            {previewKind === "image" ? <ImageIcon className="size-3.5" /> : <File className="size-3.5" />}
            Use this file
          </button>
        </footer>
      </section>
    </div>
  )
}
