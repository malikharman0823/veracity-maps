-- ============================================================================
-- Veracity Maps: Migration 013 - Dynamic Map Categories, Saved Maps & Recent Session Tracking
-- Tables: User_Recent_Maps (ephemeral per session), User_Saved_Maps (durable across sessions),
--         User_Map_Categories (dynamic category grouping per user)
-- ============================================================================

-- Ensure pre-existing constraints do not block idempotent execution
IF OBJECT_ID(N'dbo.FK_UserRecentMaps_User') IS NOT NULL
BEGIN
    DECLARE @parent_FK_UserRecentMaps_User NVARCHAR(512);
    SELECT @parent_FK_UserRecentMaps_User = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_UserRecentMaps_User');
    IF @parent_FK_UserRecentMaps_User IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_FK_UserRecentMaps_User + ' DROP CONSTRAINT [FK_UserRecentMaps_User];');
END;
IF OBJECT_ID(N'dbo.FK_UserRecentMaps_Map') IS NOT NULL
BEGIN
    DECLARE @parent_FK_UserRecentMaps_Map NVARCHAR(512);
    SELECT @parent_FK_UserRecentMaps_Map = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_UserRecentMaps_Map');
    IF @parent_FK_UserRecentMaps_Map IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_FK_UserRecentMaps_Map + ' DROP CONSTRAINT [FK_UserRecentMaps_Map];');
END;
IF OBJECT_ID(N'dbo.UQ_UserRecentMaps_User_Map_Session') IS NOT NULL
BEGIN
    DECLARE @parent_UQ_UserRecentMaps NVARCHAR(512);
    SELECT @parent_UQ_UserRecentMaps = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.UQ_UserRecentMaps_User_Map_Session');
    IF @parent_UQ_UserRecentMaps IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_UQ_UserRecentMaps + ' DROP CONSTRAINT [UQ_UserRecentMaps_User_Map_Session];');
END;

IF OBJECT_ID(N'dbo.FK_UserSavedMaps_User') IS NOT NULL
BEGIN
    DECLARE @parent_FK_UserSavedMaps_User NVARCHAR(512);
    SELECT @parent_FK_UserSavedMaps_User = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_UserSavedMaps_User');
    IF @parent_FK_UserSavedMaps_User IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_FK_UserSavedMaps_User + ' DROP CONSTRAINT [FK_UserSavedMaps_User];');
END;
IF OBJECT_ID(N'dbo.FK_UserSavedMaps_Map') IS NOT NULL
BEGIN
    DECLARE @parent_FK_UserSavedMaps_Map NVARCHAR(512);
    SELECT @parent_FK_UserSavedMaps_Map = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_UserSavedMaps_Map');
    IF @parent_FK_UserSavedMaps_Map IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_FK_UserSavedMaps_Map + ' DROP CONSTRAINT [FK_UserSavedMaps_Map];');
END;
IF OBJECT_ID(N'dbo.UQ_UserSavedMaps_User_Map') IS NOT NULL
BEGIN
    DECLARE @parent_UQ_UserSavedMaps NVARCHAR(512);
    SELECT @parent_UQ_UserSavedMaps = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.UQ_UserSavedMaps_User_Map');
    IF @parent_UQ_UserSavedMaps IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_UQ_UserSavedMaps + ' DROP CONSTRAINT [UQ_UserSavedMaps_User_Map];');
END;

IF OBJECT_ID(N'dbo.FK_UserMapCategories_User') IS NOT NULL
BEGIN
    DECLARE @parent_FK_UserMapCategories_User NVARCHAR(512);
    SELECT @parent_FK_UserMapCategories_User = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_UserMapCategories_User');
    IF @parent_FK_UserMapCategories_User IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_FK_UserMapCategories_User + ' DROP CONSTRAINT [FK_UserMapCategories_User];');
END;
IF OBJECT_ID(N'dbo.FK_UserMapCategories_Map') IS NOT NULL
BEGIN
    DECLARE @parent_FK_UserMapCategories_Map NVARCHAR(512);
    SELECT @parent_FK_UserMapCategories_Map = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_UserMapCategories_Map');
    IF @parent_FK_UserMapCategories_Map IS NOT NULL EXEC(N'ALTER TABLE ' + @parent_FK_UserMapCategories_Map + ' DROP CONSTRAINT [FK_UserMapCategories_Map];');
END;
GO

-- 1. User_Recent_Maps Table (Ephemeral per session, erased when session terminates)
IF OBJECT_ID(N'dbo.User_Recent_Maps', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.User_Recent_Maps (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        map_id NVARCHAR(64) NOT NULL,
        session_id NVARCHAR(128) NOT NULL,
        action_type NVARCHAR(64) NOT NULL DEFAULT 'opened', -- 'opened', 'viewed', 'edited', 'saved', 'created'
        work_summary NVARCHAR(500) NULL,
        opened_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_UserRecentMaps_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id) ON DELETE CASCADE,
        CONSTRAINT FK_UserRecentMaps_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT UQ_UserRecentMaps_User_Map_Session UNIQUE (user_id, map_id, session_id)
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_UserRecentMaps_User_Session' AND object_id = OBJECT_ID(N'dbo.User_Recent_Maps'))
BEGIN
    CREATE INDEX IX_UserRecentMaps_User_Session ON dbo.User_Recent_Maps(user_id, session_id);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_UserRecentMaps_Session' AND object_id = OBJECT_ID(N'dbo.User_Recent_Maps'))
BEGIN
    CREATE INDEX IX_UserRecentMaps_Session ON dbo.User_Recent_Maps(session_id);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_UserRecentMaps_OpenedAt' AND object_id = OBJECT_ID(N'dbo.User_Recent_Maps'))
BEGIN
    CREATE INDEX IX_UserRecentMaps_OpenedAt ON dbo.User_Recent_Maps(opened_at DESC);
END;
GO

-- 2. User_Saved_Maps Table (Durable per user, retained permanently across sessions)
IF OBJECT_ID(N'dbo.User_Saved_Maps', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.User_Saved_Maps (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        map_id NVARCHAR(64) NOT NULL,
        notes NVARCHAR(500) NULL,
        saved_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_UserSavedMaps_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id) ON DELETE CASCADE,
        CONSTRAINT FK_UserSavedMaps_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE,
        CONSTRAINT UQ_UserSavedMaps_User_Map UNIQUE (user_id, map_id)
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_UserSavedMaps_User' AND object_id = OBJECT_ID(N'dbo.User_Saved_Maps'))
BEGIN
    CREATE INDEX IX_UserSavedMaps_User ON dbo.User_Saved_Maps(user_id);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_UserSavedMaps_SavedAt' AND object_id = OBJECT_ID(N'dbo.User_Saved_Maps'))
BEGIN
    CREATE INDEX IX_UserSavedMaps_SavedAt ON dbo.User_Saved_Maps(saved_at DESC);
END;
GO

-- 3. User_Map_Categories Table (Dynamic category classification)
IF OBJECT_ID(N'dbo.User_Map_Categories', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.User_Map_Categories (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        category_name NVARCHAR(64) NOT NULL, -- 'my_maps', 'shared_with_me', 'saved_maps', 'recent_maps'
        map_id NVARCHAR(64) NOT NULL,
        session_id NVARCHAR(128) NULL,
        is_ephemeral BIT NOT NULL DEFAULT 0, -- 1 for session-scoped, 0 for retained
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_UserMapCategories_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id) ON DELETE CASCADE,
        CONSTRAINT FK_UserMapCategories_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE CASCADE
    );
END;
GO

IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_UserMapCategories_User_Cat' AND object_id = OBJECT_ID(N'dbo.User_Map_Categories'))
BEGIN
    CREATE INDEX IX_UserMapCategories_User_Cat ON dbo.User_Map_Categories(user_id, category_name);
END;
IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_UserMapCategories_Session' AND object_id = OBJECT_ID(N'dbo.User_Map_Categories'))
BEGIN
    CREATE INDEX IX_UserMapCategories_Session ON dbo.User_Map_Categories(session_id);
END;
GO

