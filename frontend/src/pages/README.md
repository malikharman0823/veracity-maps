# Directory: `frontend/src/pages`

Located in [`frontend/src/pages/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/pages)

This directory contains the top-level route views in the Veracity Maps application.

---

## Route Pages

| Page Component | Route Path | Responsibility |
| :--- | :--- | :--- |
| **`HomePage.jsx`** | `/` | Application landing dashboard displaying dataset summaries, recent activity, quick upload dropzone, and workspace entry CTA. |
| **`UploadPage.jsx`** | `/upload` | Dedicated file upload interface supporting PNG, JPEG, and multi-file ZIP bundles with real-time validation and progress feedback. |
| **`DatasetPage.jsx`** | `/datasets/:id` | Visual Georeferencer Studio. Hosts the dual-pane canvas (source raster on left, reference map on right) for placing, editing, and calculating control points. |
| **`WorkspacePage.jsx`** | `/workspace` | Full-screen interactive map and dataset database view. Features layer ordering, building floor selectors, and opacity adjustment. |
| **`GeoreferencePage.jsx`**| Sub-route | Specialized control point management view for batch GCP coordinate inputs. |
