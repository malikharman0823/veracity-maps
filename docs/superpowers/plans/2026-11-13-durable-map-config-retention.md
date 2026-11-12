# Durable Map Configuration Retention Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Make every successful map-configuration save durable across session cleanup, logout, backend restart, and shared-map collaboration.

**Architecture:** Keep `Maps`, `Map_Assets`, and immutable `Map_Revisions` as the only runtime source of truth. Route both existing-map saves and first saves from standalone dataset workspaces through one application service that validates access, stages durable asset copies, and asks the map repository to commit asset locations plus the revision atomically. MSSQL is mandatory outside tests; the memory repository remains an isolated test double.

**Tech Stack:** Go 1.24, `database/sql` with Microsoft SQL Server, React 19, React Router 7, Axios, Node's built-in test runner.

---

## Working Rules

- Preserve the existing dirty worktree. Stage and commit only files listed by the active task.
- Use test-driven development: add one failing test, run it, make the smallest change, and rerun it.
- Do not write durable map data to `SessionRepository.SavedConfigs` or `saved_configs.json`.
- Derive owner and revision author from the authenticated request context; never accept them from JSON.
- Keep durable files below `<UPLOAD_DIR>/maps/<owner-id>/<map-id>/assets/<asset-id>/`, never below `temp/sessions`.
- A reported success means both the durable file copy and MSSQL transaction succeeded.
- If a staged copy or transaction fails, remove only the new staging directory with `storage.SafeRemoveAll`.
- Do not delete the source session files during save. Normal session cleanup remains responsible for them.

## File Map

**Create**

- `backend/services/map_configuration_service.go` — save orchestration, validation, asset staging, and compensation.
- `backend/services/map_configuration_service_test.go` — focused service tests with fake repository and temporary files.
- `frontend/src/lib/mapConfigPersistence.js` — build the durable request and select create-versus-update.
- `frontend/src/lib/mapConfigPersistence.test.js` — frontend orchestration tests.
- `backend/migrations/011_legacy_config_promotions.sql` — idempotency records for explicit legacy promotion.
- `backend/cmd/promote-session-configs/main.go` — opt-in legacy promotion command.

**Modify**

- `backend/models/personal_map.go` — commit input/result and persistence error types.
- `backend/repository/map_repository.go` — atomic create/save repository operations and MSSQL transactions.
- `backend/functions/maps/function.go` — create-from-configuration handler and stable error mapping.
- `backend/api/router.go` — register `POST /api/maps/from-configuration` before dynamic map IDs.
- `backend/main.go` — wire the service and reject memory map persistence outside tests.
- `backend/tests/personal_maps_and_sharing_test.go` — owner/editor/viewer/session/logout retention acceptance.
- `frontend/src/api.js` — expose create-from-configuration API and stop exporting session config APIs for runtime use.
- `frontend/src/pages/WorkspacePage.jsx` — use durable saves, replace the URL after first save, and retain errors.
- `frontend/src/pages/DashboardPage.jsx` — load saved maps only from persistent map endpoints.
- `backend/services/README.md`, `backend/tests/README.md`, `frontend/src/lib/README.md`, `frontend/src/pages/README.md`, and root `README.md` — document the persistent contract and migration boundary.

### Task 1: Define the atomic persistence contract

**Files:**

- Modify: `backend/models/personal_map.go`
- Modify: `backend/repository/map_repository.go`
- Test: `backend/services/map_configuration_service_test.go`

**Step 1: Write the failing contract test**

Add `TestMapConfigurationService_CreateRequiresAtomicRepositoryCommit`. The fake repository must record exactly one `CreateMapFromConfiguration` call and return map revision 1. Assert that the input includes:

```go
models.MapConfigurationCommit{
    Name:             "Ground Floor",
    PrimaryDatasetID: "ds_primary",
    MemberAssetIDs:   []string{"ds_primary", "ds_overlay"},
    ConfigurationJSON: `{"active_view":"map"}`,
}
```

Run:

```powershell
Set-Location backend
go test ./services -run TestMapConfigurationService_CreateRequiresAtomicRepositoryCommit -count=1
```

Expected: compile failure because the service and commit types do not exist.

**Step 2: Add the domain types and sentinel errors**

In `backend/models/personal_map.go`, add:

```go
type MapAssetLocation struct {
    AssetID     string
    OwnerUserID string
    StoragePath string
}

type MapConfigurationCommit struct {
    Name               string
    Description        string
    PrimaryDatasetID   string
    MemberAssetIDs     []string
    ConfigurationJSON  string
    ExpectedRevision   int
    AssetLocations     []MapAssetLocation
}

type MapConfigurationResult struct {
    Map      *Map         `json:"map"`
    Revision *MapRevision `json:"revision"`
}
```

Add errors for invalid configuration, missing assets, persistence unavailable, and revision conflict. Reuse existing access and lock errors.

**Step 3: Extend `MapRepo` with two atomic operations**

```go
CreateMapFromConfiguration(ctx context.Context, ownerUserID, authorName string, commit models.MapConfigurationCommit) (*models.MapConfigurationResult, error)
SaveMapConfiguration(ctx context.Context, mapID, userID, authorName string, commit models.MapConfigurationCommit) (*models.MapConfigurationResult, error)
```

Keep the old `CreateMap` and `SaveMapConfig` temporarily as compatibility wrappers, then remove their runtime callers in later tasks.

**Step 4: Implement the fake and minimal memory behavior**

The memory implementation is only for tests. It must enforce owner/editor permission, expected revision, author identity, asset ownership, and revision numbering so acceptance tests cannot pass with weaker rules than MSSQL.

**Step 5: Run the focused test**

```powershell
Set-Location backend
go test ./services -run TestMapConfigurationService_CreateRequiresAtomicRepositoryCommit -count=1
```

Expected: PASS.

**Step 6: Commit**

```powershell
git add backend/models/personal_map.go backend/repository/map_repository.go backend/services/map_configuration_service_test.go
git commit -m "refactor: define durable map configuration commits"
```

### Task 2: Stage durable assets safely

**Files:**

- Create: `backend/services/map_configuration_service.go`
- Modify: `backend/services/map_configuration_service_test.go`
- Modify: `backend/storage/temporary.go`

**Step 1: Write failing filesystem tests**

Add these table-driven tests:

- `TestMapConfigurationService_StagesDatasetTreeOutsideSessionStorage`
- `TestMapConfigurationService_RejectsAssetOwnedByAnotherUser`
- `TestMapConfigurationService_RemovesOnlyNewStageOnRepositoryFailure`
- `TestMapConfigurationService_DoesNotDeleteSessionSource`

Build test files with `t.TempDir()` and direct `os.WriteFile` calls. Assert exact paths under `filepath.Join(uploadDir, "maps", ownerID, mapID, "assets", assetID)` and assert the source tree still exists.

Run:

```powershell
Set-Location backend
go test ./services -run 'TestMapConfigurationService_(Stages|Rejects|Removes|DoesNotDelete)' -count=1
```

Expected: FAIL because staging is not implemented.

**Step 2: Add safe directory-copy support**

Add `storage.CopyTree(src, dst string) error` beside `CopyFile`. It must:

- reject a missing source;
- create directories with ordinary non-executable permissions;
- preserve relative paths only after validating them with `SafePath`;
- close every file before returning so Windows cleanup can succeed;
- never follow a path outside `src`.

**Step 3: Implement service validation and staging**

Construct the service with explicit dependencies:

```go
type MapConfigurationService struct {
    maps       repository.MapRepo
    uploadRoot string
    sessionRoot string
}
```

For each distinct `primary_dataset_id` and `members` entry:

1. Call `GetAsset(ctx, assetID, authenticatedUserID)`.
2. Reject empty IDs, foreign ownership, or missing source paths.
3. If already below the target map's durable root, reuse it.
4. Otherwise copy the complete dataset directory to a `.staging-<unique>` sibling.
5. Verify the promoted primary file exists and its byte size matches the source.
6. Atomically rename the staging directory to the final asset directory.
7. Pass the new primary file path to the repository commit.

On commit failure, remove only final directories created by this attempt. Never remove a reused durable directory or a session source.

**Step 4: Run focused tests**

```powershell
Set-Location backend
go test ./services -run 'TestMapConfigurationService_(Stages|Rejects|Removes|DoesNotDelete)' -count=1
```

Expected: PASS.

**Step 5: Commit**

```powershell
git add backend/services/map_configuration_service.go backend/services/map_configuration_service_test.go backend/storage/temporary.go
git commit -m "feat: stage durable map assets"
```

### Task 3: Make MSSQL commits transactional and concurrency-safe

**Files:**

- Modify: `backend/repository/map_repository.go`
- Test: `backend/tests/mssql_multi_user_test.go`

**Step 1: Write the failing live-SQL test**

Add `TestMSSQLMapConfigurationCommitIsAtomic` to the existing opt-in MSSQL suite. Use its current database setup and user seeding helpers. The test must assert:

- create-from-configuration inserts one map, revision 1, and both asset path/map associations;
- a forced invalid asset update rolls back the map and revision;
- two saves with the same expected revision yield one success and one `ErrOutdatedRevision`;
- the winning revision's `created_by` is the authenticated editor.

Run only when `MSSQL_CONNECTION_STRING` is configured:

```powershell
Set-Location backend
go test ./tests -run TestMSSQLMapConfigurationCommitIsAtomic -count=1
```

Expected before implementation: FAIL. If SQL is unavailable, record this as a skipped live gate rather than claiming acceptance.

**Step 2: Implement `CreateMapFromConfiguration` in one transaction**

Within one `sql.Tx`:

1. Re-read every asset with `UPDLOCK, HOLDLOCK` and verify `owner_user_id`.
2. Insert `Maps` with revision number 1.
3. Update only the validated asset IDs with `map_id` and the staged storage path.
4. Insert revision 1 using the authenticated owner as `created_by`.
5. Update `current_revision_id`.
6. Commit and return both records.

Check `RowsAffected` for every asset update; any mismatch must roll back.

**Step 3: Make existing-map save one locked transaction**

Move permission, lock, and expected-revision checks inside the transaction. Read the map and share row with update locks, update staged asset paths only for assets already owned by the map owner and associated with the map, insert the next revision, and update the current revision pointer. This prevents two collaborators from both committing the same next revision.

**Step 4: Run repository tests**

```powershell
Set-Location backend
go test ./repository ./tests -run 'Test(MSSQLMapConfigurationCommitIsAtomic|PersonalMap)' -count=1
```

Expected: PASS for configured gates; explicit skip for unavailable live SQL.

**Step 5: Commit**

```powershell
git add backend/repository/map_repository.go backend/tests/mssql_multi_user_test.go
git commit -m "feat: commit map revisions atomically"
```

### Task 4: Expose the durable create/save API and fail closed

**Files:**

- Modify: `backend/functions/maps/function.go`
- Modify: `backend/api/router.go`
- Modify: `backend/main.go`
- Modify: `backend/tests/personal_maps_and_sharing_test.go`

**Step 1: Add failing HTTP tests**

Add:

- `TestCreateMapFromConfigurationReturnsRevisionOne`
- `TestCreateMapFromConfigurationRequiresAuthentication`
- `TestCreateMapFromConfigurationRejectsForeignAsset`
- `TestMapSaveReturnsRevisionConflictMetadata`
- `TestMapSaveReturnsPersistenceUnavailable`

The success test posts to `/api/maps/from-configuration` and asserts `data.map.id`, `data.map.revision_number == 1`, and `data.revision.revision_number == 1`. The persistence test injects a repository stub returning `models.ErrPersistenceUnavailable` and asserts HTTP 503 with code `PERSISTENCE_UNAVAILABLE`.

Run:

```powershell
Set-Location backend
go test ./tests -run 'Test(CreateMapFromConfiguration|MapSaveReturns)' -count=1
```

Expected: FAIL because the route does not exist.

**Step 2: Add the handler and stable errors**

`POST /api/maps/from-configuration` accepts only:

```json
{
  "name": "Ground Floor",
  "description": "Saved from workspace",
  "primary_dataset_id": "ds_primary",
  "members": ["ds_primary", "ds_overlay"],
  "configuration": {"active_view": "map", "camera": {}}
}
```

The handler gets the user from middleware, validates required fields and JSON, calls the service, and returns HTTP 201. Map service errors map to the codes in the approved design: 403, 404, 409, 422, and 503. Existing-map save uses the same service and returns the updated map plus revision.

**Step 3: Register the static route before `mapID` parsing**

Handle `path == "/api/maps/from-configuration"` before splitting `/api/maps/{mapId}` so `from-configuration` can never be mistaken for a map ID.

**Step 4: Remove the production in-memory fallback**

In `backend/main.go`, allow `NewMemoryMapRepository()` only when `cfg.Environment == "test"`. Outside tests, failure to initialize MSSQL must leave the map service unavailable and produce 503, or stop startup if the shared DB is a required dependency. It must never acknowledge a durable save in process memory.

**Step 5: Run HTTP tests**

```powershell
Set-Location backend
go test ./tests -run 'Test(CreateMapFromConfiguration|MapSaveReturns)' -count=1
```

Expected: PASS.

**Step 6: Commit**

```powershell
git add backend/functions/maps/function.go backend/api/router.go backend/main.go backend/tests/personal_maps_and_sharing_test.go
git commit -m "feat: expose durable map configuration saves"
```

### Task 5: Prove retention across session, logout, and collaboration

**Files:**

- Modify: `backend/tests/personal_maps_and_sharing_test.go`

**Step 1: Add the failing lifecycle acceptance test**

Add `TestDurableMapConfigurationSurvivesSessionLogoutAndSharedEditing` with this exact sequence:

1. Register User A and User B.
2. User A uploads a dataset without a map ID.
3. User A creates a map from configuration and captures revision 1.
4. Explicitly end and delete the associated GIS session.
5. Log User A out, log in again, and verify the map and revision 1 still load.
6. User A shares the map with User B as editor.
7. User B acquires the lock and saves with `expected_revision: 1`.
8. Assert revision 2 belongs to the same map and `created_by` is User B.
9. Log User B out and in; assert both users list the same two revisions.
10. Change User B to viewer and assert save returns 403 without revision 3.
11. Submit a stale expected revision and assert 409 without revision 3.

Run:

```powershell
Set-Location backend
go test ./tests -run TestDurableMapConfigurationSurvivesSessionLogoutAndSharedEditing -count=1
```

Expected before all lifecycle behavior is wired: FAIL.

**Step 2: Fix only failures exposed by this lifecycle**

Do not introduce another persistence path. Correct handler, permission, logout, or session-cleanup behavior only where the acceptance test identifies a mismatch.

**Step 3: Run the full map suite**

```powershell
Set-Location backend
go test ./tests -run 'Test(PersonalMap|DurableMap|CreateMapFromConfiguration|MapSave)' -count=1
```

Expected: PASS.

**Step 4: Commit**

```powershell
git add backend/tests/personal_maps_and_sharing_test.go backend/functions/maps/function.go backend/api/router.go backend/repository/map_repository.go
git commit -m "test: cover durable collaborative map retention"
```

### Task 6: Switch the frontend to the durable API

**Files:**

- Create: `frontend/src/lib/mapConfigPersistence.js`
- Create: `frontend/src/lib/mapConfigPersistence.test.js`
- Modify: `frontend/src/api.js`
- Modify: `frontend/src/pages/WorkspacePage.jsx`

**Step 1: Write failing orchestration tests**

Test the pure controller with injected API functions:

- map workspace calls `saveMapConfig(mapId, payload, name, expectedRevision)`;
- standalone workspace calls `createMapFromConfiguration(payload)`;
- standalone success returns `/maps/<id>/edit?view=<active-view>`;
- errors are rethrown and no navigation target is returned;
- no branch calls `createSavedConfig`.

Run:

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test -- --test-name-pattern=map.config.persistence"
```

Expected: FAIL because the module does not exist.

**Step 2: Add `createMapFromConfiguration` to `api.js`**

```js
export async function createMapFromConfiguration(payload) {
  const response = await client.post("/api/maps/from-configuration", payload)
  return response.data.data
}
```

Keep session config functions temporarily only for explicit legacy tooling; remove them from workspace and dashboard imports.

**Step 3: Implement and use the pure controller**

Move request selection out of `WorkspacePage`. On standalone success:

1. set the returned map as current state;
2. close the dialog only after success;
3. call `navigate(`/maps/${map.id}/edit?view=${view}`, { replace: true })`;
4. keep the error visible and the dialog open for 403, 409, 422, and 503.

Update the modal copy to: “Saved maps and revision history remain available after the current session ends or you log out.”

**Step 4: Run frontend tests and build**

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"
```

Expected: both PASS.

**Step 5: Commit**

```powershell
git add frontend/src/lib/mapConfigPersistence.js frontend/src/lib/mapConfigPersistence.test.js frontend/src/api.js frontend/src/pages/WorkspacePage.jsx
git commit -m "feat: save workspace configurations as durable maps"
```

### Task 7: Remove session-config runtime dependencies and protect logout

**Files:**

- Modify: `frontend/src/pages/DashboardPage.jsx`
- Modify: `frontend/src/api.js`
- Modify: `frontend/src/lib/sessionLoading.js`
- Modify: `frontend/src/lib/sessionLoading.test.js`
- Modify: `frontend/src/lib/mapConfigPersistence.test.js`

**Step 1: Add failing regression tests**

Assert:

- dashboard saved-map loading calls the persistent `/api/maps` path only;
- logout clears auth/UI state but never calls `endSession`, `deleteSession`, map deletion, or revision deletion;
- active session cleanup does not control whether a saved map is displayed;
- `WorkspacePage` has no runtime import of `createSavedConfig`.

Use source-level import assertions only for the final “no runtime import” guard; use behavior tests for all other cases.

**Step 2: Remove runtime session-config calls**

Delete saved-config fetch/create/delete usage from normal dashboard and workspace flows. Keep `getSession`, `endSession`, and dataset session recovery intact for temporary GIS processing.

**Step 3: Run frontend tests**

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
```

Expected: PASS.

**Step 4: Commit**

```powershell
git add frontend/src/pages/DashboardPage.jsx frontend/src/api.js frontend/src/lib/sessionLoading.js frontend/src/lib/sessionLoading.test.js frontend/src/lib/mapConfigPersistence.test.js
git commit -m "refactor: remove session-only saved map flows"
```

### Task 8: Add explicit, idempotent legacy promotion

**Files:**

- Create: `backend/migrations/011_legacy_config_promotions.sql`
- Create: `backend/cmd/promote-session-configs/main.go`
- Create: `backend/cmd/promote-session-configs/main_test.go`

**Step 1: Write failing promotion tests**

Cover:

- all assets with one non-empty owner create one map and marker;
- rerunning the same session/config pair creates nothing;
- anonymous, missing, mixed-owner, and malformed configurations are skipped;
- source JSON remains after success;
- logs contain counts and IDs only, never configuration JSON, tokens, or file contents.

Run:

```powershell
Set-Location backend
go test ./cmd/promote-session-configs -count=1
```

Expected: FAIL because the command does not exist.

**Step 2: Add the marker table**

Create `dbo.Legacy_Config_Promotions` with a unique `(legacy_session_id, legacy_config_id)` key, nullable resulting `map_id`, status, reason code, and timestamps. Make the migration idempotent with `OBJECT_ID` and index existence checks.

**Step 3: Implement an explicit dry-run-first command**

Require `-source`, default to dry-run, and require `-apply` for writes. Resolve every member against `Map_Assets`, require exactly one owner, call the same application service used by HTTP, and insert the marker in the same logical operation. Never run this command automatically during server startup.

**Step 4: Run tests**

```powershell
Set-Location backend
go test ./cmd/promote-session-configs -count=1
```

Expected: PASS.

**Step 5: Commit**

```powershell
git add backend/migrations/011_legacy_config_promotions.sql backend/cmd/promote-session-configs/main.go backend/cmd/promote-session-configs/main_test.go
git commit -m "feat: add safe legacy config promotion"
```

### Task 9: Documentation and full verification

**Files:**

- Modify: `README.md`
- Modify: `backend/services/README.md`
- Modify: `backend/tests/README.md`
- Modify: `frontend/src/lib/README.md`
- Modify: `frontend/src/pages/README.md`

**Step 1: Update the contract documentation**

Document:

- maps/revisions and durable assets survive session end and logout;
- editor saves append to the owner's shared map;
- standalone saves create a personal map and canonical URL;
- viewer/download cannot save;
- session saved configs are legacy migration input only;
- MSSQL unavailable means save unavailable, not memory fallback;
- exact dry-run and apply commands for legacy promotion.

**Step 2: Run formatting and focused static checks**

```powershell
Set-Location backend
gofmt -w models/personal_map.go repository/map_repository.go services/map_configuration_service.go services/map_configuration_service_test.go functions/maps/function.go tests/personal_maps_and_sharing_test.go cmd/promote-session-configs/main.go cmd/promote-session-configs/main_test.go
go vet ./...
$unformatted = gofmt -l .
if ($unformatted) { $unformatted; throw 'gofmt check failed' }
```

Expected: no vet errors and no unformatted files.

**Step 3: Run backend tests**

```powershell
Set-Location backend
go test -v -count=1 ./tests/...
go test -v -count=1 ./services/... ./repository/... ./cmd/promote-session-configs/...
```

Expected: PASS. Report any live MSSQL skip separately.

**Step 4: Run frontend tests and production build**

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
if ($LASTEXITCODE -ne 0) { throw 'frontend tests failed' }
cmd.exe /c "npm.cmd run build"
if ($LASTEXITCODE -ne 0) { throw 'frontend build failed' }
```

Expected: PASS. Rollup size notices are warnings, not failures.

**Step 5: Run the repository automation**

```powershell
Set-Location ..
powershell -ExecutionPolicy Bypass -File test-all.ps1
```

Expected: exit code 0.

**Step 6: Perform live acceptance when MSSQL is available**

With fresh User A and User B accounts, repeat the lifecycle from Task 5 against the running frontend/backend, restart the backend between saves, and verify revisions through the API after both users log back in. Do not report browser or live-SQL acceptance unless it was actually executed.

**Step 7: Final diff review and commit**

```powershell
git diff --check
git status --short
git add README.md backend/services/README.md backend/tests/README.md frontend/src/lib/README.md frontend/src/pages/README.md
git commit -m "docs: describe durable map retention"
```

## Completion Criteria

- A standalone save returns a persistent map at revision 1 and navigates to its canonical map URL.
- Ending or deleting its GIS session and logging out cannot remove the map, revision, or promoted asset bytes.
- User B with editor access appends a revision to User A's same map; both users see identical history after new logins.
- Viewer/download access, stale revisions, lock loss, invalid assets, and unavailable persistence fail without a success state or partial revision.
- Runtime UI contains no session saved-config path.
- Legacy data is promoted only by the explicit idempotent command.
- Focused tests, full backend tests, vet, formatting, frontend tests/build, and `test-all.ps1` pass; live MSSQL/browser results are reported honestly and separately.
