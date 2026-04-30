ARG QBITTORRENT_BASE_IMAGE=alpine:3.22

FROM ${QBITTORRENT_BASE_IMAGE}

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG http_proxy
ARG https_proxy
ARG NO_PROXY
ARG no_proxy

RUN apk add --no-cache qbittorrent-nox ca-certificates tzdata

COPY docker/qbittorrent.entrypoint.sh /usr/local/bin/qbittorrent-entrypoint.sh

RUN chmod +x /usr/local/bin/qbittorrent-entrypoint.sh \
    && mkdir -p /config /downloads

ENV QBITTORRENT_CONFIG_DIR=/config \
    QBITTORRENT_DOWNLOAD_DIR=/downloads \
    QBITTORRENT_WEBUI_PORT=8080 \
    QBITTORRENT_LISTEN_PORT=6889 \
    XDG_CONFIG_HOME=/config \
    XDG_DATA_HOME=/config/data

EXPOSE 8080 6889 6889/udp

VOLUME ["/config", "/downloads"]

ENTRYPOINT ["qbittorrent-entrypoint.sh"]
