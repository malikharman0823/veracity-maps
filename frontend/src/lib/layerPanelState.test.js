import assert from "node:assert/strict"
import test from "node:test"

import { nextExpandedLayerId } from "./layerPanelState.js"

test("layer settings accordion collapses the current layer and switches to another", () => {
  assert.equal(nextExpandedLayerId(null, "layer-a"), "layer-a")
  assert.equal(nextExpandedLayerId("layer-a", "layer-a"), null)
  assert.equal(nextExpandedLayerId("layer-a", "layer-b"), "layer-b")
})
