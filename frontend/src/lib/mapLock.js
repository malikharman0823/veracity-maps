export function lockHolderNameFromLock(lock) {
  const holderName = lock?.user_name
  return typeof holderName === "string" && holderName.trim()
    ? holderName.trim()
    : "another collaborator"
}

export function lockHeldByAnotherUser(status, currentUserId) {
  const lock = status?.active ? status.lock : null
  if (!lock || !currentUserId || lock.user_id === currentUserId) return null
  return lock
}

export function lockHolderNameFromError(error) {
  const data = error?.response?.data
  const details = data?.error?.details

  const candidates = [
    details && typeof details === "object" ? details.user_name : null,
    typeof details === "string" ? details : null,
    data?.data?.user_name,
  ]

  const holderName = candidates.find((value) => typeof value === "string" && value.trim())
  return holderName?.trim() || "another collaborator"
}
