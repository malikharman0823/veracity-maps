import { readdir, stat } from "node:fs/promises"
import path from "node:path"

const assetsDir = path.resolve("dist/assets")
const maxJavaScriptChunkBytes = 1075 * 1024
const maxEntryChunkBytes = 540 * 1024

const names = await readdir(assetsDir)
const javascriptNames = names.filter((name) => name.endsWith(".js"))

if (javascriptNames.length === 0) {
  throw new Error(`No production JavaScript chunks found in ${assetsDir}; run npm run build first`)
}

const chunks = await Promise.all(
  javascriptNames.map(async (name) => ({
    name,
    bytes: (await stat(path.join(assetsDir, name))).size,
  })),
)

const violations = []
for (const chunk of chunks) {
  if (chunk.bytes > maxJavaScriptChunkBytes) {
    violations.push(`${chunk.name} is ${(chunk.bytes / 1024).toFixed(2)} KiB (limit: 1075 KiB)`)
  }
  if (/^index-[^.]+\.js$/.test(chunk.name) && chunk.bytes > maxEntryChunkBytes) {
    violations.push(`${chunk.name} entry chunk is ${(chunk.bytes / 1024).toFixed(2)} KiB (limit: 540 KiB)`)
  }
}

if (violations.length > 0) {
  throw new Error(`Production bundle budget exceeded:\n${violations.join("\n")}`)
}

const largest = chunks.sort((left, right) => right.bytes - left.bytes)[0]
console.log(
  `Bundle budget passed: ${chunks.length} JavaScript chunks; largest ${largest.name} ` +
    `${(largest.bytes / 1024).toFixed(2)} KiB (limit: 1075 KiB)`,
)
