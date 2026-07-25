#!/usr/bin/env bash
# Deletes the most recent synced entry from both the Mastodon and Bluesky
# sqlite DBs so poster.go will re-post it on the next sync.
set -euo pipefail

DIR="${1:-$HOME/.mastosync}"
MAST_DB="$DIR/sync.sqlite3"
SKY_DB="$DIR/skysync.sqlite3"

for db in "$MAST_DB" "$SKY_DB"; do
  echo "== $db =="
  sqlite3 "$db" "SELECT rowid, rssguid, timestamp FROM mastosync ORDER BY rowid DESC LIMIT 1;"
done

read -r -p "Delete the row(s) shown above from both DBs? [y/N] " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
  echo "Aborted."
  exit 1
fi

sqlite3 "$MAST_DB" "DELETE FROM mastosync WHERE rowid = (SELECT MAX(rowid) FROM mastosync);"
sqlite3 "$SKY_DB" "DELETE FROM mastosync WHERE rowid = (SELECT MAX(rowid) FROM mastosync);"

echo "Done. Deleted last entry from both DBs."
