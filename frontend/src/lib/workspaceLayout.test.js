import test from "node:test"
import assert from "node:assert/strict"
import {
  DEFAULT_WORKSPACE_LAYOUT,
  LAYOUT_LIMITS,
  sanitizeWorkspaceLayout,
  computeMapScaleDenominator,
  formatMapScale,
  haversineDistanceMeters,
  polylineDistanceMeters,
  sphericalPolygonAreaSqMeters,
  formatDistance,
  formatArea,
} from "./workspaceLayout.js"

test("sanitizeWorkspaceLayout clamps out-of-range dimensions and preserves booleans", () => {
  const sanitized = sanitizeWorkspaceLayout({
    leftOpen: false,
    leftWidth: 9999,
    rightWidth: 20,
    recordsOpen: true,
    recordsHeight: 50,
    browserSplitRatio: 0.99,
  })
  assert.equal(sanitized.leftOpen, false)
  assert.equal(sanitized.leftWidth, LAYOUT_LIMITS.leftMax)
  assert.equal(sanitized.rightWidth, LAYOUT_LIMITS.rightMin)
  assert.equal(sanitized.recordsOpen, true)
  assert.equal(sanitized.recordsHeight, LAYOUT_LIMITS.recordsMin)
  assert.equal(sanitized.browserSplitRatio, LAYOUT_LIMITS.splitMax)
  assert.equal(sanitized.browserOpen, DEFAULT_WORKSPACE_LAYOUT.browserOpen)
})

test("computeMapScaleDenominator decreases monotonically as zoom increases", () => {
  const z5 = computeMapScaleDenominator(5, 28.6)
  const z12 = computeMapScaleDenominator(12, 28.6)
  const z18 = computeMapScaleDenominator(18, 28.6)
  assert.ok(z5 > z12 && z12 > z18)
  assert.ok(formatMapScale(12, 28.6).startsWith("1:"))
})

test("haversineDistanceMeters and sphericalPolygonAreaSqMeters calculate accurate geodesic metrics", () => {
  // ~1 degree of latitude is ~111.19 km
  const dist = haversineDistanceMeters([0, 0], [0, 1])
  assert.ok(dist > 110_000 && dist < 112_000)

  const polyDist = polylineDistanceMeters([
    [0, 0],
    [0, 1],
    [1, 1],
  ])
  assert.ok(polyDist > 220_000 && polyDist < 224_000)

  // ~0.01 deg x 0.01 deg square near equator ≈ 1.23 km²
  const area = sphericalPolygonAreaSqMeters([
    [0, 0],
    [0.01, 0],
    [0.01, 0.01],
    [0, 0.01],
  ])
  assert.ok(area > 1_100_000 && area < 1_350_000)
  assert.equal(formatDistance(1500), "1.500 km")
  assert.equal(formatArea(25000), "2.50 ha")
})
