# Durable Map Configuration Retention Design

Date: 2026-11-13
Status: Approved design

## Goal

Every successful **Save Map Configuration** action must create durable,
user-owned map history that survives temporary session termination, session
expiry, backend restart, and logout. A collaborator with editor access saves a
new revision of the same shared map, visible to its owner and other authorized
collaborators. A standalone dataset save automatically creates a persistent
personal map for the authenticated user.

## Decisions

- `Maps` and immutable `Map_Revisions` are the canonical configuration store.
- Temporary `SessionRepository.SavedConfigs` and `saved_configs.json` are not a
  runtime persistence fallback after cutover.
- A save is reported as successful only after its map and revision commit to
  MSSQL.
- Production must use `MSSQLMapRepository` for map saves. If durable map
  persistence is unavailable, the API returns `503 PERSISTENCE_UNAVAILABLE`;
  it must not acknowledge an in-memory save.
- Owners and editors may save revisions. Viewers and download-only recipients
  cannot save.
- Shared-map revisions remain on the shared map. They are not copied into the
  collaborator's personal library.
- Each revision records the authenticated author independently of map owner.
- Logging out revokes authentication only. It does not delete maps, assets, or
  revisions.
- Ending or expiring a GIS session may delete temporary processing state, but
  cannot delete `Maps`, `Map_Assets`, or `Map_Revisions` records.

## Current Problem

The client still uses two save paths:

1. Map routes call `POST /api/maps/{mapId}/save`, which already targets map
   revisions.
2. Standalone dataset routes call `POST /sessions/{sessionId}/configs`, which
   stores configuration under temporary session state.

The second path ties saved configurations to session lifetime. It also makes
logout and collaboration behavior dependent on whichever browser session ID is
active. The application currently has memory and MSSQL map repository
implementations, so production can also appear to save successfully while only
retaining data in process memory.

## Architecture

### Canonical persistent model

No new configuration table is required. The existing records remain the source
of truth:

- `Maps`: ownership, display metadata, current revision pointer, and revision
  number.
- `Map_Assets`: durable association between uploaded datasets/files and a map.
- `Map_Revisions`: immutable configuration snapshots with revision number,
  author, description, and timestamp.
- `Map_Shares`: owner/editor/viewer/download permissions.

The latest committed map revision is loaded when a map is reopened. Session
state may cache working GIS data, but it never determines whether a saved map
configuration exists.

An asset referenced by a committed revision must also have durable bytes. Before
the first durable revision is committed, owned source files and required
processed artifacts are copied from the temporary session tree into a
user/map-owned storage path. `Map_Assets.storage_path` is updated only after the
durable copy is verified. Session cleanup may remove the temporary copy but must
never remove the committed map-owned copy.

### Persistent save service

Introduce one backend application service responsible for both save flows. Its
operations are:

1. `SaveExistingMapConfiguration`
   - authenticate the caller;
   - verify owner/editor permission;
   - verify the caller holds the active editing lock when lock enforcement is
     enabled;
   - validate configuration JSON and referenced assets;
   - compare `expected_revision` with the current map revision;
   - insert an immutable `Map_Revisions` row and update the map's current
     revision in one transaction.

2. `CreatePersonalMapFromConfiguration`
   - authenticate the caller;
   - validate that the primary dataset and every requested member asset are
     owned by the caller;
   - promote required session files into durable user/map storage and verify the
     copies;
   - create the personal map;
   - associate the assets with that map;
   - insert revision 1 and update the current revision pointer in the same
     transaction;
   - return the new map and revision so the client can navigate to the canonical
     `/maps/{mapId}/edit` URL.

Repository operations used by these services must be transactional in MSSQL.
Filesystem promotion uses staged writes and compensating cleanup: database rows
are committed only after durable files are present, and failed transactions
remove only newly staged files. Existing user files are never deleted during a
failed save.
The in-memory map repository remains available only for isolated tests and must
not be selected as a production persistence fallback.

## API Contract

### Save an existing owned or shared map

`POST /api/maps/{mapId}/save`

Request:

```json
{
  "configuration": {
    "name": "Master Floorplan Review",
    "primary_dataset_id": "ds_...",
    "members": ["ds_..."],
    "configuration": {
      "active_view": "map",
      "camera": {}
    }
  },
  "description": "Master Floorplan Review",
  "expected_revision": 3
}
```

Response includes the committed revision and updated map metadata. User B's
save on User A's editor-shared map creates revision 4 on User A's map with User
B as `created_by`.

### Save a standalone dataset as a personal map

`POST /api/maps/from-configuration`

Request contains `name`, `description`, `primary_dataset_id`, `members`, and the
full configuration payload. The endpoint performs map creation, asset
association, and revision creation atomically.

Response:

```json
{
  "map": { "id": "map_...", "revision_number": 1 },
  "revision": { "id": "rev_...", "revision_number": 1 }
}
```

The frontend replaces the dataset-workspace URL with the returned canonical map
URL after a successful save.

### Errors

- `401 UNAUTHORIZED`: no valid authenticated user.
- `403 FORBIDDEN`: viewer/download permission or asset ownership failure.
- `404 MAP_NOT_FOUND` or `ASSET_NOT_FOUND`: referenced object is unavailable.
- `409 REVISION_CONFLICT`: `expected_revision` is stale; return current revision
  metadata so the client can reload before retrying.
- `409 LOCK_HELD` or `LOCK_LOST`: another editor owns the lock.
- `422 INVALID_CONFIGURATION`: malformed JSON or invalid member references.
- `503 PERSISTENCE_UNAVAILABLE`: durable MSSQL persistence is unavailable.

No failure path writes a session-only configuration and no failure response is
displayed as a successful save.

## Frontend Flow

`WorkspacePage` uses one durable save controller:

- If `mapId` exists, call the existing-map save endpoint.
- If `mapId` is absent, call `POST /api/maps/from-configuration`, store the
  returned map state, and navigate with `replace` to `/maps/{mapId}/edit`.
- Remove `createSavedConfig` from the Save Map Configuration action.
- Update dialog copy to state that the configuration is saved to the user's map
  library and survives logout.
- Display revision conflicts, permission failures, lock loss, and persistence
  outages without closing the dialog.
- The dashboard and shared-map lists continue loading maps from persistent map
  APIs rather than session saved-config APIs.

Logout clears only the authentication token and in-memory UI identity. It must
not call session deletion as a side effect. An explicit session-end operation
may clean temporary GIS files but cannot invoke map or revision deletion.

## Collaboration Rules

- Owner: view, save, restore, share, and delete.
- Editor: view and save revisions to the shared map; cannot transfer ownership
  or delete the map.
- Viewer: view only.
- Download: download-only behavior; no revision save.
- Revision history records both map ownership and revision authorship.
- All authorized users see the same ordered revision history for a shared map.
- Optimistic revision checks prevent User A and User B from silently
  overwriting each other's saves.

## Legacy Session Configuration Promotion

Legacy session configurations are migration input only. Promotion is safe only
when ownership is unambiguous:

1. Resolve every member dataset against `Map_Assets`.
2. Promote only when all assets have the same non-empty owner user ID.
3. Create one personal map and revision for that owner.
4. Record an idempotency marker keyed by legacy session/config ID so retries do
   not duplicate maps.
5. Preserve the legacy JSON until promotion is verified.
6. Skip anonymous, missing, mixed-owner, or malformed configurations and report
   aggregate counts without logging configuration contents or user secrets.

The promotion tool is explicit and administrative; startup does not silently
claim legacy anonymous data for whichever user logs in first.

## Data Retention

- Map and revision rows have no session-based expiry.
- Logout does not alter retention.
- Session cleanup excludes durable asset paths referenced by `Map_Assets`.
- A collaborator saving a shared map references the owner's existing durable
  assets; it does not transfer asset ownership or create session-bound copies.
- Map deletion remains the explicit destructive boundary. Deleting a map may
  cascade its revisions according to the existing foreign key, but requires
  owner authorization and confirmation.
- Account deletion and retention-policy work are outside this change.

## Security and Integrity

- Derive user identity exclusively from authenticated request context.
- Never accept `owner_user_id` or `created_by` from the browser.
- Validate every asset ID against the authenticated owner when creating a
  personal map.
- Validate map permission on every shared-map save and revision read.
- Use named SQL parameters and one transaction for each map/revision commit.
- Do not log configuration JSON, authentication tokens, share tokens, or file
  contents.

## Verification

Backend tests:

- owner save survives session deletion and a new login token;
- standalone dataset save atomically creates a personal map and revision 1;
- editor User B saves revision 2 to User A's shared map;
- User A and User B both read the same revision history after logout/login;
- viewer and download-only users receive `403`;
- stale expected revisions receive `409` and do not create rows;
- persistence failure receives `503` and creates no in-memory success state;
- explicit session deletion leaves maps and revisions intact;
- legacy promotion is idempotent and skips ambiguous ownership.

Frontend tests:

- map workspaces call the existing-map save endpoint;
- standalone workspaces call the create-from-configuration endpoint and replace
  the URL with the returned map ID;
- no Save Map Configuration path calls session saved-config APIs;
- conflict, permission, lock, and persistence errors remain visible;
- logout does not invoke map or revision deletion.

Project gates remain `go test -v -count=1 ./tests/...`, `go vet ./...`,
`gofmt -l .`, frontend tests, the production frontend build, and
`test-all.ps1`. Live MSSQL acceptance must verify owner and collaborator flows
with fresh users and must be reported separately from memory-repository tests.

## Documentation Changes

Update backend repository/API documentation and frontend API/page documentation
to state that map revisions are durable and session configurations are legacy
migration input only. Remove UI wording that promises retention only "during
this session."

## Out of Scope

- Private per-collaborator branches or personal copies of shared maps.
- Automatic merge of concurrent map revisions.
- Account deletion and long-term regulatory retention policy.
- A new configuration table or a second persistent source of truth.
