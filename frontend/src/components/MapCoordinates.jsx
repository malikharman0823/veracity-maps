import React from "react"

function coordinateText(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number.toFixed(6) : "—"
}

export default function MapCoordinates({ coordinate }) {
  return (
    <output className="map-coordinates dark:bg-[#202020]/95 dark:border-[#2e2e2e] dark:text-white" aria-live="polite">
      <span>Lat: {coordinateText(coordinate?.latitude)}</span>
      <span>Lon: {coordinateText(coordinate?.longitude)}</span>
    </output>
  )
}
