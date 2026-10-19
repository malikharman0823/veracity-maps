# Directory: `frontend/src`

Located in [`frontend/src/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src)

This directory contains the primary JavaScript and CSS source code for the Veracity Maps client-side application.

---

## Files in Directory

### 1. `main.jsx`
- **Purpose**: Application entry point. Mounts the root React component tree into the `#root` DOM element using `ReactDOM.createRoot`. Imports `@fontsource-variable/inter` typography and `index.css`.

### 2. `App.jsx`
- **Purpose**: Defines application route mapping using `BrowserRouter` and `Routes`:
  - `/`: [`pages/HomePage`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/pages/README.md) (Workspace dataset listing and quick upload CTA).
  - `/upload`: [`pages/UploadPage`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/pages/README.md) (Interactive drag-and-drop file upload).
  - `/datasets/:id`: [`pages/DatasetPage`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/pages/README.md) (Visual georeferencing canvas & GCP editor).
  - `/workspace`: [`pages/WorkspacePage`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/pages/README.md) (Map viewport & multi-layer management).

### 3. `api.js`
- **Purpose**: Centralized Axios API client managing backend communication and session isolation.
- **Key Exports & Functions**:
  - `SESSION_STORAGE_KEY = "veracity_maps_active_session_id"`: Key for persisting anonymous session tokens across page reloads.
  - `getActiveSessionId()`: Retrieves token with fallback to legacy `maptiler_active_session_id`.
  - `setActiveSessionId(sessionId)`: Stores token to local/session storage.
  - `clearActiveSessionId()`: Clears session token on session termination.
  - `api`: Configured Axios instance with request interceptor automatically injecting `X-Session-ID: <session_id>`.
  - Endpoint functions: `fetchSession`, `createSession`, `endSession`, `fetchDatasets`, `uploadDataset`, `deleteDataset`, `fetchControlPoints`, `createControlPoint`, `updateControlPoint`, `deleteControlPoint`, `calculateGeoreference`, `fetchLayers`, `updateLayer`, `downloadProjectExport`.

### 4. `index.css`
- **Purpose**: Core design tokens, TailwindCSS configuration, Apple-inspired minimalist dark and light mode UI themes, glassmorphism blur tokens, and custom scrollbars.
