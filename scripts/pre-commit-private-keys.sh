#!/usr/bin/env bash
# Optional git pre-commit hook: refuse commits that stage private keys.
#
# Install (from the repo root):
#   ln -sf ../../scripts/pre-commit-private-keys.sh .git/hooks/pre-commit
# or, if you already have a pre-commit hook, call this script from it.
# (git worktrees: use "$(git rev-parse --git-common-dir)/hooks/pre-commit".)
exec "$(git rev-parse --show-toplevel)/scripts/check-private-keys.sh"
