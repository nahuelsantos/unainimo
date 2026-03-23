# ── Build Stage ──────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build the binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o unanimo-server ./cmd/server

# ── Runtime Stage ─────────────────────────────────────────────────────────────
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/unanimo-server /unanimo-server
COPY --from=builder /build/web /app/web

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["/unanimo-server"]
