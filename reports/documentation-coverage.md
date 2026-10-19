# Documentation Coverage Report

**Generated:** 2026-10-19  
**Scope:** All meaningful source code and configuration directories across Veracity Maps  

| Directory | README Present | Source Files Count | Key Source Files | Status |
| :--- | :--- | :--- | :--- | :--- |
| `.` | YES | 1 | test-all.ps1 | **COMPLETE** |
| `backend` | YES | 1 | main.go | **COMPLETE** |
| `backend/api` | YES | 2 | middleware.go, router.go | **COMPLETE** |
| `backend/config` | YES | 1 | config.go | **COMPLETE** |
| `backend/functions` | YES | 0 |  | **COMPLETE** |
| `backend/functions/health` | YES | 1 | function.go | **COMPLETE** |
| `backend/functions/upload` | YES | 3 | function.go, request.go, response.go | **COMPLETE** |
| `backend/functions/datasets` | YES | 3 | function.go, request.go, response.go | **COMPLETE** |
| `backend/functions/georeference` | YES | 3 | function.go, request.go, response.go | **COMPLETE** |
| `backend/functions/export` | YES | 3 | function.go, request.go, response.go | **COMPLETE** |
| `backend/gis` | YES | 2 | inspect.go, transform.go | **COMPLETE** |
| `backend/models` | YES | 4 | control_point.go, dataset.go, layer.go, response.go | **COMPLETE** |
| `backend/repository` | YES | 1 | session_repository.go | **COMPLETE** |
| `backend/scripts` | YES | 1 | test-all.ps1 | **COMPLETE** |
| `backend/services` | YES | 3 | dataset_service.go, export_service.go, georeference_service.go | **COMPLETE** |
| `backend/storage` | YES | 1 | temporary.go | **COMPLETE** |
| `backend/tests` | YES | 4 | api_full_matrix_test.go, api_integration_test.go, gis_transform_test.go, repository_test.go | **COMPLETE** |
| `frontend` | YES | 9 | components.json, index.html, jsconfig.json, package-lock.json | **COMPLETE** |
| `frontend/src` | YES | 3 | api.js, App.jsx, main.jsx | **COMPLETE** |
| `frontend/src/components` | YES | 21 | BackgroundRemovalDialog.jsx, ControlPointList.jsx, DashboardLayout.jsx, DatasetCard.jsx | **COMPLETE** |
| `frontend/src/components/ui` | YES | 9 | alert-dialog.jsx, attachment.jsx, button.jsx, input.jsx | **COMPLETE** |
| `frontend/src/hooks` | YES | 2 | use-mobile.js, useWorkspace.js | **COMPLETE** |
| `frontend/src/lib` | YES | 3 | utils.js, workspaceNavigation.js, workspaceNavigation.test.js | **COMPLETE** |
| `frontend/src/pages` | YES | 5 | DatasetPage.jsx, GeoreferencePage.jsx, HomePage.jsx, UploadPage.jsx | **COMPLETE** |
| `testdata` | YES | 1 | generate_testdata.go | **COMPLETE** |
| `testdata/maptiler-mvp` | YES | 2 | control_points_fixture.json, manifest.json | **COMPLETE** |
| `reports` | YES | 0 |  | **COMPLETE** |

---

## Summary
- **Total Meaningful Directories:** 27
- **Directories with READMEs:** 27
- **Documentation Coverage:** 100% (27/27)
