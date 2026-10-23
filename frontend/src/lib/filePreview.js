export function getFilePreviewKind(file) {
  const mediaType = String(file?.type || "").toLowerCase()
  return mediaType.startsWith("image/") ? "image" : "file"
}

export function getFileTypeLabel(file) {
  const mediaType = String(file?.type || "").trim()
  if (mediaType) return mediaType

  const filename = String(file?.name || "")
  const extension = filename.includes(".") ? filename.split(".").pop() : ""
  return extension ? `${extension.toUpperCase()} file` : "Unknown file type"
}
