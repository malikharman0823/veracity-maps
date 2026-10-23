package datasets

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

type RasterTransformRequest struct {
	Action          string            `json:"action"` // "status", "preview", "apply", "undo", "redo", "reset"
	Mode            string            `json:"mode,omitempty"`
	Corners         gis.RasterCorners `json:"corners,omitempty"`
	RotationAngle   *float64          `json:"rotation_angle,omitempty"`
	LockAspectRatio bool              `json:"lock_aspect_ratio,omitempty"`
	Resampling      string            `json:"resampling,omitempty"`
}

type RasterTransformData struct {
	DatasetID       string            `json:"dataset_id"`
	Status          string            `json:"status"`
	Corners         gis.RasterCorners `json:"corners"`
	OriginalCorners gis.RasterCorners `json:"original_corners"`
	Bounds          []float64         `json:"bounds"`
	Width           int               `json:"width"`
	Height          int               `json:"height"`
	RotationAngle   float64           `json:"rotation_angle"`
	LockAspectRatio bool              `json:"lock_aspect_ratio"`
	CanUndo         bool              `json:"can_undo"`
	CanRedo         bool              `json:"can_redo"`
	HistoryLength   int               `json:"history_length"`
	Message         string            `json:"message"`
	UpdatedAt       string            `json:"updated_at,omitempty"`
}

type RasterTransformResponse struct {
	Data RasterTransformData `json:"data"`
}

type TransformHistoryStep struct {
	Corners         gis.RasterCorners `json:"corners"`
	RotationAngle   float64           `json:"rotation_angle"`
	LockAspectRatio bool              `json:"lock_aspect_ratio"`
	Timestamp       string            `json:"timestamp"`
}

type TransformHistoryState struct {
	DatasetID       string                 `json:"dataset_id"`
	OriginalCorners gis.RasterCorners      `json:"original_corners"`
	Width           int                    `json:"width"`
	Height          int                    `json:"height"`
	History         []TransformHistoryStep `json:"history"`
	CurrentIndex    int                    `json:"current_index"`
}

func getTransformHistoryPath(dataset *models.Dataset) string {
	datasetDir := filepath.Dir(dataset.StoragePath)
	return filepath.Join(datasetDir, "processed", "transform_history.json")
}

func loadOrCreateTransformHistory(dataset *models.Dataset) (*TransformHistoryState, error) {
	historyPath := getTransformHistoryPath(dataset)
	if fileExists(historyPath) {
		data, err := os.ReadFile(historyPath)
		if err == nil {
			var state TransformHistoryState
			if err := json.Unmarshal(data, &state); err == nil && len(state.History) > 0 {
				return &state, nil
			}
		}
	}

	corners, _, width, height, err := gis.ExtractRasterCornersWGS84(dataset)
	if err != nil {
		return nil, err
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	state := &TransformHistoryState{
		DatasetID:       dataset.ID,
		OriginalCorners: corners,
		Width:           width,
		Height:          height,
		History: []TransformHistoryStep{
			{
				Corners:         corners,
				RotationAngle:   0.0,
				LockAspectRatio: false,
				Timestamp:       nowStr,
			},
		},
		CurrentIndex: 0,
	}

	_ = saveTransformHistory(dataset, state)
	return state, nil
}

func saveTransformHistory(dataset *models.Dataset, state *TransformHistoryState) error {
	historyPath := getTransformHistoryPath(dataset)
	_ = os.MkdirAll(filepath.Dir(historyPath), 0755)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(historyPath, data, 0644)
}

// GetRasterTransform handles GET /datasets/{id}/transform
func (h *Handler) GetRasterTransform(w http.ResponseWriter, r *http.Request, datasetID string) {
	dataset, _, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil || dataset == nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	state, err := loadOrCreateTransformHistory(dataset)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "TRANSFORM_INIT_FAILED", err.Error(), nil, nil)
		return
	}

	currentStep := state.History[state.CurrentIndex]
	bounds := gis.CalculateBoundsFromCorners(currentStep.Corners)

	models.WriteJSON(w, http.StatusOK, RasterTransformData{
		DatasetID:       datasetID,
		Status:          "ready",
		Corners:         currentStep.Corners,
		OriginalCorners: state.OriginalCorners,
		Bounds:          bounds,
		Width:           state.Width,
		Height:          state.Height,
		RotationAngle:   currentStep.RotationAngle,
		LockAspectRatio: currentStep.LockAspectRatio,
		CanUndo:         state.CurrentIndex > 0,
		CanRedo:         state.CurrentIndex < len(state.History)-1,
		HistoryLength:   len(state.History),
		Message:         "Raster transformation ready.",
		UpdatedAt:       currentStep.Timestamp,
	})
}

// HandleRasterTransform handles POST /datasets/{id}/transform
func (h *Handler) HandleRasterTransform(w http.ResponseWriter, r *http.Request, datasetID string) {
	dataset, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil || dataset == nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	var req RasterTransformRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse JSON body", nil, nil)
		return
	}

	state, err := loadOrCreateTransformHistory(dataset)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "TRANSFORM_LOAD_FAILED", err.Error(), nil, nil)
		return
	}

	// 1. Status action
	if req.Action == "status" {
		h.GetRasterTransform(w, r, datasetID)
		return
	}

	// 2. Preview action
	if req.Action == "preview" {
		if !gis.PolygonIsSimpleAndConvex(req.Corners) {
			models.WriteError(w, http.StatusBadRequest, "INVALID_GEOMETRY", "Transformed footprint must form a valid, non-self-intersecting, convex quadrilateral", nil, nil)
			return
		}
		bounds := gis.CalculateBoundsFromCorners(req.Corners)
		rot := 0.0
		if req.RotationAngle != nil {
			rot = *req.RotationAngle
		}

		models.WriteJSON(w, http.StatusOK, RasterTransformData{
			DatasetID:       datasetID,
			Status:          "preview",
			Corners:         req.Corners,
			OriginalCorners: state.OriginalCorners,
			Bounds:          bounds,
			Width:           state.Width,
			Height:          state.Height,
			RotationAngle:   rot,
			LockAspectRatio: req.LockAspectRatio,
			CanUndo:         state.CurrentIndex > 0,
			CanRedo:         state.CurrentIndex < len(state.History)-1,
			HistoryLength:   len(state.History),
			Message:         "Preview transformation calculated.",
		})
		return
	}

	// 3. Undo action
	if req.Action == "undo" {
		if state.CurrentIndex <= 0 {
			models.WriteError(w, http.StatusBadRequest, "CANNOT_UNDO", "No previous transform state available to undo", nil, nil)
			return
		}
		state.CurrentIndex--
		currentStep := state.History[state.CurrentIndex]

		if err := h.applyCornersToDataset(sessionID, dataset, currentStep.Corners, state.Width, state.Height, currentStep.RotationAngle, currentStep.LockAspectRatio); err != nil {
			models.WriteError(w, http.StatusInternalServerError, "UNDO_FAILED", err.Error(), nil, nil)
			return
		}
		_ = saveTransformHistory(dataset, state)

		bounds := gis.CalculateBoundsFromCorners(currentStep.Corners)
		models.WriteJSON(w, http.StatusOK, RasterTransformData{
			DatasetID:       datasetID,
			Status:          "undone",
			Corners:         currentStep.Corners,
			OriginalCorners: state.OriginalCorners,
			Bounds:          bounds,
			Width:           state.Width,
			Height:          state.Height,
			RotationAngle:   currentStep.RotationAngle,
			LockAspectRatio: currentStep.LockAspectRatio,
			CanUndo:         state.CurrentIndex > 0,
			CanRedo:         state.CurrentIndex < len(state.History)-1,
			HistoryLength:   len(state.History),
			Message:         "Transformation undone successfully.",
			UpdatedAt:       currentStep.Timestamp,
		})
		return
	}

	// 4. Redo action
	if req.Action == "redo" {
		if state.CurrentIndex >= len(state.History)-1 {
			models.WriteError(w, http.StatusBadRequest, "CANNOT_REDO", "No forward transform state available to redo", nil, nil)
			return
		}
		state.CurrentIndex++
		currentStep := state.History[state.CurrentIndex]

		if err := h.applyCornersToDataset(sessionID, dataset, currentStep.Corners, state.Width, state.Height, currentStep.RotationAngle, currentStep.LockAspectRatio); err != nil {
			models.WriteError(w, http.StatusInternalServerError, "REDO_FAILED", err.Error(), nil, nil)
			return
		}
		_ = saveTransformHistory(dataset, state)

		bounds := gis.CalculateBoundsFromCorners(currentStep.Corners)
		models.WriteJSON(w, http.StatusOK, RasterTransformData{
			DatasetID:       datasetID,
			Status:          "redone",
			Corners:         currentStep.Corners,
			OriginalCorners: state.OriginalCorners,
			Bounds:          bounds,
			Width:           state.Width,
			Height:          state.Height,
			RotationAngle:   currentStep.RotationAngle,
			LockAspectRatio: currentStep.LockAspectRatio,
			CanUndo:         state.CurrentIndex > 0,
			CanRedo:         state.CurrentIndex < len(state.History)-1,
			HistoryLength:   len(state.History),
			Message:         "Transformation redone successfully.",
			UpdatedAt:       currentStep.Timestamp,
		})
		return
	}

	// 5. Reset action
	if req.Action == "reset" {
		orig := state.OriginalCorners
		nowStr := time.Now().UTC().Format(time.RFC3339)
		if err := h.applyCornersToDataset(sessionID, dataset, orig, state.Width, state.Height, 0.0, false); err != nil {
			models.WriteError(w, http.StatusInternalServerError, "RESET_FAILED", err.Error(), nil, nil)
			return
		}

		state.History = []TransformHistoryStep{
			{
				Corners:         orig,
				RotationAngle:   0.0,
				LockAspectRatio: false,
				Timestamp:       nowStr,
			},
		}
		state.CurrentIndex = 0
		_ = saveTransformHistory(dataset, state)

		bounds := gis.CalculateBoundsFromCorners(orig)
		models.WriteJSON(w, http.StatusOK, RasterTransformData{
			DatasetID:       datasetID,
			Status:          "reset",
			Corners:         orig,
			OriginalCorners: state.OriginalCorners,
			Bounds:          bounds,
			Width:           state.Width,
			Height:          state.Height,
			RotationAngle:   0.0,
			LockAspectRatio: false,
			CanUndo:         false,
			CanRedo:         false,
			HistoryLength:   1,
			Message:         "Raster reset to its original geographic footprint.",
			UpdatedAt:       nowStr,
		})
		return
	}

	// 6. Apply action
	if req.Action == "apply" {
		if !gis.PolygonIsSimpleAndConvex(req.Corners) {
			models.WriteError(w, http.StatusBadRequest, "INVALID_GEOMETRY", "Transformed footprint must form a valid, non-self-intersecting, convex quadrilateral", nil, nil)
			return
		}

		rot := 0.0
		if req.RotationAngle != nil {
			rot = *req.RotationAngle
		}

		if err := h.applyCornersToDataset(sessionID, dataset, req.Corners, state.Width, state.Height, rot, req.LockAspectRatio); err != nil {
			models.WriteError(w, http.StatusInternalServerError, "APPLY_FAILED", err.Error(), nil, nil)
			return
		}

		nowStr := time.Now().UTC().Format(time.RFC3339)
		newStep := TransformHistoryStep{
			Corners:         req.Corners,
			RotationAngle:   rot,
			LockAspectRatio: req.LockAspectRatio,
			Timestamp:       nowStr,
		}

		// Truncate forward history if current index was undone
		if state.CurrentIndex < len(state.History)-1 {
			state.History = state.History[:state.CurrentIndex+1]
		}
		state.History = append(state.History, newStep)

		// Max 15 entries in history stack
		if len(state.History) > 15 {
			state.History = state.History[len(state.History)-15:]
		}
		state.CurrentIndex = len(state.History) - 1
		_ = saveTransformHistory(dataset, state)

		bounds := gis.CalculateBoundsFromCorners(req.Corners)
		models.WriteJSON(w, http.StatusOK, RasterTransformData{
			DatasetID:       datasetID,
			Status:          "applied",
			Corners:         req.Corners,
			OriginalCorners: state.OriginalCorners,
			Bounds:          bounds,
			Width:           state.Width,
			Height:          state.Height,
			RotationAngle:   rot,
			LockAspectRatio: req.LockAspectRatio,
			CanUndo:         state.CurrentIndex > 0,
			CanRedo:         false,
			HistoryLength:   len(state.History),
			Message:         "Transformation applied successfully.",
			UpdatedAt:       nowStr,
		})
		return
	}

	models.WriteError(w, http.StatusBadRequest, "INVALID_ACTION", "Unsupported transform action: "+req.Action, nil, nil)
}

func (h *Handler) applyCornersToDataset(sessionID string, dataset *models.Dataset, corners gis.RasterCorners, width, height int, rotAngle float64, lockAspect bool) error {
	transform, bounds, err := gis.CalculateTransformFromCorners(corners, width, height)
	if err != nil {
		return err
	}

	if dataset.ExtractedMetadata == nil {
		dataset.ExtractedMetadata = make(map[string]interface{})
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	dataset.ExtractedMetadata["transform"] = transform
	dataset.ExtractedMetadata["bounds"] = map[string]interface{}{
		"min_x": bounds[0],
		"min_y": bounds[1],
		"max_x": bounds[2],
		"max_y": bounds[3],
	}
	dataset.ExtractedMetadata["raster_transform"] = map[string]interface{}{
		"rotation_angle":    rotAngle,
		"lock_aspect_ratio": lockAspect,
		"updated_at":        nowStr,
	}

	coords := [][]float64{
		{corners.TopLeft.Longitude, corners.TopLeft.Latitude},
		{corners.TopRight.Longitude, corners.TopRight.Latitude},
		{corners.BottomRight.Longitude, corners.BottomRight.Latitude},
		{corners.BottomLeft.Longitude, corners.BottomLeft.Latitude},
	}

	if dataset.MapLayer == nil {
		dataset.MapLayer = make(map[string]interface{})
	}
	dataset.MapLayer["coordinates"] = coords
	dataset.MapLayer["bounds"] = bounds
	dataset.MapLayer["updated_at"] = nowStr

	// Update dataset in repository
	if err := h.repo.UpdateDataset(sessionID, dataset); err != nil {
		return err
	}

	// Update existing layer bounds
	layers, _ := h.repo.ListLayers(sessionID, dataset.ID)
	if len(layers) > 0 {
		for _, lyr := range layers {
			lyr.Bounds = bounds
		}
		_ = h.repo.SetLayers(sessionID, dataset.ID, layers)
	}

	// Synchronize control points
	cps, err := h.repo.ListControlPoints(sessionID, dataset.ID)
	if err == nil && len(cps) > 0 {
		gis.SynchronizeControlPoints(cps, transform)
		_ = h.repo.SetControlPoints(sessionID, dataset.ID, cps)
	}

	// Propagate building footprint/georeferencing to sibling floors if dataset belongs to a building
	if h.datasetService != nil {
		_ = h.datasetService.PropagateBuildingGeoreference(sessionID, dataset)
	}

	return nil
}
