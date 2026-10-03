package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"

	"media_server/internal"
)

// webFiles — встроенный статический фронтенд.
//
//go:embed web
var webFiles embed.FS

var _ fs.FS

// main — точка входа приложения.
func main() {
	// Путь к config.yaml: из env CONFIG_PATH или из флага -config.
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}
	// Путь к базе данных: из env DB_PATH или из флага -db.
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "media_server.db"
	}

	flag.StringVar(&configPath, "config", configPath, "путь к config.yaml")
	flag.StringVar(&dbPath, "db", dbPath, "путь к SQLite-базе")
	flag.Parse()

	// Загружаем конфигурацию.
	cfg := internal.LoadConfig(configPath)

	// Создаём хранилище (SQLite).
	store, err := internal.NewStore(dbPath)
	if err != nil {
		log.Fatalf("cannot open store: %v", err)
	}

	// Если в config.yaml нет пользователей — создаём администратора
	// для smoke-test: username=admin, password=changeme.
	if len(cfg.Users) == 0 {
		log.Println("no users in config.yaml, seeding default admin (admin / changeme)")
		cfg.Users = []internal.UserSpec{
			{Username: "admin", Password: "changeme", Role: "admin"},
		}
	}
	if err := store.SeedUsers(cfg.Users); err != nil {
		log.Printf("seed users: %v", err)
	}

	// Создаём сервер и встраиваем фронтенд.
	server := internal.NewServer(cfg, store)
	webFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Printf("webFS: %v", err)
	} else {
		internal.SetWebFS(webFS)
	}

	addr := ":" + cfg.ServerPort
	log.Printf("starting media server on %s", addr)
	log.Printf("db path: %s", dbPath)
	log.Printf("movie root: %s", cfg.MovieRoot)

	if err := http.ListenAndServe(addr, server.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}
