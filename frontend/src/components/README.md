# Directory: `frontend/src/components`

Located in [`frontend/src/components/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/components)

This directory contains the primary React UI components for layout navigation, raster visualization, control-point editing, layer management, and dataset tables.

---

## Core Components Summary

| Component | Responsibility |
| :--- | :--- |
| **`TopHeader.jsx`** | Sticky top bar providing breadcrumb navigation, active workspace indicators, and the session termination dialog trigger. |
| **`Sidebar.jsx`** | Application navigation menu displaying the Veracity Maps branding, active view links, and quick upload CTA. |
| **`WorkspaceMap.jsx`** | Primary MapLibre GL map view. Manages raster layers, coordinate transformations, elevator/floor filtering, and dynamic layer opacity. |
| **`WorkspaceLayerPanel.jsx`** | Draggable side drawer displaying active raster layers, opacity sliders (`0%–100%`), visibility toggles, and multi-floor sorting. |
| **`WorkspaceTable.jsx`** | TanStack React Table implementation displaying active datasets with sorting, status badges, thumbnail previews, and action buttons. |
| **`SourceImagePane.jsx`** | High-resolution image canvas used on the georeferencing page for interactive ground control point pixel placement. |
| **`ReferenceMap.jsx`** | Interactive MapLibre GL reference map for georeferencing. Supports Google Maps `Lat, Lng` / DMS / URL / Plus Code search with target pin marker, 1-click draft pairing, ambiguous Lat/Lng swap, preview raster overlay, and `focusTarget` camera synchronization. |
| **`ControlPointList.jsx`** | Tabular editor for inspect, inline edit (`image_x`, `image_y`, Google Maps `lat`, `lng`, or pasted `Lat, Lng` pairs), inline draft coordinate pairing, or delete operations on placed GCP coordinate pairs. |
| **`DatasetCard.jsx`** | Visual grid card summarizing dataset metadata, dimensions, georeferencing status, and export quick-links. |
| **`DatasetInspectionPanel.jsx`** | Slide-out drawer displaying detailed GeoTIFF/PNG metadata, world file matrices, detected CRS, and bounds. |
| **`EndSessionDialog.jsx`** | Confirmation modal for ending the active anonymous session and purging temporary server files. |
| **`UploadBox.jsx`** | Drag-and-drop file upload target supporting single images and ZIP bundles. |
| **`UploadStatus.jsx`** | Upload progress feedback and error state alert banner. |
| **`FilePreviewDialog.jsx`** | Reusable selected-file preview modal. It renders browser-supported image files from an object URL and provides live metadata fallback for ZIP and other formats. |
| **`RasterTransformOverlay.jsx`** | Interactive bounding-box transformer allowing visual scale, rotation, and translation overlays directly on the map. |
| **`FloorTraversalOverlay.jsx`** | Top-corner overlay for multi-floor buildings: climbs floors via buttons/keyboard arrows, displays floor badges (B2, B1, G, 1, 2), building switcher, and quick raster opacity/visibility controls. |

---

## Subdirectory

- [`ui/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/components/ui/README.md): Low-level headless and styled primitives (buttons, inputs, sliders, tables, dialogs).
