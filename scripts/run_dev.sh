#!/bin/sh
# Runs the demo: appstoreanalytics, built from this checkout, reading an invented App Store
# Connect account from a stand-in for Apple's API. What it read is kept from one run to the next,
# and a browser signed in stays signed in. Ctrl-C stops it. See dev/README.md.
#
#   ./scripts/run_dev.sh              on http://localhost:8080
#   ./scripts/run_dev.sh --fresh      without what earlier runs kept: read anew, signed out
#   DEV_PORT=9090 ./scripts/run_dev.sh
set -eu

cd "$(dirname "$0")/.."
port="${DEV_PORT:-8080}"
compose() { docker compose -f docker-compose.dev.yml "$@"; }

logs=""
stop() {
  trap - EXIT INT TERM
  [ -n "$logs" ] && kill "$logs" 2>/dev/null
  echo
  echo "Stopping the demo…"
  compose down --remove-orphans >/dev/null 2>&1 || true
}
trap stop EXIT INT TERM

# Anything left from a run that was not stopped goes first, and with --fresh what runs kept too.
if [ "${1:-}" = "--fresh" ]; then
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
else
  compose down --remove-orphans >/dev/null 2>&1 || true
fi
compose up --build --detach

echo "Reading the account: the first time takes half a minute…"
if ! compose wait seed >/dev/null; then
  compose logs seed
  echo "The demo could not be set up." >&2
  exit 1
fi

echo
echo "Ready: http://localhost:$port"
echo "Sign in as demo, password demodemo. Ctrl-C stops the demo."
echo
# In the background and waited for, so a signal ends the wait at once and the demo goes down.
compose logs --follow --no-log-prefix appstoreanalytics &
logs=$!
wait "$logs"
