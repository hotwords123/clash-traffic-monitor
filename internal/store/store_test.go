package store

import (
	"context"
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
