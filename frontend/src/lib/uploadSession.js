export function syncUploadSession(response, setSessionId) {
  const headers = response?.headers
  const sessionId =
    (typeof headers?.get === "function" ? headers.get("x-session-id") : null) ||
    headers?.["x-session-id"] ||
    headers?.["X-Session-ID"] ||
    null

  if (sessionId) setSessionId(sessionId)
  return sessionId
}
