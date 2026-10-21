package repository

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

var (
	ErrSessionNotFound      = errors.New("SESSION_NOT_FOUND")
	ErrDatasetNotFound      = errors.New("DATASET_NOT_FOUND")
	ErrControlPointNotFound = errors.New("CONTROL_POINT_NOT_FOUND")
	ErrLayerNotFound        = errors.New("LAYER_NOT_FOUND")
)

type Session struct {
	ID            string
	CreatedAt     time.Time
	LastActiveAt  time.Time
	Datasets      map[string]*models.Dataset
	ControlPoints map[string][]*models.ControlPoint // datasetID -> control points
	Layers        map[string][]*models.Layer        // datasetID -> layers
	MapProjects   map[string]*models.MapProject     // datasetID -> project
	MapConfig     map[string]interface{}
}

type SessionRepository struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	storageDir string
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[string]*Session),
	}
}

func (r *SessionRepository) SetStorageDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storageDir = dir
	r.hydrateAllLocked()
}

func generateID(prefix string) string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	hexStr := hex.EncodeToString(bytes)
	if prefix != "" {
		return prefix + "_" + hexStr
	}
	return hexStr
}

func GenerateSessionID() string {
	return generateID("sess")
}

func GenerateDatasetID() string {
	return generateID("ds")
}

func GeneratePointID() string {
	return generateID("cp")
}

func GenerateLayerID() string {
	return generateID("lyr")
}

func (r *SessionRepository) CreateSession() *Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	id := GenerateSessionID()
	sess := &Session{
		ID:            id,
		CreatedAt:     now,
		LastActiveAt:  now,
		Datasets:      make(map[string]*models.Dataset),
		ControlPoints: make(map[string][]*models.ControlPoint),
		Layers:        make(map[string][]*models.Layer),
		MapProjects:   make(map[string]*models.MapProject),
		MapConfig:     make(map[string]interface{}),
	}
	r.sessions[id] = sess
	return sess
}

func (r *SessionRepository) GetSession(sessionID string) (*Session, error) {
	r.mu.RLock()
	sess, exists := r.sessions[sessionID]
	r.mu.RUnlock()

	if exists {
		return sess, nil
	}

	if r.storageDir != "" {
		r.mu.Lock()
		defer r.mu.Unlock()
		if sess, exists := r.sessions[sessionID]; exists {
			return sess, nil
		}
		if sess := r.hydrateSessionLocked(sessionID); sess != nil {
			return sess, nil
		}
	}

	return nil, ErrSessionNotFound
}

func (r *SessionRepository) TouchSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sess, exists := r.sessions[sessionID]; exists {
		sess.LastActiveAt = time.Now()
	}
}

func (r *SessionRepository) DeleteSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions, sessionID)
}

func (r *SessionRepository) CleanupExpiredSessions(idleTimeout, maxLifetime time.Duration, onCleanup func(sessionID string)) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var removed []string

	for id, sess := range r.sessions {
		if now.Sub(sess.LastActiveAt) > idleTimeout || now.Sub(sess.CreatedAt) > maxLifetime {
			delete(r.sessions, id)
			removed = append(removed, id)
			if onCleanup != nil {
				go onCleanup(id)
			}
		}
	}
	return removed
}

// Dataset Operations
func (r *SessionRepository) InsertDataset(sessionID string, dataset *models.Dataset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	dataset.SessionID = sessionID
	sess.Datasets[dataset.ID] = dataset
	sess.LastActiveAt = time.Now()

	// Initialize default layer
	var bounds []float64
	if b, ok := dataset.ExtractedMetadata["bounds"].(map[string]interface{}); ok {
		if minX, ok1 := b["min_x"].(float64); ok1 {
			if minY, ok2 := b["min_y"].(float64); ok2 {
				if maxX, ok3 := b["max_x"].(float64); ok3 {
					if maxY, ok4 := b["max_y"].(float64); ok4 {
						bounds = []float64{minX, minY, maxX, maxY}
					}
				}
			}
		}
	}
	sess.Layers[dataset.ID] = []*models.Layer{
		{
			ID:        GenerateLayerID(),
			DatasetID: dataset.ID,
			Name:      dataset.Filename,
			Type:      "raster",
			Visible:   true,
			Opacity:   1.0,
			Bounds:    bounds,
		},
	}
	r.saveDatasetLocked(sessionID, dataset)
	r.saveLayersLocked(sessionID, dataset.ID, sess.Layers[dataset.ID])
	return nil
}

func (r *SessionRepository) GetDataset(sessionID, datasetID string) (*models.Dataset, error) {
	sess, err := r.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ds, exists := sess.Datasets[datasetID]
	if !exists {
		return nil, ErrDatasetNotFound
	}
	return ds, nil
}

func (r *SessionRepository) FindDataset(datasetID string) (*models.Dataset, string, error) {
	r.mu.RLock()
	for sessID, sess := range r.sessions {
		if ds, exists := sess.Datasets[datasetID]; exists {
			r.mu.RUnlock()
			return ds, sessID, nil
		}
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double check under write lock
	for sessID, sess := range r.sessions {
		if ds, exists := sess.Datasets[datasetID]; exists {
			return ds, sessID, nil
		}
	}

	if r.storageDir != "" {
		entries, err := os.ReadDir(r.storageDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				sessID := entry.Name()
				dsDir := filepath.Join(r.storageDir, sessID, "datasets", datasetID)
				if fi, err := os.Stat(dsDir); err == nil && fi.IsDir() {
					sess := r.hydrateSessionLocked(sessID)
					if sess != nil {
						if ds, exists := sess.Datasets[datasetID]; exists {
							return ds, sessID, nil
						}
					}
				}
			}
		}
	}

	return nil, "", ErrDatasetNotFound
}

func (r *SessionRepository) ListDatasets(sessionID string) ([]*models.Dataset, error) {
	sess, err := r.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.Dataset
	for _, ds := range sess.Datasets {
		list = append(list, ds)
	}
	return list, nil
}

func (r *SessionRepository) UpdateDataset(sessionID string, dataset *models.Dataset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	if _, exists := sess.Datasets[dataset.ID]; !exists {
		return ErrDatasetNotFound
	}
	dataset.UpdatedAt = time.Now()
	sess.Datasets[dataset.ID] = dataset
	sess.LastActiveAt = time.Now()
	r.saveDatasetLocked(sessionID, dataset)
	return nil
}

func (r *SessionRepository) DeleteDataset(sessionID, datasetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	if _, exists := sess.Datasets[datasetID]; !exists {
		return ErrDatasetNotFound
	}
	delete(sess.Datasets, datasetID)
	delete(sess.ControlPoints, datasetID)
	delete(sess.Layers, datasetID)
	delete(sess.MapProjects, datasetID)
	sess.LastActiveAt = time.Now()
	return nil
}

// Control Points Operations
func (r *SessionRepository) ListControlPoints(sessionID, datasetID string) ([]*models.ControlPoint, error) {
	sess, err := r.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return sess.ControlPoints[datasetID], nil
}

func (r *SessionRepository) InsertControlPoint(sessionID, datasetID string, cp *models.ControlPoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	if _, exists := sess.Datasets[datasetID]; !exists {
		return ErrDatasetNotFound
	}
	if cp.ID == "" {
		cp.ID = GeneratePointID()
	}
	cp.DatasetID = datasetID
	cp.CreatedAt = time.Now()

	sess.ControlPoints[datasetID] = append(sess.ControlPoints[datasetID], cp)
	sess.LastActiveAt = time.Now()
	r.saveControlPointsLocked(sessionID, datasetID, sess.ControlPoints[datasetID])
	return nil
}

func (r *SessionRepository) UpdateControlPoint(sessionID, pointID string, updated *models.ControlPoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	for dsID, points := range sess.ControlPoints {
		for i, pt := range points {
			if pt.ID == pointID {
				updated.ID = pointID
				updated.DatasetID = dsID
				updated.CreatedAt = pt.CreatedAt
				sess.ControlPoints[dsID][i] = updated
				sess.LastActiveAt = time.Now()
				r.saveControlPointsLocked(sessionID, dsID, sess.ControlPoints[dsID])
				return nil
			}
		}
	}
	return ErrControlPointNotFound
}

func (r *SessionRepository) DeleteControlPoint(sessionID, pointID string) (*models.ControlPoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	for dsID, points := range sess.ControlPoints {
		for i, pt := range points {
			if pt.ID == pointID {
				removed := pt
				sess.ControlPoints[dsID] = append(points[:i], points[i+1:]...)
				sess.LastActiveAt = time.Now()
				r.saveControlPointsLocked(sessionID, dsID, sess.ControlPoints[dsID])
				return removed, nil
			}
		}
	}
	return nil, ErrControlPointNotFound
}

func (r *SessionRepository) SetControlPoints(sessionID, datasetID string, points []*models.ControlPoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	sess.ControlPoints[datasetID] = points
	sess.LastActiveAt = time.Now()
	r.saveControlPointsLocked(sessionID, datasetID, points)
	return nil
}

// Layer Operations
func (r *SessionRepository) ListLayers(sessionID, datasetID string) ([]*models.Layer, error) {
	sess, err := r.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	layers, ok := sess.Layers[datasetID]
	if !ok {
		return []*models.Layer{}, nil
	}
	return layers, nil
}

func (r *SessionRepository) UpdateLayer(sessionID, datasetID, layerID string, visible *bool, opacity *float64) (*models.Layer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	layers, exists := sess.Layers[datasetID]
	if !exists {
		return nil, ErrLayerNotFound
	}

	for i, lyr := range layers {
		if lyr.ID == layerID {
			if visible != nil {
				lyr.Visible = *visible
			}
			if opacity != nil {
				lyr.Opacity = *opacity
			}
			sess.Layers[datasetID][i] = lyr
			sess.LastActiveAt = time.Now()
			r.saveLayersLocked(sessionID, datasetID, sess.Layers[datasetID])
			return lyr, nil
		}
	}
	return nil, ErrLayerNotFound
}

func (r *SessionRepository) SetLayers(sessionID, datasetID string, layers []*models.Layer) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	sess.Layers[datasetID] = layers
	r.saveLayersLocked(sessionID, datasetID, layers)
	return nil
}

// MapProject Operations
func (r *SessionRepository) GetMapProject(sessionID, datasetID string) (*models.MapProject, error) {
	sess, err := r.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	project, ok := sess.MapProjects[datasetID]
	if !ok {
		return nil, nil
	}
	return project, nil
}

func (r *SessionRepository) SaveMapProject(sessionID string, project *models.MapProject) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	if project.ID == "" {
		project.ID = generateID("proj")
	}
	now := time.Now()
	if project.CreatedAt.IsZero() {
		project.CreatedAt = now
	}
	project.UpdatedAt = now
	sess.MapProjects[project.DatasetID] = project
	r.saveProjectLocked(sessionID, project)
	return nil
}

// Disk persistence and hydration helpers
func saveJSONFile(path string, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, data, 0644)
	}
}

func (r *SessionRepository) saveDatasetLocked(sessionID string, dataset *models.Dataset) {
	if r.storageDir == "" || dataset == nil {
		return
	}
	saveJSONFile(filepath.Join(r.storageDir, sessionID, "datasets", dataset.ID, "dataset.json"), dataset)
}

func (r *SessionRepository) saveControlPointsLocked(sessionID, datasetID string, cps []*models.ControlPoint) {
	if r.storageDir == "" {
		return
	}
	saveJSONFile(filepath.Join(r.storageDir, sessionID, "datasets", datasetID, "control_points.json"), cps)
}

func (r *SessionRepository) saveLayersLocked(sessionID, datasetID string, layers []*models.Layer) {
	if r.storageDir == "" {
		return
	}
	saveJSONFile(filepath.Join(r.storageDir, sessionID, "datasets", datasetID, "layers.json"), layers)
}

func (r *SessionRepository) saveProjectLocked(sessionID string, project *models.MapProject) {
	if r.storageDir == "" || project == nil {
		return
	}
	saveJSONFile(filepath.Join(r.storageDir, sessionID, "datasets", project.DatasetID, "project.json"), project)
}

func (r *SessionRepository) hydrateAllLocked() {
	if r.storageDir == "" {
		return
	}
	entries, err := os.ReadDir(r.storageDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "sess") {
			r.hydrateSessionLocked(entry.Name())
		}
	}
}

func (r *SessionRepository) hydrateSessionLocked(sessionID string) *Session {
	if r.storageDir == "" {
		return nil
	}
	sessionDir := filepath.Join(r.storageDir, sessionID)
	info, err := os.Stat(sessionDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	sess := &Session{
		ID:            sessionID,
		CreatedAt:     info.ModTime(),
		LastActiveAt:  time.Now(),
		Datasets:      make(map[string]*models.Dataset),
		ControlPoints: make(map[string][]*models.ControlPoint),
		Layers:        make(map[string][]*models.Layer),
		MapProjects:   make(map[string]*models.MapProject),
		MapConfig:     make(map[string]interface{}),
	}

	datasetsDir := filepath.Join(sessionDir, "datasets")
	entries, err := os.ReadDir(datasetsDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dsID := entry.Name()
			dsDir := filepath.Join(datasetsDir, dsID)

			var ds *models.Dataset
			metaPath := filepath.Join(dsDir, "dataset.json")
			if metaBytes, err := os.ReadFile(metaPath); err == nil {
				var loaded models.Dataset
				if json.Unmarshal(metaBytes, &loaded) == nil {
					ds = &loaded
				}
			}

			if ds == nil {
				ds = discoverDatasetFromDir(sessionID, dsID, dsDir)
			}

			if ds != nil {
				sess.Datasets[dsID] = ds
			}

			cpPath := filepath.Join(dsDir, "control_points.json")
			if cpBytes, err := os.ReadFile(cpPath); err == nil {
				var cps []*models.ControlPoint
				if json.Unmarshal(cpBytes, &cps) == nil {
					sess.ControlPoints[dsID] = cps
				}
			}

			layersPath := filepath.Join(dsDir, "layers.json")
			if lyrBytes, err := os.ReadFile(layersPath); err == nil {
				var lyrs []*models.Layer
				if json.Unmarshal(lyrBytes, &lyrs) == nil {
					sess.Layers[dsID] = lyrs
				}
			}

			projPath := filepath.Join(dsDir, "project.json")
			if projBytes, err := os.ReadFile(projPath); err == nil {
				var proj models.MapProject
				if json.Unmarshal(projBytes, &proj) == nil {
					sess.MapProjects[dsID] = &proj
				}
			}
		}
	}

	r.sessions[sessionID] = sess
	return sess
}

func discoverDatasetFromDir(sessionID, dsID, dsDir string) *models.Dataset {
	entries, err := os.ReadDir(dsDir)
	if err != nil {
		return nil
	}

	var imgPath, filename, ext string
	var imgSize int64
	var imgMod time.Time

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		candidateExt := strings.ToLower(filepath.Ext(e.Name()))
		if candidateExt == ".png" || candidateExt == ".jpg" || candidateExt == ".jpeg" {
			imgPath = filepath.Join(dsDir, e.Name())
			filename = e.Name()
			ext = candidateExt
			if fi, err := e.Info(); err == nil {
				imgSize = fi.Size()
				imgMod = fi.ModTime()
			}
			break
		}
	}

	if imgPath == "" {
		return nil
	}

	meta, err := gis.InspectRasterFile(imgPath)
	if err != nil {
		return nil
	}

	contentType := "image/png"
	if ext == ".jpg" || ext == ".jpeg" {
		contentType = "image/jpeg"
	}

	extractedMeta := map[string]interface{}{
		"driver":               meta.Driver,
		"width":                meta.Width,
		"height":               meta.Height,
		"bands":                meta.Bands,
		"dtype":                meta.DType,
		"crs":                  meta.CRS,
		"bounds":               meta.Bounds,
		"resolution":           meta.Resolution,
		"transform":            meta.Transform,
		"georeference_source":  meta.GeoreferenceSource,
		"color_interpretation": meta.ColorInterpretation,
		"format":               strings.ToLower(meta.Driver),
		"archive_filename":     filename,
	}

	georefStatus := "not_started"
	var georefPath string
	vrtPath := filepath.Join(dsDir, "processed", "georeferenced.vrt")
	if _, err := os.Stat(vrtPath); err == nil {
		georefStatus = "completed"
		georefPath = vrtPath
		if vrtMeta, err := gis.InspectRasterFile(vrtPath); err == nil && vrtMeta != nil {
			extractedMeta["bounds"] = vrtMeta.Bounds
			extractedMeta["transform"] = vrtMeta.Transform
			extractedMeta["crs"] = vrtMeta.CRS
		}
	} else if meta.IsGeoreferenced {
		georefStatus = "completed"
		georefPath = imgPath
	}

	ds := &models.Dataset{
		ID:                 dsID,
		SessionID:          sessionID,
		Filename:           filename,
		FileSize:           imgSize,
		ContentType:        contentType,
		StoragePath:        imgPath,
		GeoreferencedPath:  georefPath,
		ProcessedPath:      georefPath,
		GeoreferenceStatus: georefStatus,
		TileStatus:         "not_started",
		ExtractedMetadata:  extractedMeta,
		CreatedAt:          imgMod,
		UpdatedAt:          time.Now(),
	}

	saveJSONFile(filepath.Join(dsDir, "dataset.json"), ds)
	return ds
}
