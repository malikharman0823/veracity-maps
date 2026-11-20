import assert from "node:assert/strict"
import test from "node:test"
import { buildExportHierarchy } from "./exportHierarchy.js"

test("buildExportHierarchy with only standalone floorplan.png produces no phantom buildings", () => {
  const members = [
    { dataset_id: "ds_single", name: "floorplan.png", raster_opacity: 1 },
  ]
  const datasets = [
    { id: "ds_single", filename: "floorplan.png", file_size: 1024 },
  ]

  const result = buildExportHierarchy({ members, datasets })

  assert.equal(result.buildings.length, 0, "Should have 0 buildings")
  assert.equal(result.standaloneLayers.length, 1, "Should have 1 standalone layer")
  assert.equal(result.standaloneLayers[0].id, "ds_single")
  assert.equal(result.standaloneLayers[0].name, "floorplan.png")
  assert.deepEqual(result.allLayerIds, ["ds_single"])
})

test("buildExportHierarchy with multi-floor building groups floors under that building name", () => {
  const members = [
    {
      dataset_id: "ds_1",
      name: "Building B Ground Floor",
      building_name: "Building B",
      floor_level: 0,
      floor_label: "Ground Floor",
    },
    {
      dataset_id: "ds_2",
      name: "Ground_0.png",
      building_name: "Building B",
      floor_level: 0,
      floor_label: "Ground Floor",
    },
    {
      dataset_id: "ds_standalone",
      name: "standalone.png",
      raster_opacity: 0.8,
    },
  ]
  const datasets = [
    { id: "ds_1", filename: "Building_B_0.png" },
    { id: "ds_2", filename: "Ground_0.png" },
    { id: "ds_standalone", filename: "standalone.png" },
  ]

  const result = buildExportHierarchy({ members, datasets })

  assert.equal(result.buildings.length, 1, "Should have 1 building")
  assert.equal(result.buildings[0].name, "Building B")
  assert.equal(result.buildings[0].floors.length, 1)
  assert.equal(result.buildings[0].floors[0].layers.length, 2)
  assert.equal(result.standaloneLayers.length, 1)
  assert.equal(result.standaloneLayers[0].name, "standalone.png")
  assert.equal(result.allLayerIds.length, 3)
})

test("buildExportHierarchy classifies layers into 6 standard categories", () => {
  const members = [
    { dataset_id: "ds_raster", name: "aerial.tif" },
    { dataset_id: "ds_vector", name: "boundaries.gpkg" },
  ]
  const datasets = [
    { id: "ds_raster", filename: "aerial.tif", extracted_metadata: { control_points_count: 4 } },
    { id: "ds_vector", filename: "boundaries.gpkg", extracted_metadata: { document_type: "vector" } },
  ]

  const result = buildExportHierarchy({ members, datasets })

  assert.ok(result.categories, "categories object should be defined")
  assert.equal(result.categories.base_maps.length, 1, "Should have default base map")
  assert.equal(result.categories.raster_layers.length, 1, "Should have 1 raster layer")
  assert.equal(result.categories.raster_layers[0].id, "ds_raster")
  assert.equal(result.categories.vector_layers.length, 1, "Should have 1 vector layer")
  assert.equal(result.categories.vector_layers[0].id, "ds_vector")
  assert.equal(result.categories.control_points.length, 1, "Should have control points entry for raster")
  assert.equal(result.categories.icon_points.length, 1, "Should have icon points entry for raster")
})

test("buildExportHierarchy includes active GIS layers and standalone vector datasets in vector_layers category", () => {
  const members = [
    { dataset_id: "ds_raster", name: "floorplan.png" },
  ]
  const datasets = [
    { id: "ds_raster", filename: "floorplan.png" },
    { id: "ds_gpkg", filename: "points_layer.gpkg", extracted_metadata: { feature_count: 42, geometry_type: "Point" } },
  ]
  const gisLayers = [
    {
      id: "wl_roads",
      layer_id: "lyr_roads",
      display_name: "Road Network",
      geometry_type: "LineString",
      feature_count: 15,
      visibility: true,
      source_dataset_id: "ds_gpkg",
    },
  ]

  const result = buildExportHierarchy({ members, datasets, gisLayers })

  assert.equal(result.categories.vector_layers.length, 2, "Should have 2 vector layers (1 from gisLayers + 1 from datasets)")
  const roadLyr = result.categories.vector_layers.find((v) => v.id === "wl_roads")
  assert.ok(roadLyr, "Should find wl_roads")
  assert.equal(roadLyr.name, "Road Network")
  assert.equal(roadLyr.geometryType, "LineString")
  assert.equal(roadLyr.featureCount, 15)

  const gpkgLyr = result.categories.vector_layers.find((v) => v.id === "ds_gpkg")
  assert.ok(gpkgLyr, "Should find ds_gpkg")
  assert.equal(gpkgLyr.name, "points_layer.gpkg")
  assert.equal(gpkgLyr.geometryType, "Point")

  assert.ok(result.allLayerIds.includes("wl_roads"), "allLayerIds should include wl_roads")
  assert.ok(result.allLayerIds.includes("ds_gpkg"), "allLayerIds should include ds_gpkg")
})

