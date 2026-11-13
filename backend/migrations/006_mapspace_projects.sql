-- ============================================================================
-- Veracity Maps 1.3: Migration 006 - Mapspace Projects (Folders) Schema
-- Establishes Mapspace -> Projects (Folders) -> Maps hierarchy
-- ============================================================================

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
    CREATE INDEX IX_MapProjects_MapspaceID ON dbo.MapProjects(mapspace_id);
    CREATE INDEX IX_MapProjects_CreatedBy ON dbo.MapProjects(created_by);
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
    CREATE INDEX IX_Maps_ProjectID ON dbo.Maps(project_id);
END;
GO

