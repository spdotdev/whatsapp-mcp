#!/usr/bin/env bash
# wa-repair — re-pair the WhatsApp MCP bridge when its linked-device session expires.
#
# WhatsApp invalidates linked-device sessions roughly every ~20 days. The systemd
# service can't show a QR (it's non-interactive), so when that happens this script:
#   1. stops the whatsapp-bridge service (releases the lock on the session DB),
#   2. runs the bridge in the foreground so a live, in-place QR code appears,
#   3. restarts the service automatically once you're done (on any exit).
set -u

SERVICE="whatsapp-bridge.service"
BRIDGE_DIR="$HOME/.mcp/whatsapp-mcp/whatsapp-bridge"
BIN="$BRIDGE_DIR/whatsapp-bridge"

restart_service() {
  echo
  echo "==> Restarting ${SERVICE} ..."
  if systemctl --user start "$SERVICE"; then
    sleep 2
    echo "==> Service restarted:"
    systemctl --user --no-pager status "$SERVICE" 2>/dev/null | grep -E "Active:|Main PID:"
  else
    echo "!! Failed to restart ${SERVICE}. Start it manually with:"
    echo "   systemctl --user start ${SERVICE}"
  fi
}

if [ ! -x "$BIN" ]; then
  echo "!! Bridge binary not found / not executable at: $BIN"
  echo "   Build it with: (cd \"$BRIDGE_DIR\" && CGO_ENABLED=1 go build -o whatsapp-bridge .)"
  exit 1
fi

echo "==> Stopping ${SERVICE} (frees the session database) ..."
systemctl --user stop "$SERVICE" 2>/dev/null

# Guarantee the service comes back no matter how we leave (Ctrl+C, error, normal).
trap restart_service EXIT

cat <<'EOF'

==> Starting the bridge interactively.
    1. A QR code appears below and refreshes itself every ~20 seconds.
    2. On your phone:  WhatsApp > Settings > Linked Devices > Link a Device.
    3. Scan whatever QR is CURRENTLY on screen (it's live, so it won't be stale).
    4. Wait for "Successfully authenticated" / "Connected to WhatsApp" and for the
       history sync to finish, then press Ctrl+C to hand control back to the service.

EOF

cd "$BRIDGE_DIR" || exit 1
"$BIN"
