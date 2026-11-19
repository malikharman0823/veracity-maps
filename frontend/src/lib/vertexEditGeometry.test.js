import assert from "node:assert/strict"
import test from "node:test"
import {
  extractEditableVertices,
  buildGeometryFromVertices,
  computeEdgeMidpoints,
  updateVertexAt,
  scaleVerticesFromVertex,
  insertVertexAtEdge,
  removeVertexAt,
} from "./vertexEditGeometry.js"

test("extractEditableVertices and buildGeometryFromVertices round-trip Polygon rings cleanly", () => {
  const poly = {
    type: "Polygon",
    coordinates: [
      [
        [77.0, 28.0],
        [77.2, 28.0],
        [77.2, 28.2],
        [77.0, 28.2],
        [77.0, 28.0],
      ],
    ],
  }
  const verts = extractEditableVertices(poly)
  assert.equal(verts.length, 4)

  const moved = updateVertexAt(verts, 1, [77.4, 28.0])
  const rebuilt = buildGeometryFromVertices(poly, moved)
  assert.equal(rebuilt.type, "Polygon")
  assert.equal(rebuilt.coordinates[0].length, 5)
  assert.deepEqual(rebuilt.coordinates[0][1], [77.4, 28.0])
  assert.deepEqual(rebuilt.coordinates[0][0], rebuilt.coordinates[0][4])
})

test("scaleVerticesFromVertex proportionally doubles a square polygon when dragging a corner to 2x distance", () => {
  const square = [
    [0, 0],
    [2, 0],
    [2, 2],
    [0, 2],
  ]
  // Centroid is (1, 1). Vertex 2 is at (2, 2) -> dragging to (3, 3) doubles distance from centroid.
  const scaled = scaleVerticesFromVertex(square, 2, [3, 3])
  assert.deepEqual(scaled, [
    [-1, -1],
    [3, -1],
    [3, 3],
    [-1, 3],
  ])
})

test("computeEdgeMidpoints, insertVertexAtEdge, and removeVertexAt maintain valid polygon topology", () => {
  const tri = [
    [0, 0],
    [2, 0],
    [1, 2],
  ]
  const mids = computeEdgeMidpoints(tri, true)
  assert.equal(mids.length, 3)
  assert.deepEqual(mids[0].coordinates, [1, 0])

  const quad = insertVertexAtEdge(tri, 0, [1, -0.5])
  assert.equal(quad.length, 4)
  assert.deepEqual(quad[1], [1, -0.5])

  // Cannot remove below 3 vertices on a triangle
  assert.equal(removeVertexAt(tri, 0, 3), null)
  // Can remove on a quad
  const backToTri = removeVertexAt(quad, 1, 3)
  assert.equal(backToTri.length, 3)
})
