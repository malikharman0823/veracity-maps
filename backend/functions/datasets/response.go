package datasets

import "github.com/malikharman0823/veracity-maps/models"

type CreateDatasetResponse struct {
	DatasetID     string `json:"datasetId"`
	Name          string `json:"name"`
	Format        string `json:"format"`
	Georeferenced bool   `json:"georeferenced"`
	Status        string `json:"status"`
}

type DatasetListResponse struct {
	Datasets []*models.Dataset `json:"datasets"`
}

type DeleteDatasetResponse struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

type LayersResponse struct {
	Layers []*models.Layer `json:"layers"`
}
