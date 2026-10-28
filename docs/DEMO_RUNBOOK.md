# pve CLI — Demonstration & Validation Runbook (Phase 1: Snapshots)

**Date**: 2026-06-26

**Environment**: dev (`dev` cluster: `pve-node4`, `pve-node5`, `pve-node6` — one cluster)

**Release under test**: `pve` **v1.5.0** (GitHub release `yg-codes/proxmox-cli`; built via GoReleaser, ldflags stamp `v1.5.0`)

**Repo**: `github.com/yg-codes/proxmox-cli` (personal GitHub mirror)

**Purpose**: Validate the `pve` snapshot subcommands end-to-end the way an end user receives the tool — install the released binary (or run via mise), authenticate via API token (optionally resolved from 1Password), then exercise the full snapshot lifecycle (list → create → rollback → delete) on a single VM (Parts 2 / 2B) and as a concurrent bulk operation across three VMs on three nodes (Parts 3 / 3B), driven by both `--vmid` and `--vmname`.

> **Version note**: this walkthrough was recorded against `v1.5.0`; the command shapes are unchanged in later releases, but check `pve --version` output details against your installed release.

> **This is Phase 1 of a phased runbook series.** Phase 1 covers snapshot functions — the single-VM lifecycle (Parts 2 / 2B) and the bulk snapshot lifecycle (Parts 3 / 3B). Later phases will cover backup management, VM power ops, nodes, storage, etc.

## Overview

This runbook installs the released `pve` binary, points it at the `dev` Proxmox cluster with an API token, and runs the four snapshot verbs against a single VM (Parts 2 / 2B) and against three VMs on three nodes as a concurrent bulk operation (Parts 3 / 3B) — once by VMID and once by VM name in each case. All snapshots created here are **disk-only** (`--vmstate` omitted) and are deleted at the end. Note that **rollback discards every disk write made since the snapshot** (see the disposable-test-VM warning at the end of this runbook) — the demo returns VMs to their snapshot-time state, not to "whatever was on disk last". Because a disk-only rollback powers the VM off, **every rollback is followed by a power-status check and a start if needed** (see Parts 2.5 / 2B.5 / 3.5 / 3B.5).

**Test targets (all running before the test):**

| VMID | Name | Node | Role in test |
|------|------|------|--------------|
| 5003 | app-01 | pve-node5 | single-VM lifecycle (Parts 2/2B) + bulk (Parts 3/3B) |
| 4103 | docs-vm-03 | pve-node6 | bulk only (Parts 3/3B) |
| 4105 | docs-vm-05 | pve-node4 | bulk only (Parts 3/3B) |

> **Pre-existing snapshots:** all three test VMs start with no user-created snapshots. The bulk create/delete loops target snapshots exclusively by a `bulkdemo-*` / `bulkvn-*` name prefix, so any other snapshot a VM happens to carry is never selected. The final-state checks (Step 4.1) assert that only the runbook's own test prefixes were created and removed.

> **Why these results are pre-filled:** The ✅ Result lines record the *expected* outcome. The validator executing this runbook should **overwrite each Result** with their own observed output (or mark ❌ on deviation). Result lines marked `*(to be observed)*` have not yet been run against the live cluster.

> **⚠️ `pve`-specific gotchas baked into every command below (these differ from the predecessor snapshot tooling):**
> 1. **Command path is `pve vm snapshot <verb>`** — NOT `pve snapshot <verb>`. The snapshot verbs' own `--help` examples show `pve snapshot ...`, but that path is **stale help text and fails** with "unknown command". Only `vm`, `cluster`, `node`, `container` are attached to the root. Always use `pve vm snapshot ...`.
> 2. **The snapshot-name flag is `--snapshot`** — NOT `--snapshot_name` / `--snap`. (And VM selection is `--vmname`, one word — there is no `--vm-name`.)
> 3. **Bulk is just multi-value `--vmid` / `--vmname`** — there is **no** `bulk` subcommand and **no** `--batch` flag for snapshot ops. Pass comma-separated values to the ordinary `create`/`rollback`/`delete` verbs (e.g. `--vmid 5003,4103,4105`). The two-VM/keyword/wildcard/range selectors (`running`, `stopped`, `all`, `72*`, `7201-7205`, `i`) also expand to bulk targets. `pve vm bulk ...` exists but is for **power/backup ops only** — not snapshots.
> 4. **Default concurrency is 2** (`MaxConcurrentSnapshots`). With 3 VMs targeted, the third VM completes shortly after the first two — this staggered completion is expected, not a hang. There is **no** `--workers` / `--concurrency` flag; raise it via config (`max_concurrent_snapshots`) if needed.
> 5. **Bulk output = per-VM lines + a summary block.** Each VM logs a `✅ VM <id> (<name>): ...` line; the run ends with a `BULK OPERATION SUMMARY` showing `Total Operations:`, `Successful: N (X.X%)`, `Failed:`. The sibling tool's summary looks similar but is not identical — do not grep the sibling's exact strings.
> 6. **`--all` on delete is a different (worse) path.** `pve vm snapshot delete --vmid ... --all` deletes *every* snapshot of each VM sequentially with **no concurrency and no summary block**, including any snapshot the runbook did not create. **Never use `--all` in this runbook** — delete by explicit per-VM name via the loops in Steps 3.6 / 3B.6.

---

## Prerequisites

- **`pve` v1.5.0** installed — either from the GitHub release archive, via `go install github.com/yg-codes/proxmox-cli/pve@v1.5.0`, or via mise (`go:github.com/yg-codes/proxmox-cli/pve` = `v1.5.0`). See Part 1.
- An **API token** on the `dev` cluster with at least `PVEVMAdmin` on the target VM (and the VM's node). The token name is the plain label; the token value is the secret (or an `op://` reference).
- **1Password CLI** (`op` / `op.exe`) authenticated — only required if you pass `op://` references for credentials. The tool resolves any credential env var whose value starts with `op://` at startup. Plaintext credentials skip this.
- SSH as `root` to the cluster nodes — needed **only** for the post-rollback `qm start` (the `pve` snapshot verbs do **not** do power operations). Single-VM parts need only `pve-node5`; the bulk parts need all three: `pve-node4`, `pve-node5`, `pve-node6`.
- **Approval:** this is a live cluster: obtain whatever approval your environment requires before executing the bulk parts (3 / 3B).

---

## Part 0: Pre-checks

### Step 0.1: Confirm the target VMs are running and in one cluster

A single `PVE_HOST` resolves all VMs only if the nodes form one cluster. The bulk parts (3 / 3B) need all three VMs reachable.

**Command**:
```bash
ssh -o ConnectTimeout=5 root@pve.example.com \
  "pvesh get /cluster/resources --type vm 2>/dev/null | grep -E ' 5003 | 4103 | 4105 '"
```

**Expected**: All three VMs appear with `running` status — 5003 (`app-01`) on pve-node5, 4103 (`docs-vm-03`) on pve-node6, 4105 (`docs-vm-05`) on pve-node4 — confirming one cluster.

**Result**: ✅ *(to be observed)*

### Step 0.2: (Optional) Verify 1Password CLI is authenticated

Only needed if credentials will be `op://` references.

**Command**:
```bash
op account list 2>&1 | head -3      # WSL/Windows: op.exe account list
```

**Expected**: Lists the `myaccount.1password.com` account.

**Result**: ✅ *(to be observed, or N/A if using plaintext credentials)*

---

## Part 1: Install From Release (end-user flow)

### Step 1.1: Install the binary

Pick **one** of the following. The release-archive path is the canonical end-user flow.

**Option A — GitHub release archive (canonical):**
```bash
rm -rf /tmp/pve-release-test && mkdir -p /tmp/pve-release-test && cd /tmp/pve-release-test
# download the archive matching your platform, e.g. linux-amd64:
gh release download v1.5.0 --repo yg-codes/proxmox-cli --pattern 'proxmox-1.5.0-linux-amd64.tar.gz' --dir .
tar -xzf proxmox-1.5.0-linux-amd64.tar.gz
sudo install -m 0755 pve /usr/local/bin/pve
```

**Option B — `go install` (out-of-the-box since v1.5.0):**
```bash
go install github.com/yg-codes/proxmox-cli/pve@v1.5.0
# → $(go env GOPATH)/bin/pve
```

**Option C — mise:**
```bash
mise install    # with "go:github.com/yg-codes/proxmox-cli/pve" = "v1.5.0" in config
```

**Expected**: `pve` on `PATH`.

**Result**: ✅ *(to be observed)*

### Step 1.2: Verify version

**Command**:
```bash
pve --version
which pve
```

**Expected**: `pve v1.5.0 (commit <short>, built <date>)`. (Note: a `go install`/mise build without ldflags reports `pve dev (commit: none, built: unknown)` — the binary still works; only the version string is blank. Release-archive binaries stamp `v1.5.0` correctly.)

**Result**: ✅ *(to be observed)*

### Step 1.3: Export credentials

Point at any cluster node. `PVE_TOKEN_NAME` is the plain token name; `PVE_TOKEN_VALUE` is either the secret or an `op://` reference resolved at startup.

**Command**:
```bash
export PVE_HOST=pve.example.com
export PVE_USER=cli-user@pve
export PVE_TOKEN_NAME=snapshot
export PVE_TOKEN_VALUE='op://Vault/pve-cli/credential'   # or the plaintext secret
```

**Expected**: No output. Variables set in the shell that runs the remaining commands.

**Result**: ✅ Environment exported

---

## Part 2: Single-VM Lifecycle by VMID — VM 5003

> **Reminder:** every command uses `pve vm snapshot <verb>`, and the snapshot-name flag is `--snapshot`.

### Step 2.1: List snapshots (read-only auth check)

Confirms credentials resolve and the API is reachable.

**Command**:
```bash
pve vm snapshot list --vmid 5003 2>&1 | tail -15
```

**Expected**: Output like `VM 5003: app-01`, `Status: 🟢 running`, then `Snapshots (N total):` (or `No snapshots found for VM 5003` if none). A `403 Permission check failed` means the token role is wrong.

**Result**: ✅ *(to be observed)*

### Step 2.2: Create a disk-only snapshot

**Command**:
```bash
pve vm snapshot create --vmid 5003 --prefix demo -y 2>&1 | tail -8
```

**Expected**: `✅ Snapshot 'demo-app-01-<YYYYMMDD-HHMM>' created successfully for VM 5003`. The generated name is `<prefix>-<vmname>-<YYYYMMDD-HHMM>`, **truncated to 40 chars total** (`pkg/snapshot/operations.go` `maxSnapshotNameLength`) — long VM names can lose the `-HHMM` suffix, so the suffix is **not** guaranteed. **Capture the exact name** for Steps 2.4–2.6 (this variable is the only input those steps use — re-run the capture in the same shell if you skipped it, or a stale value will make rollback/delete fail with `snapshot '...' not found`):

```bash
SNAP=$(pve vm snapshot list --vmid 5003 2>&1 | grep -oP 'demo-app-01-\S+')
echo "Captured: $SNAP"
[ -z "$SNAP" ] && echo "ABORT: no demo snapshot captured — do not proceed to 2.4"
```

**Result**: ✅ *(to be observed — record the captured snapshot name)*

### Step 2.3: Verify the snapshot is listed

**Command**:
```bash
pve vm snapshot list --vmid 5003 2>&1 | grep -E 'demo-app-01'
```

**Expected**: One line showing the `demo-app-01-<TS>` snapshot, `VM State: ❌ Not included (disk only)`.

**Result**: ✅ *(to be observed)*

### Step 2.4: Roll back to the snapshot

Uses the `SNAP` variable captured in Step 2.2. `--snapshot` is **required** (single value).

**Command**:
```bash
pve vm snapshot rollback --vmid 5003 --snapshot "$SNAP" -y 2>&1 | tail -8
```

**Expected**: `✅ VM 5003 rolled back to snapshot '<SNAP>' successfully`.

**Result**: ✅ *(to be observed)*

### Step 2.5: Post-rollback power check — start VM if stopped (MANDATORY)

A disk-only snapshot has no RAM state, so rollback leaves the VM **powered off** even if it was running. This step is required after every rollback. (`pve` snapshot verbs do not start VMs.)

**Command**:
```bash
ST=$(ssh -o ConnectTimeout=5 root@pve.example.com "qm status 5003")
echo "5003: $ST"
echo "$ST" | grep -q stopped && ssh -o ConnectTimeout=5 root@pve.example.com "qm start 5003"
sleep 3
ssh -o ConnectTimeout=5 root@pve.example.com "qm status 5003"
```

**Expected**: Status reads `stopped` after rollback; after `qm start`, final status reads `running`.

**Result**: ✅ *(to be observed)*

### Step 2.6: Delete the snapshot

**Command**:
```bash
pve vm snapshot delete --vmid 5003 --snapshot "$SNAP" -y 2>&1 | tail -8
```

**Expected**: Success log line for the deletion of `<SNAP>` on VM 5003.

**Result**: ✅ *(to be observed)*

### Step 2.7: Verify clean state

**Command**:
```bash
pve vm snapshot list --vmid 5003 2>&1 | tail -8
```

**Expected**: `No snapshots found for VM 5003` (or only `current` state), VM `Status: 🟢 running`.

**Result**: ✅ *(to be observed)*

---

## Part 2B: Single-VM Lifecycle by VM Name — VM `app-01`

Mirrors Part 2's full lifecycle but drives every command with **`--vmname`** instead of `--vmid`, validating that the name selector works end-to-end across all four verbs. Uses a distinct prefix (`vndemo`) so these snapshots never collide with Part 2's `demo-*`. Cleanup deletes the snapshot created here in Step 2B.6 — but note the rollback data-loss warning at the end of this runbook: run only on disposable test VMs.

### Step 2B.1: List by VM name (auth + name resolution)

**Command**:
```bash
pve vm snapshot list --vmname app-01 2>&1 | tail -15
```

**Expected**: Identical to Step 2.1 — `VM 5003: app-01`, `Status: 🟢 running`. Confirms `--vmname app-01` resolves to VMID 5003.

**Result**: ✅ *(to be observed)*

### Step 2B.2: Create a disk-only snapshot by name

**Command**:
```bash
pve vm snapshot create --vmname app-01 --prefix vndemo -y 2>&1 | tail -8
```

**Expected**: `✅ Snapshot 'vndemo-app-01-<YYYYMMDD-HHMM>' created successfully for VM 5003`. The generated name follows the same 40-char truncation rule as Step 2.2 — the `-HHMM` suffix is **not** guaranteed for long names. **Capture the exact name** for Steps 2B.4–2B.6 (re-run this capture in the same shell if you skipped it, or a stale value will make rollback/delete fail):

```bash
VNSNAP=$(pve vm snapshot list --vmname app-01 2>&1 | grep -oP 'vndemo-app-01-\S+')
echo "Captured: $VNSNAP"
[ -z "$VNSNAP" ] && echo "ABORT: no vndemo snapshot captured — do not proceed to 2B.4"
```

**Result**: ✅ *(to be observed)*

### Step 2B.3: Verify the snapshot is listed (by name)

**Command**:
```bash
pve vm snapshot list --vmname app-01 2>&1 | grep -E 'vndemo-app-01'
```

**Expected**: One line showing the `vndemo-app-01-<TS>` snapshot, disk-only.

**Result**: ✅ *(to be observed)*

### Step 2B.4: Roll back by name

Uses the `VNSNAP` variable captured in Step 2B.2. The VM is located via `--vmname`; the snapshot via `--snapshot`.

**Command**:
```bash
pve vm snapshot rollback --vmname app-01 --snapshot "$VNSNAP" -y 2>&1 | tail -8
```

**Expected**: `✅ VM 5003 rolled back to snapshot '<VNSNAP>' successfully`.

**Result**: ✅ *(to be observed)*

### Step 2B.5: Post-rollback power check — start VM if stopped (MANDATORY)

**Command**:
```bash
ST=$(ssh -o ConnectTimeout=5 root@pve.example.com "qm status 5003")
echo "5003: $ST"
echo "$ST" | grep -q stopped && ssh -o ConnectTimeout=5 root@pve.example.com "qm start 5003"
sleep 3
ssh -o ConnectTimeout=5 root@pve.example.com "qm status 5003"
```

**Expected**: `stopped` after rollback → `running` after `qm start`.

**Result**: ✅ *(to be observed)*

### Step 2B.6: Delete by name

**Command**:
```bash
pve vm snapshot delete --vmname app-01 --snapshot "$VNSNAP" -y 2>&1 | tail -8
```

**Expected**: Success log line for the deletion of `<VNSNAP>`.

**Result**: ✅ *(to be observed)*

### Step 2B.7: Verify clean state (by name)

**Command**:
```bash
pve vm snapshot list --vmname app-01 2>&1 | tail -8
```

**Expected**: `No snapshots found for VM 5003`, VM `Status: 🟢 running`.

**Result**: ✅ *(to be observed)*

---

## Part 3: Bulk Lifecycle by VMID — VMs 5003, 4103, 4105

Drives all four verbs against **three VMs on three different nodes** in a single cluster via comma-separated `--vmid`. This validates multi-target selection, the concurrent worker pool (default cap **2**), cross-node operation, and the `BULK OPERATION SUMMARY` output. Uses a distinct prefix (`bulkdemo`) so these never collide with Part 2's `demo-*` / Part 2B's `vndemo-*`.

> **Note on names:** generated snapshot names embed each VM's name and a per-VM timestamp (e.g. `bulkdemo-docs-vm-05-...`), so a single `--snapshot` value cannot match all three for rollback/delete. Capture each name from Step 3.2 and operate **per-VM by exact name** in 3.4 and 3.6 (the loops do this). Rollback takes exactly one `--snapshot` (String); delete takes one-or-more (StringSlice) — either way, per-VM loops are the clean approach.

### Step 3.1: Bulk list (read-only)

**Command**:
```bash
pve vm snapshot list --vmid 5003,4103,4105 2>&1 | grep -E 'VM [0-9]+:|No snapshots'
```

**Expected**: All three VMs listed `Status: 🟢 running`, each showing no user-created snapshots (after Parts 2/2B cleaned up).

**Result**: ✅ *(to be observed)*

### Step 3.2: Bulk create

**Command**:
```bash
pve vm snapshot create --vmid 5003,4103,4105 --prefix bulkdemo -y 2>&1 | tail -20
```

**Expected**: Three `✅ VM <id> (<name>): Snapshot ... created successfully (<dur>s)` lines, then a `BULK OPERATION SUMMARY` block ending `Total Operations: 3`, `Successful: 3 (100.0%)`, `Failed: 0 (0.0%)`. With concurrency capped at 2, one VM finishes slightly after the other two.

> **⚠️ This capture block is load-bearing — Steps 3.4 and 3.6 will fail without it.** A generated name embeds the VM's name and a timestamp, so it cannot be predicted; it must be read back from `list`. The `SNAP` map populated here is the **only** input the rollback/delete loops use. If you skip it (or re-run this runbook in a shell that still holds a stale map from an earlier part), the loops will target wrong/old names and every VM will fail with `snapshot '...' not found`. **Run this block now, and confirm all three lines print a non-empty `bulkdemo-*` name before continuing.**

```bash
declare -A SNAP
for vmid in 5003 4103 4105; do
  SNAP[$vmid]=$(pve vm snapshot list --vmid "$vmid" 2>&1 | grep -oP 'bulkdemo-\S+')
  echo "VM $vmid: ${SNAP[$vmid]}"
  [ -z "${SNAP[$vmid]}" ] && { echo "ABORT: no bulkdemo snapshot captured for VM $vmid — do not proceed to 3.4"; break; }
done
```

> **Note on the generated name format:** the name is `<prefix>-<vmname>-<YYYYMMDD-HHMM>`, then **truncated to 40 chars** (`pkg/snapshot/operations.go` `maxSnapshotNameLength`). Long VM names lose the `-HHMM` suffix — e.g. with prefix `bulkdemo`, VM `app-01` (short) yields `bulkdemo-app-01-20260626-1727`, but `docs-vm-03` (long) yields `bulkdemo-docs-vm-03-20260626` (date only). **Do not assume the timestamp suffix is present** — always capture the exact name from `list`.

**Result**: ✅ *(to be observed — record the three captured snapshot names)*

### Step 3.3: Verify all three snapshots present

**Command**:
```bash
pve vm snapshot list --vmid 5003,4103,4105 2>&1 | grep -E 'VM [0-9]+:|bulkdemo'
```

**Expected**: One `bulkdemo-*` snapshot per VM (`VM State: ❌ Not included (disk only)`).

**Result**: ✅ *(to be observed)*

### Step 3.4: Bulk rollback (per-VM, by exact name)

Uses the `SNAP` map captured in Step 3.2. Per-VM loop because rollback's `--snapshot` takes a single name.

**Command**:
```bash
for vmid in 5003 4103 4105; do
  echo "=== rollback $vmid -> ${SNAP[$vmid]} ==="
  pve vm snapshot rollback --vmid "$vmid" --snapshot "${SNAP[$vmid]}" -y 2>&1 | tail -6
done
```

**Expected**: Each VM reports `✅ VM <id> ... rolled back to snapshot '<name>' successfully`. (Each single-VM rollback prints its own one-row summary.)

**Result**: ✅ *(to be observed)*

### Step 3.5: Post-rollback power check — start any stopped VM (MANDATORY)

Disk-only rollback leaves each VM **powered off**; required after every rollback.

**Command**:
```bash
declare -A NODE=( [5003]=pve-node5 [4103]=pve-node6 [4105]=pve-node4 )
for vmid in 5003 4103 4105; do
  node="${NODE[$vmid]}.example.com"
  st=$(ssh -o ConnectTimeout=5 root@"$node" "qm status $vmid")
  echo "VM $vmid @ ${NODE[$vmid]}: $st"
  echo "$st" | grep -q stopped && ssh -o ConnectTimeout=5 root@"$node" "qm start $vmid"
done
sleep 3
for vmid in 5003 4103 4105; do
  node="${NODE[$vmid]}.example.com"
  echo "VM $vmid: $(ssh -o ConnectTimeout=5 root@"$node" "qm status $vmid")"
done
```

**Expected**: All three read `stopped` after rollback, then `running` after start. (VM 4103 may print a benign Proxmox EFI/secure-boot certificate **warning** on start — the start still succeeds.)

**Result**: ✅ *(to be observed)*

### Step 3.6: Bulk delete (per-VM, by exact name)

Reuses the `SNAP` map from Step 3.2. Per-VM loop — do **NOT** use `--all` (see gotcha 6: `--all` is sequential, summary-less, and would delete snapshots the runbook did not create).

**Command**:
```bash
for vmid in 5003 4103 4105; do
  echo "=== delete $vmid -> ${SNAP[$vmid]} ==="
  pve vm snapshot delete --vmid "$vmid" --snapshot "${SNAP[$vmid]}" -y 2>&1 | tail -6
done
```

**Expected**: Each VM reports `✅ Snapshot '<name>' deleted successfully` for its `bulkdemo-*` snapshot.

**Result**: ✅ *(to be observed)*

### Step 3.7: Verify clean state

**Command**:
```bash
pve vm snapshot list --vmid 5003,4103,4105 2>&1 | grep -E 'VM [0-9]+:|bulkdemo|No snapshots'
```

**Expected**: No `bulkdemo-*` snapshots anywhere. All three VMs show `No snapshots` and `🟢 running`.

**Result**: ✅ *(to be observed)*

---

## Part 3B: Bulk Lifecycle by VM Name — VMs `app-01`, `docs-vm-03`, `docs-vm-05`

Mirrors Part 3's bulk lifecycle but drives every command with **`--vmname`** (comma-separated names) instead of `--vmid`, validating name resolution across all three nodes through the full create → rollback → delete cycle. Uses a distinct prefix (`bulkvn`) so these never collide with Part 3's `bulkdemo-*`.

> **Note on names:** as in Part 3, generated names embed each VM's name and a per-VM timestamp, so rollback/delete operate **per-VM by exact name**. The capture and loops below resolve each VM by name.

### Step 3B.1: Bulk list by name (read-only)

**Command**:
```bash
pve vm snapshot list \
  --vmname app-01,docs-vm-03,docs-vm-05 \
  2>&1 | grep -E 'VM [0-9]+:|No snapshots'
```

**Expected**: All three VMs `Status: 🟢 running`, each with no user-created snapshots. Confirms the three names resolve to VMIDs 5003, 4103, 4105.

**Result**: ✅ *(to be observed)*

### Step 3B.2: Bulk create by name

**Command**:
```bash
pve vm snapshot create \
  --vmname app-01,docs-vm-03,docs-vm-05 \
  --prefix bulkvn -y 2>&1 | tail -20
```

**Expected**: Three `✅ ... created successfully` lines + `BULK OPERATION SUMMARY` with `Total Operations: 3`, `Successful: 3 (100.0%)`.

> **⚠️ This capture block is load-bearing — Steps 3B.4 and 3B.6 will fail without it**, for the same reason as Step 3.2. The `VNSNAP` map populated here is the **only** input the rollback/delete loops use. If you skip it, or run this in a shell holding a stale map from Part 2B, the loops target wrong/old names and every VM fails with `snapshot '...' not found`. **Run this block now and confirm all three lines print a non-empty `bulkvn-*` name before continuing.** (This is exactly the failure mode that orphaned snapshots during the first live run of this runbook.)

```bash
declare -A VNSNAP
for name in app-01 docs-vm-03 docs-vm-05; do
  VNSNAP[$name]=$(pve vm snapshot list --vmname "$name" 2>&1 | grep -oP 'bulkvn-\S+')
  echo "$name: ${VNSNAP[$name]}"
  [ -z "${VNSNAP[$name]}" ] && { echo "ABORT: no bulkvn snapshot captured for $name — do not proceed to 3B.4"; break; }
done
```

> **Note on the generated name format:** the name is `<prefix>-<vmname>-<YYYYMMDD-HHMM>`, truncated to 40 chars total (`pkg/snapshot/operations.go` `maxSnapshotNameLength`). The longer VM names here (`docs-vm-03/05`) lose the `-HHMM` suffix — e.g. `bulkvn-docs-vm-03-20260626-17` — while the shorter one (`app-01`) keeps it (`bulkvn-app-01-20260626-1730`). **Do not assume the suffix is present** — always capture the exact name from `list`.

**Result**: ✅ *(to be observed — record the three captured snapshot names)*

### Step 3B.3: Verify all three snapshots present (by name)

**Command**:
```bash
pve vm snapshot list \
  --vmname app-01,docs-vm-03,docs-vm-05 \
  2>&1 | grep -E 'VM [0-9]+:|bulkvn'
```

**Expected**: One `bulkvn-*` snapshot per VM (disk-only).

**Result**: ✅ *(to be observed)*

### Step 3B.4: Bulk rollback by name (per-VM, by exact snapshot name)

Uses the `VNSNAP` map captured in Step 3B.2.

**Command**:
```bash
for name in app-01 docs-vm-03 docs-vm-05; do
  echo "=== rollback $name -> ${VNSNAP[$name]} ==="
  pve vm snapshot rollback --vmname "$name" --snapshot "${VNSNAP[$name]}" -y 2>&1 | tail -6
done
```

**Expected**: Each VM reports `✅ VM <id> ... rolled back to snapshot '<name>' successfully`.

**Result**: ✅ *(to be observed)*

### Step 3B.5: Post-rollback power check — start any stopped VM (MANDATORY)

**Command**:
```bash
declare -A NODE=( [app-01]=pve-node5 [docs-vm-03]=pve-node6 [docs-vm-05]=pve-node4 )
declare -A VMID=( [app-01]=5003 [docs-vm-03]=4103 [docs-vm-05]=4105 )
for name in app-01 docs-vm-03 docs-vm-05; do
  node="${NODE[$name]}.example.com"; id="${VMID[$name]}"
  st=$(ssh -o ConnectTimeout=5 root@"$node" "qm status $id")
  echo "$name ($id) @ ${NODE[$name]}: $st"
  echo "$st" | grep -q stopped && ssh -o ConnectTimeout=5 root@"$node" "qm start $id"
done
sleep 3
for name in app-01 docs-vm-03 docs-vm-05; do
  node="${NODE[$name]}.example.com"; id="${VMID[$name]}"
  echo "$name: $(ssh -o ConnectTimeout=5 root@"$node" "qm status $id")"
done
```

**Expected**: All three read `stopped` after rollback, then `running` after start. (VM 4103 may emit the benign EFI cert warning on start.)

**Result**: ✅ *(to be observed)*

### Step 3B.6: Bulk delete by name (per-VM, by exact snapshot name)

Reuses the `VNSNAP` map from Step 3B.2. Per-VM loop — not `--all`.

**Command**:
```bash
for name in app-01 docs-vm-03 docs-vm-05; do
  echo "=== delete $name -> ${VNSNAP[$name]} ==="
  pve vm snapshot delete --vmname "$name" --snapshot "${VNSNAP[$name]}" -y 2>&1 | tail -6
done
```

**Expected**: Each VM reports `✅ Snapshot '<name>' deleted successfully` for its `bulkvn-*` snapshot.

**Result**: ✅ *(to be observed)*

### Step 3B.7: Verify clean state (by name)

**Command**:
```bash
pve vm snapshot list \
  --vmname app-01,docs-vm-03,docs-vm-05 \
  2>&1 | grep -E 'VM [0-9]+:|bulkvn|No snapshots'
```

**Expected**: No `bulkvn-*` snapshots anywhere. All three VMs show `No snapshots` and `🟢 running`.

**Result**: ✅ *(to be observed)*

---

## Part 4: Post-checks (final state verification)

### Step 4.1: Verify all test snapshots removed and VMs running

**Command**:
```bash
pve vm snapshot list --vmid 5003,4103,4105 2>&1 | grep -E 'VM [0-9]+:|demo-app-01|vndemo-app-01|bulkdemo|bulkvn|No snapshots'
for n in pve-node4 pve-node5 pve-node6; do
  case $n in *pve-node4) id=4105;; *pve-node5) id=5003;; *pve-node6) id=4103;; esac
  echo "$n / $id: $(ssh -o ConnectTimeout=5 root@pve-node2.example.com "qm status $id")"
done
```

**Expected**: No `demo-*` / `vndemo-*` / `bulkdemo-*` / `bulkvn-*` snapshots anywhere. All three VMs show `No snapshots` and `running`.

**Result**: ✅ *(to be observed)*

### Step 4.2: Clean up the scratch directory (Option A install only)

**Command**:
```bash
cd / && rm -rf /tmp/pve-release-test && ls -d /tmp/pve-release-test 2>&1 || echo "removed"
```

**Expected**: `removed`.

**Result**: ✅ *(to be observed)*

---

## Part 5: Capacity & Disk Reports (read-only)

These commands exercise the **read-only report surface** added after the
snapshot lifecycle parts. They are all GET-only — no snapshots, no VM state
changes, no storage writes. They reuse the credentials exported in Step 1.3.

> **Permission note:** the `/nodes/{node}/disks/*` endpoints (ZFS, disk list,
> SMART) require `Sys.Audit` on `/nodes`. The `cli-user@pve` token used in this
> runbook has it; if yours does not, `node disks *` surfaces a clear permission
> hint instead of failing opaquely. Run **Step 5.0** first to confirm.

### Step 5.0: Token capability preflight

**Command**:
```bash
pve node capability
```

**Expected**: a ✅/❌ matrix. With the runbook token, all six endpoints
(`/cluster/resources`, `/cluster/status`, `/cluster/ha/status/current`,
`/nodes/{node}/status`, `/nodes/{node}/disks/zfs`, `/nodes/{node}/disks/list`)
show ✅.

**Result**: ✅ *(to be observed)*

### Step 5.1: Cluster capacity and N-1 failover

**Command**:
```bash
pve node resource capacity
```

**Expected**: per-node and cluster CPU/RAM overcommit, plus an N-1 failover
verdict (lose the largest online node). Note the output states RAM figures are
**allocated** (VM `maxmem`), not actual in-guest used memory.

**Result**: ✅ *(to be observed)*

### Step 5.2: ZFS pool usage (cluster-wide)

**Command**:
```bash
pve node disks zfs
```

**Expected**: one row per ZFS pool (e.g. `rpool` on each of the 3 nodes) with
size/alloc/free/frag/health (🟢 ONLINE).

**Result**: ✅ *(to be observed)*

### Step 5.3: ZFS pool detail (vdev tree)

**Command**:
```bash
pve node disks zfs-detail --node pve-node4 --pool rpool
```

**Expected**: the pool's vdev tree with read/write/cksum error counters (all 0
on a healthy pool) and the last scrub line.

**Result**: ✅ *(to be observed)*

### Step 5.4: Physical disks

**Command**:
```bash
pve node disks list --node pve-node4
```

**Expected**: one row per physical disk with size/model/wearout/health (🟢 OK)
and usage (e.g. `zfs_member`).

**Result**: ✅ *(to be observed)*

### Step 5.5: SMART health for one disk

**Command**:
```bash
pve node disks smart --node pve-node4 --disk /dev/sda
```

**Expected**: overall SMART health (🟢 OK), plus either an attribute table
(ATA disks) or raw smartctl text (NVMe/SCSI disks: endurance %, temperature,
power-on hours).

**Result**: ✅ *(to be observed)*

### Step 5.6: Cluster health snapshot

**Command**:
```bash
pve cluster health
```

**Expected**: quorum status (🟢 quorate), node membership (3/3 online), storage
availability, and HA state. On this cluster HA is not configured → prints
`HA: not configured` (distinct from a ⚠️ permission/transient-error line). The
footer reminds this is a REST-only snapshot, not a substitute for on-node checks.

**Result**: ✅ *(to be observed)*

---

## Cleanup & Recovery Plan (if a step fails or leaves residue)

| Situation | Action |
|-----------|--------|
| A `demo-*` / `vndemo-*` snapshot left behind on 5003 | `pve vm snapshot delete --vmid 5003 --snapshot '<exact-name>' -y` |
| A `bulkdemo-*` / `bulkvn-*` snapshot left behind on 5003 / 4103 / 4105 | `pve vm snapshot delete --vmid <id> --snapshot '<exact-name>' -y` (per-VM) — **never** `--all` |
| A VM left `stopped` after rollback | `ssh root@<node>.example.com "qm start <vmid>"`, then confirm `qm status <vmid>` → running (node map: 5003→pve-node5, 4103→pve-node6, 4105→pve-node4) |
| Bulk create shows `Successful: 2 (66.7%)` / one `Failed:` | One VM errored mid-batch (e.g. node unreachable, VM locked). The summary's `FAILED OPERATIONS:` block names the VM and cause; fix and re-run create for that single VM |
| Third bulk VM appears to "hang" | It is not hung — concurrency cap is 2, so the 3rd VM starts only after one of the first two finishes. Wait for the `BULK OPERATION SUMMARY` block |
| Credentials fail (`op` not signed in / 403) | Re-check 1Password auth (Step 0.2) and the token role on Proxmox; do **not** proceed past Step 2.1 until `list` works |
| `pve snapshot <verb>` fails with "unknown command" | You hit the stale-help-text gotcha — use **`pve vm snapshot <verb>`** instead |
| `--snapshot_name` rejected | Wrong flag — use **`--snapshot`** |
| Generated name capture returns empty | The `grep -oP` pattern expects `<prefix>-<vmname>-<TS>`; re-run `pve vm snapshot list --vmid <id>` and copy the exact name manually |
| Rollback/delete fails `snapshot '...' not found` for **every** VM | The `SNAP`/`VNSNAP` map is empty or **stale** (left over from an earlier part in the same shell). Re-run the Step 2.2 / 2B.2 / 3.2 / 3B.2 capture block in the current shell and confirm non-empty names before retrying |
| Generated name is missing the `-HHMM` suffix | Not a bug — names are truncated to 40 chars (`pkg/snapshot/operations.go` `maxSnapshotNameLength`); long VM names lose the suffix. Capture the exact name from `list`; never hardcode or assume the timestamp |

**⚠️ Run this against disposable test VMs only.** Although the walkthrough deletes every snapshot it creates, **rollback is not a no-op**: rolling back a disk snapshot discards every disk write made since that snapshot was taken and can power the VM off mid-run. On a VM with valuable data, that is data loss. Treat the demo as safe only on throwaway test VMs, and verify real backup and recovery procedures independently of snapshot tooling — snapshots are not backups.

---

## Completion Summary

| Step | Description | Status |
|------|-------------|--------|
| 0.1 | Target VM 5003 running on pve-node5 | ⬜ |
| 0.2 | 1Password CLI authenticated (if `op://` used) | ⬜ |
| 1.1 | `pve` v1.5.0 installed | ⬜ |
| 1.2 | Version verified | ⬜ |
| 1.3 | Credentials exported | ⬜ |
| 2.1 | List by `--vmid` (auth) | ⬜ |
| 2.2 | Create by `--vmid` | ⬜ |
| 2.3 | Verify snapshot listed | ⬜ |
| 2.4 | Rollback by `--vmid` | ⬜ |
| 2.5 | Post-rollback start (5003) | ⬜ |
| 2.6 | Delete by `--vmid` | ⬜ |
| 2.7 | Verify clean (`--vmid`) | ⬜ |
| 2B.1 | List by `--vmname` | ⬜ |
| 2B.2 | Create by `--vmname` | ⬜ |
| 2B.3 | Verify snapshot listed (name) | ⬜ |
| 2B.4 | Rollback by `--vmname` | ⬜ |
| 2B.5 | Post-rollback start (5003) | ⬜ |
| 2B.6 | Delete by `--vmname` | ⬜ |
| 2B.7 | Verify clean (`--vmname`) | ⬜ |
| 3.1 | Bulk list (3 VMs) | ⬜ |
| 3.2 | Bulk create (3) | ⬜ |
| 3.3 | Verify 3 snapshots | ⬜ |
| 3.4 | Bulk rollback (3) | ⬜ |
| 3.5 | Post-rollback start (3) | ⬜ |
| 3.6 | Bulk delete (3) | ⬜ |
| 3.7 | Verify clean (bulk) | ⬜ |
| 3B.1 | Bulk list by `--vmname` | ⬜ |
| 3B.2 | Bulk create (3) by `--vmname` | ⬜ |
| 3B.3 | Verify 3 snapshots (name) | ⬜ |
| 3B.4 | Bulk rollback (3) by `--vmname` | ⬜ |
| 3B.5 | Post-rollback start (3) | ⬜ |
| 3B.6 | Bulk delete (3) by `--vmname` | ⬜ |
| 3B.7 | Verify clean (name) | ⬜ |
| 4.1 | Final state clean (3 VMs, no test snapshots) | ⬜ |
| 4.2 | Scratch dir removed | ⬜ |
| 5.0 | Token capability preflight (6/6 ✅) | ⬜ |
| 5.1 | Capacity + N-1 failover report | ⬜ |
| 5.2 | ZFS pool usage (cluster-wide) | ⬜ |
| 5.3 | ZFS pool detail (vdev tree) | ⬜ |
| 5.4 | Physical disks | ⬜ |
| 5.5 | SMART health | ⬜ |
| 5.6 | Cluster health snapshot | ⬜ |

**Escalation**: If any step fails and the Cleanup & Recovery Plan does not resolve it, stop and seek approval before retrying destructive steps — Do not delete snapshots you did not create, and never use `--all` on delete.
