ARG GO_IMAGE=golang:1.25-bookworm
ARG RUNTIME_IMAGE=debian:bookworm-slim

FROM ${GO_IMAGE} AS builder

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG http_proxy
ARG https_proxy
ARG NO_PROXY
ARG no_proxy

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/yunxia-server ./cmd/server

FROM ${RUNTIME_IMAGE} AS runner

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG http_proxy
ARG https_proxy
ARG NO_PROXY
ARG no_proxy

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

RUN mkdir -p /app/data/storage /app/data/temp

COPY --from=builder /out/yunxia-server /usr/local/bin/yunxia-server

ENV TZ=Asia/Shanghai \
    YUNXIA_SERVER_HOST=0.0.0.0 \
    YUNXIA_SERVER_PORT=8080 \
    YUNXIA_SERVER_MODE=release \
    YUNXIA_DATABASE_DSN=postgres://yunxia:yunxia@postgres:5432/yunxia?sslmode=disable&TimeZone=Asia%2FShanghai \
    YUNXIA_DATABASE_AUTO_MIGRATE=true \
    YUNXIA_DATABASE_MAX_OPEN_CONNS=25 \
    YUNXIA_DATABASE_MAX_IDLE_CONNS=5 \
    YUNXIA_DATABASE_CONN_MAX_LIFETIME=1h \
    YUNXIA_DATABASE_CONN_MAX_IDLE_TIME=10m \
    YUNXIA_DATABASE_SLOW_THRESHOLD=500ms \
    YUNXIA_STORAGE_DATA_DIR=/app/data/storage \
    YUNXIA_STORAGE_TEMP_DIR=/app/data/temp \
    YUNXIA_ARIA2_RPC_URL=http://aria2:6800/jsonrpc

EXPOSE 8080

VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["curl", "-fsS", "http://127.0.0.1:8080/api/v1/health"]

ENTRYPOINT ["yunxia-server"]
