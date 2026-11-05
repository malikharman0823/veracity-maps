# Database Setup & Configuration: Microsoft SQL Server 2022

Veracity Maps 1.3 uses **Microsoft SQL Server 2022** for multi-user accounts, persistent authentication tokens, organizations, project collaboration, RBAC permissions, and GIS metadata management.

---

## 1. Prerequisites

- **Database Engine**: Microsoft SQL Server 2022 (or SQL Server 2019+)
- **Go Driver**: `github.com/microsoft/go-mssqldb`
- **Database Name**: `veracity_maps`

---

## 2. Environment Variables

Configure your database connection via `.env` or system environment variables:

### Option A: Standard Connection String (Recommended)
```env
MSSQL_CONNECTION_STRING="server=127.0.0.1;port=1433;database=veracity_maps;user id=sa;password=YourStrongPassword123!;encrypt=disable"
```
*Note for Windows Authentication / Trusted Connections:*
```env
MSSQL_CONNECTION_STRING="server=127.0.0.1;port=1433;database=veracity_maps;trusted_connection=yes;encrypt=disable"
```

### Option B: Discrete Parameters
```env
MSSQL_HOST=127.0.0.1
MSSQL_PORT=1433
MSSQL_DATABASE=veracity_maps
MSSQL_USER=sa
MSSQL_PASSWORD=YourStrongPassword123!
```

---

## 3. Database Initialization & Migrations

1. Ensure the database `veracity_maps` exists in SQL Server:
   ```sql
   CREATE DATABASE veracity_maps;
   ```

2. When the backend application starts (`go run main.go`), it automatically:
   - Connects to Microsoft SQL Server 2022.
   - Runs idempotent migrations from [`backend/migrations/001_initial_schema.sql`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/migrations/001_initial_schema.sql).
   - Migrates existing local JSON storage records (`storage/users`, `storage/projects`, `storage/organizations`) into MSSQL with ID preservation and creates a timestamped backup under `storage/backups/`.

You can also run the schema script manually using `sqlcmd`:
```powershell
sqlcmd -S localhost -E -d veracity_maps -i backend/migrations/001_initial_schema.sql
```

---

## 4. Tables Created

1. **`Users`**: User accounts with SHA-256 multi-round cryptographic salt & hash.
2. **`AuthTokens`**: Bearer tokens with expiration and user ID foreign key cascading.
3. **`Organizations`**: Organization workspaces with storage tracking and quotas.
4. **`OrganizationMembers`**: Member roles (`owner`, `admin`, `editor`, `viewer`) and invitation statuses.
5. **`OrganizationInvitations`**: Email-based invitations with secure tokens.
6. **`Projects`**: Personal and organizational projects with metadata references.
7. **`ProjectMembers`**: Collaborator roles (`owner`, `editor`, `viewer`) and statuses.
8. **`ProjectInvitations`**: Project sharing invitations.
9. **`ProjectActivities`**: Audit log of project operations.
10. **`OrganizationActivities`**: Audit log of organizational events.
11. **`Datasets`**: File and GIS raster/vector metadata linkage (file size, storage path, owner, project).

---

## 5. Running the Application

### Backend:
```powershell
cd backend
go run main.go
```

### Frontend:
```powershell
cd frontend
npm run dev
```

