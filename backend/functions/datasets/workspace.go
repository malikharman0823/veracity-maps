package datasets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

var defaultIcons = []string{
	"cinema",
	"fingerprint",
	"key",
	"locked-with-key",
	"locked",
	"person-police-officer",
	"security-lock",
	"shield",
	"unlocked",
	"video-call",
}

func (h *Handler) getOrCreateProject(sessionID, datasetID string) (*models.MapProject, *models.Dataset, error) {
	dataset, err := h.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return nil, nil, err
	}

	project, err := h.repo.GetMapProject(sessionID, datasetID)
	if err != nil {
		return nil, nil, err
	}

	if project == nil {
		datasetDir := filepath.Dir(dataset.StoragePath)
		studioCandidates := []string{
			filepath.Join(datasetDir, "mapstudio.json"),
			filepath.Join(datasetDir, "extracted", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "extracted", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "..", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "..", "extracted", "mapstudio.json"),
		}
		var loadedFromStudio bool
		for _, c := range studioCandidates {
			if data, err := os.ReadFile(c); err == nil {
				var doc struct {
					Project struct {
						Name          string                 `json:"name"`
						Configuration map[string]interface{} `json:"configuration"`
					} `json:"project"`
					ControlPoints []*models.ControlPoint `json:"control_points"`
				}
				if err := json.Unmarshal(data, &doc); err == nil && doc.Project.Configuration != nil {
					pName := doc.Project.Name
					if pName == "" {
						pName = strings.TrimSuffix(dataset.Filename, filepath.Ext(dataset.Filename))
					}
					project = &models.MapProject{
						ID:            "proj_" + datasetID,
						DatasetID:     datasetID,
						Name:          pName,
						Configuration: doc.Project.Configuration,
					}
					if len(doc.ControlPoints) > 0 {
						for _, cp := range doc.ControlPoints {
							cp.DatasetID = datasetID
						}
						_ = h.repo.SetControlPoints(sessionID, datasetID, doc.ControlPoints)
					}
					_ = h.repo.SaveMapProject(sessionID, project)
					loadedFromStudio = true
					break
				}
			}
		}

		if !loadedFromStudio {
			name := dataset.Filename
			if ext := filepath.Ext(name); ext != "" {
				name = strings.TrimSuffix(name, ext)
			}

			centerLng := 0.0
			centerLat := 20.0
			zoom := 1.5
			if dataset.ExtractedMetadata != nil {
				if b, ok := dataset.ExtractedMetadata["bounds"].(map[string]interface{}); ok {
					minX, ok1 := b["min_x"].(float64)
					minY, ok2 := b["min_y"].(float64)
					maxX, ok3 := b["max_x"].(float64)
					maxY, ok4 := b["max_y"].(float64)
					if ok1 && ok2 && ok3 && ok4 && maxX > minX && maxY > minY {
						centerLng = (minX + maxX) / 2.0
						centerLat = (minY + maxY) / 2.0
						zoom = 15.0
					}
				}
			}

			project = &models.MapProject{
				ID:        "proj_" + datasetID,
				DatasetID: datasetID,
				Name:      name,
				Configuration: map[string]interface{}{
					"active_view": "database",
					"camera": map[string]interface{}{
						"longitude": centerLng,
						"latitude":  centerLat,
						"center":    []float64{centerLng, centerLat},
						"zoom":      zoom,
						"bearing":   0.0,
						"pitch":     0.0,
					},
					"members": []map[string]interface{}{
						{
							"dataset_id":             dataset.ID,
							"name":                   dataset.Filename,
							"floor_label":            "Ground Floor",
							"floor_level":            0,
							"visible":                true,
							"raster_visible":         true,
							"raster_opacity":         1.0,
							"control_points_visible": true,
							"icon_points_visible":    true,
							"sort_order":             0,
						},
					},
				},
			}
			_ = h.repo.SaveMapProject(sessionID, project)
		}
	}

	if project != nil && project.Configuration != nil {
		cam, _ := project.Configuration["camera"].(map[string]interface{})
		if cam != nil {
			cLng, _ := toFloat64(cam["longitude"])
			cLat, _ := toFloat64(cam["latitude"])
			if (cLng == 0 && cLat == 20) || (cLng == 0 && cLat == 0) {
				if dataset.ExtractedMetadata != nil {
					if b, ok := dataset.ExtractedMetadata["bounds"].(map[string]interface{}); ok {
						minX, _ := toFloat64(b["min_x"])
						minY, _ := toFloat64(b["min_y"])
						maxX, _ := toFloat64(b["max_x"])
						maxY, _ := toFloat64(b["max_y"])
						if maxX > minX && maxY > minY {
							centerLng := (minX + maxX) / 2.0
							centerLat := (minY + maxY) / 2.0
							cam["longitude"] = centerLng
							cam["latitude"] = centerLat
							cam["center"] = []float64{centerLng, centerLat}
							cam["zoom"] = 15.0
							_ = h.repo.SaveMapProject(sessionID, project)
						}
					}
				}
			}
		}

		rawMembers, _ := project.Configuration["members"].([]interface{})
		for _, rm := range rawMembers {
			if m, ok := rm.(map[string]interface{}); ok {
				memDsID, _ := m["dataset_id"].(string)
				if memDsID != "" && memDsID != datasetID {
					if _, err := h.repo.GetDataset(sessionID, memDsID); err != nil {
						m["dataset_id"] = datasetID
						m["name"] = dataset.Filename
						_ = h.repo.SaveMapProject(sessionID, project)
					}
				}
			}
		}
	}

	h.ensureDatasetControlPoints(sessionID, dataset)

	return project, dataset, nil
}

func (h *Handler) ensureDatasetControlPoints(sessionID string, ds *models.Dataset) []*models.ControlPoint {
	if ds == nil {
		return nil
	}
	existing, _ := h.repo.ListControlPoints(sessionID, ds.ID)
	if len(existing) > 0 {
		return existing
	}

	datasetDir := filepath.Dir(ds.StoragePath)
	candidates := []string{
		filepath.Join(datasetDir, "control_points.json"),
		filepath.Join(datasetDir, "extracted", "control_points.json"),
		filepath.Join(datasetDir, "..", "control_points.json"),
		filepath.Join(datasetDir, "..", "extracted", "control_points.json"),
	}
	for _, c := range candidates {
		if data, err := os.ReadFile(c); err == nil {
			var points []*models.ControlPoint
			if err := json.Unmarshal(data, &points); err == nil && len(points) > 0 {
				for _, p := range points {
					p.DatasetID = ds.ID
				}
				_ = h.repo.SetControlPoints(sessionID, ds.ID, points)
				return points
			}
		}
	}

	studioCandidates := []string{
		filepath.Join(datasetDir, "mapstudio.json"),
		filepath.Join(datasetDir, "extracted", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "extracted", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "..", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "..", "extracted", "mapstudio.json"),
	}
	for _, c := range studioCandidates {
		if data, err := os.ReadFile(c); err == nil {
			var doc struct {
				ControlPoints []*models.ControlPoint `json:"control_points"`
			}
			if err := json.Unmarshal(data, &doc); err == nil && len(doc.ControlPoints) > 0 {
				for _, p := range doc.ControlPoints {
					p.DatasetID = ds.ID
				}
				_ = h.repo.SetControlPoints(sessionID, ds.ID, doc.ControlPoints)
				if cpBytes, err := json.MarshalIndent(doc.ControlPoints, "", "  "); err == nil {
					_ = os.WriteFile(filepath.Join(datasetDir, "control_points.json"), cpBytes, 0644)
				}
				return doc.ControlPoints
			}
		}
	}

	return nil
}

func ensureDatasetMapLayer(ds *models.Dataset) {
	if ds == nil || ds.MapLayer != nil {
		return
	}

	if ds.ExtractedMetadata == nil {
		ds.ExtractedMetadata = make(map[string]interface{})
	}

	// 1. If bounds or transform is missing, attempt to restore from metadata.json or mapstudio.json in dataset folder
	if ds.ExtractedMetadata["bounds"] == nil || ds.ExtractedMetadata["transform"] == nil {
		datasetDir := filepath.Dir(ds.StoragePath)
		candidates := []string{
			filepath.Join(datasetDir, "metadata.json"),
			filepath.Join(datasetDir, "extracted", "metadata.json"),
			filepath.Join(datasetDir, "..", "metadata.json"),
			filepath.Join(datasetDir, "..", "extracted", "metadata.json"),
			filepath.Join(datasetDir, "..", "..", "metadata.json"),
			filepath.Join(datasetDir, "..", "..", "extracted", "metadata.json"),
		}
		for _, c := range candidates {
			if data, err := os.ReadFile(c); err == nil {
				var mDoc struct {
					Bounds []float64 `json:"bounds"`
				}
				if err := json.Unmarshal(data, &mDoc); err == nil && len(mDoc.Bounds) == 4 {
					minX, minY, maxX, maxY := mDoc.Bounds[0], mDoc.Bounds[1], mDoc.Bounds[2], mDoc.Bounds[3]
					if maxX > minX && maxY > minY {
						ds.ExtractedMetadata["bounds"] = map[string]interface{}{
							"min_x": minX,
							"min_y": minY,
							"max_x": maxX,
							"max_y": maxY,
						}
						ds.GeoreferenceStatus = "completed"
						break
					}
				}
			}
		}

		studioCandidates := []string{
			filepath.Join(datasetDir, "mapstudio.json"),
			filepath.Join(datasetDir, "extracted", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "extracted", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "..", "mapstudio.json"),
			filepath.Join(datasetDir, "..", "..", "extracted", "mapstudio.json"),
		}
		for _, c := range studioCandidates {
			if data, err := os.ReadFile(c); err == nil {
				var doc struct {
					ControlPoints []*models.ControlPoint `json:"control_points"`
				}
				if err := json.Unmarshal(data, &doc); err == nil && len(doc.ControlPoints) >= 2 {
					wInt, _ := toFloat64(ds.ExtractedMetadata["width"])
					hInt, _ := toFloat64(ds.ExtractedMetadata["height"])
					if wInt > 0 && hInt > 0 {
						if res, err := gis.CalculateGeoreference(doc.ControlPoints, int(wInt), int(hInt), ""); err == nil && res != nil {
							ds.ExtractedMetadata["bounds"] = res.Bounds
							ds.ExtractedMetadata["transform"] = res.Transform
							ds.ExtractedMetadata["crs"] = "EPSG:4326"
							ds.ExtractedMetadata["georeference_source"] = "control_points"
							ds.GeoreferenceStatus = "completed"
							break
						}
					}
				}
			}
		}
	}

	if ds.GeoreferenceStatus != "completed" && ds.ExtractedMetadata["bounds"] == nil {
		return
	}

	var bounds interface{}
	var coordinates [][]float64
	if ds.ExtractedMetadata != nil {
		bounds = ds.ExtractedMetadata["bounds"]
		transform := ds.ExtractedMetadata["transform"]
		width, _ := toFloat64(ds.ExtractedMetadata["width"])
		height, _ := toFloat64(ds.ExtractedMetadata["height"])
		if transform != nil && width > 0 && height > 0 {
			var t []float64
			if tArr, ok := transform.([]interface{}); ok && len(tArr) == 6 {
				for _, v := range tArr {
					f, _ := toFloat64(v)
					t = append(t, f)
				}
			} else if tFloat, ok := transform.([]float64); ok && len(tFloat) == 6 {
				t = tFloat
			}
			if len(t) == 6 {
				a, b, c := t[0], -t[1], t[2]
				d, e, f := t[3], -t[4], t[5]
				project := func(ix, iy float64) []float64 {
					return []float64{
						a*ix + b*(-iy) + c,
						d*ix + e*(-iy) + f,
					}
				}
				coordinates = [][]float64{
					project(0, 0),
					project(width, 0),
					project(width, height),
					project(0, height),
				}
			}
		}

		if len(coordinates) == 0 && bounds != nil {
			if bMap, ok := bounds.(map[string]interface{}); ok {
				minX, _ := toFloat64(bMap["min_x"])
				minY, _ := toFloat64(bMap["min_y"])
				maxX, _ := toFloat64(bMap["max_x"])
				maxY, _ := toFloat64(bMap["max_y"])
				if maxX > minX && maxY > minY {
					coordinates = [][]float64{
						{minX, maxY},
						{maxX, maxY},
						{maxX, minY},
						{minX, minY},
					}
				}
			}
		}
	}

	ds.MapLayer = map[string]interface{}{
		"id":          "lyr_" + ds.ID,
		"name":        ds.Filename,
		"type":        "raster",
		"bounds":      bounds,
		"bounds_crs":  "EPSG:4326",
		"coordinates": coordinates,
	}
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func (h *Handler) buildEnrichedManifest(sessionID string, project *models.MapProject, primaryDataset *models.Dataset) map[string]interface{} {
	ensureDatasetMapLayer(primaryDataset)
	cfg := project.Configuration
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	rawMembers, _ := cfg["members"].([]interface{})
	if len(rawMembers) == 0 {
		if rawMapMembers, ok := cfg["members"].([]map[string]interface{}); ok && len(rawMapMembers) > 0 {
			for _, m := range rawMapMembers {
				rawMembers = append(rawMembers, m)
			}
		}
	}

	if len(rawMembers) == 0 {
		rawMembers = append(rawMembers, map[string]interface{}{
			"dataset_id":             primaryDataset.ID,
			"name":                   primaryDataset.Filename,
			"floor_label":            "Ground Floor",
			"floor_level":            0,
			"visible":                true,
			"raster_visible":         true,
			"raster_opacity":         1.0,
			"control_points_visible": true,
			"icon_points_visible":    true,
			"sort_order":             0,
		})
	}

	enrichedMembers := make([]map[string]interface{}, 0, len(rawMembers))
	for _, rm := range rawMembers {
		m, ok := rm.(map[string]interface{})
		if !ok {
			continue
		}
		datasetID, _ := m["dataset_id"].(string)
		if datasetID == "" {
			continue
		}

		memberCopy := make(map[string]interface{})
		for k, v := range m {
			memberCopy[k] = v
		}

		ds, err := h.repo.GetDataset(sessionID, datasetID)
		if err != nil || ds == nil {
			if datasetID == primaryDataset.ID || len(rawMembers) == 1 {
				ds = primaryDataset
				datasetID = primaryDataset.ID
				memberCopy["dataset_id"] = primaryDataset.ID
			}
		}
		if ds != nil {
			ensureDatasetMapLayer(ds)
			memberCopy["dataset"] = ds
			if memberCopy["bounds"] == nil && ds.ExtractedMetadata != nil {
				memberCopy["bounds"] = ds.ExtractedMetadata["bounds"]
			}
			if memberCopy["map_layer"] == nil && ds.MapLayer != nil {
				memberCopy["map_layer"] = ds.MapLayer
			}
		}

		if memberCopy["visible"] == nil {
			memberCopy["visible"] = true
		}
		if memberCopy["raster_visible"] == nil {
			memberCopy["raster_visible"] = true
		}
		if memberCopy["raster_opacity"] == nil {
			memberCopy["raster_opacity"] = 1.0
		}

		cps := h.ensureDatasetControlPoints(sessionID, ds)
		if len(cps) > 0 {
			if memberCopy["control_points_visible"] == nil || memberCopy["control_points_visible"] == false {
				memberCopy["control_points_visible"] = true
				if m != nil {
					m["control_points_visible"] = true
					_ = h.repo.SaveMapProject(sessionID, project)
				}
			}
		} else if memberCopy["control_points_visible"] == nil {
			memberCopy["control_points_visible"] = true
		}

		if memberCopy["icon_points_visible"] == nil {
			memberCopy["icon_points_visible"] = true
		}
		if memberCopy["floor_level"] == nil {
			memberCopy["floor_level"] = 0
		}
		if memberCopy["floor_label"] == nil || memberCopy["floor_label"] == "" {
			memberCopy["floor_label"] = memberCopy["name"]
		}

		memberCopy["sublayers"] = []map[string]interface{}{
			{
				"kind":  "raster",
				"label": "Raster Overlay",
				"count": 1,
			},
			{
				"kind":  "control_point",
				"label": "Control Points",
				"count": len(cps),
			},
			{
				"kind":  "icon_point",
				"label": "Icon Points",
				"count": 0,
			},
		}

		enrichedMembers = append(enrichedMembers, memberCopy)
	}

	activeView, _ := cfg["active_view"].(string)
	if activeView == "" {
		activeView = "database"
	}

	camMap, ok := cfg["camera"].(map[string]interface{})
	if !ok || camMap == nil {
		camMap = make(map[string]interface{})
	}
	if camMap["longitude"] == nil || camMap["latitude"] == nil {
		if centerArr, ok := camMap["center"].([]interface{}); ok && len(centerArr) == 2 {
			camMap["longitude"] = centerArr[0]
			camMap["latitude"] = centerArr[1]
		} else if centerF, ok := camMap["center"].([]float64); ok && len(centerF) == 2 {
			camMap["longitude"] = centerF[0]
			camMap["latitude"] = centerF[1]
		} else {
			camMap["longitude"] = 0.0
			camMap["latitude"] = 20.0
		}
	}
	if camMap["center"] == nil {
		camMap["center"] = []float64{camMap["longitude"].(float64), camMap["latitude"].(float64)}
	}
	if camMap["zoom"] == nil {
		camMap["zoom"] = 1.5
	}
	if camMap["bearing"] == nil {
		camMap["bearing"] = 0.0
	}
	if camMap["pitch"] == nil {
		camMap["pitch"] = 0.0
	}

	return map[string]interface{}{
		"id":          project.ID,
		"dataset_id":  primaryDataset.ID,
		"name":        project.Name,
		"active_view": activeView,
		"camera":      camMap,
		"members":     enrichedMembers,
	}
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	project, dataset, err := h.getOrCreateProject(sessionID, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	manifest := h.buildEnrichedManifest(sessionID, project, dataset)
	models.WriteJSON(w, http.StatusOK, manifest)
}

func (h *Handler) AttachWorkspaceMember(w http.ResponseWriter, r *http.Request, primaryDatasetID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	var req struct {
		DatasetID string `json:"dataset_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DatasetID == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "dataset_id is required", nil, nil)
		return
	}

	memberDataset, err := h.repo.GetDataset(sessionID, req.DatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Target member dataset not found", nil, nil)
		return
	}

	project, primaryDataset, err := h.getOrCreateProject(sessionID, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	rawMembers, _ := project.Configuration["members"].([]interface{})
	for _, rm := range rawMembers {
		if m, ok := rm.(map[string]interface{}); ok {
			if m["dataset_id"] == req.DatasetID {
				// Already attached; return manifest
				manifest := h.buildEnrichedManifest(sessionID, project, primaryDataset)
				models.WriteJSON(w, http.StatusOK, manifest)
				return
			}
		}
	}

	newMember := map[string]interface{}{
		"dataset_id":             memberDataset.ID,
		"name":                   memberDataset.Filename,
		"floor_label":            fmt.Sprintf("Floor %d", len(rawMembers)),
		"floor_level":            len(rawMembers),
		"visible":                true,
		"raster_visible":         true,
		"raster_opacity":         1.0,
		"control_points_visible": true,
		"icon_points_visible":    true,
		"sort_order":             len(rawMembers),
	}

	project.Configuration["members"] = append(rawMembers, newMember)
	_ = h.repo.SaveMapProject(sessionID, project)

	manifest := h.buildEnrichedManifest(sessionID, project, primaryDataset)
	models.WriteJSON(w, http.StatusOK, manifest)
}

func (h *Handler) PatchWorkspaceMember(w http.ResponseWriter, r *http.Request, primaryDatasetID, memberID string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only PATCH method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse JSON patch", nil, nil)
		return
	}

	project, primaryDataset, err := h.getOrCreateProject(sessionID, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	rawMembers, _ := project.Configuration["members"].([]interface{})
	updated := false
	for _, rm := range rawMembers {
		if m, ok := rm.(map[string]interface{}); ok {
			if m["dataset_id"] == memberID {
				for k, v := range patch {
					m[k] = v
				}
				updated = true
				break
			}
		}
	}

	if updated {
		project.Configuration["members"] = rawMembers
		_ = h.repo.SaveMapProject(sessionID, project)
	}

	manifest := h.buildEnrichedManifest(sessionID, project, primaryDataset)
	models.WriteJSON(w, http.StatusOK, manifest)
}

func (h *Handler) DetachWorkspaceMember(w http.ResponseWriter, r *http.Request, primaryDatasetID, memberID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	project, primaryDataset, err := h.getOrCreateProject(sessionID, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	rawMembers, _ := project.Configuration["members"].([]interface{})
	remaining := make([]interface{}, 0, len(rawMembers))
	for _, rm := range rawMembers {
		if m, ok := rm.(map[string]interface{}); ok {
			if m["dataset_id"] != memberID {
				remaining = append(remaining, rm)
			}
		}
	}

	project.Configuration["members"] = remaining
	_ = h.repo.SaveMapProject(sessionID, project)

	manifest := h.buildEnrichedManifest(sessionID, project, primaryDataset)
	models.WriteJSON(w, http.StatusOK, manifest)
}

func (h *Handler) PatchWorkspaceState(w http.ResponseWriter, r *http.Request, primaryDatasetID string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only PATCH method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse JSON patch", nil, nil)
		return
	}

	project, primaryDataset, err := h.getOrCreateProject(sessionID, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	for k, v := range patch {
		project.Configuration[k] = v
	}
	_ = h.repo.SaveMapProject(sessionID, project)

	manifest := h.buildEnrichedManifest(sessionID, project, primaryDataset)
	models.WriteJSON(w, http.StatusOK, manifest)
}

func (h *Handler) GetWorkspaceRecords(w http.ResponseWriter, r *http.Request, primaryDatasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	project, primaryDataset, err := h.getOrCreateProject(sessionID, primaryDatasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Primary dataset not found", nil, nil)
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 || pageSize > 500 {
		pageSize = 25
	}
	filterSearch := strings.ToLower(strings.TrimSpace(q.Get("search")))
	filterDataset := q.Get("member_dataset_id")
	filterKind := q.Get("kind")
	filterStatus := q.Get("status")

	manifest := h.buildEnrichedManifest(sessionID, project, primaryDataset)
	rawMembers, _ := manifest["members"].([]map[string]interface{})

	var allRecords []map[string]interface{}

	for _, m := range rawMembers {
		dsID, _ := m["dataset_id"].(string)
		if dsID == "" {
			continue
		}
		if filterDataset != "" && filterDataset != dsID {
			continue
		}

		dsName, _ := m["name"].(string)
		ds, _ := m["dataset"].(*models.Dataset)
		if ds == nil {
			ds, _ = h.repo.GetDataset(sessionID, dsID)
		}
		if ds == nil {
			continue
		}

		// 1. Raster record
		if filterKind == "" || filterKind == "raster" {
			status := ds.GeoreferenceStatus
			if filterStatus == "" || filterStatus == status {
				name := fmt.Sprintf("%s (Raster)", dsName)
				if filterSearch == "" || strings.Contains(strings.ToLower(name), filterSearch) || strings.Contains(strings.ToLower(dsName), filterSearch) {
					allRecords = append(allRecords, map[string]interface{}{
						"id":           "raster_" + ds.ID,
						"dataset_id":   ds.ID,
						"dataset_name": dsName,
						"name":         name,
						"kind":         "raster",
						"status":       status,
						"bounds":       ds.ExtractedMetadata["bounds"],
						"visible":      true,
						"updated_at":   ds.UpdatedAt,
					})
				}
			}
		}

		// 2. Control points records
		if filterKind == "" || filterKind == "control_point" {
			cps := h.ensureDatasetControlPoints(sessionID, ds)
			for idx, cp := range cps {
				name := fmt.Sprintf("GCP %d", idx+1)
				if filterSearch == "" || strings.Contains(strings.ToLower(name), filterSearch) || strings.Contains(strings.ToLower(dsName), filterSearch) {
					allRecords = append(allRecords, map[string]interface{}{
						"id":           cp.ID,
						"dataset_id":   ds.ID,
						"dataset_name": dsName,
						"name":         name,
						"kind":         "control_point",
						"status":       "paired",
						"longitude":    cp.Longitude,
						"latitude":     cp.Latitude,
						"image_x":      cp.ImageX,
						"image_y":      cp.ImageY,
						"pair_number":  idx + 1,
						"visible":      true,
						"updated_at":   cp.CreatedAt,
					})
				}
			}
		}
	}

	sortField := q.Get("sort")
	sortOrder := q.Get("order")
	if sortField == "" {
		sortField = "name"
	}
	if sortOrder == "" {
		sortOrder = "asc"
	}

	sort.Slice(allRecords, func(i, j int) bool {
		valI := fmt.Sprintf("%v", allRecords[i][sortField])
		valJ := fmt.Sprintf("%v", allRecords[j][sortField])
		if sortOrder == "desc" {
			return valI > valJ
		}
		return valI < valJ
	})

	total := len(allRecords)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	paged := allRecords[start:end]
	if paged == nil {
		paged = []map[string]interface{}{}
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"records":   paged,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     totalPages,
	})
}

func (h *Handler) ListIcons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"icons":   defaultIcons,
		"data": map[string]interface{}{
			"icons": defaultIcons,
		},
	})
}

func (h *Handler) GetIcon(w http.ResponseWriter, r *http.Request, iconName string) {
	cleanName := filepath.Base(iconName)
	if !strings.HasSuffix(cleanName, ".png") {
		cleanName += ".png"
	}

	searchPaths := []string{
		filepath.Join("..", "frontend", "public", "icons", cleanName),
		filepath.Join("frontend", "public", "icons", cleanName),
	}

	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			w.Header().Set("Content-Type", "image/png")
			http.ServeFile(w, r, p)
			return
		}
	}

	models.WriteError(w, http.StatusNotFound, "ICON_NOT_FOUND", "Icon not found: "+iconName, nil, nil)
}
