// Package store persists monitoring results in SQLite.
package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// Result statuses.
const (
	StatusOK    = "ok"
	StatusAlert = "alert"
	StatusError = "error"
)

type Result struct {
	ID        int64     `json:"id"`
	AppID     string    `json:"app_id"`
	Value     float64   `json:"value"`
	Total     float64   `json:"total"`
	Unit      string    `json:"unit"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	LatencyMs int64     `json:"latency_ms"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS results (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	app_id     TEXT    NOT NULL,
	value      REAL    NOT NULL DEFAULT 0,
	total      REAL    NOT NULL DEFAULT 0,
	unit       TEXT    NOT NULL DEFAULT '',
	status     TEXT    NOT NULL,
	message    TEXT    NOT NULL DEFAULT '',
	latency_ms INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_results_app_time ON results(app_id, created_at);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Insert(r Result) error {
	_, err := s.db.Exec(
		`INSERT INTO results (app_id, value, total, unit, status, message, latency_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.AppID, r.Value, r.Total, r.Unit, r.Status, r.Message, r.LatencyMs, r.CreatedAt.Unix(),
	)
	return err
}

func scan(rows *sql.Rows) ([]Result, error) {
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		var r Result
		var ts int64
		if err := rows.Scan(&r.ID, &r.AppID, &r.Value, &r.Total, &r.Unit, &r.Status, &r.Message, &r.LatencyMs, &ts); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(ts, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

const cols = `id, app_id, value, total, unit, status, message, latency_ms, created_at`

// Latest returns the most recent result per app.
func (s *Store) Latest() (map[string]Result, error) {
	rows, err := s.db.Query(`SELECT ` + cols + ` FROM results WHERE id IN (SELECT MAX(id) FROM results GROUP BY app_id)`)
	if err != nil {
		return nil, err
	}
	list, err := scan(rows)
	if err != nil {
		return nil, err
	}
	m := make(map[string]Result, len(list))
	for _, r := range list {
		m[r.AppID] = r
	}
	return m, nil
}

// History returns results for an app since the given time, oldest first.
func (s *Store) History(appID string, since time.Time) ([]Result, error) {
	rows, err := s.db.Query(
		`SELECT `+cols+` FROM results WHERE app_id = ? AND created_at >= ? ORDER BY created_at ASC`,
		appID, since.Unix(),
	)
	if err != nil {
		return nil, err
	}
	return scan(rows)
}

// Prune deletes results older than the given time.
func (s *Store) Prune(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM results WHERE created_at < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
