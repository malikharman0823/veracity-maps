-- ============================================================================
-- Veracity Maps 1.3: Migration 008 - Multi-User Shared Workspace Schema
-- Enforces: Users -> Workspace -> Maps -> Assets
-- ============================================================================

-- Ensure pre-existing constraints do not block idempotent execution
IF OBJECT_ID(N'dbo.FK_Workspaces_OwnerUser') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_Workspaces_OwnerUser NVARCHAR(512);
    SELECT @parentTable_FK_Workspaces_OwnerUser = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_Workspaces_OwnerUser');
    IF @parentTable_FK_Workspaces_OwnerUser IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_Workspaces_OwnerUser + ' DROP CONSTRAINT [FK_Workspaces_OwnerUser];');
END;
IF OBJECT_ID(N'dbo.FK_WorkspaceMembers_Workspace') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_WorkspaceMembers_Workspace NVARCHAR(512);
    SELECT @parentTable_FK_WorkspaceMembers_Workspace = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_WorkspaceMembers_Workspace');
    IF @parentTable_FK_WorkspaceMembers_Workspace IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_WorkspaceMembers_Workspace + ' DROP CONSTRAINT [FK_WorkspaceMembers_Workspace];');
END;
IF OBJECT_ID(N'dbo.FK_WorkspaceMembers_User') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_WorkspaceMembers_User NVARCHAR(512);
    SELECT @parentTable_FK_WorkspaceMembers_User = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_WorkspaceMembers_User');
    IF @parentTable_FK_WorkspaceMembers_User IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_WorkspaceMembers_User + ' DROP CONSTRAINT [FK_WorkspaceMembers_User];');
END;
IF OBJECT_ID(N'dbo.UQ_WorkspaceMember') IS NOT NULL
BEGIN
    DECLARE @parentTable_UQ_WorkspaceMember NVARCHAR(512);
    SELECT @parentTable_UQ_WorkspaceMember = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.UQ_WorkspaceMember');
    IF @parentTable_UQ_WorkspaceMember IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_UQ_WorkspaceMember + ' DROP CONSTRAINT [UQ_WorkspaceMember];');
END;
IF OBJECT_ID(N'dbo.FK_WorkspaceInvitations_Workspace') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_WorkspaceInvitations_Workspace NVARCHAR(512);
    SELECT @parentTable_FK_WorkspaceInvitations_Workspace = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_WorkspaceInvitations_Workspace');
    IF @parentTable_FK_WorkspaceInvitations_Workspace IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_WorkspaceInvitations_Workspace + ' DROP CONSTRAINT [FK_WorkspaceInvitations_Workspace];');
END;
IF OBJECT_ID(N'dbo.FK_WorkspaceInvitations_Inviter') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_WorkspaceInvitations_Inviter NVARCHAR(512);
    SELECT @parentTable_FK_WorkspaceInvitations_Inviter = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_WorkspaceInvitations_Inviter');
    IF @parentTable_FK_WorkspaceInvitations_Inviter IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_WorkspaceInvitations_Inviter + ' DROP CONSTRAINT [FK_WorkspaceInvitations_Inviter];');
END;
IF OBJECT_ID(N'dbo.UQ_WorkspaceInvitation_Token') IS NOT NULL
BEGIN
    DECLARE @parentTable_UQ_WorkspaceInvitation_Token NVARCHAR(512);
    SELECT @parentTable_UQ_WorkspaceInvitation_Token = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.UQ_WorkspaceInvitation_Token');
    IF @parentTable_UQ_WorkspaceInvitation_Token IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_UQ_WorkspaceInvitation_Token + ' DROP CONSTRAINT [UQ_WorkspaceInvitation_Token];');
END;
IF OBJECT_ID(N'dbo.FK_MapEditingLocks_User') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_MapEditingLocks_User NVARCHAR(512);
    SELECT @parentTable_FK_MapEditingLocks_User = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_MapEditingLocks_User');
    IF @parentTable_FK_MapEditingLocks_User IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_MapEditingLocks_User + ' DROP CONSTRAINT [FK_MapEditingLocks_User];');
END;
GO

-- 1. Workspaces Table (One Canonical Shared Workspace initially)
IF OBJECT_ID(N'dbo.Workspaces', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Workspaces (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(MAX) NULL,
        owner_user_id NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Workspaces_OwnerUser FOREIGN KEY (owner_user_id) REFERENCES dbo.Users(id)
    );
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_Workspaces_OwnerUserID' AND object_id = OBJECT_ID(N'dbo.Workspaces'))
BEGIN
    CREATE INDEX IX_Workspaces_OwnerUserID ON dbo.Workspaces(owner_user_id);
END;
END;
GO

-- 2. Workspace_Members Table (Roles: owner, editor, viewer; max 10 members)
IF OBJECT_ID(N'dbo.Workspace_Members', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Workspace_Members (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        role NVARCHAR(32) NOT NULL DEFAULT 'editor', -- 'owner', 'editor', 'viewer'
        joined_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_WorkspaceMembers_Workspace FOREIGN KEY (workspace_id) REFERENCES dbo.Workspaces(id) ON DELETE CASCADE,
        CONSTRAINT FK_WorkspaceMembers_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_WorkspaceMember UNIQUE (workspace_id, user_id)
    );
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceMembers_UserID' AND object_id = OBJECT_ID(N'dbo.Workspace_Members'))
BEGIN
    CREATE INDEX IX_WorkspaceMembers_UserID ON dbo.Workspace_Members(user_id);
END;
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceMembers_WorkspaceID' AND object_id = OBJECT_ID(N'dbo.Workspace_Members'))
BEGIN
    CREATE INDEX IX_WorkspaceMembers_WorkspaceID ON dbo.Workspace_Members(workspace_id);
END;
END;
GO

-- 3. Workspace_Invitations Table
IF OBJECT_ID(N'dbo.Workspace_Invitations', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Workspace_Invitations (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        workspace_id NVARCHAR(64) NOT NULL,
        email NVARCHAR(255) NOT NULL,
        role NVARCHAR(32) NOT NULL DEFAULT 'editor',
        token NVARCHAR(128) NOT NULL,
        status NVARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending', 'accepted', 'rejected', 'revoked'
        invited_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        expires_at DATETIMEOFFSET NOT NULL,
        CONSTRAINT FK_WorkspaceInvitations_Workspace FOREIGN KEY (workspace_id) REFERENCES dbo.Workspaces(id) ON DELETE CASCADE,
        CONSTRAINT FK_WorkspaceInvitations_Inviter FOREIGN KEY (invited_by) REFERENCES dbo.Users(id),
        CONSTRAINT UQ_WorkspaceInvitation_Token UNIQUE (token)
    );
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceInvitations_Email' AND object_id = OBJECT_ID(N'dbo.Workspace_Invitations'))
BEGIN
    CREATE INDEX IX_WorkspaceInvitations_Email ON dbo.Workspace_Invitations(email);
END;
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_WorkspaceInvitations_WorkspaceID' AND object_id = OBJECT_ID(N'dbo.Workspace_Invitations'))
BEGIN
    CREATE INDEX IX_WorkspaceInvitations_WorkspaceID ON dbo.Workspace_Invitations(workspace_id);
END;
END;
GO

-- 4. Map_Editing_Locks Table (Single editor lock per map with expiration heartbeat)
IF OBJECT_ID(N'dbo.Map_Editing_Locks', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Map_Editing_Locks (
        map_id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        user_name NVARCHAR(255) NOT NULL,
        locked_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        expires_at DATETIMEOFFSET NOT NULL,
        CONSTRAINT FK_MapEditingLocks_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id)
    );
    IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = N'IX_MapEditingLocks_ExpiresAt' AND object_id = OBJECT_ID(N'dbo.Map_Editing_Locks'))
BEGIN
    CREATE INDEX IX_MapEditingLocks_ExpiresAt ON dbo.Map_Editing_Locks(expires_at);
END;
END;
GO

