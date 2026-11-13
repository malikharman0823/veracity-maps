-- ============================================================================
-- Veracity Maps 1.3: Migration 009 - Workspace Assets & Sessions Schema
-- Supports Release 1 (Centralized Files) & Release 3 (Workspace Sessions)
-- ============================================================================

-- 1. Workspace_Assets Table
IF OBJECT_ID(N'dbo.Workspace_Assets', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Workspace_Assets (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        original_filename NVARCHAR(255) NOT NULL,
        file_type NVARCHAR(64) NOT NULL,
        file_size BIGINT NOT NULL,
        storage_reference NVARCHAR(MAX) NOT NULL,
        uploaded_by NVARCHAR(64) NOT NULL,
        uploaded_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        processing_status NVARCHAR(64) NOT NULL DEFAULT 'ready',
        associated_map_id NVARCHAR(64) NULL,
        CONSTRAINT FK_WorkspaceAssets_Workspace FOREIGN KEY (workspace_id) REFERENCES dbo.Workspaces(id) ON DELETE CASCADE,
        CONSTRAINT FK_WorkspaceAssets_User FOREIGN KEY (uploaded_by) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_WorkspaceAssets_WorkspaceID ON dbo.Workspace_Assets(workspace_id);
    CREATE INDEX IX_WorkspaceAssets_UploadedBy ON dbo.Workspace_Assets(uploaded_by);
END;
GO

-- 2. Workspace_Sessions Table (Active collaboration sessions controlled by owner)
IF OBJECT_ID(N'dbo.Workspace_Sessions', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Workspace_Sessions (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        started_by NVARCHAR(64) NOT NULL,
        status NVARCHAR(32) NOT NULL DEFAULT 'active', -- 'active', 'ended'
        started_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        ended_at DATETIMEOFFSET NULL,
        CONSTRAINT FK_WorkspaceSessions_Workspace FOREIGN KEY (workspace_id) REFERENCES dbo.Workspaces(id) ON DELETE CASCADE,
        CONSTRAINT FK_WorkspaceSessions_User FOREIGN KEY (started_by) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_WorkspaceSessions_WorkspaceID ON dbo.Workspace_Sessions(workspace_id);
    CREATE INDEX IX_WorkspaceSessions_Status ON dbo.Workspace_Sessions(status);
END;
GO

