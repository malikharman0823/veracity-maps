import assert from "node:assert/strict"
import test from "node:test"
import {
  parseGoogleMapsCoordinates,
  buildDirectCoordinateControlPoints,
} from "./coordinateParser.js"

test("parses standard Google Maps decimal lat, lng pairs (including longitudes <= 90 like India and UK)", () => {
  // New Delhi, India (both |lat| <= 90 and |lng| <= 90 — previously inverted to Arctic Ocean!)
  const delhi = parseGoogleMapsCoordinates("28.613939, 77.209021")
  assert.equal(delhi.ok, true)
  assert.ok(Math.abs(delhi.latitude - 28.613939) < 1e-6)
  assert.ok(Math.abs(delhi.longitude - 77.209021) < 1e-6)
  assert.equal(delhi.ambiguous, true)

  // London, UK
  const london = parseGoogleMapsCoordinates("51.507351, -0.127758")
  assert.equal(london.ok, true)
  assert.ok(Math.abs(london.latitude - 51.507351) < 1e-6)
  assert.ok(Math.abs(london.longitude - -0.127758) < 1e-6)

  // San Francisco, USA (|lng| > 90 — previously rejected by ReferenceMap!)
  const sf = parseGoogleMapsCoordinates("37.774929, -122.419416")
  assert.equal(sf.ok, true)
  assert.ok(Math.abs(sf.latitude - 37.774929) < 1e-6)
  assert.ok(Math.abs(sf.longitude - -122.419416) < 1e-6)
  assert.equal(sf.ambiguous, false)
})

test("parses space-separated, parenthesized, and auto-swapped unambiguous lng, lat pairs", () => {
  const spaced = parseGoogleMapsCoordinates("(28.613939 77.209021)")
  assert.equal(spaced.ok, true)
  assert.ok(Math.abs(spaced.latitude - 28.613939) < 1e-6)
  assert.ok(Math.abs(spaced.longitude - 77.209021) < 1e-6)

  // Unambiguous lng, lat where |first| > 90
  const gisOrder = parseGoogleMapsCoordinates("-122.419416, 37.774929")
  assert.equal(gisOrder.ok, true)
  assert.equal(gisOrder.autoSwapped, true)
  assert.ok(Math.abs(gisOrder.latitude - 37.774929) < 1e-6)
  assert.ok(Math.abs(gisOrder.longitude - -122.419416) < 1e-6)
})

test("parses Google Maps DMS, DDM, and cardinal direction formats", () => {
  const dms = parseGoogleMapsCoordinates(`28°36'50.2"N 77°12'32.5"E`)
  assert.equal(dms.ok, true)
  assert.ok(Math.abs(dms.latitude - (28 + 36 / 60 + 50.2 / 3600)) < 1e-5)
  assert.ok(Math.abs(dms.longitude - (77 + 12 / 60 + 32.5 / 3600)) < 1e-5)

  // Reversed cardinal order (E before S) and curly quotes
  const syd = parseGoogleMapsCoordinates(`151°12’33.5”E, 33°52’07.7”S`)
  assert.equal(syd.ok, true)
  assert.ok(Math.abs(syd.latitude - -(33 + 52 / 60 + 7.7 / 3600)) < 1e-5)
  assert.ok(Math.abs(syd.longitude - (151 + 12 / 60 + 33.5 / 3600)) < 1e-5)

  // Decimal degrees with cardinal letters
  const cardDec = parseGoogleMapsCoordinates("40.7128° N, 74.0060° W")
  assert.equal(cardDec.ok, true)
  assert.ok(Math.abs(cardDec.latitude - 40.7128) < 1e-6)
  assert.ok(Math.abs(cardDec.longitude - -74.006) < 1e-6)
})

test("parses explicit lat/lng labels in any order", () => {
  const labeled = parseGoogleMapsCoordinates("Lon: 77.2090, Lat: 28.6139")
  assert.equal(labeled.ok, true)
  assert.ok(Math.abs(labeled.latitude - 28.6139) < 1e-6)
  assert.ok(Math.abs(labeled.longitude - 77.209) < 1e-6)
})

test("parses Google Maps URLs (prioritizing !3d!4d pin over @viewport)", () => {
  const pinUrl =
    "https://www.google.com/maps/place/India+Gate/@28.612912,77.227321,17z/data=!3m1!4b1!4m6!3m5!1s0x0:0x0!8m2!3d28.612912!4d77.229510"
  const parsedPin = parseGoogleMapsCoordinates(pinUrl)
  assert.equal(parsedPin.ok, true)
  assert.ok(Math.abs(parsedPin.latitude - 28.612912) < 1e-6)
  assert.ok(Math.abs(parsedPin.longitude - 77.22951) < 1e-6)
  assert.equal(parsedPin.zoom, 17)

  const queryUrl = "https://maps.google.com/?q=19.0760,72.8777"
  const parsedQuery = parseGoogleMapsCoordinates(queryUrl)
  assert.equal(parsedQuery.ok, true)
  assert.ok(Math.abs(parsedQuery.latitude - 19.076) < 1e-6)
  assert.ok(Math.abs(parsedQuery.longitude - 72.8777) < 1e-6)
})

test("parses full Google Maps Plus Codes (Open Location Code) and flags short Plus Codes clearly", () => {
  // India Gate full Plus Code: 7JWVJ67H+5R -> ~(28.6129, 77.2295)
  const olc = parseGoogleMapsCoordinates("7JWVJ67H+5R")
  assert.equal(olc.ok, true)
  assert.ok(Math.abs(olc.latitude - 28.6129375) < 1e-4)
  assert.ok(Math.abs(olc.longitude - 77.2295625) < 1e-4)

  // Short Plus Code without 8-char prefix cannot be resolved offline without faking
  const shortOlc = parseGoogleMapsCoordinates("J67H+5R New Delhi")
  assert.equal(shortOlc.ok, false)
  assert.match(shortOlc.error, /Plus Code/i)
})

test("rejects invalid and out-of-range coordinates", () => {
  assert.equal(parseGoogleMapsCoordinates("").ok, false)
  assert.equal(parseGoogleMapsCoordinates("95.0, 200.0").ok, false)
  assert.equal(parseGoogleMapsCoordinates("28°75'00\"N 77°12'00\"E").ok, false)
  assert.equal(parseGoogleMapsCoordinates("28°10'00\"N 77°12'00\"N").ok, false)
})

test("buildDirectCoordinateControlPoints generates valid in-bounds control points preserving exact center and aspect ratio", () => {
  const res = buildDirectCoordinateControlPoints({
    mode: "center",
    width: 1000,
    height: 500,
    center: { latitude: 28.6139, longitude: 77.209 },
    widthMeters: 200,
    rotationDegrees: 0,
  })
  assert.equal(res.ok, true)
  assert.equal(res.points.length, 4)

  for (const pt of res.points) {
    assert.ok(pt.image_x >= 0 && pt.image_x < 1000)
    assert.ok(pt.image_y >= 0 && pt.image_y < 500)
    assert.ok(pt.latitude >= -90 && pt.latitude <= 90)
    assert.ok(pt.longitude >= -180 && pt.longitude <= 180)
  }

  // Top-left is north-west of bottom-right
  const [tl, tr, br, bl] = res.points
  assert.ok(tl.latitude > br.latitude)
  assert.ok(tr.longitude > tl.longitude)
  assert.ok(Math.abs(tl.latitude - tr.latitude) < 1e-12)
  assert.ok(Math.abs(tl.longitude - bl.longitude) < 1e-12)

  // Two-corner placement
  const cornersRes = buildDirectCoordinateControlPoints({
    mode: "corners",
    width: 800,
    height: 600,
    topLeft: { latitude: 28.615, longitude: 77.208 },
    bottomRight: { latitude: 28.613, longitude: 77.211 },
  })
  assert.equal(cornersRes.ok, true)
  assert.equal(cornersRes.points.length, 4)
  assert.ok(Math.abs(cornersRes.points[0].latitude - 28.615) < 1e-9)
  assert.ok(Math.abs(cornersRes.points[0].longitude - 77.208) < 1e-9)
})
