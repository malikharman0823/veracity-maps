-- ============================================================================
-- Veracity Maps 1.3 — Custom Base Layers and Multiple Map Views Schema
-- Tables: base_layers, workspace_base_layers, map_view_preferences
-- ============================================================================

-- 1. base_layers Table (User-registered custom base layers + optional system records)
IF OBJECT_ID(N'dbo.base_layers', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.base_layers (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(1000) NOT NULL DEFAULT '',
        dataset_id NVARCHAR(64) NULL,
        source_type NVARCHAR(64) NOT NULL DEFAULT 'raster',
        source_crs NVARCHAR(128) NOT NULL DEFAULT '',
        extent_min_x FLOAT NULL,
        extent_min_y FLOAT NULL,
        extent_max_x FLOAT NULL,
        extent_max_y FLOAT NULL,
        preview_path NVARCHAR(1000) NOT NULL DEFAULT '',
        rendering_source NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        status NVARCHAR(32) NOT NULL DEFAULT 'ready',
        is_default BIT NOT NULL DEFAULT 0,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_BaseLayers_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_BaseLayers_UserID' AND object_id = OBJECT_ID(N'dbo.base_layers'))
BEGIN
    CREATE INDEX IX_BaseLayers_UserID ON dbo.base_layers(user_id, created_at DESC);
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_BaseLayers_DatasetID' AND object_id = OBJECT_ID(N'dbo.base_layers'))
BEGIN
    CREATE INDEX IX_BaseLayers_DatasetID ON dbo.base_layers(dataset_id);
END;
GO

-- 2. workspace_base_layers Table (Selected base layer per workspace/map)
IF OBJECT_ID(N'dbo.workspace_base_layers', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.workspace_base_layers (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        base_layer_id NVARCHAR(64) NULL,
        base_layer_type NVARCHAR(32) NOT NULL DEFAULT 'world_map',
        selected_view NVARCHAR(32) NOT NULL DEFAULT 'world',
        opacity FLOAT NOT NULL DEFAULT 1.0,
        visibility BIT NOT NULL DEFAULT 1,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT UQ_WorkspaceBaseLayers_WorkspaceID UNIQUE (workspace_id)
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceBaseLayers_BaseLayerID' AND object_id = OBJECT_ID(N'dbo.workspace_base_layers'))
BEGIN
    CREATE INDEX IX_WorkspaceBaseLayers_BaseLayerID ON dbo.workspace_base_layers(base_layer_id);
END;
GO

-- 3. map_view_preferences Table (Saved background map view & filter preferences per workspace)
IF OBJECT_ID(N'dbo.map_view_preferences', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.map_view_preferences (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        view_type NVARCHAR(32) NOT NULL DEFAULT 'world',
        style_configuration NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        show_labels BIT NOT NULL DEFAULT 1,
        show_roads BIT NOT NULL DEFAULT 1,
        show_places BIT NOT NULL DEFAULT 1,
        base_layer_opacity FLOAT NOT NULL DEFAULT 1.0,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT UQ_MapViewPreferences_WorkspaceID UNIQUE (workspace_id)
    );
END;
GO
