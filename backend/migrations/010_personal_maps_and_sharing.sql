-- ============================================================================
-- Veracity Maps: Migration 010 - Personal Accounts and Map-Level Sharing
-- Architecture: User -> Personal Dashboard -> My Maps -> Share Map -> Shared With Me
-- Removes: Workspaces, Organizations, Projects, Mapspaces
-- ============================================================================

-- 1. Ensure dbo.Maps has owner_user_id and is independent of workspaces/projects
IF OBJECT_ID(N'dbo.Maps', N'U') IS NOT NULL
BEGIN
    IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'owner_user_id')
    BEGIN
        ALTER TABLE dbo.Maps ADD owner_user_id NVARCHAR(64) NULL;
    END;

    -- Ensure owner_user_id is populated from created_by or user_id
    EXEC(N'UPDATE dbo.Maps SET owner_user_id = created_by WHERE owner_user_id IS NULL AND created_by IS NOT NULL;');

    -- Make project_id / workspace_id nullable if foreign keys exist
    IF EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = N'FK_Maps_Project')
    BEGIN
        ALTER TABLE dbo.Maps DROP CONSTRAINT FK_Maps_Project;
    END;
    IF EXISTS (SELECT 1 FROM sys.foreign_keys WHERE name = N'FK_Maps_Workspace')
    BEGIN
        ALTER TABLE dbo.Maps DROP CONSTRAINT FK_Maps_Workspace;
    END;

    IF EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'project_id')
    BEGIN
        ALTER TABLE dbo.Maps ALTER COLUMN project_id NVARCHAR(64) NULL;
    END;
    IF EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'workspace_id')
    BEGIN
        ALTER TABLE dbo.Maps ALTER COLUMN workspace_id NVARCHAR(64) NULL;
    END;

    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'IX_Maps_OwnerUserID')
    BEGIN
        CREATE INDEX IX_Maps_OwnerUserID ON dbo.Maps(owner_user_id);
    END;
END
ELSE
BEGIN
    CREATE TABLE dbo.Maps (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        owner_user_id NVARCHAR(64) NOT NULL,
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(MAX) NULL,
        current_revision_id NVARCHAR(64) NULL,
        revision_number INT NOT NULL DEFAULT 1,
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Maps_Owner FOREIGN KEY (owner_user_id) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_Maps_OwnerUserID ON dbo.Maps(owner_user_id);
END;
GO

-- 2. Map_Assets Table (Assets owned by user and linked to maps)
IF OBJECT_ID(N'dbo.Map_Assets', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Map_Assets (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NULL,
        owner_user_id NVARCHAR(64) NOT NULL,
        original_filename NVARCHAR(255) NOT NULL,
        storage_path NVARCHAR(1024) NOT NULL,
        file_type NVARCHAR(128) NOT NULL,
        file_size BIGINT NOT NULL DEFAULT 0,
        mime_type NVARCHAR(128) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Map_Assets_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE SET NULL,
        CONSTRAINT FK_Map_Assets_User FOREIGN KEY (owner_user_id) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_MapAssets_MapID ON dbo.Map_Assets(map_id);
    CREATE INDEX IX_MapAssets_OwnerUserID ON dbo.Map_Assets(owner_user_id);
END;
GO

-- 3. Map_Shares Table (Explicit user-to-user sharing permissions)
IF OBJECT_ID(N'dbo.Map_Shares', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Map_Shares (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        permission NVARCHAR(32) NOT NULL DEFAULT 'viewer', -- 'viewer', 'editor', 'download'
        shared_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapShares_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapShares_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id),
        CONSTRAINT FK_MapShares_SharedBy FOREIGN KEY (shared_by) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_MapShares_Map_User UNIQUE (map_id, user_id)
    );
    CREATE INDEX IX_MapShares_UserID ON dbo.Map_Shares(user_id);
    CREATE INDEX IX_MapShares_MapID ON dbo.Map_Shares(map_id);
END;
GO

-- 4. Map_Share_Invitations Table (Email-based map invitations via Gmail SMTP)
IF OBJECT_ID(N'dbo.Map_Share_Invitations', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Map_Share_Invitations (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        email NVARCHAR(255) NOT NULL,
        permission NVARCHAR(32) NOT NULL DEFAULT 'viewer',
        token_hash NVARCHAR(128) NOT NULL,
        status NVARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending', 'accepted', 'declined', 'revoked'
        invited_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        expires_at DATETIMEOFFSET NOT NULL,
        CONSTRAINT FK_MapShareInvitations_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapShareInvitations_Inviter FOREIGN KEY (invited_by) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_MapShareInvitations_Token UNIQUE (token_hash)
    );
    CREATE INDEX IX_MapShareInvitations_Email ON dbo.Map_Share_Invitations(email);
    CREATE INDEX IX_MapShareInvitations_MapID ON dbo.Map_Share_Invitations(map_id);
END;
GO

-- 5. Share_Links Table (Secure shareable URLs with optional expiry)
IF OBJECT_ID(N'dbo.Share_Links', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Share_Links (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        token_hash NVARCHAR(128) NOT NULL,
        permission NVARCHAR(32) NOT NULL DEFAULT 'viewer', -- 'viewer', 'editor'
        is_public BIT NOT NULL DEFAULT 0,
        expires_at DATETIMEOFFSET NULL,
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_ShareLinks_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_ShareLinks_Creator FOREIGN KEY (created_by) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_ShareLinks_Token UNIQUE (token_hash)
    );
    CREATE INDEX IX_ShareLinks_MapID ON dbo.Share_Links(map_id);
END;
GO

-- 6. Map_Revisions Table (Versioned configuration JSON snapshots)
IF OBJECT_ID(N'dbo.Map_Revisions', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Map_Revisions (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        revision_number INT NOT NULL DEFAULT 1,
        configuration_json NVARCHAR(MAX) NOT NULL,
        revision_description NVARCHAR(500) NULL,
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Map_Revisions_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapRevisions_Creator FOREIGN KEY (created_by) REFERENCES dbo.Users(id),
        CONSTRAINT CK_MapRevisions_JSON CHECK (ISJSON(configuration_json) = 1)
    );
    CREATE INDEX IX_MapRevisions_Map_Rev ON dbo.Map_Revisions(map_id, revision_number);
END;
GO

-- 7. Map_Editing_Locks Table (Single-editor exclusive lock per map)
IF OBJECT_ID(N'dbo.Map_Editing_Locks', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Map_Editing_Locks (
        map_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        user_name NVARCHAR(255) NOT NULL,
        locked_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        expires_at DATETIMEOFFSET NOT NULL,
        CONSTRAINT FK_MapEditingLocks_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_Map_Editing_Locks_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id)
    );
END;
GO

-- 8. Activity_Logs Table (Action audit trail per user and map)
IF OBJECT_ID(N'dbo.Activity_Logs', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Activity_Logs (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        map_id NVARCHAR(64) NULL,
        action NVARCHAR(100) NOT NULL,
        details NVARCHAR(MAX) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_ActivityLogs_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_ActivityLogs_User ON dbo.Activity_Logs(user_id);
    CREATE INDEX IX_ActivityLogs_Map ON dbo.Activity_Logs(map_id);
END;
GO

