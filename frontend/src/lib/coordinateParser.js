const OLC_ALPHABET = "23456789CFGHJMPQRVWX"

function validateLatLng(latitude, longitude, extra = {}) {
  if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) {
    return {
      ok: false,
      error: "Coordinates must be valid numbers (e.g. 28.6139, 77.2090).",
    }
  }
  if (Math.abs(latitude) > 90 || Math.abs(longitude) > 180) {
    return {
      ok: false,
      error: "Latitude must be between -90 and 90, and longitude between -180 and 180.",
    }
  }
  return {
    ok: true,
    latitude,
    longitude,
    ambiguous: false,
    autoSwapped: false,
    ...extra,
  }
}

function clampZoom(rawZoom, fallback = 17) {
  const z = Number(rawZoom)
  if (!Number.isFinite(z)) return fallback
  return Math.max(1, Math.min(21, Math.round(z * 10) / 10))
}

function tryParseGoogleMapsUrl(text) {
  if (!/^https?:\/\//i.test(text) && !/google\.[a-z.]+\/maps/i.test(text) && !/!3d-?\d/i.test(text)) {
    return null
  }

  let decoded = text
  try {
    decoded = decodeURIComponent(text)
  } catch {
    // Keep raw text when percent decoding fails
  }

  const atMatch = decoded.match(/@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)(?:,(\d+(?:\.\d+)?)z)?/i)
  const extractedZoom = atMatch?.[3] ? clampZoom(atMatch[3], 17) : 17

  // 1. Exact place pin (!3d<lat>!4d<lng>) takes priority over camera viewport (@lat,lng)
  const pinMatches = [...decoded.matchAll(/!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)/gi)]
  if (pinMatches.length > 0) {
    const lastPin = pinMatches[pinMatches.length - 1]
    return validateLatLng(Number(lastPin[1]), Number(lastPin[2]), {
      format: "google_maps_url",
      zoom: extractedZoom,
    })
  }

  // 2. Query parameter (?q=lat,lng or ?ll=lat,lng or ?center=lat,lng)
  const queryMatch = decoded.match(
    /[?&](?:q|query|ll|sll|center|destination|origin|daddr|saddr)=\s*([+-]?\d+(?:\.\d+)?)\s*,\s*([+-]?\d+(?:\.\d+)?)/i,
  )
  if (queryMatch) {
    return validateLatLng(Number(queryMatch[1]), Number(queryMatch[2]), {
      format: "google_maps_url",
      zoom: extractedZoom,
    })
  }

  // 3. Viewport center (@lat,lng,17z)
  if (atMatch) {
    return validateLatLng(Number(atMatch[1]), Number(atMatch[2]), {
      format: "google_maps_url",
      zoom: extractedZoom,
    })
  }

  return {
    ok: false,
    error: "No latitude/longitude coordinates found in this URL. Right-click the place in Google Maps to copy Lat, Lng.",
  }
}

function decodeFullPlusCode(code) {
  const upper = code.toUpperCase()
  const plusIndex = upper.indexOf("+")
  if (plusIndex !== 8) return null

  const prefix = upper.slice(0, 8)
  const suffix = upper.slice(9)

  // Validate zero padding if present
  const firstZero = prefix.indexOf("0")
  if (firstZero !== -1) {
    if (firstZero < 2 || firstZero % 2 !== 0 || suffix.length > 0) return null
    for (let i = firstZero; i < 8; i++) {
      if (prefix[i] !== "0") return null
    }
  }

  const digits = (prefix.replace(/0+$/, "") + suffix)
  if (digits.length < 2) return null

  for (const ch of digits) {
    if (!OLC_ALPHABET.includes(ch)) return null
  }

  let lat = 0
  let lng = 0
  let latRes = 20
  let lngRes = 20

  const pairCount = Math.min(5, Math.floor(digits.length / 2))
  for (let p = 0; p < pairCount; p++) {
    if (p > 0) {
      latRes /= 20
      lngRes /= 20
    }
    const latVal = OLC_ALPHABET.indexOf(digits[p * 2])
    const lngVal = OLC_ALPHABET.indexOf(digits[p * 2 + 1])
    lat += latVal * latRes
    lng += lngVal * lngRes
  }

  // Grid refinement digits beyond 10 characters (5 rows x 4 columns)
  for (let i = 10; i < digits.length; i++) {
    latRes /= 5
    lngRes /= 4
    const val = OLC_ALPHABET.indexOf(digits[i])
    const row = Math.floor(val / 4)
    const col = val % 4
    lat += row * latRes
    lng += col * lngRes
  }

  const centerLat = lat - 90 + latRes / 2
  const centerLng = lng - 180 + lngRes / 2
  const zoom = digits.length >= 10 ? 18 : 15

  return validateLatLng(centerLat, centerLng, {
    format: "plus_code",
    zoom,
  })
}

function tryParsePlusCode(text) {
  if (!text.includes("+")) return null

  const fullMatch = text.match(
    /(?:^|[\s,(])([23456789CFGHJMPQRVWX0]{8}\+[23456789CFGHJMPQRVWX]{0,7})(?=$|[\s,)])/i,
  )
  if (fullMatch) {
    const decoded = decodeFullPlusCode(fullMatch[1])
    if (decoded) return decoded
  }

  const shortMatch = text.match(
    /(?:^|[\s,(])([23456789CFGHJMPQRVWX]{4,6}\+[23456789CFGHJMPQRVWX]{2,5})(?=$|[\s,)])/i,
  )
  if (shortMatch) {
    return {
      ok: false,
      error:
        "Short Plus Code requires a city lookup. Right-click the spot in Google Maps to copy decimal Lat, Lng (e.g. 28.6139, 77.2090) or paste the full 8-character Plus Code.",
    }
  }

  return null
}

function tryParseLabeledCoordinates(text) {
  if (!/\b(?:lat(?:itude)?|lon(?:gitude)?|lng)\b/i.test(text)) return null

  const latMatch = text.match(/\blat(?:itude)?\s*[:=]?\s*([+-]?\d+(?:\.\d+)?)/i)
  const lngMatch = text.match(/\b(?:lon(?:gitude)?|lng)\s*[:=]?\s*([+-]?\d+(?:\.\d+)?)/i)

  if (!latMatch || !lngMatch) {
    return {
      ok: false,
      error: "Provide both Latitude and Longitude values (e.g. Lat: 28.6139, Lng: 77.2090).",
    }
  }

  return validateLatLng(Number(latMatch[1]), Number(lngMatch[1]), {
    format: "labeled",
  })
}

function parseDmsComponent(part) {
  const normalized = part
    .trim()
    .replace(/[º°]/g, "°")
    .replace(/[’′']/g, "'")
    .replace(/[”″"]/g, '"')

  const cardinals = normalized.match(/[NSEW]/gi)
  if (!cardinals || cardinals.length !== 1) return null
  const hemisphere = cardinals[0].toUpperCase()

  const numbers = normalized.match(/[+-]?\d+(?:\.\d+)?/g)
  if (!numbers || numbers.length < 1 || numbers.length > 3) return null

  const deg = Number(numbers[0])
  const min = numbers.length >= 2 ? Number(numbers[1]) : 0
  const sec = numbers.length >= 3 ? Number(numbers[2]) : 0

  if (!Number.isFinite(deg) || !Number.isFinite(min) || !Number.isFinite(sec)) return null
  if (deg < 0 || min < 0 || min >= 60 || sec < 0 || sec >= 60) {
    return { invalidDms: true }
  }

  const sign = hemisphere === "S" || hemisphere === "W" ? -1 : 1
  const value = sign * (Math.abs(deg) + min / 60 + sec / 3600)
  const axis = hemisphere === "N" || hemisphere === "S" ? "lat" : "lng"

  return { axis, value }
}

function tryParseCardinalOrDms(text) {
  if (!/[NSEW]/i.test(text)) return null

  const normalized = text
    .trim()
    .replace(/^[([ ]+|[)\]]+$/g, "")
    .replace(/[º°]/g, "°")
    .replace(/[’′']/g, "'")
    .replace(/[”″"]/g, '"')

  // Split into two cardinal coordinate tokens:
  // Either separated by comma/semicolon, or right after a trailing N/S/E/W token
  let parts = normalized.split(/\s*[,;]\s*/).filter(Boolean)
  if (parts.length !== 2) {
    const trailingSplit = normalized.match(
      /^(.+?[NSEW])\s+(.+)$/i,
    )
    if (trailingSplit) {
      parts = [trailingSplit[1], trailingSplit[2]]
    }
  }

  if (parts.length !== 2) return null

  const first = parseDmsComponent(parts[0])
  const second = parseDmsComponent(parts[1])
  if (!first || !second) return null

  if (first.invalidDms || second.invalidDms) {
    return {
      ok: false,
      error: "Invalid DMS value: minutes and seconds must be between 0 and 59.999.",
    }
  }

  if (first.axis === second.axis) {
    return {
      ok: false,
      error: "Provide one N/S latitude and one E/W longitude coordinate.",
    }
  }

  const latitude = first.axis === "lat" ? first.value : second.value
  const longitude = first.axis === "lng" ? first.value : second.value

  return validateLatLng(latitude, longitude, {
    format: "dms",
  })
}

export function parseGoogleMapsCoordinates(input, options = {}) {
  const order = options.order === "lng-lat" ? "lng-lat" : "lat-lng"
  if (typeof input !== "string" || !input.trim()) {
    return {
      ok: false,
      error: "Enter Google Maps Lat, Lng coordinates (e.g. 28.6139, 77.2090).",
    }
  }

  const trimmed = input.trim()

  const urlResult = tryParseGoogleMapsUrl(trimmed)
  if (urlResult) return urlResult

  const plusResult = tryParsePlusCode(trimmed)
  if (plusResult) return plusResult

  const labeledResult = tryParseLabeledCoordinates(trimmed)
  if (labeledResult) return labeledResult

  const dmsResult = tryParseCardinalOrDms(trimmed)
  if (dmsResult) return dmsResult

  // Plain decimal pair: "lat, lng", "(lat, lng)", "lat lng", "lat; lng"
  const cleaned = trimmed
    .replace(/^[([ ]+|[)\]]+$/g, "")
    .replace(/[º°]/g, "")
    .trim()

  const pairMatch = cleaned.match(
    /^([+-]?\d+(?:\.\d+)?)\s*(?:[,;/]|\s+)\s*([+-]?\d+(?:\.\d+)?)$/,
  )
  if (!pairMatch) {
    return {
      ok: false,
      error:
        "Unrecognized format. Enter Google Maps Lat, Lng (e.g. 28.6139, 77.2090 or 28°36'50.2\"N 77°12'32.5\"E).",
    }
  }

  const first = Number(pairMatch[1])
  const second = Number(pairMatch[2])

  if (!Number.isFinite(first) || !Number.isFinite(second)) {
    return {
      ok: false,
      error: "Coordinates must be valid numbers.",
    }
  }

  let latitude = order === "lat-lng" ? first : second
  let longitude = order === "lat-lng" ? second : first
  let autoSwapped = false

  // If the candidate latitude exceeds 90° (up to 180°) while candidate longitude is within [-90, 90],
  // automatically swap since latitude can never exceed 90°.
  if (Math.abs(latitude) > 90 && Math.abs(latitude) <= 180 && Math.abs(longitude) <= 90) {
    const tmp = latitude
    latitude = longitude
    longitude = tmp
    autoSwapped = true
  }

  const validated = validateLatLng(latitude, longitude, {
    format: "decimal",
    autoSwapped,
  })
  if (!validated.ok) return validated

  validated.ambiguous =
    !autoSwapped &&
    Math.abs(latitude) <= 90 &&
    Math.abs(longitude) <= 90 &&
    Math.abs(latitude - longitude) > 1e-7

  return validated
}

/**
 * Computes WGS84 ellipsoidal meters per degree of latitude and longitude at a given latitude.
 */
export function metersPerDegreeAtLatitude(latitudeDegrees) {
  const phi = (latitudeDegrees * Math.PI) / 180
  const metersPerDegLat =
    111132.92 - 559.82 * Math.cos(2 * phi) + 1.175 * Math.cos(4 * phi)
  const metersPerDegLon = Math.max(
    1e-3,
    111412.84 * Math.cos(phi) - 93.5 * Math.cos(3 * phi),
  )
  return { metersPerDegLat, metersPerDegLon }
}

/**
 * Generates 4 corner control points strictly within [0, width) x [0, height)
 * so that the raster overlays accurately at the desired Google Maps coordinates
 * with preserved aspect ratio (center mode) or exact corner bounds (corners mode).
 */
export function buildDirectCoordinateControlPoints({
  mode = "center",
  width,
  height,
  center,
  widthMeters = 100,
  rotationDegrees = 0,
  topLeft,
  bottomRight,
}) {
  const w = Number(width)
  const h = Number(height)
  if (!Number.isFinite(w) || !Number.isFinite(h) || w < 2 || h < 2) {
    return {
      ok: false,
      error: "Source image dimensions are not loaded yet.",
    }
  }

  const maxPixelX = w - 1
  const maxPixelY = h - 1

  if (mode === "corners") {
    if (!topLeft || !bottomRight) {
      return {
        ok: false,
        error: "Enter both Top-Left and Bottom-Right Lat, Lng coordinates.",
      }
    }
    const north = Math.max(topLeft.latitude, bottomRight.latitude)
    const south = Math.min(topLeft.latitude, bottomRight.latitude)
    const west = Math.min(topLeft.longitude, bottomRight.longitude)
    const east = Math.max(topLeft.longitude, bottomRight.longitude)

    if (north - south < 1e-7 || east - west < 1e-7) {
      return {
        ok: false,
        error: "Top-Left and Bottom-Right coordinates must define a non-zero area.",
      }
    }

    const pixelLon = (east - west) / w
    const pixelLat = (north - south) / h
    const project = (ix, iy) => ({
      image_x: ix,
      image_y: iy,
      longitude: west + ix * pixelLon,
      latitude: north - iy * pixelLat,
    })

    return {
      ok: true,
      center: {
        latitude: (north + south) / 2,
        longitude: (west + east) / 2,
      },
      points: [
        project(0, 0),
        project(maxPixelX, 0),
        project(maxPixelX, maxPixelY),
        project(0, maxPixelY),
      ],
    }
  }

  // Default: mode === "center"
  if (!center || !Number.isFinite(center.latitude) || !Number.isFinite(center.longitude)) {
    return {
      ok: false,
      error: "Enter a valid center Lat, Lng coordinate from Google Maps.",
    }
  }

  const spanMeters = Number(widthMeters)
  if (!Number.isFinite(spanMeters) || spanMeters <= 0 || spanMeters > 5_000_000) {
    return {
      ok: false,
      error: "Map ground width must be between 1 and 5,000,000 meters.",
    }
  }

  const rotDeg = Number(rotationDegrees) || 0
  const rad = (rotDeg * Math.PI) / 180
  const cosR = Math.cos(rad)
  const sinR = Math.sin(rad)

  const metersPerPx = spanMeters / w
  const cx = w / 2
  const cy = h / 2
  const { metersPerDegLat, metersPerDegLon } = metersPerDegreeAtLatitude(center.latitude)

  const project = (ix, iy) => {
    const eastLocal = (ix - cx) * metersPerPx
    const northLocal = -(iy - cy) * metersPerPx
    const eastMeters = eastLocal * cosR + northLocal * sinR
    const northMeters = -eastLocal * sinR + northLocal * cosR
    return {
      image_x: ix,
      image_y: iy,
      longitude: center.longitude + eastMeters / metersPerDegLon,
      latitude: center.latitude + northMeters / metersPerDegLat,
    }
  }

  const candidatePoints = [
    project(0, 0),
    project(maxPixelX, 0),
    project(maxPixelX, maxPixelY),
    project(0, maxPixelY),
  ]

  for (const pt of candidatePoints) {
    if (Math.abs(pt.latitude) > 90 || Math.abs(pt.longitude) > 180) {
      return {
        ok: false,
        error: "Map footprint extends beyond valid geographic bounds ([-90, 90] lat, [-180, 180] lng).",
      }
    }
  }

  return {
    ok: true,
    center: {
      latitude: center.latitude,
      longitude: center.longitude,
    },
    points: candidatePoints,
  }
}
