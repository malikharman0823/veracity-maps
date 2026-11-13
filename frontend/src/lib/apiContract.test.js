import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const apiSource = readFileSync(new URL("../api.js", import.meta.url), "utf8")
const routerSource = readFileSync(
  new URL("../../../backend/api/router.go", import.meta.url),
  "utf8"
)

function clientRequests(source) {
  const requests = []
  const pattern = /client\.(get|post|put|patch|delete)\(\s*([`"'])([\s\S]*?)\2/g

  for (const match of source.matchAll(pattern)) {
    const [, method, , route] = match
    if (!route.startsWith("/")) continue
    requests.push({ method: method.toUpperCase(), route })
  }

  return requests
}

function staticRoutePrefix(route) {
  return route.split("${", 1)[0].split("?", 1)[0]
}

test("frontend API exports do not contain hardcoded async stubs", () => {
  const staticAsyncExport =
    /export\s+const\s+(\w+)\s*=\s*async\s*\([^)]*\)\s*=>\s*(null|undefined|true|false|\[\]|\{\})/g
  const matches = [...apiSource.matchAll(staticAsyncExport)].map((match) => match[1])

  assert.deepEqual(matches, [], `static async API exports: ${matches.join(", ")}`)
})

test("every frontend HTTP wrapper targets a registered backend route family", () => {
  const requests = clientRequests(apiSource)
  const missing = requests.filter(({ route }) => {
    const prefix = staticRoutePrefix(route)
    return prefix.length > 0 && !routerSource.includes(prefix)
  })

  assert.ok(requests.length >= 100, `expected at least 100 API requests, found ${requests.length}`)
  assert.deepEqual(
    missing,
    [],
    `frontend routes without backend families: ${missing
      .map(({ method, route }) => `${method} ${route}`)
      .join(", ")}`
  )
})

test("dynamic path values are URL encoded before being sent", () => {
  const unsafe = []

  for (const { method, route } of clientRequests(apiSource)) {
    const pathname = route.split("?", 1)[0]
    for (const expression of pathname.matchAll(/\$\{([^}]+)\}/g)) {
      if (!expression[1].startsWith("encodeURIComponent(")) {
        unsafe.push(`${method} ${route}`)
      }
    }
  }

  assert.deepEqual(unsafe, [], `unencoded dynamic API paths: ${unsafe.join(", ")}`)
})
