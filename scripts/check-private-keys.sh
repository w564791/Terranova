#!/usr/bin/env bash
# Fail if any tracked file contains a private key.
#
# Checks the git index (what is / will be committed), so it serves both CI
# (index == checked-out commit) and the optional pre-commit hook
# (scripts/pre-commit-private-keys.sh).
#
# Detects:
#   1. PEM headers:  -----BEGIN <...> PRIVATE KEY-----  (RSA, EC, DSA, OPENSSH,
#      ENCRYPTED, PKCS#8, ...)
#   2. The same headers base64-encoded, e.g. tls.key / data fields of
#      Kubernetes Secrets (base64 of "-----BEGIN" starts with LS0tLS1CRUdJTi).
#
# Exclusions: paths listed in scripts/private-key-allowlist.txt (one per line).
# Only add a path there if it is a clearly fake, test-only fixture; prefer
# generating test keys at runtime instead.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

PEM_RE='-----BEGIN [A-Z ]*PRIVATE KEY-----'
B64_RE='LS0tLS1CRUdJTi[A-Za-z0-9+/]+'
ALLOWLIST="scripts/private-key-allowlist.txt"

pathspec=(.)
if [ -f "$ALLOWLIST" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%%#*}"
    line="$(printf '%s' "$line" | tr -d '[:space:]')"
    [ -n "$line" ] && pathspec+=(":(exclude,literal)$line")
  done < "$ALLOWLIST"
fi

found=0

# git grep exits 1 when nothing matches; anything above 1 is a real error.
run_grep() {
  local rc=0
  out=$(git grep --cached -I "$@" -- "${pathspec[@]}") || rc=$?
  if [ "$rc" -gt 1 ]; then echo "git grep failed ($rc)" >&2; exit 2; fi
  return "$rc"
}

# 1. Plain PEM private-key headers.
if run_grep -n -E -e "$PEM_RE"; then
  printf '%s\n' "$out" | while IFS= read -r h; do
    printf 'private key: %s\n' "${h%%:-----*}" >&2
  done
  found=1
fi

# 2. Base64-encoded PEM headers.
if run_grep -o -E -e "$B64_RE"; then
  while IFS= read -r hit; do
    file="${hit%%:*}"
    blob="${hit#*:}"
    blob="${blob:0:64}"                       # header fits in the first 64 chars
    decoded=$(printf '%s' "$blob" | base64 -d 2>/dev/null || true)
    if printf '%s' "$decoded" | grep -Eq -e "$PEM_RE"; then
      printf 'base64-encoded private key: %s\n' "$file" >&2
      found=1
    fi
  done <<< "$out"
fi

if [ "$found" -ne 0 ]; then
  cat >&2 <<'MSG'

Private key material is tracked in git. Remove it (git rm --cached <file>),
generate keys locally (e.g. `make dev-certs`) or at test runtime, and rotate
any key that was ever pushed. See docs/security/private-keys.md.
MSG
  exit 1
fi
echo "OK: no private keys in tracked files"
