#!/usr/bin/env bash
# Spin up one template locally with raw `docker compose`, probe each
# endpoint, print observations, tear down. Used for interactive
# investigation of template correctness — NOT from Playwright.
#
# Usage: probe-template.sh <template-id> [--keep]
#   --keep: leave the stack running for manual inspection
set -euo pipefail

ID="${1:-}"
[ -z "$ID" ] && { echo "usage: probe-template.sh <template-id> [--keep]" >&2; exit 2; }
KEEP=0
[ "${2:-}" = "--keep" ] && KEEP=1

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKDIR="$(mktemp -d -t sd-tpl-probe-XXXX)"
PROJECT="sdtpl-${ID}"

cleanup() {
  if [ "$KEEP" -eq 0 ]; then
    echo "[probe] tearing down..." >&2
    (cd "$WORKDIR" && docker compose -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true)
    rm -rf "$WORKDIR"
  else
    echo "[probe] stack kept at $WORKDIR (project=$PROJECT)" >&2
  fi
}
trap cleanup EXIT

META_JSON="$(node "$SCRIPT_DIR/render-template.js" "$ID" "$WORKDIR/docker-compose.yml")"
echo "[probe] rendered to $WORKDIR/docker-compose.yml" >&2
echo "$META_JSON" | sed 's/^/[meta] /' >&2

(cd "$WORKDIR" && docker compose -p "$PROJECT" up -d --quiet-pull)

echo "[probe] waiting 15s for services to boot..." >&2
sleep 15

# Emit per-endpoint observations as JSON lines.
echo "$META_JSON" | node -e '
const meta = JSON.parse(require("fs").readFileSync(0, "utf8"));
for (const ep of meta.endpoints) console.log(JSON.stringify(ep));
' | while read -r ep; do
  host_port=$(echo "$ep" | node -e 'console.log(JSON.parse(require("fs").readFileSync(0,"utf8")).hostPort)')
  service=$(echo "$ep" | node -e 'console.log(JSON.parse(require("fs").readFileSync(0,"utf8")).service)')
  echo "---" >&2
  echo "[probe] svc=$service port=$host_port" >&2
  # Retry up to 60s
  ok=0
  for i in $(seq 1 30); do
    if curl -fsS -m 3 -o /tmp/probe-body-$$ -w "status=%{http_code} ct=%{content_type} size=%{size_download}\n" "http://127.0.0.1:${host_port}/" 2>/dev/null; then ok=1; break; fi
    code=$(curl -sS -m 3 -o /tmp/probe-body-$$ -w "%{http_code}" "http://127.0.0.1:${host_port}/" 2>/dev/null || echo "000")
    if [ "$code" != "000" ] && [ "$code" != "502" ] && [ "$code" != "504" ]; then
      echo "status=$code (non-success but reachable)"; ok=1; break
    fi
    sleep 2
  done
  if [ "$ok" -eq 1 ] && [ -s /tmp/probe-body-$$ ]; then
    echo "[probe] body first 200 bytes:" >&2
    head -c 200 /tmp/probe-body-$$ >&2; echo >&2
    title=$(grep -oE '<title>[^<]+</title>' /tmp/probe-body-$$ | head -1 || true)
    [ -n "$title" ] && echo "[probe] title: $title" >&2
  elif [ "$ok" -eq 0 ]; then
    echo "[probe] UNREACHABLE after 60s" >&2
    echo "[probe] container logs:" >&2
    (cd "$WORKDIR" && docker compose -p "$PROJECT" logs --tail=40 "$service" 2>&1 | sed 's/^/[log] /') >&2
  fi
  rm -f /tmp/probe-body-$$
done

echo "---" >&2
echo "[probe] ps:" >&2
(cd "$WORKDIR" && docker compose -p "$PROJECT" ps --format json 2>/dev/null | sed 's/^/[ps] /') >&2 || true
