-- ============================================================================
-- Veracity Maps 1.4 — Phase 1: GIS Foundation Schema Migration
-- Modules A-G: Dataset Layers, Layer Groups, Map Layers, Layer Fields,
-- Vector Features (SQL Server GEOMETRY), Feature History, Layer Styles,
-- and Spatial Queries.
-- ============================================================================

-- 1. dataset_layers Table (Multi-layer inspection for GeoPackage, Shapefile, DXF, GeoTIFF)
IF OBJECT_ID(N'dbo.dataset_layers', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.dataset_layers (
        dataset_layer_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        dataset_id NVARCHAR(64) NOT NULL,
        layer_name NVARCHAR(255) NOT NULL,
        geometry_type NVARCHAR(64) NOT NULL DEFAULT 'Unknown',
        crs NVARCHAR(128) NULL,
        feature_count INT NOT NULL DEFAULT 0,
        bounds_json NVARCHAR(MAX) NULL,
        fields_json NVARCHAR(MAX) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_DatasetLayers_DatasetID' AND object_id = OBJECT_ID(N'dbo.dataset_layers'))
BEGIN
    CREATE INDEX IX_DatasetLayers_DatasetID ON dbo.dataset_layers(dataset_id);
END;
GO

-- 2. layer_groups Table (Hierarchical groups and subgroups per map)
IF OBJECT_ID(N'dbo.layer_groups', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.layer_groups (
        group_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        parent_group_id NVARCHAR(64) NULL,
        group_name NVARCHAR(255) NOT NULL,
        display_order INT NOT NULL DEFAULT 0,
        is_expanded BIT NOT NULL DEFAULT 1,
        is_visible BIT NOT NULL DEFAULT 1,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_LayerGroups_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_LayerGroups_MapID_Order' AND object_id = OBJECT_ID(N'dbo.layer_groups'))
BEGIN
    CREATE INDEX IX_LayerGroups_MapID_Order ON dbo.layer_groups(map_id, display_order);
END;
GO

-- 3. map_layers Table (Ordered raster, vector, and CAD-derived layers per map)
IF OBJECT_ID(N'dbo.map_layers', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.map_layers (
        layer_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NOT NULL,
        dataset_id NVARCHAR(64) NULL,
        dataset_layer_id NVARCHAR(64) NULL,
        layer_name NVARCHAR(255) NOT NULL,
        layer_type NVARCHAR(32) NOT NULL DEFAULT 'vector', -- 'raster', 'vector', 'cad'
        geometry_type NVARCHAR(64) NULL,
        parent_group_id NVARCHAR(64) NULL,
        display_order INT NOT NULL DEFAULT 0,
        is_visible BIT NOT NULL DEFAULT 1,
        is_locked BIT NOT NULL DEFAULT 0,
        opacity FLOAT NOT NULL DEFAULT 1.0,
        min_zoom FLOAT NOT NULL DEFAULT 0.0,
        max_zoom FLOAT NOT NULL DEFAULT 24.0,
        crs NVARCHAR(128) NULL DEFAULT 'EPSG:4326',
        bounds_json NVARCHAR(MAX) NULL,
        metadata_json NVARCHAR(MAX) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_MapLayers_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_MapLayers_MapID_Order' AND object_id = OBJECT_ID(N'dbo.map_layers'))
BEGIN
    CREATE INDEX IX_MapLayers_MapID_Order ON dbo.map_layers(map_id, display_order);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_MapLayers_DatasetID' AND object_id = OBJECT_ID(N'dbo.map_layers'))
BEGIN
    CREATE INDEX IX_MapLayers_DatasetID ON dbo.map_layers(dataset_id);
END;
GO

-- 4. layer_fields Table (Attribute schema definition for vector layers)
IF OBJECT_ID(N'dbo.layer_fields', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.layer_fields (
        field_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        layer_id NVARCHAR(64) NOT NULL,
        field_name NVARCHAR(128) NOT NULL,
        field_type NVARCHAR(32) NOT NULL DEFAULT 'string', -- 'string', 'integer', 'float', 'boolean', 'date'
        is_nullable BIT NOT NULL DEFAULT 1,
        default_value NVARCHAR(255) NULL,
        display_order INT NOT NULL DEFAULT 0,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_LayerFields_Layer FOREIGN KEY (layer_id) REFERENCES dbo.map_layers(layer_id) ON DELETE CASCADE,
        CONSTRAINT UQ_LayerFields_Layer_Name UNIQUE (layer_id, field_name)
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_LayerFields_LayerID' AND object_id = OBJECT_ID(N'dbo.layer_fields'))
BEGIN
    CREATE INDEX IX_LayerFields_LayerID ON dbo.layer_fields(layer_id, display_order);
END;
GO

-- 5. features Table (Vector features with SQL Server GEOMETRY and JSON attributes)
IF OBJECT_ID(N'dbo.features', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.features (
        feature_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        layer_id NVARCHAR(64) NOT NULL,
        geometry GEOMETRY NULL,
        geometry_wkt NVARCHAR(MAX) NOT NULL,
        geometry_type NVARCHAR(64) NOT NULL,
        srid INT NOT NULL DEFAULT 4326,
        bbox_minx FLOAT NULL,
        bbox_miny FLOAT NULL,
        bbox_maxx FLOAT NULL,
        bbox_maxy FLOAT NULL,
        attributes NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        created_by NVARCHAR(64) NULL,
        updated_by NVARCHAR(64) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Features_Layer FOREIGN KEY (layer_id) REFERENCES dbo.map_layers(layer_id) ON DELETE CASCADE,
        CONSTRAINT CK_Features_Attributes_JSON CHECK (ISJSON(attributes) = 1)
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_Features_LayerID' AND object_id = OBJECT_ID(N'dbo.features'))
BEGIN
    CREATE INDEX IX_Features_LayerID ON dbo.features(layer_id);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_Features_BBox' AND object_id = OBJECT_ID(N'dbo.features'))
BEGIN
    CREATE INDEX IX_Features_BBox ON dbo.features(layer_id, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'SIX_Features_Geometry' AND object_id = OBJECT_ID(N'dbo.features'))
BEGIN
    CREATE SPATIAL INDEX SIX_Features_Geometry ON dbo.features(geometry)
    USING GEOMETRY_AUTO_GRID
    WITH (BOUNDING_BOX = (-20037508.35, -20037508.35, 20037508.35, 20037508.35));
END;
GO

-- 6. feature_history Table (Audit & Undo/Redo history for vector feature edits)
IF OBJECT_ID(N'dbo.feature_history', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.feature_history (
        history_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        feature_id NVARCHAR(64) NOT NULL,
        layer_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NULL,
        operation NVARCHAR(32) NOT NULL, -- 'create', 'update', 'delete', 'split', 'merge', 'attribute_update'
        old_data NVARCHAR(MAX) NULL,
        new_data NVARCHAR(MAX) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_FeatureHistory_FeatureID' AND object_id = OBJECT_ID(N'dbo.feature_history'))
BEGIN
    CREATE INDEX IX_FeatureHistory_FeatureID ON dbo.feature_history(feature_id, created_at DESC);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_FeatureHistory_LayerID' AND object_id = OBJECT_ID(N'dbo.feature_history'))
BEGIN
    CREATE INDEX IX_FeatureHistory_LayerID ON dbo.feature_history(layer_id, created_at DESC);
END;
GO

-- 7. layer_styles Table (Symbology & MapLibre style configuration per layer)
IF OBJECT_ID(N'dbo.layer_styles', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.layer_styles (
        style_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        layer_id NVARCHAR(64) NOT NULL UNIQUE,
        style_type NVARCHAR(64) NOT NULL DEFAULT 'single', -- 'single', 'categorized', 'graduated', 'rule_based', 'heatmap'
        style_config NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        maplibre_style NVARCHAR(MAX) NULL,
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_LayerStyles_Layer FOREIGN KEY (layer_id) REFERENCES dbo.map_layers(layer_id) ON DELETE CASCADE,
        CONSTRAINT CK_LayerStyles_Config_JSON CHECK (ISJSON(style_config) = 1)
    );
END;
GO

-- 8. spatial_queries Table (Saved reusable attribute, spatial, and distance queries)
IF OBJECT_ID(N'dbo.spatial_queries', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.spatial_queries (
        query_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        map_id NVARCHAR(64) NULL,
        layer_id NVARCHAR(64) NOT NULL,
        query_name NVARCHAR(255) NOT NULL,
        query_config NVARCHAR(MAX) NOT NULL,
        created_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_SpatialQueries_Layer FOREIGN KEY (layer_id) REFERENCES dbo.map_layers(layer_id) ON DELETE CASCADE,
        CONSTRAINT CK_SpatialQueries_Config_JSON CHECK (ISJSON(query_config) = 1)
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_SpatialQueries_Map_Layer' AND object_id = OBJECT_ID(N'dbo.spatial_queries'))
BEGIN
    CREATE INDEX IX_SpatialQueries_Map_Layer ON dbo.spatial_queries(map_id, layer_id);
END;
GO
