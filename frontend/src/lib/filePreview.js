export function getFilePreviewKind(file) {
  const mediaType = String(file?.type || "").toLowerCase()
  const filename = String(file?.name || "").toLowerCase()
  if (
    mediaType === "image/tiff" ||
    mediaType === "image/tif" ||
    filename.endsWith(".tif") ||
    filename.endsWith(".tiff")
  ) {
    return "file"
  }
  return mediaType.startsWith("image/") ? "image" : "file"
}

export function getFileTypeLabel(file) {
  const filename = String(file?.name || "")
  const lower = filename.toLowerCase()
  if (lower.endsWith(".mbtiles")) return "MBTiles SQLite database (.mbtiles)"
  if (lower.endsWith(".tif") || lower.endsWith(".tiff")) return "TIFF / GeoTIFF raster"

  const mediaType = String(file?.type || "").trim()
  if (mediaType) return mediaType

  const extension = filename.includes(".") ? filename.split(".").pop() : ""
  return extension ? `${extension.toUpperCase()} file` : "Unknown file type"
}
