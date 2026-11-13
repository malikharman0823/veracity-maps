IF OBJECT_ID(N'dbo.Legacy_Config_Promotions', N'U') IS NULL
BEGIN
    CREATE TABLE dbo.Legacy_Config_Promotions (
        legacy_session_id NVARCHAR(128) NOT NULL,
        legacy_config_id NVARCHAR(128) NOT NULL,
        map_id NVARCHAR(64) NULL,
        status NVARCHAR(32) NOT NULL,
        reason_code NVARCHAR(64) NULL,
        created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
        CONSTRAINT PK_Legacy_Config_Promotions PRIMARY KEY (legacy_session_id, legacy_config_id),
        CONSTRAINT FK_LegacyConfigPromotions_Map FOREIGN KEY (map_id) REFERENCES dbo.Maps(id) ON DELETE SET NULL
    );
END;
GO
