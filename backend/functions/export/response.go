package export

type ExportResponse struct {
	ExportID    string `json:"export_id"`
	DownloadURL string `json:"download_url"`
	Format      string `json:"format"`
	Filename    string `json:"filename"`
}
