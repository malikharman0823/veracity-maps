-- ============================================================================
-- Veracity Maps 1.3: Microsoft SQL Server 2022 Initial Multi-User Schema
-- ============================================================================

-- 1. Users Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'Users')
BEGIN
    CREATE TABLE Users (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        email NVARCHAR(255) NOT NULL UNIQUE,
        name NVARCHAR(255) NOT NULL,
        password_hash NVARCHAR(255) NOT NULL,
        salt NVARCHAR(255) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
    );
    CREATE INDEX IX_Users_Email ON Users(email);
END;

-- 2. AuthTokens Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'AuthTokens')
BEGIN
    CREATE TABLE AuthTokens (
        token NVARCHAR(128) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        expires_at DATETIMEOFFSET NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_AuthTokens_User FOREIGN KEY (user_id) REFERENCES Users(id) ON DELETE CASCADE
    );
    CREATE INDEX IX_AuthTokens_UserID ON AuthTokens(user_id);
    CREATE INDEX IX_AuthTokens_ExpiresAt ON AuthTokens(expires_at);
END;

-- 3. Organizations Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'Organizations')
BEGIN
    CREATE TABLE Organizations (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(MAX) NULL,
        created_by NVARCHAR(64) NOT NULL,
        storage_used_bytes BIGINT NOT NULL DEFAULT 0,
        storage_quota_bytes BIGINT NOT NULL DEFAULT 5368709120, -- 5GB default
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Organizations_CreatedBy FOREIGN KEY (created_by) REFERENCES Users(id)
    );
    CREATE INDEX IX_Organizations_CreatedBy ON Organizations(created_by);
END;

-- 4. Projects Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'Projects')
BEGIN
    CREATE TABLE Projects (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        user_id NVARCHAR(64) NOT NULL,
        org_id NVARCHAR(64) NULL,
        owner_type NVARCHAR(32) NOT NULL DEFAULT 'personal', -- 'personal' or 'organization'
        name NVARCHAR(255) NOT NULL,
        description NVARCHAR(MAX) NULL,
        primary_dataset_id NVARCHAR(64) NULL,
        dataset_ids NVARCHAR(MAX) NULL, -- JSON array of dataset IDs
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Projects_User FOREIGN KEY (user_id) REFERENCES Users(id),
        CONSTRAINT FK_Projects_Org FOREIGN KEY (org_id) REFERENCES Organizations(id) ON DELETE SET NULL
    );
    CREATE INDEX IX_Projects_UserID ON Projects(user_id);
    CREATE INDEX IX_Projects_OrgID ON Projects(org_id);
END;

-- 5. OrganizationMembers Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'OrganizationMembers')
BEGIN
    CREATE TABLE OrganizationMembers (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        org_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        user_email NVARCHAR(255) NOT NULL,
        user_name NVARCHAR(255) NOT NULL,
        role NVARCHAR(32) NOT NULL, -- 'owner', 'admin', 'editor', 'viewer'
        invitation_status NVARCHAR(32) NOT NULL DEFAULT 'accepted', -- 'pending', 'accepted', 'rejected'
        invited_by NVARCHAR(64) NOT NULL,
        joined_at DATETIMEOFFSET NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_OrgMembers_Org FOREIGN KEY (org_id) REFERENCES Organizations(id) ON DELETE CASCADE,
        CONSTRAINT FK_OrgMembers_User FOREIGN KEY (user_id) REFERENCES Users(id),
        CONSTRAINT UQ_Org_User UNIQUE (org_id, user_id)
    );
    CREATE INDEX IX_OrgMembers_OrgID ON OrganizationMembers(org_id);
    CREATE INDEX IX_OrgMembers_UserID ON OrganizationMembers(user_id);
    CREATE INDEX IX_OrgMembers_UserEmail ON OrganizationMembers(user_email);
END;

-- 6. OrganizationInvitations Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'OrganizationInvitations')
BEGIN
    CREATE TABLE OrganizationInvitations (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        org_id NVARCHAR(64) NOT NULL,
        email NVARCHAR(255) NOT NULL,
        role NVARCHAR(32) NOT NULL,
        token NVARCHAR(128) NOT NULL,
        status NVARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending', 'accepted', 'rejected', 'revoked'
        invited_by NVARCHAR(64) NOT NULL,
        expires_at DATETIMEOFFSET NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_OrgInv_Org FOREIGN KEY (org_id) REFERENCES Organizations(id) ON DELETE CASCADE
    );
    CREATE INDEX IX_OrgInv_OrgID ON OrganizationInvitations(org_id);
    CREATE INDEX IX_OrgInv_Email ON OrganizationInvitations(email);
    CREATE INDEX IX_OrgInv_Status ON OrganizationInvitations(status);
END;

-- 7. ProjectMembers Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'ProjectMembers')
BEGIN
    CREATE TABLE ProjectMembers (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        user_email NVARCHAR(255) NOT NULL,
        user_name NVARCHAR(255) NOT NULL,
        role NVARCHAR(32) NOT NULL, -- 'owner', 'editor', 'viewer'
        invitation_status NVARCHAR(32) NOT NULL DEFAULT 'accepted', -- 'pending', 'accepted', 'rejected'
        invited_by NVARCHAR(64) NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_ProjectMembers_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE CASCADE,
        CONSTRAINT FK_ProjectMembers_User FOREIGN KEY (user_id) REFERENCES Users(id),
        CONSTRAINT UQ_Project_User UNIQUE (project_id, user_id)
    );
    CREATE INDEX IX_ProjectMembers_ProjectID ON ProjectMembers(project_id);
    CREATE INDEX IX_ProjectMembers_UserID ON ProjectMembers(user_id);
    CREATE INDEX IX_ProjectMembers_UserEmail ON ProjectMembers(user_email);
END;

-- 8. ProjectInvitations Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'ProjectInvitations')
BEGIN
    CREATE TABLE ProjectInvitations (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NOT NULL,
        email NVARCHAR(255) NOT NULL,
        role NVARCHAR(32) NOT NULL,
        token NVARCHAR(128) NOT NULL,
        status NVARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending', 'accepted', 'rejected', 'revoked'
        invited_by NVARCHAR(64) NOT NULL,
        expires_at DATETIMEOFFSET NOT NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_ProjInv_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE CASCADE
    );
    CREATE INDEX IX_ProjInv_ProjectID ON ProjectInvitations(project_id);
    CREATE INDEX IX_ProjInv_Email ON ProjectInvitations(email);
    CREATE INDEX IX_ProjInv_Status ON ProjectInvitations(status);
END;

-- 9. ProjectActivities Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'ProjectActivities')
BEGIN
    CREATE TABLE ProjectActivities (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        user_name NVARCHAR(255) NOT NULL,
        action NVARCHAR(64) NOT NULL,
        details NVARCHAR(MAX) NULL,
        timestamp DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_ProjectActivities_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE CASCADE
    );
    CREATE INDEX IX_ProjectActivities_ProjectID ON ProjectActivities(project_id);
    CREATE INDEX IX_ProjectActivities_Timestamp ON ProjectActivities(timestamp);
END;

-- 10. OrganizationActivities Table
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'OrganizationActivities')
BEGIN
    CREATE TABLE OrganizationActivities (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        org_id NVARCHAR(64) NOT NULL,
        user_id NVARCHAR(64) NOT NULL,
        user_name NVARCHAR(255) NOT NULL,
        action NVARCHAR(64) NOT NULL,
        details NVARCHAR(MAX) NULL,
        timestamp DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_OrgActivities_Org FOREIGN KEY (org_id) REFERENCES Organizations(id) ON DELETE CASCADE
    );
    CREATE INDEX IX_OrgActivities_OrgID ON OrganizationActivities(org_id);
    CREATE INDEX IX_OrgActivities_Timestamp ON OrganizationActivities(timestamp);
END;

-- 11. Datasets Table (File & GIS Dataset Metadata Linkage)
IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'Datasets')
BEGIN
    CREATE TABLE Datasets (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        project_id NVARCHAR(64) NULL,
        owner_id NVARCHAR(64) NOT NULL,
        filename NVARCHAR(255) NOT NULL,
        storage_path NVARCHAR(MAX) NOT NULL,
        file_type NVARCHAR(32) NOT NULL,
        file_size BIGINT NOT NULL DEFAULT 0,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT FK_Datasets_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE SET NULL,
        CONSTRAINT FK_Datasets_Owner FOREIGN KEY (owner_id) REFERENCES Users(id)
    );
    CREATE INDEX IX_Datasets_ProjectID ON Datasets(project_id);
    CREATE INDEX IX_Datasets_OwnerID ON Datasets(owner_id);
END;

