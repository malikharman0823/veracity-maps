import test from "node:test"
import assert from "node:assert/strict"

import { getFilePreviewKind, getFileTypeLabel } from "./filePreview.js"

test("image preview detection follows the MIME type", () => {
  assert.equal(getFilePreviewKind({ type: "image/png" }), "image")
  assert.equal(getFilePreviewKind({ type: "image/jpeg" }), "image")
  assert.equal(getFilePreviewKind({ type: "application/zip" }), "file")
})

test("file type labels use MIME data and fall back to the selected extension", () => {
  assert.equal(getFileTypeLabel({ type: "application/zip", name: "maps.zip" }), "application/zip")
  assert.equal(getFileTypeLabel({ type: "", name: "maps.zip" }), "ZIP file")
  assert.equal(getFileTypeLabel({ type: "", name: "README" }), "Unknown file type")
})
