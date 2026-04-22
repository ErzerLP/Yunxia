FROM golang:1.25-bookworm AS builder

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/yunxia-server ./cmd/server

FROM debian:bookworm-slim AS runner

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
    YUNXIA_DATABASE_DSN=/app/data/database.db \
    YUNXIA_STORAGE_DATA_DIR=/app/data/storage \
    YUNXIA_STORAGE_TEMP_DIR=/app/data/temp \
    YUNXIA_ARIA2_RPC_URL=http://aria2:6800/jsonrpc

EXPOSE 8080

VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["curl", "-fsS", "http://127.0.0.1:8080/api/v1/health"]

ENTRYPOINT ["yunxia-server"]
