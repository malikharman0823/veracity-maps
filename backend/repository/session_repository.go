package repository

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

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
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[string]*Session),
	}
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
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	return sess, nil
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
	return nil
}

func (r *SessionRepository) GetDataset(sessionID, datasetID string) (*models.Dataset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	ds, exists := sess.Datasets[datasetID]
	if !exists {
		return nil, ErrDatasetNotFound
	}
	return ds, nil
}

func (r *SessionRepository) ListDatasets(sessionID string) ([]*models.Dataset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
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
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
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
				return removed, nil
			}
		}
	}
	return nil, ErrControlPointNotFound
}

// Layer Operations
func (r *SessionRepository) ListLayers(sessionID, datasetID string) ([]*models.Layer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
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
	return nil
}

// MapProject Operations
func (r *SessionRepository) GetMapProject(sessionID, datasetID string) (*models.MapProject, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, exists := r.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
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
	return nil
}
