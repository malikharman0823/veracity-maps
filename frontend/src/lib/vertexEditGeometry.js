/**
 * Pure geometry helpers for interactive Vertex Editor resizing & reshaping.
 */

export function isClosedGeometryType(geomType) {
  return geomType === "Polygon" || geomType === "MultiPolygon"
}

export function minVerticesForType(geomType) {
  if (geomType === "Polygon" || geomType === "MultiPolygon") return 3
  if (geomType === "LineString") return 2
  return 1
}

export function extractEditableVertices(geometry) {
  if (!geometry) return []
  if (geometry.type === "Point" && Array.isArray(geometry.coordinates)) {
    return [[Number(geometry.coordinates[0]), Number(geometry.coordinates[1])]]
  }
  if (geometry.type === "LineString" && Array.isArray(geometry.coordinates)) {
    return geometry.coordinates.map((pt) => [Number(pt[0]), Number(pt[1])])
  }
  if (geometry.type === "Polygon" && Array.isArray(geometry.coordinates?.[0])) {
    const ring = geometry.coordinates[0]
    const trimmed =
      ring.length > 1 &&
      ring[0][0] === ring[ring.length - 1][0] &&
      ring[0][1] === ring[ring.length - 1][1]
        ? ring.slice(0, -1)
        : ring
    return trimmed.map((pt) => [Number(pt[0]), Number(pt[1])])
  }
  if (geometry.type === "MultiPolygon" && Array.isArray(geometry.coordinates?.[0]?.[0])) {
    const ring = geometry.coordinates[0][0]
    const trimmed =
      ring.length > 1 &&
      ring[0][0] === ring[ring.length - 1][0] &&
      ring[0][1] === ring[ring.length - 1][1]
        ? ring.slice(0, -1)
        : ring
    return trimmed.map((pt) => [Number(pt[0]), Number(pt[1])])
  }
  return []
}

export function buildGeometryFromVertices(baseGeometry, vertices) {
  if (!baseGeometry || !Array.isArray(vertices) || vertices.length === 0) return null
  const clean = vertices.map(([x, y]) => [Number(Number(x).toFixed(7)), Number(Number(y).toFixed(7))])

  if (baseGeometry.type === "Point") {
    return { type: "Point", coordinates: clean[0] }
  }
  if (baseGeometry.type === "LineString") {
    if (clean.length < 2) return null
    return { type: "LineString", coordinates: clean }
  }
  if (baseGeometry.type === "Polygon") {
    if (clean.length < 3) return null
    const closedRing = [...clean, clean[0]]
    const holes = Array.isArray(baseGeometry.coordinates) ? baseGeometry.coordinates.slice(1) : []
    return { type: "Polygon", coordinates: [closedRing, ...holes] }
  }
  if (baseGeometry.type === "MultiPolygon") {
    if (clean.length < 3) return null
    const closedRing = [...clean, clean[0]]
    const firstHoles = Array.isArray(baseGeometry.coordinates?.[0])
      ? baseGeometry.coordinates[0].slice(1)
      : []
    const otherPolys = Array.isArray(baseGeometry.coordinates) ? baseGeometry.coordinates.slice(1) : []
    return {
      type: "MultiPolygon",
      coordinates: [[closedRing, ...firstHoles], ...otherPolys],
    }
  }
  return null
}

export function computeEdgeMidpoints(vertices, isClosed = false) {
  if (!Array.isArray(vertices) || vertices.length < 2) return []
  const out = []
  const count = isClosed ? vertices.length : vertices.length - 1
  for (let i = 0; i < count; i++) {
    const a = vertices[i]
    const b = vertices[(i + 1) % vertices.length]
    out.push({
      edgeIndex: i,
      coordinates: [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2],
    })
  }
  return out
}

export function updateVertexAt(vertices, index, nextCoord) {
  if (!Array.isArray(vertices) || index < 0 || index >= vertices.length) return vertices
  return vertices.map((pt, i) => (i === index ? [nextCoord[0], nextCoord[1]] : pt))
}

/**
 * Uniformly scales all vertices around their centroid based on dragging vertex `index`
 * from its original position to `nextCoord` (used when holding Shift while dragging a vertex).
 */
export function scaleVerticesFromVertex(origVertices, index, nextCoord) {
  if (!Array.isArray(origVertices) || origVertices.length < 2 || index < 0 || index >= origVertices.length) {
    return updateVertexAt(origVertices, index, nextCoord)
  }
  let cx = 0
  let cy = 0
  for (const [x, y] of origVertices) {
    cx += x
    cy += y
  }
  cx /= origVertices.length
  cy /= origVertices.length

  const origPt = origVertices[index]
  const origDist = Math.hypot(origPt[0] - cx, origPt[1] - cy)
  if (origDist < 1e-10) {
    return updateVertexAt(origVertices, index, nextCoord)
  }
  const nextDist = Math.hypot(nextCoord[0] - cx, nextCoord[1] - cy)
  const scale = Math.max(0.05, Math.min(50, nextDist / origDist))

  return origVertices.map(([x, y]) => [
    cx + (x - cx) * scale,
    cy + (y - cy) * scale,
  ])
}

export function insertVertexAtEdge(vertices, edgeIndex, coord) {
  if (!Array.isArray(vertices) || edgeIndex < 0 || edgeIndex >= vertices.length) return vertices
  const next = [...vertices]
  next.splice(edgeIndex + 1, 0, [coord[0], coord[1]])
  return next
}

export function removeVertexAt(vertices, index, minCount = 3) {
  if (!Array.isArray(vertices) || vertices.length <= minCount || index < 0 || index >= vertices.length) {
    return null
  }
  return vertices.filter((_, i) => i !== index)
}
