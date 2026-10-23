import assert from "node:assert/strict"
import test from "node:test"
import { resolveBuildingName, isMemberBuildingMatch } from "./buildingUtils.js"

test("resolveBuildingName infers building name from realBuildingNames and tokens", () => {
  const members = [
    { dataset_id: "1", name: "Building_A_Floor_1.png", is_building_floor: true },
    { dataset_id: "2", name: "Building_A_Floor_2.png", is_building_floor: true },
    { dataset_id: "3", name: "Processed_Raster.tif", building_name: "Processed" },
  ]
  const buildings = [{ name: "Building A" }]

  assert.equal(resolveBuildingName(members[0], members, buildings), "Building A")
  assert.equal(resolveBuildingName(members[1], members, buildings), "Building A")
  assert.equal(resolveBuildingName(members[2], members, buildings), "Processed")
})

test("isMemberBuildingMatch strictly distinguishes between buildings when target is specified", () => {
  const members = [
    { dataset_id: "1", name: "Building_A_Floor_1.png", is_building_floor: true },
    { dataset_id: "3", name: "Processed_Raster.tif", building_name: "Processed" },
  ]
  const buildings = [{ name: "Building A" }]

  assert.equal(isMemberBuildingMatch(members[0], "Building A", members, buildings), true)
  assert.equal(isMemberBuildingMatch(members[1], "Building A", members, buildings), false)
  assert.equal(isMemberBuildingMatch(members[1], "Processed", members, buildings), true)
})
