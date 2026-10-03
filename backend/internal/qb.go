package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

// QB — клиент REST API qBittorrent.
type QB struct {
	client *http.Client
}

func NewQB() *QB {
	return &QB{client: &http.Client{Timeout: 30 * time.Second}}
}

// qbAPIConfig — настройки доступа к qBittorrent.
type qbAPIConfig struct {
	URL      string
	Password string
}

// loadQBConfig — из DB settings, иначе из cfg.
func (s *Server) loadQBConfig() qbAPIConfig {
	cfg := qbAPIConfig{
		URL:      s.cfg.QB.URL,
		Password: s.cfg.QB.Password,
	}
	settings, _ := s.db.ListSettings()
	if v, ok := settings["qb_url"]; ok && v != "" {
		cfg.URL = v
	}
	if v, ok := settings["qb_password"]; ok && v != "" {
		cfg.Password = v
	}
	return cfg
}

// trimTrailingSlash — проверяет, что URL — это http/https.
func trimTrailingSlash(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("empty url")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}
	return s, nil
}

// Upload — загрузить torrent в qBittorrent.
func (qb *QB) Upload(api qbAPIConfig, data []byte) (string, error) {
	base, err := trimTrailingSlash(api.URL)
	if err != nil {
		return "", err
	}
	u := base + "/api/torrents/upload"
	if api.Password != "" {
		u += "?passphrase=" + url.QueryEscape(api.Password)
	}
	var mb bytes.Buffer
	mw := multipart.NewWriter(&mb)
	part, err := mw.CreateFormFile("torrent", "torrent.bin")
	if err != nil {
		return "", err
	}
	if _, werr := part.Write(data); werr != nil {
		return "", werr
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	resp, err := qb.client.Post(u, "multipart/form-data", &mb)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("qb upload failed: %d %s", resp.StatusCode, string(body))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	raw = bytes.TrimSpace(raw)
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		var obj struct {
			Torrent string `json:"torrent"`
		}
		if err := json.Unmarshal(raw, &obj); err == nil {
			id = obj.Torrent
		}
	}
	return id, nil
}

// List — список тороентов.
func (qb *QB) List(api qbAPIConfig) ([]map[string]interface{}, error) {
	base, err := trimTrailingSlash(api.URL)
	if err != nil {
		return nil, err
	}
	u := base + "/api/torrents"
	if api.Password != "" {
		u += "?passphrase=" + url.QueryEscape(api.Password)
	}
	resp, err := qb.client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Status — версия qBittorrent.
func (qb *QB) Status(api qbAPIConfig) (string, error) {
	base, err := trimTrailingSlash(api.URL)
	if err != nil {
		return "", err
	}
	u := base + "/api"
	if api.Password != "" {
		u += "?passphrase=" + url.QueryEscape(api.Password)
	}
	resp, err := qb.client.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Release string `json:"release"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Release, nil
}

// handleListTorrents — GET/POST /api/torrents.
func (s *Server) handleTorrents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	api := s.loadQBConfig()
	if api.URL == "" {
		s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "qBittorrent not configured"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		torrents, err := s.qb.List(api)
		if err != nil {
			log.Printf("qb list: %v", err)
			s.writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"torrents": torrents,
			"ok":       true,
		})
	case http.MethodPost:
		if err := s.handleTorrentUpload(w, r, api); err != nil {
			s.writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": err.Error()})
			return
		}
	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
	}
}

// handleTorrentUpload — POST /api/torrents (multipart).
func (s *Server) handleTorrentUpload(w http.ResponseWriter, r *http.Request, api qbAPIConfig) error {
	const maxUpload int64 = 100 * 1024 * 1024
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		return fmt.Errorf("bad multipart body: %w", err)
	}
	var file multipart.File
	var filename string
	if f, fh, err := r.FormFile("torrent"); err == nil {
		file, filename = f, fh.Filename
	} else if f, fh, err := r.FormFile("file"); err == nil {
		file, filename = f, fh.Filename
	}
	if file == nil {
		return fmt.Errorf("no file field in request")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	id, err := s.qb.Upload(api, data)
	if err != nil {
		return err
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       id,
		"uploaded": true,
		"file":     filename,
	})
	return nil
}

// handleHealthz — GET /api/healthz
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	api := s.loadQBConfig()
	if api.URL == "" {
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"qb":     false,
		})
		return
	}
	version, err := s.qb.Status(api)
	if err != nil {
		log.Printf("qb health: %v", err)
		s.writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"qb":     false,
			"qb_err": err.Error(),
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"qb":     true,
		"qb_ver": version,
	})
}
