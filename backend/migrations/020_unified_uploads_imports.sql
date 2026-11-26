-- ============================================================================
-- Migration 020: Unified Uploads, Atomic Imports & Workspace Layers + RLS
-- Target: Microsoft SQL Server 2022
--
-- Reuses existing dbo.Uploads (002_user_owned_persistence.sql, owner column user_id),
-- dbo.Datasets, and dbo.processing_jobs, adds unified upload metadata columns and
-- dbo.Unified_Layers, and binds both to security.UserIsolationPolicy.
-- ============================================================================

IF OBJECT_ID('dbo.Uploads', 'U') IS NOT NULL
BEGIN
    IF COL_LENGTH('dbo.Uploads', 'detected_format') IS NULL
        ALTER TABLE dbo.Uploads ADD detected_format NVARCHAR(32) NULL;
    IF COL_LENGTH('dbo.Uploads', 'import_category') IS NULL
        ALTER TABLE dbo.Uploads ADD import_category NVARCHAR(32) NULL;
    IF COL_LENGTH('dbo.Uploads', 'mime_type') IS NULL
        ALTER TABLE dbo.Uploads ADD mime_type NVARCHAR(128) NULL;
    IF COL_LENGTH('dbo.Uploads', 'upload_status') IS NULL
        ALTER TABLE dbo.Uploads ADD upload_status NVARCHAR(32) NOT NULL CONSTRAINT DF_Uploads_Status DEFAULT 'uploaded';
    IF COL_LENGTH('dbo.Uploads', 'inspection_json') IS NULL
        ALTER TABLE dbo.Uploads ADD inspection_json NVARCHAR(MAX) NULL;
END;
GO

IF OBJECT_ID('dbo.Unified_Layers', 'U') IS NULL
BEGIN
    CREATE TABLE dbo.Unified_Layers (
        id                      NVARCHAR(64)   NOT NULL PRIMARY KEY,
        owner_id                NVARCHAR(64)   NOT NULL,
        project_id              NVARCHAR(64)   NULL,
        dataset_id              NVARCHAR(64)   NOT NULL,
        name                    NVARCHAR(255)  NOT NULL,
        layer_type              NVARCHAR(32)   NOT NULL, -- 'raster' | 'vector' | 'image_overlay'
        source_format           NVARCHAR(32)   NOT NULL,
        storage_key             NVARCHAR(1024) NOT NULL,
        crs                     NVARCHAR(64)   NULL,
        extent_json             NVARCHAR(256)  NULL,
        requires_georeferencing BIT            NOT NULL DEFAULT 0,
        visibility              BIT            NOT NULL DEFAULT 1,
        opacity                 FLOAT          NOT NULL DEFAULT 1.0,
        z_index                 INT            NOT NULL DEFAULT 0,
        status                  NVARCHAR(32)   NOT NULL DEFAULT 'ready',
        created_at              DATETIME2      NOT NULL DEFAULT SYSUTCDATETIME()
    );
    CREATE INDEX IX_Unified_Layers_Owner_Project ON dbo.Unified_Layers(owner_id, project_id, z_index);
    CREATE INDEX IX_Unified_Layers_Dataset ON dbo.Unified_Layers(owner_id, dataset_id);
END;
GO

-- Extend security.UserIsolationPolicy with FILTER + BLOCK predicates for dbo.Uploads(user_id) & dbo.Unified_Layers(owner_id)
IF EXISTS (SELECT 1 FROM sys.security_policies WHERE name = 'UserIsolationPolicy' AND schema_id = SCHEMA_ID('security'))
BEGIN
    IF OBJECT_ID('dbo.Uploads', 'U') IS NOT NULL
       AND NOT EXISTS (
        SELECT 1 FROM sys.security_predicates sp
        JOIN sys.security_policies pol ON sp.object_id = pol.object_id
        WHERE pol.name = 'UserIsolationPolicy' AND sp.target_object_id = OBJECT_ID('dbo.Uploads')
    )
    BEGIN
        ALTER SECURITY POLICY security.UserIsolationPolicy
            ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.Uploads,
            ADD BLOCK PREDICATE  security.fn_user_access(user_id) ON dbo.Uploads AFTER INSERT,
            ADD BLOCK PREDICATE  security.fn_user_access(user_id) ON dbo.Uploads BEFORE UPDATE,
            ADD BLOCK PREDICATE  security.fn_user_access(user_id) ON dbo.Uploads AFTER UPDATE,
            ADD BLOCK PREDICATE  security.fn_user_access(user_id) ON dbo.Uploads BEFORE DELETE;
    END;

    IF NOT EXISTS (
        SELECT 1 FROM sys.security_predicates sp
        JOIN sys.security_policies pol ON sp.object_id = pol.object_id
        WHERE pol.name = 'UserIsolationPolicy' AND sp.target_object_id = OBJECT_ID('dbo.Unified_Layers')
    )
    BEGIN
        ALTER SECURITY POLICY security.UserIsolationPolicy
            ADD FILTER PREDICATE security.fn_user_access(owner_id) ON dbo.Unified_Layers,
            ADD BLOCK PREDICATE  security.fn_user_access(owner_id) ON dbo.Unified_Layers AFTER INSERT,
            ADD BLOCK PREDICATE  security.fn_user_access(owner_id) ON dbo.Unified_Layers BEFORE UPDATE,
            ADD BLOCK PREDICATE  security.fn_user_access(owner_id) ON dbo.Unified_Layers AFTER UPDATE,
            ADD BLOCK PREDICATE  security.fn_user_access(owner_id) ON dbo.Unified_Layers BEFORE DELETE;
    END;
END;
GO
