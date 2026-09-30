package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"slimbox/internal/domain"
)

// GetByIDs returns a list of tasks matching the provided IDs.
func (r *TaskRepository) GetByIDs(ids []string) ([]*domain.Task, error) {
	if len(ids) == 0 {
		return []*domain.Task{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
	SELECT id, source_file_name, source_file_path, source_file_size,
		   output_file_name, output_file_path, output_file_size,
		   status, media_info_json, params_json, progress_json,
		   error_msg, download_count, priority, created_at, started_at, completed_at
	FROM tasks
	WHERE id IN (%s);
	`, strings.Join(placeholders, ","))

	rows, err := r.db.SQL.Query(query, args...)
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

// BatchDelete deletes multiple tasks by their IDs in a single transaction.
func (r *TaskRepository) BatchDelete(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM tasks WHERE id IN (%s);", strings.Join(placeholders, ","))
	_, err := r.db.SQL.Exec(query, args...)
	return err
}

// BatchReorder updates task priorities in a single transaction according to their order.
// Earlier tasks in orderedIDs receive higher priority values.
func (r *TaskRepository) BatchReorder(orderedIDs []string) error {
	if len(orderedIDs) == 0 {
		return nil
	}

	tx, err := r.db.SQL.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	stmt, err := tx.Prepare("UPDATE tasks SET priority = ? WHERE id = ?;")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	total := len(orderedIDs)
	for i, id := range orderedIDs {
		// Assign descending priorities so that index 0 has the highest priority
		priority := (total - i) * 10
		if _, err := stmt.Exec(priority, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// Helper to check if row scanning is needed
var _ = sql.ErrNoRows
