-- Deliberate authentication cutover: invalidate every legacy bearer token.
-- Raw secrets must never be copied into the new lookup-hash table.
DROP TABLE dbo.AuthTokens;
CREATE TABLE dbo.AuthTokens (
    id NVARCHAR(64) NOT NULL PRIMARY KEY,
    token_hash CHAR(64) NOT NULL UNIQUE,
    user_id NVARCHAR(64) NOT NULL,
    expires_at DATETIMEOFFSET NOT NULL,
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    revoked_at DATETIMEOFFSET NULL,
    CONSTRAINT FK_AuthTokens_User FOREIGN KEY (user_id) REFERENCES dbo.Users(id) ON DELETE CASCADE
);
CREATE INDEX IX_AuthTokens_UserID ON dbo.AuthTokens(user_id);
CREATE INDEX IX_AuthTokens_ExpiresAt ON dbo.AuthTokens(expires_at);
