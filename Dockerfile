# syntax=docker/dockerfile:1
# Multi-stage build for the ticketing API + migrate + seed binaries.

FROM golang:1.24-alpine AS builder
WORKDIR /src
# Cache modules first.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Build all three commands. CGO is off because pgx is pure Go.
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -trimpath -ldflags='-s -w' -o /out/api     ./cmd/api && \
    go build -trimpath -ldflags='-s -w' -o /out/migrate ./cmd/migrate && \
    go build -trimpath -ldflags='-s -w' -o /out/seed    ./cmd/seed

# ---- Runtime stage ----
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

# Copy binaries.
COPY --from=builder /out/api     /usr/local/bin/api
COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY --from=builder /out/seed    /usr/local/bin/seed

# Copy the SQL migrations so cmd/migrate can find them by default.
COPY db /app/db

# Persistent upload directory; the compose file mounts a host volume here.
RUN mkdir -p /app/storage/uploads && chmod 700 /app/storage/uploads

# Run as an unprivileged user.
RUN adduser -D -u 10001 ticketing && chown -R ticketing:ticketing /app
USER ticketing

EXPOSE 8080
ENTRYPOINT ["api"]
