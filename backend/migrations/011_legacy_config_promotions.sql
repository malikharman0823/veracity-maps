-- Ensure pre-existing constraints do not block idempotent execution
IF OBJECT_ID(N'dbo.FK_LegacyConfigPromotions_Map') IS NOT NULL
BEGIN
    DECLARE @parentTable_FK_LegacyConfigPromotions_Map NVARCHAR(512);
    SELECT @parentTable_FK_LegacyConfigPromotions_Map = QUOTENAME(OBJECT_SCHEMA_NAME(parent_object_id)) + '.' + QUOTENAME(OBJECT_NAME(parent_object_id))
    FROM sys.objects WHERE object_id = OBJECT_ID(N'dbo.FK_LegacyConfigPromotions_Map');
    IF @parentTable_FK_LegacyConfigPromotions_Map IS NOT NULL EXEC(N'ALTER TABLE ' + @parentTable_FK_LegacyConfigPromotions_Map + ' DROP CONSTRAINT [FK_LegacyConfigPromotions_Map];');
END;
GO

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
