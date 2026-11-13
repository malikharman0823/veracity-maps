import assert from "node:assert/strict"
import test from "node:test"

import { getShareMapId } from "./mapSharing.js"

test("sharing activates only for an open modal with a valid map ID", () => {
  assert.equal(getShareMapId({ id: "map_123" }, true), "map_123")
  assert.equal(getShareMapId({ id: "map_123" }, false), null)
  assert.equal(getShareMapId({ id: undefined }, true), null)
  assert.equal(getShareMapId({ id: "undefined" }, true), null)
  assert.equal(getShareMapId({ id: "null" }, true), null)
  assert.equal(getShareMapId(null, true), null)
})
