export function safeWorkspaceReturn(value, datasetId) {
  const fallback = `/datasets/${datasetId}/workspace?view=database`
  if (!value || typeof value !== "string") return fallback
  if (value.startsWith("//") || value.includes("://")) return fallback
  if (
    (value.startsWith("/datasets/") && value.includes("/workspace")) ||
    value.startsWith("/maps/") ||
    value.startsWith("/projects/") ||
    value.startsWith("/mapspaces/")
  ) {
    return value
  }
  return fallback
}
