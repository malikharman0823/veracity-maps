export async function persistMapConfiguration(options, dependencies) {
  const {
    mapId,
    payload,
    name,
    expectedRevision = 0,
    activeView = "map",
  } = options
  const { saveMapConfig, createMapFromConfiguration } = dependencies

  if (mapId) {
    const result = await saveMapConfig(mapId, payload, name, expectedRevision)
    return { ...result, navigateTo: null }
  }

  const result = await createMapFromConfiguration(payload)
  const createdMapId = result?.map?.id
  if (!createdMapId) {
    throw new Error("The server did not return the persistent map ID.")
  }
  return {
    ...result,
    navigateTo: `/maps/${encodeURIComponent(createdMapId)}/edit?view=${encodeURIComponent(activeView)}`,
  }
}
