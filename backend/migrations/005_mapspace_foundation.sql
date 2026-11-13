-- ============================================================================
-- Veracity Maps 1.3: Migration 005 - Mapspace Foundation Schema
-- Establishes User -> Mapspace -> Maps -> Assets domain hierarchy
-- ============================================================================

-- 1. Mapspaces Table
IF OBJECT_ID(N'dbo.Mapspaces', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Mapspaces (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        owner_user_id NVARCHAR(64) NOT NULL,
        name NVARCHAR(255) NOT NULL,
        mapspace_type NVARCHAR(32) NOT NULL DEFAULT 'personal', -- 'personal' | 'shared'
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Mapspaces_OwnerUser FOREIGN KEY (owner_user_id) REFERENCES Users(id)
    );
    CREATE INDEX IX_Mapspaces_OwnerUserID ON dbo.Mapspaces(owner_user_id);
END;
GO

-- 2. MapspaceMembers Table
IF OBJECT_ID(N'dbo.MapspaceMembers', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapspaceMembers (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        mapspace_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        role NVARCHAR(32) NOT NULL DEFAULT 'owner', -- 'owner', 'editor', 'viewer'
        permissions NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        joined_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapspaceMembers_Mapspace FOREIGN KEY (mapspace_id) REFERENCES dbo.Mapspaces(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapspaceMembers_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_MapspaceMember UNIQUE (mapspace_id, user_id),
        CONSTRAINT CK_MapspaceMembers_PermissionsJSON CHECK (ISJSON(permissions) = 1)
    );
    CREATE INDEX IX_MapspaceMembers_UserID ON dbo.MapspaceMembers(user_id);
END;
GO

-- 3. Maps Table (Max 10 saved maps per Mapspace)
IF OBJECT_ID(N'dbo.Maps', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Maps (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        mapspace_id NVARCHAR(64) NOT NULL,
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(MAX) NULL,
        current_revision_id NVARCHAR(64) NULL,
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Maps_Mapspace FOREIGN KEY (mapspace_id) REFERENCES dbo.Mapspaces(id) ON DELETE CASCADE,
        CONSTRAINT FK_Maps_CreatedBy FOREIGN KEY (created_by) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_Maps_MapspaceID ON dbo.Maps(mapspace_id);
    CREATE INDEX IX_Maps_CreatedBy ON dbo.Maps(created_by);
END;
GO

-- 4. MapRevisions Table (Immutable history of map configurations)
IF OBJECT_ID(N'dbo.MapRevisions', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapRevisions (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        parent_revision_id NVARCHAR(64) NULL,
        created_by NVARCHAR(64) NOT NULL,
        configuration_json NVARCHAR(MAX) NOT NULL,
        revision_description NVARCHAR(1024) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapRevisions_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapRevisions_CreatedBy FOREIGN KEY (created_by) REFERENCES dbo.Users(id),
        CONSTRAINT CK_MapRevisions_ConfigJSON CHECK (ISJSON(configuration_json) = 1)
    );
    CREATE INDEX IX_MapRevisions_MapID ON dbo.MapRevisions(map_id);
    CREATE INDEX IX_MapRevisions_CreatedAt ON dbo.MapRevisions(map_id, created_at);
END;
GO

-- 5. MapAssets Table (Filesystem assets owned by a specific Mapspace and Map)
IF OBJECT_ID(N'dbo.MapAssets', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapAssets (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        mapspace_id NVARCHAR(64) NOT NULL,
        map_id NVARCHAR(64) NOT NULL,
        owner_user_id NVARCHAR(64) NOT NULL,
        original_filename NVARCHAR(255) NOT NULL,
        storage_path NVARCHAR(1024) NOT NULL,
        file_type NVARCHAR(64) NOT NULL,
        file_size BIGINT NOT NULL,
        mime_type NVARCHAR(128) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapAssets_Mapspace FOREIGN KEY (mapspace_id) REFERENCES dbo.Mapspaces(id),
        CONSTRAINT FK_MapAssets_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapAssets_OwnerUser FOREIGN KEY (owner_user_id) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_MapAssets_MapID ON dbo.MapAssets(map_id);
    CREATE INDEX IX_MapAssets_MapspaceID ON dbo.MapAssets(mapspace_id);
    CREATE INDEX IX_MapAssets_OwnerUser ON dbo.MapAssets(owner_user_id);
END;
GO

-- 6. MapDrafts Table (Isolated working/unsaved state per User and Map)
IF OBJECT_ID(N'dbo.MapDrafts', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapDrafts (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        mapspace_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        base_revision_id NVARCHAR(64) NULL,
        draft_configuration_json NVARCHAR(MAX) NOT NULL,
        temporary_storage_path NVARCHAR(1024) NOT NULL,
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapDrafts_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapDrafts_Mapspace FOREIGN KEY (mapspace_id) REFERENCES dbo.Mapspaces(id),
        CONSTRAINT FK_MapDrafts_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_MapDraft_UserMap UNIQUE (map_id, user_id),
        CONSTRAINT CK_MapDrafts_ConfigJSON CHECK (ISJSON(draft_configuration_json) = 1)
    );
    CREATE INDEX IX_MapDrafts_MapUser ON dbo.MapDrafts(map_id, user_id);
END;
GO

-- Automatically provision Personal Mapspace for any existing users who lack one
INSERT INTO dbo.Mapspaces (id, owner_user_id, name, mapspace_type, created_at, updated_at)
SELECT 
    'msp_' + SUBSTRING(REPLACE(LOWER(CONVERT(NVARCHAR(36), NEWID())), '-', ''), 1, 16),
    u.id,
    u.name + '''s Mapspace',
    'personal',
    SYSDATETIMEOFFSET(),
    SYSDATETIMEOFFSET()
FROM dbo.Users u
WHERE NOT EXISTS (
    SELECT 1 FROM dbo.Mapspaces m 
    WHERE m.owner_user_id = u.id AND m.mapspace_type = 'personal'
);
GO

-- Automatically add owner membership for personal Mapspaces
INSERT INTO dbo.MapspaceMembers (id, mapspace_id, user_id, role, permissions, joined_at)
SELECT
    'msm_' + SUBSTRING(REPLACE(LOWER(CONVERT(NVARCHAR(36), NEWID())), '-', ''), 1, 16),
    m.id,
    m.owner_user_id,
    'owner',
    '{"all": true}',
    SYSDATETIMEOFFSET()
FROM dbo.Mapspaces m
WHERE NOT EXISTS (
    SELECT 1 FROM dbo.MapspaceMembers mem
    WHERE mem.mapspace_id = m.id AND mem.user_id = m.owner_user_id
);
GO

