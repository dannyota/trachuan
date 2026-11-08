#!/usr/bin/env bash
set -Eeuo pipefail

exec python3 - "$@" <<'PY'
import argparse
import os
from pathlib import Path
import re
import tomllib


def toml_string(value):
    escapes = {"\\": "\\\\", '"': '\\"', "\b": "\\b", "\t": "\\t",
               "\n": "\\n", "\f": "\\f", "\r": "\\r"}
    return '"' + "".join(
        escapes.get(char, f"\\u{ord(char):04x}" if ord(char) < 32 or ord(char) == 127 else char)
        for char in value
    ) + '"'


parser = argparse.ArgumentParser(prog="scripts/codex-agent.sh")
parser.add_argument("--model")
parser.add_argument("--effort")
parser.add_argument("--search", action="store_true")
parser.add_argument("role")
parser.add_argument("worktree")
parser.add_argument("brief_file")
parser.add_argument("report_file")
args = parser.parse_args()
if not re.fullmatch(r"[a-z_]+", args.role):
    parser.error("role must match ^[a-z_]+$")

role_file = Path(args.worktree) / ".codex/agents" / f"{args.role}.toml"
try:
    with role_file.open("rb") as source:
        role = tomllib.load(source)
    model = args.model if args.model is not None else role["model"]
    effort = args.effort if args.effort is not None else role["model_reasoning_effort"]
    sandbox = role["sandbox_mode"]
    instructions = role["developer_instructions"]
    if not isinstance(sandbox, str) or sandbox not in {"read-only", "workspace-write"}:
        parser.error("invalid sandbox_mode")
    if not isinstance(effort, str) or effort not in {"low", "medium", "high", "xhigh"}:
        parser.error("invalid model_reasoning_effort")
    if not isinstance(model, str) or not re.fullmatch(r"gpt-[0-9]+(\.[0-9]+)?-[a-z]+", model):
        parser.error("invalid model")
    if not isinstance(instructions, str) or not instructions:
        parser.error("developer_instructions must be a nonempty string")
    brief = Path(args.brief_file).read_text(encoding="utf-8")
except (OSError, ValueError, KeyError) as error:
    parser.error(str(error))

command = ["codex"] + (["--search"] if args.search else []) + [
    "exec", "-m", model, "-c", f"model_reasoning_effort={effort}",
    "-c", "developer_instructions=" + toml_string(instructions),
    "-s", sandbox, "-C", args.worktree, "--ephemeral", "-o", args.report_file, "--", brief,
]
with open(os.devnull, "rb") as source:
    os.dup2(source.fileno(), 0)
os.execvp(command[0], command)
PY
