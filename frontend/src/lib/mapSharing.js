export function getShareMapId(map, isOpen) {
  if (!isOpen) return null

  const mapId = String(map?.id ?? "").trim()
  if (!mapId || mapId === "undefined" || mapId === "null") return null

  return mapId
}
