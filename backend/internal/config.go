package internal

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type qbConfig struct {
	URL      string `yaml:"url"`
	Password string `yaml:"password"`
}

type UserSpec struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Role     string `yaml:"role"`
}

// Config — конфигурация приложения из config.yaml и переменных окружения.
// Переменные окружения имеют приоритет:
//
//	MOVIE_ROOT, SERVER_PORT, REFRESH_SECS, QB_URL, QB_PASSWORD
type Config struct {
	MovieRoot   string   `yaml:"movie_root"`
	ServerPort  string   `yaml:"server_port"`
	RefreshSecs int      `yaml:"refresh_secs"`
	QB          qbConfig `yaml:"qbittorrent"`
	Users       []UserSpec
}

// Role возвращает "admin", если роль admin, иначе "user".
func Role(r string) string {
	if strings.ToLower(strings.TrimSpace(r)) == "admin" {
		return "admin"
	}
	return "user"
}

func LoadConfig(path string) Config {
	c := Config{ServerPort: "8080", RefreshSecs: 300}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return c
		}
		if err := yaml.Unmarshal(data, &c); err != nil {
			// при ошибке парсинга оставляем значения по умолчанию
			return c
		}
	}
	if v := os.Getenv("MOVIE_ROOT"); v != "" {
		c.MovieRoot = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		c.ServerPort = v
	}
	if v := os.Getenv("QB_URL"); v != "" {
		c.QB.URL = v
	}
	if v := os.Getenv("QB_PASSWORD"); v != "" {
		c.QB.Password = v
	}
	if v := os.Getenv("REFRESH_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 5 {
			c.RefreshSecs = n
		}
	}
	return c
}
