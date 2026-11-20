package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// ProcessingRepo defines the persistent storage contract for Phase 2 GIS Processing Engine tables.
type ProcessingRepo interface {
	// Jobs
	CreateJob(ctx context.Context, job *models.ProcessingJob) error
	GetJob(ctx context.Context, jobID string) (*models.ProcessingJob, error)
	ListJobs(ctx context.Context, userID, mapID string, limit int) ([]*models.ProcessingJob, error)
	UpdateJobStatus(ctx context.Context, jobID, status, stage string, progress int, errCode, errMsg string, outCfg map[string]interface{}) error

	// Job Logs, Inputs, Outputs
	AppendJobLog(ctx context.Context, jobID, stage, level, message string) error
	GetJobLogs(ctx context.Context, jobID string) ([]models.ProcessingJobLog, error)
	SaveJobInputs(ctx context.Context, jobID string, inputs []models.ProcessingJobInput) error
	SaveJobOutput(ctx context.Context, out models.ProcessingJobOutput) error
	GetJobOutputs(ctx context.Context, jobID string) ([]models.ProcessingJobOutput, error)

	// Topology Rules & Errors
	SaveTopologyRule(ctx context.Context, rule *models.TopologyRule) error
	GetTopologyRules(ctx context.Context, layerID string) ([]*models.TopologyRule, error)
	SaveTopologyErrors(ctx context.Context, jobID string, errs []*models.TopologyError) error
	GetTopologyErrors(ctx context.Context, jobID string) ([]*models.TopologyError, error)
	MarkTopologyErrorResolved(ctx context.Context, errorID string) error

	// Presets
	SavePreset(ctx context.Context, preset *models.ProcessingPreset) error
	ListPresets(ctx context.Context, userID, algorithmName string) ([]*models.ProcessingPreset, error)
	DeletePreset(ctx context.Context, presetID, userID string) error
}

// ============================================================================
// 1. IN-MEMORY PROCESSING REPOSITORY
// ============================================================================

type MemoryProcessingRepository struct {
	mu      sync.RWMutex
	jobs    map[string]*models.ProcessingJob
	logs    map[string][]models.ProcessingJobLog
	inputs  map[string][]models.ProcessingJobInput
	outputs map[string][]models.ProcessingJobOutput
	rules   map[string][]*models.TopologyRule
	errors  map[string][]*models.TopologyError
	presets map[string]*models.ProcessingPreset
	logSeq  int64
}

func NewMemoryProcessingRepository() *MemoryProcessingRepository {
	return &MemoryProcessingRepository{
		jobs:    make(map[string]*models.ProcessingJob),
		logs:    make(map[string][]models.ProcessingJobLog),
		inputs:  make(map[string][]models.ProcessingJobInput),
		outputs: make(map[string][]models.ProcessingJobOutput),
		rules:   make(map[string][]*models.TopologyRule),
		errors:  make(map[string][]*models.TopologyError),
		presets: make(map[string]*models.ProcessingPreset),
	}
}

func (r *MemoryProcessingRepository) CreateJob(ctx context.Context, job *models.ProcessingJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if job.JobID == "" {
		job.JobID = generateGISID("job", job.UserID+job.AlgorithmName)
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	cp := *job
	r.jobs[job.JobID] = &cp
	if len(job.Inputs) > 0 {
		r.inputs[job.JobID] = append([]models.ProcessingJobInput{}, job.Inputs...)
	}
	return nil
}

func (r *MemoryProcessingRepository) GetJob(ctx context.Context, jobID string) (*models.ProcessingJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	j, ok := r.jobs[jobID]
	if !ok {
		return nil, models.ErrNotFound
	}
	cp := *j
	cp.Inputs = append([]models.ProcessingJobInput{}, r.inputs[jobID]...)
	cp.Outputs = append([]models.ProcessingJobOutput{}, r.outputs[jobID]...)
	return &cp, nil
}

func (r *MemoryProcessingRepository) ListJobs(ctx context.Context, userID, mapID string, limit int) ([]*models.ProcessingJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []*models.ProcessingJob
	for _, j := range r.jobs {
		if userID != "" && j.UserID != userID {
			continue
		}
		if mapID != "" && j.MapID != mapID {
			continue
		}
		cp := *j
		cp.Inputs = append([]models.ProcessingJobInput{}, r.inputs[j.JobID]...)
		cp.Outputs = append([]models.ProcessingJobOutput{}, r.outputs[j.JobID]...)
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (r *MemoryProcessingRepository) UpdateJobStatus(ctx context.Context, jobID, status, stage string, progress int, errCode, errMsg string, outCfg map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[jobID]
	if !ok {
		return models.ErrNotFound
	}
	now := time.Now().UTC()
	if status != "" {
		j.Status = status
	}
	if stage != "" {
		j.Stage = stage
	}
	if progress >= 0 {
		j.Progress = progress
	}
	if status == models.JobStatusRunning && j.StartedAt == nil {
		j.StartedAt = &now
	}
	if status == models.JobStatusCompleted || status == models.JobStatusFailed || status == models.JobStatusCancelled {
		j.CompletedAt = &now
	}
	if errCode != "" || errMsg != "" {
		j.ErrorCode = errCode
		j.ErrorMessage = errMsg
	}
	if outCfg != nil {
		j.OutputConfig = outCfg
	}
	return nil
}

func (r *MemoryProcessingRepository) AppendJobLog(ctx context.Context, jobID, stage, level, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logSeq++
	if level == "" {
		level = "INFO"
	}
	r.logs[jobID] = append(r.logs[jobID], models.ProcessingJobLog{
		LogID:     r.logSeq,
		JobID:     jobID,
		Stage:     stage,
		LogLevel:  level,
		Message:   message,
		CreatedAt: time.Now().UTC(),
	})
	return nil
}

func (r *MemoryProcessingRepository) GetJobLogs(ctx context.Context, jobID string) ([]models.ProcessingJobLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]models.ProcessingJobLog{}, r.logs[jobID]...)
	return out, nil
}

func (r *MemoryProcessingRepository) SaveJobInputs(ctx context.Context, jobID string, inputs []models.ProcessingJobInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inputs[jobID] = append([]models.ProcessingJobInput{}, inputs...)
	return nil
}

func (r *MemoryProcessingRepository) SaveJobOutput(ctx context.Context, out models.ProcessingJobOutput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	out.OutputID = int64(len(r.outputs[out.JobID]) + 1)
	r.outputs[out.JobID] = append(r.outputs[out.JobID], out)
	return nil
}

func (r *MemoryProcessingRepository) GetJobOutputs(ctx context.Context, jobID string) ([]models.ProcessingJobOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]models.ProcessingJobOutput{}, r.outputs[jobID]...), nil
}

func (r *MemoryProcessingRepository) SaveTopologyRule(ctx context.Context, rule *models.TopologyRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rule.RuleID == "" {
		rule.RuleID = generateGISID("trule", rule.LayerID+rule.RuleType)
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	cp := *rule
	r.rules[rule.LayerID] = append(r.rules[rule.LayerID], &cp)
	return nil
}

func (r *MemoryProcessingRepository) GetTopologyRules(ctx context.Context, layerID string) ([]*models.TopologyRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]*models.TopologyRule{}, r.rules[layerID]...), nil
}

func (r *MemoryProcessingRepository) SaveTopologyErrors(ctx context.Context, jobID string, errs []*models.TopologyError) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []*models.TopologyError
	for _, e := range errs {
		cp := *e
		list = append(list, &cp)
	}
	r.errors[jobID] = list
	return nil
}

func (r *MemoryProcessingRepository) GetTopologyErrors(ctx context.Context, jobID string) ([]*models.TopologyError, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*models.TopologyError
	for _, e := range r.errors[jobID] {
		cp := *e
		out = append(out, &cp)
	}
	return out, nil
}

func (r *MemoryProcessingRepository) MarkTopologyErrorResolved(ctx context.Context, errorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, list := range r.errors {
		for _, e := range list {
			if e.ErrorID == errorID {
				e.IsResolved = true
				return nil
			}
		}
	}
	return models.ErrNotFound
}

func (r *MemoryProcessingRepository) SavePreset(ctx context.Context, preset *models.ProcessingPreset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if preset.PresetID == "" {
		preset.PresetID = generateGISID("preset", preset.UserID+preset.AlgorithmName+preset.PresetName)
	}
	if preset.CreatedAt.IsZero() {
		preset.CreatedAt = time.Now().UTC()
	}
	cp := *preset
	r.presets[preset.PresetID] = &cp
	return nil
}

func (r *MemoryProcessingRepository) ListPresets(ctx context.Context, userID, algorithmName string) ([]*models.ProcessingPreset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*models.ProcessingPreset
	for _, p := range r.presets {
		if userID != "" && p.UserID != userID {
			continue
		}
		if algorithmName != "" && p.AlgorithmName != algorithmName {
			continue
		}
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}

func (r *MemoryProcessingRepository) DeletePreset(ctx context.Context, presetID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.presets[presetID]
	if !ok {
		return models.ErrNotFound
	}
	if userID != "" && p.UserID != userID {
		return models.ErrMapAccessDenied
	}
	delete(r.presets, presetID)
	return nil
}

// ============================================================================
// 2. MSSQL PROCESSING REPOSITORY
// ============================================================================

type MSSQLProcessingRepository struct {
	db  *sql.DB
	mem *MemoryProcessingRepository
}

func NewMSSQLProcessingRepository(db *sql.DB) *MSSQLProcessingRepository {
	return &MSSQLProcessingRepository{
		db:  db,
		mem: NewMemoryProcessingRepository(),
	}
}

func (r *MSSQLProcessingRepository) CreateJob(ctx context.Context, job *models.ProcessingJob) error {
	if job.JobID == "" {
		job.JobID = generateGISID("job", job.UserID+job.AlgorithmName)
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	_ = r.mem.CreateJob(ctx, job)
	if r.db == nil {
		return nil
	}
	inBytes, _ := json.Marshal(job.InputConfig)
	outBytes, _ := json.Marshal(job.OutputConfig)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.processing_jobs (
			job_id, project_id, map_id, user_id, algorithm_name, status, progress, stage,
			input_config, output_config, error_code, error_message, created_at
		) VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12, @p13)
	`, job.JobID, nullStr(job.ProjectID), nullStr(job.MapID), job.UserID, job.AlgorithmName,
		job.Status, job.Progress, job.Stage, string(inBytes), string(outBytes),
		nullStr(job.ErrorCode), nullStr(job.ErrorMessage), job.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed inserting processing_job: %w", err)
	}
	if len(job.Inputs) > 0 {
		_ = r.SaveJobInputs(ctx, job.JobID, job.Inputs)
	}
	return nil
}

func (r *MSSQLProcessingRepository) GetJob(ctx context.Context, jobID string) (*models.ProcessingJob, error) {
	if r.db == nil {
		return r.mem.GetJob(ctx, jobID)
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT job_id, ISNULL(project_id,''), ISNULL(map_id,''), user_id, algorithm_name,
		       status, progress, ISNULL(stage,''), ISNULL(input_config,'{}'), ISNULL(output_config,'{}'),
		       ISNULL(error_code,''), ISNULL(error_message,''), created_at, started_at, completed_at
		FROM dbo.processing_jobs WHERE job_id = @p1
	`, jobID)
	var j models.ProcessingJob
	var inStr, outStr string
	var started, completed sql.NullTime
	if err := row.Scan(&j.JobID, &j.ProjectID, &j.MapID, &j.UserID, &j.AlgorithmName,
		&j.Status, &j.Progress, &j.Stage, &inStr, &outStr, &j.ErrorCode, &j.ErrorMessage,
		&j.CreatedAt, &started, &completed); err != nil {
		return r.mem.GetJob(ctx, jobID)
	}
	_ = json.Unmarshal([]byte(inStr), &j.InputConfig)
	_ = json.Unmarshal([]byte(outStr), &j.OutputConfig)
	if started.Valid {
		j.StartedAt = &started.Time
	}
	if completed.Valid {
		j.CompletedAt = &completed.Time
	}
	j.Outputs, _ = r.GetJobOutputs(ctx, jobID)
	return &j, nil
}

func (r *MSSQLProcessingRepository) ListJobs(ctx context.Context, userID, mapID string, limit int) ([]*models.ProcessingJob, error) {
	if r.db == nil {
		return r.mem.ListJobs(ctx, userID, mapID, limit)
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT TOP (@p1) job_id, ISNULL(project_id,''), ISNULL(map_id,''), user_id, algorithm_name,
		       status, progress, ISNULL(stage,''), ISNULL(input_config,'{}'), ISNULL(output_config,'{}'),
		       ISNULL(error_code,''), ISNULL(error_message,''), created_at, started_at, completed_at
		FROM dbo.processing_jobs
		WHERE (@p2 = '' OR user_id = @p2) AND (@p3 = '' OR map_id = @p3)
		ORDER BY created_at DESC
	`, limit, userID, mapID)
	if err != nil {
		return r.mem.ListJobs(ctx, userID, mapID, limit)
	}
	defer rows.Close()
	var list []*models.ProcessingJob
	for rows.Next() {
		var j models.ProcessingJob
		var inStr, outStr string
		var started, completed sql.NullTime
		if err := rows.Scan(&j.JobID, &j.ProjectID, &j.MapID, &j.UserID, &j.AlgorithmName,
			&j.Status, &j.Progress, &j.Stage, &inStr, &outStr, &j.ErrorCode, &j.ErrorMessage,
			&j.CreatedAt, &started, &completed); err == nil {
			_ = json.Unmarshal([]byte(inStr), &j.InputConfig)
			_ = json.Unmarshal([]byte(outStr), &j.OutputConfig)
			if started.Valid {
				j.StartedAt = &started.Time
			}
			if completed.Valid {
				j.CompletedAt = &completed.Time
			}
			j.Outputs, _ = r.GetJobOutputs(ctx, j.JobID)
			list = append(list, &j)
		}
	}
	return list, rows.Err()
}

func (r *MSSQLProcessingRepository) UpdateJobStatus(ctx context.Context, jobID, status, stage string, progress int, errCode, errMsg string, outCfg map[string]interface{}) error {
	_ = r.mem.UpdateJobStatus(ctx, jobID, status, stage, progress, errCode, errMsg, outCfg)
	if r.db == nil {
		return nil
	}
	outStr := ""
	if outCfg != nil {
		b, _ := json.Marshal(outCfg)
		outStr = string(b)
	}
	now := time.Now().UTC()
	var completedAt interface{}
	if status == models.JobStatusCompleted || status == models.JobStatusFailed || status == models.JobStatusCancelled {
		completedAt = now
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE dbo.processing_jobs
		SET status = CASE WHEN @p2 <> '' THEN @p2 ELSE status END,
		    stage = CASE WHEN @p3 <> '' THEN @p3 ELSE stage END,
		    progress = CASE WHEN @p4 >= 0 THEN @p4 ELSE progress END,
		    error_code = CASE WHEN @p5 <> '' THEN @p5 ELSE error_code END,
		    error_message = CASE WHEN @p6 <> '' THEN @p6 ELSE error_message END,
		    output_config = CASE WHEN @p7 <> '' THEN @p7 ELSE output_config END,
		    started_at = CASE WHEN @p2 = 'running' AND started_at IS NULL THEN @p8 ELSE started_at END,
		    completed_at = COALESCE(@p9, completed_at)
		WHERE job_id = @p1
	`, jobID, status, stage, progress, errCode, errMsg, outStr, now, completedAt)
	return err
}

func (r *MSSQLProcessingRepository) AppendJobLog(ctx context.Context, jobID, stage, level, message string) error {
	_ = r.mem.AppendJobLog(ctx, jobID, stage, level, message)
	if r.db == nil {
		return nil
	}
	if level == "" {
		level = "INFO"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.processing_job_logs (job_id, stage, log_level, message, created_at)
		VALUES (@p1, @p2, @p3, @p4, SYSUTCDATETIME())
	`, jobID, stage, level, message)
	return err
}

func (r *MSSQLProcessingRepository) GetJobLogs(ctx context.Context, jobID string) ([]models.ProcessingJobLog, error) {
	if r.db == nil {
		return r.mem.GetJobLogs(ctx, jobID)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT log_id, job_id, stage, log_level, message, created_at
		FROM dbo.processing_job_logs WHERE job_id = @p1 ORDER BY log_id ASC
	`, jobID)
	if err != nil {
		return r.mem.GetJobLogs(ctx, jobID)
	}
	defer rows.Close()
	var logs []models.ProcessingJobLog
	for rows.Next() {
		var l models.ProcessingJobLog
		if err := rows.Scan(&l.LogID, &l.JobID, &l.Stage, &l.LogLevel, &l.Message, &l.CreatedAt); err == nil {
			logs = append(logs, l)
		}
	}
	return logs, rows.Err()
}

func (r *MSSQLProcessingRepository) SaveJobInputs(ctx context.Context, jobID string, inputs []models.ProcessingJobInput) error {
	_ = r.mem.SaveJobInputs(ctx, jobID, inputs)
	if r.db == nil {
		return nil
	}
	for _, in := range inputs {
		_, _ = r.db.ExecContext(ctx, `
			INSERT INTO dbo.processing_job_inputs (job_id, dataset_id, layer_id, input_role)
			VALUES (@p1, @p2, @p3, @p4)
		`, jobID, nullStr(in.DatasetID), nullStr(in.LayerID), in.InputRole)
	}
	return nil
}

func (r *MSSQLProcessingRepository) SaveJobOutput(ctx context.Context, out models.ProcessingJobOutput) error {
	_ = r.mem.SaveJobOutput(ctx, out)
	if r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.processing_job_outputs (job_id, dataset_id, layer_id, output_name, storage_path, output_format, created_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, SYSUTCDATETIME())
	`, out.JobID, nullStr(out.DatasetID), nullStr(out.LayerID), out.OutputName, nullStr(out.StoragePath), out.OutputFormat)
	return err
}

func (r *MSSQLProcessingRepository) GetJobOutputs(ctx context.Context, jobID string) ([]models.ProcessingJobOutput, error) {
	if r.db == nil {
		return r.mem.GetJobOutputs(ctx, jobID)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT output_id, job_id, ISNULL(dataset_id,''), ISNULL(layer_id,''), output_name, ISNULL(storage_path,''), output_format, created_at
		FROM dbo.processing_job_outputs WHERE job_id = @p1 ORDER BY output_id ASC
	`, jobID)
	if err != nil {
		return r.mem.GetJobOutputs(ctx, jobID)
	}
	defer rows.Close()
	var list []models.ProcessingJobOutput
	for rows.Next() {
		var o models.ProcessingJobOutput
		if err := rows.Scan(&o.OutputID, &o.JobID, &o.DatasetID, &o.LayerID, &o.OutputName, &o.StoragePath, &o.OutputFormat, &o.CreatedAt); err == nil {
			list = append(list, o)
		}
	}
	return list, rows.Err()
}

func (r *MSSQLProcessingRepository) SaveTopologyRule(ctx context.Context, rule *models.TopologyRule) error {
	return r.mem.SaveTopologyRule(ctx, rule)
}

func (r *MSSQLProcessingRepository) GetTopologyRules(ctx context.Context, layerID string) ([]*models.TopologyRule, error) {
	return r.mem.GetTopologyRules(ctx, layerID)
}

func (r *MSSQLProcessingRepository) SaveTopologyErrors(ctx context.Context, jobID string, errs []*models.TopologyError) error {
	_ = r.mem.SaveTopologyErrors(ctx, jobID, errs)
	if r.db == nil {
		return nil
	}
	for _, e := range errs {
		geomJSON := ""
		if e.Geometry != nil {
			b, _ := json.Marshal(e.Geometry)
			geomJSON = string(b)
		}
		_, _ = r.db.ExecContext(ctx, `
			INSERT INTO dbo.topology_errors (
				error_id, job_id, layer_id, feature_id, error_type, error_message,
				geometry, geom_wkt, geom_json, suggestion, is_resolved, created_at
			) VALUES (
				@p1, @p2, @p3, @p4, @p5, @p6,
				CASE WHEN @p7 <> '' THEN geometry::STGeomFromText(@p7, 4326).MakeValid() ELSE NULL END,
				@p7, @p8, @p9, @p10, SYSUTCDATETIME()
			)
		`, e.ErrorID, jobID, e.LayerID, nullStr(e.FeatureID), e.ErrorType, e.ErrorMessage,
			e.GeomWKT, geomJSON, e.Suggestion, e.IsResolved)
	}
	return nil
}

func (r *MSSQLProcessingRepository) GetTopologyErrors(ctx context.Context, jobID string) ([]*models.TopologyError, error) {
	if r.db == nil {
		return r.mem.GetTopologyErrors(ctx, jobID)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT error_id, job_id, layer_id, ISNULL(feature_id,''), error_type, error_message,
		       ISNULL(geom_json,''), ISNULL(suggestion,''), is_resolved, created_at
		FROM dbo.topology_errors WHERE job_id = @p1
	`, jobID)
	if err != nil {
		return r.mem.GetTopologyErrors(ctx, jobID)
	}
	defer rows.Close()
	var list []*models.TopologyError
	for rows.Next() {
		var e models.TopologyError
		var gStr string
		if err := rows.Scan(&e.ErrorID, &e.JobID, &e.LayerID, &e.FeatureID, &e.ErrorType, &e.ErrorMessage,
			&gStr, &e.Suggestion, &e.IsResolved, &e.CreatedAt); err == nil {
			if gStr != "" {
				var g models.GeoJSONGeometry
				if json.Unmarshal([]byte(gStr), &g) == nil {
					e.Geometry = &g
				}
			}
			list = append(list, &e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return r.mem.GetTopologyErrors(ctx, jobID)
	}
	return list, nil
}

func (r *MSSQLProcessingRepository) MarkTopologyErrorResolved(ctx context.Context, errorID string) error {
	_ = r.mem.MarkTopologyErrorResolved(ctx, errorID)
	if r.db != nil {
		_, _ = r.db.ExecContext(ctx, `UPDATE dbo.topology_errors SET is_resolved = 1 WHERE error_id = @p1`, errorID)
	}
	return nil
}

func (r *MSSQLProcessingRepository) SavePreset(ctx context.Context, preset *models.ProcessingPreset) error {
	_ = r.mem.SavePreset(ctx, preset)
	if r.db != nil {
		pBytes, _ := json.Marshal(preset.Parameters)
		_, _ = r.db.ExecContext(ctx, `
			INSERT INTO dbo.processing_presets (preset_id, project_id, map_id, user_id, algorithm_name, preset_name, parameters, created_at)
			VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, SYSUTCDATETIME())
		`, preset.PresetID, nullStr(preset.ProjectID), nullStr(preset.MapID), preset.UserID, preset.AlgorithmName, preset.PresetName, string(pBytes))
	}
	return nil
}

func (r *MSSQLProcessingRepository) ListPresets(ctx context.Context, userID, algorithmName string) ([]*models.ProcessingPreset, error) {
	return r.mem.ListPresets(ctx, userID, algorithmName)
}

func (r *MSSQLProcessingRepository) DeletePreset(ctx context.Context, presetID, userID string) error {
	if r.db != nil {
		_, _ = r.db.ExecContext(ctx, `DELETE FROM dbo.processing_presets WHERE preset_id = @p1 AND user_id = @p2`, presetID, userID)
	}
	return r.mem.DeletePreset(ctx, presetID, userID)
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
