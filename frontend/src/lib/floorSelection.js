export function applyFloorSelection({
  level,
  buildingName,
  members = [],
  setFloorLevel,
  setBuildingName,
  updateMember,
}) {
  const numLevel = Number(level)
  setFloorLevel(numLevel)

  if (buildingName !== undefined) {
    setBuildingName(buildingName || "")
  }

  members
    .filter((member) => {
      const floorLevel = Number(
        member.floor_level ?? member.dataset?.extracted_metadata?.floor_level ?? 0,
      )
      if (floorLevel !== numLevel) return false
      if (
        buildingName
        && member.building_name
        && member.building_name.trim().toLowerCase() !== buildingName.trim().toLowerCase()
      ) {
        return false
      }
      return member.visible === false || member.raster_visible === false
    })
    .forEach((member) => {
      updateMember(member.dataset_id, { visible: true, raster_visible: true })
    })
}
