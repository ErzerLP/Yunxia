#!/bin/sh
set -eu

CONF_DIR="${ARIA2_CONF_DIR:-/config}"
DOWNLOAD_DIR="${ARIA2_DOWNLOAD_DIR:-/downloads}"
RPC_PORT="${ARIA2_RPC_PORT:-6800}"
LISTEN_PORT="${ARIA2_LISTEN_PORT:-6888}"
MAX_CONCURRENT_DOWNLOADS="${ARIA2_MAX_CONCURRENT_DOWNLOADS:-5}"
SESSION_FILE="${CONF_DIR}/aria2.session"

mkdir -p "${CONF_DIR}" "${DOWNLOAD_DIR}"
touch "${SESSION_FILE}"

set -- \
  --enable-rpc=true \
  --rpc-listen-all=true \
  --rpc-allow-origin-all=true \
  --rpc-listen-port="${RPC_PORT}" \
  --dir="${DOWNLOAD_DIR}" \
  --input-file="${SESSION_FILE}" \
  --save-session="${SESSION_FILE}" \
  --save-session-interval=60 \
  --continue=true \
  --max-concurrent-downloads="${MAX_CONCURRENT_DOWNLOADS}" \
  --min-split-size=10M \
  --split=16 \
  --bt-save-metadata=true \
  --follow-torrent=mem \
  --listen-port="${LISTEN_PORT}" \
  --dht-listen-port="${LISTEN_PORT}" \
  --seed-time=0 \
  --enable-color=false

if [ -n "${ARIA2_RPC_SECRET:-}" ]; then
  set -- "$@" "--rpc-secret=${ARIA2_RPC_SECRET}"
fi

exec aria2c "$@"
