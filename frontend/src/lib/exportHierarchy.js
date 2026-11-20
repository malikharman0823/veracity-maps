import { resolveBuildingName } from "./buildingUtils.js"

/**
 * Builds export groups (buildings with floors, and standalone layers)
 * strictly from the layers present in manifest.members and actual datasets.
 * Never invents phantom buildings like "Building A" or "Building B".
 *
 * @param {Object} options
 * @param {Array} options.members - Workspace manifest members (ground truth for layers)
 * @param {Array} options.datasets - List of datasets in the session/workspace
 * @param {Array} options.buildings - Optional formal buildings list
 * @param {Array} options.gisLayers - Active GIS/vector workspace layers
 * @param {string} options.primaryDatasetId - ID of active primary dataset
 * @returns {{ buildings: Array, standaloneLayers: Array, allLayerIds: Array, categories: Object }}
 */
export function buildExportHierarchy({
  members = [],
  datasets = [],
  buildings = [],
  gisLayers = [],
  primaryDatasetId = null,
} = {}) {
  const memberList = Array.isArray(members) ? members : []
  const datasetList = Array.isArray(datasets) ? datasets : []
  const buildingList = Array.isArray(buildings) ? buildings : []

  // Map each member to an enriched export layer object
  const enrichedLayers = memberList.map((m) => {
    const ds = datasetList.find((d) => d.id === m.dataset_id)
    const effectiveBuildingName = resolveBuildingName(m, memberList, buildingList).trim()

    return {
      id: m.dataset_id,
      name: m.name || ds?.filename || "Layer",
      dataset: ds,
      member: m,
      isPrimary: m.dataset_id === primaryDatasetId,
      opacity: m.raster_opacity ?? ds?.extracted_metadata?.raster_opacity ?? 1,
      bgRemoved: !!(ds?.extracted_metadata?.background_removal?.enabled),
      rotated: !!(ds?.extracted_metadata?.raster_transform?.rotation_angle),
      rotationAngle: ds?.extracted_metadata?.raster_transform?.rotation_angle || 0,
      buildingName: effectiveBuildingName,
      isBuildingFloor: !!m.is_building_floor || !!effectiveBuildingName,
      floorLevel: m.floor_level ?? ds?.extracted_metadata?.floor_level ?? 0,
      floorLabel:
        m.floor_label ||
        ds?.extracted_metadata?.floor_label ||
        (m.floor_level === 0 ? "Ground Floor" : `Floor ${m.floor_level ?? 0}`),
    }
  })

  const buildingGroupsMap = new Map()
  const standaloneLayers = []

  enrichedLayers.forEach((layer) => {
    // If it has a real building name, group under that building
    if (layer.buildingName) {
      if (!buildingGroupsMap.has(layer.buildingName)) {
        buildingGroupsMap.set(layer.buildingName, new Map())
      }
      const floorMap = buildingGroupsMap.get(layer.buildingName)
      const fKey = layer.floorLevel
      if (!floorMap.has(fKey)) {
        floorMap.set(fKey, {
          id: `floor_${layer.buildingName}_${fKey}`,
          name: layer.floorLabel,
          level: fKey,
          layers: [],
        })
      }
      floorMap.get(fKey).layers.push(layer)
    } else {
      standaloneLayers.push(layer)
    }
  })

  // Convert buildingGroupsMap to an array of building structures
  const buildingsResult = Array.from(buildingGroupsMap.entries()).map(([bName, floorMap], idx) => {
    // Find formal building ID if present in buildingList
    const formalBld = buildingList.find(
      (b) => (b.name || "").trim().toLowerCase() === bName.toLowerCase()
    )
    const bId = formalBld?.id || `bld_${idx}_${bName.replace(/\s+/g, "_")}`

    const floors = Array.from(floorMap.values()).sort((a, b) => b.level - a.level)

    return {
      id: bId,
      name: bName,
      floors: floors,
    }
  })

  const allLayerIds = enrichedLayers.map((l) => l.id)

  // Categorize into the 6 standard export categories
  const categories = {
    base_maps: [
      {
        id: "base_map_osm",
        name: "OpenStreetMap Standard",
        type: "Base Map",
        format: "XYZ URL",
        visible: true,
        associated: "Global",
        selectable: true,
      },
    ],
    raster_layers: [],
    vector_layers: [],
    icon_points: [],
    control_points: [],
    areas: [],
  }

  const seenVectorIds = new Set()

  enrichedLayers.forEach((layer) => {
    const fn = layer.dataset?.filename || layer.name || ""
    const ext = fn.slice(fn.lastIndexOf(".")).toLowerCase()
    const isVector = [".gpkg", ".shp", ".kml", ".kmz", ".geojson", ".osm", ".pbf"].includes(ext) ||
      layer.dataset?.extracted_metadata?.document_type === "vector"

    const layerMeta = {
      id: layer.id,
      name: layer.name,
      type: isVector ? "Vector Layer" : "Raster Layer",
      format: ext.toUpperCase().replace(".", "") || (isVector ? "GPKG" : "PNG"),
      visible: layer.member?.visible ?? true,
      associated: layer.buildingName ? `${layer.buildingName} (${layer.floorLabel})` : "Standalone",
      buildingName: layer.buildingName,
      floorLabel: layer.floorLabel,
      isVector,
      dataset: layer.dataset,
      member: layer.member,
    }

    if (isVector) {
      seenVectorIds.add(layer.id)
      categories.vector_layers.push(layerMeta)
    } else {
      categories.raster_layers.push(layerMeta)

      // Derive control points and icon points for this raster layer
      const meta = layer.dataset?.extracted_metadata
      const cpCount = meta?.control_points_count || (meta?.control_points ? meta.control_points.length : 0)
      categories.control_points.push({
        id: `cp_group_${layer.id}`,
        rasterId: layer.id,
        name: `Control Points (${layer.name})`,
        type: "Control Points",
        format: "GeoJSON",
        count: cpCount,
        visible: true,
        associated: layer.name,
      })

      const ipCount = meta?.icon_points_count || (meta?.icon_points ? meta.icon_points.length : 0)
      categories.icon_points.push({
        id: `ip_group_${layer.id}`,
        rasterId: layer.id,
        name: `Icon Points & Landmarks (${layer.name})`,
        type: "Icon Points",
        format: "GeoJSON",
        count: ipCount,
        visible: true,
        associated: layer.name,
      })

      if (meta?.areas && Array.isArray(meta.areas) && meta.areas.length > 0) {
        categories.areas.push({
          id: `area_group_${layer.id}`,
          rasterId: layer.id,
          name: `Areas & Drawings (${layer.name})`,
          type: "Areas",
          format: "GeoJSON",
          count: meta.areas.length,
          visible: true,
          associated: layer.name,
        })
      }
    }
  })

  // Integrate active GIS/vector workspace layers (e.g. from GeoPackage, Shapefile imports)
  const gisList = Array.isArray(gisLayers) ? gisLayers : []
  gisList.forEach((lyr) => {
    const lId = lyr.id || lyr.layer_id || lyr.workspace_layer_id
    if (!lId || seenVectorIds.has(lId)) return
    seenVectorIds.add(lId)

    const geomType = lyr.geometry_type || lyr.geometry || "Vector"
    const fCount = lyr.feature_count ?? lyr.features_count ?? 0
    const sourceDs = datasetList.find((d) => d.id === lyr.source_dataset_id || d.id === lyr.dataset_id)
    const displayName = lyr.display_name || lyr.name || lyr.source_layer_name || sourceDs?.filename || "Vector Layer"
    const ext = sourceDs?.filename ? sourceDs.filename.slice(sourceDs.filename.lastIndexOf(".")).toLowerCase() : ".gpkg"

    categories.vector_layers.push({
      id: lId,
      name: displayName,
      type: "Vector Layer",
      format: ext.toUpperCase().replace(".", "") || "GPKG",
      geometryType: geomType,
      featureCount: fCount,
      visible: lyr.visibility !== false && lyr.is_visible !== false,
      associated: lyr.buildingName || "Workspace",
      isVector: true,
      dataset: sourceDs,
      layer: lyr,
    })
    if (!allLayerIds.includes(lId)) {
      allLayerIds.push(lId)
    }
  })

  // Integrate any standalone vector datasets from the session/workspace not in members
  datasetList.forEach((ds) => {
    if (!ds || !ds.id || seenVectorIds.has(ds.id)) return
    const fn = ds.filename || ""
    const ext = fn.slice(fn.lastIndexOf(".")).toLowerCase()
    const isVecExt = [".gpkg", ".shp", ".kml", ".kmz", ".geojson", ".json", ".osm", ".pbf"].includes(ext)
    const isVecMeta = ds.extracted_metadata?.document_type === "vector" ||
      (ds.extracted_metadata?.content?.has_vector && !ds.extracted_metadata?.content?.has_raster)

    if (isVecExt || isVecMeta) {
      seenVectorIds.add(ds.id)
      const featCount = ds.extracted_metadata?.feature_count || ds.extracted_metadata?.record_count || 0
      categories.vector_layers.push({
        id: ds.id,
        name: ds.filename || "Vector Dataset",
        type: "Vector Layer",
        format: ext.toUpperCase().replace(".", "") || "VECTOR",
        geometryType: ds.extracted_metadata?.geometry_type || "Geometry",
        featureCount: featCount,
        visible: true,
        associated: "Standalone",
        isVector: true,
        dataset: ds,
      })
      if (!allLayerIds.includes(ds.id)) {
        allLayerIds.push(ds.id)
      }
    }
  })

  return {
    buildings: buildingsResult,
    standaloneLayers,
    allLayerIds,
    categories,
  }
}
