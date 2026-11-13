import assert from "node:assert/strict"
import test from "node:test"

import * as sessionLoading from "./sessionLoading.js"
import {
  loadSessionResources,
  mapConfigurationMemberIds,
} from "./sessionLoading.js"

test("map workspace waits for a real session before loading the map", async () => {
  assert.equal(typeof sessionLoading.loadMapWorkspace, "function")

  const calls = []
  const result = await sessionLoading.loadMapWorkspace({
    ensureSession: async () => {
      calls.push("session")
      return "sess_123"
    },
    loadMap: async () => {
      calls.push("map")
      return { id: "map_123" }
    },
  })

  assert.deepEqual(calls, ["session", "map"])
  assert.deepEqual(result, { id: "map_123" })
})

test("only backend session IDs are eligible for recovery probes", () => {
  assert.equal(typeof sessionLoading.isBackendSessionId, "function")
  assert.equal(sessionLoading.isBackendSessionId("map_map_b259951007958ecb"), false)
  assert.equal(sessionLoading.isBackendSessionId("sess_343abe418e1a1378834caac92a4e56a0"), true)
})

test("dataset recovery selects the live session that owns the requested dataset", () => {
  assert.equal(typeof sessionLoading.findDatasetSessionId, "function")

  const sessionId = sessionLoading.findDatasetSessionId([
    { id: "sess_empty", first_dataset_id: "" },
    { id: "sess_owner", first_dataset_id: "ds_target" },
  ], "ds_target")

  assert.equal(sessionId, "sess_owner")

  const secondarySessionId = sessionLoading.findDatasetSessionId([
    { id: "sess_multi", first_dataset_id: "ds_first", dataset_ids: ["ds_first", "ds_second"] },
  ], "ds_second")

  assert.equal(secondarySessionId, "sess_multi")
})

test("dataset workspace recovery completes before loading workspace data", async () => {
  assert.equal(typeof sessionLoading.loadDatasetWorkspace, "function")

  const calls = []
  await sessionLoading.loadDatasetWorkspace({
    datasetId: "ds_target",
    ensureSession: async (datasetId) => calls.push(`session:${datasetId}`),
    loadWorkspace: async (datasetId) => calls.push(`workspace:${datasetId}`),
  })

  assert.deepEqual(calls, ["session:ds_target", "workspace:ds_target"])
})

test("session resources wait for stale-session recovery before loading", async () => {
  let releaseSession
  const sessionReady = new Promise((resolve) => {
    releaseSession = resolve
  })
  const calls = []

  const loading = loadSessionResources({
    ensureSession: async () => {
      calls.push("ensure:start")
      await sessionReady
      calls.push("ensure:end")
    },
    loadResources: [
      async () => calls.push("datasets"),
      async () => calls.push("maps"),
    ],
  })

  await Promise.resolve()
  assert.deepEqual(calls, ["ensure:start"])

  releaseSession()
  await loading
  assert.deepEqual(calls, ["ensure:start", "ensure:end", "datasets", "maps"])
})

test("map configuration members match the backend string ID contract", () => {
  assert.deepEqual(mapConfigurationMemberIds([
    { dataset_id: "ds-primary", name: "Ground" },
    { dataset_id: "ds-upper", name: "First" },
  ]), ["ds-primary", "ds-upper"])
})
