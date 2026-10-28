#!/bin/bash
#
# pve-token.sh — Proxmox VE API user/token manager.
#
# NOTE: The Go CLI now has a no-SSH equivalent: `pve access` (create / add-token /
# revoke-token / remove / list). It performs the same pveum operations over the
# REST API (/access/*) without SSHing into a node. Prefer `pve access` for new
# workflows; this script remains for SSH-based or node-local provisioning.
#
# One script covering the full token lifecycle. The --action flag selects the
# scope so there is no silent over-reach (e.g. revoking a token never deletes
# the user, adding a token never recreates the user):
#
#   create        Q3  Create a user (if missing) AND a token, assign role, verify
#   add-token     Q2  Add a token to an EXISTING user (user is not created)
#   revoke-token  Q1  Revoke ONE token (user and other tokens are preserved)
#   expire-token  --  Set or clear the expiration of an EXISTING token
#                     (uses 'pveum user token modify', so the secret VALUE is
#                     preserved — delete + re-add would regenerate it)
#   remove        --  Delete the user (and all its tokens) entirely
#   list          --  List a user's tokens (read-only)
#
# Usage:
#   ./pve-token.sh [OPTIONS] --action <ACTION> [POSITIONAL...]
#
# Options:
#   -a, --action <ACTION>   create | add-token | revoke-token | expire-token |
#                           remove | list (default: create, for backward
#                           compatibility)
#   -n, --node <node>       Proxmox node (SSH target). Required unless --local
#                           or --nodes auto-discovery succeeds.
#   --nodes <n1,n2,...>     Comma-separated node list (uses the first as target;
#                           user/token objects are cluster-wide).
#   -u, --user <name>       Username, or "user@realm" (required for all actions)
#   -t, --token <name>      Token name (required for create/add-token/
#                           revoke-token/expire-token)
#   --realm <realm>         Auth realm: pam, pve, ldap (default: pve)
#                           An explicit --realm overrides any realm embedded in --user.
#   --role <role>           Role to assign at / (default: PVEVMAdmin)
#   --expire-in <DUR>       Token expiration, used by create / add-token /
#                           expire-token: '30d', '12h', '90d', '2w', '1y',
#                           bare seconds, or 'never'. Converted to an absolute
#                           epoch for pveum's --expire (0 = no expiration).
#                           Default when omitted: pveum's default, which
#                           inherits the user's expiry — and since this script
#                           never sets a user expiry, in practice: never.
#                           Applies to the TOKEN only, never the user account.
#   --nodes-audit           Also grant PVEAuditor on /nodes (adds Sys.Audit for
#                           node status, physical disks and SMART). Opt-in.
#                           NOTE: this does NOT enable the ZFS pool report,
#                           cluster health, or HA status — those check Sys.Audit
#                           on '/', not '/nodes'. For those, grant PVEAuditor at
#                           the root path as well:
#                             pveum aclmod / -user '<user>@<realm>' -role PVEAuditor
#   --local                 Run directly on this host (no SSH) — for use ON a node
#   --dry-run               Show commands without executing
#   -h, --help              Show this help
#
# Positional shortcut (no --action needed): the common "create user + token"
# path keeps the fast-script ergonomics:
#   ./pve-token.sh <node> [user] [token]        # == --action create
#   ./pve-token.sh --local [user] [token]       # == --action create, local
#
# Requirements:
#   - SSH root access to a Proxmox node (or run with --local on a node)
#   - jq installed locally (reliable token-value parsing + node discovery)
#
# Examples:
#   # Q3 — create user + token
#   ./pve-token.sh pve1                                 # pve-admin@pam + admin-token
#   ./pve-token.sh pve1 automation prod-token           # custom names
#   ./pve-token.sh --action create --node pve1 --user automation
#
#   # Q2 — add a SECOND token to an existing user
#   ./pve-token.sh --action add-token --node pve1 --user automation --token ci
#
#   # Provision a token that expires in 30 days
#   ./pve-token.sh --action create --node pve1 --user ci --token build --expire-in 30d
#
#   # Update an EXISTING token's expiration (secret value is preserved)
#   ./pve-token.sh --action expire-token --node pve1 --user ci --token build --expire-in 90d
#
#   # Make an existing token non-expiring again
#   ./pve-token.sh --action expire-token --node pve1 --user ci --token build --expire-in never
#
#   # Grant /nodes audit too (ZFS/disk/SMART reporting via the pve CLI)
#   ./pve-token.sh --action create --node pve1 --user snapshot --nodes-audit
#
#   # Q1 — revoke ONE token (user kept)
#   ./pve-token.sh --action revoke-token --node pve1 --user automation --token ci
#
#   # Remove the whole user and all its tokens
#   ./pve-token.sh --action remove --node pve1 --user automation
#
#   # List a user's tokens
#   ./pve-token.sh --action list --node pve1 --user automation
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ---- defaults ---------------------------------------------------------------

ACTION=""
USER_NAME="pve-admin"
TOKEN_NAME="admin-token"
REALM="pve"
ROLE="PVEVMAdmin"
EXPIRE_IN=""
NODES_AUDIT=false
REALM_EXPLICIT=false
USER_INPUT=false
TOKEN_INPUT=false
NODE=""
NODES=""
LOCAL_MODE=false
DRY_RUN=false
SSH_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=10 -o BatchMode=yes"

print_help() {
  sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

# ---- parse flags, collect positionals separately ----------------------------
# Positional args drive the backward-compatible "create" shortcut. Flags can
# appear in any order.

POSITIONAL=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    -a|--action)        [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; ACTION="$2"; shift 2 ;;
    -n|--node)          [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; NODE="$2"; shift 2 ;;
    --nodes)            [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; NODES="$2"; shift 2 ;;
    -u|--user|--user-name) [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; USER_NAME="$2"; USER_INPUT=true; shift 2 ;;
    -t|--token|--token-name) [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; TOKEN_NAME="$2"; TOKEN_INPUT=true; shift 2 ;;
    --realm)            [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; REALM="$2"; REALM_EXPLICIT=true; shift 2 ;;
    --role)             [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; ROLE="$2"; shift 2 ;;
    --expire-in)        [[ $# -ge 2 ]] || { echo -e "${RED}Error: $1 requires an argument${NC}" >&2; exit 2; }; EXPIRE_IN="$2"; shift 2 ;;
    --nodes-audit)      NODES_AUDIT=true; shift ;;
    --local)            LOCAL_MODE=true; shift ;;
    --dry-run)          DRY_RUN=true; shift ;;
    -h|--help)          print_help 0 ;;
    --)                 shift; while [[ $# -gt 0 ]]; do POSITIONAL+=("$1"); shift; done ;;
    -*)                 echo -e "${RED}Unknown option: $1${NC}" >&2; print_help 1 ;;
    *)                  POSITIONAL+=("$1"); shift ;;
  esac
done

# Backward-compatible positional shortcut: <node> [user] [token] => create.
# Flip USER_INPUT/TOKEN_INPUT only when the positional is actually present, so a
# bare <node> (no user) still fails the required-arg check rather than silently
# targeting the default user.
if [[ -z "$ACTION" ]]; then
  if $LOCAL_MODE; then
    # --local with positionals: [user] [token]
    [[ -n "${POSITIONAL[0]:-}" ]] && { USER_NAME="${POSITIONAL[0]}"; USER_INPUT=true; }
    [[ -n "${POSITIONAL[1]:-}" ]] && { TOKEN_NAME="${POSITIONAL[1]}"; TOKEN_INPUT=true; }
  elif [[ ${#POSITIONAL[@]} -ge 1 ]]; then
    NODE="${POSITIONAL[0]}"
    [[ -n "${POSITIONAL[1]:-}" ]] && { USER_NAME="${POSITIONAL[1]}"; USER_INPUT=true; }
    [[ -n "${POSITIONAL[2]:-}" ]] && { TOKEN_NAME="${POSITIONAL[2]}"; TOKEN_INPUT=true; }
  fi
  ACTION="create"
fi

# Resolve node from positionals for explicit --action calls too (so
# `--action list pve1` works the same as `--action list --node pve1`).
# NOTE: $LOCAL_MODE is a boolean string ("true"/"false"); test it as a command,
# not with -z (a non-empty "false" would wrongly short-circuit -z).
if [[ -z "$NODE" ]] && ! $LOCAL_MODE && [[ ${#POSITIONAL[@]} -ge 1 && "$ACTION" != "create" ]]; then
  NODE="${POSITIONAL[0]}"
fi

# Normalize user input: accept "user@realm" via --user (or positional), and
# split the realm out of it. Precedence: explicit --realm > embedded @realm >
# default. Runs once, after both parse paths converge, before USER_ID is built.
if [[ "$USER_NAME" == *@* ]]; then
  embedded_realm="${USER_NAME##*@}"     # part after the last @
  USER_NAME="${USER_NAME%@*}"           # part before the last @
  if ! $REALM_EXPLICIT; then
    REALM="$embedded_realm"
  fi
fi

case "$ACTION" in
  create|add-token|revoke-token|expire-token|remove|list) : ;;
  *) echo -e "${RED}Error: unknown --action '$ACTION'${NC}" >&2; print_help 1 ;;
esac

# Require --user for every action — there is no sensible default target for a
# token operation, so a missing flag must fail loudly, not act on pve-admin.
if ! $USER_INPUT; then
  echo -e "${RED}Error: --user is required for action '$ACTION'.${NC}" >&2
  exit 1
fi
# Require --token for actions that name/create a specific token.
case "$ACTION" in
  create|add-token|revoke-token|expire-token)
    if ! $TOKEN_INPUT; then
      echo -e "${RED}Error: --token is required for action '$ACTION'.${NC}" >&2
      exit 1
    fi
    ;;
esac

# expire-token has nothing to do without an expiration to apply.
if [[ "$ACTION" == "expire-token" ]] && [[ -z "$EXPIRE_IN" ]]; then
  echo -e "${RED}Error: --expire-in is required for action 'expire-token' (use a duration or 'never').${NC}" >&2
  exit 1
fi

# Resolve --expire-in to an absolute epoch — pveum's --expire takes seconds
# since epoch (integer, minimum 0; 0 = no expiration). Accepted input:
# 'never', bare seconds, or <N><s|m|h|d|w|y>. EXPIRE_EPOCH stays empty when
# --expire-in was not given, so pveum's default applies.
EXPIRE_EPOCH=""
EXPIRE_DESC=""
if [[ -n "$EXPIRE_IN" ]]; then
  if [[ "$EXPIRE_IN" == "never" ]]; then
    EXPIRE_EPOCH=0
    EXPIRE_DESC="never"
  elif [[ "$EXPIRE_IN" =~ ^([0-9]+)([smhdwy]?)$ ]]; then
    _n="${BASH_REMATCH[1]}"; _u="${BASH_REMATCH[2]:-}"
    case "$_u" in
      s|"") _s=1 ;;
      m)    _s=60 ;;
      h)    _s=3600 ;;
      d)    _s=86400 ;;
      w)    _s=604800 ;;
      y)    _s=31536000 ;;
    esac
    EXPIRE_EPOCH=$(( $(date +%s) + _n * _s ))
    EXPIRE_DESC="$(date -u -d "@${EXPIRE_EPOCH}" '+%Y-%m-%d %H:%M UTC' 2>/dev/null \
      || date -u -r "${EXPIRE_EPOCH}" '+%Y-%m-%d %H:%M UTC' 2>/dev/null \
      || echo "epoch ${EXPIRE_EPOCH}")"
  else
    echo -e "${RED}Error: invalid --expire-in '${EXPIRE_IN}' (use e.g. 30d, 12h, 90d, 2w, 1y, or never)${NC}" >&2
    exit 2
  fi
fi

USER_ID="${USER_NAME}@${REALM}"
TOKEN_ID="${USER_ID}!${TOKEN_NAME}"

# ---- helpers ----------------------------------------------------------------

# Resolve the target node. Explicit --node wins; else --nodes first entry;
# else auto-discover via the first reachable common node name.
resolve_node() {
  if [[ -n "$NODE" ]]; then
    echo "$NODE"
    return
  fi
  if [[ -n "$NODES" ]]; then
    echo "${NODES%%,*}"
    return
  fi
  if $LOCAL_MODE; then
    echo "localhost"
    return
  fi
  for n in pve pve1 pve1.local proxmox; do
    if ssh $SSH_OPTS "$n" "echo ok" &>/dev/null; then
      echo "$n"
      return
    fi
  done
  echo -e "${RED}Error: could not determine node. Use --node, --nodes, or --local.${NC}" >&2
  exit 1
}

# Run a command locally (--local) or over SSH. $1 = shell command string.
run_cmd() {
  local cmd="$1"
  if $LOCAL_MODE; then
    bash -c "$cmd"
  else
    ssh $SSH_OPTS "root@${NODE}" "$cmd"
  fi
}

# Run a command with a labeled status line (used for visible provisioning steps).
run_labeled() {
  local node="$1"; local cmd="$2"; local desc="$3"
  echo -e "${YELLOW}[$node]${NC} $desc"
  if $DRY_RUN; then
    echo -e "  ${BLUE}[DRY-RUN]${NC} $cmd"
    return 0
  fi
  if run_cmd "$cmd" 2>&1; then
    echo -e "  ${GREEN}✓ Success${NC}"
    return 0
  else
    echo -e "  ${RED}✗ Failed${NC}"
    return 1
  fi
}

# Extract the token secret from `pveum user token add` output. JSON first
# (version-stable), grep fallbacks for older PVE.
extract_token_value() {
  local output="$1"
  local value
  if value=$(echo "$output" | jq -r '.value // empty' 2>/dev/null) && [[ -n "$value" ]]; then
    echo "$value"; return 0
  fi
  echo "$output" \
    | grep -oP 'value\s*│\s*\K\S+' 2>/dev/null \
    || echo "$output" | grep -oP '^\s*value\s*:?\s*\K[0-9a-f-]+' 2>/dev/null \
    || echo "$output" | grep -oP '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' 2>/dev/null \
    || true
}

# Print the token result block + ready-to-use exports. $1 = resolved secret.
print_token_result() {
  local token_value="$1"
  echo -e "\n${GREEN}================================================${NC}"
  echo -e "${GREEN}  API Token Created Successfully${NC}"
  echo -e "${GREEN}================================================${NC}"
  echo ""
  echo -e "  Node:        ${NODE}"
  echo -e "  User:        ${USER_ID}"
  echo -e "  Token:       ${TOKEN_NAME}"
  echo -e "  Token ID:    ${TOKEN_ID}"
  echo -e "  Token Value: ${YELLOW}${token_value}${NC}"
  echo -e "  Role:        ${ROLE}"
  if [[ -n "$EXPIRE_EPOCH" ]]; then
    echo -e "  Expires:     ${EXPIRE_DESC} (epoch ${EXPIRE_EPOCH})"
  fi
  echo ""
  echo -e "${RED}Save the token value now - it cannot be retrieved later!${NC}"
  echo ""
  echo -e "${BLUE}--- Shell exports (add to ~/.bashrc) ---${NC}"
  cat <<EOF
export PVE_HOST="${NODE}"
export PVE_USER="${USER_ID}"
export PVE_TOKEN_NAME="${TOKEN_NAME}"
export PVE_TOKEN_VALUE="${token_value}"
EOF
  echo ""
  echo -e "${BLUE}--- .env file format ---${NC}"
  cat <<EOF
PVE_HOST=${NODE}
PVE_USER=${USER_ID}
PVE_TOKEN_NAME=${TOKEN_NAME}
PVE_TOKEN_VALUE=${token_value}
EOF
  echo ""
  echo -e "${BLUE}--- Quick test ---${NC}"
  echo "  pve vm list"
  echo "  pve node list"
}

# ---- action: create (Q3) ----------------------------------------------------
do_create() {
  local node="$1"
  echo -e "${BLUE}Creating API token on ${node}...${NC}"

  # User: create only if missing.
  if run_cmd "pveum user list" 2>/dev/null | grep -q "${USER_ID}"; then
    echo -e "${YELLOW}User ${USER_ID} already exists, skipping creation${NC}"
  else
    run_labeled "$node" "pveum user add ${USER_ID} --comment 'PVE CLI admin user'" \
      "Creating user ${USER_ID}" || true
  fi

  do_add_token "$node"   # token + role assignment + result block

  # Verify
  echo -e "\n${BLUE}=== Verifying Setup ===${NC}"
  run_labeled "$node" "pveum user list | grep -q ${USER_ID}" "User ${USER_ID} exists"
  run_labeled "$node" "pveum user token list ${USER_ID} | grep -q ${TOKEN_NAME}" "Token ${TOKEN_NAME} exists"
  run_labeled "$node" "pveum acl list | grep -q ${TOKEN_ID}" "ACL entry for token exists"
  if $NODES_AUDIT; then
    run_labeled "$node" "pveum acl list | grep '/nodes' | grep -q PVEAuditor" "PVEAuditor on /nodes granted"
  fi
}

# ---- action: add-token (Q2) -------------------------------------------------
# Adds a token to an EXISTING user. Errors clearly if the user does not exist.
do_add_token() {
  local node="$1"

  # Guard: refuse to create the user in this action.
  if ! $DRY_RUN; then
    if ! run_cmd "pveum user list" 2>/dev/null | grep -q "${USER_ID}"; then
      echo -e "${RED}Error: user ${USER_ID} does not exist. Use --action create to provision it first.${NC}" >&2
      exit 1
    fi
  fi

  echo -e "\n${BLUE}=== Creating API Token for ${USER_ID} ===${NC}"

  # Appended to every 'pveum user token add' variant below; empty = default.
  local expire_arg=""
  [[ -n "$EXPIRE_EPOCH" ]] && expire_arg=" --expire ${EXPIRE_EPOCH}"

  local output token_value
  if $DRY_RUN; then
    run_labeled "$node" \
      "pveum user token add ${USER_ID} ${TOKEN_NAME} --privsep 0${expire_arg}" \
      "Adding token ${TOKEN_NAME}"
    echo -e "  ${YELLOW}[DRY-RUN] token value would be shown here${NC}"
    return 0
  fi

  output=$(run_cmd "pveum user token add ${USER_ID} ${TOKEN_NAME} --privsep 0${expire_arg} --output-format json 2>/dev/null \
    || pveum user token add ${USER_ID} ${TOKEN_NAME} --privsep 0${expire_arg} 2>&1")
  token_value=$(extract_token_value "$output")

  if [[ -z "$token_value" ]]; then
    echo -e "${RED}Failed to parse token value. Raw output:${NC}"
    echo "$output"
    echo ""
    echo -e "${YELLOW}If the token already exists, revoke it first:${NC}"
    echo "  $0 --action revoke-token --node ${node} --user ${USER_NAME} --token ${TOKEN_NAME}"
    exit 1
  fi

  # Assign role to the token (and the user, for ops that check user perms).
  run_labeled "$node" "pveum aclmod / -tokens '${TOKEN_ID}' -roles ${ROLE}" \
    "Assigning ${ROLE} to token at /"
  run_labeled "$node" "pveum aclmod / -users '${USER_ID}' -roles ${ROLE}" \
    "Assigning ${ROLE} to user at /"

  # Opt-in: grant PVEAuditor on /nodes so node status, physical disks and SMART
  # work. PVEAuditor carries Sys.Audit, which the default PVEVMAdmin role lacks.
  #
  # This does NOT cover the ZFS pool report, cluster health, or HA status: those
  # endpoints check Sys.Audit on '/' rather than '/nodes', so they stay 403 until
  # PVEAuditor is granted at the root path too. Verified 2026-08-25 — the ZFS 403
  # body reads "Permission check failed (/, Sys.Audit)" while /disks/list on the
  # same node returns 200 under the identical ACL.
  if $NODES_AUDIT; then
    run_labeled "$node" "pveum aclmod /nodes -tokens '${TOKEN_ID}' -roles PVEAuditor" \
      "Assigning PVEAuditor to token at /nodes (disk reporting)"
    run_labeled "$node" "pveum aclmod /nodes -users '${USER_ID}' -roles PVEAuditor" \
      "Assigning PVEAuditor to user at /nodes (disk reporting)"
  fi

  print_token_result "$token_value"
}

# ---- action: revoke-token (Q1) ----------------------------------------------
# Removes ONE token. The user and all OTHER tokens are preserved.
do_revoke_token() {
  local node="$1"
  echo -e "${RED}Revoking token ${TOKEN_NAME} for ${USER_ID} on ${node}...${NC}"
  echo -e "${YELLOW}(the user and any other tokens are preserved)${NC}"
  run_labeled "$node" "pveum user token remove ${USER_ID} ${TOKEN_NAME}" \
    "Revoking token ${TOKEN_NAME}" || true
  echo -e "\n${GREEN}Done.${NC}"
}

# ---- action: expire-token ---------------------------------------------------
# Sets (or clears) the expiration of an EXISTING token. Uses 'pveum user token
# modify' (PUT /access/users/{userid}/token/{tokenid}) which updates the token
# in place — the secret VALUE is preserved. Delete + re-add would regenerate
# the secret and silently break every client that has it stored.
do_expire_token() {
  local node="$1"
  echo -e "${BLUE}Updating token ${TOKEN_NAME} for ${USER_ID} on ${node}...${NC}"
  echo -e "${YELLOW}(secret value is preserved; only the expiry changes)${NC}"
  run_labeled "$node" \
    "pveum user token modify ${USER_ID} ${TOKEN_NAME} --expire ${EXPIRE_EPOCH}" \
    "Setting expire → ${EXPIRE_DESC} (epoch ${EXPIRE_EPOCH})"
  if ! $DRY_RUN; then
    echo -e "\n${BLUE}Current tokens for ${USER_ID}:${NC}"
    run_cmd "pveum user token list ${USER_ID}" || true
  fi
}

# ---- action: remove ---------------------------------------------------------
# Deletes the user entirely (tokens are removed with it).
do_remove() {
  local node="$1"
  echo -e "${RED}Removing user ${USER_ID} (and all its tokens) on ${node}...${NC}"
  run_labeled "$node" "pveum user delete ${USER_ID}" "Deleting user ${USER_ID}" || true
  echo -e "\n${GREEN}Done.${NC}"
}

# ---- action: list -----------------------------------------------------------
do_list() {
  local node="$1"
  echo -e "${BLUE}Tokens for ${USER_ID} on ${node}:${NC}"
  run_cmd "pveum user token list ${USER_ID}" || true
}

# ---- main -------------------------------------------------------------------

NODE="$(resolve_node)"

echo -e "${BLUE}==============================================${NC}"
echo -e "${BLUE}  pve-token  | action: ${ACTION}  | node: ${NODE}${NC}"
echo -e "${BLUE}==============================================${NC}"

case "$ACTION" in
  create)        do_create        "$NODE" ;;
  add-token)     do_add_token     "$NODE" ;;
  revoke-token)  do_revoke_token  "$NODE" ;;
  expire-token)  do_expire_token  "$NODE" ;;
  remove)        do_remove        "$NODE" ;;
  list)          do_list          "$NODE" ;;
esac
