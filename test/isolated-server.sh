#!/usr/bin/env bash
# Start/stop a throwaway Herdr server for testing this plugin.
#
# The server gets its own XDG_CONFIG_HOME, so it has a private socket, plugin
# registry, plugin config and session state. Nothing here can affect the
# developer's real Herdr session.
#
# The root lives under /tmp rather than a longer scratch path because a unix
# socket path must fit in sun_path (~104 bytes on macOS).
set -euo pipefail

ROOT="${GR_TEST_ROOT:-/tmp/grx}"
SOCK="$ROOT/herdr/herdr.sock"

case "${1:-start}" in
start)
  if [ -S "$SOCK" ] && HERDR_SOCKET_PATH="$SOCK" herdr status server >/dev/null 2>&1; then
    echo "already running"
  else
    rm -rf "$ROOT"
    mkdir -p "$ROOT/herdr"
    printf 'onboarding = false\n' > "$ROOT/herdr/config.toml"
    # Drop the ambient Herdr context so the new server cannot inherit a
    # connection to the real session.
    env -u HERDR_ENV -u HERDR_SOCKET_PATH -u HERDR_PANE_ID \
        -u HERDR_TAB_ID -u HERDR_WORKSPACE_ID \
        XDG_CONFIG_HOME="$ROOT" nohup herdr server >"$ROOT/server.out" 2>&1 &
    for _ in $(seq 1 40); do
      [ -S "$SOCK" ] && break
      sleep 0.25
    done
    if ! [ -S "$SOCK" ]; then
      echo "failed to start:" >&2; cat "$ROOT/server.out" >&2; exit 1
    fi
    echo "started"
  fi
  echo "export HERDR_SOCKET_PATH=$SOCK"
  ;;
stop)
  if [ -S "$SOCK" ]; then
    HERDR_SOCKET_PATH="$SOCK" herdr server stop >/dev/null 2>&1 || true
    sleep 1
  fi
  rm -rf "$ROOT"
  echo "stopped and removed $ROOT"
  ;;
socket)
  echo "$SOCK"
  ;;
*)
  echo "usage: $0 {start|stop|socket}" >&2; exit 2
  ;;
esac
