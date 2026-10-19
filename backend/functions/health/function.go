package health

import (
	"net/http"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

var startTime = time.Now()

type HealthResponse struct {
	Status    string        `json:"status"`
	Version   string        `json:"version"`
	Uptime    string        `json:"uptime"`
	Timestamp time.Time     `json:"timestamp"`
	GIS       gis.GDALTools `json:"gis"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	gdal := gis.DetectGDAL()
	models.WriteJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Version:   "1.0.0",
		Uptime:    time.Since(startTime).String(),
		Timestamp: time.Now(),
		GIS:       gdal,
	})
}
