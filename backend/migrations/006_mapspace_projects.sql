-- ============================================================================
-- Veracity Maps 1.3: Migration 006 - Mapspace Projects (Folders) Schema
-- Establishes Mapspace -> Projects (Folders) -> Maps hierarchy
-- ============================================================================

-- Ensure pre-existing constraints do not block idempotent execution
IF OBJECT_ID(N'dbo.FK_MapProjects_Mapspace') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_MapProjects_Mapspace NVARCHAR(512);
    SELECT @parentTable_FK_MapProjects_Mapspace = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_MapProjects_Mapspace');
    IF @parentTable_FK_MapProjects_Mapspace IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_MapProjects_Mapspace + ' DROP CONSTRAINT [FK_MapProjects_Mapspace];');
END;
IF OBJECT_ID(N'dbo.FK_MapProjects_CreatedBy') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_MapProjects_CreatedBy NVARCHAR(512);
    SELECT @parentTable_FK_MapProjects_CreatedBy = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_MapProjects_CreatedBy');
    IF @parentTable_FK_MapProjects_CreatedBy IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_MapProjects_CreatedBy + ' DROP CONSTRAINT [FK_MapProjects_CreatedBy];');
END;
IF OBJECT_ID(N'dbo.FK_Maps_Project') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_Maps_Project NVARCHAR(512);
    SELECT @parentTable_FK_Maps_Project = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_Maps_Project');
    IF @parentTable_FK_Maps_Project IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_Maps_Project + ' DROP CONSTRAINT [FK_Maps_Project];');
END;
GO

-- 1. MapProjects Table
IF OBJECT_ID(N'dbo.MapProjects', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.MapProjects (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        mapspace_id NVARCHAR(64) NOT NULL,
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(MAX) NULL,
        color NVARCHAR(32) NULL DEFAULT '#3ecf8e',
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapProjects_Mapspace FOREIGN KEY (mapspace_id) REFERENCES dbo.Mapspaces(id) ON DELETE CASCADE,
        CONSTRAINT FK_MapProjects_CreatedBy FOREIGN KEY (created_by) REFERENCES dbo.Users(id)
    );
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_MapProjects_MapspaceID' AND object_id = OBJECT_ID(N'dbo.MapProjects'))
BEGIN
    CREATE INDEX IX_MapProjects_MapspaceID ON dbo.MapProjects(mapspace_id);
END;
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_MapProjects_CreatedBy' AND object_id = OBJECT_ID(N'dbo.MapProjects'))
BEGIN
    CREATE INDEX IX_MapProjects_CreatedBy ON dbo.MapProjects(created_by);
END;
END;
GO

-- 2. Add project_id column to dbo.Maps
IF NOT EXISTS (
    SELECT 1 FROM sys.columns 
    WHERE object_id = OBJECT_ID(N'dbo.Maps') AND name = N'project_id'
)
BEGIN
    ALTER TABLE dbo.Maps ADD project_id NVARCHAR(64) NULL;
    ALTER TABLE dbo.Maps ADD CONSTRAINT FK_Maps_Project FOREIGN KEY (project_id) REFERENCES dbo.MapProjects(id) ON DELETE SET NULL;
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_Maps_ProjectID' AND object_id = OBJECT_ID(N'dbo.Maps'))
BEGIN
    CREATE INDEX IX_Maps_ProjectID ON dbo.Maps(project_id);
END;
END;
GO

