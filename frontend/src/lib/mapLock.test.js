import assert from "node:assert/strict"
import test from "node:test"

import { lockHeldByAnotherUser, lockHolderNameFromError, lockHolderNameFromLock } from "./mapLock.js"

test("reads a displayable holder name from a structured lock conflict", () => {
  const error = {
    response: {
      status: 409,
      data: {
        error: {
          code: "LOCK_HELD",
          details: {
            map_id: "map_123",
            user_id: "user_456",
            user_name: "Alice",
            locked_at: "2026-11-12T10:00:00Z",
            expires_at: "2026-11-12T10:01:00Z",
          },
        },
      },
    },
  }

  assert.equal(lockHolderNameFromError(error), "Alice")
})

test("falls back safely when lock details do not contain a name", () => {
  assert.equal(lockHolderNameFromError({ response: { data: { error: { details: {} } } } }), "another collaborator")
  assert.equal(lockHolderNameFromError(null), "another collaborator")
})

test("preflight identifies only an active lock held by another user", () => {
  const status = {
    active: true,
    lock: { map_id: "map_123", user_id: "user_456", user_name: "Alice" },
  }

  assert.deepEqual(lockHeldByAnotherUser(status, "user_789"), status.lock)
  assert.equal(lockHeldByAnotherUser(status, "user_456"), null)
  assert.equal(lockHeldByAnotherUser({ active: false }, "user_789"), null)
})

test("reads a safe display name directly from an active lock", () => {
  assert.equal(lockHolderNameFromLock({ user_name: " Alice " }), "Alice")
  assert.equal(lockHolderNameFromLock({ user_name: {} }), "another collaborator")
})
