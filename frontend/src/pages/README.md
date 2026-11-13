# Directory: `frontend/src/pages`

Located in [`frontend/src/pages/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/pages)

This directory contains the top-level route views in the Veracity Maps application.

---

## Route Pages

| Page Component | Route Path | Responsibility |
| :--- | :--- | :--- |
| **`DashboardPage.jsx`** | `/dashboard` | Authenticated user dashboard for owned/shared projects, project invitations, activity, and settings. Collaboration is managed directly through projects; the organization dashboard is removed. |
| **`HomePage.jsx`** | `/` | Application landing dashboard displaying temporary datasets plus persistent `saved_maps` from `/api/maps`; shared maps appear after the current user saves a revision, while deletion remains owner-only and is not tied to session cleanup. |
| **`UploadPage.jsx`** | `/upload` | Dedicated file upload interface supporting PNG, JPEG, and multi-file ZIP bundles with a dynamic selected-file preview, real-time validation, and progress feedback. |
| **`DatasetPage.jsx`** | `/datasets/:id` | Visual Georeferencer Studio. Hosts the dual-pane canvas (source raster on left, reference map on right) for placing, editing, and calculating control points. |
| **`WorkspacePage.jsx`** | `/workspace` and `/maps/:mapId/edit` | Full-screen interactive map and dataset database view. Saving a standalone workspace creates a persistent personal map and replaces the URL; saving an owned or editor-shared map appends a revision to that same map. |
| **`DashboardPage.jsx`** | `/dashboard` | Separates owned, shared, recent, and saved map views. Saved Maps includes owned maps plus accessible shared maps with a revision authored by the signed-in user; owner-only actions are hidden for shared entries. |
| **`GeoreferencePage.jsx`**| `/datasets/:id/georeference` | Dual-pane Visual Georeferencer view supporting interactive GCP pairing, direct Google Maps `Lat, Lng` placement (Center + Ground Width/Rotation or NW/SE Corners), inline coordinate pairing/editing, live raster preview overlay, and publishing to workspace. |
