#!/usr/bin/env bash
# ABOUTME: End-to-end test against a real bbs board and real tmux panes, all throwaway
# ABOUTME: Stand-in agents are `cat` processes; asserts what actually got typed into each pane

set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/tagteam"
TMP="$(mktemp -d)"
SESSION="tagteam-e2e-$$"
export XDG_DATA_HOME="$TMP/data" XDG_CONFIG_HOME="$TMP/config" XDG_STATE_HOME="$TMP/state"
FAIL=0

cleanup() {
  [ -n "${PID:-}" ] && kill "$PID" 2>/dev/null
  tmux kill-session -t "$SESSION" 2>/dev/null
  rm -rf "$TMP"
}
trap cleanup EXIT

pass() { echo "PASS  $1"; }
fail() { echo "FAIL  $1"; FAIL=1; }
pane() { tmux capture-pane -p -t "$SESSION:$1"; }
# Delivered wakes per agent, from tagteam's own trace (panes echo input, so don't count lines there).
count() { grep '"status":"sent"' "$XDG_STATE_HOME/tagteam/trace.jsonl" 2>/dev/null | grep -c "\"agent\":\"$1\"" ; }

go build -o "$BIN" "$ROOT" || exit 1

tmux new-session -d -s "$SESSION" -n claude "exec cat"
tmux new-window -t "$SESSION" -n codex "exec cat"
tmux new-window -t "$SESSION" -n gemini "exec bash --norc"

mkdir -p "$XDG_CONFIG_HOME/tagteam"
cat > "$XDG_CONFIG_HOME/tagteam/config.yaml" <<EOF
poll: 1s
cooldown: 4s
max_wakes_per_hour: 5
agents:
  - name: claude
    target: $SESSION:claude
    aliases: [dembe]
  - name: codex
    target: $SESSION:codex
  - name: gemini
    target: $SESSION:gemini
EOF

bbs topic new e2e >/dev/null
TH="$(bbs --as setup thread new e2e "launch review" | sed -n 's/^ID: //p')"
bbs --as old post "$TH" "@codex this is history and must not replay" >/dev/null

"$BIN" run > "$TMP/run.log" 2>&1 &
PID=$!
sleep 2

[ "$(count codex)" = 0 ] && pass "history is not replayed on first start" || fail "history replayed"

bbs --as "Dembe (Opus)" post "$TH" $'Built the branch.\n\n@codex please review' >/dev/null
sleep 3
[ "$(count codex)" = 1 ] && pass "multi-line post with @codex wakes codex" || fail "codex not woken: $(pane codex)"
pane codex | grep -q 'Dembe (Opus)@cli tagged you in bbs thread "launch review"' && pass "wake names the author and thread" || fail "wake text wrong: $(pane codex)"
[ "$(count claude)" = 0 ] && pass "author (alias dembe) is not woken" || fail "author woken"

bbs --as codex post "$TH" "@claude two nits, then ship" >/dev/null
bbs --as codex post "$TH" "@claude also check the README" >/dev/null
sleep 3
[ "$(count claude)" = 1 ] && pass "two quick tags collapse into one wake" || fail "claude wakes: $(count claude)"

bbs --as claude post "$TH" "@codex fixed one" >/dev/null
sleep 2
[ "$(count codex)" = 2 ] && pass "tag after cooldown wakes immediately" || fail "codex wakes: $(count codex)"
bbs --as claude post "$TH" "@codex fixed two" >/dev/null
sleep 1.5
[ "$(count codex)" = 2 ] && pass "tag during cooldown waits" || fail "cooldown not respected"
sleep 4
[ "$(count codex)" = 3 ] && pass "waiting tag is delivered after cooldown" || fail "queued wake lost: $(count codex)"

bbs --as claude post "$TH" "@gemini can you look" >/dev/null
sleep 3
pane gemini | grep -q "tagteam:" && fail "typed into a bare shell" || pass "refuses to type into a bare shell"
grep -q '"reason":"pane-shell"' "$XDG_STATE_HOME/tagteam/trace.jsonl" && pass "shell refusal is in the trace" || fail "no pane-shell trace"

bbs --as claude post "$TH" 'done, the regex handles `@codex` fine' >/dev/null
sleep 6
[ "$(count codex)" = 3 ] && pass "untagged post and tag in code wake nobody" || fail "false wake"

# MCP: real stdio session, call roster and tag.
MCP_OUT="$( { printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"roster","arguments":{}}}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"tag","arguments":{"thread":"'"$TH"'","to":["claude"],"message":"over to you","agent_name":"codex"}}}' \
  '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"tag","arguments":{"thread":"'"$TH"'","to":["cursor"],"message":"x","agent_name":"codex"}}}'; sleep 2; } | "$BIN" mcp 2>&1 )"
echo "$MCP_OUT" | grep -q '"name":"gemini","pane":"shell"' && pass "mcp roster reports pane state" || fail "roster: $MCP_OUT"
echo "$MCP_OUT" | grep -q '@claude over to you' && pass "mcp tag posts to the board" || fail "tag: $MCP_OUT"
echo "$MCP_OUT" | grep -q 'no agent named \\"cursor\\"' && pass "mcp tag rejects unknown agents" || fail "unknown agent accepted"
sleep 3
[ "$(count claude)" = 2 ] && pass "mcp tag wakes the tagged agent" || fail "mcp tag did not wake claude: $(count claude)"

kill "$PID"; wait "$PID" 2>/dev/null; PID=
grep -c '"event":"wake"' "$XDG_STATE_HOME/tagteam/trace.jsonl" | xargs -I{} echo "trace: {} wake events recorded"

# Hourly cap: no cooldown, two wakes allowed, the third is dropped (it ran 3 for gemini's
# neighbor above, so use a fresh agent window and config).
tmux new-window -t "$SESSION" -n looper "exec cat"
cat > "$XDG_CONFIG_HOME/tagteam/config.yaml" <<EOF
poll: 1s
cooldown: 0s
max_wakes_per_hour: 2
agents:
  - name: looper
    target: $SESSION:looper
EOF
"$BIN" run >> "$TMP/run.log" 2>&1 &
PID=$!
sleep 1.5
for i in 1 2 3; do bbs --as claude post "$TH" "@looper round $i" >/dev/null; sleep 2; done
[ "$(count looper)" = 2 ] && pass "hourly cap stops the third wake" || fail "looper wakes: $(count looper)"
grep '"agent":"looper"' "$XDG_STATE_HOME/tagteam/trace.jsonl" | grep -q '"reason":"hourly-cap"' && pass "capped wake is logged as dropped" || fail "no hourly-cap trace"
kill "$PID"; wait "$PID" 2>/dev/null; PID=

[ "$FAIL" = 0 ] && echo "ALL PASSED" || { echo "--- run log"; cat "$TMP/run.log"; exit 1; }
