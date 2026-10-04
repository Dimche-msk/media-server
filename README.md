# Проект не работает! Попытка разработки локальной моделью Ternary Bonsai2 (QWEN3.8 27B) на RTX5060Ti 16GB 
# модель https://huggingface.co/collections/prism-ml/bonsai-2
# Media Server

Лайт-веб-интерфейс и бэкенд для каталога фильмов и qBittorrent-каталого.

- Go 1.26, один статический бинарник (SQLite, embedded фронтенд)
- Svelte 5 SPA, встроен через `go:embed web`
- Аутентификация (HTTP-only cookie + Argon2id-хэши паролей)
- Список фильмов (скан `MOVIE_ROOT`), стриминг с `Range`, скачивание
- qBittorrent Web-API (graceful degradation при недоступности)
- Docker + docker-compose

## Структура репозитория

```
.
├── backend/
│   ├── main.go              # точка входа, флаги -config, -db, env CONFIG_PATH/DB_PATH
│   ├── internal/
│   │   ├── config.go        # Config из config.yaml + env
│   │   ├── auth.go          # логин, cookie-сессии, Argon2id, requireAuth middleware
│   │   ├── store.go         # SQLite (modernc.org/sqlite), таблицы users, sessions
│   │   ├── films.go         # FilmCache, скан, list/stream/download/cover
│   │   ├── qb.go            # qBittorrent Web-API (list, status)
│   │   ├── admin.go         # /api/admin/users, /api/admin/qbittorrent, settings
│   │   └── server.go        # mux, logging, SPA fallback, static serve
│   ├── web/                 # собранный Svelte фронтенд (встроенный в бинарник)
│   ├── go.mod, go.sum
│   └── config.example.yaml
├── frontend/
│   ├── src/
│   │   ├── App.svelte       # роутер: login / films / admin
│   │   ├── Login.svelte
│   │   ├── Films.svelte
│   │   ├── Admin.svelte
│   │   ├── lib/api.js       # fetch-клиент
│   │   └── main.js
│   ├── index.html
│   ├── vite.config.js       # outDir: ../backend/web
│   └── package.json
├── Dockerfile
├── docker-compose.yaml
├── .dockerignore
└── README.md
```

## Быстрый старт (без Docker)

### Требования

- Go >= 1.26
- Node.js >= 18 (для сборки фронтенда)

### 1. Собрать фронтенд

```bash
cd frontend && npm install && npm run build
```

Результат: `backend/web/index.html` + `backend/web/assets/`.

### 2. Собрать бинарник

```bash
cd backend && go build -o media_server .
```

Результат: `backend/media_server`.

### 3. Запустить

```bash
# минимально: без config.yaml, с дефолтным админом admin/changeme
./media_server -db /tmp/media.db
```

Или с config.yaml:

```bash
cp backend/config.example.yaml config.yaml
# отредактировать movie_root, qbittorrent, users
./media_server -config config.yaml -db media.db
```

Откройте http://127.0.0.1:8080/

## API

| Метод | Путь | Опи |
|-------|------|-----|
| GET | `/` | SPA (HTML + статика) |
| GET | `/assets/*` | статика фронтенда |
| POST | `/api/auth/login` | `{"username":"admin","password":"changeme"}` → cookie `session` |
| POST | `/api/auth/logout` | — |
| GET | `/api/auth/me` | текущий пользователь |
| PUT | `/api/auth/password` | смена пароля `{old,new,confirm}` |
| GET | `/api/films` | список фильмов |
| GET | `/api/films/stream/<id>` | стрим (поддержка `Range: bytes=...`) |
| GET | `/api/films/download/<id>` | скачивание (attachment) |
| GET | `/api/films/cover/<id>` | обложка |
| GET | `/api/torrents` | список qBittorrent-качавок |
| GET | `/api/health`, `/api/healthz` | статус (qb ok/не ok) |
| GET | `/api/admin` | статус админки (qb) |
| GET | `/api/admin/users` | список пользователей |
| POST | `/api/admin/users` | создать `{username,password,role}` |
| GET | `/api/admin/users/<id>` | пользователь |
| PUT | `/api/admin/users/<id>` | обновить `{username?,password?,role?}` |
| DELETE | `/api/admin/users/<id>` | удалить (последнего админа нельзя) |
| GET | `/api/admin/qbittorrent` | qBittorrent-настройки |
| POST | `/api/admin/qbittorrent` | установить qb URL/пароль |
| GET | `/api/admin/status` | qb version/ok |
| POST | `/api/admin/refresh` | рескан фильмов |

Примечания:
- Смена роли защищена: нельзя понизить/удалить **последнего** админа.
- Пароли хранятся в SQLite как Argon2id-хэши.
- Сессии — HTTP-only cookie `session` (random, TTL из конфига).

## Конфигурация

`config.yaml`:

```yaml
movie_root: /movies
server_port: "8080"
refresh_secs: 300
qbittorrent:
  url: "http://qb:8080"
  password: "admin"
users:
  - username: admin
    password: s3cret!
    role: admin
  - username: alice
    password: w4ll00py
    role: user
```

Переменные окружения (приоритет выше config.yaml):

| Env | Опис |
|-----|------|
| `MOVIE_ROOT` | путь к каталогу фильмов |
| `SERVER_PORT` | порт |
| `CONFIG_PATH` | путь к config.yaml |
| `DB_PATH` | путь к SQLite |
| `QB_URL` | qBittorrent URL |
| `QB_PASSWORD` | qBittorrent пароль |

## Docker

### Сборка образа

```bash
docker build -t media-server .
```

### Запуск

```bash
docker run -d \
  --name media-server \
  -p 8080:8080 \
  -v ./movies:/movies \
  -v ./db:/data \
  -e CONFIG_PATH=/config.yaml \
  -e MOVIE_ROOT=/movies \
  --env-file ./env \
  media-server
```

### docker-compose

```bash
# media-server + qBittorrent (опционально)
docker compose up -d
```

## Развёртывание в продакшене

- Замените дефолтный админ `admin / changeme` на своего
- Задайте `MOVIE_ROOT` на реальном каталоге
- Настройте qBittorrent Web-API
- Задайте strong password в `users`
- Уберите debug-лог

## Лицензия

MIT
