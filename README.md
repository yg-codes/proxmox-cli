# pve

Single-binary Proxmox VE management CLI with AWS-style command interface.

Manages VM/container lifecycle, snapshots, backups, nodes, networking — plus
read-only **capacity, ZFS/disk, and cluster-health reporting** via the REST API
(no SSH required).

[![Release](https://img.shields.io/github/v/release/yg-codes/proxmox-cli)](https://github.com/yg-codes/proxmox-cli/releases)
[![Build Status](https://img.shields.io/github/actions/workflow/status/yg-codes/proxmox-cli/ci.yml)](https://github.com/yg-codes/proxmox-cli/actions)
[![Go Version](https://img.shields.io/github/go-mod/go-version/yg-codes/proxmox-cli)](https://github.com/yg-codes/proxmox-cli)

## How-to

### Install

**On a Proxmox node (via `.deb` package):**

Proxmox VE is Debian-based, so install the `.deb` with `dpkg`. The package
places the binary at `/usr/bin/pve` and docs at `/usr/share/doc/pve/`.

Grab the `.deb` for your arch from the [releases page](https://github.com/yg-codes/proxmox-cli/releases/latest)
(named `proxmox-cli_<version>_linux_amd64.deb` — or `_arm64.deb` on ARM), then:

```bash
sudo dpkg -i proxmox-cli_*_linux_amd64.deb

pve --version
```

To upgrade to a newer release, download the new `.deb` and run `dpkg -i` again
over the existing install (it replaces the old version in place — no need to
remove first). To uninstall:

```bash
sudo dpkg -r pve          # removes /usr/bin/pve and the packaged docs
```

> **No `apt upgrade` path:** the `.deb` is a standalone release asset, not a
> hosted apt repository, so `apt-get update`/`upgrade` will not pick up new
> versions — re-download the latest `.deb` and `dpkg -i` it. (An `.rpm` is also
> published for RPM-based hosts: `sudo rpm -Uvh proxmox-cli_*_linux_amd64.rpm` /
> `rpm -e pve`.)


**From pre-built binary (Linux amd64):**
```bash
gh release download --repo yg-codes/proxmox-cli --pattern 'proxmox-cli-*linux-amd64.tar.gz' --dir .
tar -xzf proxmox-cli-*-linux-amd64.tar.gz
sudo install -m 755 pve /usr/local/bin/pve
```
(Or grab the `proxmox-cli-<version>-linux-amd64.tar.gz` archive from the
[releases page](https://github.com/yg-codes/proxmox-cli/releases/latest) and
extract it — the archive also carries the license and third-party notices.)

**From source with mise:**
```bash
cd proxmox-cli
mise run install    # builds + installs pve to $GOPATH/bin
```

**From source with make:**
```bash
make install
```

**With mise (tracks latest release):**
```toml
# ~/.config/mise/config.toml
"github:yg-codes/proxmox-cli[pve]" = "latest"
```
```bash
mise install -g pve      # install the latest GitHub release
mise upgrade pve         # pull each new release after a tag ships
pve --version            # -> pve v1.10.0 (commit: ..., built: ...)
```

> ⚠️ Do **not** use the `go:` backend (`"go:github.com/yg-codes/proxmox-cli/pve"`). It runs
> `go install`, which bypasses the Makefile/goreleaser ldflags that inject the version,
> so `pve --version` reports `dev (commit: none, built: unknown)`. The `github:` backend
> above downloads the goreleaser-built release binary, which has the version baked in.

### Configure

Set environment variables for Proxmox API access:

```bash
export PVE_HOST=pve.example.com
export PVE_USER=username@pam
export PVE_TOKEN_NAME=token-name
export PVE_TOKEN_VALUE=token-value
```

For password auth instead of token: set `PVE_PASSWORD` instead of `PVE_TOKEN_NAME`/`PVE_TOKEN_VALUE`. Token auth is recommended for production.

To create an API token on a Proxmox node:
```bash
./scripts/pve-token.sh pve1
```

### Verify

```bash
pve --version
# pve v1.10.0 (commit: ce2fea8, built: 2026-08-25_00:44:53)

pve node list
# NAME    STATUS   CPU   MEM       UPTIME
# pve1    online   12%   45.2%     42d 5h
```

### Release a New Version

```bash
git tag -a v1.10.0 -m "Release v1.10.0"
git push origin v1.10.0
# GitHub Actions builds 6-platform binaries automatically (linux/darwin/windows × amd64/arm64)
```

## Reference

### Commands

| Command | Purpose |
|---------|---------|
| `pve cluster task list` | List cluster tasks |
| `pve cluster storage list-backup` | List backup storages |
| `pve cluster backup-job list` | List scheduled vzdump backup jobs |
| `pve cluster backup-job not-covered` | Guests covered by no backup job |
| `pve cluster network list --node pve1` | List network interfaces |
| `pve node list` | List cluster nodes |
| `pve node status --node pve1` | Node status and resources |
| `pve node services --node pve1` | List node services |
| `pve node reboot --node pve1 --confirm` | Reboot a node (dry-run by default) |
| `pve node shutdown --node pve1 --confirm` | Shutdown a node |
| `pve node resource capacity` | Cluster CPU/RAM overcommit + N-1 failover verdict |
| `pve node disks zfs` | ZFS pool usage (size/alloc/free/frag/health) |
| `pve node disks zfs-detail --node pve1 --pool rpool` | ZFS pool vdev tree + error counts |
| `pve node disks list --node pve1` | Physical disks (size/model/wearout/health) |
| `pve node disks smart --node pve1 --disk /dev/sda` | SMART health for a disk |
| `pve node apt repos list --node pve1` | APT repositories + standard handles |
| `pve node apt repos add --node pve1 --handle no-subscription` | Add/enable a standard repo (needs Sys.Modify) |
| `pve node apt repos enable --node pve1 --handle enterprise` | Enable a repository entry |
| `pve node apt repos disable --node pve1 --path /etc/apt/sources.list.d/ceph.sources --index 0` | Disable by file+index |
| `pve zfs pool list` | List ZFS pools across the cluster |
| `pve zfs pool status --node pve1 --pool rpool` | Pool vdev tree, health, errors, last scrub |
| `pve zfs pool create --node pve1 --name tank --raid-level mirror --devices /dev/sdb,/dev/sdc` | Create a pool (wipes the devices) |
| `pve zfs pool destroy --node pve1 --name tank` | Destroy a pool (irreversible) |
| `pve zfs storage list` | List configured zfspool storages |
| `pve zfs storage create --storage tank-vms --pool tank/vms` | Register a pool/dataset as a PVE storage |
| `pve zfs storage update --storage tank-vms --sparse` | Change a zfspool storage's settings |
| `pve zfs storage delete --storage tank-vms` | Remove a storage entry (pool and data kept) |
| `pve zfs replication list` | List replication jobs |
| `pve zfs replication status --node pve1` | Job last/next sync, duration, failures |
| `pve zfs replication create --id 100-0 --target pve2` | Create a replication job |
| `pve zfs replication run --node pve1 --id 100-0` | Trigger a job outside its schedule |
| `pve zfs replication delete --id 100-0` | Remove a replication job |
| `pve node capability` | Token-permission preflight (✅/❌ matrix) |
| `pve cluster health` | REST-only quorum/HA/storage health snapshot |
| `pve vm list` | List all VMs |
| `pve vm start --vmid 100` | Start a VM |
| `pve vm stop --vmid 100` | Stop a VM |
| `pve vm shutdown --vmid 100` | Graceful shutdown |
| `pve vm reboot --vmid 100` | Graceful reboot (applies pending config changes) |
| `pve vm reset --vmid 100` | Hard reset (no graceful shutdown) |
| `pve vm suspend --vmid 100` | Pause a VM in RAM |
| `pve vm suspend --vmid 100 --to-disk` | Suspend to disk (survives a node reboot) |
| `pve vm resume --vmid 100` | Resume a suspended VM |
| `pve vm config get --vmid 100` | Show VM configuration |
| `pve vm config get --vmid 100,101 --key cores,memory` | Compare keys across several VMs |
| `pve vm config set --vmid 100 --set cores=4` | Change VM configuration |
| `pve vm config set --vmid 100,101 --set cores=4` | Same change across VMs (type the VM count to confirm) |
| `pve vm config pending --vmid 100` | Show changes awaiting a power cycle |
| `pve vm clone --vmid 100 --newid 200` | Clone a VM or template |
| `pve vm migrate --vmid 100 --target node2` | Migrate to another node (checks preconditions first) |
| `pve vm resize --vmid 100 --disk scsi0 --size +4G` | Extend a disk (grow only) |
| `pve vm move-disk --vmid 100 --disk scsi0 --storage local-zfs` | Move a disk to another storage |
| `pve vm template --vmid 100` | Convert a VM into a template (one-way) |
| `pve vm create --node pve1 --name web01 --cores 2 --memory 4096` | Create a VM |
| `pve vm destroy --vmid 999` | Destroy a VM and its disks (irreversible) |
| `pve vm agent ping --vmid 100` | Check the guest agent responds |
| `pve vm agent info --vmid 100` | Which agent commands this guest permits |
| `pve vm agent osinfo / network / fsinfo --vmid 100` | Guest OS, interfaces, filesystems |
| `pve vm agent exec --vmid 100 -- uname -a` | Run a command in the guest, with output |
| `pve vm cloudinit pending --vmid 100` | Cloudinit settings not yet on the drive |
| `pve vm cloudinit dump --vmid 100 --type user` | Rendered cloud-config the guest reads |
| `pve vm cloudinit regenerate --vmid 100` | Rebuild the cloudinit drive |
| `pve vm firewall rules list --vmid 100` | List firewall rules in chain order |
| `pve vm firewall rules add --vmid 100 --type in --action ACCEPT --dport 22` | Add a rule |
| `pve vm firewall options get --vmid 100` | Show firewall options |
| `pve vm firewall alias list --vmid 100` | Named CIDRs usable in rules |
| `pve vm firewall ipset list --vmid 100` | IP sets, or a set's entries with `--name` |
| `pve vm firewall refs --vmid 100` | Aliases/IP sets available to rules |
| `pve vm snapshot create --vmid 100 --prefix backup` | Create snapshot |
| `pve vm snapshot list --vmid 100` | List snapshots |
| `pve vm snapshot rollback --vmid 100 --snapshot name` | Rollback to snapshot |
| `pve vm snapshot delete --vmid 100 --snapshot name` | Delete snapshot |
| `pve vm snapshot preview` | Preview every snapshot in the cluster |
| `pve vm snapshot preview --older-than 30d` | Cluster-wide, filtered by age |
| `pve vm snapshot prune --older-than 30d --dry-run` | Preview a batch delete |
| `pve vm snapshot prune --name-pattern 'pre-update-*'` | Batch delete by name (filter required) |
| `pve vm backup create --vmid 100 --storage local` | Create backup |
| `pve vm backup list --vmid 100` | List VM backups |
| `pve vm backup list --all --storage local` | List all backups on storage |
| `pve vm backup restore --vmid 100 --backup-file "..." --node pve1` | Restore backup |
| `pve vm bulk start` | Start all stopped VMs |
| `pve vm bulk stop` | Force-stop all running VMs |
| `pve vm bulk shutdown` | Gracefully shut down all running VMs |
| `pve vm bulk reboot` | Reboot all running VMs |
| `pve vm bulk backup --storage local` | Backup all VMs |
| `pve container list` | List containers |
| `pve container start --node pve1 --vmid 200` | Start container |
| `pve access create --user automation --token prod` | Create a user + token, assign role |
| `pve access add-token --user automation --token ci` | Add a token to an existing user |
| `pve access revoke-token --user automation --token ci` | Revoke one token (user kept) |
| `pve access remove --user automation` | Delete a user and all its tokens |
| `pve access list --user automation` | List a user's API tokens |
| `pve access grant --user snapshot --path / --grant-role PVEAuditor` | Grant a role to an existing user/token (no reissue) |
| `pve access acl-list --user snapshot` | Show ACL entries (all, or filtered by user) |

### VM Selection Patterns

| Pattern | Example | Matches |
|---------|---------|---------|
| Single ID | `--vmid 4103` | One VM |
| List | `--vmid 7201,7203,7205` | Specific VMs |
| Range | `--vmid 7201-7205` | All VMs in range |
| Wildcard | `--vmid 72*` | Pattern match |
| Keyword | `--vmid running` | All running VMs |
| Interactive | `--vmid i` | Checkbox UI |
| Name | `--vmname web01` | VM by name — **exact match only** |

> **These patterns apply to the multi-VM commands** — snapshots, lifecycle
> (`start`/`stop`/`shutdown`/`reboot`/`reset`/`suspend`/`resume`) and
> `vm config get`/`set`/`pending`. Single-target commands (`destroy`, `clone`,
> `migrate`, `resize`, `move-disk`, `template`, `agent`, `firewall`,
> `cloudinit`) take exactly one VM and reject a pattern, because acting on a
> set you did not enumerate is unrecoverable there. Quote a selector containing
> a comma or wildcard so the shell does not split it: `--vmid "100,101"`.
>
> **`--vmname` never partial-matches.** `--vmname web` will not resolve to
> `web-prod-01`; an unknown name and an ambiguous one are both errors listing the
> candidates. Convenience there would mean acting on a VM you did not name, which
> on `pve vm destroy` is unrecoverable. `--vmid` and `--vmname` are mutually
> exclusive, so there is no precedence rule to remember. `pve vm create` has no
> `--vmname` — it creates a VM, so use `--name` for the new VM's name.

### Safety Flags

| Flag | Behavior |
|------|----------|
| `--dry-run` | Preview without executing |
| `--yes` / `-y` | Skip confirmation prompts |
| `--confirm` | Required for node power operations |

### Build Commands

| Command | Purpose |
|---------|---------|
| `mise run build` | Build `pve` binary (uses Go from mise) |
| `mise run install` | Build + install to `$GOPATH/bin` |
| `mise run clean` | Remove build artifacts |
| `make build-all` | Cross-compile all platforms |
| `make release` | Create release archives |

### Configuration

| Variable | Required | Description |
|----------|:--------:|-------------|
| `PVE_HOST` | Yes | Proxmox hostname or IP |
| `PVE_PORT` | No | API port (default `8006`; also accepts an `httptest` port for the test harness) |
| `PVE_USER` | Yes | Username (e.g. `root@pam`) |
| `PVE_TOKEN_NAME` | Token auth | API token name |
| `PVE_TOKEN_VALUE` | Token auth | API token secret |
| `PVE_PASSWORD` | Password auth | User password |
| `PVE_VERIFY_SSL` | No | `true` by default (secure). Set `false` only for self-signed certificates — skipping verification permits man-in-the-middle interception. Prefer `PVE_CA_CERT` with your CA bundle instead |
| `PVE_CA_CERT` | No | Path to a PEM CA bundle. Verifies TLS against a private CA instead of disabling verification. Fails fast if unreadable or not PEM |
| `PVE_TASK_TIMEOUT` | No | Go duration (`90s`, `5m`) bounding how long the CLI waits on a single Proxmox task before failing. Default `5m` |

> **Batch mode authentication:** `--batch` accepts either a complete token
> pair or `PVE_PASSWORD` — the same contract as interactive authentication.
> `PVE_HOST` and `PVE_USER` are always required.
>
> **Exit codes:** any multi-target command exits non-zero when one or more
> targets fail (the per-target summary is still printed first), when an
> operation is cancelled (Ctrl-C), or when cluster inventory could not be
> read completely. Scripts gating on `$?` see partial failures as failures.

> **Reporting permissions — two different ACL paths.** The report endpoints do
> not all check the same path, so one grant is usually not enough:
>
> | Commands | Endpoint checks |
> |---|---|
> | `pve node status`, `pve node disks list` / `smart` | `Sys.Audit` on **`/nodes`** |
> | `pve node disks zfs`, `pve cluster health`, HA status | `Sys.Audit` on **`/`** |
>
> ```bash
> pveum acl modify /      --users '<user>@<realm>' --roles PVEAuditor
> pveum acl modify /nodes --users '<user>@<realm>' --roles PVEAuditor
> ```
>
> Grant to `--users`, **not** `--tokens`: a token created with `--privsep 0` (what
> `pve access create` uses) inherits the user's privileges and its own token-level
> ACL is ignored, so a token-only grant silently has no effect. Check the privsep
> column with `pveum user token list <user>@<realm>`.
>
> Run `pve node capability` to see which report endpoints your token can reach;
> failed probes print the exact grant each one needs.

> **Token/user management (`pve access`):** the `pve access create / add-token /
> revoke-token / remove / grant` commands hit `/access/*`, which require
> `User.Modify` and `Permissions.Modify`. A VM-scoped API token cannot perform
> these — authenticate as `root@pve` (set `PVE_PASSWORD`) or a `PVEUserAdmin`
> service account. This is the no-SSH successor to `scripts/pve-token.sh`.
> ```bash
> export PVE_USER=root@pve
> export PVE_PASSWORD=<root password>
> pve access create --user snapshot --token t1 --nodes-audit
>
> # Add a role to a credential that already exists — no reissue, secret unchanged
> pve access grant --user snapshot --path / --grant-role PVEAuditor
> pve access acl-list --user snapshot
> ```
> `--nodes-audit` on `create` grants `/nodes` only. For ZFS, cluster health and
> HA status, add the `/` grant shown above.

### Troubleshooting: API Token Permissions

Proxmox reports missing privileges **one at a time** — fix one 403, retry,
and the next missing privilege appears. Expect to peel the onion when
running a new command family under a narrowly-scoped token.

Worked example: cloning a template to a new storage with a snapshot-scoped
token failed twice in sequence before succeeding —

1. `403 forbidden` on `/storage/<storage-id>` — the clone allocates disk
   on the target storage (`Datastore.AllocateSpace`).
2. `403 forbidden` on `/sdn/zones/<zone>/<bridge>` — the VM's bridge
   belonged to an SDN zone, and **`PVEVMAdmin` does not include `SDN.Use`**.
   A grant set that works on plain bridges still fails here.

The complete working grant set (run as root on any cluster node; `pveum`
modifies cluster-wide state):

```bash
pveum acl modify /storage/<storage-id> --users <user>@<realm> --roles PVEDatastoreUser
pveum acl modify /                --users <user>@<realm> --roles PVEVMAdmin
pveum acl modify /sdn             --users <user>@<realm> --roles PVESDNUser
```

| Grant | Unblocks |
|---|---|
| `PVEDatastoreUser` on `/storage/<storage-id>` | disk allocation on the target storage |
| `PVEVMAdmin` on `/` | clone, config and lifecycle on the new VM |
| `PVESDNUser` on `/sdn` | NICs attached to an **SDN-zoned** bridge (`SDN.Use`) — omitted by `PVEVMAdmin`, so add it only if the target bridge is SDN-managed |

Grant to `--users`, not to the token — see the privsep note above. Run
`pve node capability` to probe which endpoints the token can already reach.

### 1Password Secret References

Any `PVE_*` value set to an `op://` reference is resolved to plaintext at
startup by shelling out to the 1Password CLI. The tool looks for `op.exe`
first, then `op`, on `PATH`. References and plain values can be mixed
freely; `op` is only invoked when a value actually starts with `op://`.

**Prerequisite:** install the 1Password CLI and authenticate:
```bash
op signin            # Linux / macOS
op.exe account list  # WSL-on-Windows (uses the Windows desktop app)
```

```bash
# Set any PVE_* value to an op://<vault>/<item>/<field> reference.
export PVE_HOST=op://SRE/proxmox/host
export PVE_USER=op://SRE/proxmox/user
export PVE_TOKEN_NAME=op://SRE/proxmox/token_name
export PVE_TOKEN_VALUE=op://SRE/proxmox/credential
```

The reference format is `op://<vault>/<item>/<field>`, where `<field>` is the
field **label** as it appears in the 1Password item. If the CLI is not signed
in, startup fails before contacting Proxmox. See
`pkg/onepassword/onepassword.go` for details.

### Directory Structure

```
proxmox/
├── pve/                      # Cobra commands (CLI entry point)
├── pkg/                      # Core packages
├── scripts/                  # API token setup, SSH runner
├── .github/workflows/        # CI/CD release pipeline
├── .mise.toml                # mise build tasks
└── docs/                     # Command reference, spec, demo runbook
```

## Explanation

### Architecture

`pve` is a Go CLI built with Cobra. It talks to the Proxmox VE REST API over HTTPS — no agent on the Proxmox side, no local config files. All state comes from the cluster API at runtime.

Binary compiles statically with version/commit/build-time injected via ldflags. No runtime dependencies beyond the Proxmox API endpoint.

Full feature reference: [docs/FUNCTIONAL_SPECIFICATION.md](docs/FUNCTIONAL_SPECIFICATION.md)

## Security

TLS verification is **on by default**; for hosts signed by a private CA set
`PVE_CA_CERT` to your CA bundle rather than disabling verification. See
[SECURITY.md](SECURITY.md) for the reporting policy and the full threat-model
and limitations table.

---
Last Updated: 2026-10-28
