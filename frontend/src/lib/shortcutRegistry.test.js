import assert from "node:assert/strict"
import test from "node:test"
import {
  DEFAULT_SHORTCUTS,
  normalizeShortcutCombo,
  formatShortcutForOS,
  isTextInputTarget,
  matchesShortcutEvent,
  getEffectiveShortcuts,
  validateShortcutAssignment,
} from "./shortcutRegistry.js"

test("DEFAULT_SHORTCUTS has unique non-empty shortcuts for all registered tools", () => {
  const seen = new Set()
  for (const item of DEFAULT_SHORTCUTS) {
    const norm = normalizeShortcutCombo(item.shortcut)
    assert.ok(norm.length > 0, `empty shortcut for ${item.tool_id}`)
    assert.equal(seen.has(norm), false, `duplicate default shortcut ${norm} on ${item.tool_id}`)
    seen.add(norm)
  }
})

test("matchesShortcutEvent matches single-key, modifier, and special shortcuts accurately", () => {
  assert.equal(matchesShortcutEvent({ key: "v", ctrlKey: false, shiftKey: false, altKey: false }, "V"), true)
  assert.equal(matchesShortcutEvent({ key: "z", ctrlKey: true, shiftKey: false, altKey: false }, "Ctrl+Z"), true)
  assert.equal(matchesShortcutEvent({ key: "Z", ctrlKey: true, shiftKey: true, altKey: false }, "Ctrl+Shift+Z"), true)
  assert.equal(matchesShortcutEvent({ key: "F6", ctrlKey: false, shiftKey: false, altKey: false }, "F6"), true)
  assert.equal(matchesShortcutEvent({ key: "Escape", ctrlKey: false, shiftKey: false, altKey: false }, "Escape"), true)
  assert.equal(matchesShortcutEvent({ key: "?", ctrlKey: false, shiftKey: true, altKey: false }, "?"), true)
})

test("isTextInputTarget protects inputs, textareas, selects, and contenteditable elements", () => {
  assert.equal(isTextInputTarget({ tagName: "INPUT", type: "text" }), true)
  assert.equal(isTextInputTarget({ tagName: "TEXTAREA" }), true)
  assert.equal(isTextInputTarget({ tagName: "SELECT" }), true)
  assert.equal(isTextInputTarget({ tagName: "DIV", isContentEditable: true }), true)
  assert.equal(isTextInputTarget({ tagName: "BUTTON" }), false)
  assert.equal(isTextInputTarget({ tagName: "CANVAS" }), false)
})

test("validateShortcutAssignment blocks duplicate assignments and reserved browser keys", () => {
  const effective = getEffectiveShortcuts({})
  const dup = validateShortcutAssignment(effective, "point", "V")
  assert.equal(dup.valid, false)

  const reserved = validateShortcutAssignment(effective, "point", "Ctrl+W")
  assert.equal(reserved.valid, false)

  const ok = validateShortcutAssignment(effective, "point", "Alt+P")
  assert.equal(ok.valid, true)
  assert.equal(ok.shortcut, "Alt+P")

  assert.equal(formatShortcutForOS("Ctrl+Shift+L", "MacIntel"), "⌘⇧L")
  assert.equal(formatShortcutForOS("Ctrl+Shift+L", "Win32"), "Ctrl+Shift+L")
})
