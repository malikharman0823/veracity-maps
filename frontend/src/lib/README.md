# Directory: `frontend/src/lib`

Located in [`frontend/src/lib/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/lib)

This directory contains utility functions and pure helper logic supporting UI styling and navigation.

---

## Files in Directory

### 1. `utils.js`
- **Purpose**: Exports the canonical `cn(...inputs)` helper combining `clsx` and `tailwind-merge` for safe conditional class name composition.

### 2. `workspaceNavigation.js`
- **Purpose**: Encapsulates workspace URL route generation and view mode switching (`"map"` vs `"database"`).
- **Functions**:
  - `workspaceViewPath(requestedView, id)`: Generates the route path for switching between tabular dataset view and interactive map view.

### 3. `workspaceNavigation.test.js`
- **Purpose**: Unit tests verifying route generation for default, map, and database view modes. Run via `npm.cmd test`.
