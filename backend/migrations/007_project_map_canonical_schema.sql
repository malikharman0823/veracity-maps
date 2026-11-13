-- ============================================================================
-- Veracity Maps 1.3: Migration 007 - Canonical Project-Based Storage Schema
-- Enforces: USER -> PROJECT -> MAP -> ASSETS & CONFIGURATIONS
-- ============================================================================

-- 1. Ensure Projects table has all canonical columns
IF OBJECT_ID(N'dbo.Projects', N'U') IS NOT NULL
BEGIN
    IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Projects') AND name = N'owner_user_id')
    BEGIN
        ALTER TABLE dbo.Projects ADD owner_user_id NVARCHAR(64) NULL;
    END;

    -- Ensure owner_user_id is populated from legacy user_id
    EXEC(N'UPDATE dbo.Projects SET owner_user_id = user_id WHERE owner_user_id IS NULL AND user_id IS NOT NULL;');
END;
GO

-- 2. Ensure Maps table has canonical columns
IF OBJECT_ID(N'dbo.Maps', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Maps (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NOT NULL,
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(MAX) NULL,
        current_revision_id NVARCHAR(64) NULL,
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Maps_Project FOREIGN KEY (project_id) REFERENCES dbo.Projects(id) ON DELETE CASCADE,
        CONSTRAINT FK_Maps_Creator FOREIGN KEY (created_by) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_Maps_ProjectID ON dbo.Maps(project_id);
    CREATE INDEX IX_Maps_CreatedBy ON dbo.Maps(created_by);
END
ELSE
BEGIN
    -- If Maps was created with map_name in migration 002, add name column and synchronize
    IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'name')
    BEGIN
        ALTER TABLE dbo.Maps ADD name NVARCHAR(255) NULL;
        EXEC(N'UPDATE dbo.Maps SET name = map_name WHERE name IS NULL AND map_name IS NOT NULL;');
    END;

    IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'description')
    BEGIN
        ALTER TABLE dbo.Maps ADD description NVARCHAR(MAX) NULL;
    END;

    IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'current_revision_id')
    BEGIN
        ALTER TABLE dbo.Maps ADD current_revision_id NVARCHAR(64) NULL;
    END;
END;
GO

-- 3. MapConfigurations Table (Immutable saved revisions with RevisionNumber)
IF OBJECT_ID(N'dbo.MapConfigurations', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapConfigurations (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        configuration_json NVARCHAR(MAX) NOT NULL,
        revision_number INT NOT NULL DEFAULT 1,
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapConfigurations_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapConfigurations_Creator FOREIGN KEY (created_by) REFERENCES dbo.Users(id),
        CONSTRAINT CK_MapConfigurations_JSON CHECK (ISJSON(configuration_json) = 1)
    );
    CREATE INDEX IX_MapConfigurations_MapID ON dbo.MapConfigurations(map_id);
    CREATE INDEX IX_MapConfigurations_Revision ON dbo.MapConfigurations(map_id, revision_number);
END;
GO

-- 4. MapAssets Table (Files owned by specific Map and Project)
IF OBJECT_ID(N'dbo.MapAssets', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapAssets (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NOT NULL,
        map_id NVARCHAR(64) NOT NULL,
        owner_user_id NVARCHAR(64) NOT NULL,
        original_filename NVARCHAR(255) NOT NULL,
        storage_path NVARCHAR(1024) NOT NULL,
        file_type NVARCHAR(128) NOT NULL,
        file_size BIGINT NOT NULL DEFAULT 0,
        mime_type NVARCHAR(128) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapAssets_Project FOREIGN KEY (project_id) REFERENCES dbo.Projects(id),
        CONSTRAINT FK_MapAssets_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapAssets_User FOREIGN KEY (owner_user_id) REFERENCES dbo.Users(id)
    );
    CREATE INDEX IX_MapAssets_ProjectID ON dbo.MapAssets(project_id);
    CREATE INDEX IX_MapAssets_MapID ON dbo.MapAssets(map_id);
    CREATE INDEX IX_MapAssets_UserID ON dbo.MapAssets(owner_user_id);
END;
GO

-- 5. MapDrafts Table (Unsaved temporary state isolated per user and map)
IF OBJECT_ID(N'dbo.MapDrafts', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapDrafts (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NOT NULL,
        map_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        base_revision_id NVARCHAR(64) NULL,
        draft_configuration_json NVARCHAR(MAX) NOT NULL,
        temporary_storage_path NVARCHAR(1024) NULL,
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapDrafts_Project FOREIGN KEY (project_id) REFERENCES dbo.Projects(id),
        CONSTRAINT FK_MapDrafts_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapDrafts_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_MapDrafts_MapUser UNIQUE (map_id, user_id)
    );
    CREATE INDEX IX_MapDrafts_MapUser ON dbo.MapDrafts(map_id, user_id);
END;
GO

-- 6. Activities Table MapID association
IF OBJECT_ID(N'dbo.Activities', N'U') IS NOT NULL
BEGIN
    IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID(N'dbo.Activities') AND name = N'map_id')
    BEGIN
        ALTER TABLE dbo.Activities ADD map_id NVARCHAR(64) NULL;
        ALTER TABLE dbo.Activities ADD CONSTRAINT FK_Activities_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id);
        CREATE INDEX IX_Activities_MapID ON dbo.Activities(map_id);
    END;
END;
GO

