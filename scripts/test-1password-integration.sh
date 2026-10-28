#!/usr/bin/env bash
#
# test-1password-integration.sh — manual integration test for op:// credential
# resolution in the pve CLI.
#
# This is a MANUAL, OPERATOR-RUN script. It is NOT part of `make test` / CI:
# it requires the 1Password CLI (op / op.exe) to be installed and signed in,
# a real 1Password vault item holding the Proxmox credentials, and a reachable
# Proxmox node with the matching token. Run it on a workstation where those
# are true.
#
# Usage:
#   scripts/test-1password-integration.sh <OP_REF_PREFIX>
#
#   <OP_REF_PREFIX>  the common op:// prefix for the test item, e.g.
#                    "op://SRE/pve-test" — the script appends the field
#                    labels /host, /user, /token_name, /token_value.
#
# Environment overrides (all optional):
#   PVE_BIN          path to the pve binary (default: autodetect, then "pve")
#   SKIP_NOTSIGNED   =1 to skip the "not signed in" subtest (you will be
#                      signed out during it).
#
# Example:
#   scripts/test-1password-integration.sh "op://SRE/pve-test"
#
# Exit codes: 0 = all assertions passed, non-zero = at least one failed.
#
# Safety: this script only runs read-only pve commands (node list). It creates
# no snapshots, backups, or VMs. It does NOT store any resolved secret to disk
# or print it.

set -euo pipefail

# ---- helpers ----------------------------------------------------------------

bin="${PVE_BIN:-}"
if [[ -z "$bin" ]]; then
  if [[ -x ./build/pve ]]; then
    bin="./build/pve"
  elif command -v pve >/dev/null 2>&1; then
    bin="pve"
  else
    echo "FAIL: no pve binary found (build it with 'make build' or set PVE_BIN)" >&2
    exit 2
  fi
fi

OP_PREFIX="${1:-}"
if [[ -z "$OP_PREFIX" ]]; then
  echo "Usage: $0 <OP_REF_PREFIX>   (e.g. op://SRE/pve-test)" >&2
  exit 2
fi

PASS=0
FAIL=0

ok()   { echo "PASS: $*"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $*" >&2; FAIL=$((FAIL + 1)); }

# Resolve a single ref to stdout, or empty on error (does not print secrets —
# this helper is only used to confirm the ref resolves, value is discarded).
ref_is_resolvable() {
  local ref="$1"
  local out
  if ! out=$(op read "$ref" 2>/dev/null); then
    return 1
  fi
  [[ -n "$out" ]]
}

# ---- prerequisite check -----------------------------------------------------

OP_BIN=""
if command -v op.exe >/dev/null 2>&1; then
  OP_BIN="op.exe"
elif command -v op >/dev/null 2>&1; then
  OP_BIN="op"
else
  echo "FAIL: 1Password CLI (op / op.exe) not found on PATH" >&2
  exit 2
fi
echo "# using op binary: $OP_BIN"

# ---- save and trap cleanup of PVE_* env -------------------------------------

declare -A SAVED_ENV=()
save_env() {
  for v in PVE_HOST PVE_USER PVE_PASSWORD PVE_TOKEN_NAME PVE_TOKEN_VALUE; do
    if [[ -n "${!v:-}" ]]; then
      SAVED_ENV["$v"]="${!v}"
    fi
  done
}
restore_env() {
  for v in PVE_HOST PVE_USER PVE_PASSWORD PVE_TOKEN_NAME PVE_TOKEN_VALUE; do
    unset "$v" || true
  done
  for k in "${!SAVED_ENV[@]}"; do
    export "$k=${SAVED_ENV[$k]}"
  done
}
save_env
trap restore_env EXIT

# ---- subtests ---------------------------------------------------------------

echo
echo "=== 2.1 all refs resolve ==="
export PVE_HOST="${OP_PREFIX}/host"
export PVE_USER="${OP_PREFIX}/user"
export PVE_TOKEN_NAME="${OP_PREFIX}/token_name"
export PVE_TOKEN_VALUE="${OP_PREFIX}/token_value"
if "$bin" node list >/dev/null 2>&1; then
  ok "all-op:// credentials authenticated against Proxmox"
else
  fail "all-op:// credentials did not authenticate (check item fields + token ACL)"
fi

echo
echo "=== 2.2 mixed refs + plain ==="
# Keep HOST/USER/TOKEN_NAME as plain literals, resolve only TOKEN_VALUE via op.
# Reuse already-validated plain creds by reading them back from the resolved
# refs above is not safe (they may differ); instead require the operator's
# current saved PVE_HOST/PVE_USER/PVE_TOKEN_NAME as the "plain" set.
restore_env
if [[ -z "${PVE_HOST:-}" || -z "${PVE_USER:-}" || -z "${PVE_TOKEN_NAME:-}" ]]; then
  echo "SKIP: 2.2 needs PVE_HOST/PVE_USER/PVE_TOKEN_NAME in env as plain values"
else
  export PVE_TOKEN_VALUE="${OP_PREFIX}/token_value"
  if "$bin" node list >/dev/null 2>&1; then
    ok "mixed plain + single op:// token authenticated"
  else
    fail "mixed credentials did not authenticate"
  fi
fi

echo
echo "=== 2.3 field-label mismatch surfaces a clear error ==="
restore_env
export PVE_HOST="${PVE_HOST:-placeholder.example}"
export PVE_USER="${PVE_USER:-root@pam}"
export PVE_TOKEN_NAME="${PVE_TOKEN_NAME:-tok}"
export PVE_TOKEN_VALUE="${OP_PREFIX}/__definitely_not_a_field_label__"
# op read for a bogus field should fail OR resolve to empty. pve must then fail
# to start cleanly (either ResolveSecrets errors, or Validate rejects empty).
err_out=$("$bin" node list 2>&1 >/dev/null || true)
if echo "$err_out" | grep -Eqi "op read|resolve|token|1password|invalid configuration|token value"; then
  ok "bogus field produced an actionable startup error"
else
  fail "bogus field produced unexpected/empty error: ${err_out:-<none>}"
fi

echo
echo "=== 2.4 not-signed-in fails before contacting Proxmox ==="
if [[ "${SKIP_NOTSIGNED:-0}" == "1" ]]; then
  echo "SKIP: SKIP_NOTSIGNED=1"
else
  # Sign out for this subtest, then sign back in.
  "$OP_BIN" signout >/dev/null 2>&1 || true
  restore_env
  export PVE_HOST="${PVE_HOST:-placeholder.example}"
  export PVE_USER="${PVE_USER:-root@pam}"
  export PVE_TOKEN_NAME="${PVE_TOKEN_NAME:-tok}"
  export PVE_TOKEN_VALUE="${OP_PREFIX}/token_value"
  err_out=$("$bin" node list 2>&1 >/dev/null || true)
  # Re-authenticate before reporting, so a later subtest or the operator is not left signed out.
  "$OP_BIN" signin >/dev/null 2>&1 || echo "NOTE: please run '$OP_BIN signin' to restore your session" >&2
  if echo "$err_out" | grep -Eqi "op read|resolve|1password|not.*sign|session|authentication required"; then
    ok "unsigned state failed before contacting Proxmox"
  else
    fail "unsigned state produced unexpected error: ${err_out:-<none>}"
  fi
fi

restore_env

echo
echo "=========================================="
echo "Integration results: ${PASS} passed, ${FAIL} failed"
[[ "$FAIL" -eq 0 ]]
