package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"slimbox/internal/domain"
)

type TaskRepository struct {
	db *DB
}

func NewTaskRepository(db *DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(task *domain.Task) error {
	mediaJSON, _ := json.Marshal(task.MediaInfo)
	paramsJSON, _ := json.Marshal(task.Params)
	progJSON, _ := json.Marshal(task.Progress)

	query := `
	INSERT INTO tasks (
		id, source_file_name, source_file_path, source_file_size,
		output_file_name, output_file_path, output_file_size,
		status, media_info_json, params_json, progress_json,
		error_msg, download_count, priority, created_at, started_at, completed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := r.db.SQL.Exec(
		query,
		task.ID, task.SourceFileName, task.SourceFilePath, task.SourceFileSize,
		task.OutputFileName, task.OutputFilePath, task.OutputFileSize,
		task.Status, string(mediaJSON), string(paramsJSON), string(progJSON),
		task.ErrorMsg, task.DownloadCount, task.Priority, task.CreatedAt, task.StartedAt, task.CompletedAt,
	)
	return err
}

func (r *TaskRepository) GetByID(id string) (*domain.Task, error) {
	query := `
	SELECT id, source_file_name, source_file_path, source_file_size,
		   output_file_name, output_file_path, output_file_size,
		   status, media_info_json, params_json, progress_json,
		   error_msg, download_count, priority, created_at, started_at, completed_at
	FROM tasks WHERE id = ?;
	`
	row := r.db.SQL.QueryRow(query, id)
	return r.scanTask(row)
}

func (r *TaskRepository) GetNextQueued() (*domain.Task, error) {
	query := `
	SELECT id, source_file_name, source_file_path, source_file_size,
		   output_file_name, output_file_path, output_file_size,
		   status, media_info_json, params_json, progress_json,
		   error_msg, download_count, priority, created_at, started_at, completed_at
	FROM tasks
	WHERE status = ?
	ORDER BY priority DESC, created_at ASC
	LIMIT 1;
	`
	row := r.db.SQL.QueryRow(query, domain.StatusQueued)
	task, err := r.scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return task, err
}

func (r *TaskRepository) Update(task *domain.Task) error {
	mediaJSON, _ := json.Marshal(task.MediaInfo)
	paramsJSON, _ := json.Marshal(task.Params)
	progJSON, _ := json.Marshal(task.Progress)

	query := `
	UPDATE tasks SET
		output_file_name = ?, output_file_path = ?, output_file_size = ?,
		status = ?, media_info_json = ?, params_json = ?, progress_json = ?,
		error_msg = ?, download_count = ?, priority = ?,
		started_at = CASE
			WHEN ? IS NOT NULL THEN ?
			WHEN ? = 'queued' THEN NULL
			ELSE started_at
		END,
		completed_at = ?
	WHERE id = ?;
	`
	_, err := r.db.SQL.Exec(
		query,
		task.OutputFileName, task.OutputFilePath, task.OutputFileSize,
		task.Status, string(mediaJSON), string(paramsJSON), string(progJSON),
		task.ErrorMsg, task.DownloadCount, task.Priority,
		task.StartedAt, task.StartedAt, string(task.Status),
		task.CompletedAt,
		task.ID,
	)
	return err
}

func (r *TaskRepository) UpdateProgress(id string, progress domain.TaskProgress) error {
	progJSON, _ := json.Marshal(progress)
	query := `UPDATE tasks SET progress_json = ? WHERE id = ?;`
	_, err := r.db.SQL.Exec(query, string(progJSON), id)
	return err
}

func (r *TaskRepository) UpdateStatus(id string, status domain.TaskStatus, errorMsg string) error {
	var query string
	now := time.Now()
	switch status {
	case domain.StatusTranscoding:
		query = `UPDATE tasks SET status = ?, started_at = ?, error_msg = ? WHERE id = ?;`
		_, err := r.db.SQL.Exec(query, status, now, errorMsg, id)
		return err
	case domain.StatusCompleted, domain.StatusFailed, domain.StatusAborted:
		query = `UPDATE tasks SET status = ?, completed_at = ?, error_msg = ? WHERE id = ?;`
		_, err := r.db.SQL.Exec(query, status, now, errorMsg, id)
		return err
	default:
		query = `UPDATE tasks SET status = ?, error_msg = ? WHERE id = ?;`
		_, err := r.db.SQL.Exec(query, status, errorMsg, id)
		return err
	}
}

func (r *TaskRepository) IncrementDownloadCount(id string) error {
	query := `UPDATE tasks SET download_count = download_count + 1 WHERE id = ?;`
	_, err := r.db.SQL.Exec(query, id)
	return err
}

func (r *TaskRepository) Delete(id string) error {
	query := `DELETE FROM tasks WHERE id = ?;`
	_, err := r.db.SQL.Exec(query, id)
	return err
}

func (r *TaskRepository) UpdatePriority(id string, priority int) error {
	query := `UPDATE tasks SET priority = ? WHERE id = ?;`
	_, err := r.db.SQL.Exec(query, priority, id)
	return err
}

func (r *TaskRepository) GetQueuedTasks() ([]*domain.Task, error) {
	query := `
	SELECT id, source_file_name, source_file_path, source_file_size,
		   output_file_name, output_file_path, output_file_size,
		   status, media_info_json, params_json, progress_json,
		   error_msg, download_count, priority, created_at, started_at, completed_at
	FROM tasks
	WHERE status IN (?, ?, ?)
	ORDER BY priority DESC, created_at ASC;
	`
	rows, err := r.db.SQL.Query(query, domain.StatusQueued, domain.StatusPending, domain.StatusPaused)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := r.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (r *TaskRepository) List(statusFilter string, limit, offset int) ([]*domain.Task, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var countQuery string
	var listQuery string
	var args []interface{}

	if statusFilter != "" {
		countQuery = `SELECT COUNT(*) FROM tasks WHERE status = ?;`
		orderBy := "created_at DESC"
		if statusFilter == string(domain.StatusQueued) || statusFilter == string(domain.StatusPending) || statusFilter == string(domain.StatusPaused) {
			orderBy = "priority DESC, created_at ASC"
		}
		listQuery = fmt.Sprintf(`
		SELECT id, source_file_name, source_file_path, source_file_size,
			   output_file_name, output_file_path, output_file_size,
			   status, media_info_json, params_json, progress_json,
			   error_msg, download_count, priority, created_at, started_at, completed_at
		FROM tasks
		WHERE status = ?
		ORDER BY %s
		LIMIT ? OFFSET ?;
		`, orderBy)
		args = append(args, statusFilter)
	} else {
		countQuery = `SELECT COUNT(*) FROM tasks;`
		listQuery = `
		SELECT id, source_file_name, source_file_path, source_file_size,
			   output_file_name, output_file_path, output_file_size,
			   status, media_info_json, params_json, progress_json,
			   error_msg, download_count, priority, created_at, started_at, completed_at
		FROM tasks
		ORDER BY 
			CASE 
				WHEN status = 'transcoding' THEN 0 
				WHEN status = 'queued' THEN 1 
				WHEN status = 'pending' THEN 2 
				WHEN status = 'paused' THEN 3
				ELSE 4 
			END ASC,
			priority DESC, 
			created_at ASC
		LIMIT ? OFFSET ?;
		`
	}

	var total int
	if statusFilter != "" {
		if err := r.db.SQL.QueryRow(countQuery, statusFilter).Scan(&total); err != nil {
			return nil, 0, err
		}
	} else {
		if err := r.db.SQL.QueryRow(countQuery).Scan(&total); err != nil {
			return nil, 0, err
		}
	}

	listArgs := append(args, limit, offset)
	rows, err := r.db.SQL.Query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := r.scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, task)
	}
	return tasks, total, nil
}

func (r *TaskRepository) GetExpiredCompleted(retentionHours int) ([]*domain.Task, error) {
	if retentionHours <= 0 {
		return nil, nil
	}
	cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
	query := `
	SELECT id, source_file_name, source_file_path, source_file_size,
		   output_file_name, output_file_path, output_file_size,
		   status, media_info_json, params_json, progress_json,
		   error_msg, download_count, priority, created_at, started_at, completed_at
	FROM tasks
	WHERE status = ? AND completed_at IS NOT NULL AND completed_at < ?;
	`
	rows, err := r.db.SQL.Query(query, domain.StatusCompleted, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := r.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func (r *TaskRepository) scanTask(s scannable) (*domain.Task, error) {
	var task domain.Task
	var mediaJSON, paramsJSON, progJSON string
	var startedAt, completedAt sql.NullTime

	err := s.Scan(
		&task.ID, &task.SourceFileName, &task.SourceFilePath, &task.SourceFileSize,
		&task.OutputFileName, &task.OutputFilePath, &task.OutputFileSize,
		&task.Status, &mediaJSON, &paramsJSON, &progJSON,
		&task.ErrorMsg, &task.DownloadCount, &task.Priority, &task.CreatedAt,
		&startedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}

	if mediaJSON != "" && mediaJSON != "null" {
		var info domain.MediaInfo
		if err := json.Unmarshal([]byte(mediaJSON), &info); err == nil {
			task.MediaInfo = &info
		}
	}
	if paramsJSON != "" && paramsJSON != "null" {
		json.Unmarshal([]byte(paramsJSON), &task.Params)
	}
	if progJSON != "" && progJSON != "null" {
		json.Unmarshal([]byte(progJSON), &task.Progress)
	}

	return &task, nil
}

// GetAllTrackedFilePaths returns a normalized set of all source and output file paths tracked by tasks.
func (r *TaskRepository) GetAllTrackedFilePaths() (map[string]struct{}, error) {
	query := `SELECT source_file_path, output_file_path FROM tasks;`
	rows, err := r.db.SQL.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]struct{})
	for rows.Next() {
		var src, out sql.NullString
		if err := rows.Scan(&src, &out); err != nil {
			return nil, err
		}
		if src.Valid && src.String != "" {
			paths[filepath.Clean(src.String)] = struct{}{}
		}
		if out.Valid && out.String != "" {
			paths[filepath.Clean(out.String)] = struct{}{}
		}
	}
	return paths, nil
}

// GetStalePendingTasks retrieves tasks in pending status that were created before the cutoff duration.
func (r *TaskRepository) GetStalePendingTasks(staleDuration time.Duration) ([]*domain.Task, error) {
	if staleDuration <= 0 {
		return nil, nil
	}
	cutoff := time.Now().Add(-staleDuration)
	query := `
	SELECT id, source_file_name, source_file_path, source_file_size,
		   output_file_name, output_file_path, output_file_size,
		   status, media_info_json, params_json, progress_json,
		   error_msg, download_count, priority, created_at, started_at, completed_at
	FROM tasks
	WHERE status = ? AND created_at < ?;
	`
	rows, err := r.db.SQL.Query(query, domain.StatusPending, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := r.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

