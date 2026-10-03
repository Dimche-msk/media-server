# Задание на разработку: Media Server

## 1. Описание и цель

Веб-приложение «домашний сервер фильмов», работающее в локальном Docker-контейнере с подмонтированным диском с фильмами. Порт приложения проброшен с белого IP (порт-форвардинг), поэтому доступ возможен из интернета.

Цели:
- пользователь из интернета авторизуется по логину/паролю;
- видит список фильмов с диска и опциональные метаданные (комментарий, рейтинг Кинопоиска, постер, год);
- играет фильмы в браузере (перемотка, fullscreen);
- скачивает фильм;
- закидывает торрент-файл в qBittorrent;
- админ управляет пользователями и настройками qBittorrent из веб-интерфейса.

## 2. Архитектура и стек

| Компонент | Технология | Обоснование |
|---|---|---|
| Бэкенд | Go 1.23, stdlib + `modernc.org/sqlite` (чистый Go, без CGo) + `gopkg.in/yaml.v3` | один статичный бинарник, лёгкий образ, встроенный SPA |
| Фронтенд | Svelte 5 + Vite, SPA | `<video>` с нативным seek/fullscreen, лёгкий бандл |
| Хранилище | SQLite (всё: пользователи, сессии, кэш сканирования) | один файл БД, нет внешнего сервиса |
| Конфиг | YAML-файл `/etc/media-server/config.yaml` + переменные окружения (env имеет приоритет над YAML) | настройка qBittorrent, корневой каталог, начальные пользователи |
| qBittorrent | **уже работает** в соседнем контейнере | приложение не поднимает qBittorrent, а подключается к его REST API по URL/паролю из конфига |
| Диск | volume `media:/media/movies:ro` (read-only) | файлы не могут быть изменены приложением |

**Топология:**
```
[интернет] --port (white IP)--> [media-server]
                                        |-- Svelte SPA (embed)
                                        |-- REST API + admin panel
                                        |-- SQLite + config.yaml
                                        \--> [qbittorrent] (already running)
[media disk] ------------------>/media/movies (ro)
```

## 3. Конфигурация

### 3.1 Файл `config.yaml` (внутри образа, или смонтирован как volume):
```yaml
movie_root: /media/movies
server_port: 8080
qbittorrent:
  url: http://qbittorrent:8080/api   # API-URL уже работающего контейнера
  password: your_qb_password
users:
  - username: admin
    password: change_me
    role: admin
  - username: user1
    password: secret1
    role: user
```

### 3.2 Переменные окружения (перезаписывают YAML):
| Переменная | Описание |
|---|---|
| `MOVIE_ROOT` | каталог с фильмами, override `movie_root` |
| `QB_URL` | override `qbittorrent.url` |
| `QB_PASSWORD` | override `qbittorrent.password` |
| `INITIAL_USERNAME` / `INITIAL_PASSWORD` | создание admin при первом запуске, если `users` пуст |
| `ADMIN_TOKEN` | (опционально) secret для автоматизации админ-операций |

### 3.3 При первом запуске:
1. Если `users` в конфиге пуст → создаётся `admin` из `INITIAL_USERNAME/INITIAL_PASSWORD`.
2. Если `users` есть → все записи из YAML добавляются в SQLite (не перезаписывая существующие).
3. Пароли хранятся только как argon2id-хэши (в YAML — plain, они хэшируются при первом запуске).
4. После первого запуска пользователи управляются только через админ-интерфейс.

### 3.4 qBittorrent API
- **Бэкенд** делает `POST {QB_URL}/torrents/upload` с `{torrent: <base64>, password: QB_PASSWORD}`.
- **qBittorrent уже работает** — приложение не управляет его жизненным циклом.
- **Диагностика:** `GET /api/health` возвращает статус подключения к qBittorrent: `qb_connected: true/false`, `qb_version`.
- **Настройка qBittorrent через админ-интерфейс:** поле `url`, `password` → сохраняется в SQLite (перезаписывает YAML). Редирект-к `qbtt` — можно проверить.

## 4. Требования

### 4.1 Развёртывание
- `Dockerfile`: multi-stage (`golang:1.23-alpine` → статичный бинарник на `scratch`).
- `docker-compose.yaml`:
  ```yaml
  services:
    media-server:
      build: .
      environment:
        MOVIE_ROOT: /media/movies
        QB_URL: http://qbittorrent:8080/api   # соседний контейнер
        QB_PASSWORD: ${QB_PASSWORD}
        INITIAL_USERNAME: admin
        INITIAL_PASSWORD: ${INITIAL_PASSWORD}
      ports:
        - "8080:8080"
      volumes:
        - media:/media/movies
        - db:/var/lib/media
        - ./config.yaml:/etc/media-server/config.yaml:ro
      depends_on:
        - qbittorrent   # только чтобы проверить доступность при старте
  ```

### 4.2 Пользователи и аутентификация
- Таблица `users(id, username, password_hash, role, created_at)`.
- Хеш — **argon2id**.
- Роли: `admin` (полный доступ) / `user` (только просмотр, скачивание, торренты).
- Сессия: cookie `session` (httpOnly, SameSite=Lax, TTL 12 ч) + таблица `sessions`.
- Изменение пароля — в настройках профиля (старый → новый).
- Защита: 10 неудачных попыток → 15 мин блокировка; rate limit 5 req/min на `/api/auth/login`.

### 4.3 Админ-интерфейс (`/admin`, только role=admin)
- Список пользователей (username, role, created_at).
- Добавить пользователя (username, password, role).
- Удалить пользователя (не администратора, если он сам).
- Смена роли (admin ↔ user).
- Настройки qBittorrent: `url`, `password`, проверка связи.
- Настройки фильма: `movie_root` (только для перезапуска), `server_port`.
- Статусы: версия, uptime, count фильмов, status qBittorrent.

### 4.4 Каталогизация фильмов
- Рекурсивное сканирование `MOVIE_ROOT` по расширениям: `mkv, mp4, m4v, mov, avi, ts, webm`.
- **Группировка по `film.json`:** если рядом с видеотечением (в той же директории или с тем же base-name) лежит `film.json` → все видео с этим base-name собираются в «фильм» (часть 1/2/3, remaster и т.п.). Иначе — одно видео = один фильм, название из base-name.
- Кэш сканирования: полный пересчёт при старте + событие `inotify` + периодический refresh (env, по умолчанию 5 мин).
- Служащие файлы (`.DS_Store`, `.git`, `*~`) игнорировать.

### 4.5 Метаданные (`film.json`)
Формат рядом с фильмом (создаётся пользователем вручную, опционально):
```json
{
  "title": "Властелин колец: Братство колец",
  "year": 2001,
  "description": "Путь Фродо...",
  "rating": 9.0,
  "comment": "Смотреть с семьёй",
  "genres": ["фэнтези", "боевик"],
  "cover": "poster.jpg"
}
```
- `cover` — относительный путь к изображению внутри папки фильма.
- `rating` — рейтинг Кинопоиска (0–10).
- При отсутствии файла — базовые данные (название из base-name, год из шаблона `\((19|20)\d{2}\)`).

### 4.6 Плеер
- HTML5 `<video controls>` в модалке над списком (fullscreen-по умолчанию на десктопе; весь экран на мобильном).
- **Перемотка и fullscreen** — нативные возможности `<video>`.
- **Кодирование:** без транскодинга. Рекомендация: H.264 (AVC) + MP4/MKV.
- Стриминг: `HTTP Range (206 Partial Content)` — обязанность бэкенда.

### 4.7 Скачивание
- Кнопка «Скачать» → `GET /api/films/{id}/download` → 200 + `Content-Disposition: attachment`.
- Поддерживается Range (пауза/резюмирование).
- Имя файла — оригинальное; кириллица — `filename*=UTF-8''`.

### 4.8 Интеграция с qBittorrent
- Страница «Загрузить торрент» в UI: drag & drop + выбор файла `.torrent`, ≤ 5 МБ.
- Сервер валидирует: расширение, размер, обязательное ключевое поле `info` в Bittorrent-словаре.
- Фронтенд отправляет multipart/form-data → бэкенд → qBittorrent API.
- Ответ: `{id: <qb-torrent-id>}` — показывается пользователю.
- Ошибки: qBittorrent недоступен → HTTP 502 + понятное сообщение; неверный пароль API → 502.
- (Опционально, фаза 3) `GET /api/torrents` (проксирование `/api/torrents` qBittorrent) + таблица активных торрентов в UI.

### 4.9 API (REST)
| Метод | Путь | Назначение |
|---|---|---|
| POST | `/api/auth/login` | `{username, password}` → 200 + Set-Cookie; 401 — неверные; 429 — блокировка |
| POST | `/api/auth/logout` | удаление cookie |
| GET | `/api/auth/me` | текущий пользователь (username, role, created_at) |
| PUT | `/api/auth/password` | смена пароля (role: user/admin) |
| GET | `/api/films` | список фильмов (role: user/admin) |
| GET | `/api/films/{id}/stream` | видеопоток с Range (role: user/admin) |
| GET | `/api/films/{id}/download` | скачивание, Range (role: user/admin) |
| GET | `/api/films/{id}/cover` | постер (kэшируемый, role: user/admin) |
| POST | `/api/torrents` | multipart `torrent` → 201 + `{id}` (role: user/admin) |
| GET | `/api/torrents` | (фаза 3) проксирование qBittorrent (role: admin) |
| GET | `/api/health` | `{version, uptime, movies_count, qb_connected, qb_version}` (публичный) |
| GET | `/admin/users` | список пользователей (role: admin) |
| POST | `/admin/users` | создание пользователя (role: admin) |
| PATCH | `/admin/users/{id}` | смена роли/пароля (role: admin) |
| DELETE | `/admin/users/{id}` | удаление (role: admin) |
| GET/PUT | `/admin/qbittorrent` | настройки qBittorrent (role: admin) |
| GET | `/admin/status` | статусы (version, uptime, movies, qb) (role: admin) |

**Безопасность:**
- Все `/api/*` требуют сессию, кроме `/api/auth/*` и `/api/health`.
- `/admin/*` — только role=admin.
- Защита path traversal (`..`, `\`) — resolve → `strings.HasPrefix(resolved, MOVIE_ROOT + path.Separator)`.

### 4.10 UI (Svelte SPA)
- **Маршруты:** `/` (логин), `/films` (список), `/films/play` (видео), `/torrent` (загрузка), `/settings` (смена пароля), `/admin` (только admin).
- **Список фильмов:** сетка (grid) 3–4 колонки; карточка: постер (16:9, `poster.jpg` или placeholder), название, год, рейтинг (звёзды/цифры 0–10), комментарий (1 строка), кнопки **▶ Play** / **⬇ Download**. Сортировка: по алфавиту, по году, по рейтингу. Поиск по названию.
- **Видео:** `<video controls autoplay>` в модалке; fullscreen по click; при закрытии — `pause()`.
- **Загрузка торрента:** drag & drop зона + progress.
- **Админ:** страницы как в 4.3.
- **Дизайн:** тёмная тема, адаптивность (mobile — 1 колонка, desktop — 4).

## 5. Структура проекта

```
media_server/
├── backend/
│   ├── go.mod
│   ├── embed/                  # Svelte build output (dist)
│   └── internal/
│       ├── main.go             # HTTP mux, config, startup
│       ├── config.go           # YAML parsing, env override
│       ├── auth.go             # login, sessions, argon2id, rate-limit
│       ├── films.go            # scanner, film.json parser, cache
│       ├── qb.go               # qBittorrent client
│       ├── admin.go            # admin handlers
│       ├── store.go            # SQLite (users, sessions, cache)
│       └── static.go           # embed SPA + 404 fallback
├── frontend/
│   ├── package.json
│   ├── svelte.config.js
│   ├── vite.config.js
│   └── src/
│       ├── main.js
│       ├── App.svelte
│       ├── routes/ (films, play, torrent, settings, admin, login)
│       └── components/ (FilmCard, VideoOverlay, TorrentUpload, RatingStars)
├── config.yaml                 # пример конфигурации
├── Dockerfile
├── docker-compose.yaml
├── .gitignore
└── README.md
```

## 6. Docker

### Dockerfile
```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal internal
COPY backend/embed embed
RUN CGO_ENABLED=0 GOOS=linux go build -o media-server -ldflags "-s -w" ./backend

FROM scratch
ADD /build/media-server /media-server
ENTRYPOINT ["/media-server"]
```

### docker-compose.yaml (полный)
```yaml
services:
  media-server:
    build: .
    restart: unless-stopped
    environment:
      MOVIE_ROOT: /media/movies
      QB_URL: http://qbittorrent:8080/api
      QB_PASSWORD: ${QB_PASSWORD}
      INITIAL_USERNAME: ${INITIAL_USERNAME:-admin}
      INITIAL_PASSWORD: ${INITIAL_PASSWORD}
    ports:
      - "8080:8080"
    volumes:
      - media:/media/movies
      - db:/var/lib/media
      - ./config.yaml:/etc/media-server/config.yaml:ro
    networks:
      - media_net

  # qBittorrent уже работает отдельно, его здесь не поднимать
  # Но для compose-файла можно оставить зависимость:
  # depends_on:
  #   - qbittorrent

volumes:
  media:
    bind:
      to: /media/movies
      from: ./media
  db: {}

networks:
  media_net:
    driver: bridge
```

## 7. Критерии приёмки

- [ ] `docker compose up -d` → приложение поднялось, `GET /api/health` → 200
- [ ] `config.yaml` с users → все пользователи создались при первом запуске
- [ ] `POST /api/auth/login` с верными данными → 200 + cookie; с неверными → 401; после 10 попыток → 429
- [ ] В `./media` кладётся `movie1.mkv` + `film.json` → в UI появляется карточка с метаданными
- [ ] Кнопка «Play» → видео играет, перемотка работает без стиксов (HTTP Range), fullscreen включается
- [ ] Кнопка «Download» → файл скачивается с правильным именем, resume работает
- [ ] Загрузка `.torrent` → появляется в списке qBittorrent
- [ ] Запрос `GET /api/films/../secret.txt` → 403
- [ ] Запрос без сессии на `/api/films` → 401
- [ ] Запрос `/admin/users` без роли admin → 403
- [ ] Админ может добавить/удалить пользователя, сменить роль
- [ ] Админ может изменить `QB_URL`/`QB_PASSWORD` в UI → связь проверяется
- [ ] `GET /api/health` → `{qb_connected: true, qb_version: "4.x.x"}`
- [ ] Фильм 1080p H.264 воспроизводится в Chrome/Edge/Firefox/Safari без ошибок
- [ ] Закрытый порт 8080 недоступен из сети без qBittorrent

## 8. Этапы разработки

**Этап 1 — ядро (бэкенд + базовый UI):**
1. `go mod init`, структура, config (YAML + env), SQLite schema, auth (login/logout/me)
2. Файл-сканер + film.json parser + cache
3. Endpoints: `/api/films`, `/stream`, `/download`, `/cover`, `/api/health`
4. Svelte SPA: login, list, video overlay
5. Dockerfile + docker-compose.yaml

**Этап 2 — qBittorrent:**
6. qBittorrent client (connect, upload, health)
7. UI: страница загрузки торрента
8. `GET /api/health` с `qb_connected`

**Этап 3 — Админ:**
9. `/admin` routes (users, qbittorrent settings, status)
10. UI: админ-панель
11. Rate limit, блокировки, защита

**Этап 4 — Доработка:**
12. Опционально: `/api/torrents` + таблица активных торрентов
13. README: инструкция по развёртыванию, пример `film.json`, настройка белого IP
14. Тесты: `go test` для film.json parser, path traversal, range handling, auth
