#!/usr/bin/env bash
# End-to-end tests for herdr-golden-ratio.
#
# Runs against an isolated Herdr server so it never touches the developer's own
# session, layout, plugin registry or config. Start one with:
#
#   test/isolated-server.sh start
#   test/e2e.sh
#   test/isolated-server.sh stop
set -uo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
: "${HERDR_SOCKET_PATH:?set HERDR_SOCKET_PATH to the isolated server socket}"
export HERDR_SOCKET_PATH

# `herdr plugin config-dir` resolves the path client-side from XDG_CONFIG_HOME,
# not from the server it is talking to. Without this the tests would read and
# write the developer's real plugin config while the plugin process, launched by
# the isolated server, reads the isolated one.
if [ -z "${XDG_CONFIG_HOME:-}" ]; then
  # .../<root>/herdr/herdr.sock -> <root>
  XDG_CONFIG_HOME="$(cd "$(dirname "$HERDR_SOCKET_PATH")/.." && pwd)"
  export XDG_CONFIG_HOME
fi
case "$XDG_CONFIG_HOME" in
  "$HOME"/*|"$HOME") echo "refusing to run: XDG_CONFIG_HOME ($XDG_CONFIG_HOME) is inside \$HOME; use test/isolated-server.sh" >&2; exit 2 ;;
esac
echo "using isolated config root: $XDG_CONFIG_HOME"

pass=0
fail=0

ok()   { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass+1)); }
bad()  { printf '  \033[31mFAIL\033[0m %s\n  %s\n' "$1" "$2"; fail=$((fail+1)); }
section() { printf '\n\033[1m%s\033[0m\n' "$1"; }

jqp() { python3 -c "$1"; }

# ratios <pane_id> -> "dir:ratio dir:ratio"
ratios() {
  herdr pane layout --pane "$1" | jqp '
import json,sys
l=json.load(sys.stdin)["result"]["layout"]
print(" ".join("%s:%.3f"%(s["direction"],s["ratio"]) for s in l["splits"]))'
}

focused() {
  herdr pane layout --pane "$1" | jqp '
import json,sys
print(json.load(sys.stdin)["result"]["layout"]["focused_pane_id"])'
}

# fraction <pane_id> <axis> -> pane share of the tab along that axis
fraction() {
  herdr pane layout --pane "$1" | jqp "
import json,sys
l=json.load(sys.stdin)['result']['layout']
ax='$2'
tot=l['area'][ax]
for p in l['panes']:
    if p['pane_id']=='$1':
        print('%.3f'%(p['rect'][ax]/tot)); break"
}

# focus_pane <pane_id> — absolute focus. The CLI only exposes directional focus,
# so this goes straight to the pane.focus socket method.
focus_pane() {
  python3 - "$1" <<'PY'
import json,os,socket,sys
s=socket.socket(socket.AF_UNIX); s.connect(os.environ["HERDR_SOCKET_PATH"])
s.sendall((json.dumps({"id":"t","method":"pane.focus",
    "params":{"pane_id":sys.argv[1]}})+"\n").encode())
b=b""
while not b.endswith(b"\n"): b+=s.recv(65536)
PY
}

set_ratio() {
  python3 - "$1" "$2" "$3" <<'PY'
import json,os,socket,sys
tab,path,ratio=sys.argv[1],json.loads(sys.argv[2]),float(sys.argv[3])
s=socket.socket(socket.AF_UNIX); s.connect(os.environ["HERDR_SOCKET_PATH"])
s.sendall((json.dumps({"id":"t","method":"layout.set_split_ratio",
    "params":{"tab_id":tab,"path":path,"ratio":ratio}})+"\n").encode())
b=b""
while not b.endswith(b"\n"): b+=s.recv(65536)
PY
}

plugin_cfg() {
  local dir; dir="$(herdr plugin config-dir vv.golden-ratio)"
  mkdir -p "$dir"
  if [ -n "${1:-}" ]; then printf '%s\n' "$1" > "$dir/config.toml"
  else rm -f "$dir/config.toml"; fi
}

# The server keeps plugin logs across relinks, so a failure from an earlier run
# would be reported forever. Snapshot the ids already present and ignore them.
SEEN_LOGS="$(herdr plugin log list --plugin vv.golden-ratio --limit 200 2>/dev/null | jqp '
import json,sys
try: logs=json.load(sys.stdin)["result"]["logs"]
except Exception: logs=[]
print(",".join(str(l.get("log_id")) for l in logs))' || echo "")"

# Any non-zero exit or stderr from a plugin invocation since the snapshot fails.
assert_clean_log() {
  local out
  out="$(herdr plugin log list --plugin vv.golden-ratio --limit 200 | SEEN="$SEEN_LOGS" jqp '
import json,os,sys
seen=set(filter(None, os.environ.get("SEEN","").split(",")))
logs=[l for l in json.load(sys.stdin)["result"]["logs"] if str(l.get("log_id")) not in seen]
bad=[l for l in logs
     if l.get("exit_code") not in (0,None) or (l.get("stderr") or "").strip()]
print("%d bad of %d new"%(len(bad),len(logs)) if bad else "clean (%d new)"%len(logs))
for l in bad[:3]: print(" exit=%s %s"%(l.get("exit_code"),(l.get("stderr") or "")[:200]))')"
  case "$out" in
    clean*) ok "$1  ($out)" ;;
    *)      bad "$1" "$out" ;;
  esac
}

# refocus <pane> — guarantee an actual focus transition, so pane.focused fires.
# Focusing a pane that already has focus emits no event.
refocus() {
  local target="$1" other="$2"
  if [ "$(focused "$target")" = "$target" ]; then
    focus_pane "$other"; sleep 0.4
  fi
  focus_pane "$target"
}

trap 'herdr plugin unlink vv.golden-ratio >/dev/null 2>&1' EXIT

section "setup"
herdr plugin unlink vv.golden-ratio >/dev/null 2>&1
herdr plugin link "$PLUGIN_DIR" | jqp '
import json,sys
p=json.load(sys.stdin)["result"]["plugin"]
w=p.get("warnings")
print("linked", p["plugin_id"], "warnings:", w if w else "none")
sys.exit(1 if w else 0)' \
  && ok "manifest links with no warnings" \
  || bad "manifest links with no warnings" "see above"

plugin_cfg ""   # manual mode

# Build a nested tab:  root(right) { A | inner(down) { B / C } }
IDS=$(herdr workspace create --label gr | jqp '
import json,sys
d=json.load(sys.stdin)["result"]
print(d["workspace"]["workspace_id"], d["tab"]["tab_id"], d["root_pane"]["pane_id"])')
read -r WS TAB A <<<"$IDS"
B=$(herdr pane split "$A" --direction right --no-focus | jqp 'import json,sys; print(json.load(sys.stdin)["result"]["pane"]["pane_id"])')
C=$(herdr pane split "$B" --direction down  --no-focus | jqp 'import json,sys; print(json.load(sys.stdin)["result"]["pane"]["pane_id"])')
echo "  workspace=$WS tab=$TAB A=$A B=$B C=$C"

reset() { set_ratio "$TAB" '[]' 0.5; set_ratio "$TAB" '[true]' 0.5; }

section "manual apply: nested pane targets its immediate parent only"
reset
focus_pane "$C"; sleep 0.3
herdr plugin action invoke apply --plugin vv.golden-ratio >/dev/null; sleep 0.8
got="$(ratios "$A")"
[ "$got" = "right:0.500 down:0.382" ] \
  && ok "inner split -> 0.382, root untouched  ($got)" \
  || bad "inner split -> 0.382, root untouched" "got: $got"
f="$(fraction "$C" height)"
awk -v f="$f" 'BEGIN{exit !(f>0.58 && f<0.65)}' \
  && ok "focused pane occupies ~61.8% of its axis  ($f)" \
  || bad "focused pane occupies ~61.8%" "got $f"

section "manual apply: outer pane targets the root split"
reset
focus_pane "$A"; sleep 0.3
herdr plugin action invoke apply --plugin vv.golden-ratio >/dev/null; sleep 0.8
got="$(ratios "$A")"
[ "$got" = "right:0.618 down:0.500" ] \
  && ok "root split -> 0.618, inner untouched  ($got)" \
  || bad "root split -> 0.618, inner untouched" "got: $got"

section "idempotency"
before="$(ratios "$A")"
herdr plugin action invoke apply --plugin vv.golden-ratio >/dev/null; sleep 0.8
after="$(ratios "$A")"
[ "$before" = "$after" ] \
  && ok "re-applying an already-golden split changes nothing  ($after)" \
  || bad "re-applying is a no-op" "before=$before after=$after"

section "pane identity preserved"
now="$(herdr pane layout --pane "$A" | jqp '
import json,sys
print(" ".join(sorted(p["pane_id"] for p in json.load(sys.stdin)["result"]["layout"]["panes"])))')"
want="$(printf '%s\n%s\n%s\n' "$A" "$B" "$C" | sort | tr '\n' ' ' | sed 's/ $//')"
[ "$now" = "$want" ] \
  && ok "same pane ids after resizing  ($now)" \
  || bad "pane ids preserved" "want=$want got=$now"

section "single-pane tab is a clean no-op"
S=$(herdr tab create --workspace "$WS" --label solo --no-focus | jqp '
import json,sys
d=json.load(sys.stdin)["result"]; print(d["tab"]["tab_id"], d["root_pane"]["pane_id"])')
read -r STAB SP <<<"$S"
herdr tab focus "$STAB" >/dev/null 2>&1; sleep 0.3
herdr plugin action invoke apply --plugin vv.golden-ratio >/dev/null; sleep 0.8
assert_clean_log "no error on a tab with nothing to resize"
herdr tab close "$STAB" >/dev/null 2>&1

section "zoomed tab is left alone"
reset
herdr tab focus "$TAB" >/dev/null 2>&1
focus_pane "$C"; sleep 0.2
herdr pane zoom "$C" --on >/dev/null 2>&1; sleep 0.2
herdr plugin action invoke apply --plugin vv.golden-ratio >/dev/null; sleep 0.8
got="$(ratios "$A")"
herdr pane zoom "$C" --off >/dev/null 2>&1
[ "$got" = "right:0.500 down:0.500" ] \
  && ok "ratios unchanged while zoomed  ($got)" \
  || bad "ratios unchanged while zoomed" "got: $got"

section "auto mode: hook fires on focus change with no keypress"
plugin_cfg "$(printf 'auto = true\ndebounce_ms = 80\n')"
focus_pane "$A"; sleep 0.5
reset
refocus "$C" "$A"
sleep 1.5
got="$(ratios "$A")"
[ "$got" = "right:0.500 down:0.382" ] \
  && ok "focus alone triggered the resize  ($got)" \
  || bad "focus alone triggered the resize" "got: $got"

section "auto mode: rapid focus burst coalesces on the final pane"
reset
focus_pane "$C"; focus_pane "$B"; focus_pane "$C"; focus_pane "$A"
sleep 2
fin="$(focused "$A")"
[ "$fin" = "$A" ] && ok "focus settled on the expected pane ($fin)" \
                  || bad "focus settled" "want $A got $fin"
r="$(ratios "$A")"
case "$r" in
  right:0.618*) ok "final pane's split is golden  ($r)" ;;
  *)            bad "final pane's split is golden" "got: $r" ;;
esac
assert_clean_log "no invocation errored during the burst"

section "custom ratio from config"
plugin_cfg "$(printf 'ratio = 0.75\n')"
reset
focus_pane "$A"; sleep 0.3
herdr plugin action invoke apply --plugin vv.golden-ratio >/dev/null; sleep 0.8
got="$(ratios "$A")"
[ "${got%% *}" = "right:0.750" ] \
  && ok "configured ratio honoured  ($got)" \
  || bad "configured ratio honoured" "got: $got"

section "teardown"
plugin_cfg ""
herdr workspace close "$WS" >/dev/null 2>&1 && echo "  closed test workspace"

printf '\n\033[1m%d passed, %d failed\033[0m\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
