package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/storage"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrUnauthorized    = errors.New("unauthorized access to project")
)

type ProjectRepository struct {
	mu          sync.RWMutex
	projectsDir string
	projects    map[string]*models.UserProject // Key: project ID
}

func NewProjectRepository(storageDir string) (*ProjectRepository, error) {
	projDir := filepath.Join(storageDir, "projects")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create projects storage directory: %w", err)
	}

	repo := &ProjectRepository{
		projectsDir: projDir,
		projects:    make(map[string]*models.UserProject),
	}

	if err := repo.load(); err != nil {
		return nil, fmt.Errorf("failed to load projects: %w", err)
	}

	return repo, nil
}

func (r *ProjectRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.projectsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projFile := filepath.Join(r.projectsDir, entry.Name(), "project.json")
		if _, err := os.Stat(projFile); err == nil {
			data, err := os.ReadFile(projFile)
			if err == nil {
				var p models.UserProject
				if err := json.Unmarshal(data, &p); err == nil && p.ID != "" {
					r.projects[p.ID] = &p
				}
			}
		}
	}
	return nil
}

func (r *ProjectRepository) saveProjectLocked(p *models.UserProject) error {
	dir := filepath.Join(r.projectsDir, p.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, "project.json")
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

// Create creates and persists a new project owned by userID.
func (r *ProjectRepository) Create(userID, name, description string) (*models.UserProject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pidHex, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	projectID := "proj_" + pidHex
	now := time.Now()

	p := &models.UserProject{
		ID:          projectID,
		UserID:      userID,
		Name:        name,
		Description: description,
		DatasetIDs:  make([]string, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := r.saveProjectLocked(p); err != nil {
		return nil, fmt.Errorf("failed to save project: %w", err)
	}

	r.projects[projectID] = p
	return p, nil
}

// ListByUser returns all projects owned by userID, sorted by UpdatedAt desc.
func (r *ProjectRepository) ListByUser(userID string) []*models.UserProject {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.UserProject
	for _, p := range r.projects {
		if p.UserID == userID {
			result = append(result, p)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	return result
}

// GetByID returns the project if it exists and belongs to userID (or returns ErrUnauthorized).
func (r *ProjectRepository) GetByID(projectID, userID string) (*models.UserProject, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, ErrProjectNotFound
	}
	if p.UserID != userID {
		return nil, ErrUnauthorized
	}
	return p, nil
}

// Update updates name and description of a project owned by userID.
func (r *ProjectRepository) Update(projectID, userID, name, description string) (*models.UserProject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, ErrProjectNotFound
	}
	if p.UserID != userID {
		return nil, ErrUnauthorized
	}

	if name != "" {
		p.Name = name
	}
	p.Description = description
	p.UpdatedAt = time.Now()

	if err := r.saveProjectLocked(p); err != nil {
		return nil, err
	}
	return p, nil
}

// AddDataset associates a dataset ID with a project.
func (r *ProjectRepository) AddDataset(projectID, userID, datasetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return ErrProjectNotFound
	}
	if p.UserID != userID {
		return ErrUnauthorized
	}

	for _, id := range p.DatasetIDs {
		if id == datasetID {
			return nil
		}
	}

	p.DatasetIDs = append(p.DatasetIDs, datasetID)
	if p.PrimaryDatasetID == "" {
		p.PrimaryDatasetID = datasetID
	}
	p.UpdatedAt = time.Now()

	return r.saveProjectLocked(p)
}

// Delete removes project metadata and safely cleans up its disk directory.
func (r *ProjectRepository) Delete(projectID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return ErrProjectNotFound
	}
	if p.UserID != userID {
		return ErrUnauthorized
	}

	delete(r.projects, projectID)

	dir := filepath.Join(r.projectsDir, projectID)
	_ = storage.SafeRemoveAll(dir)

	return nil
}

// GetProjectDir returns absolute storage directory for a project.
func (r *ProjectRepository) GetProjectDir(projectID string) string {
	return filepath.Join(r.projectsDir, projectID)
}
