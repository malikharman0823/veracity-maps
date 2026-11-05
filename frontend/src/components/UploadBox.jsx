import React, { useRef, useState } from "react"
import { Button } from "@/components/ui/button"
import { UploadCloud, FileArchive } from "lucide-react"

export const ACCEPTED_FORMATS = ".png,.jpg,.jpeg,.tif,.tiff,.pdf,.dxf,.dwg,.svf,.mbtiles,.zip,image/png,image/jpeg,image/tiff,application/pdf,application/zip,application/vnd.mapbox-vector-tile"

export default function UploadBox({ disabled, onSelect }) {
  const inputRef = useRef(null)
  const [dragging, setDragging] = useState(false)

  function chooseFile() {
    inputRef.current?.click()
  }

  function selectFiles(files) {
    const [file] = files
    if (file) onSelect(file)
  }

  function handleDrop(event) {
    event.preventDefault()
    setDragging(false)
    if (!disabled) selectFiles(event.dataTransfer.files)
  }

  return (
    <div
      onClick={!disabled ? chooseFile : undefined}
      className={`relative group bg-white dark:bg-[#202020] border-2 border-dashed transition-all duration-200 rounded-[12px] p-8 sm:p-12 flex flex-col items-center justify-center text-center cursor-pointer shadow-xs ${
        dragging
          ? "border-[#3ecf8e] bg-[#3ecf8e]/5"
          : "border-neutral-300 dark:border-[#2e2e2e] hover:border-[#3ecf8e]/60 hover:bg-neutral-50 dark:hover:bg-[#252525]"
      } ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
      onDragEnter={(event) => {
        event.preventDefault()
        if (!disabled) setDragging(true)
      }}
      onDragOver={(event) => event.preventDefault()}
      onDragLeave={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setDragging(false)
      }}
      onDrop={handleDrop}
    >
      <div className="size-14 rounded-[12px] bg-neutral-100 dark:bg-[#171717] border border-neutral-200 dark:border-[#2e2e2e] flex items-center justify-center text-[#24b47e] dark:text-[#3ecf8e] mb-4 group-hover:scale-105 transition-transform">
        <UploadCloud className="size-7" />
      </div>

      <h3 className="text-base font-semibold text-neutral-900 dark:text-white mb-1">
        Drop a dataset file here or click to browse
      </h3>
      <p className="text-xs text-neutral-500 dark:text-neutral-400 max-w-md mb-6">
        Supports PNG, JPG/JPEG, TIFF/GeoTIFF (.tif, .tiff), PDF, CAD (.dxf, .dwg, .svf), standalone MBTiles (.mbtiles), and Veracity Maps project packages (.zip).
      </p>

      <div className="flex flex-wrap items-center justify-center gap-2.5">
        <Button
          type="button"
          disabled={disabled}
          size="sm"
          className="bg-[#3ecf8e] hover:bg-[#24b47e] text-[#171717] font-semibold text-xs rounded-[6px] shadow-xs cursor-pointer"
        >
          <FileArchive className="size-3.5 mr-1.5" />
          <span>Choose file</span>
        </Button>
      </div>

      <input
        ref={inputRef}
        className="sr-only"
        type="file"
        accept={ACCEPTED_FORMATS}
        disabled={disabled}
        onChange={(event) => selectFiles(event.target.files)}
      />
    </div>
  )
}
