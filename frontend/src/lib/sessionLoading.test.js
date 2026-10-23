import assert from "node:assert/strict"
import test from "node:test"

import {
  loadSessionResources,
  normalizeSavedConfigs,
  savedConfigMemberIds,
} from "./sessionLoading.js"

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
      async () => calls.push("configs"),
    ],
  })

  await Promise.resolve()
  assert.deepEqual(calls, ["ensure:start"])

  releaseSession()
  await loading
  assert.deepEqual(calls, ["ensure:start", "ensure:end", "datasets", "configs"])
})

test("saved configurations use the API array response", () => {
  const configs = [{ id: "cfg-1", name: "Campus" }]

  assert.deepEqual(normalizeSavedConfigs(configs), configs)
  assert.deepEqual(normalizeSavedConfigs({ configs }), configs)
})

test("saved configuration members match the backend string ID contract", () => {
  assert.deepEqual(savedConfigMemberIds([
    { dataset_id: "ds-primary", name: "Ground" },
    { dataset_id: "ds-upper", name: "First" },
  ]), ["ds-primary", "ds-upper"])
})
