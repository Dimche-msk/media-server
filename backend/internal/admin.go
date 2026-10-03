package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleAdminIndex — GET /api/admin (статус + qb)
func (s *Server) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"admin":  true,
	})
}

// parseAdminPath — извлекает подпуть и метод после /api/admin/.
type adminRoute struct {
	method string
}

func (s *Server) handleAdminPrefix(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/")
	switch r.Method {
	case http.MethodGet:
		s.handleAdminGet(w, r, path)
	case http.MethodPost:
		s.handleAdminPost(w, r, path)
	case http.MethodPut:
		s.handleAdminPost(w, r, path)
	case http.MethodDelete:
		s.handleAdminDelete(w, r, path)
	default:
		s.writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
	}
}

func (s *Server) handleAdminGet(w http.ResponseWriter, r *http.Request, path string) {
	switch path {
	case "users":
		users, err := s.db.ListUsers()
		if err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})
	case "qbittorrent":
		settings, _ := s.db.ListSettings()
		qb := map[string]interface{}{
			"url":      "",
			"password": "",
		}
		if v, ok := settings["qb_url"]; ok {
			qb["url"] = v
		}
		if v, ok := settings["qb_password"]; ok {
			qb["password"] = v
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"qb": qb})
	case "status":
		api := s.loadQBConfig()
		version, err := s.qb.Status(api)
		resp := map[string]interface{}{
			"qb_ok": api.URL != "",
			"qb":    version,
		}
		if err != nil {
			resp["qb_err"] = err.Error()
		}
		s.writeJSON(w, http.StatusOK, resp)
	default:
		s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "unknown route"})
	}
}

func (s *Server) handleAdminPost(w http.ResponseWriter, r *http.Request, path string) {
	switch path {
	case "refresh":
		if err := s.films.scan(); err != nil {
			log.Printf("refresh: %v", err)
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"refreshed": true})
	case "qbittorrent":
		var in struct {
			URL      string `json:"url"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		if err := s.db.SetSetting("qb_url", in.URL); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		if err := s.db.SetSetting("qb_password", in.Password); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		// POST /api/admin/users/{id} или /api/admin/users
		s.handleAdminUser(w, r, path, http.MethodPost)
	}
}

func (s *Server) handleAdminDelete(w http.ResponseWriter, r *http.Request, path string) {
	// DELETE /api/admin/users/{id}
	s.handleAdminUser(w, r, path, http.MethodDelete)
}

// handleAdminUser — управление пользователями.
func (s *Server) handleAdminUser(w http.ResponseWriter, r *http.Request, path string, method string) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if method != http.MethodDelete && path != "users" && len(strings.TrimSuffix(path, "/")) > 0 {
		// создаём нового: body содержит username/password/role
		if method == http.MethodPost && path == "users" {
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
				return
			}
			username := strings.TrimSpace(in.Username)
			if username == "" {
				s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "username required"})
				return
			}
			h, err := hashPassword(in.Password)
			if err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if _, err := s.db.Exec("INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)",
				username, h, Role(in.Role), time.Now().Unix()); err != nil {
				if strings.Contains(err.Error(), "UNIQUE") {
					s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "username already exists"})
					return
				}
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			s.writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true})
			return
		}
	}
	// GET/PUT/DELETE /api/admin/users/{id}
	idStr := strings.Trim(path, "/")
	id := 0
	if idStr != "" {
		if _, err := fmt.Sscan(idStr, &id); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "invalid id"})
			return
		}
	}
	switch method {
	case http.MethodGet:
		u, err := s.db.GetUser(id)
		if err != nil {
			s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "user not found"})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"user": u})
	case http.MethodPut:
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		if in.Username != "" {
			// смена имени
			if _, err := s.db.Exec("UPDATE users SET username = ? WHERE id = ?", strings.TrimSpace(in.Username), id); err != nil {
				if strings.Contains(err.Error(), "UNIQUE") {
					s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "username already exists"})
					return
				}
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
		}
		if in.Password != "" {
			h, err := hashPassword(in.Password)
			if err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
			if err := s.db.SetUserPassword(id, h); err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
		}
		// смена роли
		if in.Role != "" {
			// нельзя деградирует последнего админа
			if Role(in.Role) == "user" {
				admins, _ := s.db.CountAdmins()
				current, _ := s.db.GetUser(id)
				if current.Role == "admin" && admins <= 1 {
					s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "cannot demote last admin"})
					return
				}
			}
			if err := s.db.SetUserRole(id, in.Role); err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
				return
			}
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	case http.MethodDelete:
		u, err := s.db.GetUser(id)
		if err != nil {
			s.writeJSON(w, http.StatusNotFound, map[string]interface{}{"error": "user not found"})
			return
		}
		if u.Role == "admin" {
			admins, _ := s.db.CountAdmins()
			if admins <= 1 {
				s.writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "cannot delete last admin"})
				return
			}
		}
		if err := s.db.DeleteUser(id); err != nil {
			s.writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	}
}
