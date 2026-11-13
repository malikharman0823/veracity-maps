export async function loadSessionResources({ ensureSession, loadResources }) {
  await ensureSession()
  return Promise.all(loadResources.map((loadResource) => loadResource()))
}

export async function loadMapWorkspace({ ensureSession, loadMap }) {
  await ensureSession()
  return loadMap()
}

export function isBackendSessionId(sessionId) {
  return /^sess_[0-9a-f]{32}$/.test(sessionId || "")
}

export function findDatasetSessionId(sessions, datasetId) {
  return (
    sessions.find(
      (session) =>
        session?.first_dataset_id === datasetId ||
        (Array.isArray(session?.dataset_ids) && session.dataset_ids.includes(datasetId)),
    )?.id || null
  )
}

export async function loadDatasetWorkspace({ datasetId, ensureSession, loadWorkspace }) {
  await ensureSession(datasetId)
  return loadWorkspace(datasetId)
}

export function mapConfigurationMemberIds(members = []) {
  return members.map((member) => member.dataset_id).filter(Boolean)
}
