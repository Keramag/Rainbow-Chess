# Multi-stage Dockerfile for Rainbow Chess.
# Adapted from ../virusgame, but with NO bot-hoster stage — this platform ships
# no AI/bot opponents (see docs/plans Overview "Out of scope").

# Stage 1: build the Go server (static binary, no CGO so it runs on bare alpine).
FROM golang:1.24-alpine AS go-builder

WORKDIR /build

# Copy module files first so `go mod download` is cached across source changes.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy the backend source (package main + the engine/ subpackage) and build.
COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o rainbow-chess-server .

# Stage 2: minimal runtime image.
FROM alpine:latest

# ca-certificates is harmless to include and future-proofs any outbound HTTPS.
RUN apk --no-cache add ca-certificates

WORKDIR /app

# The server detects the container by the presence of /app/index.html and then
# serves the static frontend from /app and stores its SQLite DB under
# /app/backend/data (see backend/main.go and backend/storage.go).
COPY --from=go-builder /build/rainbow-chess-server .

# Static frontend (no build step): the shell, styles, and the ES-module sources
# loaded via <script type="module">.
COPY index.html style.css ./
COPY js ./js

# Inject the build commit so the footer can show which image is running.
ARG COMMIT_SHA=unknown
RUN sed -i "s/__COMMIT_SHA__/${COMMIT_SHA}/g" /app/index.html

# WebSocket + static HTTP both served on 8080.
EXPOSE 8080

CMD ["./rainbow-chess-server"]
