import assert from "node:assert/strict"
import test from "node:test"

import { persistMapConfiguration } from "./mapConfigPersistence.js"

test("map config persistence saves an existing shared map without creating a copy", async () => {
  const calls = []
  const result = await persistMapConfiguration({
    mapId: "map_shared",
    payload: { primary_dataset_id: "ds_primary", members: ["ds_primary"] },
    name: "Shared Floor",
    expectedRevision: 4,
    activeView: "map",
  }, {
    saveMapConfig: async (...args) => {
      calls.push(["save", ...args])
      return { map: { id: "map_shared", revision_number: 5 }, revision: { revision_number: 5 } }
    },
    createMapFromConfiguration: async () => {
      throw new Error("existing map save must not create a personal copy")
    },
  })

  assert.deepEqual(calls, [[
    "save",
    "map_shared",
    { primary_dataset_id: "ds_primary", members: ["ds_primary"] },
    "Shared Floor",
    4,
  ]])
  assert.equal(result.map.id, "map_shared")
  assert.equal(result.navigateTo, null)
})

test("map config persistence creates a personal map for a standalone workspace", async () => {
  const payload = {
    name: "Ground Floor",
    primary_dataset_id: "ds_primary",
    members: ["ds_primary"],
    configuration: { active_view: "database" },
  }
  const result = await persistMapConfiguration({
    mapId: null,
    payload,
    name: "Ground Floor",
    expectedRevision: 0,
    activeView: "database",
  }, {
    saveMapConfig: async () => {
      throw new Error("standalone save must not target a missing map")
    },
    createMapFromConfiguration: async (received) => {
      assert.deepEqual(received, payload)
      return { map: { id: "map_personal", revision_number: 1 }, revision: { revision_number: 1 } }
    },
  })

  assert.equal(result.map.id, "map_personal")
  assert.equal(result.revision.revision_number, 1)
  assert.equal(result.navigateTo, "/maps/map_personal/edit?view=database")
})
