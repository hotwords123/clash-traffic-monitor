package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/local/clash-traffic-monitor/internal/collector"
	"github.com/local/clash-traffic-monitor/internal/store"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestQueryFilter(t *testing.T) {
	for _, test := range []struct {
		query string
		valid bool
	}{
		{"from=60&to=180&app=browser.exe&domain=example.com", true},
		{"from=61&to=180", false}, {"from=180&to=60", false},
		{"from=invalid", false}, {"to=-60", false}, {"from=0&to=999999999999", false},
	} {
		t.Run(test.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			_, _, ok := queryFilter(w, httptest.NewRequest("GET", "/api/series?"+test.query, nil))
			if ok != test.valid {
				t.Fatalf("valid=%v, response=%s", ok, w.Body.String())
			}
			if !ok && w.Code != 400 {
				t.Fatalf("status=%d", w.Code)
			}
		})
	}
}

func TestConnectionsRetentionAndFilters(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Truncate(time.Minute)
	err = db.Record(context.Background(), []store.Delta{
		{At: now, Connection: "match", App: "browser.exe", Domain: "example.com", Download: 10},
		{At: now, Connection: "other", App: "other.exe", Domain: "example.com", Download: 20},
		{At: now.Add(-8 * 24 * time.Hour), Connection: "expired", App: "browser.exe", Domain: "example.com", Download: 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(db, collector.New(nil, db), fstest.MapFS{}).Handler()
	url := fmt.Sprintf("/api/connections?from=%d&to=%d&app=browser.exe&app_exact=1&page_size=1", now.Add(-9*24*time.Hour).Unix(), now.Add(time.Minute).Unix())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	var page store.EventPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Connection != "match" {
		t.Fatalf("response=%d %s", w.Code, w.Body.String())
	}
}

func TestSelectedQueryNames(t *testing.T) {
	_, f, ok := queryFilter(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/summary?app=search&app_selected=a.exe&app_selected=b.exe&app_selected=a.exe&domain_selected=one.test", nil))
	if !ok || len(f.Apps) != 2 || len(f.Domains) != 1 || f.App != "search" {
		t.Fatalf("multi selection parsing: %+v, %v", f, ok)
	}
}

func TestRankingPageAPI(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Truncate(time.Minute)
	if err := db.Record(context.Background(), []store.Delta{
		{At: now, Connection: "a", App: "a.exe", Domain: "test", Download: 10},
		{At: now, Connection: "b", App: "b.exe", Domain: "test", Download: 20},
	}); err != nil {
		t.Fatal(err)
	}
	handler := New(db, collector.New(nil, db), fstest.MapFS{}).Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/top/apps?page=2&page_size=1&app=.exe", nil))
	var result store.RankedPage
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || result.Total != 2 || result.Page != 2 || result.Items[0].Name != "a.exe" {
		t.Fatalf("page response: %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/top/apps?limit=10", nil))
	var legacy []store.RankedItem
	if err := json.Unmarshal(w.Body.Bytes(), &legacy); err != nil || len(legacy) != 2 {
		t.Fatalf("legacy response: %s, %v", w.Body.String(), err)
	}
}

func TestSeriesBucketAPI(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Truncate(time.Minute)
	if err := db.Record(context.Background(), []store.Delta{
		{At: now, Connection: "a", App: "a", Domain: "test", Download: 10},
		{At: now.Add(time.Minute), Connection: "b", App: "b", Domain: "test", Download: 20},
	}); err != nil {
		t.Fatal(err)
	}
	handler := New(db, collector.New(nil, db), fstest.MapFS{}).Handler()
	w := httptest.NewRecorder()
	url := fmt.Sprintf("/api/series?from=%d&to=%d&bucket_minutes=20", now.Unix(), now.Add(40*time.Minute).Unix())
	handler.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	var points []store.TimePoint
	if err := json.Unmarshal(w.Body.Bytes(), &points); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(points) != 1 || points[0].Download != 30 {
		t.Fatalf("series=%s", w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/series?bucket_minutes=17", nil))
	if w.Code != 400 {
		t.Fatalf("invalid bucket: %s", w.Body.String())
	}
}
