#!/usr/bin/env bash
# Runs check-migrations.sh against throwaway migration directories.
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
failures=0
trees=()
trap 'rm -rf "${trees[@]}"' EXIT

# new_tree makes a repo-shaped temp dir with the script, two migrations and an empty checksum list.
new_tree() {
    tree=$(mktemp -d)
    trees+=("$tree")
    mkdir -p "$tree/scripts" "$tree/internal/db/migrations"
    cp "$here/check-migrations.sh" "$tree/scripts/"
    : > "$tree/scripts/migration-checksums.txt"
    echo "CREATE TABLE a (x TEXT);" > "$tree/internal/db/migrations/00001_a.sql"
    echo "CREATE TABLE b (x TEXT);" > "$tree/internal/db/migrations/00002_b.sql"
}

expect() { # expect <name> <want exit> <got exit>
    if [[ $2 != "$3" ]]; then
        echo "FAIL: $1 (exit $3, want $2)"
        failures=$((failures + 1))
    else
        echo "ok: $1"
    fi
}

run() { "$tree/scripts/check-migrations.sh" "$@" > "$tree/out.txt" 2>&1; }

new_tree
run; expect "nothing frozen passes" 0 $?
run --freeze; run; expect "freshly frozen passes" 0 $?
grep -c 00001_a "$tree/scripts/migration-checksums.txt" | grep -qx 1; expect "freeze lists each file once" 0 $?
run --freeze; [[ $(grep -c sql "$tree/scripts/migration-checksums.txt") == 2 ]]; expect "second freeze adds nothing" 0 $?

echo "CREATE TABLE c (x TEXT);" > "$tree/internal/db/migrations/00003_c.sql"
run; expect "a new migration above the frozen ones passes" 0 $?

echo "-- edited" >> "$tree/internal/db/migrations/00001_a.sql"
run; expect "editing a frozen migration fails" 1 $?
grep -q "was changed" "$tree/out.txt"; expect "edit is reported by name" 0 $?

new_tree; run --freeze
rm "$tree/internal/db/migrations/00002_b.sql"
run; expect "deleting a frozen migration fails" 1 $?
grep -q "is missing" "$tree/out.txt"; expect "deletion is reported by name" 0 $?

new_tree; run --freeze
echo "CREATE TABLE z (x TEXT);" > "$tree/internal/db/migrations/00001_inserted.sql"
run; expect "a migration numbered below the frozen ones fails" 1 $?

exit $((failures > 0))
