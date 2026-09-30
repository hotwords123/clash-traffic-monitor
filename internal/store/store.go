package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	NewConnection bool
}

type Summary struct {
	Upload      int64 `json:"upload"`
	Download    int64 `json:"download"`
	Connections int64 `json:"connections"`
}

type RankedItem struct {
	Name        string `json:"name"`
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
  PRIMARY KEY (connection_id, observed_at)
);
CREATE INDEX IF NOT EXISTS idx_events_observed_at ON connection_events(observed_at);
CREATE INDEX IF NOT EXISTS idx_minute_bucket ON traffic_minute(bucket);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
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
INSERT INTO connection_events(connection_id, observed_at, app, domain, upload, download, rule, proxy)
VALUES(?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(connection_id, observed_at) DO UPDATE SET
  upload = upload + excluded.upload,
  download = download + excluded.download,
  app = excluded.app,
  domain = excluded.domain,
  rule = excluded.rule,
  proxy = excluded.proxy`)
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
		if _, err := event.ExecContext(ctx, d.Connection, bucket, d.App, d.Domain, d.Upload, d.Download, d.Rule, d.Proxy); err != nil {
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

func (s *Store) Summary(ctx context.Context, since time.Time) (Summary, error) {
	var result Summary
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(upload), 0), COALESCE(SUM(download), 0), COALESCE(SUM(connections), 0)
FROM traffic_minute WHERE bucket >= ?`, since.Unix()).Scan(&result.Upload, &result.Download, &result.Connections)
	return result, err
}

func (s *Store) Top(ctx context.Context, since time.Time, dimension string, limit int) ([]RankedItem, error) {
	column := "app"
	if dimension == "domain" {
		column = "domain"
	}
	query := fmt.Sprintf(`
SELECT %s, SUM(upload), SUM(download), SUM(connections)
FROM traffic_minute WHERE bucket >= ?
GROUP BY %s ORDER BY SUM(upload + download) DESC LIMIT ?`, column, column)
	rows, err := s.db.QueryContext(ctx, query, since.Unix(), limit)
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
	return items, rows.Err()
}

func (s *Store) Series(ctx context.Context, since time.Time) ([]TimePoint, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT bucket, SUM(upload), SUM(download)
FROM traffic_minute WHERE bucket >= ?
GROUP BY bucket ORDER BY bucket`, since.Unix())
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
