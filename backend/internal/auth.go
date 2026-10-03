package internal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// sessionCookieName — имя cookie сессии
const sessionCookieName = "session"

var (
	rateLimitPerWindow = 5
	rateLimitWindow    = 60 * time.Second
)

// ErrLocked — пользователь заблокирован после серии неудачных попыток.
var ErrLocked = errors.New("account locked")

// RateLimiter — rate limit по IP для логина (простое скользящее окно).
type RateLimiter struct {
	mu sync.Mutex
	m  map[string][]int64
}

func (l *RateLimiter) Allow(ip string) bool {
	now := time.Now().Unix()
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now - int64(rateLimitWindow.Seconds())
	var kept []int64
	for _, t := range l.m[ip] {
		if t > cutoff {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	l.m[ip] = kept
	return len(kept) <= rateLimitPerWindow
}

func (l *RateLimiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().Unix()
	for ip, ts := range l.m {
		if len(ts) > 0 && ts[len(ts)-1] < now-int64(rateLimitWindow.Seconds()) {
			delete(l.m, ip)
		}
	}
}

func newSessionID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic("random: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func setSessionCookie(w http.ResponseWriter, sessionID string) {
	c := &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   12 * 60 * 60,
	}
	w.Header().Set("Set-Cookie", c.String())
}

func clearSessionCookie(w http.ResponseWriter) {
	c := &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-time.Hour),
		HttpOnly: true,
	}
	w.Header().Set("Set-Cookie", c.String())
}

func currentSessionID(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

// requireAuth — проверка сессии. Если нет сессии — 401 и true/false.
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (*UserRow, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		clearSessionCookie(w)
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	u, err := s.db.UserBySession(c.Value)
	if err != nil || u == nil {
		clearSessionCookie(w)
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	return u, true
}

// requireAdmin — проверка роли admin.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (*UserRow, bool) {
	u, ok := s.requireAuth(w, r)
	if !ok {
		return nil, false
	}
	if u.Role != "admin" {
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access only"})
		return nil, false
	}
	return u, true
}

// handleLogin — POST /api/auth/login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.rate.Allow(clientIP(r)) {
		s.writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, wait a bit"})
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	username := strings.TrimSpace(in.Username)
	u, err := s.db.Login(username, in.Password)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			s.writeJSON(w, http.StatusLocked, map[string]string{"error": "account locked, wait 15 minutes"})
			return
		}
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong login or password"})
		return
	}
	sid := newSessionID()
	if err := s.db.CreateSession(sid, u.ID); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	setSessionCookie(w, sid)
	s.writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "role": u.Role})
}

// handleLogout — POST /api/auth/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handleMe — GET /api/auth/me
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"username":   u.Username,
		"role":       u.Role,
		"created_at": u.CreatedAt,
	})
}

// handleChangePassword — PUT /api/auth/password
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	var in struct {
		Old     string `json:"old"`
		New     string `json:"new"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	if len(in.New) < 8 || in.New != in.Confirm {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password too short (>= 8) or not confirmed"})
		return
	}
	hash, ok := s.db.UserHash(u.ID)
	if !ok {
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session expired"})
		return
	}
	if err := verifyPassword(in.Old, hash); err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wrong old password"})
		return
	}
	h, err := hashPassword(in.New)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if err := s.db.SetUserPassword(u.ID, h); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	_ = s.db.InvalidateUserSessionsExcept(u.ID, currentSessionID(r))
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
