-- Keep legacy Maps tables compatible with the durable personal-map repository.
IF OBJECT_ID(N'dbo.Maps', N'U') IS NOT NULL
   AND COL_LENGTH(N'dbo.Maps', N'revision_number') IS NULL
BEGIN
    ALTER TABLE dbo.Maps ADD revision_number INT NOT NULL
        CONSTRAINT DF_Maps_RevisionNumber DEFAULT (1) WITH VALUES;
END;
GO
