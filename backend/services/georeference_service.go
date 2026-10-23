package services

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

type GeoreferenceService struct {
	repo    *repository.SessionRepository
	storage *storage.StorageManager
}

func NewGeoreferenceService(repo *repository.SessionRepository, sm *storage.StorageManager) *GeoreferenceService {
	return &GeoreferenceService{
		repo:    repo,
		storage: sm,
	}
}

func (s *GeoreferenceService) ListControlPoints(sessionID, datasetID string) ([]*models.ControlPoint, error) {
	cps, err := s.repo.ListControlPoints(sessionID, datasetID)
	if err == nil && len(cps) > 0 {
		return cps, nil
	}

	ds, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || ds == nil {
		return cps, err
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
					p.DatasetID = datasetID
				}
				_ = s.repo.SetControlPoints(sessionID, datasetID, points)
				return points, nil
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
					p.DatasetID = datasetID
				}
				_ = s.repo.SetControlPoints(sessionID, datasetID, doc.ControlPoints)
				return doc.ControlPoints, nil
			}
		}
	}

	return cps, nil
}

func getDim(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	default:
		return 0
	}
}

func (s *GeoreferenceService) CreateControlPoint(sessionID, datasetID string, cp *models.ControlPoint) (*models.ControlPoint, error) {
	if math.IsNaN(cp.ImageX) || math.IsNaN(cp.ImageY) || math.IsNaN(cp.Longitude) || math.IsNaN(cp.Latitude) {
		return nil, fmt.Errorf("INVALID_COORDINATES: Coordinates must be finite numbers")
	}
	if cp.Longitude < -180 || cp.Longitude > 180 || cp.Latitude < -90 || cp.Latitude > 90 {
		return nil, fmt.Errorf("INVALID_COORDINATES: Longitude must be [-180, 180] and latitude [-90, 90]")
	}
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}
	if w := getDim(dataset.ExtractedMetadata["width"]); w > 0 {
		h := getDim(dataset.ExtractedMetadata["height"])
		if cp.ImageX < 0 || cp.ImageX >= w || cp.ImageY < 0 || cp.ImageY >= h {
			return nil, fmt.Errorf("INVALID_CONTROL_POINT: Image coordinates must fall within raster dimensions (%.0fx%.0f)", w, h)
		}
	}

	if err := s.repo.InsertControlPoint(sessionID, datasetID, cp); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *GeoreferenceService) UpdateControlPoint(sessionID, pointID string, cp *models.ControlPoint) (*models.ControlPoint, error) {
	if math.IsNaN(cp.ImageX) || math.IsNaN(cp.ImageY) || math.IsNaN(cp.Longitude) || math.IsNaN(cp.Latitude) {
		return nil, fmt.Errorf("INVALID_COORDINATES: Coordinates must be finite numbers")
	}
	if cp.Longitude < -180 || cp.Longitude > 180 || cp.Latitude < -90 || cp.Latitude > 90 {
		return nil, fmt.Errorf("INVALID_COORDINATES: Longitude must be [-180, 180] and latitude [-90, 90]")
	}
	if err := s.repo.UpdateControlPoint(sessionID, pointID, cp); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *GeoreferenceService) DeleteControlPoint(sessionID, pointID string) (*models.ControlPoint, error) {
	return s.repo.DeleteControlPoint(sessionID, pointID)
}

func (s *GeoreferenceService) Georeference(sessionID, datasetID, action, transformation string) (*gis.TransformResult, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}

	points, err := s.repo.ListControlPoints(sessionID, datasetID)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(dataset.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("INVALID_SOURCE_IMAGE: Source image could not be opened: %w", err)
	}
	defer file.Close()

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("INVALID_SOURCE_IMAGE: Failed reading image dimensions: %w", err)
	}

	result, err := gis.CalculateGeoreference(points, cfg.Width, cfg.Height, transformation)
	if err != nil {
		return nil, err
	}

	// Update dataset metadata
	dataset.ExtractedMetadata["transform"] = result.Transform
	dataset.ExtractedMetadata["bounds"] = result.Bounds
	crsStr := "EPSG:4326"
	dataset.ExtractedMetadata["crs"] = crsStr
	dataset.ExtractedMetadata["georeference_source"] = "control_points"

	// Create processed directory
	processedDir := filepath.Join(s.storage.GetDatasetDir(sessionID, datasetID), "processed")
	_ = os.MkdirAll(processedDir, 0755)

	// Write world file
	baseExt := strings.ToLower(filepath.Ext(dataset.Filename))
	worldExt := ".wld"
	switch baseExt {
	case ".png":
		worldExt = ".pgw"
	case ".jpg", ".jpeg":
		worldExt = ".jgw"
	}
	worldPath := filepath.Join(processedDir, "georeferenced"+worldExt)
	_ = gis.WriteWorldFile(worldPath, result.Transform)

	// Also write world file in the source directory next to source file for permanence
	sourceWorldPath := filepath.Join(filepath.Dir(dataset.StoragePath), strings.TrimSuffix(filepath.Base(dataset.Filename), baseExt)+worldExt)
	_ = gis.WriteWorldFile(sourceWorldPath, result.Transform)

	// Generate VRT
	vrtPath := filepath.Join(processedDir, "georeferenced.vrt")
	_ = gis.WriteGDALVrt(vrtPath, dataset.StoragePath, cfg.Width, cfg.Height, result.Transform, "EPSG:4326")

	// If GDAL is installed, run gdal_translate to produce GeoTIFF
	tifPath := filepath.Join(processedDir, "georeferenced.tif")
	gdal := gis.DetectGDAL()
	if gdal.Available {
		cmd := exec.Command(gdal.TranslatePath, "-of", "GTiff", "-co", "COMPRESS=DEFLATE", vrtPath, tifPath)
		if out, err := cmd.CombinedOutput(); err == nil && fileExists(tifPath) {
			dataset.GeoreferencedPath = tifPath
			dataset.ProcessedPath = tifPath
		} else {
			_ = out
			dataset.GeoreferencedPath = vrtPath
			dataset.ProcessedPath = vrtPath
		}
	} else {
		dataset.GeoreferencedPath = vrtPath
		dataset.ProcessedPath = vrtPath
	}

	// Compute corner coordinates: [top-left, top-right, bottom-right, bottom-left]
	a, b, c := result.Transform[0], -result.Transform[1], result.Transform[2]
	d, e, f := result.Transform[3], -result.Transform[4], result.Transform[5]
	project := func(ix, iy float64) []float64 {
		return []float64{
			a*ix + b*(-iy) + c,
			d*ix + e*(-iy) + f,
		}
	}
	wFloat := float64(cfg.Width)
	hFloat := float64(cfg.Height)
	coordinates := [][]float64{
		project(0, 0),
		project(wFloat, 0),
		project(wFloat, hFloat),
		project(0, hFloat),
	}

	dataset.MapLayer = map[string]interface{}{
		"id":          "lyr_" + datasetID,
		"name":        dataset.Filename,
		"type":        "raster",
		"bounds":      result.Bounds,
		"bounds_crs":  "EPSG:4326",
		"coordinates": coordinates,
	}

	dataset.GeoreferenceStatus = "completed"
	_ = s.repo.UpdateDataset(sessionID, dataset)

	// Update or initialize layers
	layers, _ := s.repo.ListLayers(sessionID, datasetID)
	if len(layers) > 0 {
		layers[0].Bounds = result.MapLibreBounds
		_ = s.repo.SetLayers(sessionID, datasetID, layers)
	} else {
		layers = []*models.Layer{
			{
				ID:        "lyr_" + datasetID,
				DatasetID: datasetID,
				Name:      dataset.Filename,
				Type:      "raster",
				Visible:   true,
				Opacity:   1.0,
				Bounds:    result.MapLibreBounds,
			},
		}
		_ = s.repo.SetLayers(sessionID, datasetID, layers)
	}

	// Center MapProject camera on georeferenced dataset if project exists
	if proj, err := s.repo.GetMapProject(sessionID, datasetID); err == nil && proj != nil && proj.Configuration != nil {
		if len(result.MapLibreBounds) == 4 {
			centerLng := (result.MapLibreBounds[0] + result.MapLibreBounds[2]) / 2.0
			centerLat := (result.MapLibreBounds[1] + result.MapLibreBounds[3]) / 2.0
			proj.Configuration["camera"] = map[string]interface{}{
				"longitude": centerLng,
				"latitude":  centerLat,
				"center":    []float64{centerLng, centerLat},
				"zoom":      15.0,
				"bearing":   0.0,
				"pitch":     0.0,
			}
			_ = s.repo.SaveMapProject(sessionID, proj)
		}
	}

	// Propagate building footprint/georeferencing to all sibling floors of the building
	_ = s.PropagateBuildingGeoreference(sessionID, dataset)

	return result, nil
}

func parseBoundsMap(raw interface{}) (map[string]float64, []float64) {
	if raw == nil {
		return nil, nil
	}
	resMap := make(map[string]float64)
	if mFloat, ok := raw.(map[string]float64); ok {
		w := mFloat["min_x"]
		s := mFloat["min_y"]
		e := mFloat["max_x"]
		n := mFloat["max_y"]
		if w == 0 && s == 0 && e == 0 && n == 0 {
			w = mFloat["west"]
			s = mFloat["south"]
			e = mFloat["east"]
			n = mFloat["north"]
		}
		if e > w && n > s {
			return mFloat, []float64{w, s, e, n}
		}
	}
	if mInt, ok := raw.(map[string]interface{}); ok {
		w := getDim(mInt["min_x"])
		s := getDim(mInt["min_y"])
		e := getDim(mInt["max_x"])
		n := getDim(mInt["max_y"])
		if w == 0 && s == 0 && e == 0 && n == 0 {
			w = getDim(mInt["west"])
			s = getDim(mInt["south"])
			e = getDim(mInt["east"])
			n = getDim(mInt["north"])
		}
		if e > w && n > s {
			resMap["min_x"] = w
			resMap["min_y"] = s
			resMap["max_x"] = e
			resMap["max_y"] = n
			return resMap, []float64{w, s, e, n}
		}
	}
	if arr, ok := raw.([]float64); ok && len(arr) == 4 {
		w, s, e, n := arr[0], arr[1], arr[2], arr[3]
		if e > w && n > s {
			resMap["min_x"] = w
			resMap["min_y"] = s
			resMap["max_x"] = e
			resMap["max_y"] = n
			return resMap, []float64{w, s, e, n}
		}
	}
	return nil, nil
}

// IsFloorName returns true if a name string represents a floor or level indicator
// (e.g. "Floor 1", "Floor B1", "Floor G", "Basement 1", "Level 2") rather than a building name.
func IsFloorName(name string) bool {
	trimmed := strings.TrimSpace(name)
	lower := strings.ToLower(trimmed)
	if lower == "" {
		return false
	}
	if lower == "b1" || lower == "b2" || lower == "b3" || lower == "b4" || lower == "b5" || lower == "g" || lower == "gf" || lower == "b" {
		return true
	}
	if len(lower) == 2 && (lower[1] == 'f' || lower[1] == 'l') && lower[0] >= '0' && lower[0] <= '9' {
		return true
	}
	prefixes := []string{
		"floor", "flr", "fl_", "fl-", "level", "lvl", "basement", "ground", "cellar", "roof", "terrace",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func inferBuildingFromDataset(ds *models.Dataset, existingRealBuildings []*models.Building) string {
	if ds == nil {
		if len(existingRealBuildings) > 0 {
			return existingRealBuildings[0].Name
		}
		return "Building 1"
	}

	// 1. Check archive_entry: e.g. "Building_A/Floor_B1/building_a_floor_b1.png"
	if ds.ExtractedMetadata != nil {
		if archEntry, ok := ds.ExtractedMetadata["archive_entry"].(string); ok && archEntry != "" {
			cleanEntry := filepath.ToSlash(archEntry)
			parts := strings.Split(cleanEntry, "/")
			for _, part := range parts {
				p := strings.TrimSpace(part)
				if p == "" || p == "." || strings.EqualFold(p, "source") || strings.EqualFold(p, "images") || strings.EqualFold(p, "rasters") {
					continue
				}
				if IsFloorName(p) {
					continue
				}
				if strings.Contains(p, ".") {
					continue
				}
				clean := strings.ReplaceAll(strings.ReplaceAll(p, "-", " "), "_", " ")
				if !IsFloorName(clean) && len(clean) > 1 {
					return strings.Title(strings.ToLower(clean))
				}
			}
		}
	}

	// 2. Check filename: e.g. "building_a_floor_b1.png" or "Building A Floor B1"
	fn := ds.Filename
	if fn != "" {
		base := strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))
		lower := strings.ToLower(base)
		floorIdx := -1
		for _, token := range []string{"_floor", "-floor", " floor", "_flr", "-flr", "_lvl", "-lvl", "_level", "-level", "_b1", "_b2", "_b3", "_b4", "_g"} {
			if idx := strings.Index(lower, token); idx > 0 {
				floorIdx = idx
				break
			}
		}
		if floorIdx > 0 {
			bldPart := strings.TrimSpace(base[:floorIdx])
			if len(bldPart) > 1 && !IsFloorName(bldPart) {
				clean := strings.ReplaceAll(strings.ReplaceAll(bldPart, "-", " "), "_", " ")
				if !IsFloorName(clean) {
					return strings.Title(strings.ToLower(clean))
				}
			}
		}
	}

	// 3. Match against existing real buildings
	for _, rb := range existingRealBuildings {
		if rb.Name != "" && !IsFloorName(rb.Name) {
			cleanRb := strings.ToLower(strings.ReplaceAll(rb.Name, " ", ""))
			cleanFn := strings.ToLower(strings.ReplaceAll(fn, " ", ""))
			if strings.Contains(cleanFn, cleanRb) {
				return rb.Name
			}
		}
	}

	if fn != "" {
		base := strings.TrimSuffix(filepath.Base(fn), filepath.Ext(fn))
		clean := strings.ReplaceAll(strings.ReplaceAll(base, "-", " "), "_", " ")
		if !IsFloorName(clean) && len(clean) > 0 {
			return strings.Title(strings.ToLower(clean))
		}
	}

	return "Building 1"
}

func inferFloorDetails(ds *models.Dataset) (int, string) {
	if ds == nil {
		return 0, "Ground Floor"
	}
	if ds.ExtractedMetadata != nil {
		lvl := 0
		hasLvl := false
		if l, ok := ds.ExtractedMetadata["floor_level"].(int); ok {
			lvl = l
			hasLvl = true
		} else if lf, ok := ds.ExtractedMetadata["floor_level"].(float64); ok {
			lvl = int(lf)
			hasLvl = true
		}
		lbl, _ := ds.ExtractedMetadata["floor_label"].(string)
		if hasLvl && lbl != "" && lbl != ds.Filename {
			return lvl, lbl
		}
	}
	name := ds.Filename
	folder := ""
	if ds.ExtractedMetadata != nil {
		if arch, ok := ds.ExtractedMetadata["archive_entry"].(string); ok {
			folder = filepath.Dir(arch)
		}
	}
	return inferFloor(name, folder)
}

var consolidationMu sync.Mutex

// ConsolidateSessionBuildings detects pseudo-buildings created from floor names
// (e.g. "Floor 1", "Floor B1", "Floor G", "Floor 2") and merges them into the appropriate
// real building entity (e.g. "Building A" or "Building 1").
// Once consolidated, if any floor in the building is georeferenced, it automatically propagates
// the georeferenced footprint, bounds, and affine transform to all sibling floors.
func ConsolidateSessionBuildings(repo *repository.SessionRepository, store *storage.StorageManager, sessionID string) error {
	if repo == nil || sessionID == "" {
		return nil
	}

	consolidationMu.Lock()
	defer consolidationMu.Unlock()

	buildings, err := repo.ListBuildings(sessionID)
	if err != nil || len(buildings) == 0 {
		return nil
	}

	allDatasets, err := repo.ListDatasets(sessionID)
	if err != nil {
		return nil
	}
	datasetMap := make(map[string]*models.Dataset, len(allDatasets))
	for _, ds := range allDatasets {
		datasetMap[ds.ID] = ds
	}

	var pseudoBuildings []*models.Building
	var realBuildings []*models.Building
	for _, b := range buildings {
		if IsFloorName(b.Name) {
			pseudoBuildings = append(pseudoBuildings, b)
		} else {
			realBuildings = append(realBuildings, b)
		}
	}

	// Quick check if any building has a georeferenced floor with unreferenced siblings
	needsPropagation := false
	for _, bld := range realBuildings {
		hasCompleted := false
		hasUncompleted := false
		for _, fl := range bld.Floors {
			for _, dID := range fl.DatasetIDs {
				if ds := datasetMap[dID]; ds != nil {
					if ds.GeoreferenceStatus == "completed" && ds.ExtractedMetadata != nil && ds.ExtractedMetadata["bounds"] != nil {
						hasCompleted = true
					} else {
						hasUncompleted = true
					}
				}
			}
		}
		if hasCompleted && hasUncompleted {
			needsPropagation = true
			break
		}
	}

	if len(pseudoBuildings) == 0 && !needsPropagation {
		return nil
	}

	// 1. Merge pseudo-buildings into real buildings
	if len(pseudoBuildings) > 0 {
		for _, pb := range pseudoBuildings {
			for _, fl := range pb.Floors {
				for _, dID := range fl.DatasetIDs {
					ds := datasetMap[dID]
					if ds == nil {
						continue
					}

					targetName := inferBuildingFromDataset(ds, realBuildings)
					var targetBld *models.Building
					for _, rb := range realBuildings {
						if strings.EqualFold(rb.Name, targetName) {
							targetBld = rb
							break
						}
					}

					if targetBld == nil {
						newBld, err := repo.CreateBuilding(sessionID, &models.Building{
							Name:              targetName,
							DefaultFloorLevel: 0,
						})
						if err == nil && newBld != nil {
							targetBld = newBld
							realBuildings = append(realBuildings, targetBld)
						}
					}

					if targetBld == nil {
						continue
					}

					lvl, lbl := inferFloorDetails(ds)
					newMeta := make(map[string]interface{})
					if ds.ExtractedMetadata != nil {
						for k, v := range ds.ExtractedMetadata {
							newMeta[k] = v
						}
					}
					newMeta["building_name"] = targetBld.Name
					newMeta["floor_level"] = lvl
					newMeta["floor_label"] = lbl
					newMeta["is_building_floor"] = true
					ds.ExtractedMetadata = newMeta
					_ = repo.UpdateDataset(sessionID, ds)

					// Attach dataset to target building floor
					floorFound := false
					for _, tf := range targetBld.Floors {
						if tf.Level == lvl {
							alreadyIn := false
							for _, id := range tf.DatasetIDs {
								if id == dID {
									alreadyIn = true
									break
								}
							}
							if !alreadyIn {
								tf.DatasetIDs = append(tf.DatasetIDs, dID)
							}
							if tf.Name == "" || tf.Name == "Ground Floor" {
								tf.Name = lbl
							}
							floorFound = true
							break
						}
					}
					if !floorFound {
						targetBld.Floors = append(targetBld.Floors, &models.BuildingFloor{
							ID:         repository.GenerateFloorID(),
							BuildingID: targetBld.ID,
							Level:      lvl,
							Name:       lbl,
							DatasetIDs: []string{dID},
						})
					}
					_, _ = repo.UpdateBuilding(sessionID, targetBld.ID, targetBld)
				}
			}
			// Delete obsolete pseudo-building
			_, _ = repo.DeleteBuilding(sessionID, pb.ID)
		}
	}

	// 2. Propagate georeference for all real buildings having any georeferenced floor
	currentBuildings, _ := repo.ListBuildings(sessionID)
	for _, bld := range currentBuildings {
		var refDataset *models.Dataset
		for _, fl := range bld.Floors {
			for _, dID := range fl.DatasetIDs {
				if ds := datasetMap[dID]; ds != nil {
					if ds.GeoreferenceStatus == "completed" && ds.ExtractedMetadata != nil && ds.ExtractedMetadata["bounds"] != nil {
						refDataset = ds
						break
					}
				}
			}
			if refDataset != nil {
				break
			}
		}

		if refDataset != nil {
			_ = propagateBuildingGeoreferenceInternal(repo, store, sessionID, refDataset, false)
		}
	}

	if len(allDatasets) > 0 {
		for _, bld := range currentBuildings {
			modified := false
			for _, fl := range bld.Floors {
				validIDs := make([]string, 0, len(fl.DatasetIDs))
				for _, dID := range fl.DatasetIDs {
					if datasetMap[dID] != nil {
						validIDs = append(validIDs, dID)
					} else {
						modified = true
					}
				}
				if len(validIDs) != len(fl.DatasetIDs) {
					fl.DatasetIDs = validIDs
					modified = true
				}
			}
			if modified {
				_, _ = repo.UpdateBuilding(sessionID, bld.ID, bld)
			}
		}
	}

	return nil
}

// PropagateBuildingGeoreference propagates a georeferenced footprint, bounds, and coordinates
// to all sibling floors within the same structural building entity.
func (s *GeoreferenceService) PropagateBuildingGeoreference(sessionID string, refDataset *models.Dataset) error {
	return PropagateBuildingGeoreference(s.repo, s.storage, sessionID, refDataset)
}

// PropagateBuildingGeoreference propagates a georeferenced footprint, bounds, and coordinates
// to all sibling floors within the same structural building entity using the shared repo and storage.
func PropagateBuildingGeoreference(repo *repository.SessionRepository, store *storage.StorageManager, sessionID string, refDataset *models.Dataset) error {
	return propagateBuildingGeoreferenceInternal(repo, store, sessionID, refDataset, true)
}

func propagateBuildingGeoreferenceInternal(repo *repository.SessionRepository, store *storage.StorageManager, sessionID string, refDataset *models.Dataset, consolidate bool) error {
	if refDataset == nil || sessionID == "" {
		return nil
	}
	if refDataset.GeoreferenceStatus != "completed" || refDataset.ExtractedMetadata == nil || refDataset.ExtractedMetadata["bounds"] == nil {
		return nil
	}

	if consolidate {
		_ = ConsolidateSessionBuildings(repo, store, sessionID)
	}

	buildings, err := repo.ListBuildings(sessionID)
	if err != nil || len(buildings) == 0 {
		return nil
	}

	var matchedBld *models.Building
	var refBldName string
	if refDataset.ExtractedMetadata != nil {
		if bn, ok := refDataset.ExtractedMetadata["building_name"].(string); ok && bn != "" {
			refBldName = bn
		}
	}

	for _, b := range buildings {
		if refBldName != "" && strings.EqualFold(b.Name, refBldName) {
			matchedBld = b
			break
		}
		for _, fl := range b.Floors {
			for _, dID := range fl.DatasetIDs {
				if dID == refDataset.ID {
					matchedBld = b
					break
				}
			}
			if matchedBld != nil {
				break
			}
		}
		if matchedBld != nil {
			break
		}
	}

	if matchedBld == nil {
		return nil
	}

	// Update building bounds, shared transform, shared control points, and shared geometry
	if refBounds, ok := refDataset.ExtractedMetadata["bounds"].(map[string]interface{}); ok {
		matchedBld.Bounds = refBounds
		matchedBld.SharedRasterGeometry = refBounds
	} else if refMapBounds, ok := refDataset.MapLayer["bounds"].(map[string]interface{}); ok {
		matchedBld.Bounds = refMapBounds
		matchedBld.SharedRasterGeometry = refMapBounds
	}

	if tRaw, ok := refDataset.ExtractedMetadata["transform"].([]float64); ok && len(tRaw) == 6 {
		matchedBld.SharedTransform = tRaw
	} else if tIface, ok := refDataset.ExtractedMetadata["transform"].([]interface{}); ok && len(tIface) == 6 {
		var tVals []float64
		for _, v := range tIface {
			tVals = append(tVals, getDim(v))
		}
		matchedBld.SharedTransform = tVals
	}

	if cps, err := repo.ListControlPoints(sessionID, refDataset.ID); err == nil && len(cps) > 0 {
		matchedBld.ControlPoints = cps
	}
	_, _ = repo.UpdateBuilding(sessionID, matchedBld.ID, matchedBld)

	// Determine reference coordinates and bounds
	var refCoords [][]float64

	if refDataset.MapLayer != nil {
		if cList, ok := refDataset.MapLayer["coordinates"].([][]float64); ok && len(cList) == 4 {
			refCoords = cList
		} else if rawList, ok := refDataset.MapLayer["coordinates"].([]interface{}); ok && len(rawList) == 4 {
			for _, item := range rawList {
				if pt, ok := item.([]interface{}); ok && len(pt) == 2 {
					p0 := getDim(pt[0])
					p1 := getDim(pt[1])
					refCoords = append(refCoords, []float64{p0, p1})
				} else if ptF, ok := item.([]float64); ok && len(ptF) == 2 {
					refCoords = append(refCoords, ptF)
				}
			}
		}
	}

	_, normBounds := parseBoundsMap(refDataset.ExtractedMetadata["bounds"])
	if len(refCoords) != 4 && len(normBounds) == 4 {
		west, south, east, north := normBounds[0], normBounds[1], normBounds[2], normBounds[3]
		refCoords = [][]float64{
			{west, north},
			{east, north},
			{east, south},
			{west, south},
		}
	}

	for _, fl := range matchedBld.Floors {
		for _, dID := range fl.DatasetIDs {
			if dID == refDataset.ID {
				continue
			}
			sib, err := repo.GetDataset(sessionID, dID)
			if err != nil || sib == nil {
				continue
			}

			sib.GeoreferenceStatus = "completed"
			newMeta := make(map[string]interface{})
			if sib.ExtractedMetadata != nil {
				for k, v := range sib.ExtractedMetadata {
					newMeta[k] = v
				}
			}
			newMeta["bounds"] = refDataset.ExtractedMetadata["bounds"]
			newMeta["crs"] = refDataset.ExtractedMetadata["crs"]
			newMeta["inherited_from"] = refDataset.ID
			newMeta["building_name"] = matchedBld.Name
			newMeta["is_building_floor"] = true
			newMeta["floor_level"] = fl.Level
			newMeta["floor_label"] = fl.Name

			// Scale affine transform to sibling's own width and height
			sibW := getDim(newMeta["width"])
			sibH := getDim(newMeta["height"])
			if sibW > 0 && sibH > 0 && len(refCoords) == 4 {
				corners := gis.RasterCorners{
					TopLeft:     gis.CoordinatePoint{Longitude: refCoords[0][0], Latitude: refCoords[0][1]},
					TopRight:    gis.CoordinatePoint{Longitude: refCoords[1][0], Latitude: refCoords[1][1]},
					BottomRight: gis.CoordinatePoint{Longitude: refCoords[2][0], Latitude: refCoords[2][1]},
					BottomLeft:  gis.CoordinatePoint{Longitude: refCoords[3][0], Latitude: refCoords[3][1]},
				}
				if t, _, err := gis.CalculateTransformFromCorners(corners, int(sibW), int(sibH)); err == nil {
					newMeta["transform"] = t
					if store != nil {
						processedDir := filepath.Join(store.GetDatasetDir(sessionID, sib.ID), "processed")
						_ = os.MkdirAll(processedDir, 0755)
						baseExt := strings.ToLower(filepath.Ext(sib.Filename))
						wExt := ".wld"
						switch baseExt {
						case ".png":
							wExt = ".pgw"
						case ".jpg", ".jpeg":
							wExt = ".jgw"
						}
						_ = gis.WriteWorldFile(filepath.Join(processedDir, "georeferenced"+wExt), t)
						if sib.StoragePath != "" {
							_ = gis.WriteWorldFile(filepath.Join(filepath.Dir(sib.StoragePath), strings.TrimSuffix(filepath.Base(sib.StoragePath), baseExt)+wExt), t)
						}
						vrtPath := filepath.Join(processedDir, "georeferenced.vrt")
						_ = gis.WriteGDALVrt(vrtPath, sib.StoragePath, int(sibW), int(sibH), t, "EPSG:4326")
					}
				}
			}

			sib.ExtractedMetadata = newMeta

			sib.MapLayer = map[string]interface{}{
				"id":          "lyr_" + sib.ID,
				"name":        sib.Filename,
				"type":        "raster",
				"bounds":      refDataset.ExtractedMetadata["bounds"],
				"bounds_crs":  "EPSG:4326",
				"coordinates": refCoords,
			}
			if sib.GeoreferencedPath == "" {
				sib.GeoreferencedPath = sib.StoragePath
			}
			if sib.ProcessedPath == "" {
				sib.ProcessedPath = sib.StoragePath
			}
			_ = repo.UpdateDataset(sessionID, sib)

			// Update or add layer in repository
			lyrs, _ := repo.ListLayers(sessionID, sib.ID)
			if len(lyrs) > 0 {
				lyrs[0].Bounds = normBounds
				_ = repo.SetLayers(sessionID, sib.ID, lyrs)
			} else {
				_ = repo.SetLayers(sessionID, sib.ID, []*models.Layer{
					{
						ID:        "lyr_" + sib.ID,
						DatasetID: sib.ID,
						Name:      sib.Filename,
						Type:      "raster",
						Visible:   true,
						Opacity:   1.0,
						Bounds:    normBounds,
					},
				})
			}
		}
	}

	return nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
