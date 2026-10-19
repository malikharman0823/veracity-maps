# Directory: `frontend/src/hooks`

Located in [`frontend/src/hooks/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/hooks)

This directory contains custom React hooks that encapsulate data fetching, responsive viewport detection, and state synchronization.

---

## Files in Directory

### 1. `useWorkspace.js`
- **Purpose**: Central state management hook for the active workspace.
- **Responsibilities**:
  - Automatically fetches and polls datasets for the active session.
  - Exposes dataset mutations: `deleteDataset`, `updateLayerOpacity`, `toggleLayerVisibility`, and `reorderLayers`.
  - Normalizes layer hierarchies and building floor levels (`floor_level`, `is_building_floor`).
  - Returns loading indicators (`loading`, `error`, `datasets`, `layers`).

### 2. `use-mobile.js`
- **Purpose**: Responsive layout hook detecting mobile viewport dimensions (`window.innerWidth < 768px`) for adaptive sidebar collapsing and sheet drawer rendering.
