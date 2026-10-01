package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAndQuery(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().Truncate(time.Minute)
	deltas := []Delta{
		{At: now, Connection: "a", App: "browser.exe", Domain: "example.com", Upload: 100, Download: 900, NewConnection: true},
		{At: now.Add(10 * time.Second), Connection: "a", App: "browser.exe", Domain: "example.com", Upload: 50, Download: 450},
		{At: now, Connection: "b", App: "chat.exe", Domain: "api.example.com", Upload: 20, Download: 80, NewConnection: true},
	}
	if err := db.Record(context.Background(), deltas); err != nil {
		t.Fatal(err)
	}

	summary, err := db.Summary(context.Background(), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Upload != 170 || summary.Download != 1430 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Connections != 2 {
		t.Fatalf("unexpected connection count: %+v", summary)
	}

	apps, err := db.Top(context.Background(), now.Add(-time.Minute), "app", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 || apps[0].Name != "browser.exe" || apps[0].Download != 1350 {
		t.Fatalf("unexpected app ranking: %+v", apps)
	}

	points, err := db.Series(context.Background(), now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].Upload != 170 || points[0].Download != 1430 {
		t.Fatalf("unexpected series: %+v", points)
	}
}

func TestRankingDomainCount(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Minute)
	if err := db.Record(ctx, []Delta{
		{At: at, Connection: "a", App: "browser.exe", Domain: "one.test", Download: 10},
		{At: at.Add(time.Minute), Connection: "b", App: "browser.exe", Domain: "one.test", Download: 20},
		{At: at.Add(time.Minute), Connection: "c", App: "browser.exe", Domain: "two.test", Download: 30},
		{At: at.Add(2 * time.Minute), Connection: "d", App: "browser.exe", Domain: "outside.test", Download: 40},
		{At: at, Connection: "e", App: "other.exe", Domain: "other.test", Download: 50},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Cleanup(ctx, 7); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		filter Filter
		want   int64
	}{
		{"deduplicated across minutes after detail cleanup", Filter{Until: at.Add(2 * time.Minute), App: "browser.exe", AppExact: true}, 2},
		{"time range", Filter{Until: at.Add(time.Minute), App: "browser.exe", AppExact: true}, 1},
		{"domain selection", Filter{Until: at.Add(2 * time.Minute), App: "browser.exe", AppExact: true, Domains: []string{"two.test"}}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := db.Ranking(ctx, at, "app", tc.filter, 1, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 || result.Items[0].Name != "browser.exe" || result.Items[0].DomainCount != tc.want {
				t.Fatalf("domain count: %+v, want %d", result, tc.want)
			}
		})
	}
}

func TestFiltersAndEventPagination(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Now().Truncate(time.Minute)
	err = db.Record(ctx, []Delta{
		{At: at, Connection: "a", App: "Browser.exe", Domain: "example.com", Download: 10, Proxy: "自动选择", Details: json.RawMessage(`{"chains":["节点","自动选择"]}`), NewConnection: true},
		{At: at.Add(time.Minute), Connection: "a", App: "Browser.exe", Domain: "example.com", Download: 20},
		{At: at, Connection: "b", App: "other.exe", Domain: "example.com", Download: 100},
		{At: at, Connection: "c", App: "Browser.exe", Domain: "other.com", Download: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	f := Filter{Until: at.Add(time.Minute), App: "browser.exe", AppExact: true, Domain: "example.com", DomainExact: true}
	summary, err := db.Summary(ctx, at, f)
	if err != nil || summary.Download != 10 || summary.Connections != 1 {
		t.Fatalf("summary: %+v, %v", summary, err)
	}
	points, err := db.Series(ctx, at, f)
	if err != nil || len(points) != 1 || points[0].Download != 10 {
		t.Fatalf("series: %+v, %v", points, err)
	}
	top, err := db.Top(ctx, at, "domain", 10, f)
	if err != nil || len(top) != 1 || top[0].Proxies != "自动选择" {
		t.Fatalf("top: %+v, %v", top, err)
	}
	events, err := db.Events(ctx, at, Filter{}, 1, 2)
	if err != nil || events.Total != 4 || len(events.Items) != 2 {
		t.Fatalf("events: %+v, %v", events, err)
	}
	events, err = db.Events(ctx, at, f, 1, 10)
	if err != nil || len(events.Items) != 1 || events.Items[0].Proxy != "自动选择" || !json.Valid(events.Items[0].Details) {
		t.Fatalf("filtered events: %+v, %v", events, err)
	}
	summary, err = db.Summary(ctx, at, Filter{App: "%"})
	if err != nil || summary.Download != 0 {
		t.Fatalf("literal wildcard: %+v, %v", summary, err)
	}
}

func TestLegacyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE connection_events (
 connection_id TEXT NOT NULL, observed_at INTEGER NOT NULL, app TEXT NOT NULL,
 domain TEXT NOT NULL, upload INTEGER NOT NULL, download INTEGER NOT NULL,
 rule TEXT NOT NULL, proxy TEXT NOT NULL, PRIMARY KEY(connection_id, observed_at));
 INSERT INTO connection_events VALUES ('old', 60, 'browser.exe', 'example.com', 1, 2, 'Domain', 'DIRECT')`)
	legacy.Close()
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		events, err := db.Events(context.Background(), time.Unix(0, 0), Filter{}, 1, 10)
		db.Close()
		if err != nil || len(events.Items) != 1 || string(events.Items[0].Details) != "{}" || events.Items[0].Download != 2 {
			t.Fatalf("migration: %+v, %v", events, err)
		}
	}
}

func TestMultiSelectionOverridesSearch(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Now().Truncate(time.Minute)
	if err := db.Record(ctx, []Delta{
		{At: at, Connection: "a", App: "a.exe", Domain: "one.test", Download: 10},
		{At: at, Connection: "b", App: "b.exe", Domain: "one.test", Download: 20},
		{At: at, Connection: "c", App: "a.exe", Domain: "two.test", Download: 40},
		{At: at, Connection: "d", App: "c.exe", Domain: "one.test", Download: 80},
	}); err != nil {
		t.Fatal(err)
	}
	f := Filter{Until: at.Add(time.Minute), App: "no-match", Domain: "no-match", Apps: []string{"A.exe", "b.exe"}, Domains: []string{"one.test"}}
	summary, err := db.Summary(ctx, at, f)
	if err != nil || summary.Download != 30 {
		t.Fatalf("selected OR/AND: %+v, %v", summary, err)
	}
	points, err := db.Series(ctx, at, f)
	if err != nil || len(points) != 1 || points[0].Download != 30 {
		t.Fatalf("series: %+v, %v", points, err)
	}
	events, err := db.Events(ctx, at, f, 1, 50)
	if err != nil || events.Total != 2 {
		t.Fatalf("events: %+v, %v", events, err)
	}
	f.Apps = nil
	f.Domains = nil
	f.App = ".exe"
	f.Domain = "one"
	summary, err = db.Summary(ctx, at, f)
	if err != nil || summary.Download != 110 {
		t.Fatalf("search fallback: %+v, %v", summary, err)
	}
}

func TestRankingPagesBeyondTop100(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Now().Truncate(time.Minute)
	deltas := []Delta{}
	for i := range 105 {
		deltas = append(deltas, Delta{At: at, Connection: fmt.Sprint(i), App: fmt.Sprintf("app-%03d.exe", i), Domain: "example.com", Download: 1})
	}
	if err := db.Record(ctx, deltas); err != nil {
		t.Fatal(err)
	}
	result, err := db.Ranking(ctx, at, "app", Filter{}, 11, 10)
	if err != nil || result.Total != 105 || len(result.Items) != 5 || result.Items[0].Name != "app-100.exe" {
		t.Fatalf("last page: %+v, %v", result, err)
	}
	result, err = db.Ranking(ctx, at, "app", Filter{App: "app-10"}, 11, 10)
	if err != nil || result.Page != 1 || result.Total != 5 {
		t.Fatalf("search clamps page: %+v, %v", result, err)
	}
}

func TestSeriesBucketAggregationPreservesMissingBuckets(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Date(2026, time.October, 1, 10, 0, 0, 0, time.Local)
	if err := db.Record(ctx, []Delta{
		{At: at, Connection: "a", App: "a", Domain: "one", Upload: 1, Download: 10},
		{At: at.Add(time.Minute), Connection: "b", App: "b", Domain: "two", Upload: 2, Download: 20},
		{At: at.Add(19 * time.Minute), Connection: "a", App: "a", Domain: "one", Upload: 3, Download: 30},
		{At: at.Add(60 * time.Minute), Connection: "a", App: "a", Domain: "one", Upload: 4, Download: 40},
	}); err != nil {
		t.Fatal(err)
	}
	points, err := db.Series(ctx, at, Filter{Until: at.Add(80 * time.Minute), BucketSeconds: 1200})
	if err != nil || len(points) != 2 {
		t.Fatalf("buckets: %+v, %v", points, err)
	}
	if points[0].Download != 60 || points[0].Upload != 6 || points[0].ObservedMinutes != 3 || points[1].Time != at.Add(60*time.Minute).Unix() {
		t.Fatalf("aggregation/gap: %+v", points)
	}
	points, err = db.Series(ctx, at, Filter{Apps: []string{"a"}, BucketSeconds: 1200})
	if err != nil || points[0].Download != 40 || points[0].ObservedMinutes != 2 {
		t.Fatalf("filtered buckets: %+v, %v", points, err)
	}
}

func TestSeriesBucketsAlignToLocalMidnight(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	midnight := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.Local)
	at := midnight.Add(10*time.Hour + 7*time.Minute)
	if err := db.Record(ctx, []Delta{
		{At: at.Add(-time.Minute), Connection: "before", App: "a", Domain: "test", Download: 1000},
		{At: at, Connection: "a", App: "a", Domain: "test", Download: 10},
		{At: midnight.Add(10*time.Hour + 19*time.Minute), Connection: "b", App: "a", Domain: "test", Download: 20},
		{At: midnight.Add(10*time.Hour + 20*time.Minute), Connection: "c", App: "a", Domain: "test", Download: 30},
		{At: midnight.Add(10*time.Hour + 21*time.Minute), Connection: "after", App: "a", Domain: "test", Download: 2000},
	}); err != nil {
		t.Fatal(err)
	}
	points, err := db.Series(ctx, at, Filter{Until: midnight.Add(10*time.Hour + 21*time.Minute), BucketSeconds: 1200})
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 || points[0].Time != midnight.Add(10*time.Hour).Unix() || points[0].Download != 30 || points[1].Time != midnight.Add(10*time.Hour+20*time.Minute).Unix() || points[1].Download != 30 {
		t.Fatalf("aligned partial buckets: %+v", points)
	}
	shifted, err := db.Series(ctx, at.Add(time.Minute), Filter{Until: midnight.Add(10*time.Hour + 21*time.Minute), BucketSeconds: 1200})
	if err != nil || len(shifted) != 2 || shifted[0].Time != points[0].Time || shifted[0].Download != 20 {
		t.Fatalf("moving query start changed alignment: %+v, %v", shifted, err)
	}
	for _, minutes := range []int64{180, 360, 720, 1440, 4320, 10080} {
		points, err := db.Series(ctx, at, Filter{Until: at.Add(time.Hour), BucketSeconds: minutes * 60})
		if err != nil || len(points) != 1 || (points[0].Time-midnight.Unix())%(minutes*60) != 0 {
			t.Fatalf("%d-minute alignment: %+v, %v", minutes, points, err)
		}
	}
}
