-- ============================================================================
-- Veracity Maps 1.3: Migration 008 - Multi-User Shared Workspace Schema
-- Enforces: Users -> Workspace -> Maps -> Assets
-- ============================================================================

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
    CREATE INDEX IX_Workspaces_OwnerUserID ON dbo.Workspaces(owner_user_id);
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
    CREATE INDEX IX_WorkspaceMembers_UserID ON dbo.Workspace_Members(user_id);
    CREATE INDEX IX_WorkspaceMembers_WorkspaceID ON dbo.Workspace_Members(workspace_id);
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
    CREATE INDEX IX_WorkspaceInvitations_Email ON dbo.Workspace_Invitations(email);
    CREATE INDEX IX_WorkspaceInvitations_WorkspaceID ON dbo.Workspace_Invitations(workspace_id);
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
    CREATE INDEX IX_MapEditingLocks_ExpiresAt ON dbo.Map_Editing_Locks(expires_at);
END;
GO

