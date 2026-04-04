package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/its-ernest/rundown-workers/pkg/engine"
	_ "modernc.org/sqlite"
)

// Store defines the required operations for job persistence and lifecycle management.
type Store interface {
	// Enqueue adds a new job to the specified queue.
	Enqueue(queue, payload string, timeout, maxRetries int) (*engine.Job, error)

	// Poll picks the oldest pending job that is ready to run.
	Poll(queue string) (*engine.Job, error)
	// Complete marks a job as permanently finished.

	Complete(id string) error
	// Fail handles job failures by scheduling a retry or marking it as failed.

	Fail(id string) error
	// CleanupStale recovers jobs that have exceeded their timeout during execution.

	CleanupStale() (int64, error)
}

// SQLiteStore provides a lightweight persistent storage implementation using SQLite.
//
// It is designed to handle multiple worker processes safely using IMMEDIATE
// transactions for polling.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore initializes a new Store and runs all necessary schema migrations.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// 1. Concurrency Optimization: WAL Mode
	// This allows multiple readers (polling workers) while one writer is busy.
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	_, _ = db.Exec("PRAGMA synchronous=NORMAL;")
	_, _ = db.Exec("PRAGMA busy_timeout=5000;") // Wait up to 5s instead of failing immediately on lock

	// Clean full schema
	query := `
	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		queue TEXT NOT NULL,
		payload TEXT NOT NULL,
		status TEXT NOT NULL,
		retries INTEGER DEFAULT 0,
		max_retries INTEGER DEFAULT 3,
		timeout INTEGER DEFAULT 300,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		next_run_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_jobs_queue_status_next ON jobs(queue, status, next_run_at);
	`
	if _, err := db.Exec(query); err != nil {
		return nil, err
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Enqueue(queue, payload string, timeout, maxRetries int) (*engine.Job, error) {
	if timeout <= 0 {
		timeout = 300 // default 5 mins
	}
	if maxRetries < 0 {
		maxRetries = 3 // default
	}

	job := &engine.Job{
		ID:         uuid.New().String(),
		Queue:      queue,
		Payload:    payload,
		Status:     engine.StatusPending,
		Retries:    0,
		MaxRetries: maxRetries,
		Timeout:    timeout,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		NextRunAt:  time.Now(),
	}

	query := `INSERT INTO jobs (id, queue, payload, status, retries, max_retries, timeout, created_at, updated_at, next_run_at) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, job.ID, job.Queue, job.Payload, job.Status, job.Retries, job.MaxRetries, job.Timeout, job.CreatedAt, job.UpdatedAt, job.NextRunAt)
	if err != nil {
		return nil, err
	}

	return job, nil
}

func (s *SQLiteStore) Poll(queue string) (*engine.Job, error) {
	// 1. Start an IMMEDIATE transaction to prevent multiple workers from picking the same job.
	// This ensures that the SELECT and the subsequent UPDATE are atomic from the perspective of other writers.
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// In SQLite, BEGIN IMMEDIATE reserves the write lock.
	_, err = tx.Exec("BEGIN IMMEDIATE")
	if err != nil {
		return nil, err
	}

	var job engine.Job
	now := time.Now()
	// Only pick jobs that are PENDING and where next_run_at is in the past.
	query := `SELECT id, queue, payload, status, retries, max_retries, timeout, created_at, updated_at, next_run_at FROM jobs 
              WHERE queue = ? AND status = ? AND next_run_at <= ? ORDER BY created_at ASC LIMIT 1`
	err = tx.QueryRow(query, queue, engine.StatusPending, now).Scan(
		&job.ID, &job.Queue, &job.Payload, &job.Status, &job.Retries, &job.MaxRetries, &job.Timeout, &job.CreatedAt, &job.UpdatedAt, &job.NextRunAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No jobs available
		}
		return nil, err
	}

	// Mark as running
	updateQuery := `UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?`
	_, err = tx.Exec(updateQuery, engine.StatusRunning, now, job.ID)
	if err != nil {
		return nil, err
	}
	job.Status = engine.StatusRunning
	job.UpdatedAt = now

	return &job, tx.Commit()
}

func (s *SQLiteStore) Complete(id string) error {
	query := `UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, engine.StatusDone, time.Now(), id)
	return err
}

func (s *SQLiteStore) Fail(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var retries, maxRetries int
	err = tx.QueryRow("SELECT retries, max_retries FROM jobs WHERE id = ?", id).Scan(&retries, &maxRetries)
	if err != nil {
		return err
	}

	now := time.Now()
	if retries < maxRetries {
		// Schedule retry with exponential backoff (e.g., 5 * retry_count^2 seconds)
		backoff := 5 * (retries + 1) * (retries + 1)
		nextRun := now.Add(time.Duration(backoff) * time.Second)

		query := `UPDATE jobs SET status = ?, retries = retries + 1, updated_at = ?, next_run_at = ? WHERE id = ?`
		_, err = tx.Exec(query, engine.StatusPending, now, nextRun, id)
	} else {
		// Permanently fail
		query := `UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?`
		_, err = tx.Exec(query, engine.StatusFailed, now, id)
	}

	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) CleanupStale() (int64, error) {
	// Revert running jobs back to pending if they have been running longer than their timeout.
	// We use unixepoch to compare seconds (SQLite handles CURRENT_TIMESTAMP as UTC)
	now := time.Now()
	query := `UPDATE jobs 
	          SET status = ?, updated_at = ?, retries = retries + 1
			  WHERE status = ? 
			  AND (strftime('%s', ?) - strftime('%s', updated_at)) > timeout`

	res, err := s.db.Exec(query, engine.StatusPending, now, engine.StatusRunning, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
