# Veracity Maps

**Veracity Maps** is a full-fledged geospatial raster & vector management, visual georeferencing, and **Custom Web Tile** publishing application. Built with a single-binary Go backend (`net/http`), a React 19 / Vite / TailwindCSS / MapLibre GL frontend, and a Python GIS bridge (`rasterio`, `pyogrio`, `shapely`, `OpenCV`, `ezdxf`, `PyMuPDF`), it runs as a self-contained web application on Windows, macOS, and Linux — both locally and across a LAN for external clients such as **Viewscape**.

---

## Key Capabilities

- **Full-Fledged Single-Server Application**: The compiled Go server serves the production React UI, REST API, local MapLibre GL assets, and live XYZ raster/vector/composite PNG tiles from a single port (`:8000`) with zero external CDN dependencies on LAN.
- **Private User-Isolated Workspaces & MSSQL 22 Row-Level Security (RLS)**:
  - Each registered account operates in an independent private workspace (no cross-user sharing or collaboration surfaces).
  - SQL Server 2022 RLS policy ([`019_rls_user_isolation.sql`](backend/migrations/019_rls_user_isolation.sql)) enforces `FILTER` + `BLOCK` predicates (`AFTER INSERT`, `BEFORE UPDATE`, `AFTER UPDATE`, `BEFORE DELETE`) via `SESSION_CONTEXT(N'user_id')` across all 12 user-owned tables, paired with pooled connection cleanup in [`backend/db/rls.go`](backend/db/rls.go).
  - Per-user private file storage ([`backend/storage/private_storage.go`](backend/storage/private_storage.go)) isolates files under `storage/private/<owner_id>/projects/<project_id>/{uploads,processed,exports,assets,maps}` with path traversal protection and per-user storage quotas.
- **Universal GIS Ingestion**: Upload rasters (`PNG`, `JPG`, `GeoTIFF`), vector datasets (`GeoPackage .gpkg`, `GeoJSON`, `Shapefile .zip`, `KML`), multi-page `PDF` floorplans, `CAD (.dxf)` drawings, `MBTiles`, and multi-dataset `.zip` archives.
- **Interactive Georeferencing & Multi-Floor Management**: Place up to 20 ground control points (GCPs) with 2-point similarity or 3–20-point affine least-squares transforms, apply 4 essential raster enhancements, manage multi-floor buildings, and place custom geographic icon markers.
- **Dynamic Custom Web Tile Publishing & Local XYZ Tile ZIP Export**:
  - Publish any combination of rasters and vector layers (polygons, lines, and points) into live `256×256` composite PNG tiles (`z0–z18+`) via `GET /tiles/{map_id}/composite/{z}/{x}/{y}.png`.
  - Export offline `p3_webtiles_xyz`-compatible XYZ tile ZIP packages via `POST /api/export/custom-local-tiles`.
  - Automatically generates Viewscape-ready URLs using `{0}/{1}/{2}` placeholders and your machine's LAN IP:
    ```text
    http://<your-lan-ip>:8000/tiles/<map_id>/composite/{0}/{1}/{2}.png
    ```
  - Includes a built-in interactive Published Map Viewer at `http://<your-lan-ip>:8000/maps/<map_id>`.
- **Default World Base Map**: Uses the default OpenStreetMap World base layer cleanly across workspaces and published maps.

---

## 1. Prerequisites (For Any New Device)

Install the following tools before running the installer:

| Tool | Minimum Version | Download |
|---|---|---|
| **Git** | 2.30+ | https://git-scm.com/downloads |
| **Go** | 1.22+ | https://go.dev/dl/ |
| **Node.js & npm** | Node 20+ LTS | https://nodejs.org/ |
| **Python** | 3.10+ (with `venv` & `pip`) | https://www.python.org/downloads/ |

> **Note (Optional Database):** Microsoft SQL Server 2022 is optional for local/LAN usage (`APP_ENV=development` automatically uses local file-backed storage under `backend/storage/`). SQL Server is required only when running in `APP_ENV=production`.

---

## 2. Quick Installation (Recommended)

### Windows (PowerShell)

```powershell
# 1. Clone the repository
git clone https://github.com/malikharman0823/veracity-maps.git
cd veracity-maps

# 2. Run the one-command installer (sets up .env, Python .venv, builds React UI & Go binary)
powershell -ExecutionPolicy Bypass -File scripts\install.ps1

# 3. Start the full application
powershell -ExecutionPolicy Bypass -File scripts\start.ps1
```

### macOS / Linux (Bash)

```bash
# 1. Clone the repository
git clone https://github.com/malikharman0823/veracity-maps.git
cd veracity-maps

# 2. Run the one-command installer (sets up .env, Python .venv, builds React UI & Go binary)
bash scripts/install.sh

# 3. Start the full application
bash scripts/start.sh
```

Once started, open the application in your browser:
- **This device:** `http://localhost:8000`
- **Other devices on the same Wi-Fi/LAN:** `http://<your-lan-ip>:8000` (printed automatically on startup)

---

## 3. Manual Step-by-Step Installation

If you prefer to run each step manually instead of using the installer script:

### Step 1 — Clone & Create Environment Files
```powershell
git clone https://github.com/malikharman0823/veracity-maps.git
cd veracity-maps
Copy-Item .env.example .env
Copy-Item frontend\.env.example frontend\.env
```
*(On macOS/Linux use `cp .env.example .env && cp frontend/.env.example frontend/.env`.)*

### Step 2 — Set Up the Python GIS Bridge
**Windows:**
```powershell
python -m venv backend\.venv
backend\.venv\Scripts\python.exe -m pip install --upgrade pip
backend\.venv\Scripts\python.exe -m pip install -r backend\requirements.txt
```

**macOS / Linux:**
```bash
python3 -m venv backend/.venv
backend/.venv/bin/python -m pip install --upgrade pip
backend/.venv/bin/python -m pip install -r backend/requirements.txt
```

### Step 3 — Build the Production Frontend UI
**Windows:**
```powershell
cd frontend
cmd.exe /c "npm.cmd install"
$env:VITE_API_URL = "same-origin"
cmd.exe /c "npm.cmd run build"
Remove-Item Env:\VITE_API_URL
cd ..
```

**macOS / Linux:**
```bash
cd frontend
npm install
VITE_API_URL=same-origin npm run build
cd ..
```

### Step 4 — Compile & Run the Go Application
**Windows:**
```powershell
cd backend
go mod download
go build -o veracity-maps.exe .
.\veracity-maps.exe
```

**macOS / Linux:**
```bash
cd backend
go mod download
go build -o veracity-maps .
./veracity-maps
```

---

## 4. Configuration Guide

### Root Backend Configuration (`.env`)

Edit `.env` in the repository root to customize server behavior:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8000` | HTTP port for the application, API, and tile server. |
| `HOST` | `0.0.0.0` | Bind address (`0.0.0.0` allows LAN devices & Viewscape to connect). |
| `APP_ENV` | `development` | `development` allows local file-backed storage when MSSQL is offline; `production` requires a live MSSQL database. |
| `APP_PUBLIC_URL` | `http://localhost:8000` | Base URL used in emails and share links. Set to `http://<your-lan-ip>:8000` or your domain for external access. |
| `FRONTEND_DIST_DIR` | `../frontend/dist` | Optional custom path to the built React SPA directory. |
| `SESSION_STORAGE_DIR` | `temp/sessions` | Ephemeral per-session working directory (relative to `backend/`). |
| `UPLOAD_DIR` | `storage/uploads` | Uploaded dataset storage path. |
| `PROCESSED_DIR` | `storage/processed` | Processed rasters and tile caches path. |
| `MAX_UPLOAD_SIZE_BYTES` | `524288000` | Maximum upload size in bytes (default `500 MB`). |
| `WORKSPACE_TTL` | `24h` | Idle timeout before temporary workspaces are cleaned up. |
| `BASEMAP_TILE_URL` | `https://tile.openstreetmap.org/{z}/{x}/{y}.png` | Default World base map tile template. |
| `BASEMAP_ATTRIBUTION` | `© OpenStreetMap contributors` | Attribution shown for the default World base map. |

#### Optional: Microsoft SQL Server (Production Persistence)
By default in `APP_ENV=development`, if SQL Server is not running on port `1433`, Veracity Maps automatically uses local storage (`backend/storage/`). To enable durable MSSQL persistence:
```env
APP_ENV=production
MSSQL_CONNECTION_STRING=sqlserver://<user>:<password>@<host>:1433?database=veracity_maps
```
*(See [DATABASE_SETUP.md](DATABASE_SETUP.md) for full SQL Server setup instructions.)*

#### Optional: Outbound Email (OTP Verification & Invitations)
Configure any standard SMTP provider (Gmail App Password, Outlook, Brevo, SendGrid, or local Mailpit):
```env
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-16-char-app-password
SMTP_FROM_EMAIL=no-reply@veracitymaps.com
SMTP_USE_TLS=false
```

### Frontend Configuration (`frontend/.env`)

| Variable | Production / Single-Server | Separate Vite Dev Mode |
|---|---|---|
| `VITE_API_URL` | `same-origin` | `http://localhost:8000` |
| `VITE_BASEMAP_TILE_URL` | `https://tile.openstreetmap.org/{z}/{x}/{y}.png` | `https://tile.openstreetmap.org/{z}/{x}/{y}.png` |
| `VITE_BASEMAP_ATTRIBUTION` | `© OpenStreetMap contributors` | `© OpenStreetMap contributors` |

### LAN & Firewall Configuration (For Viewscape & Other Devices)

To let other devices on your network (or Viewscape) load Custom Web Tiles from `http://<your-lan-ip>:8000`:
1. Ensure `HOST=0.0.0.0` in `.env` (this is the default).
2. Allow inbound TCP traffic on port `8000` in your OS firewall:
   - **Windows (Admin PowerShell):**
     ```powershell
     New-NetFirewallRule -DisplayName "Veracity Maps (TCP 8000)" -Direction Inbound -Protocol TCP -LocalPort 8000 -Action Allow
     ```
   - **Linux (UFW):**
     ```bash
     sudo ufw allow 8000/tcp
     ```
3. In the Veracity Maps workspace, click **Export → Generate Custom Web Tile URL** and paste the generated `http://<your-lan-ip>:8000/tiles/<map_id>/composite/{0}/{1}/{2}.png` URL directly into Viewscape's Custom Web Tile field.

---

## 5. Development Mode (Hot Reload)

If you are actively editing React components or Go code and want live reload:

**Windows:**
```powershell
powershell -ExecutionPolicy Bypass -File scripts\start.ps1 -Dev
```

**macOS / Linux:**
```bash
bash scripts/start.sh --dev
```

This runs the Go backend on `http://localhost:8000` and the Vite dev server on `http://localhost:5173`. *(Ensure `VITE_API_URL=http://localhost:8000` in `frontend/.env` when using Vite dev mode.)*

---

## 6. Architecture Overview

```text
veracity-maps/
├── backend/
│   ├── main.go                 # Application entrypoint (HTTP server + SPA hosting)
│   ├── api/                    # HTTP router, middleware, and embedded SPA file server
│   ├── config/                 # Environment loader & validator
│   ├── functions/              # Feature HTTP handlers (upload, datasets, georeference, publish, export, gis, auth)
│   ├── services/               # Core business logic & composite tile rendering orchestration
│   ├── gis/                    # Pure-Go GIS transforms, world files, vector tile utilities
│   ├── scripts/                # Python GIS bridge (GeoPackage, raster, CAD, PDF, composite PNG renderer)
│   ├── repository/             # In-memory / file-backed & MSSQL repositories
│   ├── storage/                # Local runtime storage & tile cache (gitignored)
│   └── requirements.txt        # Clean, portable Python GIS dependencies
├── frontend/
│   ├── src/                    # React 19 + MapLibre GL workspace, export modal & published viewer
│   ├── public/icons/           # Portable icon library & manifest.json
│   └── dist/                   # Production SPA bundle served directly by Go backend
├── scripts/
│   ├── install.ps1             # Windows one-command installer
│   ├── install.sh              # macOS/Linux one-command installer
│   ├── start.ps1               # Windows launcher (full app or -Dev)
│   ├── start.sh                # macOS/Linux launcher (full app or --dev)
│   └── import-icons.ps1        # Deterministic icon asset importer
├── .env.example                # Root environment template
└── test-all.ps1                # Full automated verification suite
```

---

## 7. Running Tests & Verification

Run the complete automated verification suite (Go formatting, `go vet`, Go unit/integration tests, frontend unit/API-contract tests, and production bundle check):

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File test-all.ps1
```

Or run targeted checks:

```powershell
# Backend formatting, static analysis & tests
cd backend
gofmt -l .
go vet ./...
go test -v -count=1 ./tests/...

# Frontend tests & production build
cd ../frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"
```
