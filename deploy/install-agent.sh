#!/bin/sh
# vps-node panel-agent installer.
#
# Usage:
#   PANEL_DOWNLOAD_BASE=https://example.com/downloads/vps-node \
#   PANEL_VERSION=20260920 \
#   PANEL_URL=https://panel.example.com \
#   SERVER_ID=1 \
#   REGISTER_TOKEN=... \
#   sh install-agent.sh
#
# PANEL_DOWNLOAD_BASE and PANEL_VERSION select the release tarball:
#   $PANEL_DOWNLOAD_BASE/panel-agent-$PANEL_VERSION-linux-$ARCH.tar.gz
# The tarball must contain: panel-agent, panel-agent.service, install-agent.sh
set -eu

PANEL_DOWNLOAD_BASE="${PANEL_DOWNLOAD_BASE:-https://example.com/downloads/vps-node}"
PANEL_VERSION="${PANEL_VERSION:-latest}"
PANEL_URL="${PANEL_URL:-}"
SERVER_ID="${SERVER_ID:-}"
REGISTER_TOKEN="${REGISTER_TOKEN:-}"

BIN_DIR=/usr/local/bin
ETC_DIR=/etc/panel-agent
STATE_DIR=/var/lib/panel-agent
SERVICE=panel-agent.service

case "$(uname -m)" in
    x86_64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    i386|i686) ARCH=386 ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ "$(id -u)" -ne 0 ]; then
    echo "must run as root" >&2
    exit 1
fi

TARBALL="panel-agent-$PANEL_VERSION-linux-$ARCH.tar.gz"
URL="$PANEL_DOWNLOAD_BASE/$TARBALL"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "downloading $URL"
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$TMP/$TARBALL"
else
    wget -qO "$TMP/$TARBALL" "$URL"
fi
tar -xzf "$TMP/$TARBALL" -C "$TMP"

install -m 0755 "$TMP/panel-agent" "$BIN_DIR/panel-agent"

if [ -f "$TMP/$SERVICE" ]; then
    install -m 0644 "$TMP/$SERVICE" "/etc/systemd/system/$SERVICE"
fi

id panel-agent >/dev/null 2>&1 || useradd --system --home-dir "$STATE_DIR" --shell /usr/sbin/nologin panel-agent
mkdir -p "$ETC_DIR" "$STATE_DIR" /etc/sing-box
chown panel-agent:panel-agent "$STATE_DIR"
chmod 0750 "$STATE_DIR"

if [ ! -f "$ETC_DIR/agent.yaml" ]; then
    echo "writing $ETC_DIR/agent.yaml"
    cat > "$ETC_DIR/agent.yaml" <<EOF
panel_url: ${PANEL_URL:-https://panel.example.com}
register_token: ${REGISTER_TOKEN:-paste-register-token-from-server-detail}
server_id: ${SERVER_ID:-1}
state_path: $STATE_DIR/state.json
log_level: info
heartbeat_interval: 30
sync_interval: 30
traffic_interval: 60
singbox:
  config_path: /etc/sing-box/config.json
  check_bin: /usr/local/bin/sing-box
  reload_command: ""
collection:
  traffic: true
  sessions: true
  connection_logs: true
EOF
    chmod 0600 "$ETC_DIR/agent.yaml"
else
    echo "$ETC_DIR/agent.yaml already exists; keeping it"
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable "$SERVICE"
    systemctl restart "$SERVICE"
    systemctl --no-pager --lines=20 status "$SERVICE" || true
else
    echo "systemd not found; start manually: $BIN_DIR/panel-agent -config $ETC_DIR/agent.yaml"
fi

echo "panel-agent installed."
