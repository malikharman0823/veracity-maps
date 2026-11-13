export function nextExpandedLayerId(currentLayerId, clickedLayerId) {
  return currentLayerId === clickedLayerId ? null : clickedLayerId
}
