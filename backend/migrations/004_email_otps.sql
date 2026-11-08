-- ============================================================================
-- Migration 004: Email OTP System for Passwordless Login & Registration
-- ============================================================================

IF NOT EXISTS (SELECT * FROM sys.tables WHERE name = 'EmailOTPs')
BEGIN
    CREATE TABLE EmailOTPs (
        id NVARCHAR(64) NOT NULL PRIMARY KEY,
        email NVARCHAR(255) NOT NULL,
        name NVARCHAR(255) NULL,
        otp_hash CHAR(64) NOT NULL,
        purpose NVARCHAR(32) NOT NULL,
        attempts INT NOT NULL CONSTRAINT DF_EmailOTPs_Attempts DEFAULT 0,
        max_attempts INT NOT NULL CONSTRAINT DF_EmailOTPs_MaxAttempts DEFAULT 5,
        expires_at DATETIMEOFFSET NOT NULL,
        resend_available_at DATETIMEOFFSET NOT NULL,
        consumed_at DATETIMEOFFSET NULL,
        created_at DATETIMEOFFSET NOT NULL CONSTRAINT DF_EmailOTPs_CreatedAt DEFAULT SYSDATETIMEOFFSET()
    );
    CREATE INDEX IX_EmailOTPs_EmailPurpose ON EmailOTPs(email, purpose);
    CREATE INDEX IX_EmailOTPs_ExpiresAt ON EmailOTPs(expires_at);
END;

