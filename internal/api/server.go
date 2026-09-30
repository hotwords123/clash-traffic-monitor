package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/local/clash-traffic-monitor/internal/collector"
	"github.com/local/clash-traffic-monitor/internal/store"
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
	mux.Handle("/", spaHandler(s.assets))
	return withHeaders(mux)
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.collector.Status())
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.Summary(r.Context(), since(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	series, err := s.store.Series(r.Context(), since(r))
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
		items, err := s.store.Top(r.Context(), since(r), dimension, limit)
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
