import assert from "node:assert/strict"
import test from "node:test"

import { canDeleteSavedMap, resolveSavedMaps } from "./savedMaps.js"

test("saved map dashboard uses the saved_maps view so a shared map saved by the current user is visible", () => {
  const ownedMap = { id: "map_owned", owner_user_id: "user_b" }
  const sharedSavedMap = { id: "map_shared", owner_user_id: "user_a" }

  assert.deepEqual(resolveSavedMaps({
    my_maps: [ownedMap],
    saved_maps: [ownedMap, sharedSavedMap],
  }), [ownedMap, sharedSavedMap])
})

test("saved map dashboard allows deleting only maps owned by the current user", () => {
  assert.equal(canDeleteSavedMap({ owner_user_id: "user_b" }, { id: "user_b" }), true)
  assert.equal(canDeleteSavedMap({ owner_user_id: "user_a" }, { id: "user_b" }), false)
})
