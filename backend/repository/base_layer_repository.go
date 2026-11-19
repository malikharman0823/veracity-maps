package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// BaseLayerRepo defines persistence for custom base layers, workspace base layer selections,
// and per-workspace map view preferences.
type BaseLayerRepo interface {
	CreateBaseLayer(ctx context.Context, bl *models.BaseLayer) error
	GetBaseLayer(ctx context.Context, id string) (*models.BaseLayer, error)
	ListUserBaseLayers(ctx context.Context, userID string) ([]*models.BaseLayer, error)
	UpdateBaseLayer(ctx context.Context, bl *models.BaseLayer) error
	DeleteBaseLayer(ctx context.Context, id string) error

	GetWorkspaceBaseLayer(ctx context.Context, workspaceID string) (*models.WorkspaceBaseLayer, error)
	UpsertWorkspaceBaseLayer(ctx context.Context, wbl *models.WorkspaceBaseLayer) error

	GetMapViewPreferences(ctx context.Context, workspaceID string) (*models.MapViewPreferences, error)
	UpsertMapViewPreferences(ctx context.Context, prefs *models.MapViewPreferences) error
}

// ============================================================================
// IN-MEMORY IMPLEMENTATION (Tests & Dev Fallback)
// ============================================================================

type MemoryBaseLayerRepository struct {
	mu              sync.RWMutex
	baseLayers      map[string]*models.BaseLayer
	workspaceLayers map[string]*models.WorkspaceBaseLayer
	viewPreferences map[string]*models.MapViewPreferences
}

func NewMemoryBaseLayerRepository() *MemoryBaseLayerRepository {
	return &MemoryBaseLayerRepository{
		baseLayers:      make(map[string]*models.BaseLayer),
		workspaceLayers: make(map[string]*models.WorkspaceBaseLayer),
		viewPreferences: make(map[string]*models.MapViewPreferences),
	}
}

func cloneBaseLayer(in *models.BaseLayer) *models.BaseLayer {
	if in == nil {
		return nil
	}
	cp := *in
	if in.ExtentMinX != nil {
		v := *in.ExtentMinX
		cp.ExtentMinX = &v
	}
	if in.ExtentMinY != nil {
		v := *in.ExtentMinY
		cp.ExtentMinY = &v
	}
	if in.ExtentMaxX != nil {
		v := *in.ExtentMaxX
		cp.ExtentMaxX = &v
	}
	if in.ExtentMaxY != nil {
		v := *in.ExtentMaxY
		cp.ExtentMaxY = &v
	}
	if in.Extent != nil {
		cp.Extent = append([]float64(nil), in.Extent...)
	}
	if in.RenderingSource != nil {
		raw, _ := json.Marshal(in.RenderingSource)
		var m map[string]interface{}
		_ = json.Unmarshal(raw, &m)
		cp.RenderingSource = m
	}
	return &cp
}

func (r *MemoryBaseLayerRepository) CreateBaseLayer(_ context.Context, bl *models.BaseLayer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if bl.CreatedAt.IsZero() {
		bl.CreatedAt = now
	}
	bl.UpdatedAt = now
	r.baseLayers[bl.ID] = cloneBaseLayer(bl)
	return nil
}

func (r *MemoryBaseLayerRepository) GetBaseLayer(_ context.Context, id string) (*models.BaseLayer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bl, ok := r.baseLayers[id]
	if !ok {
		return nil, models.ErrBaseLayerNotFound
	}
	return cloneBaseLayer(bl), nil
}

func (r *MemoryBaseLayerRepository) ListUserBaseLayers(_ context.Context, userID string) ([]*models.BaseLayer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*models.BaseLayer, 0)
	for _, bl := range r.baseLayers {
		if bl.UserID == userID {
			out = append(out, cloneBaseLayer(bl))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (r *MemoryBaseLayerRepository) UpdateBaseLayer(_ context.Context, bl *models.BaseLayer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.baseLayers[bl.ID]; !ok {
		return models.ErrBaseLayerNotFound
	}
	bl.UpdatedAt = time.Now().UTC()
	r.baseLayers[bl.ID] = cloneBaseLayer(bl)
	return nil
}

func (r *MemoryBaseLayerRepository) DeleteBaseLayer(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.baseLayers[id]; !ok {
		return models.ErrBaseLayerNotFound
	}
	delete(r.baseLayers, id)
	return nil
}

func (r *MemoryBaseLayerRepository) GetWorkspaceBaseLayer(_ context.Context, workspaceID string) (*models.WorkspaceBaseLayer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	wbl, ok := r.workspaceLayers[workspaceID]
	if !ok {
		return nil, models.ErrBaseLayerNotFound
	}
	cp := *wbl
	cp.BaseLayer = cloneBaseLayer(wbl.BaseLayer)
	return &cp, nil
}

func (r *MemoryBaseLayerRepository) UpsertWorkspaceBaseLayer(_ context.Context, wbl *models.WorkspaceBaseLayer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if existing, ok := r.workspaceLayers[wbl.WorkspaceID]; ok {
		wbl.ID = existing.ID
		wbl.CreatedAt = existing.CreatedAt
	} else if wbl.CreatedAt.IsZero() {
		wbl.CreatedAt = now
	}
	wbl.UpdatedAt = now
	cp := *wbl
	cp.BaseLayer = nil // hydrated dynamically on read
	r.workspaceLayers[wbl.WorkspaceID] = &cp
	return nil
}

func (r *MemoryBaseLayerRepository) GetMapViewPreferences(_ context.Context, workspaceID string) (*models.MapViewPreferences, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.viewPreferences[workspaceID]
	if !ok {
		return nil, models.ErrBaseLayerNotFound
	}
	cp := *p
	if p.StyleConfiguration != nil {
		raw, _ := json.Marshal(p.StyleConfiguration)
		var m map[string]interface{}
		_ = json.Unmarshal(raw, &m)
		cp.StyleConfiguration = m
	}
	return &cp, nil
}

func (r *MemoryBaseLayerRepository) UpsertMapViewPreferences(_ context.Context, prefs *models.MapViewPreferences) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if existing, ok := r.viewPreferences[prefs.WorkspaceID]; ok {
		prefs.ID = existing.ID
		prefs.CreatedAt = existing.CreatedAt
	} else if prefs.CreatedAt.IsZero() {
		prefs.CreatedAt = now
	}
	prefs.UpdatedAt = now
	cp := *prefs
	if prefs.StyleConfiguration != nil {
		raw, _ := json.Marshal(prefs.StyleConfiguration)
		var m map[string]interface{}
		_ = json.Unmarshal(raw, &m)
		cp.StyleConfiguration = m
	}
	r.viewPreferences[prefs.WorkspaceID] = &cp
	return nil
}

// ============================================================================
// MSSQL SERVER 2022 IMPLEMENTATION
// ============================================================================

type MSSQLBaseLayerRepository struct {
	db *sql.DB
}

func NewMSSQLBaseLayerRepository(db *sql.DB) *MSSQLBaseLayerRepository {
	return &MSSQLBaseLayerRepository{db: db}
}

func (r *MSSQLBaseLayerRepository) CreateBaseLayer(ctx context.Context, bl *models.BaseLayer) error {
	now := time.Now().UTC()
	if bl.CreatedAt.IsZero() {
		bl.CreatedAt = now
	}
	bl.UpdatedAt = now
	renderJSON := "{}"
	if bl.RenderingSource != nil {
		if b, err := json.Marshal(bl.RenderingSource); err == nil {
			renderJSON = string(b)
		}
	}
	var dsID interface{}
	if bl.DatasetID != "" {
		dsID = bl.DatasetID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.base_layers (
			id, user_id, name, description, dataset_id, source_type, source_crs,
			extent_min_x, extent_min_y, extent_max_x, extent_max_y,
			preview_path, rendering_source, status, is_default, created_at, updated_at
		) VALUES (
			@p1, @p2, @p3, @p4, @p5, @p6, @p7,
			@p8, @p9, @p10, @p11,
			@p12, @p13, @p14, @p15, @p16, @p17
		);
	`, bl.ID, bl.UserID, bl.Name, bl.Description, dsID, bl.SourceType, bl.SourceCRS,
		bl.ExtentMinX, bl.ExtentMinY, bl.ExtentMaxX, bl.ExtentMaxY,
		bl.PreviewPath, renderJSON, bl.Status, bl.IsDefault, bl.CreatedAt, bl.UpdatedAt)
	return err
}

func scanBaseLayerRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.BaseLayer, error) {
	var bl models.BaseLayer
	var dsID sql.NullString
	var minX, minY, maxX, maxY sql.NullFloat64
	var renderJSON sql.NullString

	err := scanner.Scan(
		&bl.ID, &bl.UserID, &bl.Name, &bl.Description, &dsID, &bl.SourceType, &bl.SourceCRS,
		&minX, &minY, &maxX, &maxY,
		&bl.PreviewPath, &renderJSON, &bl.Status, &bl.IsDefault, &bl.CreatedAt, &bl.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if dsID.Valid {
		bl.DatasetID = dsID.String
	}
	if minX.Valid && minY.Valid && maxX.Valid && maxY.Valid {
		v1, v2, v3, v4 := minX.Float64, minY.Float64, maxX.Float64, maxY.Float64
		bl.ExtentMinX = &v1
		bl.ExtentMinY = &v2
		bl.ExtentMaxX = &v3
		bl.ExtentMaxY = &v4
		bl.Extent = []float64{v1, v2, v3, v4}
	}
	if renderJSON.Valid && renderJSON.String != "" {
		var m map[string]interface{}
		if json.Unmarshal([]byte(renderJSON.String), &m) == nil {
			bl.RenderingSource = m
			if dn, ok := m["dataset_name"].(string); ok {
				bl.DatasetName = dn
			}
		}
	}
	return &bl, nil
}

func (r *MSSQLBaseLayerRepository) GetBaseLayer(ctx context.Context, id string) (*models.BaseLayer, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, description, dataset_id, source_type, source_crs,
		       extent_min_x, extent_min_y, extent_max_x, extent_max_y,
		       preview_path, rendering_source, status, is_default, created_at, updated_at
		FROM dbo.base_layers
		WHERE id = @p1;
	`, id)
	bl, err := scanBaseLayerRow(row)
	if err == sql.ErrNoRows {
		return nil, models.ErrBaseLayerNotFound
	}
	return bl, err
}

func (r *MSSQLBaseLayerRepository) ListUserBaseLayers(ctx context.Context, userID string) ([]*models.BaseLayer, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, dataset_id, source_type, source_crs,
		       extent_min_x, extent_min_y, extent_max_x, extent_max_y,
		       preview_path, rendering_source, status, is_default, created_at, updated_at
		FROM dbo.base_layers
		WHERE user_id = @p1
		ORDER BY created_at DESC;
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*models.BaseLayer, 0)
	for rows.Next() {
		bl, scanErr := scanBaseLayerRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, bl)
	}
	return out, rows.Err()
}

func (r *MSSQLBaseLayerRepository) UpdateBaseLayer(ctx context.Context, bl *models.BaseLayer) error {
	bl.UpdatedAt = time.Now().UTC()
	renderJSON := "{}"
	if bl.RenderingSource != nil {
		if b, err := json.Marshal(bl.RenderingSource); err == nil {
			renderJSON = string(b)
		}
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE dbo.base_layers
		SET name = @p1, description = @p2, source_crs = @p3,
		    extent_min_x = @p4, extent_min_y = @p5, extent_max_x = @p6, extent_max_y = @p7,
		    preview_path = @p8, rendering_source = @p9, status = @p10, updated_at = @p11
		WHERE id = @p12;
	`, bl.Name, bl.Description, bl.SourceCRS,
		bl.ExtentMinX, bl.ExtentMinY, bl.ExtentMaxX, bl.ExtentMaxY,
		bl.PreviewPath, renderJSON, bl.Status, bl.UpdatedAt, bl.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrBaseLayerNotFound
	}
	return nil
}

func (r *MSSQLBaseLayerRepository) DeleteBaseLayer(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM dbo.base_layers WHERE id = @p1;`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrBaseLayerNotFound
	}
	return nil
}

func (r *MSSQLBaseLayerRepository) GetWorkspaceBaseLayer(ctx context.Context, workspaceID string) (*models.WorkspaceBaseLayer, error) {
	var wbl models.WorkspaceBaseLayer
	var blID sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, base_layer_id, base_layer_type, selected_view, opacity, visibility, created_at, updated_at
		FROM dbo.workspace_base_layers
		WHERE workspace_id = @p1;
	`, workspaceID).Scan(
		&wbl.ID, &wbl.WorkspaceID, &blID, &wbl.BaseLayerType, &wbl.SelectedView,
		&wbl.Opacity, &wbl.Visibility, &wbl.CreatedAt, &wbl.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, models.ErrBaseLayerNotFound
	}
	if err != nil {
		return nil, err
	}
	if blID.Valid {
		wbl.BaseLayerID = blID.String
	}
	return &wbl, nil
}

func (r *MSSQLBaseLayerRepository) UpsertWorkspaceBaseLayer(ctx context.Context, wbl *models.WorkspaceBaseLayer) error {
	now := time.Now().UTC()
	if wbl.CreatedAt.IsZero() {
		wbl.CreatedAt = now
	}
	wbl.UpdatedAt = now
	var blID interface{}
	if wbl.BaseLayerID != "" {
		blID = wbl.BaseLayerID
	}
	_, err := r.db.ExecContext(ctx, `
		MERGE dbo.workspace_base_layers AS target
		USING (SELECT @p1 AS workspace_id) AS source
		ON target.workspace_id = source.workspace_id
		WHEN MATCHED THEN
			UPDATE SET base_layer_id = @p2, base_layer_type = @p3, selected_view = @p4,
			           opacity = @p5, visibility = @p6, updated_at = @p7
		WHEN NOT MATCHED THEN
			INSERT (id, workspace_id, base_layer_id, base_layer_type, selected_view, opacity, visibility, created_at, updated_at)
			VALUES (@p8, @p1, @p2, @p3, @p4, @p5, @p6, @p9, @p7);
	`, wbl.WorkspaceID, blID, wbl.BaseLayerType, wbl.SelectedView, wbl.Opacity, wbl.Visibility, wbl.UpdatedAt, wbl.ID, wbl.CreatedAt)
	return err
}

func (r *MSSQLBaseLayerRepository) GetMapViewPreferences(ctx context.Context, workspaceID string) (*models.MapViewPreferences, error) {
	var prefs models.MapViewPreferences
	var styleJSON sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, view_type, style_configuration, show_labels, show_roads, show_places, base_layer_opacity, created_at, updated_at
		FROM dbo.map_view_preferences
		WHERE workspace_id = @p1;
	`, workspaceID).Scan(
		&prefs.ID, &prefs.WorkspaceID, &prefs.ViewType, &styleJSON,
		&prefs.ShowLabels, &prefs.ShowRoads, &prefs.ShowPlaces, &prefs.BaseLayerOpacity,
		&prefs.CreatedAt, &prefs.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, models.ErrBaseLayerNotFound
	}
	if err != nil {
		return nil, err
	}
	prefs.BaseLayerVisible = true
	if styleJSON.Valid && styleJSON.String != "" {
		var m map[string]interface{}
		if json.Unmarshal([]byte(styleJSON.String), &m) == nil {
			prefs.StyleConfiguration = m
			if vis, ok := m["base_layer_visible"].(bool); ok {
				prefs.BaseLayerVisible = vis
			}
		}
	}
	return &prefs, nil
}

func (r *MSSQLBaseLayerRepository) UpsertMapViewPreferences(ctx context.Context, prefs *models.MapViewPreferences) error {
	now := time.Now().UTC()
	if prefs.CreatedAt.IsZero() {
		prefs.CreatedAt = now
	}
	prefs.UpdatedAt = now
	cfg := make(map[string]interface{})
	for k, v := range prefs.StyleConfiguration {
		cfg[k] = v
	}
	cfg["base_layer_visible"] = prefs.BaseLayerVisible
	styleJSON := "{}"
	if b, err := json.Marshal(cfg); err == nil {
		styleJSON = string(b)
	}
	_, err := r.db.ExecContext(ctx, `
		MERGE dbo.map_view_preferences AS target
		USING (SELECT @p1 AS workspace_id) AS source
		ON target.workspace_id = source.workspace_id
		WHEN MATCHED THEN
			UPDATE SET view_type = @p2, style_configuration = @p3, show_labels = @p4,
			           show_roads = @p5, show_places = @p6, base_layer_opacity = @p7, updated_at = @p8
		WHEN NOT MATCHED THEN
			INSERT (id, workspace_id, view_type, style_configuration, show_labels, show_roads, show_places, base_layer_opacity, created_at, updated_at)
			VALUES (@p9, @p1, @p2, @p3, @p4, @p5, @p6, @p7, @p10, @p8);
	`, prefs.WorkspaceID, prefs.ViewType, styleJSON, prefs.ShowLabels, prefs.ShowRoads, prefs.ShowPlaces, prefs.BaseLayerOpacity, prefs.UpdatedAt, prefs.ID, prefs.CreatedAt)
	return err
}
