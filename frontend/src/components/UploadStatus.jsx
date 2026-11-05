import React from "react"
import { fileSize } from "./DatasetCard"
import { Button } from "@/components/ui/button"
import {
  Attachment,
  AttachmentMedia,
  AttachmentContent,
  AttachmentTitle,
  AttachmentDescription,
  AttachmentAction,
} from "@/components/ui/attachment"
import {
  CheckCircle2,
  AlertCircle,
  FileArchive,
  ArrowRight,
  Upload,
  RefreshCw,
  X,
  Loader2,
} from "lucide-react"

export default function UploadStatus({
  state,
  file,
  progress,
  result,
  error,
  onRemove,
  onReplace,
  onUpload,
  onRetry,
  onOpen,
}) {
  if (state === "success") {
    return (
      <div className="relative bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] p-8 sm:p-12 flex flex-col items-center justify-center text-center shadow-xs">
        <div className="size-14 rounded-[12px] bg-[#3ecf8e]/10 border border-[#3ecf8e]/30 flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e] mb-4">
          <CheckCircle2 className="size-7" />
        </div>
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-white mb-1">Upload complete</h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 max-w-sm mb-6 font-mono truncate">
          {result?.filename || file?.name || "Uploaded dataset"}
        </p>
        <Button
          type="button"
          size="default"
          onClick={onOpen}
          className="bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] text-xs font-semibold rounded-[6px] shadow-xs cursor-pointer"
        >
          <span>Continue</span>
          <ArrowRight className="size-3.5 ml-1.5" />
        </Button>
      </div>
    )
  }

  if (state === "error") {
    return (
      <div className="relative bg-white dark:bg-[#202020] border border-red-500/30 rounded-[12px] p-8 sm:p-12 flex flex-col items-center justify-center text-center shadow-xs">
        <div className="size-14 rounded-[12px] bg-red-500/10 border border-red-500/20 flex items-center justify-center text-red-500 mb-4">
          <AlertCircle className="size-7" />
        </div>
        <h3 className="text-lg font-semibold text-neutral-900 dark:text-white mb-1">Upload failed</h3>
        <p className="text-xs text-red-500 mt-1 max-w-sm font-semibold">{error?.message}</p>
        {error?.details && (
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-1 max-w-sm mb-6">{error.details}</p>
        )}
        <div className="flex gap-2.5 mt-4">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onRetry}
            className="rounded-[6px] border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 text-xs"
          >
            <RefreshCw className="size-3.5 mr-1.5" />
            <span>Try again</span>
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onRemove}
            className="rounded-[6px] text-red-500 hover:bg-red-500/10 text-xs"
          >
            Cancel
          </Button>
        </div>
      </div>
    )
  }

  if (!file && !result) {
    return null
  }

  const fileName = file?.name || result?.filename || "Selected file"
  const formattedSize = file?.size != null ? fileSize(file.size) : (result?.file_size != null ? fileSize(result.file_size) : "")

  return (
    <div className="relative bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] rounded-[12px] p-6 sm:p-8 flex flex-col justify-between min-h-[320px] shadow-xs">
      <div>
        <div className="flex items-center justify-between mb-4">
          <span className="inline-flex items-center px-2.5 py-1 rounded-[4px] text-[11px] font-mono border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-100 dark:bg-[#171717] text-neutral-800 dark:text-neutral-200">
            {state === "uploading" ? "UPLOADING CHUNKS" : "READY TO INGEST"}
          </span>
          {formattedSize ? (
            <span className="text-xs text-neutral-500 dark:text-neutral-400 font-mono">{formattedSize}</span>
          ) : null}
        </div>

        {/* Shadcn Attachment Component */}
        <div className="mt-3">
          <Attachment
            state={state === "uploading" ? "uploading" : "done"}
            size="default"
            className="w-full border border-neutral-200 dark:border-[#2e2e2e] bg-neutral-50 dark:bg-[#171717] rounded-[10px] p-3 flex items-center gap-3"
          >
            <AttachmentMedia className="size-11 rounded-[8px] bg-white dark:bg-[#202020] border border-neutral-200 dark:border-[#2e2e2e] text-[#24b47e] dark:text-[#3ecf8e]">
              {state === "uploading" ? (
                <Loader2 className="size-5 animate-spin" />
              ) : (
                <FileArchive className="size-5" />
              )}
            </AttachmentMedia>
            <AttachmentContent className="min-w-0 flex-1">
              <AttachmentTitle className="text-xs font-semibold text-neutral-900 dark:text-white truncate">
                {fileName}
              </AttachmentTitle>
              <AttachmentDescription className="text-[11px] text-neutral-500 dark:text-neutral-400">
                {formattedSize ? `${formattedSize} • ` : ""}Ready for spatial ingestion
              </AttachmentDescription>
            </AttachmentContent>
            {state !== "uploading" && (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={onRemove}
                className="size-8 rounded-[6px] text-neutral-400 hover:text-red-500"
                title="Remove file"
              >
                <X className="size-4" />
              </Button>
            )}
          </Attachment>
        </div>
      </div>

      {state === "uploading" ? (
        <div className="mt-8 space-y-2.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-neutral-500 dark:text-neutral-400 flex items-center gap-1.5">
              <Loader2 className="size-3.5 animate-spin text-[#3ecf8e]" />
              Processing spatial payload…
            </span>
            <span className="text-neutral-900 dark:text-white font-mono font-semibold">{progress}%</span>
          </div>
          <div className="h-2 bg-neutral-100 dark:bg-[#171717] rounded-full overflow-hidden border border-neutral-200 dark:border-[#2e2e2e]">
            <div
              className="h-full bg-[#3ecf8e] transition-[width] duration-300 rounded-full"
              style={{ width: `${progress}%` }}
            />
          </div>
        </div>
      ) : (
        <div className="mt-8 flex flex-wrap items-center gap-3 border-t border-neutral-200 dark:border-[#2e2e2e] pt-6">
          <Button
            type="button"
            size="sm"
            onClick={onUpload}
            className="bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs rounded-[6px] shadow-xs cursor-pointer"
          >
            <Upload className="size-3.5 mr-1.5" />
            <span>Start Upload</span>
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onReplace}
            className="rounded-[6px] border-neutral-300 dark:border-[#2e2e2e] bg-white dark:bg-[#202020] text-neutral-800 dark:text-neutral-200 text-xs"
          >
            Replace file
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onRemove}
            className="rounded-[6px] text-red-500 hover:bg-red-500/10 text-xs ml-auto"
          >
            Cancel
          </Button>
        </div>
      )}
    </div>
  )
}
