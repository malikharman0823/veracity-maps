package services

import (
	"context"
	"math"
	"sync"
	"time"
)

// ExportProgress defines live execution metrics for ongoing Custom Local Tiles export jobs.
type ExportProgress struct {
	DatasetID                 string    `json:"dataset_id"`
	SessionID                 string    `json:"session_id"`
	Stage                     string    `json:"stage"`
	StageIndex                int       `json:"stage_index"`
	TotalStages               int       `json:"total_stages"`
	CurrentLayer              string    `json:"current_layer"`
	CompletedTiles            int       `json:"completed_tiles"`
	ExpectedTiles             int       `json:"expected_tiles"`
	EmptyTilesSkipped         int       `json:"empty_tiles_skipped"`
	CachedTilesReused         int       `json:"cached_tiles_reused"`
	Percent                   float64   `json:"percent"`
	DurationSeconds           float64   `json:"duration_seconds"`
	EstimatedRemainingSeconds float64   `json:"estimated_remaining_seconds"`
	FinalPackageSizeBytes     int64     `json:"final_package_size_bytes"`
	Status                    string    `json:"status"` // "processing" | "completed" | "error" | "cancelled"
	Error                     string    `json:"error,omitempty"`
	StartTime                 time.Time `json:"start_time"`
	UpdatedAt                 time.Time `json:"updated_at"`

	cancelFunc context.CancelFunc `json:"-"`
}

// ExportProgressManager coordinates thread-safe live progress updates across active exports.
type ExportProgressManager struct {
	mu   sync.RWMutex
	jobs map[string]*ExportProgress
}

// NewExportProgressManager initializes an in-memory export progress tracker.
func NewExportProgressManager() *ExportProgressManager {
	return &ExportProgressManager{
		jobs: make(map[string]*ExportProgress),
	}
}

// StartExport initializes progress tracking for a new export operation.
func (m *ExportProgressManager) StartExport(datasetID, sessionID string, cancel context.CancelFunc) *ExportProgress {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	prog := &ExportProgress{
		DatasetID:                 datasetID,
		SessionID:                 sessionID,
		Stage:                     "Inspecting selected layers",
		StageIndex:                1,
		TotalStages:               8,
		CurrentLayer:              "",
		CompletedTiles:            0,
		ExpectedTiles:             0,
		EmptyTilesSkipped:         0,
		CachedTilesReused:         0,
		Percent:                   5.0,
		DurationSeconds:           0.0,
		EstimatedRemainingSeconds: 0.0,
		FinalPackageSizeBytes:     0,
		Status:                    "processing",
		StartTime:                 now,
		UpdatedAt:                 now,
		cancelFunc:                cancel,
	}
	m.jobs[datasetID] = prog
	return prog
}

// UpdateStage updates the current stage name and index, recalculating baseline percent.
func (m *ExportProgressManager) UpdateStage(datasetID string, stageIndex int, stageName, currentLayer string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prog := m.jobs[datasetID]
	if prog == nil || prog.Status != "processing" {
		return
	}

	prog.StageIndex = stageIndex
	prog.Stage = stageName
	if currentLayer != "" {
		prog.CurrentLayer = currentLayer
	}
	prog.UpdatedAt = time.Now()
	prog.DurationSeconds = math.Round(prog.UpdatedAt.Sub(prog.StartTime).Seconds()*10) / 10

	// Compute base percentage by stage
	basePercent := float64(stageIndex-1) / float64(prog.TotalStages) * 100.0
	if basePercent < 5.0 {
		basePercent = 5.0
	}
	if prog.Percent < basePercent {
		prog.Percent = math.Round(basePercent*10) / 10
	}
}

// UpdateTileMetrics updates tile counters and recalculates dynamic progress and time estimates.
func (m *ExportProgressManager) UpdateTileMetrics(datasetID string, completed, expected, skipped, cached int, layerName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prog := m.jobs[datasetID]
	if prog == nil || prog.Status != "processing" {
		return
	}

	prog.CompletedTiles = completed
	prog.ExpectedTiles = expected
	prog.EmptyTilesSkipped = skipped
	prog.CachedTilesReused = cached
	if layerName != "" {
		prog.CurrentLayer = layerName
	}
	prog.UpdatedAt = time.Now()
	prog.DurationSeconds = math.Round(prog.UpdatedAt.Sub(prog.StartTime).Seconds()*10) / 10

	// Interpolate tile completion progress between stage 3 and stage 4
	if prog.StageIndex >= 2 && prog.StageIndex <= 4 && expected > 0 {
		stageBase := 25.0
		stageSpan := 50.0 // from 25% to 75%
		ratio := float64(completed) / float64(expected)
		if ratio > 1.0 {
			ratio = 1.0
		}
		newPct := stageBase + ratio*stageSpan
		if newPct > prog.Percent {
			prog.Percent = math.Round(newPct*10) / 10
		}

		// Estimate remaining time
		if ratio > 0.05 && prog.DurationSeconds > 0.5 {
			totalEstSec := prog.DurationSeconds / ratio
			rem := totalEstSec - prog.DurationSeconds
			if rem > 0 {
				prog.EstimatedRemainingSeconds = math.Round(rem*10) / 10
			}
		}
	}
}

// CompleteExport marks the export as successfully finished with final size metrics.
func (m *ExportProgressManager) CompleteExport(datasetID string, finalSizeBytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prog := m.jobs[datasetID]
	if prog == nil {
		return
	}

	prog.Status = "completed"
	prog.Stage = "Export completed"
	prog.StageIndex = 8
	prog.Percent = 100.0
	prog.EstimatedRemainingSeconds = 0.0
	prog.FinalPackageSizeBytes = finalSizeBytes
	prog.UpdatedAt = time.Now()
	prog.DurationSeconds = math.Round(prog.UpdatedAt.Sub(prog.StartTime).Seconds()*10) / 10
}

// FailExport records an error on the active export job.
func (m *ExportProgressManager) FailExport(datasetID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prog := m.jobs[datasetID]
	if prog == nil {
		return
	}

	prog.Status = "error"
	if err != nil {
		prog.Error = err.Error()
	} else {
		prog.Error = "Unknown export error"
	}
	prog.UpdatedAt = time.Now()
	prog.DurationSeconds = math.Round(prog.UpdatedAt.Sub(prog.StartTime).Seconds()*10) / 10
}

// CancelExport aborts the export job if active and signals its cancelFunc.
func (m *ExportProgressManager) CancelExport(datasetID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	prog := m.jobs[datasetID]
	if prog == nil || prog.Status != "processing" {
		return false
	}

	prog.Status = "cancelled"
	prog.Error = "Export cancelled by user"
	prog.UpdatedAt = time.Now()

	if prog.cancelFunc != nil {
		prog.cancelFunc()
	}
	return true
}

// GetProgress retrieves the current progress snapshot for a dataset.
func (m *ExportProgressManager) GetProgress(datasetID string) *ExportProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prog := m.jobs[datasetID]
	if prog == nil {
		return nil
	}
	// Return a shallow copy to prevent concurrent map read/write
	cp := *prog
	return &cp
}
