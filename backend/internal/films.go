package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// videoExts — поддерживаемые видеозаверения.
var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".mov": true,
	".avi": true, ".ts": true, ".webm": true,
}

var mimeByExt = map[string]string{
	".mkv":  "video/x-matroska",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".avi":  "video/avi",
	".ts":   "video/mp2t",
	".webm": "video/webm",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
}

// FilmMeta — метаданные из film.json.
type FilmMeta struct {
	Title       string   `json:"title"`
	Year        string   `json:"year"`
	Description string   `json:"description"`
	Rating      float64  `json:"rating"`
	Comment     string   `json:"comment"`
	Genres      []string `json:"genres"`
	Cover       string   `json:"cover"`
}

// Film — отдельный фильм в кэше.
type Film struct {
	ID          string
	Name        string
	Description string
	Year        string
	Rating      float64
	Comment     string
	Genres      []string
	Cover       string
	CoverPath   string
	Size        int64
	Path        string
	MimeType    string
}

// FilmCache — кэш обнаруженных фильмов с фоновым обновлением.
type FilmCache struct {
	mu      sync.RWMutex
	root    string
	refresh int
	files   map[string]Film
	last    time.Time
}

// stableFilmID — детерминированный ID: хэш абсолютного пути файла.
// Один и тот же файл всегда даёт один и тот же ID, независимо от количества
// фоновых пересканирований (refresh-тикер / admin-сканы). Устраняет
// "film not found" при повторных открытиях после фоновых сканов.
func stableFilmID(path string) string {
	h := sha256.Sum256([]byte(path))
	return "f" + hex.EncodeToString(h[:6])
}

func NewFilmCache(root string, refreshSecs int) *FilmCache {
	c := &FilmCache{
		root:    filepath.Clean(root),
		refresh: refreshSecs,
		files:   make(map[string]Film),
	}
	if c.refresh < 30 {
		c.refresh = 30
	}
	return c
}

func (c *FilmCache) Load() {
	c.scan()
	if c.refresh > 0 {
		ticker := time.NewTicker(time.Duration(c.refresh) * time.Second)
		go func() {
			defer ticker.Stop()
			for range ticker.C {
				if err := c.scan(); err != nil {
					log.Printf("films refresh error: %v", err)
				}
			}
		}()
	}
}

func (c *FilmCache) scan() error {
	files := make(map[string]Film)
	err := filepath.Walk(c.root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !videoExts[filepath.Ext(info.Name())] {
			return nil
		}
		dir := filepath.Dir(p)
		meta := FilmMeta{}
		if b, err := os.ReadFile(filepath.Join(dir, "film.json")); err == nil {
			if perr := json.Unmarshal(b, &meta); perr != nil {
				log.Printf("film.json parse error: %v", perr)
			}
		}
		name := bareName(info.Name(), meta.Title)
		f := Film{
			ID:          stableFilmID(p),
			Name:        name,
			Description: meta.Description,
			Year:        meta.Year,
			Rating:      meta.Rating,
			Comment:     meta.Comment,
			Genres:      meta.Genres,
			Size:        info.Size(),
			Path:        p,
		}
		f.CoverPath = findCover(dir, name, meta.Cover)
		if f.CoverPath != "" {
			f.Cover = strings.TrimPrefix(filepath.Clean(f.CoverPath), filepath.Clean(c.root)+"/")
		}
		if m, ok := mimeByExt[filepath.Ext(info.Name())]; ok {
			f.MimeType = m
		}
		files[f.ID] = f
		return nil
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.files = files
	c.last = time.Now()
	c.mu.Unlock()
	return nil
}

// bareName — имя фильма: из film.json, иначе от имени без номера части.
func bareName(full, title string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	ext := filepath.Ext(full)
	stem := strings.TrimSuffix(full, ext)
	return trimPart(stem)
}

// trimPart — убирает номер части вида " 1", "(Part 2)" и т.п.
func trimPart(s string) string {
	s = strings.TrimSpace(s)
	// число в конце после пробела
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ' ' {
			rest := strings.TrimSpace(s[i+1:])
			if allDigits(rest) {
				s = strings.TrimSpace(s[:i])
				break
			}
		}
	}
	// "(Part 2)"
	lower := strings.ToLower(s)
	if strings.Contains(lower, "part") && strings.HasSuffix(lower, ")") {
		if lid := strings.LastIndex(s, "("); lid >= 0 {
			s = strings.TrimSpace(s[:lid])
		}
	}
	return strings.TrimSpace(s)
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// findCover — обложка из film.json.cover, либо cover.* рядом.
func findCover(dir, name, coverRel string) string {
	rel := strings.TrimSpace(coverRel)
	if rel != "" {
		cand := filepath.Join(dir, rel)
		if info, err := os.Stat(cand); err == nil && info.Mode().IsRegular() {
			return cand
		}
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		cand := filepath.Join(dir, name+ext)
		if info, err := os.Stat(cand); err == nil && info.Mode().IsRegular() {
			return cand
		}
	}
	return ""
}

// sortedFilms — отсортированный срез фильмов.
func (c *FilmCache) List() []Film {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Film, 0, len(c.files))
	for _, f := range c.files {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Get — фильм по id.
func (c *FilmCache) Get(id string) (Film, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	f, ok := c.files[id]
	return f, ok
}

// cover — GET /api/films/cover/<id>
func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	id := filmIDFromPath("/api/films/cover/", r.URL.Path)
	f, found := s.films.Get(id)
	if !found {
		s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "film not found"})
		return
	}
	if f.CoverPath == "" {
		s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "no cover"})
		return
	}
	file, err := os.Open(f.CoverPath)
	if err != nil {
		s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "cannot open cover"})
		return
	}
	defer file.Close()
	mime := mimeByExt[filepath.Ext(f.CoverPath)]
	if mime == "" {
		mime = "image/jpeg"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, file)
}

// stream — GET /api/films/stream/<id>
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	id := filmIDFromPath("/api/films/stream/", r.URL.Path)
	f, found := s.films.Get(id)
	if !found {
		s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "film not found"})
		return
	}
	s.serveContent(w, r, f, f.MimeType)
}

// download — GET /api/films/download/<id>
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	id := filmIDFromPath("/api/films/download/", r.URL.Path)
	f, found := s.films.Get(id)
	if !found {
		s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "film not found"})
		return
	}
	s.serveDownload(w, r, f, f.MimeType, filepath.Base(f.Path))
}

// serveContent — streaming с поддержкой Range.
func (s *Server) serveContent(w http.ResponseWriter, r *http.Request, f Film, mime string) {
	file, err := os.Open(f.Path)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "cannot open"})
		return
	}
	defer file.Close()
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(f.Path), time.Now(), file)
}

// serveDownload — как stream, но attachment.
func (s *Server) serveDownload(w http.ResponseWriter, r *http.Request, f Film, mime string, name string) {
	file, err := os.Open(f.Path)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "cannot open"})
		return
	}
	defer file.Close()
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeContent(w, r, filepath.Base(f.Path), time.Now(), file)
}

// filmIDFromPath — извлекает ID из пути вида /api/films/stream/<id>.
func filmIDFromPath(prefix, path string) string {
	s := strings.TrimPrefix(path, prefix)
	return strings.TrimSpace(strings.Trim(s, "/"))
}
