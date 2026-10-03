# syntax=docker/dockerfile:1
#
# Двухэтапная сборка:
# 1) frontend-stage: npm ci + vite build -> /out/web (index.html + assets)
# 2) build-stage:    Go build, встраивая фронтенд в backend/web (go:embed)
# 3) runtime:        distroless static, nonroot
#
# Собирается из корня репозитория:
#   docker build -t media-server .

# ---- frontend ----
FROM node:20-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/src ./src
COPY frontend/index.html frontend/vite.config.js ./
# --outDir перекрывает outDir из vite.config.js, собираем в /out/web
RUN npx vite build --outDir /out/web

# ---- backend (Go) ----
FROM golang:1.26-alpine AS build
WORKDIR /app
# модуль media_server живёт в backend/, поэтому исходники кладём в /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/internal ./internal
COPY backend/main.go .
# go:embed web ожидает директорию web/ рядом с main.go
COPY --from=frontend /out/web ./web
RUN go build -ldflags="-s -w" -o /out/media_server .

# ---- runtime ----
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/media_server /usr/local/bin/media_server
COPY backend/config.example.yaml /config.yaml
ENV CONFIG_PATH=/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/media_server"]
