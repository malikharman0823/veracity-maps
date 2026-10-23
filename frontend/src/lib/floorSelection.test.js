import assert from "node:assert/strict"
import test from "node:test"

import { applyFloorSelection } from "./floorSelection.js"

test("floor selection updates owned state without notifying the map back through a callback", () => {
  const levels = []
  const buildings = []
  const updates = []

  applyFloorSelection({
    level: "2",
    buildingName: "North",
    members: [
      { dataset_id: "north-2", floor_level: 2, building_name: "North", visible: false },
      { dataset_id: "south-2", floor_level: 2, building_name: "South", visible: false },
    ],
    setFloorLevel: (level) => levels.push(level),
    setBuildingName: (name) => buildings.push(name),
    updateMember: (datasetId, patch) => updates.push({ datasetId, patch }),
  })

  assert.deepEqual(levels, [2])
  assert.deepEqual(buildings, ["North"])
  assert.deepEqual(updates, [{
    datasetId: "north-2",
    patch: { visible: true, raster_visible: true },
  }])
})
