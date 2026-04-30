#!/bin/sh
set -eu

CONFIG_DIR="${QBITTORRENT_CONFIG_DIR:-/config}"
DOWNLOAD_DIR="${QBITTORRENT_DOWNLOAD_DIR:-/downloads}"
WEBUI_PORT="${QBITTORRENT_WEBUI_PORT:-8080}"
LISTEN_PORT="${QBITTORRENT_LISTEN_PORT:-6889}"

mkdir -p "${CONFIG_DIR}/qBittorrent" "${DOWNLOAD_DIR}"
export XDG_CONFIG_HOME="${CONFIG_DIR}"
export XDG_DATA_HOME="${CONFIG_DIR}/data"

CONF_FILE="${CONFIG_DIR}/qBittorrent/qBittorrent.conf"
if [ ! -f "${CONF_FILE}" ]; then
  cat > "${CONF_FILE}" <<EOF
[BitTorrent]
Session\\DefaultSavePath=${DOWNLOAD_DIR}
Session\\Port=${LISTEN_PORT}
Session\\QueueingSystemEnabled=false

[LegalNotice]
Accepted=true

[Preferences]
WebUI\\Address=0.0.0.0
WebUI\\AuthSubnetWhitelist=0.0.0.0/0
WebUI\\AuthSubnetWhitelistEnabled=true
WebUI\\LocalHostAuth=false
WebUI\\Port=${WEBUI_PORT}
WebUI\\ServerDomains=*
EOF
fi

exec qbittorrent-nox \
  --webui-port="${WEBUI_PORT}"
