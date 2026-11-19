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

// WorkspaceRegistryRepo persists workspace datasets, independently selectable workspace layers,
// and workspace raster metadata (reusing vector_features, control points, and icon points).
type WorkspaceRegistryRepo interface {
	CreateWorkspaceDataset(ctx context.Context, ds *models.WorkspaceDataset) error
	GetWorkspaceDatasetBySource(ctx context.Context, workspaceID, sourceDatasetID string) (*models.WorkspaceDataset, error)
	ListWorkspaceDatasets(ctx context.Context, workspaceID string) ([]*models.WorkspaceDataset, error)

	CreateWorkspaceLayer(ctx context.Context, lyr *models.WorkspaceLayer) error
	GetWorkspaceLayer(ctx context.Context, workspaceID, layerID string) (*models.WorkspaceLayer, error)
	ListWorkspaceLayers(ctx context.Context, workspaceID string) ([]*models.WorkspaceLayer, error)
	UpdateWorkspaceLayer(ctx context.Context, lyr *models.WorkspaceLayer) error
	DeleteWorkspaceLayer(ctx context.Context, workspaceID, layerID string) error

	UpsertWorkspaceRaster(ctx context.Context, raster *models.WorkspaceRaster) error
	GetWorkspaceRaster(ctx context.Context, workspaceID, rasterID string) (*models.WorkspaceRaster, error)
	ListWorkspaceRasters(ctx context.Context, workspaceID string) ([]*models.WorkspaceRaster, error)
	DeleteWorkspaceRasterByLayer(ctx context.Context, workspaceID, layerID string) error
}

// ============================================================================
// IN-MEMORY IMPLEMENTATION (Tests & Dev Fallback)
// ============================================================================

type MemoryWorkspaceRegistryRepository struct {
	mu       sync.RWMutex
	datasets map[string]*models.WorkspaceDataset
	layers   map[string]*models.WorkspaceLayer
	rasters  map[string]*models.WorkspaceRaster
}

func NewMemoryWorkspaceRegistryRepository() *MemoryWorkspaceRegistryRepository {
	return &MemoryWorkspaceRegistryRepository{
		datasets: make(map[string]*models.WorkspaceDataset),
		layers:   make(map[string]*models.WorkspaceLayer),
		rasters:  make(map[string]*models.WorkspaceRaster),
	}
}

func cloneWorkspaceLayer(in *models.WorkspaceLayer) *models.WorkspaceLayer {
	if in == nil {
		return nil
	}
	cp := *in
	if in.Extent != nil {
		cp.Extent = append([]float64(nil), in.Extent...)
	}
	if in.Fields != nil {
		raw, _ := json.Marshal(in.Fields)
		var f []map[string]interface{}
		_ = json.Unmarshal(raw, &f)
		cp.Fields = f
	}
	if in.RenderingSource != nil {
		raw, _ := json.Marshal(in.RenderingSource)
		var m map[string]interface{}
		_ = json.Unmarshal(raw, &m)
		cp.RenderingSource = m
	}
	return &cp
}

func (r *MemoryWorkspaceRegistryRepository) CreateWorkspaceDataset(_ context.Context, ds *models.WorkspaceDataset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = time.Now().UTC()
	}
	cp := *ds
	r.datasets[ds.ID] = &cp
	return nil
}

func (r *MemoryWorkspaceRegistryRepository) GetWorkspaceDatasetBySource(_ context.Context, workspaceID, sourceDatasetID string) (*models.WorkspaceDataset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ds := range r.datasets {
		if ds.WorkspaceID == workspaceID && ds.SourceDatasetID == sourceDatasetID {
			cp := *ds
			return &cp, nil
		}
	}
	return nil, models.ErrWorkspaceLayerNotFound
}

func (r *MemoryWorkspaceRegistryRepository) ListWorkspaceDatasets(_ context.Context, workspaceID string) ([]*models.WorkspaceDataset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*models.WorkspaceDataset, 0)
	for _, ds := range r.datasets {
		if ds.WorkspaceID == workspaceID {
			cp := *ds
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *MemoryWorkspaceRegistryRepository) CreateWorkspaceLayer(_ context.Context, lyr *models.WorkspaceLayer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if lyr.CreatedAt.IsZero() {
		lyr.CreatedAt = now
	}
	lyr.UpdatedAt = now
	r.layers[lyr.ID] = cloneWorkspaceLayer(lyr)
	return nil
}

func (r *MemoryWorkspaceRegistryRepository) GetWorkspaceLayer(_ context.Context, workspaceID, layerID string) (*models.WorkspaceLayer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lyr, ok := r.layers[layerID]
	if !ok || (workspaceID != "" && lyr.WorkspaceID != workspaceID) {
		return nil, models.ErrWorkspaceLayerNotFound
	}
	return cloneWorkspaceLayer(lyr), nil
}

func (r *MemoryWorkspaceRegistryRepository) ListWorkspaceLayers(_ context.Context, workspaceID string) ([]*models.WorkspaceLayer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*models.WorkspaceLayer, 0)
	for _, lyr := range r.layers {
		if lyr.WorkspaceID == workspaceID {
			out = append(out, cloneWorkspaceLayer(lyr))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LayerOrder == out[j].LayerOrder {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].LayerOrder < out[j].LayerOrder
	})
	return out, nil
}

func (r *MemoryWorkspaceRegistryRepository) UpdateWorkspaceLayer(_ context.Context, lyr *models.WorkspaceLayer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.layers[lyr.ID]; !ok {
		return models.ErrWorkspaceLayerNotFound
	}
	lyr.UpdatedAt = time.Now().UTC()
	r.layers[lyr.ID] = cloneWorkspaceLayer(lyr)
	return nil
}

func (r *MemoryWorkspaceRegistryRepository) DeleteWorkspaceLayer(_ context.Context, workspaceID, layerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	lyr, ok := r.layers[layerID]
	if !ok || (workspaceID != "" && lyr.WorkspaceID != workspaceID) {
		return models.ErrWorkspaceLayerNotFound
	}
	delete(r.layers, layerID)
	return nil
}

func (r *MemoryWorkspaceRegistryRepository) UpsertWorkspaceRaster(_ context.Context, raster *models.WorkspaceRaster) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if raster.CreatedAt.IsZero() {
		raster.CreatedAt = now
	}
	raster.UpdatedAt = now
	cp := *raster
	if raster.Bounds != nil {
		cp.Bounds = append([]float64(nil), raster.Bounds...)
	}
	r.rasters[raster.ID] = &cp
	return nil
}

func (r *MemoryWorkspaceRegistryRepository) GetWorkspaceRaster(_ context.Context, workspaceID, rasterID string) (*models.WorkspaceRaster, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rst, ok := r.rasters[rasterID]
	if !ok || (workspaceID != "" && rst.WorkspaceID != workspaceID) {
		return nil, models.ErrWorkspaceRasterNotFound
	}
	cp := *rst
	if rst.Bounds != nil {
		cp.Bounds = append([]float64(nil), rst.Bounds...)
	}
	return &cp, nil
}

func (r *MemoryWorkspaceRegistryRepository) ListWorkspaceRasters(_ context.Context, workspaceID string) ([]*models.WorkspaceRaster, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*models.WorkspaceRaster, 0)
	for _, rst := range r.rasters {
		if rst.WorkspaceID == workspaceID {
			cp := *rst
			if rst.Bounds != nil {
				cp.Bounds = append([]float64(nil), rst.Bounds...)
			}
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *MemoryWorkspaceRegistryRepository) DeleteWorkspaceRasterByLayer(_ context.Context, workspaceID, layerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, rst := range r.rasters {
		if rst.WorkspaceID == workspaceID && rst.WorkspaceLayerID == layerID {
			delete(r.rasters, id)
		}
	}
	return nil
}

// ============================================================================
// MSSQL SERVER 2022 IMPLEMENTATION
// ============================================================================

type MSSQLWorkspaceRegistryRepository struct {
	db *sql.DB
}

func NewMSSQLWorkspaceRegistryRepository(db *sql.DB) *MSSQLWorkspaceRegistryRepository {
	return &MSSQLWorkspaceRegistryRepository{db: db}
}

func (r *MSSQLWorkspaceRegistryRepository) CreateWorkspaceDataset(ctx context.Context, ds *models.WorkspaceDataset) error {
	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.workspace_datasets (
			id, workspace_id, user_id, source_dataset_id, original_filename,
			stored_file_path, format, file_size, upload_status, created_at
		) VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10);
	`, ds.ID, ds.WorkspaceID, ds.UserID, ds.SourceDatasetID, ds.OriginalFilename,
		ds.StoredFilePath, ds.Format, ds.FileSize, ds.UploadStatus, ds.CreatedAt)
	return err
}

func (r *MSSQLWorkspaceRegistryRepository) GetWorkspaceDatasetBySource(ctx context.Context, workspaceID, sourceDatasetID string) (*models.WorkspaceDataset, error) {
	var ds models.WorkspaceDataset
	err := r.db.QueryRowContext(ctx, `
		SELECT TOP 1 id, workspace_id, user_id, source_dataset_id, original_filename,
		       stored_file_path, format, file_size, upload_status, created_at
		FROM dbo.workspace_datasets
		WHERE workspace_id = @p1 AND source_dataset_id = @p2;
	`, workspaceID, sourceDatasetID).Scan(
		&ds.ID, &ds.WorkspaceID, &ds.UserID, &ds.SourceDatasetID, &ds.OriginalFilename,
		&ds.StoredFilePath, &ds.Format, &ds.FileSize, &ds.UploadStatus, &ds.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, models.ErrWorkspaceLayerNotFound
	}
	return &ds, err
}

func (r *MSSQLWorkspaceRegistryRepository) ListWorkspaceDatasets(ctx context.Context, workspaceID string) ([]*models.WorkspaceDataset, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, user_id, source_dataset_id, original_filename,
		       stored_file_path, format, file_size, upload_status, created_at
		FROM dbo.workspace_datasets
		WHERE workspace_id = @p1
		ORDER BY created_at DESC;
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.WorkspaceDataset, 0)
	for rows.Next() {
		var ds models.WorkspaceDataset
		if err := rows.Scan(
			&ds.ID, &ds.WorkspaceID, &ds.UserID, &ds.SourceDatasetID, &ds.OriginalFilename,
			&ds.StoredFilePath, &ds.Format, &ds.FileSize, &ds.UploadStatus, &ds.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &ds)
	}
	return out, rows.Err()
}

func (r *MSSQLWorkspaceRegistryRepository) CreateWorkspaceLayer(ctx context.Context, lyr *models.WorkspaceLayer) error {
	now := time.Now().UTC()
	if lyr.CreatedAt.IsZero() {
		lyr.CreatedAt = now
	}
	lyr.UpdatedAt = now
	fieldsJSON := "[]"
	if lyr.Fields != nil {
		if b, err := json.Marshal(lyr.Fields); err == nil {
			fieldsJSON = string(b)
		}
	}
	renderJSON := "{}"
	if lyr.RenderingSource != nil {
		if b, err := json.Marshal(lyr.RenderingSource); err == nil {
			renderJSON = string(b)
		}
	}
	var gisID interface{}
	if lyr.GISLayerID != "" {
		gisID = lyr.GISLayerID
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.workspace_layers (
			id, workspace_dataset_id, workspace_id, gis_layer_id, source_layer_name, display_name,
			geometry_type, source_crs, feature_count, extent_min_x, extent_min_y, extent_max_x, extent_max_y,
			fields_json, rendering_source, visibility, opacity, layer_order, status, created_at, updated_at
		) VALUES (
			@p1, @p2, @p3, @p4, @p5, @p6,
			@p7, @p8, @p9, @p10, @p11, @p12, @p13,
			@p14, @p15, @p16, @p17, @p18, @p19, @p20, @p21
		);
	`, lyr.ID, lyr.WorkspaceDatasetID, lyr.WorkspaceID, gisID, lyr.SourceLayerName, lyr.DisplayName,
		lyr.GeometryType, lyr.SourceCRS, lyr.FeatureCount, lyr.ExtentMinX, lyr.ExtentMinY, lyr.ExtentMaxX, lyr.ExtentMaxY,
		fieldsJSON, renderJSON, lyr.Visibility, lyr.Opacity, lyr.LayerOrder, lyr.Status, lyr.CreatedAt, lyr.UpdatedAt)
	return err
}

func scanWorkspaceLayerRow(scanner interface{ Scan(...interface{}) error }) (*models.WorkspaceLayer, error) {
	var lyr models.WorkspaceLayer
	var gisID sql.NullString
	var minX, minY, maxX, maxY sql.NullFloat64
	var fieldsJSON, renderJSON sql.NullString
	err := scanner.Scan(
		&lyr.ID, &lyr.WorkspaceDatasetID, &lyr.WorkspaceID, &gisID, &lyr.SourceLayerName, &lyr.DisplayName,
		&lyr.GeometryType, &lyr.SourceCRS, &lyr.FeatureCount, &minX, &minY, &maxX, &maxY,
		&fieldsJSON, &renderJSON, &lyr.Visibility, &lyr.Opacity, &lyr.LayerOrder, &lyr.Status,
		&lyr.CreatedAt, &lyr.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if gisID.Valid {
		lyr.GISLayerID = gisID.String
	}
	if minX.Valid && minY.Valid && maxX.Valid && maxY.Valid {
		v1, v2, v3, v4 := minX.Float64, minY.Float64, maxX.Float64, maxY.Float64
		lyr.ExtentMinX, lyr.ExtentMinY, lyr.ExtentMaxX, lyr.ExtentMaxY = &v1, &v2, &v3, &v4
		lyr.Extent = []float64{v1, v2, v3, v4}
	}
	if fieldsJSON.Valid && fieldsJSON.String != "" {
		_ = json.Unmarshal([]byte(fieldsJSON.String), &lyr.Fields)
	}
	if renderJSON.Valid && renderJSON.String != "" {
		_ = json.Unmarshal([]byte(renderJSON.String), &lyr.RenderingSource)
		if srcDs, ok := lyr.RenderingSource["source_dataset_id"].(string); ok {
			lyr.SourceDatasetID = srcDs
		}
	}
	return &lyr, nil
}

func (r *MSSQLWorkspaceRegistryRepository) GetWorkspaceLayer(ctx context.Context, workspaceID, layerID string) (*models.WorkspaceLayer, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_dataset_id, workspace_id, gis_layer_id, source_layer_name, display_name,
		       geometry_type, source_crs, feature_count, extent_min_x, extent_min_y, extent_max_x, extent_max_y,
		       fields_json, rendering_source, visibility, opacity, layer_order, status, created_at, updated_at
		FROM dbo.workspace_layers
		WHERE id = @p1 AND (@p2 = '' OR workspace_id = @p2);
	`, layerID, workspaceID)
	lyr, err := scanWorkspaceLayerRow(row)
	if err == sql.ErrNoRows {
		return nil, models.ErrWorkspaceLayerNotFound
	}
	return lyr, err
}

func (r *MSSQLWorkspaceRegistryRepository) ListWorkspaceLayers(ctx context.Context, workspaceID string) ([]*models.WorkspaceLayer, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_dataset_id, workspace_id, gis_layer_id, source_layer_name, display_name,
		       geometry_type, source_crs, feature_count, extent_min_x, extent_min_y, extent_max_x, extent_max_y,
		       fields_json, rendering_source, visibility, opacity, layer_order, status, created_at, updated_at
		FROM dbo.workspace_layers
		WHERE workspace_id = @p1
		ORDER BY layer_order ASC, created_at ASC;
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.WorkspaceLayer, 0)
	for rows.Next() {
		lyr, err := scanWorkspaceLayerRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lyr)
	}
	return out, rows.Err()
}

func (r *MSSQLWorkspaceRegistryRepository) UpdateWorkspaceLayer(ctx context.Context, lyr *models.WorkspaceLayer) error {
	lyr.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
		UPDATE dbo.workspace_layers
		SET display_name = @p1, visibility = @p2, opacity = @p3, layer_order = @p4, status = @p5, updated_at = @p6
		WHERE id = @p7 AND workspace_id = @p8;
	`, lyr.DisplayName, lyr.Visibility, lyr.Opacity, lyr.LayerOrder, lyr.Status, lyr.UpdatedAt, lyr.ID, lyr.WorkspaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrWorkspaceLayerNotFound
	}
	return nil
}

func (r *MSSQLWorkspaceRegistryRepository) DeleteWorkspaceLayer(ctx context.Context, workspaceID, layerID string) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM dbo.workspace_layers WHERE id = @p1 AND workspace_id = @p2;
	`, layerID, workspaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrWorkspaceLayerNotFound
	}
	return nil
}

func (r *MSSQLWorkspaceRegistryRepository) UpsertWorkspaceRaster(ctx context.Context, raster *models.WorkspaceRaster) error {
	now := time.Now().UTC()
	if raster.CreatedAt.IsZero() {
		raster.CreatedAt = now
	}
	raster.UpdatedAt = now
	boundsStr := "[]"
	if raster.Bounds != nil {
		if b, err := json.Marshal(raster.Bounds); err == nil {
			boundsStr = string(b)
		}
	}
	var wlID interface{}
	if raster.WorkspaceLayerID != "" {
		wlID = raster.WorkspaceLayerID
	}
	_, err := r.db.ExecContext(ctx, `
		MERGE dbo.workspace_rasters AS target
		USING (SELECT @p1 AS id) AS source
		ON target.id = source.id
		WHEN MATCHED THEN
			UPDATE SET display_name = @p2, visibility = @p3, opacity = @p4, status = @p5, bounds = @p6, updated_at = @p7
		WHEN NOT MATCHED THEN
			INSERT (id, workspace_id, workspace_layer_id, dataset_id, display_name, raster_path, width, height, band_count, source_crs, resolution_x, resolution_y, bounds, visibility, opacity, status, created_at, updated_at)
			VALUES (@p1, @p8, @p9, @p10, @p2, @p11, @p12, @p13, @p14, @p15, @p16, @p17, @p6, @p3, @p4, @p5, @p18, @p7);
	`, raster.ID, raster.DisplayName, raster.Visibility, raster.Opacity, raster.Status, boundsStr, raster.UpdatedAt,
		raster.WorkspaceID, wlID, raster.DatasetID, raster.RasterPath, raster.Width, raster.Height, raster.BandCount,
		raster.SourceCRS, raster.ResolutionX, raster.ResolutionY, raster.CreatedAt)
	return err
}

func scanWorkspaceRasterRow(scanner interface{ Scan(...interface{}) error }) (*models.WorkspaceRaster, error) {
	var rst models.WorkspaceRaster
	var wlID, boundsStr sql.NullString
	var resX, resY sql.NullFloat64
	err := scanner.Scan(
		&rst.ID, &rst.WorkspaceID, &wlID, &rst.DatasetID, &rst.DisplayName, &rst.RasterPath,
		&rst.Width, &rst.Height, &rst.BandCount, &rst.SourceCRS, &resX, &resY,
		&boundsStr, &rst.Visibility, &rst.Opacity, &rst.Status, &rst.CreatedAt, &rst.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if wlID.Valid {
		rst.WorkspaceLayerID = wlID.String
	}
	if resX.Valid {
		v := resX.Float64
		rst.ResolutionX = &v
	}
	if resY.Valid {
		v := resY.Float64
		rst.ResolutionY = &v
	}
	if boundsStr.Valid && boundsStr.String != "" {
		_ = json.Unmarshal([]byte(boundsStr.String), &rst.Bounds)
	}
	return &rst, nil
}

func (r *MSSQLWorkspaceRegistryRepository) GetWorkspaceRaster(ctx context.Context, workspaceID, rasterID string) (*models.WorkspaceRaster, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, workspace_layer_id, dataset_id, display_name, raster_path,
		       width, height, band_count, source_crs, resolution_x, resolution_y,
		       bounds, visibility, opacity, status, created_at, updated_at
		FROM dbo.workspace_rasters
		WHERE id = @p1 AND workspace_id = @p2;
	`, rasterID, workspaceID)
	rst, err := scanWorkspaceRasterRow(row)
	if err == sql.ErrNoRows {
		return nil, models.ErrWorkspaceRasterNotFound
	}
	return rst, err
}

func (r *MSSQLWorkspaceRegistryRepository) ListWorkspaceRasters(ctx context.Context, workspaceID string) ([]*models.WorkspaceRaster, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, workspace_layer_id, dataset_id, display_name, raster_path,
		       width, height, band_count, source_crs, resolution_x, resolution_y,
		       bounds, visibility, opacity, status, created_at, updated_at
		FROM dbo.workspace_rasters
		WHERE workspace_id = @p1
		ORDER BY created_at DESC;
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.WorkspaceRaster, 0)
	for rows.Next() {
		rst, err := scanWorkspaceRasterRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rst)
	}
	return out, rows.Err()
}

func (r *MSSQLWorkspaceRegistryRepository) DeleteWorkspaceRasterByLayer(ctx context.Context, workspaceID, layerID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM dbo.workspace_rasters WHERE workspace_id = @p1 AND workspace_layer_id = @p2;
	`, workspaceID, layerID)
	return err
}
