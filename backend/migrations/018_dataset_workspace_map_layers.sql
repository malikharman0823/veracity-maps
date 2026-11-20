-- ============================================================================
-- Allow standalone dataset workspaces (ds_...) to persist GIS layers and layer
-- groups in dbo.map_layers and dbo.layer_groups alongside saved dbo.Maps rows.
-- ============================================================================

IF EXISTS (
    SELECT 1 FROM sys.foreign_keys
    WHERE name = N'FK_MapLayers_Map'
      AND parent_object_id = OBJECT_ID(N'dbo.map_layers')
)
BEGIN
    ALTER TABLE dbo.map_layers DROP CONSTRAINT FK_MapLayers_Map;
END;
GO

IF EXISTS (
    SELECT 1 FROM sys.foreign_keys
    WHERE name = N'FK_LayerGroups_Map'
      AND parent_object_id = OBJECT_ID(N'dbo.layer_groups')
)
BEGIN
    ALTER TABLE dbo.layer_groups DROP CONSTRAINT FK_LayerGroups_Map;
END;
GO
