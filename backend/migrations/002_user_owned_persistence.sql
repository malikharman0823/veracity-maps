-- Additive preparation for user-owned persistence. Legacy organization and raw
-- token columns remain until their repository cutovers have been verified.
ALTER TABLE Users ADD
    profile_picture_path NVARCHAR(1024) NULL,
    email_verified BIT NOT NULL CONSTRAINT DF_Users_EmailVerified DEFAULT 0,
    pending_email NVARCHAR(255) NULL,
    account_status NVARCHAR(32) NOT NULL CONSTRAINT DF_Users_Status DEFAULT 'active';

ALTER TABLE Projects ADD owner_user_id NVARCHAR(64) NULL;
GO
-- Only unambiguous personal ownership can be copied automatically.
UPDATE p SET owner_user_id = p.user_id
FROM Projects p INNER JOIN Users u ON u.id = p.user_id
WHERE p.owner_type = 'personal' AND p.org_id IS NULL;
ALTER TABLE Projects ADD CONSTRAINT FK_Projects_OwnerUser
    FOREIGN KEY (owner_user_id) REFERENCES Users(id);
CREATE INDEX IX_Projects_OwnerUserID ON Projects(owner_user_id);

ALTER TABLE ProjectMembers ADD
    permissions NVARCHAR(MAX) NOT NULL CONSTRAINT DF_ProjectMembers_Permissions DEFAULT '{}',
    joined_at DATETIMEOFFSET NULL;
GO
ALTER TABLE ProjectMembers ADD CONSTRAINT CK_ProjectMembers_PermissionsJSON CHECK (ISJSON(permissions) = 1);
UPDATE ProjectMembers SET joined_at = created_at WHERE invitation_status = 'accepted';

-- No raw token is copied into a hash field. Task 3 invalidates legacy tokens and
-- changes the primary key when the authentication repository switches to hashes.
ALTER TABLE AuthTokens ADD
    id NVARCHAR(64) NULL,
    token_hash CHAR(64) NULL,
    revoked_at DATETIMEOFFSET NULL;
GO
CREATE UNIQUE INDEX UX_AuthTokens_ID ON AuthTokens(id) WHERE id IS NOT NULL;
CREATE UNIQUE INDEX UX_AuthTokens_TokenHash ON AuthTokens(token_hash) WHERE token_hash IS NOT NULL;

ALTER TABLE ProjectInvitations ADD
    invitee_user_id NVARCHAR(64) NULL,
    invitee_key NVARCHAR(320) NULL,
    token_hash CHAR(64) NULL,
    permissions NVARCHAR(MAX) NOT NULL CONSTRAINT DF_ProjectInvitations_Permissions DEFAULT '{}',
    delivery_status NVARCHAR(32) NOT NULL CONSTRAINT DF_ProjectInvitations_Delivery DEFAULT 'pending',
    updated_at DATETIMEOFFSET NOT NULL CONSTRAINT DF_ProjectInvitations_UpdatedAt DEFAULT SYSDATETIMEOFFSET();
GO
ALTER TABLE ProjectInvitations ADD
    CONSTRAINT FK_ProjectInvitations_InviteeUser FOREIGN KEY (invitee_user_id) REFERENCES Users(id),
    CONSTRAINT CK_ProjectInvitations_PermissionsJSON CHECK (ISJSON(permissions) = 1);
CREATE UNIQUE INDEX UX_ProjectInvitations_PendingInvitee ON ProjectInvitations(project_id, invitee_key)
    WHERE status = 'pending' AND invitee_key IS NOT NULL;
CREATE UNIQUE INDEX UX_ProjectInvitations_TokenHash ON ProjectInvitations(token_hash) WHERE token_hash IS NOT NULL;

CREATE TABLE Maps (
    id NVARCHAR(64) NOT NULL PRIMARY KEY,
    project_id NVARCHAR(64) NOT NULL,
    map_name NVARCHAR(255) NOT NULL,
    map_configuration NVARCHAR(MAX) NOT NULL CONSTRAINT CK_Maps_ConfigJSON CHECK (ISJSON(map_configuration) = 1),
    created_by NVARCHAR(64) NOT NULL,
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    CONSTRAINT FK_Maps_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE CASCADE,
    CONSTRAINT FK_Maps_Creator FOREIGN KEY (created_by) REFERENCES Users(id)
);
CREATE INDEX IX_Maps_ProjectID ON Maps(project_id);

CREATE TABLE Uploads (
    id NVARCHAR(64) NOT NULL PRIMARY KEY,
    user_id NVARCHAR(64) NOT NULL,
    project_id NVARCHAR(64) NOT NULL,
    map_id NVARCHAR(64) NULL,
    original_filename NVARCHAR(255) NOT NULL,
    storage_path NVARCHAR(1024) NOT NULL,
    file_type NVARCHAR(128) NOT NULL,
    file_size BIGINT NOT NULL CONSTRAINT CK_Uploads_FileSize CHECK (file_size >= 0),
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    CONSTRAINT FK_Uploads_User FOREIGN KEY (user_id) REFERENCES Users(id),
    CONSTRAINT FK_Uploads_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE CASCADE,
    CONSTRAINT FK_Uploads_Map FOREIGN KEY (map_id) REFERENCES Maps(id)
);
CREATE INDEX IX_Uploads_UserID ON Uploads(user_id);
CREATE INDEX IX_Uploads_ProjectID ON Uploads(project_id);
CREATE INDEX IX_Uploads_MapID ON Uploads(map_id);

CREATE TABLE Activities (
    id NVARCHAR(64) NOT NULL PRIMARY KEY,
    user_id NVARCHAR(64) NOT NULL,
    project_id NVARCHAR(64) NOT NULL,
    action NVARCHAR(64) NOT NULL,
    description NVARCHAR(2048) NULL,
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    CONSTRAINT FK_Activities_User FOREIGN KEY (user_id) REFERENCES Users(id),
    CONSTRAINT FK_Activities_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE CASCADE
);
CREATE INDEX IX_Activities_ProjectCreatedAt ON Activities(project_id, created_at);

CREATE TABLE WorkspaceDrafts (
    id NVARCHAR(64) NOT NULL PRIMARY KEY,
    user_id NVARCHAR(64) NOT NULL,
    project_id NVARCHAR(64) NULL,
    map_id NVARCHAR(64) NULL,
    draft_configuration NVARCHAR(MAX) NOT NULL CONSTRAINT CK_WorkspaceDrafts_ConfigJSON CHECK (ISJSON(draft_configuration) = 1),
    storage_path NVARCHAR(1024) NOT NULL,
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    expires_at DATETIMEOFFSET NOT NULL,
    CONSTRAINT FK_WorkspaceDrafts_User FOREIGN KEY (user_id) REFERENCES Users(id),
    CONSTRAINT FK_WorkspaceDrafts_Project FOREIGN KEY (project_id) REFERENCES Projects(id),
    CONSTRAINT FK_WorkspaceDrafts_Map FOREIGN KEY (map_id) REFERENCES Maps(id)
);
CREATE INDEX IX_WorkspaceDrafts_UserID ON WorkspaceDrafts(user_id);
CREATE INDEX IX_WorkspaceDrafts_ExpiresAt ON WorkspaceDrafts(expires_at);

CREATE TABLE UserVerificationTokens (
    id NVARCHAR(64) NOT NULL PRIMARY KEY,
    user_id NVARCHAR(64) NOT NULL,
    purpose NVARCHAR(32) NOT NULL,
    requested_email NVARCHAR(255) NULL,
    token_hash CHAR(64) NOT NULL,
    expires_at DATETIMEOFFSET NOT NULL,
    consumed_at DATETIMEOFFSET NULL,
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    CONSTRAINT FK_UserVerificationTokens_User FOREIGN KEY (user_id) REFERENCES Users(id) ON DELETE CASCADE,
    CONSTRAINT UQ_UserVerificationTokens_TokenHash UNIQUE (token_hash)
);
CREATE INDEX IX_UserVerificationTokens_UserID ON UserVerificationTokens(user_id);
CREATE INDEX IX_UserVerificationTokens_ExpiresAt ON UserVerificationTokens(expires_at);
