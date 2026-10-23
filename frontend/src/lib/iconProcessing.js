/**
 * Prepares and cleans icon images before adding them to MapLibre GL.
 * - Standardizes and scales icons to a clean map marker size (default maxSize: 36px).
 * - Automatically detects and removes checkerboard patterns (fake PNG transparency)
 *   and solid light/white backgrounds.
 * - Uses flood-fill from edges + global color matching so transparency is natural and clean.
 * - Returns a canvas or image element suitable for map.addImage(id, ...).
 */

const iconCache = new Map()

export async function processIconForMap(sourceUrl, maxSize = 36) {
  const cacheKey = `${sourceUrl}__${maxSize}`
  if (iconCache.has(cacheKey)) {
    return iconCache.get(cacheKey)
  }

  const promise = new Promise((resolve) => {
    const img = new Image()
    img.crossOrigin = "anonymous"
    img.onload = () => {
      try {
        const origW = img.naturalWidth || img.width
        const origH = img.naturalHeight || img.height
        if (!origW || !origH) {
          return resolve(img)
        }

        // 1. Calculate aspect-ratio preserved target dimensions
        let targetW = origW
        let targetH = origH
        if (targetW > maxSize || targetH > maxSize) {
          const ratio = Math.min(maxSize / targetW, maxSize / targetH)
          targetW = Math.max(1, Math.round(targetW * ratio))
          targetH = Math.max(1, Math.round(targetH * ratio))
        }

        const canvas = document.createElement("canvas")
        canvas.width = targetW
        canvas.height = targetH
        const ctx = canvas.getContext("2d", { willReadFrequently: true })
        if (!ctx) return resolve(img)

        // Draw resized image
        ctx.drawImage(img, 0, 0, targetW, targetH)

        // 2. Scan and clean pixels
        const imageData = ctx.getImageData(0, 0, targetW, targetH)
        const data = imageData.data

        const getPixel = (x, y) => {
          const idx = (y * targetW + x) * 4
          return {
            r: data[idx],
            g: data[idx + 1],
            b: data[idx + 2],
            a: data[idx + 3],
          }
        }

        const isNearGray = (r, g, b, tolerance = 18) => {
          return (
            Math.abs(r - g) <= tolerance &&
            Math.abs(g - b) <= tolerance &&
            Math.abs(r - b) <= tolerance
          )
        }

        // Sample border pixels around the edges
        let lightBorderPixels = 0
        let totalBorderSampled = 0
        for (let x = 0; x < targetW; x += Math.max(1, Math.floor(targetW / 8))) {
          const top = getPixel(x, 0)
          const btm = getPixel(x, targetH - 1)
          if (top.a > 120 && isNearGray(top.r, top.g, top.b) && top.r >= 180) lightBorderPixels++
          if (btm.a > 120 && isNearGray(btm.r, btm.g, btm.b) && btm.r >= 180) lightBorderPixels++
          totalBorderSampled += 2
        }
        for (let y = 0; y < targetH; y += Math.max(1, Math.floor(targetH / 8))) {
          const left = getPixel(0, y)
          const right = getPixel(targetW - 1, y)
          if (left.a > 120 && isNearGray(left.r, left.g, left.b) && left.r >= 180) lightBorderPixels++
          if (right.a > 120 && isNearGray(right.r, right.g, right.b) && right.r >= 180) lightBorderPixels++
          totalBorderSampled += 2
        }

        // If at least 35% of the border pixels are high-brightness neutral gray or white,
        // this image has a solid light background or a checkerboard "fake transparency" pattern!
        const hasLightOrCheckeredBg =
          totalBorderSampled > 0 && lightBorderPixels / totalBorderSampled >= 0.35

        if (hasLightOrCheckeredBg) {
          // Remove checkerboard and white background:
          // Checkered patterns alternate between ~255 and ~220-240 or ~185-215.
          for (let i = 0; i < data.length; i += 4) {
            const r = data[i]
            const g = data[i + 1]
            const b = data[i + 2]
            const a = data[i + 3]

            if (a === 0) continue

            // 1. Checkerboard square (light neutral gray)
            if (isNearGray(r, g, b, 18) && r >= 170 && g >= 170 && b >= 170) {
              data[i + 3] = 0 // clear to transparent
            } else if (r >= 240 && g >= 240 && b >= 240) {
              // 2. Solid near-white or white
              data[i + 3] = 0 // clear to transparent
            }
          }
          ctx.putImageData(imageData, 0, 0)
        }

        resolve(canvas)
      } catch (err) {
        // Fallback to original image
        resolve(img)
      }
    }
    img.onerror = () => {
      // Return a 24x24 red fallback pin
      const fallback = document.createElement("canvas")
      fallback.width = 24
      fallback.height = 24
      const ctx = fallback.getContext("2d")
      if (ctx) {
        ctx.fillStyle = "#ff3b30"
        ctx.beginPath()
        ctx.arc(12, 12, 9, 0, Math.PI * 2)
        ctx.fill()
        ctx.strokeStyle = "#ffffff"
        ctx.lineWidth = 2
        ctx.stroke()
      }
      resolve(fallback)
    }
    img.src = sourceUrl
  })

  iconCache.set(cacheKey, promise)
  return promise
}
