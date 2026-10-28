# scripts/

Operator-run helper scripts for provisioning Proxmox API credentials and
validating the `pve` CLI's 1Password integration. These are **not** part of
`make test` / CI — they touch live Proxmox nodes and require real access.

## Scripts

| Script | Purpose |
|---|---|
| [`pve-token.sh`](pve-token.sh) | Manage Proxmox API users/tokens: create, add-token, revoke-token, expire-token, remove, list |
| [`test-1password-integration.sh`](test-1password-integration.sh) | Manual end-to-end test of `op://` credential resolution in the `pve` binary |

## Requirements

- **Run from a workstation** with SSH (`root@`) access to at least one Proxmox node
  (or run `pve-token.sh --local` directly on a node).
- `jq` installed locally (token-value parsing + node discovery).
- `openssl` for random password generation (not currently used by the merged script).
- For 1Password testing: the 1Password CLI (`op` / `op.exe`), signed in, plus a
  vault item holding the Proxmox credentials.

## pve-token.sh — the one token manager

A single script covering the full token lifecycle. The `--action` flag selects
the scope so there is **no silent over-reach** — revoking a token never deletes
the user, adding a token never recreates the user.

| `--action` | What it does | Use when |
|---|---|---|
| `create` (default) | Create user (if missing) + token, assign role, verify | Provisioning a brand-new service identity |
| `add-token` | Add a token to an **existing** user (user not created) | You want a second/CI token for an existing user |
| `revoke-token` | Revoke **one** token; user and other tokens preserved | Rotating or retiring a single token |
| `expire-token` | Set/clear the expiration of an **existing** token (secret preserved) | Time-boxing a credential without rotating it |
| `remove` | Delete the user (and all its tokens) entirely | Decommissioning the whole identity |
| `list` | List a user's tokens (read-only) | Inspecting what exists |

### Options

| Flag | Default | Description |
|---|---|---|
| `-a, --action <ACTION>` | `create` | `create \| add-token \| revoke-token \| expire-token \| remove \| list` |
| `-n, --node <node>` | — | Proxmox node (SSH target). Required unless `--nodes`/`--local` resolves one |
| `--nodes <n1,n2,...>` | — | Comma-separated list; first entry is the target (objects are cluster-wide) |
| `-u, --user <name>` | — (required) | Username, or `user@realm` (realm is split out) |
| `-t, --token <name>` | — | Token name; required for `create`/`add-token`/`revoke-token`/`expire-token` |
| `--realm <realm>` | `pve` | Auth realm: `pam`, `pve`, `ldap`. Overrides any realm in `--user` |
| `--role <role>` | `PVEVMAdmin` | Role to assign at `/` |
| `--expire-in <DUR>` | — (never) | Token expiration for `create`/`add-token`/`expire-token`: `30d`, `12h`, `90d`, `2w`, `1y`, bare seconds, or `never`. Converted to an absolute epoch for pveum's `--expire` (0 = never). Token only — the user account never gets an expiry. **Required** for `expire-token` |
| `--local` | off | Run directly on this host (no SSH) — for use ON a node |
| `--dry-run` | off | Print commands without executing |
| `-h, --help` | — | Show help |

### Positional shortcut

The common "create user + token" path keeps the fast-script ergonomics — no
`--action` flag needed:

```bash
./scripts/pve-token.sh <node> [user] [token]      # == --action create
./scripts/pve-token.sh --local [user] [token]     # == --action create, local
```

For the other actions, the node may be given positionally too:
`./scripts/pve-token.sh --action list pve1`.

### Examples

```bash
# --- create: brand-new user + token (Q3) ---
./scripts/pve-token.sh pve1                                  # defaults
./scripts/pve-token.sh pve1 automation prod-token             # custom names
./scripts/pve-token.sh --action create --node pve1 --user automation

# --- add-token: second token for an EXISTING user (Q2) ---
./scripts/pve-token.sh --action add-token --node pve1 --user automation --token ci

# --- expiration: provision a token that expires in 30 days ---
./scripts/pve-token.sh --action create --node pve1 --user ci --token build --expire-in 30d

# --- expire-token: re-time-box an EXISTING token (secret value preserved) ---
./scripts/pve-token.sh --action expire-token --node pve1 --user ci --token build --expire-in 90d
./scripts/pve-token.sh --action expire-token --node pve1 --user ci --token build --expire-in never

# --- revoke-token: kill ONE token, keep the user (Q1) ---
./scripts/pve-token.sh --action revoke-token --node pve1 --user automation --token ci

# --- remove: delete the user and all its tokens ---
./scripts/pve-token.sh --action remove --node pve1 --user automation

# --- list: show a user's tokens ---
./scripts/pve-token.sh --action list --node pve1 --user automation

# --- dry-run any action first ---
./scripts/pve-token.sh --action revoke-token --node pve1 --user automation --token ci --dry-run
```

> The token secret is shown **once** at creation (`create` / `add-token`). Save
> the printed export lines to `~/.bashrc` immediately — it cannot be retrieved
> later.

### How it works

- Uses `pveum user token add ... --privsep 0` (token inherits user perms), then
  assigns the role at the cluster root (`/`).
- Token value is parsed via `--output-format json` + `jq`, with `grep` fallbacks
  for older Proxmox releases.
- `pveum` syntax verified against the
  [PVE Administration Guide](https://github.com/proxmox/pve-docs/blob/master/pve-admin-guide.adoc).

## test-1password-integration.sh

Validates `op://` credential resolution end-to-end: all-op creds, mixed
plain + op creds, a bogus field label, and the signed-out state. Read-only
(runs only `pve node list`); never prints or stores resolved secrets.

```bash
./scripts/test-1password-integration.sh "op://SRE/pve-test"
```

The prefix is the common vault-item path; the script appends the field labels
`/host`, `/user`, `/token_name`, `/token_value`.

**Environment overrides:**

| Var | Description |
|---|---|
| `PVE_BIN` | Path to the `pve` binary (default: `./build/pve`, then `pve` on `PATH`) |
| `SKIP_NOTSIGNED=1` | Skip the subtest that signs you out of 1Password |

Exit code `0` = all assertions passed.

## Note on the old scripts

`create-api-token.sh` and `setup-pve-cli-user.sh` were merged into
`pve-token.sh`. The old scripts' remove/uninstall paths conflated "revoke token"
with "delete user" and had no "add token to existing user" mode — `pve-token.sh`
fixes both with explicit `--action` scopes. A separate `pve-ssh-exec.sh`
multi-node SSH runner was also removed earlier; use
[`parallel-ssh`](https://linux.die.net/man/1/parallel-ssh) for that.
