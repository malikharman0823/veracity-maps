# Veracity Maps — Frontend Application

The **Veracity Maps Frontend** is a modern Single Page Application (SPA) built with React 19, Vite 7, TailwindCSS 4, and MapLibre GL JS. It provides an intuitive canvas for inspecting uploaded raster datasets, placing ground control points (GCPs), previewing georeferenced raster overlays, configuring layer opacities, and downloading project packages.

---

## 1. Directory Structure

```text
frontend/
├── src/
│   ├── components/      # UI components (Header, Sidebar, WorkspaceMap, LayerPanel, Dialogs)
│   │   └── ui/          # Primitives: buttons, dialogs, dropdowns, separators, sliders, table
│   ├── hooks/           # Custom React hooks (useWorkspace, use-mobile)
│   ├── lib/             # Helper utilities and navigation state management
│   ├── pages/           # Application views (HomePage, UploadPage, DatasetPage, WorkspacePage)
│   ├── api.js           # Centralized Axios HTTP client and session management
│   ├── App.jsx          # Route configuration
│   ├── index.css        # Global TailwindCSS styling and design tokens
│   └── main.jsx         # Application DOM mount
├── index.html           # HTML entry point (title: "Veracity Maps")
├── package.json         # Package manifest (name: "veracity-maps")
└── vite.config.js       # Vite build & proxy configuration
```

---

## 2. Key Technologies

- **React 19 & React Router 7**: Declarative component hierarchy and client-side route navigation.
- **MapLibre GL JS**: High-performance WebGL-based map renderer displaying baseline maps and custom georeferenced raster image sources.
- **TailwindCSS 4**: Responsive, theme-aware utility styling with polished dark-mode palette.
- **Axios**: HTTP communication with automatic `X-Session-ID` injection and unified error normalization.

---

## 3. Development & Build Commands

All commands should be executed from the `frontend/` directory:

### Start Development Server (Port 5173)
```powershell
cmd.exe /c "npm.cmd run dev"
```

### Production Build
```powershell
cmd.exe /c "npm.cmd run build"
```
Build output is generated into `frontend/dist/`.

### Run Unit Tests
```powershell
cmd.exe /c "npm.cmd test"
```
