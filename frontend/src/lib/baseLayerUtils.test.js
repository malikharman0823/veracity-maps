import assert from "node:assert/strict"
import test from "node:test"
import {
  BASEMAP_LAYER_ID,
  BASEMAP_LABELS_LAYER_ID,
  BASEMAP_ROADS_LAYER_ID,
  CUSTOM_BASE_RASTER_LAYER_ID,
  isBackgroundLayerId,
  findFirstWorkspaceLayerId,
  resolveBaseLayerCoordinates,
} from "./baseLayerUtils.js"

test("isBackgroundLayerId separates base layers from workspace and GIS layers", () => {
  assert.equal(isBackgroundLayerId(BASEMAP_LAYER_ID), true)
  assert.equal(isBackgroundLayerId(BASEMAP_ROADS_LAYER_ID), true)
  assert.equal(isBackgroundLayerId(BASEMAP_LABELS_LAYER_ID), true)
  assert.equal(isBackgroundLayerId(CUSTOM_BASE_RASTER_LAYER_ID), true)
  assert.equal(isBackgroundLayerId("workspace-ds_123-raster-layer"), false)
  assert.equal(isBackgroundLayerId("gis-fill-lyr_456"), false)
})

test("findFirstWorkspaceLayerId returns the lowest foreground workspace layer so base layers render underneath", () => {
  const fakeMap = {
    getStyle: () => ({
      layers: [
        { id: BASEMAP_LAYER_ID },
        { id: CUSTOM_BASE_RASTER_LAYER_ID },
        { id: "workspace-ds_survey-raster-layer" },
        { id: "gis-fill-lyr_1" },
      ],
    }),
  }
  assert.equal(findFirstWorkspaceLayerId(fakeMap), "workspace-ds_survey-raster-layer")
})

test("resolveBaseLayerCoordinates derives clockwise corners from WGS84 extent and preserves explicit affine corners", () => {
  const fromExtent = resolveBaseLayerCoordinates({
    extent: [77.1, 28.5, 77.3, 28.7],
  })
  assert.deepEqual(fromExtent, [
    [77.1, 28.7],
    [77.3, 28.7],
    [77.3, 28.5],
    [77.1, 28.5],
  ])

  const rotated = [
    [77.11, 28.71],
    [77.31, 28.69],
    [77.29, 28.49],
    [77.09, 28.51],
  ]
  const fromAffine = resolveBaseLayerCoordinates({
    extent: [77.09, 28.49, 77.31, 28.71],
    rendering_source: { coordinates: rotated },
  })
  assert.deepEqual(fromAffine, rotated)
})
