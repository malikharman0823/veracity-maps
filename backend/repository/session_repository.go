package repository

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	ErrIconPointNotFound    = errors.New("ICON_POINT_NOT_FOUND")
	ErrBuildingNotFound     = errors.New("BUILDING_NOT_FOUND")
	ErrFloorNotFound        = errors.New("FLOOR_NOT_FOUND")
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
	SavedConfigs  map[string]*models.SavedConfig // configID -> saved config
	IconPoints    map[string]*models.IconPoint   // pointID -> icon point
	Buildings     map[string]*models.Building    // buildingID -> building
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

func GenerateIconPointID() string {
	return generateID("ip")
}

func GenerateBuildingID() string {
	return generateID("bld")
}

func GenerateFloorID() string {
	return generateID("flr")
}

func isDeterministicSessionID(sessionID string) bool {
	return strings.HasPrefix(sessionID, "map_") || strings.HasPrefix(sessionID, "proj_")
}

func newEmptySession(id string) *Session {
	now := time.Now()
	return &Session{
		ID:            id,
		CreatedAt:     now,
		LastActiveAt:  now,
		Datasets:      make(map[string]*models.Dataset),
		ControlPoints: make(map[string][]*models.ControlPoint),
		Layers:        make(map[string][]*models.Layer),
		MapProjects:   make(map[string]*models.MapProject),
		MapConfig:     make(map[string]interface{}),
		SavedConfigs:  make(map[string]*models.SavedConfig),
		IconPoints:    make(map[string]*models.IconPoint),
		Buildings:     make(map[string]*models.Building),
	}
}

func (r *SessionRepository) CreateSession() *Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := GenerateSessionID()
	sess := newEmptySession(id)
	r.sessions[id] = sess
	r.touchSessionLocked(sess)
	return sess
}

func (r *SessionRepository) GetSession(sessionID string) (*Session, error) {
	r.mu.RLock()
	sess, exists := r.sessions[sessionID]
	r.mu.RUnlock()

	if exists {
		return sess, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if sess, exists := r.sessions[sessionID]; exists {
		return sess, nil
	}
	if r.storageDir != "" {
		if sess := r.hydrateSessionLocked(sessionID); sess != nil {
			return sess, nil
		}
	}
	if isDeterministicSessionID(sessionID) {
		sess := newEmptySession(sessionID)
		r.sessions[sessionID] = sess
		r.touchSessionLocked(sess)
		return sess, nil
	}

	return nil, ErrSessionNotFound
}

func (r *SessionRepository) TouchSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sess, exists := r.sessions[sessionID]; exists {
		sess.LastActiveAt = time.Now()
		return
	}
	if r.storageDir != "" {
		if sess := r.hydrateSessionLocked(sessionID); sess != nil {
			sess.LastActiveAt = time.Now()
			return
		}
	}
	if isDeterministicSessionID(sessionID) {
		sess := newEmptySession(sessionID)
		r.sessions[sessionID] = sess
		r.touchSessionLocked(sess)
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
	if err == nil && sess != nil {
		r.mu.RLock()
		ds, exists := sess.Datasets[datasetID]
		r.mu.RUnlock()
		if exists && ds != nil {
			return ds, nil
		}
		return nil, ErrDatasetNotFound
	}

	if sessionID == "" {
		ds, _, findErr := r.FindDataset(datasetID)
		if findErr == nil && ds != nil {
			return ds, nil
		}
	}

	return nil, ErrDatasetNotFound
}

func (r *SessionRepository) FindDataset(datasetID string) (*models.Dataset, string, error) {
	r.mu.RLock()
	for sessID, sess := range r.sessions {
		if ds, exists := sess.Datasets[datasetID]; exists {
			r.mu.RUnlock()
			return ds, sessID, nil
		}
		if proj, pExists := sess.MapProjects[datasetID]; pExists && proj != nil {
			if ds, dExists := sess.Datasets[proj.DatasetID]; dExists {
				r.mu.RUnlock()
				return ds, sessID, nil
			}
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
		if proj, pExists := sess.MapProjects[datasetID]; pExists && proj != nil {
			if ds, dExists := sess.Datasets[proj.DatasetID]; dExists {
				return ds, sessID, nil
			}
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

		// Also check durable map asset storage (<temp>/uploads/maps/<ownerUserID>/<mapID>/assets/<datasetID>)
		mapsRoot := filepath.Join(filepath.Dir(r.storageDir), "uploads", "maps")
		if ownerEntries, err := os.ReadDir(mapsRoot); err == nil {
			for _, ownerEntry := range ownerEntries {
				if !ownerEntry.IsDir() {
					continue
				}
				ownerMapsDir := filepath.Join(mapsRoot, ownerEntry.Name())
				mapEntries, err := os.ReadDir(ownerMapsDir)
				if err != nil {
					continue
				}
				for _, mapEntry := range mapEntries {
					if !mapEntry.IsDir() {
						continue
					}
					mapID := mapEntry.Name()
					assetDir := filepath.Join(ownerMapsDir, mapID, "assets", datasetID)
					if fi, err := os.Stat(assetDir); err == nil && fi.IsDir() {
						sessID := mapID
						if !strings.HasPrefix(sessID, "map_") {
							sessID = "map_" + sessID
						}
						if ds := r.hydrateDurableAssetLocked(sessID, datasetID, assetDir); ds != nil {
							return ds, sessID, nil
						}
					}
				}
			}
		}
	}

	return nil, "", ErrDatasetNotFound
}

func (r *SessionRepository) hydrateDurableAssetLocked(sessionID, dsID, assetDir string) *models.Dataset {
	sess, exists := r.sessions[sessionID]
	if !exists || sess == nil {
		sess = newEmptySession(sessionID)
		r.sessions[sessionID] = sess
	}

	var ds *models.Dataset
	metaPath := filepath.Join(assetDir, "dataset.json")
	if metaBytes, err := os.ReadFile(metaPath); err == nil {
		var loaded models.Dataset
		if json.Unmarshal(metaBytes, &loaded) == nil {
			ds = &loaded
		}
	}
	if ds == nil {
		ds = discoverDatasetFromDir(sessionID, dsID, assetDir)
	}
	if ds == nil {
		return nil
	}

	absDir, err := filepath.Abs(assetDir)
	if err != nil {
		absDir = assetDir
	}
	if ds.StoragePath == "" || !fileExistsOnDisk(ds.StoragePath) {
		if ds.Filename != "" {
			candidate := filepath.Join(absDir, filepath.Base(ds.Filename))
			if fileExistsOnDisk(candidate) {
				ds.StoragePath = candidate
			}
		}
		if ds.StoragePath == "" || !fileExistsOnDisk(ds.StoragePath) {
			if entries, err := os.ReadDir(absDir); err == nil {
				for _, e := range entries {
					if !e.IsDir() {
						ext := strings.ToLower(filepath.Ext(e.Name()))
						if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".tif" || ext == ".tiff" || ext == ".webp" {
							ds.StoragePath = filepath.Join(absDir, e.Name())
							break
						}
					}
				}
			}
		}
	}
	if ds.GeoreferencedPath != "" && !fileExistsOnDisk(ds.GeoreferencedPath) {
		vrtCand := filepath.Join(absDir, "processed", filepath.Base(ds.GeoreferencedPath))
		if fileExistsOnDisk(vrtCand) {
			ds.GeoreferencedPath = vrtCand
		} else if rootCand := filepath.Join(absDir, filepath.Base(ds.GeoreferencedPath)); fileExistsOnDisk(rootCand) {
			ds.GeoreferencedPath = rootCand
		}
	}
	if ds.ProcessedPath != "" && !fileExistsOnDisk(ds.ProcessedPath) {
		procCand := filepath.Join(absDir, "processed", filepath.Base(ds.ProcessedPath))
		if fileExistsOnDisk(procCand) {
			ds.ProcessedPath = procCand
		} else if rootCand := filepath.Join(absDir, filepath.Base(ds.ProcessedPath)); fileExistsOnDisk(rootCand) {
			ds.ProcessedPath = rootCand
		}
	}
	ds.SessionID = sessionID
	sess.Datasets[dsID] = ds

	cpPath := filepath.Join(assetDir, "control_points.json")
	if cpBytes, err := os.ReadFile(cpPath); err == nil {
		var cps []*models.ControlPoint
		if json.Unmarshal(cpBytes, &cps) == nil {
			sess.ControlPoints[dsID] = cps
		}
	}

	layersPath := filepath.Join(assetDir, "layers.json")
	if lyrBytes, err := os.ReadFile(layersPath); err == nil {
		var lyrs []*models.Layer
		if json.Unmarshal(lyrBytes, &lyrs) == nil {
			sess.Layers[dsID] = lyrs
		}
	}

	projPath := filepath.Join(assetDir, "project.json")
	if projBytes, err := os.ReadFile(projPath); err == nil {
		var proj models.MapProject
		if json.Unmarshal(projBytes, &proj) == nil {
			sess.MapProjects[dsID] = &proj
		}
	}

	return ds
}

func fileExistsOnDisk(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
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

// SessionSummary represents high-level metrics of an existing workspace session.
type SessionSummary struct {
	ID             string     `json:"id"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	LastActiveAt   *time.Time `json:"last_active_at,omitempty"`
	DatasetCount   int        `json:"dataset_count"`
	TotalMemory    int64      `json:"total_memory"`
	MemorySize     string     `json:"memory_size"`
	Datasets       []string   `json:"datasets"`
	DatasetIDs     []string   `json:"dataset_ids,omitempty"`
	FirstDatasetID string     `json:"first_dataset_id,omitempty"`
	BuildingCount  int        `json:"building_count"`
	ConfigCount    int        `json:"config_count"`
}

func formatMemoryBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ListSessions returns summaries of all active and persisted workspace sessions.
func (r *SessionRepository) ListSessions() []*SessionSummary {
	r.mu.Lock()
	r.hydrateAllLocked()
	r.mu.Unlock()

	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]*SessionSummary, 0, len(r.sessions))
	for id, sess := range r.sessions {
		if sess == nil {
			continue
		}
		var totalMem int64
		var dsNames []string
		var dsIDs []string
		for dsID, ds := range sess.Datasets {
			if ds != nil {
				totalMem += ds.FileSize
				dsNames = append(dsNames, ds.Filename)
				dsIDs = append(dsIDs, dsID)
			}
		}
		sort.Strings(dsNames)
		sort.Strings(dsIDs)
		var firstDSID string
		if len(dsIDs) > 0 {
			firstDSID = dsIDs[0]
		}

		var pCreated, pActive *time.Time
		if !sess.CreatedAt.IsZero() {
			c := sess.CreatedAt
			pCreated = &c
		}
		if !sess.LastActiveAt.IsZero() {
			a := sess.LastActiveAt
			pActive = &a
		}

		summaries = append(summaries, &SessionSummary{
			ID:             id,
			CreatedAt:      pCreated,
			LastActiveAt:   pActive,
			DatasetCount:   len(sess.Datasets),
			TotalMemory:    totalMem,
			MemorySize:     formatMemoryBytes(totalMem),
			Datasets:       dsNames,
			DatasetIDs:     dsIDs,
			FirstDatasetID: firstDSID,
			BuildingCount:  len(sess.Buildings),
			ConfigCount:    len(sess.SavedConfigs),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		var timeI, timeJ time.Time
		if summaries[i].LastActiveAt != nil {
			timeI = *summaries[i].LastActiveAt
		} else if summaries[i].CreatedAt != nil {
			timeI = *summaries[i].CreatedAt
		}
		if summaries[j].LastActiveAt != nil {
			timeJ = *summaries[j].LastActiveAt
		} else if summaries[j].CreatedAt != nil {
			timeJ = *summaries[j].CreatedAt
		}
		return timeI.After(timeJ)
	})
	return summaries
}

// ListAllDatasets returns all unique datasets across all active or persisted sessions.
func (r *SessionRepository) ListAllDatasets() []*models.Dataset {
	r.mu.Lock()
	r.hydrateAllLocked()
	r.mu.Unlock()

	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.Dataset
	seen := make(map[string]bool)
	for _, sess := range r.sessions {
		for id, ds := range sess.Datasets {
			if !seen[id] && ds != nil {
				seen[id] = true
				list = append(list, ds)
			}
		}
	}
	return list
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

func (r *SessionRepository) FindControlPoint(pointID string) (*models.ControlPoint, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for sID, sess := range r.sessions {
		for _, points := range sess.ControlPoints {
			for _, pt := range points {
				if pt.ID == pointID {
					return pt, sID, nil
				}
			}
		}
	}
	return nil, "", ErrControlPointNotFound
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
	if err == nil && sess != nil {
		r.mu.RLock()
		project, ok := sess.MapProjects[datasetID]
		r.mu.RUnlock()
		if ok && project != nil {
			return project, nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.sessions {
		if p, ok := s.MapProjects[datasetID]; ok && p != nil {
			if sess != nil {
				sess.MapProjects[datasetID] = p
			}
			return p, nil
		}
	}

	return nil, nil
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

// SavedConfig Operations
func (r *SessionRepository) SaveConfig(sessionID string, cfg *models.SavedConfig) (*models.SavedConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	if cfg.ID == "" {
		cfg.ID = generateID("cfg")
	}
	now := time.Now()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	if sess.SavedConfigs == nil {
		sess.SavedConfigs = make(map[string]*models.SavedConfig)
	}
	sess.SavedConfigs[cfg.ID] = cfg
	sess.LastActiveAt = now
	r.saveSavedConfigsLocked(sessionID)
	return cfg, nil
}

func (r *SessionRepository) GetSavedConfig(sessionID, configID string) (*models.SavedConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	cfg, ok := sess.SavedConfigs[configID]
	if !ok {
		return nil, errors.New("CONFIG_NOT_FOUND")
	}
	return cfg, nil
}

func (r *SessionRepository) ListSavedConfigs(sessionID string) ([]*models.SavedConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	res := make([]*models.SavedConfig, 0, len(sess.SavedConfigs))
	for _, c := range sess.SavedConfigs {
		res = append(res, c)
	}
	return res, nil
}

func (r *SessionRepository) DeleteSavedConfig(sessionID, configID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	if _, ok := sess.SavedConfigs[configID]; !ok {
		return errors.New("CONFIG_NOT_FOUND")
	}
	delete(sess.SavedConfigs, configID)
	sess.LastActiveAt = time.Now()
	r.saveSavedConfigsLocked(sessionID)
	return nil
}

// IconPoint Operations
func (r *SessionRepository) CreateIconPoint(sessionID string, pt *models.IconPoint) (*models.IconPoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	if pt.ID == "" {
		pt.ID = GenerateIconPointID()
	}
	pt.SessionID = sessionID
	now := time.Now()
	if pt.CreatedAt.IsZero() {
		pt.CreatedAt = now
	}
	pt.UpdatedAt = now

	if sess.IconPoints == nil {
		sess.IconPoints = make(map[string]*models.IconPoint)
	}
	sess.IconPoints[pt.ID] = pt
	sess.LastActiveAt = now
	r.saveIconPointsLocked(sessionID)
	return pt, nil
}

func (r *SessionRepository) GetIconPoint(sessionID, pointID string) (*models.IconPoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	pt, ok := sess.IconPoints[pointID]
	if !ok {
		return nil, ErrIconPointNotFound
	}
	return pt, nil
}

func (r *SessionRepository) FindIconPoint(pointID string) (*models.IconPoint, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for sID, sess := range r.sessions {
		if pt, ok := sess.IconPoints[pointID]; ok {
			return pt, sID, nil
		}
	}
	return nil, "", ErrIconPointNotFound
}

func (r *SessionRepository) ListIconPoints(sessionID, datasetID, buildingID string) ([]*models.IconPoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	res := make([]*models.IconPoint, 0, len(sess.IconPoints))
	for _, pt := range sess.IconPoints {
		if datasetID != "" && pt.DatasetID != "" && pt.DatasetID != datasetID {
			continue
		}
		if buildingID != "" && pt.BuildingID != "" && pt.BuildingID != buildingID {
			continue
		}
		res = append(res, pt)
	}
	return res, nil
}

func (r *SessionRepository) UpdateIconPoint(sessionID, pointID string, updated *models.IconPoint) (*models.IconPoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	pt, ok := sess.IconPoints[pointID]
	if !ok {
		return nil, ErrIconPointNotFound
	}

	updated.ID = pointID
	updated.SessionID = sessionID
	updated.CreatedAt = pt.CreatedAt
	updated.UpdatedAt = time.Now()
	sess.IconPoints[pointID] = updated
	sess.LastActiveAt = time.Now()
	r.saveIconPointsLocked(sessionID)
	return updated, nil
}

func (r *SessionRepository) DeleteIconPoint(sessionID, pointID string) (*models.IconPoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	pt, ok := sess.IconPoints[pointID]
	if !ok {
		return nil, ErrIconPointNotFound
	}
	delete(sess.IconPoints, pointID)
	sess.LastActiveAt = time.Now()
	r.saveIconPointsLocked(sessionID)
	return pt, nil
}

// Building Operations
func (r *SessionRepository) CreateBuilding(sessionID string, b *models.Building) (*models.Building, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	if b.ID == "" {
		b.ID = GenerateBuildingID()
	}
	b.SessionID = sessionID
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	if sess.Buildings == nil {
		sess.Buildings = make(map[string]*models.Building)
	}
	sess.Buildings[b.ID] = b
	sess.LastActiveAt = now
	r.saveBuildingsLocked(sessionID)
	return b, nil
}

func (r *SessionRepository) GetBuilding(sessionID, buildingID string) (*models.Building, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	b, ok := sess.Buildings[buildingID]
	if !ok {
		return nil, ErrBuildingNotFound
	}
	return b, nil
}

func (r *SessionRepository) ListBuildings(sessionID string) ([]*models.Building, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	res := make([]*models.Building, 0, len(sess.Buildings))
	for _, b := range sess.Buildings {
		res = append(res, b)
	}
	return res, nil
}

func (r *SessionRepository) UpdateBuilding(sessionID, buildingID string, updated *models.Building) (*models.Building, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	b, ok := sess.Buildings[buildingID]
	if !ok {
		return nil, ErrBuildingNotFound
	}

	updated.ID = buildingID
	updated.SessionID = sessionID
	updated.CreatedAt = b.CreatedAt
	updated.UpdatedAt = time.Now()
	sess.Buildings[buildingID] = updated
	sess.LastActiveAt = time.Now()
	r.saveBuildingsLocked(sessionID)
	return updated, nil
}

func (r *SessionRepository) DeleteBuilding(sessionID, buildingID string) (*models.Building, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	b, ok := sess.Buildings[buildingID]
	if !ok {
		return nil, ErrBuildingNotFound
	}
	delete(sess.Buildings, buildingID)
	sess.LastActiveAt = time.Now()
	r.saveBuildingsLocked(sessionID)
	return b, nil
}

func (r *SessionRepository) AddFloor(sessionID, buildingID string, floor *models.BuildingFloor) (*models.BuildingFloor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	b, ok := sess.Buildings[buildingID]
	if !ok {
		return nil, ErrBuildingNotFound
	}

	if floor.ID == "" {
		floor.ID = GenerateFloorID()
	}
	floor.BuildingID = buildingID
	b.Floors = append(b.Floors, floor)
	b.UpdatedAt = time.Now()
	sess.LastActiveAt = time.Now()
	r.saveBuildingsLocked(sessionID)
	return floor, nil
}

func (r *SessionRepository) UpdateFloor(sessionID, buildingID, floorID string, updated *models.BuildingFloor) (*models.BuildingFloor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	b, ok := sess.Buildings[buildingID]
	if !ok {
		return nil, ErrBuildingNotFound
	}

	for i, f := range b.Floors {
		if f.ID == floorID {
			updated.ID = floorID
			updated.BuildingID = buildingID
			b.Floors[i] = updated
			b.UpdatedAt = time.Now()
			sess.LastActiveAt = time.Now()
			r.saveBuildingsLocked(sessionID)
			return updated, nil
		}
	}
	return nil, ErrFloorNotFound
}

func (r *SessionRepository) DeleteFloor(sessionID, buildingID, floorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}
	b, ok := sess.Buildings[buildingID]
	if !ok {
		return ErrBuildingNotFound
	}

	for i, f := range b.Floors {
		if f.ID == floorID {
			b.Floors = append(b.Floors[:i], b.Floors[i+1:]...)
			b.UpdatedAt = time.Now()
			sess.LastActiveAt = time.Now()
			r.saveBuildingsLocked(sessionID)
			return nil
		}
	}
	return ErrFloorNotFound
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

func (r *SessionRepository) saveIconPointsLocked(sessionID string) {
	if r.storageDir == "" {
		return
	}
	sess := r.sessions[sessionID]
	if sess == nil {
		return
	}
	list := make([]*models.IconPoint, 0, len(sess.IconPoints))
	for _, pt := range sess.IconPoints {
		list = append(list, pt)
	}
	saveJSONFile(filepath.Join(r.storageDir, sessionID, "icon_points.json"), list)
}

func (r *SessionRepository) saveBuildingsLocked(sessionID string) {
	if r.storageDir == "" {
		return
	}
	sess := r.sessions[sessionID]
	if sess == nil {
		return
	}
	list := make([]*models.Building, 0, len(sess.Buildings))
	for _, b := range sess.Buildings {
		list = append(list, b)
	}
	saveJSONFile(filepath.Join(r.storageDir, sessionID, "buildings.json"), list)
}

func (r *SessionRepository) saveSavedConfigsLocked(sessionID string) {
	if r.storageDir == "" {
		return
	}
	sess := r.sessions[sessionID]
	if sess == nil {
		return
	}
	list := make([]*models.SavedConfig, 0, len(sess.SavedConfigs))
	for _, c := range sess.SavedConfigs {
		list = append(list, c)
	}
	saveJSONFile(filepath.Join(r.storageDir, sessionID, "saved_configs.json"), list)
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
			if _, exists := r.sessions[entry.Name()]; exists {
				continue
			}
			r.hydrateSessionLocked(entry.Name())
		}
	}
}

// SessionMeta stores persistent timestamps and high-level identity for a session directory.
type SessionMeta struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

func (r *SessionRepository) touchSessionLocked(sess *Session) {
	if sess == nil {
		return
	}
	sess.LastActiveAt = time.Now()
	if r.storageDir != "" && sess.ID != "" {
		meta := SessionMeta{
			ID:           sess.ID,
			CreatedAt:    sess.CreatedAt,
			LastActiveAt: sess.LastActiveAt,
		}
		if metaBytes, err := json.Marshal(meta); err == nil {
			_ = os.WriteFile(filepath.Join(r.storageDir, sess.ID, "session_meta.json"), metaBytes, 0644)
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

	createdAt := info.ModTime()
	lastActiveAt := info.ModTime()

	metaPath := filepath.Join(sessionDir, "session_meta.json")
	if metaBytes, err := os.ReadFile(metaPath); err == nil {
		var meta SessionMeta
		if json.Unmarshal(metaBytes, &meta) == nil {
			if !meta.CreatedAt.IsZero() {
				createdAt = meta.CreatedAt
			}
			if !meta.LastActiveAt.IsZero() {
				lastActiveAt = meta.LastActiveAt
			}
		}
	}

	sess := &Session{
		ID:            sessionID,
		CreatedAt:     createdAt,
		LastActiveAt:  lastActiveAt,
		Datasets:      make(map[string]*models.Dataset),
		ControlPoints: make(map[string][]*models.ControlPoint),
		Layers:        make(map[string][]*models.Layer),
		MapProjects:   make(map[string]*models.MapProject),
		MapConfig:     make(map[string]interface{}),
		SavedConfigs:  make(map[string]*models.SavedConfig),
		IconPoints:    make(map[string]*models.IconPoint),
		Buildings:     make(map[string]*models.Building),
	}

	cfgPath := filepath.Join(sessionDir, "saved_configs.json")
	if cfgBytes, err := os.ReadFile(cfgPath); err == nil {
		if cfgInfo, sErr := os.Stat(cfgPath); sErr == nil && cfgInfo.ModTime().After(lastActiveAt) {
			lastActiveAt = cfgInfo.ModTime()
		}
		var cfgs []*models.SavedConfig
		if json.Unmarshal(cfgBytes, &cfgs) == nil {
			for _, c := range cfgs {
				sess.SavedConfigs[c.ID] = c
			}
		}
	}

	ipPath := filepath.Join(sessionDir, "icon_points.json")
	if ipBytes, err := os.ReadFile(ipPath); err == nil {
		if ipInfo, sErr := os.Stat(ipPath); sErr == nil && ipInfo.ModTime().After(lastActiveAt) {
			lastActiveAt = ipInfo.ModTime()
		}
		var ips []*models.IconPoint
		if json.Unmarshal(ipBytes, &ips) == nil {
			for _, ip := range ips {
				sess.IconPoints[ip.ID] = ip
			}
		}
	}

	bldPath := filepath.Join(sessionDir, "buildings.json")
	if bldBytes, err := os.ReadFile(bldPath); err == nil {
		if bldInfo, sErr := os.Stat(bldPath); sErr == nil && bldInfo.ModTime().After(lastActiveAt) {
			lastActiveAt = bldInfo.ModTime()
		}
		var blds []*models.Building
		if json.Unmarshal(bldBytes, &blds) == nil {
			for _, b := range blds {
				sess.Buildings[b.ID] = b
			}
		}
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
				if dInfo, sErr := os.Stat(metaPath); sErr == nil && dInfo.ModTime().After(lastActiveAt) {
					lastActiveAt = dInfo.ModTime()
				}
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
				if !ds.CreatedAt.IsZero() && (createdAt.IsZero() || ds.CreatedAt.Before(createdAt)) {
					createdAt = ds.CreatedAt
				}
				if !ds.UpdatedAt.IsZero() && ds.UpdatedAt.After(lastActiveAt) {
					lastActiveAt = ds.UpdatedAt
				}
			}

			cpPath := filepath.Join(dsDir, "control_points.json")
			if cpBytes, err := os.ReadFile(cpPath); err == nil {
				if cpInfo, sErr := os.Stat(cpPath); sErr == nil && cpInfo.ModTime().After(lastActiveAt) {
					lastActiveAt = cpInfo.ModTime()
				}
				var cps []*models.ControlPoint
				if json.Unmarshal(cpBytes, &cps) == nil {
					sess.ControlPoints[dsID] = cps
				}
			}

			layersPath := filepath.Join(dsDir, "layers.json")
			if lyrBytes, err := os.ReadFile(layersPath); err == nil {
				if lyrInfo, sErr := os.Stat(layersPath); sErr == nil && lyrInfo.ModTime().After(lastActiveAt) {
					lastActiveAt = lyrInfo.ModTime()
				}
				var lyrs []*models.Layer
				if json.Unmarshal(lyrBytes, &lyrs) == nil {
					sess.Layers[dsID] = lyrs
				}
			}

			projPath := filepath.Join(dsDir, "project.json")
			if projBytes, err := os.ReadFile(projPath); err == nil {
				if prjInfo, sErr := os.Stat(projPath); sErr == nil && prjInfo.ModTime().After(lastActiveAt) {
					lastActiveAt = prjInfo.ModTime()
				}
				var proj models.MapProject
				if json.Unmarshal(projBytes, &proj) == nil {
					sess.MapProjects[dsID] = &proj
					if !proj.CreatedAt.IsZero() && (createdAt.IsZero() || proj.CreatedAt.Before(createdAt)) {
						createdAt = proj.CreatedAt
					}
					if !proj.UpdatedAt.IsZero() && proj.UpdatedAt.After(lastActiveAt) {
						lastActiveAt = proj.UpdatedAt
					}
				}
			}
		}
	}

	sess.CreatedAt = createdAt
	sess.LastActiveAt = lastActiveAt

	// Persist session_meta.json for instant subsequent reads
	if metaBytes, err := json.Marshal(SessionMeta{
		ID:           sessionID,
		CreatedAt:    sess.CreatedAt,
		LastActiveAt: sess.LastActiveAt,
	}); err == nil {
		_ = os.WriteFile(metaPath, metaBytes, 0644)
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
