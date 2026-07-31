package usage

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ktsoator/or/llm"
	_ "modernc.org/sqlite"
)

const (
	ledgerSchemaVersion = 1
	maxLedgerLineBytes  = 2 * 1024 * 1024
)

const createLedgerSchema = `
CREATE TABLE IF NOT EXISTS usage_events (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	response_model TEXT NOT NULL,
	response_id TEXT NOT NULL,
	occurred_at TEXT NOT NULL,
	occurred_at_seconds INTEGER NOT NULL,
	occurred_at_nanos INTEGER NOT NULL,
	input_tokens INTEGER NOT NULL,
	input_unknown INTEGER NOT NULL,
	output_tokens INTEGER NOT NULL,
	cache_read_tokens INTEGER NOT NULL,
	cache_write_tokens INTEGER NOT NULL,
	total_tokens INTEGER NOT NULL,
	cost_input REAL NOT NULL,
	cost_output REAL NOT NULL,
	cost_cache_read REAL NOT NULL,
	cost_cache_write REAL NOT NULL,
	cost_total REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS usage_events_occurred
	ON usage_events (occurred_at_seconds DESC, occurred_at_nanos DESC, id DESC);
CREATE INDEX IF NOT EXISTS usage_events_provider_occurred
	ON usage_events (provider, occurred_at_seconds DESC, occurred_at_nanos DESC, id DESC);
CREATE INDEX IF NOT EXISTS usage_events_model_occurred
	ON usage_events (model, occurred_at_seconds DESC, occurred_at_nanos DESC, id DESC);
CREATE INDEX IF NOT EXISTS usage_events_provider_model_occurred
	ON usage_events (
		provider, model, occurred_at_seconds DESC, occurred_at_nanos DESC, id DESC
	);
CREATE TABLE IF NOT EXISTS usage_metadata (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

const insertEventSQL = `
INSERT OR IGNORE INTO usage_events (
	id, session_id, provider, model, response_model, response_id,
	occurred_at, occurred_at_seconds, occurred_at_nanos,
	input_tokens, input_unknown, output_tokens, cache_read_tokens,
	cache_write_tokens, total_tokens,
	cost_input, cost_output, cost_cache_read, cost_cache_write, cost_total
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const replaceEventSQL = `
INSERT OR REPLACE INTO usage_events (
	id, session_id, provider, model, response_model, response_id,
	occurred_at, occurred_at_seconds, occurred_at_nanos,
	input_tokens, input_unknown, output_tokens, cache_read_tokens,
	cache_write_tokens, total_tokens,
	cost_input, cost_output, cost_cache_read, cost_cache_write, cost_total
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

// Store is a disk-backed, append-only, deduplicated usage ledger. SQLite owns
// event indexes and aggregation so process memory does not grow with history.
type Store struct {
	db *sql.DB
}

// NewStore opens the SQLite ledger derived from legacyPath. Existing JSONL
// data is imported transactionally and retained as a recovery source.
func NewStore(legacyPath string) (*Store, error) {
	dbPath := databasePath(legacyPath)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("usage: create ledger directory: %w", err)
	}
	if err := createPrivateFile(dbPath); err != nil {
		return nil, err
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("usage: secure database permissions: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("usage: open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrateJSONL(legacyPath, dbPath); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the database connection. Call it after every conversation
// manager using the ledger has stopped.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func databasePath(legacyPath string) string {
	if strings.EqualFold(filepath.Ext(legacyPath), ".jsonl") {
		return strings.TrimSuffix(legacyPath, filepath.Ext(legacyPath)) + ".sqlite"
	}
	return legacyPath
}

func createPrivateFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("usage: create database: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("usage: close new database: %w", err)
	}
	return nil
}

func (s *Store) initialize() error {
	if _, err := s.db.Exec(`
		PRAGMA busy_timeout = 5000;
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
	`); err != nil {
		return fmt.Errorf("usage: configure database: %w", err)
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("usage: read database schema version: %w", err)
	}
	if version > ledgerSchemaVersion {
		return fmt.Errorf(
			"usage: database schema version %d is newer than supported version %d",
			version,
			ledgerSchemaVersion,
		)
	}
	if _, err := s.db.Exec(createLedgerSchema); err != nil {
		return fmt.Errorf("usage: create database schema: %w", err)
	}
	if version < ledgerSchemaVersion {
		if _, err := s.db.Exec("PRAGMA user_version = " + strconv.Itoa(ledgerSchemaVersion)); err != nil {
			return fmt.Errorf("usage: set database schema version: %w", err)
		}
	}
	return nil
}

func (s *Store) migrateJSONL(legacyPath, dbPath string) error {
	if filepath.Clean(legacyPath) == filepath.Clean(dbPath) {
		return nil
	}
	info, err := os.Stat(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("usage: stat legacy ledger: %w", err)
	}
	state := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
	var previous string
	err = s.db.QueryRow(
		"SELECT value FROM usage_metadata WHERE key = 'legacy_jsonl_state'",
	).Scan(&previous)
	switch {
	case err == nil && previous == state:
		return nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("usage: read migration state: %w", err)
	}
	firstMigration := errors.Is(err, sql.ErrNoRows)
	if firstMigration {
		var eventCount int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM usage_events").Scan(&eventCount); err != nil {
			return fmt.Errorf("usage: count events before migration: %w", err)
		}
		firstMigration = eventCount == 0
	}

	file, err := os.Open(legacyPath)
	if err != nil {
		return fmt.Errorf("usage: open legacy ledger: %w", err)
	}
	defer file.Close()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("usage: begin legacy migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statement := insertEventSQL
	if firstMigration {
		// The old in-memory map kept the final duplicate from a JSONL file.
		statement = replaceEventSQL
	}
	insert, err := tx.Prepare(statement)
	if err != nil {
		return fmt.Errorf("usage: prepare legacy migration: %w", err)
	}
	defer insert.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLedgerLineBytes)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("usage: decode legacy ledger: %w", err)
		}
		if event.ID == "" {
			continue
		}
		if _, err := insert.Exec(eventValues(event)...); err != nil {
			return fmt.Errorf("usage: migrate legacy event: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("usage: read legacy ledger: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO usage_metadata (key, value) VALUES ('legacy_jsonl_state', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, state); err != nil {
		return fmt.Errorf("usage: record migration state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("usage: commit legacy migration: %w", err)
	}
	return nil
}

func eventValues(event Event) []any {
	return []any{
		event.ID,
		event.SessionID,
		event.Provider,
		event.Model,
		event.ResponseModel,
		event.ResponseID,
		event.Timestamp.Format(time.RFC3339Nano),
		event.Timestamp.Unix(),
		event.Timestamp.Nanosecond(),
		event.Usage.Input,
		event.Usage.InputUnknown,
		event.Usage.Output,
		event.Usage.CacheRead,
		event.Usage.CacheWrite,
		event.Usage.TotalTokens,
		event.Usage.Cost.Input,
		event.Usage.Cost.Output,
		event.Usage.Cost.CacheRead,
		event.Usage.Cost.CacheWrite,
		event.Usage.Cost.Total,
	}
}

func (s *Store) append(event Event) error {
	if _, err := s.db.Exec(insertEventSQL, eventValues(event)...); err != nil {
		return fmt.Errorf("usage: append event: %w", err)
	}
	return nil
}

// Report aggregates usage in SQLite. A zero since value includes the complete
// ledger; only the small set of grouped model rows enters process memory.
func (s *Store) Report(since time.Time) (Report, error) {
	latestFilter := ""
	reportFilter := ""
	var args []any
	if !since.IsZero() {
		latestFilter = `
			AND (latest.occurred_at_seconds, latest.occurred_at_nanos) >= (?, ?)`
		reportFilter = `
			WHERE (e.occurred_at_seconds, e.occurred_at_nanos) >= (?, ?)`
		sinceSeconds := since.Unix()
		sinceNanos := since.Nanosecond()
		args = []any{
			sinceSeconds, sinceNanos,
			sinceSeconds, sinceNanos,
			sinceSeconds, sinceNanos,
		}
	}
	query := `
		SELECT
			e.provider,
			e.model,
			COALESCE((
				SELECT latest.response_model
				FROM usage_events AS latest
				WHERE latest.provider = e.provider AND latest.model = e.model
			` + latestFilter + `
				ORDER BY
					latest.occurred_at_seconds DESC,
					latest.occurred_at_nanos DESC,
					latest.id DESC
				LIMIT 1
			), ''),
			COALESCE((
				SELECT latest.occurred_at
				FROM usage_events AS latest
				WHERE latest.provider = e.provider AND latest.model = e.model
			` + latestFilter + `
				ORDER BY
					latest.occurred_at_seconds DESC,
					latest.occurred_at_nanos DESC,
					latest.id DESC
				LIMIT 1
			), ''),
			COUNT(*),
			COALESCE(SUM(e.input_tokens), 0),
			COALESCE(MAX(e.input_unknown), 0),
			COALESCE(SUM(e.output_tokens), 0),
			COALESCE(SUM(e.cache_read_tokens), 0),
			COALESCE(SUM(e.cache_write_tokens), 0),
			COALESCE(SUM(CASE
				WHEN e.total_tokens = 0 THEN
					e.input_tokens + e.output_tokens + e.cache_read_tokens + e.cache_write_tokens
				ELSE e.total_tokens
			END), 0),
			COALESCE(SUM(e.cost_input), 0),
			COALESCE(SUM(e.cost_output), 0),
			COALESCE(SUM(e.cost_cache_read), 0),
			COALESCE(SUM(e.cost_cache_write), 0),
			COALESCE(SUM(e.cost_total), 0)
		FROM usage_events AS e
	` + reportFilter + `
		GROUP BY e.provider, e.model
	`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return Report{}, fmt.Errorf("usage: query report: %w", err)
	}
	defer rows.Close()

	report := Report{GeneratedAt: time.Now().UTC()}
	for rows.Next() {
		var summary ModelSummary
		var lastUsed string
		var inputUnknown int64
		if err := rows.Scan(
			&summary.Provider,
			&summary.Model,
			&summary.ResponseModel,
			&lastUsed,
			&summary.Requests,
			&summary.Input,
			&inputUnknown,
			&summary.Output,
			&summary.CacheRead,
			&summary.CacheWrite,
			&summary.TotalTokens,
			&summary.Cost.Input,
			&summary.Cost.Output,
			&summary.Cost.CacheRead,
			&summary.Cost.CacheWrite,
			&summary.Cost.Total,
		); err != nil {
			return Report{}, fmt.Errorf("usage: scan report: %w", err)
		}
		summary.InputUnknown = inputUnknown != 0
		summary.Name = summary.Model
		if model, ok := llm.LookupModel(summary.Provider, summary.Model); ok && model.Name != "" {
			summary.Name = model.Name
		}
		if lastUsed != "" {
			summary.LastUsedAt, err = time.Parse(time.RFC3339Nano, lastUsed)
			if err != nil {
				return Report{}, fmt.Errorf("usage: decode report timestamp: %w", err)
			}
		}
		report.Models = append(report.Models, summary)
		mergeTotals(&report.Total, summary.Totals)
	}
	if err := rows.Err(); err != nil {
		return Report{}, fmt.Errorf("usage: read report: %w", err)
	}
	sort.Slice(report.Models, func(i, j int) bool {
		if report.Models[i].TotalTokens == report.Models[j].TotalTokens {
			return report.Models[i].LastUsedAt.After(report.Models[j].LastUsedAt)
		}
		return report.Models[i].TotalTokens > report.Models[j].TotalTokens
	})
	return report, nil
}

func mergeTotals(total *Totals, add Totals) {
	total.Requests += add.Requests
	total.Input += add.Input
	total.InputUnknown = total.InputUnknown || add.InputUnknown
	total.Output += add.Output
	total.CacheRead += add.CacheRead
	total.CacheWrite += add.CacheWrite
	total.TotalTokens += add.TotalTokens
	total.Cost.Input += add.Cost.Input
	total.Cost.Output += add.Cost.Output
	total.Cost.CacheRead += add.Cost.CacheRead
	total.Cost.CacheWrite += add.Cost.CacheWrite
	total.Cost.Total += add.Cost.Total
}

// Events returns a stable newest-first page. Filtering, counting, sorting and
// pagination stay inside SQLite rather than copying the complete ledger.
func (s *Store) Events(
	provider, model string,
	since time.Time,
	offset, limit int,
) (EventPage, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	where, args := eventFilters(provider, model, since)
	var total int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM usage_events"+where,
		args...,
	).Scan(&total); err != nil {
		return EventPage{}, fmt.Errorf("usage: count events: %w", err)
	}
	page := EventPage{
		Events: []Event{},
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if offset >= total {
		return page, nil
	}
	query := `
		SELECT
			id, session_id, provider, model, response_model, response_id,
			occurred_at,
			input_tokens, input_unknown, output_tokens, cache_read_tokens,
			cache_write_tokens, total_tokens,
			cost_input, cost_output, cost_cache_read, cost_cache_write, cost_total
		FROM usage_events
	` + where + `
		ORDER BY occurred_at_seconds DESC, occurred_at_nanos DESC, id DESC
		LIMIT ? OFFSET ?
	`
	queryArgs := append(append([]any(nil), args...), limit, offset)
	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return EventPage{}, fmt.Errorf("usage: query events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var event Event
		var timestamp string
		var inputUnknown int64
		if err := rows.Scan(
			&event.ID,
			&event.SessionID,
			&event.Provider,
			&event.Model,
			&event.ResponseModel,
			&event.ResponseID,
			&timestamp,
			&event.Usage.Input,
			&inputUnknown,
			&event.Usage.Output,
			&event.Usage.CacheRead,
			&event.Usage.CacheWrite,
			&event.Usage.TotalTokens,
			&event.Usage.Cost.Input,
			&event.Usage.Cost.Output,
			&event.Usage.Cost.CacheRead,
			&event.Usage.Cost.CacheWrite,
			&event.Usage.Cost.Total,
		); err != nil {
			return EventPage{}, fmt.Errorf("usage: scan event: %w", err)
		}
		event.Usage.InputUnknown = inputUnknown != 0
		event.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return EventPage{}, fmt.Errorf("usage: decode event timestamp: %w", err)
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, fmt.Errorf("usage: read events: %w", err)
	}
	return page, nil
}

func eventFilters(provider, model string, since time.Time) (string, []any) {
	var conditions []string
	var args []any
	if provider != "" {
		conditions = append(conditions, "provider = ?")
		args = append(args, provider)
	}
	if model != "" {
		conditions = append(conditions, "model = ?")
		args = append(args, model)
	}
	if !since.IsZero() {
		conditions = append(
			conditions,
			"(occurred_at_seconds, occurred_at_nanos) >= (?, ?)",
		)
		args = append(args, since.Unix(), since.Nanosecond())
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
