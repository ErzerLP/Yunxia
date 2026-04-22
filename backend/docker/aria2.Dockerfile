FROM alpine:3.22

RUN apk add --no-cache aria2 ca-certificates tzdata

WORKDIR /app

COPY docker/aria2.entrypoint.sh /usr/local/bin/aria2-entrypoint.sh

RUN chmod +x /usr/local/bin/aria2-entrypoint.sh \
    && mkdir -p /config /downloads

ENV TZ=Asia/Shanghai \
    ARIA2_CONF_DIR=/config \
    ARIA2_DOWNLOAD_DIR=/downloads \
    ARIA2_RPC_PORT=6800 \
    ARIA2_LISTEN_PORT=6888 \
    ARIA2_MAX_CONCURRENT_DOWNLOADS=5

EXPOSE 6800 6888 6888/udp

VOLUME ["/config", "/downloads"]

ENTRYPOINT ["aria2-entrypoint.sh"]
