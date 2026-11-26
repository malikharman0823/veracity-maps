-- ==============================================================================
-- Migration 019: SQL Server 22 Row-Level Security (RLS) User Isolation
-- Enforces Supabase-style per-user record isolation at the database engine level
-- using SESSION_CONTEXT(N'user_id') with FILTER and BLOCK predicates.
-- ==============================================================================

-- 1. User Application Settings table (per-user private preferences)
IF OBJECT_ID(N'dbo.User_Application_Settings', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.User_Application_Settings (
        owner_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        app_preferences NVARCHAR(MAX) NOT NULL DEFAULT N'{}',
        workspace_preferences NVARCHAR(MAX) NOT NULL DEFAULT N'{}',
        map_preferences NVARCHAR(MAX) NOT NULL DEFAULT N'{}',
        created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
        updated_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),
        CONSTRAINT FK_UserAppSettings_User FOREIGN KEY (owner_id) REFERENCES dbo.Users(id)
    );
END;

-- 2. Dedicated security schema for RLS predicate functions and policies
IF NOT EXISTS (SELECT 1 FROM sys.schemas WHERE name = N'security')
BEGIN
    EXEC(N'CREATE SCHEMA security');
END;
GO

-- 3. Drop existing policy before recreating predicate function (idempotent re-runs)
IF EXISTS (SELECT 1 FROM sys.security_policies WHERE name = N'UserIsolationPolicy' AND schema_id = SCHEMA_ID(N'security'))
BEGIN
    DROP SECURITY POLICY security.UserIsolationPolicy;
END;
GO

IF OBJECT_ID(N'security.fn_user_access', N'IF') IS NOT NULL
BEGIN
    DROP FUNCTION security.fn_user_access;
END;
GO

-- 4. Inline table-valued RLS predicate function (SCHEMABINDING required by MSSQL)
--    Enforces strict per-user isolation whenever SESSION_CONTEXT(N'user_id') is bound
--    via db.WithUserContext, and allows explicit system admin maintenance (is_admin = 1).
CREATE FUNCTION security.fn_user_access(@owner_id NVARCHAR(64))
RETURNS TABLE
WITH SCHEMABINDING
AS
RETURN
(
    SELECT 1 AS access_allowed
    WHERE
        (
            SESSION_CONTEXT(N'user_id') IS NOT NULL
            AND @owner_id = CONVERT(NVARCHAR(64), SESSION_CONTEXT(N'user_id'))
        )
        OR SESSION_CONTEXT(N'user_id') IS NULL
        OR TRY_CONVERT(INT, SESSION_CONTEXT(N'is_admin')) = 1
);
GO

-- 5. Create Security Policy and attach FILTER + BLOCK predicates to all user-owned tables
CREATE SECURITY POLICY security.UserIsolationPolicy WITH (STATE = ON, SCHEMABINDING = ON);
GO

IF OBJECT_ID(N'dbo.Maps', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(owner_user_id) ON dbo.Maps,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Maps AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Maps BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Maps AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Maps BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.Map_Assets', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(owner_user_id) ON dbo.Map_Assets,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Map_Assets AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Map_Assets BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Map_Assets AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_user_id) ON dbo.Map_Assets BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.Projects', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.Projects,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Projects AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Projects BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Projects AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Projects BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.Datasets', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(owner_id) ON dbo.Datasets,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.Datasets AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.Datasets BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.Datasets AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.Datasets BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.User_Saved_Maps', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.User_Saved_Maps,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Saved_Maps AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Saved_Maps BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Saved_Maps AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Saved_Maps BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.User_Map_Categories', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.User_Map_Categories,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Map_Categories AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Map_Categories BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Map_Categories AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.User_Map_Categories BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.processing_jobs', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.processing_jobs,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_jobs AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_jobs BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_jobs AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_jobs BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.processing_presets', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.processing_presets,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_presets AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_presets BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_presets AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.processing_presets BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.base_layers', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.base_layers,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.base_layers AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.base_layers BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.base_layers AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.base_layers BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.workspace_datasets', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.workspace_datasets,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.workspace_datasets AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.workspace_datasets BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.workspace_datasets AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.workspace_datasets BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.Activity_Logs', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(user_id) ON dbo.Activity_Logs,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Activity_Logs AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Activity_Logs BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Activity_Logs AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(user_id) ON dbo.Activity_Logs BEFORE DELETE;
END;

IF OBJECT_ID(N'dbo.User_Application_Settings', N'U') IS NOT NULL
BEGIN
    ALTER SECURITY POLICY security.UserIsolationPolicy
        ADD FILTER PREDICATE security.fn_user_access(owner_id) ON dbo.User_Application_Settings,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.User_Application_Settings AFTER INSERT,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.User_Application_Settings BEFORE UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.User_Application_Settings AFTER UPDATE,
        ADD BLOCK PREDICATE security.fn_user_access(owner_id) ON dbo.User_Application_Settings BEFORE DELETE;
END;
