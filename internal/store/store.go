package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Delta struct {
	At            time.Time
	Connection    string
	App           string
	Domain        string
	Upload        int64
	Download      int64
	Rule          string
	Proxy         string
	Details       json.RawMessage
	NewConnection bool
}

type Summary struct {
	Upload      int64 `json:"upload"`
	Download    int64 `json:"download"`
	Connections int64 `json:"connections"`
}

type RankedItem struct {
	Name        string `json:"name"`
	Proxies     string `json:"proxies"`
	Upload      int64  `json:"upload"`
	Download    int64  `json:"download"`
	Connections int64  `json:"connections"`
}

type TimePoint struct {
	Time     int64 `json:"time"`
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS traffic_minute (
  bucket INTEGER NOT NULL,
  app TEXT NOT NULL,
  domain TEXT NOT NULL,
  upload INTEGER NOT NULL DEFAULT 0,
  download INTEGER NOT NULL DEFAULT 0,
  connections INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (bucket, app, domain)
);
CREATE TABLE IF NOT EXISTS connection_events (
  connection_id TEXT NOT NULL,
  observed_at INTEGER NOT NULL,
  app TEXT NOT NULL,
  domain TEXT NOT NULL,
  upload INTEGER NOT NULL,
  download INTEGER NOT NULL,
  rule TEXT NOT NULL,
  proxy TEXT NOT NULL,
  details TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY (connection_id, observed_at)
);
CREATE INDEX IF NOT EXISTS idx_events_observed_at ON connection_events(observed_at);
CREATE INDEX IF NOT EXISTS idx_minute_bucket ON traffic_minute(bucket);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	// Existing databases need an explicit additive migration.
	rows, err := s.db.Query("PRAGMA table_info(connection_events)")
	if err != nil {
		return err
	}
	hasDetails := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		hasDetails = hasDetails || name == "details"
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !hasDetails {
		_, err = s.db.Exec("ALTER TABLE connection_events ADD COLUMN details TEXT NOT NULL DEFAULT '{}'")
	}
	return err
}

func (s *Store) Record(ctx context.Context, deltas []Delta) error {
	if len(deltas) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	aggregate, err := tx.PrepareContext(ctx, `
INSERT INTO traffic_minute(bucket, app, domain, upload, download, connections)
VALUES(?, ?, ?, ?, ?, ?)
ON CONFLICT(bucket, app, domain) DO UPDATE SET
  upload = upload + excluded.upload,
  download = download + excluded.download,
  connections = connections + excluded.connections`)
	if err != nil {
		return err
	}
	defer aggregate.Close()
	event, err := tx.PrepareContext(ctx, `
INSERT INTO connection_events(connection_id, observed_at, app, domain, upload, download, rule, proxy, details)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(connection_id, observed_at) DO UPDATE SET
  upload = upload + excluded.upload,
  download = download + excluded.download,
  app = excluded.app,
  domain = excluded.domain,
  rule = excluded.rule,
  proxy = excluded.proxy,
  details = excluded.details`)
	if err != nil {
		return err
	}
	defer event.Close()
	for _, d := range deltas {
		bucket := d.At.Truncate(time.Minute).Unix()
		connectionIncrement := 0
		if d.NewConnection {
			connectionIncrement = 1
		}
		if _, err := aggregate.ExecContext(ctx, bucket, d.App, d.Domain, d.Upload, d.Download, connectionIncrement); err != nil {
			return err
		}
		details := string(d.Details)
		if details == "" {
			details = "{}"
		}
		if _, err := event.ExecContext(ctx, d.Connection, bucket, d.App, d.Domain, d.Upload, d.Download, d.Rule, d.Proxy, details); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Cleanup(ctx context.Context, retentionDays int) error {
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	_, err := s.db.ExecContext(ctx, "DELETE FROM connection_events WHERE observed_at < ?", cutoff)
	return err
}

func (s *Store) Summary(ctx context.Context, since time.Time, filters ...Filter) (Summary, error) {
	var result Summary
	where, args := conditions(since, "bucket", filters...)
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(upload), 0), COALESCE(SUM(download), 0), COALESCE(SUM(connections), 0)
FROM traffic_minute WHERE `+where, args...).Scan(&result.Upload, &result.Download, &result.Connections)
	return result, err
}

func (s *Store) Top(ctx context.Context, since time.Time, dimension string, limit int, filters ...Filter) ([]RankedItem, error) {
	column := "app"
	if dimension == "domain" {
		column = "domain"
	}
	where, args := conditions(since, "bucket", filters...)
	query := fmt.Sprintf(`
SELECT %s, SUM(upload), SUM(download), SUM(connections)
FROM traffic_minute WHERE %s
GROUP BY %s ORDER BY SUM(upload + download) DESC, %s LIMIT ?`, column, where, column, column)
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]RankedItem, 0, limit)
	for rows.Next() {
		var item RankedItem
		if err := rows.Scan(&item.Name, &item.Upload, &item.Download, &item.Connections); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if dimension == "domain" {
		eventWhere, eventArgs := conditions(since, "observed_at", filters...)
		for i := range items {
			args := append(append([]any{}, eventArgs...), items[i].Name)
			err := s.db.QueryRowContext(ctx, `SELECT COALESCE(GROUP_CONCAT(proxy, ' / '), '') FROM
    (SELECT DISTINCT proxy FROM connection_events WHERE `+eventWhere+` AND domain = ? AND proxy != '' ORDER BY proxy)`, args...).Scan(&items[i].Proxies)
			if err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}

func (s *Store) Series(ctx context.Context, since time.Time, filters ...Filter) ([]TimePoint, error) {
	where, args := conditions(since, "bucket", filters...)
	rows, err := s.db.QueryContext(ctx, `
SELECT bucket, SUM(upload), SUM(download)
FROM traffic_minute WHERE `+where+`
GROUP BY bucket ORDER BY bucket`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := make([]TimePoint, 0)
	for rows.Next() {
		var point TimePoint
		if err := rows.Scan(&point.Time, &point.Upload, &point.Download); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

// Filter applies to minute buckets. Until is exclusive; text matching is literal.
type Filter struct {
	Until                 time.Time
	App, Domain           string
	AppExact, DomainExact bool
}

func conditions(since time.Time, timeColumn string, filters ...Filter) (string, []any) {
	where := timeColumn + " >= ?"
	args := []any{since.Unix()}
	if len(filters) == 0 {
		return where, args
	}
	f := filters[0]
	if !f.Until.IsZero() {
		where += " AND " + timeColumn + " < ?"
		args = append(args, f.Until.Unix())
	}
	for _, item := range []struct {
		column, value string
		exact         bool
	}{{"app", f.App, f.AppExact}, {"domain", f.Domain, f.DomainExact}} {
		if item.value == "" {
			continue
		}
		if item.exact {
			where += " AND " + item.column + " = ? COLLATE NOCASE"
			args = append(args, item.value)
		} else {
			where += " AND instr(lower(" + item.column + "), lower(?)) > 0"
			args = append(args, strings.TrimSpace(item.value))
		}
	}
	return where, args
}

type ConnectionEvent struct {
	Connection string          `json:"connection_id"`
	Time       int64           `json:"time"`
	App        string          `json:"app"`
	Domain     string          `json:"domain"`
	Upload     int64           `json:"upload"`
	Download   int64           `json:"download"`
	Rule       string          `json:"rule"`
	Proxy      string          `json:"proxy"`
	Details    json.RawMessage `json:"details"`
}
type EventPage struct {
	Items    []ConnectionEvent `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

func (s *Store) Events(ctx context.Context, since time.Time, f Filter, page, pageSize int) (EventPage, error) {
	result := EventPage{Items: []ConnectionEvent{}, Page: page, PageSize: pageSize}
	where, args := conditions(since, "observed_at", f)
	// Count and page share a read transaction so collection cannot change the result between them.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM connection_events WHERE "+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := tx.QueryContext(ctx, `SELECT connection_id, observed_at, app, domain, upload, download, rule, proxy, details
 FROM connection_events WHERE `+where+` ORDER BY observed_at DESC, connection_id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item ConnectionEvent
		var details string
		if err := rows.Scan(&item.Connection, &item.Time, &item.App, &item.Domain, &item.Upload, &item.Download, &item.Rule, &item.Proxy, &details); err != nil {
			rows.Close()
			return result, err
		}
		item.Details = json.RawMessage(details)
		result.Items = append(result.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}
