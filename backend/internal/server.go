package internal

import (
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
)

// webFS — встроенный статический фронтенд (заполняется через SetWebFS).
var webFS fs.FS

// SetWebFS — подгоняет встраиваемый FS из main (package main).
func SetWebFS(f fs.FS) {
	webFS = f
}

// Server — центральный объект сервера.
type Server struct {
	cfg   Config
	db    *Store
	films *FilmCache
	qb    *QB
	rate  *RateLimiter
}

func NewServer(cfg Config, store *Store) *Server {
	s := &Server{
		cfg:   cfg,
		db:    store,
		films: NewFilmCache(cfg.MovieRoot, cfg.RefreshSecs),
		qb:    NewQB(),
		rate:  &RateLimiter{m: make(map[string][]int64)},
	}
	s.films.Load()
	return s
}

// writeJSON — выводит JSON-ответ.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// writeErr — JSON-ошибка.
func (s *Server) writeErr(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

// Handler — mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Auth
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/me", s.handleMe)
	mux.HandleFunc("/api/auth/password", s.handleChangePassword)

	// Films
	mux.HandleFunc("/api/films", s.handleListFilms)
	mux.HandleFunc("/api/films/cover/", s.handleCover)
	mux.HandleFunc("/api/films/stream/", s.handleStream)
	mux.HandleFunc("/api/films/download/", s.handleDownload)

	// Torrents (qBittorrent)
	mux.HandleFunc("/api/torrents", s.handleTorrents)

	// Health
	mux.HandleFunc("/api/health", s.handleHealthz)
	mux.HandleFunc("/api/healthz", s.handleHealthz)

	// Admin
	mux.HandleFunc("/api/admin", s.handleAdminIndex)
	mux.Handle("/api/admin/", http.HandlerFunc(s.handleAdminPrefix))

	// SPA fallback
	mux.HandleFunc("/", s.handleStatic)

	return s.middlewares(mux)
}

func (s *Server) middlewares(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Логирование IP-адреса клиента.
		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = clientIP(r)
		}
		log.Printf("%s %s (ip=%s, ua=%s)", r.Method, r.URL.Path, ip, r.UserAgent())
		h.ServeHTTP(w, r)
	})
}

// clientIP — клиентский IP с учётом proxy.
func clientIP(r *http.Request) string {
	h := r.Header.Get("X-Forwarded-For")
	if h != "" {
		return h
	}
	remote := r.RemoteAddr
	if remote == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

// handleListFilms — GET /api/films
func (s *Server) handleListFilms(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	films := s.films.List()
	if len(films) == 0 {
		films = []Film{}
	}
	s.writeJSON(w, http.StatusOK, films)
}

// handleStatic — SPA fallback: отдаёт встраиваемый фронтенд,
// либо redirect на /app (если статические файлы не встроены).
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Для API-пути отдаём 404, чтобы не мешать SPA.
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Если встроенный FS пуст — отдаём заглушку, чтобы smoke-test работал.
	if webFS == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Media Server</title></head>
<body><h1>Media Server</h1><p>Фронтенд будет встроен при сборке. API доступен по /api/.</p></body></html>`))
		return
	}

	// SPA fallback: отдаём index.html или файл из встраиваемого FS.
	file, err := webFS.Open(r.URL.Path[1:])
	if err != nil {
		file, err = webFS.Open("index.html")
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		b, _ := io.ReadAll(file)
		w.Write(b)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", guessContentType(r.URL.Path))
	w.WriteHeader(http.StatusOK)
	b, _ := io.ReadAll(file)
	w.Write(b)
}

// guessContentType — угадывает MIME по расширению.
func guessContentType(path string) string {
	i := strings.LastIndex(path, ".")
	ext := ""
	if i >= 0 {
		ext = strings.ToLower(path[i+1:])
	}
	switch ext {
	case "html":
		return "text/html; charset=utf-8"
	case "css":
		return "text/css; charset=utf-8"
	case "js":
		return "text/javascript; charset=utf-8"
	case "json":
		return "application/json; charset=utf-8"
	case "svg":
		return "image/svg+xml; charset=utf-8"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
