-- ============================================================================
-- Veracity Maps 1.3 — Dynamic GeoPackage Layers, Workspace Records & Rasters
-- Tables: workspace_datasets, workspace_layers, workspace_records, workspace_rasters
-- (Reuses existing map_layers, vector_features, control points & icon points)
-- ============================================================================

-- 1. workspace_datasets Table (Preserves uploaded source datasets per workspace)
IF OBJECT_ID(N'dbo.workspace_datasets', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.workspace_datasets (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        source_dataset_id NVARCHAR(64) NOT NULL,
        original_filename NVARCHAR(512) NOT NULL,
        stored_file_path NVARCHAR(1000) NOT NULL DEFAULT '',
        format NVARCHAR(64) NOT NULL DEFAULT '',
        file_size BIGINT NOT NULL DEFAULT 0,
        upload_status NVARCHAR(32) NOT NULL DEFAULT 'ready',
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceDatasets_WorkspaceID' AND object_id = OBJECT_ID(N'dbo.workspace_datasets'))
BEGIN
    CREATE INDEX IX_WorkspaceDatasets_WorkspaceID ON dbo.workspace_datasets(workspace_id, created_at DESC);
END;
GO

-- 2. workspace_layers Table (Independent selectable layers per GeoPackage / GIS dataset)
IF OBJECT_ID(N'dbo.workspace_layers', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.workspace_layers (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_dataset_id NVARCHAR(64) NOT NULL,
        workspace_id NVARCHAR(64) NOT NULL,
        gis_layer_id NVARCHAR(64) NULL,
        source_layer_name NVARCHAR(255) NOT NULL,
        display_name NVARCHAR(255) NOT NULL,
        geometry_type NVARCHAR(64) NOT NULL DEFAULT 'Unknown',
        source_crs NVARCHAR(128) NOT NULL DEFAULT '',
        feature_count INT NOT NULL DEFAULT 0,
        extent_min_x FLOAT NULL,
        extent_min_y FLOAT NULL,
        extent_max_x FLOAT NULL,
        extent_max_y FLOAT NULL,
        fields_json NVARCHAR(MAX) NOT NULL DEFAULT '[]',
        rendering_source NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        visibility BIT NOT NULL DEFAULT 1,
        opacity FLOAT NOT NULL DEFAULT 1.0,
        layer_order INT NOT NULL DEFAULT 0,
        status NVARCHAR(32) NOT NULL DEFAULT 'active',
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_WorkspaceLayers_Dataset FOREIGN KEY (workspace_dataset_id) REFERENCES dbo.workspace_datasets(id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceLayers_WorkspaceID_Order' AND object_id = OBJECT_ID(N'dbo.workspace_layers'))
BEGIN
    CREATE INDEX IX_WorkspaceLayers_WorkspaceID_Order ON dbo.workspace_layers(workspace_id, layer_order);
END;
GO

-- 3. workspace_records Table (Lightweight index for workspace records without duplicating full geometry)
IF OBJECT_ID(N'dbo.workspace_records', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.workspace_records (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        workspace_layer_id NVARCHAR(64) NULL,
        record_type NVARCHAR(32) NOT NULL,
        source_feature_id NVARCHAR(128) NOT NULL DEFAULT '',
        display_name NVARCHAR(255) NOT NULL DEFAULT '',
        geometry_type NVARCHAR(64) NOT NULL DEFAULT '',
        coordinates NVARCHAR(512) NOT NULL DEFAULT '',
        bounds NVARCHAR(512) NOT NULL DEFAULT '',
        source_reference NVARCHAR(1000) NOT NULL DEFAULT '',
        status NVARCHAR(32) NOT NULL DEFAULT 'active',
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceRecords_WorkspaceID_Type' AND object_id = OBJECT_ID(N'dbo.workspace_records'))
BEGIN
    CREATE INDEX IX_WorkspaceRecords_WorkspaceID_Type ON dbo.workspace_records(workspace_id, record_type, workspace_layer_id);
END;
GO

-- 4. workspace_rasters Table (Raster metadata registry referencing source dataset)
IF OBJECT_ID(N'dbo.workspace_rasters', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.workspace_rasters (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        workspace_layer_id NVARCHAR(64) NULL,
        dataset_id NVARCHAR(64) NOT NULL,
        display_name NVARCHAR(255) NOT NULL DEFAULT '',
        raster_path NVARCHAR(1000) NOT NULL DEFAULT '',
        width INT NOT NULL DEFAULT 0,
        height INT NOT NULL DEFAULT 0,
        band_count INT NOT NULL DEFAULT 0,
        source_crs NVARCHAR(128) NOT NULL DEFAULT '',
        resolution_x FLOAT NULL,
        resolution_y FLOAT NULL,
        bounds NVARCHAR(512) NOT NULL DEFAULT '',
        visibility BIT NOT NULL DEFAULT 1,
        opacity FLOAT NOT NULL DEFAULT 1.0,
        status NVARCHAR(32) NOT NULL DEFAULT 'ready',
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceRasters_WorkspaceID' AND object_id = OBJECT_ID(N'dbo.workspace_rasters'))
BEGIN
    CREATE INDEX IX_WorkspaceRasters_WorkspaceID ON dbo.workspace_rasters(workspace_id);
END;
GO
