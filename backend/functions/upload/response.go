package upload

type UploadData struct {
	ID                 string                 `json:"id"`
	Filename           string                 `json:"filename"`
	FileSize           int64                  `json:"file_size"`
	ContentType        string                 `json:"content_type"`
	FileHash           string                 `json:"file_hash"`
	StoragePath        string                 `json:"storage_path"`
	Metadata           map[string]interface{} `json:"metadata"`
	Existing           bool                   `json:"existing"`
	IsGeoreferenced    bool                   `json:"is_georeferenced"`
	GeoreferenceStatus string                 `json:"georeference_status"`
}
