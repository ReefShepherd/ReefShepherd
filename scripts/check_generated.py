#!/usr/bin/env python3
"""Fails when generated sqlc code keeps an unrewritten sqlc call or puts a slice before another numbered parameter."""
import glob
import re
import sys

QUERY_CONST = re.compile(r"const (\w+) = `(.*?)`", re.S)
SQLC_CALL = re.compile(r"\bsqlc\.(arg|narg|slice|embed)\(")
AT_NAME = re.compile(r"(?<!\w)@[a-z_]+")
STRING_LITERAL = re.compile(r"'(?:[^']|'')*'")
NUMBERED_PARAM = re.compile(r"\?\d")


def check(source, path="<source>"):
    """Returns one message per problem found in the generated Go source."""
    problems = []
    for match in QUERY_CONST.finditer(source):
        name, sql = match.group(1), match.group(2)
        # The first line is the "-- name: X :kind" marker that sqlc writes itself.
        body = sql.split("\n", 1)[1] if "\n" in sql else ""
        body = STRING_LITERAL.sub("''", body)
        if SQLC_CALL.search(body):
            problems.append(f"{path}: {name}: unrewritten sqlc call in SQL")
        if AT_NAME.search(body):
            problems.append(f"{path}: {name}: unrewritten @name parameter")
        slice_at = sql.find("/*SLICE:")
        if slice_at != -1 and NUMBERED_PARAM.search(sql[slice_at:]):
            problems.append(f"{path}: {name}: numbered parameter after a slice")
    return problems


def main(paths):
    paths = paths or sorted(glob.glob("internal/db/dbgen/*.sql.go"))
    if not paths:
        print("check_generated: no generated query files found", file=sys.stderr)
        return 1
    problems = []
    for path in paths:
        with open(path, encoding="utf-8") as f:
            problems += check(f.read(), path)
    for problem in problems:
        print(problem)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
