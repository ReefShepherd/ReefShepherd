#!/usr/bin/env bash
# Fails when a frozen migration was edited, deleted or a new one is numbered below the frozen ones; "--freeze" records the unlisted migrations.
set -euo pipefail
cd "$(dirname "$0")/.."

list=scripts/migration-checksums.txt
dir=internal/db/migrations

number() { echo $((10#$(basename "$1" | cut -c1-5))); }

if [[ ${1:-} == --freeze ]]; then
    for file in "$dir"/*.sql; do
        if ! grep -q " $file\$" "$list"; then
            sha256sum "$file" >> "$list"
            echo "frozen: $file"
        fi
    done
    exit 0
fi

status=0
highest=0
declare -A frozen
while read -r sum file; do
    [[ -z $sum || $sum == \#* ]] && continue
    frozen[$file]=1
    if [[ ! -f $file ]]; then
        echo "frozen migration is missing: $file"
        status=1
        continue
    fi
    if [[ $(sha256sum "$file" | cut -d' ' -f1) != "$sum" ]]; then
        echo "frozen migration was changed: $file (add a new migration instead)"
        status=1
    fi
    if (($(number "$file") > highest)); then highest=$(number "$file"); fi
done < "$list"

for file in "$dir"/*.sql; do
    if [[ -z ${frozen[$file]:-} ]] && (($(number "$file") <= highest)); then
        echo "new migration $file is numbered below the frozen migration $highest"
        status=1
    fi
done
exit $status
