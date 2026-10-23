const FLOOR_TOKEN_REGEX = /^(floor|flr|level|lvl|basement|ground)\b/i

export function isFloorToken(name) {
  return FLOOR_TOKEN_REGEX.test((name || "").trim())
}

/**
 * Resolves the effective building name for a member given the list of all members and buildings.
 * Matches the resolution rules used in WorkspaceLayerPanel.
 */
export function resolveBuildingName(member, allMembers = [], buildings = []) {
  if (!member) return ""

  const realBuildingNames = []
  if (Array.isArray(allMembers)) {
    allMembers.forEach((m) => {
      const bn = (m.building_name || m.dataset?.extracted_metadata?.building_name || "").trim()
      if (bn && !isFloorToken(bn) && !realBuildingNames.includes(bn)) {
        realBuildingNames.push(bn)
      }
    })
  }

  if (Array.isArray(buildings)) {
    buildings.forEach((b) => {
      const bn = (b.name || "").trim()
      if (bn && !isFloorToken(bn) && !realBuildingNames.includes(bn)) {
        realBuildingNames.push(bn)
      }
    })
  }

  let bName = (member.building_name || member.dataset?.extracted_metadata?.building_name || "").trim()
  const mName = (member.name || member.dataset?.filename || "").trim()
  const storagePath = member.dataset?.storage_path || ""

  if (!bName || isFloorToken(bName)) {
    const matchedReal = realBuildingNames.find((rn) => {
      const cleanRn = rn.toLowerCase()
      const underRn = cleanRn.replace(/\s+/g, "_")
      const lowerMName = mName.toLowerCase()
      const lowerStorage = storagePath.toLowerCase()
      return (
        lowerMName.includes(underRn) ||
        lowerMName.includes(cleanRn) ||
        (storagePath && (lowerStorage.includes(underRn) || lowerStorage.includes(cleanRn)))
      )
    })

    if (matchedReal) {
      bName = matchedReal
    } else if (
      realBuildingNames.length > 0 &&
      (member.is_building_floor ||
        isFloorToken(mName) ||
        isFloorToken(member.floor_label || member.dataset?.extracted_metadata?.floor_label))
    ) {
      bName = realBuildingNames[0]
    }
  }

  return bName
}

/**
 * Checks if a member belongs to targetBuildingName.
 * If targetBuildingName is empty/falsy, any member matches.
 * If targetBuildingName is set, member's resolved building must match.
 */
export function isMemberBuildingMatch(member, targetBuildingName, allMembers = [], buildings = []) {
  const target = (targetBuildingName || "").trim().toLowerCase()
  if (!target) return true

  const memberBuilding = resolveBuildingName(member, allMembers, buildings).trim().toLowerCase()
  if (!memberBuilding) return false

  return (
    memberBuilding === target ||
    target.includes(memberBuilding) ||
    memberBuilding.includes(target)
  )
}
