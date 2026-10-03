package internal

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	_ "modernc.org/sqlite"
)

const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // KiB (64 MiB), RFC 9106 2nd option
	argon2Threads = 4
	argon2KeyLen  = 32
)

// UserRow — пользователь.
type UserRow struct {
	ID        int
	Username  string
	Password  string
	Role      string
	CreatedAt time.Time
}

// hashPassword — argon2id -> "salt_hex.key_hex".
func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return fmt.Sprintf("%s.%s", hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

// verifyPassword — проверка пароля против "salt_hex.key_hex".
func verifyPassword(password, stored string) error {
	parts := strings.SplitN(stored, ".", 2)
	if len(parts) != 2 {
		return errors.New("invalid hash")
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return err
	}
	key, err := hex.DecodeString(parts[1])
	if err != nil {
		return err
	}
	candidate := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	if !bytes.Equal(candidate, key) {
		return errors.New("wrong password")
	}
	return nil
}

// Store — обёртка над *sql.DB.
type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := initSchema(db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}
	return nil
}

// SeedUsers — создаёт пользователей из config.yaml (если ещё нет).
func (s *Store) SeedUsers(specs []UserSpec) error {
	for _, uspec := range specs {
		username := strings.TrimSpace(uspec.Username)
		if username == "" || uspec.Password == "" {
			continue
		}
		var exists int
		if err := s.db.QueryRow("SELECT 1 FROM users WHERE username = ?", username).Scan(&exists); err == nil {
			continue
		}
		h, err := hashPassword(uspec.Password)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec("INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)",
			username, h, Role(uspec.Role), time.Now().Unix()); err != nil {
			return err
		}
		log.Printf("seeded user %q (role=%s)", username, Role(uspec.Role))
	}
	return nil
}

// Login — проверка логина.
func (s *Store) Login(username, password string) (*UserRow, error) {
	var id int
	var uname, uhash, role string
	var createdUnix int64
	row := s.db.QueryRow("SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?", username)
	if err := row.Scan(&id, &uname, &uhash, &role, &createdUnix); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid login or password")
		}
		return nil, err
	}
	if err := verifyPassword(password, uhash); err != nil {
		return nil, errors.New("invalid login or password")
	}
	return &UserRow{
		ID:        id,
		Username:  uname,
		Password:  uhash,
		Role:      role,
		CreatedAt: time.Unix(createdUnix, 0),
	}, nil
}

// CreateSession — создаёт сессию.
func (s *Store) CreateSession(sid string, userID int) error {
	_, err := s.db.Exec("INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		sid, userID, time.Now().Unix(), time.Now().Add(12*time.Hour).Unix())
	return err
}

// UserBySession — пользователь по сессии (nil, nil — нет сессии).
func (s *Store) UserBySession(sid string) (*UserRow, error) {
	var id int
	var uname, uhash, role string
	var createdUnix, expires int64
	row := s.db.QueryRow(
		"SELECT u.id, u.username, u.password_hash, u.role, u.created_at, se.expires_at "+
			"FROM users u JOIN sessions se ON se.user_id = u.id WHERE se.id = ?", sid)
	if err := row.Scan(&id, &uname, &uhash, &role, &createdUnix, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if time.Now().Unix() > expires {
		s.db.Exec("DELETE FROM sessions WHERE id = ?", sid)
		return nil, nil
	}
	return &UserRow{
		ID:        id,
		Username:  uname,
		Password:  uhash,
		Role:      role,
		CreatedAt: time.Unix(createdUnix, 0),
	}, nil
}

// UserHash — возвращает хеш пароля пользователя.
func (s *Store) UserHash(id int) (string, bool) {
	var h string
	if err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", id).Scan(&h); err != nil {
		return "", false
	}
	return h, true
}

// SetUserPassword — смена пароля.
func (s *Store) SetUserPassword(id int, hash string) error {
	_, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
	return err
}

// InvalidateUserSessionsExcept — сбрасывает все сессии пользователя кроме текущей.
func (s *Store) InvalidateUserSessionsExcept(id int, keepSessionID string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE user_id = ? AND id != ?", id, keepSessionID)
	return err
}

// ListUsers — список всех пользователей.
func (s *Store) ListUsers() ([]UserRow, error) {
	rows, err := s.db.Query("SELECT id, username, password_hash, role, created_at FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserRow
	for rows.Next() {
		var id int
		var uname, uhash, role string
		var createdUnix int64
		if err := rows.Scan(&id, &uname, &uhash, &role, &createdUnix); err != nil {
			return nil, err
		}
		out = append(out, UserRow{
			ID:        id,
			Username:  uname,
			Password:  uhash,
			Role:      role,
			CreatedAt: time.Unix(createdUnix, 0),
		})
	}
	return out, nil
}

// GetUser — пользователь по id.
func (s *Store) GetUser(id int) (*UserRow, error) {
	rows, err := s.db.Query("SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var id2 int
	var uname, uhash, role string
	var createdUnix int64
	if !rows.Next() {
		return nil, errors.New("user not found")
	}
	rows.Scan(&id2, &uname, &uhash, &role, &createdUnix)
	return &UserRow{
		ID:        id2,
		Username:  uname,
		Password:  uhash,
		Role:      role,
		CreatedAt: time.Unix(createdUnix, 0),
	}, nil
}

// CountAdmins — количество админов.
func (s *Store) CountAdmins() (int, error) {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// DeleteUser — удаление пользователя.
func (s *Store) DeleteUser(id int) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// SetUserRole — смена роли.
func (s *Store) SetUserRole(id int, role string) error {
	_, err := s.db.Exec("UPDATE users SET role = ? WHERE id = ?", Role(role), id)
	return err
}

// SetSetting — сохранить настройку.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
	return err
}

// GetSetting — получить настройку.
func (s *Store) GetSetting(key string) (string, bool) {
	var v string
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false
		}
		return "", false
	}
	return v, true
}

// ListSettings — все настройки.
func (s *Store) ListSettings() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// Exec — выполнить SQL-запрос, вернуть количество затронутых строк.
func (s *Store) Exec(query string, args ...interface{}) (int64, error) {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Query — выполнить SELECT, вернуть rows. Вызовер обязан вызвать Close().
func (s *Store) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}
