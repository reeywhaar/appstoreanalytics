#!/bin/sh
# Runs the demo: appstoreanalytics, built from this checkout, reading an invented App Store
# Connect account from a stand-in for Apple's API. What it read is kept from one run to the next,
# and a browser signed in stays signed in. Ctrl-C stops it. See dev/README.md.
#
#   ./scripts/run_dev.sh              on http://localhost:8080
#   ./scripts/run_dev.sh --fresh      without what earlier runs kept: read anew, signed out
#
# Where it is opened is DEV_URL in .env.dev, which is yours and not the repository's; see
# .env.dev.example. The environment overrides it: DEV_URL=http://localhost:9090 ./scripts/run_dev.sh
set -eu

cd "$(dirname "$0")/.."
# Each KEY=value line of .env.dev, unless the environment has KEY already: what is set when
# running it wins, as with any default. Comments and blank lines are skipped.
if [ -f .env.dev ]; then
  while IFS='=' read -r key value || [ -n "$key" ]; do
    case "$key" in '' | \#* | *[!A-Za-z0-9_]*) continue ;; esac
    eval "[ -n \"\${$key+set}\" ]" || export "$key=$value"
  done <.env.dev
fi
# Served on the address's own port, 80 or 443 when it names none, unless DEV_PORT says another:
# a proxy in front, answering at the address, passing to that port.
url="${DEV_URL:-http://localhost:8080}"
if [ -z "${DEV_PORT:-}" ]; then
  at=${url#*://}
  at=${at%%/*}
  case "$at" in
  *:*) DEV_PORT=${at##*:} ;;
  *) case "$url" in https://*) DEV_PORT=443 ;; *) DEV_PORT=80 ;; esac ;;
  esac
fi
export DEV_URL="$url" DEV_PORT
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
# Asked before starting, which makes the volume: one there already holds what was read.
if docker volume inspect appstoreanalytics-dev_data >/dev/null 2>&1; then
  reading="Catching up with the account…"
else
  reading="Reading the account for the first time: about a minute…"
fi
compose up --build --detach

echo "$reading"
if ! compose wait seed >/dev/null; then
  compose logs seed
  echo "The demo could not be set up." >&2
  exit 1
fi

echo
echo "Ready: $url"
echo "Sign in as demo, password demodemo. Ctrl-C stops the demo."
echo
# In the background and waited for, so a signal ends the wait at once and the demo goes down.
compose logs --follow --no-log-prefix appstoreanalytics &
logs=$!
wait "$logs"
