export function resolveSavedMaps(mapsData) {
  if (Array.isArray(mapsData?.saved_maps)) {
    return mapsData.saved_maps
  }
  return []
}

export function canDeleteSavedMap(mapItem, user) {
  return Boolean(mapItem?.owner_user_id && user?.id && mapItem.owner_user_id === user.id)
}
