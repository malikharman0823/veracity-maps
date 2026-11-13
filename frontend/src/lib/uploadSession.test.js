import assert from "node:assert/strict"
import test from "node:test"

import { syncUploadSession } from "./uploadSession.js"

test("syncUploadSession adopts the session returned by an upload", () => {
  let storedSessionId = null

  const sessionId = syncUploadSession(
    { headers: { "x-session-id": "sess_recovered" } },
    (value) => {
      storedSessionId = value
    }
  )

  assert.equal(sessionId, "sess_recovered")
  assert.equal(storedSessionId, "sess_recovered")
})
