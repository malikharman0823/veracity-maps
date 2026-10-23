export async function loadSessionResources({ ensureSession, loadResources }) {
  await ensureSession()
  return Promise.all(loadResources.map((loadResource) => loadResource()))
}

export function normalizeSavedConfigs(data) {
  if (Array.isArray(data)) return data
  return data?.configs || []
}

export function savedConfigMemberIds(members = []) {
  return members.map((member) => member.dataset_id).filter(Boolean)
}
