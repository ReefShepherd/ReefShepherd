#!/usr/bin/env python3
"""Tests for check_generated.py; run with python3 -I scripts/check_generated_test.py."""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import check_generated  # noqa: E402


def query(sql):
    return "const demo = `-- name: Demo :many\n" + sql + "\n`\n"


class CheckGenerated(unittest.TestCase):
    def test_clean_query_passes(self):
        self.assertEqual(check_generated.check(query("SELECT id FROM tanks WHERE id = ?1 AND name = ?2")), [])

    def test_slice_as_last_parameter_passes(self):
        self.assertEqual(check_generated.check(query("SELECT id FROM tanks WHERE note = ?1 AND id IN (/*SLICE:ids*/?)")), [])

    def test_unrewritten_sqlc_call_is_flagged(self):
        for call in ("sqlc.arg(x)", "sqlc.narg(x)", "sqlc.slice(x)", "sqlc.embed(t)"):
            with self.subTest(call=call):
                problems = check_generated.check(query("SELECT id FROM tanks ORDER BY " + call))
                self.assertEqual(len(problems), 1)
                self.assertIn("unrewritten sqlc call", problems[0])

    def test_unrewritten_at_name_is_flagged(self):
        problems = check_generated.check(query("INSERT INTO settings (key, value) VALUES (?1, ?2) ON CONFLICT (key) DO UPDATE SET value = @value"))
        self.assertEqual(len(problems), 1)
        self.assertIn("@name", problems[0])

    def test_number_after_slice_is_flagged(self):
        problems = check_generated.check(query("SELECT id FROM tanks WHERE id IN (/*SLICE:ids*/?) AND name = ?2"))
        self.assertEqual(len(problems), 1)
        self.assertIn("after a slice", problems[0])

    def test_at_sign_inside_a_string_literal_is_not_a_parameter(self):
        self.assertEqual(check_generated.check(query("SELECT id FROM users WHERE email LIKE '%@example.com'")), [])

    def test_marker_line_is_not_inspected(self):
        src = "const demo = `-- name: UsesSqlcArg :one\nSELECT 1\n`\n"
        self.assertEqual(check_generated.check(src), [])


if __name__ == "__main__":
    unittest.main()
