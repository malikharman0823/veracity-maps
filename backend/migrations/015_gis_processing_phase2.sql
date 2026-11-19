-- ============================================================================
-- Veracity Maps 1.4 — Phase 2: GIS Processing Engine Schema Migration
-- Modules A-H: Processing Jobs, Job Logs, Job Inputs, Job Outputs,
-- Topology Rules, Topology Errors (SQL Server GEOMETRY), and Processing Presets.
-- ============================================================================

-- 1. processing_jobs Table
IF OBJECT_ID(N'dbo.processing_jobs', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.processing_jobs (
        job_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NULL,
        map_id NVARCHAR(64) NULL,
        user_id NVARCHAR(64) NOT NULL,
        algorithm_name NVARCHAR(100) NOT NULL,
        status NVARCHAR(30) NOT NULL DEFAULT 'queued', -- queued, validating, running, completed, failed, cancelled
        progress INT NOT NULL DEFAULT 0,
        stage NVARCHAR(100) NOT NULL DEFAULT 'Queued',
        input_config NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        output_config NVARCHAR(MAX) NULL,
        error_code NVARCHAR(64) NULL,
        error_message NVARCHAR(MAX) NULL,
        created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
        started_at DATETIME2 NULL,
        completed_at DATETIME2 NULL
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_ProcessingJobs_User_Created' AND object_id = OBJECT_ID(N'dbo.processing_jobs'))
BEGIN
    CREATE INDEX IX_ProcessingJobs_User_Created ON dbo.processing_jobs(user_id, created_at DESC);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_ProcessingJobs_Map_Status' AND object_id = OBJECT_ID(N'dbo.processing_jobs'))
BEGIN
    CREATE INDEX IX_ProcessingJobs_Map_Status ON dbo.processing_jobs(map_id, status);
END;
GO

-- 2. processing_job_logs Table
IF OBJECT_ID(N'dbo.processing_job_logs', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.processing_job_logs (
        log_id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
        job_id NVARCHAR(64) NOT NULL,
        stage NVARCHAR(100) NOT NULL,
        log_level NVARCHAR(20) NOT NULL DEFAULT 'INFO', -- INFO, WARN, ERROR
        message NVARCHAR(MAX) NOT NULL,
        created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
        CONSTRAINT FK_ProcessingJobLogs_Job FOREIGN KEY (job_id) REFERENCES dbo.processing_jobs(job_id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_ProcessingJobLogs_JobID' AND object_id = OBJECT_ID(N'dbo.processing_job_logs'))
BEGIN
    CREATE INDEX IX_ProcessingJobLogs_JobID ON dbo.processing_job_logs(job_id, log_id ASC);
END;
GO

-- 3. processing_job_inputs Table
IF OBJECT_ID(N'dbo.processing_job_inputs', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.processing_job_inputs (
        input_id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
        job_id NVARCHAR(64) NOT NULL,
        dataset_id NVARCHAR(64) NULL,
        layer_id NVARCHAR(64) NULL,
        input_role NVARCHAR(50) NOT NULL DEFAULT 'primary',
        CONSTRAINT FK_ProcessingJobInputs_Job FOREIGN KEY (job_id) REFERENCES dbo.processing_jobs(job_id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_ProcessingJobInputs_JobID' AND object_id = OBJECT_ID(N'dbo.processing_job_inputs'))
BEGIN
    CREATE INDEX IX_ProcessingJobInputs_JobID ON dbo.processing_job_inputs(job_id);
END;
GO

-- 4. processing_job_outputs Table
IF OBJECT_ID(N'dbo.processing_job_outputs', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.processing_job_outputs (
        output_id BIGINT IDENTITY(1,1) NOT NULL PRIMARY KEY,
        job_id NVARCHAR(64) NOT NULL,
        dataset_id NVARCHAR(64) NULL,
        layer_id NVARCHAR(64) NULL,
        output_name NVARCHAR(255) NOT NULL,
        storage_path NVARCHAR(1000) NULL,
        output_format NVARCHAR(30) NOT NULL DEFAULT 'geojson',
        created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
        CONSTRAINT FK_ProcessingJobOutputs_Job FOREIGN KEY (job_id) REFERENCES dbo.processing_jobs(job_id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_ProcessingJobOutputs_JobID' AND object_id = OBJECT_ID(N'dbo.processing_job_outputs'))
BEGIN
    CREATE INDEX IX_ProcessingJobOutputs_JobID ON dbo.processing_job_outputs(job_id);
END;
GO

-- 5. topology_rules Table
IF OBJECT_ID(N'dbo.topology_rules', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.topology_rules (
        rule_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NULL,
        map_id NVARCHAR(64) NULL,
        layer_id NVARCHAR(64) NOT NULL,
        rule_name NVARCHAR(100) NOT NULL,
        rule_type NVARCHAR(100) NOT NULL,
        rule_config NVARCHAR(MAX) NULL,
        is_enabled BIT NOT NULL DEFAULT 1,
        created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_TopologyRules_LayerID' AND object_id = OBJECT_ID(N'dbo.topology_rules'))
BEGIN
    CREATE INDEX IX_TopologyRules_LayerID ON dbo.topology_rules(layer_id);
END;
GO

-- 6. topology_errors Table (with SQL Server GEOMETRY and Spatial Index)
IF OBJECT_ID(N'dbo.topology_errors', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.topology_errors (
        error_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        job_id NVARCHAR(64) NOT NULL,
        layer_id NVARCHAR(64) NOT NULL,
        feature_id NVARCHAR(64) NULL,
        error_type NVARCHAR(100) NOT NULL,
        error_message NVARCHAR(MAX) NOT NULL,
        geometry GEOMETRY NULL,
        geom_wkt NVARCHAR(MAX) NULL,
        geom_json NVARCHAR(MAX) NULL,
        suggestion NVARCHAR(500) NULL,
        is_resolved BIT NOT NULL DEFAULT 0,
        created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
        CONSTRAINT FK_TopologyErrors_Job FOREIGN KEY (job_id) REFERENCES dbo.processing_jobs(job_id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_TopologyErrors_JobID' AND object_id = OBJECT_ID(N'dbo.topology_errors'))
BEGIN
    CREATE INDEX IX_TopologyErrors_JobID ON dbo.topology_errors(job_id, is_resolved);
END;

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'SIX_TopologyErrors_Geometry' AND object_id = OBJECT_ID(N'dbo.topology_errors'))
BEGIN
    CREATE SPATIAL INDEX SIX_TopologyErrors_Geometry
    ON dbo.topology_errors(geometry)
    USING GEOMETRY_AUTO_GRID
    WITH (
        BOUNDING_BOX = (-180, -90, 180, 90),
        CELLS_PER_OBJECT = 16
    );
END;
GO

-- 7. processing_presets Table
IF OBJECT_ID(N'dbo.processing_presets', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.processing_presets (
        preset_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NULL,
        map_id NVARCHAR(64) NULL,
        user_id NVARCHAR(64) NOT NULL,
        algorithm_name NVARCHAR(100) NOT NULL,
        preset_name NVARCHAR(255) NOT NULL,
        parameters NVARCHAR(MAX) NOT NULL DEFAULT '{}',
        created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_ProcessingPresets_User_Algo' AND object_id = OBJECT_ID(N'dbo.processing_presets'))
BEGIN
    CREATE INDEX IX_ProcessingPresets_User_Algo ON dbo.processing_presets(user_id, algorithm_name);
END;
GO
