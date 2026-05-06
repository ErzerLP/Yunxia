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

# qBittorrent 4.6+ may create a random temporary WebUI password on first
# startup, and named Docker volumes can keep an older config after this
# entrypoint changes.  Patch the internal sidecar API settings on every start so
# the backend container can call the Web API without depending on a generated
# password.  The WebUI port is not published by docker-compose.backend.yml.
set_qbt_conf() {
  section="$1"
  key="$2"
  value="$3"
  tmp_file="${CONF_FILE}.tmp"
  QBT_SECTION="${section}" QBT_KEY="${key}" QBT_VALUE="${value}" awk '
    BEGIN {
      section = ENVIRON["QBT_SECTION"]
      key = ENVIRON["QBT_KEY"]
      value = ENVIRON["QBT_VALUE"]
      in_section = 0
      section_seen = 0
      key_written = 0
    }
    $0 == "[" section "]" {
      if (section_seen && !key_written) {
        print key "=" value
        key_written = 1
      }
      in_section = 1
      section_seen = 1
      print
      next
    }
    /^\[.*\]$/ {
      if (in_section && !key_written) {
        print key "=" value
        key_written = 1
      }
      in_section = 0
      print
      next
    }
    in_section && index($0, key "=") == 1 {
      if (!key_written) {
        print key "=" value
        key_written = 1
      }
      next
    }
    { print }
    END {
      if (!section_seen) {
        print ""
        print "[" section "]"
        print key "=" value
      } else if (in_section && !key_written) {
        print key "=" value
      }
    }
  ' "${CONF_FILE}" > "${tmp_file}"
  mv "${tmp_file}" "${CONF_FILE}"
}

set_qbt_conf "BitTorrent" "Session\\DefaultSavePath" "${DOWNLOAD_DIR}"
set_qbt_conf "BitTorrent" "Session\\Port" "${LISTEN_PORT}"
set_qbt_conf "BitTorrent" "Session\\QueueingSystemEnabled" "false"
set_qbt_conf "LegalNotice" "Accepted" "true"
set_qbt_conf "Preferences" "Connection\\PortRangeMin" "${LISTEN_PORT}"
set_qbt_conf "Preferences" "Downloads\\SavePath" "${DOWNLOAD_DIR}"
set_qbt_conf "Preferences" "WebUI\\Address" "0.0.0.0"
set_qbt_conf "Preferences" "WebUI\\AuthSubnetWhitelist" "0.0.0.0/0, ::/0"
set_qbt_conf "Preferences" "WebUI\\AuthSubnetWhitelistEnabled" "true"
set_qbt_conf "Preferences" "WebUI\\CSRFProtection" "false"
set_qbt_conf "Preferences" "WebUI\\HostHeaderValidation" "false"
set_qbt_conf "Preferences" "WebUI\\LocalHostAuth" "false"
set_qbt_conf "Preferences" "WebUI\\Port" "${WEBUI_PORT}"
set_qbt_conf "Preferences" "WebUI\\SecureCookie" "false"
set_qbt_conf "Preferences" "WebUI\\ServerDomains" "*"

exec qbittorrent-nox \
  --webui-port="${WEBUI_PORT}"
