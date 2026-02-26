#!/usr/bin/env bash
# ForgeVM Spawn Benchmark
# Clears snapshots, does a cold start, then 3 snapshot restores.
# Prints per-step timing breakdown from server logs.

set -euo pipefail

SERVER="${FORGEVM_URL:-http://localhost:7423}"
IMAGE="${FORGEVM_IMAGE:-forgevm/python-sandbox:py3.12}"
MEM="${FORGEVM_MEM:-1024}"
LOG="/tmp/forgevm-bench.log"
SANDBOX_IDS=()

bold="\033[1m"
dim="\033[2m"
cyan="\033[96m"
green="\033[92m"
yellow="\033[93m"
reset="\033[0m"

header() { echo -e "\n${bold}${cyan}── $1 ──${reset}"; }
ok()     { echo -e "  ${green}✓${reset} $1"; }

cleanup_all() {
    for id in "${SANDBOX_IDS[@]}"; do
        curl -sf --max-time 5 -X DELETE "$SERVER/api/v1/sandboxes/$id" >/dev/null 2>&1 || true
    done
}
trap cleanup_all EXIT

# ── Health check ──
header "Health Check"
health=$(curl -sf --max-time 5 "$SERVER/api/v1/health" 2>/dev/null) || {
    echo "  Server not running at $SERVER"; exit 1
}
ok "Server is up"

# ── Kill existing server, clean state, restart ──
header "Preparing Clean Environment"

# Kill existing server & firecracker
pkill -9 -f 'forgevm serve' 2>/dev/null || true
pkill -9 firecracker 2>/dev/null || true
sleep 1

# Clean sandboxes & snapshots
rm -rf /var/lib/forgevm/snapshots/* /var/lib/forgevm/sb-*
ok "Cleared sandboxes & snapshots"

# Find forgevm binary
FORGEVM_BIN="$(dirname "$(readlink -f "$0")")/../forgevm"
if [ ! -f "$FORGEVM_BIN" ]; then
    FORGEVM_BIN="$(which forgevm 2>/dev/null || echo "./forgevm")"
fi
ok "Binary: $FORGEVM_BIN"

# Start fresh server with known log file
> "$LOG"
nohup "$FORGEVM_BIN" serve >> "$LOG" 2>&1 &
SERVER_PID=$!

# Wait for server
for i in $(seq 1 20); do
    if curl -sf --max-time 1 "$SERVER/api/v1/health" >/dev/null 2>&1; then break; fi
    sleep 0.5
done
ok "Server started (PID $SERVER_PID)"

echo -e "\n${bold}${cyan}══════════════════════════════════════════════════════════════${reset}"
echo -e "${bold}  ForgeVM Spawn Benchmark${reset}"
echo -e "${bold}  Image:  ${IMAGE}${reset}"
echo -e "${bold}  Memory: ${MEM}MB${reset}"
echo -e "${bold}${cyan}══════════════════════════════════════════════════════════════${reset}"

# Arrays for final table
declare -a SPAWN_LABELS SPAWN_TOTALS SPAWN_DETAILS

# Get log line count before spawn (to isolate new logs)
log_lines_before() { wc -l < "$LOG"; }

# Extract step timings from log lines after a given offset
extract_steps() {
    local offset=$1
    tail -n +"$((offset+1))" "$LOG" | grep -oP '"spawn: [^"]*"' | \
        sed 's/"spawn: //;s/"//' | head -20
}

spawn_and_time() {
    local label=$1
    local idx=$2
    local before elapsed_ms id steps

    before=$(log_lines_before)

    local start end
    start=$(date +%s%N)
    id=$(curl -sf --max-time 120 -X POST "$SERVER/api/v1/sandboxes" \
        -H "Content-Type: application/json" \
        -d "{\"image\":\"$IMAGE\",\"ttl\":\"5m\",\"memory_mb\":$MEM}" | \
        python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
    end=$(date +%s%N)
    elapsed_ms=$(( (end - start) / 1000000 ))

    SANDBOX_IDS+=("$id")
    sleep 0.5  # let logs flush

    # Parse per-step timings from server log
    local step_lines
    step_lines=$(tail -n +"$((before+1))" "$LOG" | grep '"spawn:' | \
        python3 -c "
import sys, json
for line in sys.stdin:
    try:
        d = json.loads(line.strip())
        msg = d.get('message','')
        if not msg.startswith('spawn:'): continue
        step = msg.replace('spawn: ','')
        # find timing field
        for k in ['elapsed_ms','total_ms']:
            if k in d:
                val = d[k]
                # could be float ms or duration string
                if isinstance(val, (int,float)):
                    print(f'{step}|{val:.1f}ms')
                else:
                    print(f'{step}|{val}')
                break
    except: pass
" 2>/dev/null)

    SPAWN_LABELS[$idx]="$label"
    SPAWN_TOTALS[$idx]="$elapsed_ms"
    SPAWN_DETAILS[$idx]="$step_lines"

    ok "${label}: ${bold}${elapsed_ms}ms${reset}  (${id})"

    # Print step breakdown inline
    if [ -n "$step_lines" ]; then
        echo "$step_lines" | while IFS='|' read -r step timing; do
            printf "    ${dim}%-40s %10s${reset}\n" "$step" "$timing"
        done
    fi
}

# ── Spawn 1: Cold Boot ──
header "Spawn 1 — Cold Boot (no snapshot)"
spawn_and_time "Cold Boot" 0

# Verify pandas works
SB1="${SANDBOX_IDS[0]}"
result=$(curl -sf --max-time 30 -X POST "$SERVER/api/v1/sandboxes/$SB1/exec" \
    -H "Content-Type: application/json" \
    -d '{"command":"python3 -c \"import pandas; print(pandas.__version__)\""}')
version=$(echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin)['stdout'].strip())" 2>/dev/null || echo "?")
pandas_time=$(echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin)['duration'])" 2>/dev/null || echo "?")
ok "Pandas v${version} imported in ${pandas_time}"

# Wait for background snapshot creation
echo -e "  ${dim}Waiting for snapshot creation...${reset}"
sleep 4

# ── Spawns 2-4: Snapshot Restores ──
for i in 1 2 3; do
    header "Spawn $((i+1)) — Snapshot Restore #${i}"
    spawn_and_time "Restore #${i}" "$i"
done

# ═══════════════════════════════════════════
# Final Results Table
# ═══════════════════════════════════════════
echo ""
echo -e "${bold}${cyan}══════════════════════════════════════════════════════════════${reset}"
echo -e "${bold}  Results${reset}"
echo -e "${bold}${cyan}══════════════════════════════════════════════════════════════${reset}"
echo ""

# Header
printf "  ${bold}%-14s │" "SPAWN"
# Collect all unique step names
all_steps=""
for idx in 0 1 2 3; do
    details="${SPAWN_DETAILS[$idx]:-}"
    while IFS='|' read -r step timing; do
        [ -z "$step" ] && continue
        # Shorten step names
        short=$(echo "$step" | sed 's/ API call//;s/ (no snapshot)//;s/ (4 API calls)//')
        if ! echo "$all_steps" | grep -qF "$short"; then
            all_steps="${all_steps}${short}\n"
        fi
    done <<< "$details"
done

# Build column headers from unique steps
columns=()
while IFS= read -r s; do
    [ -z "$s" ] && continue
    columns+=("$s")
    printf " %12s │" "$(echo "$s" | cut -c1-12)"
done < <(echo -e "$all_steps")
printf " %10s${reset}\n" "TOTAL"

# Separator
printf "  ──────────────┼"
for c in "${columns[@]}"; do
    printf "──────────────┼"
done
printf "───────────\n"

# Data rows
for idx in 0 1 2 3; do
    label="${SPAWN_LABELS[$idx]:-}"
    total="${SPAWN_TOTALS[$idx]:-}"
    details="${SPAWN_DETAILS[$idx]:-}"
    [ -z "$label" ] && continue

    printf "  %-14s│" "$label"

    for col in "${columns[@]}"; do
        val="-"
        while IFS='|' read -r step timing; do
            short=$(echo "$step" | sed 's/ API call//;s/ (no snapshot)//;s/ (4 API calls)//')
            if [ "$short" = "$col" ]; then
                val="$timing"
                break
            fi
        done <<< "$details"
        printf " %12s │" "$val"
    done
    printf " ${bold}%8sms${reset}\n" "$total"
done

# Summary
echo ""
if [ ${#SPAWN_TOTALS[@]} -ge 4 ]; then
    sum=0
    for i in 1 2 3; do
        sum=$((sum + SPAWN_TOTALS[i]))
    done
    avg=$((sum / 3))
    speedup=$(echo "scale=1; ${SPAWN_TOTALS[0]} / $avg" | bc 2>/dev/null || echo "?")
    echo -e "  ${bold}Cold boot:     ${SPAWN_TOTALS[0]}ms${reset}"
    echo -e "  ${bold}Avg restore:   ${avg}ms${reset}"
    echo -e "  ${bold}Speedup:       ${speedup}x${reset}"
fi
echo ""
