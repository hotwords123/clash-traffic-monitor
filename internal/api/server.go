package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hotwords123/clash-traffic-monitor/internal/collector"
	"github.com/hotwords123/clash-traffic-monitor/internal/store"
)

type Server struct {
	store     *store.Store
	collector *collector.Collector
	assets    fs.FS
}

func New(store *store.Store, collector *collector.Collector, assets fs.FS) *Server {
	return &Server{store: store, collector: collector, assets: assets}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/summary", s.summary)
	mux.HandleFunc("GET /api/series", s.series)
	mux.HandleFunc("GET /api/top/apps", s.top("app"))
	mux.HandleFunc("GET /api/top/domains", s.top("domain"))
	mux.HandleFunc("GET /api/connections", s.connections)
	mux.Handle("/", spaHandler(s.assets))
	return withHeaders(mux)
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.collector.Status())
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	start, filter, ok := queryFilter(w, r)
	if !ok {
		return
	}
	summary, err := s.store.Summary(r.Context(), start, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	start, filter, ok := queryFilter(w, r)
	if !ok {
		return
	}
	if value := r.URL.Query().Get("bucket_minutes"); value != "" {
		minutes, err := strconv.Atoi(value)
		valid := false
		for _, preset := range []int{1, 5, 10, 20, 30, 60, 180, 360, 720, 1440, 4320, 10080} {
			valid = valid || minutes == preset
		}
		if err != nil || !valid {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不支持的时间桶粒度"})
			return
		}
		filter.BucketSeconds = int64(minutes) * 60
	}
	series, err := s.store.Series(r.Context(), start, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, series)
}

func (s *Server) top(dimension string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 10
		}
		start, filter, ok := queryFilter(w, r)
		if !ok {
			return
		}
		if r.URL.Query().Has("page") {
			page, size := pagination(r, 10)
			items, err := s.store.Ranking(r.Context(), start, dimension, filter, page, size)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, items)
			return
		}
		items, err := s.store.Top(r.Context(), start, dimension, limit, filter)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func since(r *http.Request) time.Time {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours < 1 || hours > 24*365 {
		hours = 24
	}
	return time.Now().Add(-time.Duration(hours) * time.Hour)
}

func spaHandler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(assets, path); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Origin") != "" {
			if origin := r.Header.Get("Origin"); origin != "http://"+r.Host && origin != "https://"+r.Host {
				http.Error(w, fmt.Sprintf("origin %q is not allowed", origin), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Query bounds are Unix seconds aligned to minutes, [from, to).
func queryFilter(w http.ResponseWriter, r *http.Request) (time.Time, store.Filter, bool) {
	q := r.URL.Query()
	start := since(r).Truncate(time.Minute)
	f := store.Filter{Until: time.Now().Add(time.Minute).Truncate(time.Minute), App: strings.TrimSpace(q.Get("app")), Domain: strings.TrimSpace(q.Get("domain")), AppExact: q.Get("app_exact") == "1", DomainExact: q.Get("domain_exact") == "1"}
	for _, selected := range []struct {
		key    string
		target *[]string
	}{{"app_selected", &f.Apps}, {"domain_selected", &f.Domains}} {
		names := q[selected.key]
		if len(names) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "每类最多选择 200 项"})
			return start, f, false
		}
		seen := map[string]bool{}
		for _, value := range names {
			value = strings.TrimSpace(value)
			if value == "" || seen[value] {
				continue
			}
			if len(value) > 1024 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "筛选名称过长"})
				return start, f, false
			}
			seen[value] = true
			*selected.target = append(*selected.target, value)
		}
	}
	for _, bound := range []struct {
		key    string
		target *time.Time
	}{{"from", &start}, {"to", &f.Until}} {
		if value := q.Get(bound.key); value != "" {
			seconds, err := strconv.ParseInt(value, 10, 64)
			if err != nil || seconds < 0 || seconds > 253402300799 || seconds%60 != 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "时间参数必须是按分钟对齐的 Unix 秒数"})
				return start, f, false
			}
			*bound.target = time.Unix(seconds, 0)
		}
	}
	if !start.Before(f.Until) || f.Until.Sub(start) > 366*24*time.Hour {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "时间范围必须大于零且不超过 366 天"})
		return start, f, false
	}
	return start, f, true
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	start, filter, ok := queryFilter(w, r)
	if !ok {
		return
	}
	// Enforce the detail view's seven-day window even before scheduled cleanup.
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Truncate(time.Minute)
	if start.Before(cutoff) {
		start = cutoff
	}
	page, size := pagination(r, 50)
	result, err := s.store.Events(r.Context(), start, filter, page, size)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func pagination(r *http.Request, defaultSize int) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 || page > 1000000 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = defaultSize
	}
	return page, size
}
