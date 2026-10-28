# Command Reference

Complete reference for all `pve` CLI commands, flags, and parameters.

## Global Flags

All commands inherit these flags from the root command.

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--config` | | string | | Config file path |
| `--batch` | | bool | `false` | Batch mode — no interactive prompts. Accepts token auth or `PVE_PASSWORD` |
| `--yes` | `-y` | bool | `false` | Auto-confirm operations |
| `--verbose` | `-v` | bool | `false` | Verbose output |
| `--quiet` | `-q` | bool | `false` | Quiet output |
| `--dry-run` | | bool | `false` | Preview without making changes |

---

## Command Hierarchy

```
pve
├── vm [--node]
│   ├── list
│   ├── details
│   ├── start
│   ├── stop
│   ├── shutdown
│   ├── reboot
│   ├── reset
│   ├── suspend
│   ├── resume
│   ├── config
│   │   ├── get
│   │   ├── set
│   │   └── pending
│   ├── clone
│   ├── migrate
│   ├── resize
│   ├── move-disk
│   ├── template
│   ├── create
│   ├── destroy
│   ├── agent
│   │   ├── ping
│   │   ├── info
│   │   ├── osinfo
│   │   ├── hostname
│   │   ├── time
│   │   ├── timezone
│   │   ├── users
│   │   ├── vcpus
│   │   ├── memory-blocks
│   │   ├── memory-block-info
│   │   ├── network
│   │   ├── fsinfo
│   │   ├── fstrim
│   │   ├── fsfreeze-status
│   │   ├── fsfreeze-freeze
│   │   ├── fsfreeze-thaw
│   │   ├── exec
│   │   ├── set-password
│   │   ├── shutdown
│   │   ├── suspend-ram
│   │   ├── suspend-disk
│   │   └── suspend-hybrid
│   ├── cloudinit
│   │   ├── pending
│   │   ├── dump
│   │   └── regenerate
│   ├── firewall
│   │   ├── rules
│   │   │   ├── list
│   │   │   ├── add
│   │   │   ├── update
│   │   │   └── delete
│   │   ├── options
│   │   │   ├── get
│   │   │   └── set
│   │   ├── alias
│   │   │   ├── list
│   │   │   ├── add
│   │   │   ├── update
│   │   │   └── delete
│   │   ├── ipset
│   │   │   ├── list
│   │   │   ├── create
│   │   │   ├── add
│   │   │   ├── update
│   │   │   ├── remove
│   │   │   └── delete
│   │   └── refs
│   ├── snapshot
│   │   ├── create
│   │   ├── list
│   │   ├── rollback
│   │   └── delete
│   ├── backup
│   │   ├── create
│   │   ├── list
│   │   ├── restore
│   │   └── delete
│   └── bulk
│       ├── start
│       ├── stop
│       ├── shutdown
│       ├── reboot
│       └── backup
├── cluster
│   ├── task
│   │   ├── list
│   │   ├── running
│   │   ├── failed
│   │   ├── status
│   │   ├── log
│   │   └── stop
│   ├── storage
│   │   ├── list-backup
│   │   └── list-vm
│   ├── backup-job
│   │   ├── list
│   │   ├── create
│   │   ├── update
│   │   ├── delete
│   │   └── not-covered
│   ├── health
│   └── network
│       ├── list
│       ├── summary
│       ├── show
│       ├── create-bridge
│       ├── delete
│       ├── apply
│       ├── revert
│       ├── sdn
│       │   ├── zones
│       │   └── vnets
│       └── firewall
│           └── rules
├── node
│   ├── list
│   ├── status
│   ├── services
│   ├── service
│   │   ├── start
│   │   ├── stop
│   │   ├── restart
│   │   └── status
│   ├── reboot
│   ├── shutdown
│   ├── version
│   ├── capability
│   ├── disks
│   │   ├── zfs
│   │   ├── zfs-detail
│   │   ├── list
│   │   └── smart
│   └── resource
│       ├── stats
│       ├── capacity
│       ├── list
│       ├── nodes
│       ├── vms
│       ├── storages
│       ├── node
│       ├── vm
│       └── history
├── container
│   ├── list
│   ├── summary
│   ├── show
│   ├── status
│   ├── create
│   ├── start
│   ├── stop
│   ├── shutdown
│   ├── restart
│   ├── delete
│   ├── clone
│   └── snapshot
│       ├── create
│       ├── list
│       ├── rollback
│       └── delete
├── zfs
│   ├── pool
│   │   ├── list
│   │   ├── status
│   │   ├── create
│   │   └── destroy
│   ├── storage
│   │   ├── list
│   │   ├── create
│   │   ├── update
│   │   └── delete
│   └── replication
│       ├── list
│       ├── status
│       ├── create
│       ├── update
│       ├── run
│       └── delete
└── access
    ├── create
    ├── add-token
    ├── revoke-token
    ├── remove
    ├── list
    ├── grant
    └── acl-list
```

---

## VM Management

### `pve vm`

Persistent flag inherited by all VM subcommands:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Filter to VMs on a specific node |

### `pve vm list`

List all VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--status` | string | | Filter by status: `running`, `stopped` |

### `pve vm details`

Show VM details. Requires a single VM ID.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--vmid` | stringSlice | | **yes** | VM ID (single VM only) |

### `pve vm start`

Start one or more VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |

### `pve vm stop`

Force-stop one or more VMs. Not a graceful shutdown.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |

### `pve vm shutdown`

Gracefully shut down one or more VMs (ACPI signal).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |

### `pve vm reboot`

Reboot one or more VMs by shutting them down (ACPI) and starting them again.
Applies any pending configuration changes. VMs that are not running are
skipped. Requires `VM.PowerMgmt`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |

### `pve vm reset`

Hard-reset one or more VMs — the equivalent of the physical reset button. Not
graceful: the guest OS receives no ACPI event. Prefer `pve vm reboot` unless a
hard reset is specifically required. VMs that are not running are skipped.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |

### `pve vm suspend`

Suspend one or more running VMs. By default the VM is paused in RAM: it stops
executing but stays resident on the node and resumes almost instantly. With
`--to-disk` the VM state is written to storage and the memory is released,
which survives a node reboot but is slower to save and restore.

VMs that are not running are skipped. Requires `VM.PowerMgmt`; `--to-disk`
additionally requires `Datastore.AllocateSpace` on the state storage and is
refused by Proxmox for HA-managed VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |
| `--to-disk` | bool | `false` | Suspend to disk instead of pausing in RAM |
| `--state-storage` | string | | Storage for the VM state (requires `--to-disk`) |

### `pve vm resume`

Resume one or more suspended VMs. VMs that are not paused are skipped.
Requires `VM.PowerMgmt`.

**Note:** a paused VM still reports its status as `running` in VM listings —
only the QEMU-level state distinguishes it. This command therefore reads each
VM's actual run state rather than trusting the listing.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |

---

## VM Configuration

All three subcommands accept the full VM selector syntax on `--vmid` /
`--vmname` — comma-separated lists, ranges, wildcards, and the keywords `all`,
`running`, `stopped`. Output is one block per VM.

### `pve vm config get`

Show the current configuration of one or more VMs. Requires `VM.Audit`.

A VM whose config cannot be read is reported and the sweep continues; the
command exits non-zero if any VM failed. With more than one target a footer
reports `N VM(s) read, M failed`, so an empty result cannot be mistaken for a
failed scan.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs, ranges, wildcards or keywords |
| `--vmname` | stringSlice | | VM names or patterns (alternative to `--vmid`) |
| `--key` | stringSlice | | Only show these configuration keys |

```bash
pve vm config get --vmid 4103 --key cores,memory
pve vm config get --vmid 4103,4105 --key cores,memory
pve vm config get --vmid '4103-4105' --key cores
pve vm config get --vmid running --key cores
```

### `pve vm config set`

Change the configuration of one or more VMs. The same change set is applied to
every selected VM; the current and new values are shown per VM for confirmation
before anything is written, with unchanged keys marked `(no change)`.

**Confirmation requires typing the exact number of VMs** shown in the diff, not
just `y` — the same guard `pve vm snapshot prune` uses, because the selector can
target many VMs at once. As everywhere else in this CLI, `-y` skips the prompt.

Each VM's write is guarded by its own digest, captured in the read that built
its diff. A VM whose config cannot be read is excluded from the write set and
reported. Writes are applied sequentially and reported per VM; the command exits
non-zero if any VM failed, so a partial application is always visible.

Changes that cannot be applied to a running VM are staged as *pending* and take
effect on the next full stop/start or `pve vm reboot` — a shutdown and start
from inside the guest is not enough. Use `pve vm config pending` to see what is
staged.

By default the write is guarded by the configuration digest read at the start of
the command, so a concurrent modification fails cleanly instead of silently
overwriting someone else's change.

`--set` and `--delete` are repeatable and are **not** comma-split, so values
containing commas or newlines (`net0=virtio,bridge=vmbr0`, a multi-line
`description`) are passed through intact.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs, ranges, wildcards or keywords |
| `--vmname` | stringSlice | | VM names or patterns (alternative to `--vmid`) |
| `--set` | stringArray | | Configuration to set as `key=value` (repeatable) |
| `--delete` | stringArray | | Configuration keys to delete (repeatable) |
| `--no-digest-check` | bool | `false` | Do not guard the write against concurrent modification |

```bash
pve vm config set --vmid 4103 --set cores=4 --set memory=8192
pve vm config set --vmid 4103 --set net0=virtio,bridge=vmbr0,tag=973
pve vm config set --vmid 4103 --delete description
pve vm config set --vmid 4103,4105 --set cores=4     # type '2' to confirm
```

### `pve vm config pending`

Show configuration changes staged for the next power cycle. Requires
`VM.Audit`. With more than one target a footer reports
`N VM(s) with pending changes, M with none, K failed`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs, ranges, wildcards or keywords |
| `--vmname` | stringSlice | | VM names or patterns (alternative to `--vmid`) |

---

## VM Provisioning

### `pve vm clone`

Create a copy of a VM or template. Requires `VM.Clone`.

A normal VM is **always** fully cloned — Proxmox only creates linked clones from
templates, so `--full` has no effect on a normal VM. A linked clone shares disk
data with its template, which makes it near-instant but means the template
cannot be deleted while linked clones exist. The command reports which kind of
clone you will actually get before asking for confirmation.

`--target` is only allowed if the source VM is on shared storage.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | Source VM ID |
| `--newid` | string | | VM ID for the clone |
| `--name` | string | | Name for the new VM |
| `--description` | string | | Description for the new VM |
| `--pool` | string | | Add the new VM to this resource pool |
| `--snapname` | string | | Clone from this snapshot instead of the current state |
| `--storage` | string | | Target storage (full clone only) |
| `--format` | string | | Target disk format: `raw`, `qcow2` or `vmdk` |
| `--target` | string | | Target node (requires shared storage) |
| `--full` | bool | `false` | Copy all disks instead of creating a linked clone |
| `--bwlimit` | int | `0` | I/O bandwidth limit in KiB/s |

### `pve vm migrate`

Migrate a VM to another cluster node. Requires `VM.Migrate`.

Preconditions are read and displayed **before** anything is started: the nodes
the VM may move to, disks on local storage, and passed-through devices. If the
move would fail, the command says why and stops — a migration that fails partway
is far more disruptive than one that never starts.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | VM ID |
| `--target` | string | | Target node |
| `--online` | bool | `false` | Live-migrate a running VM |
| `--with-local-disks` | bool | `false` | Also migrate disks on local storage |
| `--target-storage` | string | | Target storage mapping |
| `--migration-type` | string | | `secure` or `insecure` |
| `--migration-network` | string | | CIDR of the network used for migration |
| `--bwlimit` | int | `0` | I/O bandwidth limit in KiB/s |

### `pve vm resize`

Extend a VM disk. Requires `VM.Config.Disk`.

The size is an absolute target (`32G`) or an increment (`+4G`).

**Disks can only be extended.** Proxmox has no shrink operation, so a negative
size is rejected. Extending the disk also does not extend the filesystem inside
the guest — that remains a separate step in the guest OS.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | VM ID |
| `--disk` | string | | Disk to resize, e.g. `scsi0` |
| `--size` | string | | New size (`32G`) or increment (`+4G`) |

### `pve vm move-disk`

Move a disk to a different storage, or reassign it to another VM. Requires
`VM.Config.Disk`.

Without `--delete` the original is kept as an unused volume, so the move is
reversible and the space is not reclaimed until you remove it.

`--storage` and `--target-vmid` are mutually exclusive.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | VM ID |
| `--disk` | string | | Disk to move, e.g. `scsi0` |
| `--storage` | string | | Target storage |
| `--target-vmid` | string | | Reassign the disk to this VM |
| `--target-disk` | string | | Drive name on the target VM |
| `--format` | string | | Target disk format: `raw`, `qcow2` or `vmdk` |
| `--delete` | bool | `false` | Delete the source disk after a successful copy |
| `--bwlimit` | int | `0` | I/O bandwidth limit in KiB/s |

### `pve vm template`

Convert a VM into a template. Requires `VM.Allocate`.

**This is one-way.** Proxmox provides no API to convert a template back into a
regular VM, and a template cannot be started. The VM must be stopped.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | VM ID |
| `--disk` | string | | Only convert this disk |

---

## VM Create and Destroy

### `pve vm create`

Create a new VM. The API accepts roughly a hundred parameters; this exposes the
practical subset as flags and `--raw` for anything else.

Disks and network devices are passed through as Proxmox device specs, because
that syntax is the API's own:

```bash
# Minimal: allocate the next free VMID on a node
pve vm create --node pve1 --name web01 --cores 2 --memory 4096 \
  --disk scsi0=local-zfs:32 --net net0=virtio,bridge=vmbr0

# Explicit VMID, boot from an ISO, power on when done
pve vm create --node pve1 --vmid 200 --name web01 --cores 2 --memory 4096 \
  --disk scsi0=local-zfs:32 --net net0=virtio,bridge=vmbr0 \
  --cdrom ide2=local:iso/rocky10.iso --boot 'order=ide2;scsi0' --start

# Anything not covered by a flag
pve vm create --node pve1 --name web01 --raw numa=1 --raw hotplug=disk,network
```

Device flags (`--disk`, `--net`, `--cdrom`, `--raw`) are repeatable and are
**not** comma-split: a spec like `net0=virtio,bridge=vmbr0` arrives intact.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Node to create the VM on (required) |
| `--vmid` | string | next free | VM ID |
| `--name` | string | | VM name |
| `--cores` | int | | CPU cores per socket |
| `--sockets` | int | | CPU sockets |
| `--cpu` | string | | CPU model, e.g. `host` |
| `--memory` | int | | Memory in MiB |
| `--ostype` | string | | Guest OS type, e.g. `l26`, `win11` |
| `--bios` | string | | `seabios` or `ovmf` |
| `--machine` | string | | Machine type, e.g. `q35` |
| `--scsihw` | string | | SCSI controller, e.g. `virtio-scsi-single` |
| `--agent` | string | | QEMU guest agent: `1` to enable |
| `--boot` | string | | Boot order, e.g. `order=scsi0;net0` |
| `--disk` | stringArray | | Disk as `dev=spec`, e.g. `scsi0=local-zfs:32` (repeatable) |
| `--net` | stringArray | | NIC as `dev=spec`, e.g. `net0=virtio,bridge=vmbr0` (repeatable) |
| `--cdrom` | stringArray | | CD/DVD as `dev=spec`, e.g. `ide2=local:iso/x.iso` (repeatable) |
| `--raw` | stringArray | | Any other API parameter as `key=value` (repeatable) |
| `--description` | string | | VM description |
| `--pool` | string | | Add the VM to this resource pool |
| `--start` | bool | `false` | Start the VM once created |

### `pve vm destroy`

Destroy a VM and all of its disks. **THIS CANNOT BE UNDONE** — the disks go
with the VM, no snapshot survives it, and there is no Proxmox-side recycle bin.

Because of that, this command is deliberately narrower than every other VM
command:

- `--vmid` takes exactly **one** id — no ranges, lists, wildcards, or
  `running`/`stopped`/`all` keywords. There is no bulk form.
- The VM must be stopped.
- A VM with protection enabled is refused — clear it deliberately first
  (`pve vm config set --vmid N --set protection=0`).
- Confirmation requires typing the VMID back, not y/N. Non-interactively,
  `--yes` **and** `--force` are both required, so a `-y` left over from another
  command cannot destroy anything.
- Backup jobs, HA resources and replication entries referencing the VMID are
  purged by default (a stale entry would be silently inherited by the next VM
  to reuse the id); `--no-purge` keeps them.

```bash
pve vm destroy --vmid 999 --dry-run          # preview only
pve vm destroy --vmid 999                    # interactive: type the VMID back
pve vm destroy --vmid 999 --yes --force      # scripts
pve vm destroy --vmid 999 --no-purge --yes --force
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | The single VM ID to destroy (or use `--vmname`) |
| `--vmname` | string | | VM name, matched EXACTLY |
| `--no-purge` | bool | `false` | Keep backup-job/HA/replication entries for this VMID |
| `--destroy-unreferenced-disks` | bool | `false` | Also remove disks on the VM's storages that belong to this VMID but are not in its config |
| `--force` | bool | `false` | Required alongside `--yes` for non-interactive destroy |

---

## Guest Agent

### `pve vm agent`

Talk to the QEMU guest agent running inside a VM. These commands need
`agent: 1` in the VM config **and** qemu-guest-agent installed and running in
the guest.

Two failure modes look alike and are worth telling apart:

- *"guest agent is not running"* — the agent is absent, or the VM has not
  finished booting (on Rocky 10 it can take ~5 minutes after boot to answer).
- *"Command X has been disabled"* — the agent IS running and refused. That is
  the guest's own policy: hardened images commonly disable `guest-exec` and all
  `guest-file-*` commands. `pve vm agent info` lists what a given guest permits.

Group flags (inherited by every subcommand):

| Flag | Type | Description |
|------|------|-------------|
| `--vmid` | string | VM ID (or use `--vmname`) |
| `--vmname` | string | VM name, matched exactly |
| `--json` | bool | Print the raw agent payload as JSON |

Subcommands:

| Command | Description |
|---------|-------------|
| `ping` | Check that the guest agent responds |
| `info` | Show which agent commands this guest permits |
| `osinfo` | Guest OS name, version and kernel |
| `hostname` | Guest hostname |
| `time` | Guest clock, as nanoseconds since the epoch |
| `timezone` | Guest timezone |
| `users` | Users currently logged in to the guest |
| `vcpus` | Guest vCPU list and online state |
| `memory-blocks` | Guest memory blocks |
| `memory-block-info` | Guest memory block size info |
| `network` | Guest network interfaces and addresses |
| `fsinfo` | Guest filesystems, mountpoints and usage |
| `fstrim` | Run fstrim on the guest's mounted filesystems |
| `fsfreeze-status` | Report whether the guest is frozen |
| `fsfreeze-freeze` | Freeze the guest's filesystems |
| `fsfreeze-thaw` | Thaw the guest's filesystems |
| `exec` | Run a command inside the guest and wait for its output |
| `set-password` | Set a user's password inside the guest |
| `shutdown` | Shut down the guest via the agent |
| `suspend-ram` | Suspend the guest to RAM |
| `suspend-disk` | Suspend the guest to disk |
| `suspend-hybrid` | Hybrid-suspend the guest |

### `pve vm agent exec`

Run a command inside the guest via the agent. The API returns only a PID, so
this polls `exec-status` until the command exits and then prints its stdout,
stderr and exit code — otherwise the result would be invisible.

Put the command after `--` so its own flags are not parsed by pve:

```bash
pve vm agent exec --vmid 100 -- systemctl is-active sshd
```

Requires `guest-exec` to be enabled **inside the guest** (Rocky's default
configuration disables it; check with `pve vm agent info`).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--input-data` | string | | Data to pass to the command on stdin |
| `--timeout` | duration | `30s` | How long to wait for the command to finish |

### `pve vm agent set-password`

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--username` | string | | Guest username (required) |
| `--password` | string | | New password (required) |
| `--crypted` | bool | `false` | The password is already crypt(3)-hashed |

---

## VM Cloudinit

### `pve vm cloudinit`

Work with a VM's cloudinit drive. Alias: `ci`.

Three things worth knowing before using these:

- cloudinit **settings** are changed with `pve vm config set` (`ciuser`,
  `cipassword`, `citype`, `ciupgrade`, `sshkeys`, `nameserver`, `searchdomain`,
  `ipconfigN`). This group does not set them — it reports drift, regenerates
  the drive, and shows what the guest will actually read.
- `pending` shows settings that differ between the VM config and the drive as
  last generated. Non-empty means the guest is not yet seeing them.
- A regenerated drive takes effect on the guest's **next boot**. cloudinit
  reads it at boot, so regenerating alone changes nothing in a running guest —
  and most cloudinit modules only run once per instance anyway.

### `pve vm cloudinit pending`

Show cloudinit settings not yet written to the drive.

### `pve vm cloudinit dump`

Print the rendered cloudinit data the guest will read — the **generated**
output, not the VM config that produced it. The right thing to check when a
guest is not picking up an expected setting.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | `user` | Section to render: `user`, `network`, `meta` |

### `pve vm cloudinit regenerate`

Rebuild the cloudinit drive from the current VM config (alias `update`).
Commits pending settings to the drive; the guest reads them on its next boot.

---

## VM Firewall

### `pve vm firewall`

Manage the per-VM Proxmox firewall. Alias: `fw`. Reads need `VM.Audit`; changes
need `VM.Config.Network`.

Two things about rules that matter in practice:

- **Rules have no stable id.** A rule is identified by its position in the
  chain, so inserting or deleting one renumbers everything after it. Deleting
  several rules means working from the highest position downwards.
- A new rule goes to the **top** of the chain unless `--pos` says otherwise,
  so it takes precedence over the existing ones.

The VM firewall only takes effect when it is enabled **both** on the VM
(`firewall options set --enable`) **and** on the network device (`firewall=1`
in the `netN` config).

### `pve vm firewall rules list`

List a VM's firewall rules in chain order.

### `pve vm firewall rules add`

Add a rule to the chain. `--action` takes `ACCEPT`, `REJECT`, `DROP`, **or a
security group name** (the API validates it as a name pattern, not a fixed
enum). Without `--pos` the rule is inserted at the top of the chain and
therefore takes precedence.

```bash
# Allow inbound SSH
pve vm firewall rules add --vmid 100 --type in --action ACCEPT --proto tcp --dport 22

# Drop everything from one subnet, logged
pve vm firewall rules add --vmid 100 --type in --action DROP \
  --source 192.0.2.0/16 --log warning --comment 'block lab subnet'

# Add it disabled, to enable later
pve vm firewall rules add --vmid 100 --type in --action ACCEPT --dport 443 --disabled
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | | Rule type: `in`, `out`, `forward`, `group` |
| `--action` | string | | `ACCEPT`, `REJECT`, `DROP`, or a security group name |
| `--pos` | int | top | Insert at this position |
| `--macro` | string | | Predefined macro, e.g. `SSH`, `HTTP` |
| `--proto` | string | | Protocol name or number, e.g. `tcp`, `udp`, `icmp` |
| `--source` | string | | Source IP, CIDR, alias or `+ipset` |
| `--dest` | string | | Destination IP, CIDR, alias or `+ipset` |
| `--sport` | string | | Source port, range or list |
| `--dport` | string | | Destination port, range or list |
| `--icmp-type` | string | | ICMP type (only with `--proto icmp`/`icmpv6`) |
| `--iface` | string | | Network device, e.g. `net0` |
| `--log` | string | | Log level: `emerg`…`debug`, `nolog` |
| `--comment` | string | | Free-text comment |
| `--disabled` | bool | `false` | Add the rule disabled |

### `pve vm firewall rules update`

Change the rule at `--pos`. Only passed flags are sent. `--move-to` reorders
the rule and **cannot** be combined with field edits — the API ignores every
other argument when moving, so the combination is refused rather than silently
dropping the edits. `type` and `action` cannot be deleted; change them instead.

Same flags as `rules add`, plus:

| Flag | Type | Description |
|------|------|-------------|
| `--pos` | int | Position of the rule to change (required) |
| `--move-to` | int | Move the rule to this position (mutually exclusive with edits) |
| `--delete` | strings | Rule fields to reset to their default |

### `pve vm firewall rules delete`

Delete the rule at `--pos`. Positions shift after a delete — removing rule 0
makes the old rule 1 become rule 0. To remove several, work from the highest
position downwards, or re-list between deletes.

| Flag | Type | Description |
|------|------|-------------|
| `--pos` | int | Position of the rule to delete (required) |

### `pve vm firewall options get`

Show a VM's firewall options. Warns when the VM-level firewall switch is off
(rules do nothing while it is off).

### `pve vm firewall options set`

Change a VM's firewall options. Boolean switches come in `--x` / `--no-x`
pairs so "set false" is expressible; passing both is refused.

| Flag | Type | Description |
|------|------|-------------|
| `--enable` / `--disable` | bool | Firewall on/off for this VM |
| `--policy-in` | string | Input policy: `ACCEPT`, `REJECT`, `DROP` |
| `--policy-out` | string | Output policy: `ACCEPT`, `REJECT`, `DROP` |
| `--macfilter` / `--no-macfilter` | bool | MAC address filter |
| `--dhcp` / `--no-dhcp` | bool | Allow DHCP |
| `--ndp` / `--no-ndp` | bool | Allow NDP |
| `--radv` / `--no-radv` | bool | Allow router advertisements |
| `--ipfilter` / `--no-ipfilter` | bool | Default IP filters |
| `--log-level-in` | string | Log level for inbound traffic |
| `--log-level-out` | string | Log level for outbound traffic |
| `--delete` | strings | Options to reset to their default |

### `pve vm firewall alias` (aliases: `alias`/`aliases`)

Manage named CIDRs usable in rules.

| Subcommand | Description |
|------------|-------------|
| `list` | List a VM's firewall aliases |
| `add` | Add a named CIDR (`--name`, `--cidr`, `--comment`) |
| `update` | Change an alias's CIDR, comment or name (`--rename` for a new name) |
| `delete` | Delete an alias (`--name`) |

### `pve vm firewall ipset`

Manage a VM's IP sets — named collections of CIDRs, referenced in a rule as
`+name`. An entry marked `--nomatch` inverts that entry: the set matches
everything except it.

The API has no update for the set itself; renaming or changing its comment goes
through `create --rename` (set `--rename` equal to `--name` to change only the
comment).

| Subcommand | Description |
|------------|-------------|
| `list` | List IP sets, or the entries in one with `--name` |
| `create` | Create an IP set, or rename/re-comment one with `--rename` |
| `add` | Add a CIDR to a set (`--name`, `--cidr`, `--nomatch`, `--comment`) |
| `update` | Change an entry's `--nomatch` flag or `--comment` |
| `remove` | Remove a CIDR from a set (`--name`, `--cidr`) |
| `delete` | Delete an IP set — refused while it still has entries unless `--force` |

### `pve vm firewall refs`

List the aliases and IP sets available to this VM's rules — including
datacenter- and SDN-scoped entries, which is why a rule can reference a name
that `alias list` does not show. `--type` filters by `alias` or `ipset`.

---

## VM Snapshots

### `pve vm snapshot create`

Create snapshots for one or more VMs. Use `--prefix` for timestamped names or `--name` for exact names.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |
| `--prefix` | string | | Snapshot name prefix (timestamp appended) |
| `--name` | string | | Exact snapshot name |
| `--vmstate` | bool | `false` | Include VM state (RAM) in snapshot |

### `pve vm snapshot list`

List snapshots for one or more VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |

### `pve vm snapshot rollback`

Rollback one or more VMs to a named snapshot. VM must be stopped.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--vmid` | stringSlice | | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | | VM names (comma-separated) |
| `--snapshot` | string | | **yes** | Snapshot name to rollback to |

### `pve vm snapshot delete`

Delete snapshots from one or more VMs. Use `--snapshot` for specific names or `--all` to remove all.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |
| `--snapshot` | stringSlice | | Snapshot name(s) to delete (comma-separated) |
| `--all` | bool | `false` | Delete all snapshots |

Note: `--all` means "all snapshots on the VMs you named" — it is not
cluster-wide. For that, see `prune` below.

### `pve vm snapshot preview`

Show every snapshot in the cluster as a flat table. Defaults to the whole
cluster; needs no `--vmid`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | Narrow to VM IDs or selectors (`all`, `running`, `stopped`, `72*`, `7201-7205`) |
| `--vmname` | stringSlice | | Narrow to VM names (comma-separated) |
| `--older-than` | string | | Only snapshots older than this age (`30d`, `12h`, `90m`); non-finite or overflowing values are rejected as errors |
| `--name-pattern` | string | | Only snapshots whose name matches this glob |

The global `--node` flag scopes the scan to one node.

```
$ pve vm snapshot preview
VMID      NAME                     NODE             SNAPSHOT                    AGE   RAM
4001     rocky10-vm01     pve-node1    pre-update-20260828         16d   no
4002     rocky10-vm02     pve-node2    backup-20260701-0300        58d   yes

2 snapshot(s) across 2 VM(s) (3 VMs scanned, 1 with none)
```

The trailing summary always reports the denominator, and any VMs that could
not be read are listed with a warning — so an empty table means "nothing to
do", not "the scan silently failed".

### `pve vm snapshot prune`

Delete snapshots matching a filter, across one or many VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs or selectors |
| `--vmname` | stringSlice | | VM names (comma-separated) |
| `--older-than` | string | | Only snapshots older than this age |
| `--name-pattern` | string | | Only snapshots whose name matches this glob |

**At least one filter is required.** Running `prune` with no filter exits
non-zero with an error — there is deliberately no form that deletes every
snapshot in the cluster. Multiple filters combine with AND.

Safety behaviour:

- The full list of matched snapshots is printed before anything is deleted,
  in both dry-run and real mode.
- `--dry-run` stops after printing.
- Confirmation requires typing the **exact snapshot count** shown, not a fixed
  word. `-y`/`--yes` bypasses the prompt for automation.
- If any VM could not be read, a warning states the target set is incomplete
  before the confirmation.
- `--older-than` is strictly greater-than: a snapshot exactly at the threshold
  is not selected. A snapshot with no usable timestamp is never selected by an
  age filter.

```bash
# Always preview first
pve vm snapshot prune --older-than 30d --dry-run

# Then apply
pve vm snapshot prune --older-than 30d
```

---

## VM Backups

### `pve vm backup create`

Create a backup of one or more VMs.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--vmid` | stringSlice | | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | | VM names (comma-separated) |
| `--storage` | string | | **yes** | Target storage ID |
| `--mode` | string | `snapshot` | | Backup mode: `snapshot`, `suspend`, `stop` |
| `--compress` | string | `zstd` | | Compression: `zstd`, `gzip`, `lzo` |

### `pve vm backup list`

List backups for VMs or across storage.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | stringSlice | | VM IDs (comma-separated, ranges supported) |
| `--vmname` | stringSlice | | VM names (comma-separated) |
| `--storage` | string | | Storage to check (checks all if omitted) |
| `--all` | bool | `false` | List all backups in storage (requires `--storage`) |

### `pve vm backup restore`

Restore a VM from a backup file.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--vmid` | stringSlice | | **yes** | Exactly one VM ID (multiple IDs are rejected) |
| `--backup-file` | string | | **yes** | Backup volid |
| `--node` | string | | **yes** | Target node |
| `--storage` | string | | | Target storage |

### `pve vm backup delete`

Delete VM backups. Supports multiple selection strategies.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--vmid` | stringSlice | | **yes** | Exactly one VM ID (multiple IDs are rejected) |
| `--backup-file` | string | | | Specific backup volid to delete |
| `--pattern` | string | | | Delete backups matching a glob (see below) |
| `--keep-count` | int | `0` | | Keep only N most recent backups |
| `--max-age-days` | int | `0` | | Delete backups older than N days |
| `--storage` | string | | | Storage to search |

`--pattern` is a glob matched against the **full volume ID** with
`path.Match` semantics: `*` does **not** cross the `/` storage separator, so
`*2024*` alone does not match `local:backup/vzdump-...2024...` — use
`local:backup/*2024*` or `*/*2024*`. An empty pattern matches everything. A
malformed pattern (e.g. an unterminated `[`) is rejected before any deletion.
Failed deletions are reported and the command exits non-zero, even when some
backups deleted successfully.

---

## VM Bulk Operations

Bulk commands auto-discover matching VMs across the cluster. All bulk commands inherit `--node` from `pve vm` and support `--vmid` to target specific VMs instead of all matching VMs.

### `pve vm bulk start`

Start all stopped VMs. Auto-filters to stopped VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | Comma-separated VM IDs to target (default: all stopped VMs) |

### `pve vm bulk stop`

Force-stop all running VMs. Auto-filters to running VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | Comma-separated VM IDs to target (default: all running VMs) |

### `pve vm bulk shutdown`

Gracefully shut down all running VMs (ACPI signal). Auto-filters to running VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | Comma-separated VM IDs to target (default: all running VMs) |

### `pve vm bulk reboot`

Reboot all running VMs (ACPI shutdown then start, applying pending config
changes). Auto-filters to running VMs.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vmid` | string | | Comma-separated VM IDs to target (default: all running VMs) |

### `pve vm bulk backup`

Backup all VMs.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--storage` | string | | **yes** | Target storage ID |
| `--mode` | string | `snapshot` | | Backup mode: `snapshot`, `suspend`, `stop` |
| `--compress` | string | `zstd` | | Compression: `zstd`, `gzip`, `lzo` |
| `--vmid` | string | | | Comma-separated VM IDs to target (default: all VMs) |

---

## Cluster

### `pve cluster task list`

List cluster tasks.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Filter by node |
| `--running` | bool | `false` | Show only running tasks |
| `--errors` | bool | `false` | Show only failed tasks |
| `--type` | string | | Filter by task type (e.g., `vzdump`) |
| `--user` | string | | Filter by user |
| `--limit` | int | `50` | Maximum number of tasks to return |

### `pve cluster task running`

Quick view of currently running tasks. No flags.

### `pve cluster task failed`

Quick view of recently failed tasks. No flags.

### `pve cluster task status`

Get detailed status of a specific task.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--upid` | string | | **yes** | Task UPID |

### `pve cluster task log`

View task log output.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--upid` | string | | **yes** | Task UPID |
| `--tail` | int | `100` | | Number of lines from end |
| `--follow` | bool | `false` | | Follow log output (streaming) |

### `pve cluster task stop`

Stop a running task.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--upid` | string | | **yes** | Task UPID |

### `pve cluster health`

Show a REST-only cluster health snapshot: quorum status, node membership, HA
resource states, and storage availability. No flags.

> **Note:** This is a REST-only snapshot. It does **not** cover on-node signals
> (systemd services, `journalctl`/`dmesg` errors, `zpool status`, `pvesr`
> replication) — those require SSH access to each node.

---

## Cluster Storage

### `pve cluster storage list-backup`

List backup-capable storages. No flags.

### `pve cluster storage list-vm`

List VM disk storages. No flags.

---

## Cluster Network

### `pve cluster network list`

List network interfaces on a node.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--type` | string | | | Filter by type: `bridge`, `bond`, `eth`, `vlan` |
| `--active` | string | | | Filter by active status: `true`, `false` |

### `pve cluster network summary`

Show network interface summary for a node.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve cluster network show`

Show details of a specific network interface.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--iface` | string | | **yes** | Interface name (e.g., `vmbr0`) |

### `pve cluster network create-bridge`

Create a new network bridge interface.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--iface` | string | | **yes** | Interface name (e.g., `vmbr1`) |
| `--bridge-ports` | string | | **yes** | Bridge ports (e.g., `eth1`) |
| `--address` | string | | | IP address |
| `--netmask` | string | | | Network mask |
| `--gateway` | string | | | Gateway |
| `--comments` | string | | | Description/comments |
| `--autostart` | bool | `true` | | Start on boot |
| `--vlan-aware` | bool | `false` | | Enable VLAN awareness |

### `pve cluster network delete`

Delete a network interface.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--iface` | string | | **yes** | Interface name |

### `pve cluster network apply`

Apply pending network configuration changes.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve cluster network revert`

Revert pending network configuration changes.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve cluster network sdn zones`

List SDN zones.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | | Filter by zone type: `vlan`, `vxlan`, `qinq`, `simple` |

### `pve cluster network sdn vnets`

List SDN virtual networks.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--zone` | string | | Filter by zone name |

### `pve cluster network firewall rules`

List firewall rules for a node.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

---

## Cluster Backup Jobs

Scheduled vzdump job definitions (`jobs.cfg`, the Datacenter → Backup view in
the web UI) — distinct from one-shot `pve vm backup create` and from backup
file management.

**Permissions:** reads need `Sys.Audit` on `/`; changes need `Sys.Modify` on
`/`. **PVEVMAdmin and PVEAdmin include neither** — grant `PVEAuditor` for
reads or `PVESysAdmin` for changes. `--bwlimit`/`--ionice`/`--performance`
additionally require `Sys.Modify`; `tmpdir`/`dumpdir`/`script` are
`root@pam`-only and are not exposed. There is **no digest** parameter on any
job endpoint, and **no include/exclude filter mode** — selection is the
`--all` / `--vmid` / `--pool` trio plus `--exclude`.

### `pve cluster backup-job list`

List scheduled jobs with their computed next run.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--id` | string | | | Show only this job ID (exact match) |

### `pve cluster backup-job create`

Create a job. Omitting `--id` lets the server generate one (visible in the
response or via `list`).

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--schedule` | string | | **yes** | Calendar event, e.g. `sat 02:30`, `*/15`, `daily` |
| `--storage` | string | | **yes** | Target storage ID |
| `--all` | bool | `false` | one of all/vmid/pool | Back up all guests |
| `--vmid` | string | | one of all/vmid/pool | Comma-separated guest **IDs** (no names) |
| `--pool` | string | | one of all/vmid/pool | Back up all guests in this pool |
| `--exclude` | string | | | Guest IDs to exclude (requires `--all`) |
| `--exclude-path` | stringArray | repeatable | | Path/glob to exclude from archives |
| `--id` | string | | | Optional job ID (`pve-configid`, e.g. `weekly-vms`) |
| `--enable` / `--disable` | bool | | | Job state (default when omitted: enabled) |
| `--mode` | string | | | `snapshot` \| `suspend` \| `stop` |
| `--compress` | string | | | `0` \| `1` \| `gzip` \| `lzo` \| `zstd` |
| `--comment` | string | | | Free-text comment |
| `--notes-template` | string | | | Template for backup notes |
| `--mailto` | string | | | Deprecated: comma-separated mail recipients |
| `--mailnotification` | string | | | Deprecated: `always` \| `failure` |
| `--notification-mode` | string | | | `auto` \| `legacy-sendmail` \| `notification-system` |
| `--prune-backups` | string | | | Retention property string, e.g. `keep-last=7,keep-daily=14` |
| `--performance` | string | | | Performance property string (needs `Sys.Modify`) |
| `--fleecing` | string | | | Fleecing property string (VM only) |
| `--pbs-change-detection-mode` | string | | | `legacy` \| `data` \| `metadata` |
| `--protected` | bool | `false` | | Mark resulting backups protected |
| `--remove` | bool | `false` | | Prune older backups per `prune-backups` |
| `--repeat-missed` | bool | `false` | | Run ASAP if a scheduled run was missed |
| `--node` | string | | | Only run when executed on this node |
| `--bwlimit` | int | | | I/O limit in KiB/s (needs `Sys.Modify`) |
| `--ionice` | int | | | BFQ IO priority (needs `Sys.Modify`) |
| `--zstd` | int | | | zstd threads (0 = half the cores) |

### `pve cluster backup-job update`

Change an existing job. Prints a before→after diff of the fields being
changed, then asks for confirmation. Only the passed flags are sent; boolean
flags other than `--enable`/`--disable` are set-only (use `--delete` to
reset a setting to its default).

Same flags as `create`, plus:

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--id` | string | | **yes** | Job ID to change |
| `--delete` | stringSlice | | | Settings to reset to their default, e.g. `bwlimit,protected` |

### `pve cluster backup-job delete`

Remove a job definition — **existing backup files are untouched**. Requires
typing the exact job ID back as confirmation (`-y` bypasses, the uniform
CLI-wide contract).

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--id` | string | | **yes** | Job ID to remove |

### `pve cluster backup-job not-covered`

Show guests covered by no backup job (reads
`/cluster/backup-info/not-backed-up`; results are filtered server-side by
per-guest `VM.Audit`). No flags.

---

## Node Management

### `pve node list`

List all cluster nodes. No flags.

### `pve node status`

Show node status and resource usage.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve node services`

List services on a node.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve node service start`

Start a service.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--service` | string | | **yes** | Service name |

### `pve node service stop`

Stop a service.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--service` | string | | **yes** | Service name |

### `pve node service restart`

Restart a service.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--service` | string | | **yes** | Service name |

### `pve node service status`

Get service status.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--service` | string | | **yes** | Service name |

### `pve node reboot`

Reboot a node.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve node shutdown`

Shut down a node.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve node version`

Get node PVE version.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

---

## Node Resources

### `pve node resource stats`

Show cluster resource summary. No flags.

### `pve node resource list`

List all cluster resources with filters.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | | Filter by type: `node`, `qemu`, `lxc`, `storage` |
| `--node` | string | | Filter by node |
| `--status` | string | | Filter by status |

### `pve node resource nodes`

List node resources.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Filter by specific node |
| `--status` | string | | Filter by status: `online`, `offline` |

### `pve node resource vms`

List VM resources.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--type` | string | | Filter by type: `qemu`, `lxc` |
| `--node` | string | | Filter by node |
| `--status` | string | | Filter by status: `running`, `stopped` |

### `pve node resource storages`

List storage resources.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Filter by node |
| `--status` | string | | Filter by status |

### `pve node resource node`

Show detailed resource usage for a node.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve node resource vm`

Show detailed resource usage for a VM.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | VM ID |
| `--type` | string | `qemu` | | VM type: `qemu`, `lxc` |

### `pve node resource history`

Show resource usage history (RRD data).

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | | VM ID (for VM-specific history) |
| `--type` | string | `qemu` | | VM type: `qemu`, `lxc` |
| `--timeframe` | string | `hour` | | Time range: `hour`, `day`, `week`, `month`, `year` |

### `pve node resource capacity`

Show per-node and cluster CPU/RAM overcommit plus an N-1 failover verdict
(whether the cluster can hold all allocated RAM if the largest node is lost,
while retaining quorum). No flags.

> **Note:** RAM figures are **allocated** (VM `maxmem`), not actual in-guest
> used memory. Actual-used analysis requires an external metrics server
> (Grafana/Prometheus) and is out of scope for this CLI.

## Node Disks (ZFS / SMART)

The `pve node disks` group reports ZFS pool usage, physical disks, and SMART
health via the Proxmox API (`/nodes/{node}/disks/*`). These endpoints require
**Sys.Audit on `/nodes`**. If the token lacks it, the command surfaces a clear
permission message rather than a raw 403. Grant access with:

```
pveum acl modify /nodes --tokens '<user>@<realm>!<token>' --roles PVEAuditor
```

### `pve node disks zfs`

List ZFS pool usage (size, alloc, free, fragmentation, health) across the
cluster.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Filter to a specific node (optional) |

### `pve node disks zfs-detail`

Show the device tree (vdev hierarchy + read/write/cksum error counts) for a
single ZFS pool.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--pool` | string | | **yes** | Pool name |

### `pve node disks list`

List physical disks on a node with size, model, wearout, health, and usage.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve node disks smart`

Show SMART overall health and attributes for a specific disk device.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--disk` | string | | **yes** | Disk device path, e.g. `/dev/sda` |

### `pve node capability`

Probe the report endpoints used by the capacity, ZFS/disk, SMART, and health
commands with the current token and print a ✅/❌ matrix. Read-only. Use this to
learn upfront whether the token has `Sys.Audit` (needed for ZFS/disk/SMART).

## Node APT Repositories

The `pve node apt repos` group manages APT repositories on a node via
`/nodes/{node}/apt/repositories`.

- **`repos list`** needs `Sys.Audit` on `/nodes`.
- **`repos add` / `enable` / `disable`** need `Sys.Modify` on `/nodes/{node}` —
  note `PVEAdmin` does **not** include `Sys.Modify`; use `PVESysAdmin` or grant
  it directly:
  ```
  pveum role add SysModifier -privs Sys.Modify
  pveum acl modify /nodes/<node> --users '<user>@<realm>' --roles SysModifier
  ```

API shape worth knowing:

- `add` only accepts **standard handles** (listed by `repos list`); arbitrary
  repo URLs cannot be added through the API.
- Existing entries can be enabled/disabled but **not deleted** — removing a
  line means editing the file on the node.
- `add` on a handle that is configured but disabled **enables** it (this is
  also how the web UI enables standard repos).
- Handle resolution matches components/URIs, so repositories served through a
  mirror (which the server's own status may call "not configured") still
  resolve for enable/disable. Ambiguous handles are refused with both
  candidates listed — use `--path`/`--index` (shown by `repos list`) instead.

### `pve node apt repos list`

List every configured repository entry with its file and index (what
`--path`/`--index` refer to), the standard handles `add` accepts, and any
warnings the node reports (e.g. an outdated suite).

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |

### `pve node apt repos add`

Add a standard repository by handle (`no-subscription`, `enterprise`, `test`,
`ceph-tentacle-no-subscription`, …). Enables it instead if already configured
but disabled. Skips with a note when already enabled.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--handle` | string | | **yes** | Standard repository handle |
| `--digest` | string | fetched | no | Digest from a previous `repos list` |

### `pve node apt repos enable` / `pve node apt repos disable`

Enable or disable a repository entry. Address it by standard handle, or
directly by `--path` + `--index` (as shown by `repos list`). Already-in-state
targets are a no-op — nothing is sent.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--handle` | string | | one of | Standard repository handle |
| `--path` | string | | one of | Repository file path |
| `--index` | int | -1 | with `--path` | Index within the file |
| `--digest` | string | fetched | no | Digest from a previous `repos list` |

---

## ZFS Management

### `pve zfs`

Manage ZFS through the Proxmox REST API. What the API covers, and what it does
not, is worth stating plainly — the gap is in Proxmox, not in this CLI:

- **Covered here:** pool create/destroy, pool listing and status, zfspool
  storage CRUD, replication jobs (ZFS send/recv under the hood).
- **NOT in the REST API at all:** `zpool scrub` (trigger — PVE runs a monthly
  cron instead), import/export, vdev add/attach/detach/replace/offline/online,
  trim, upgrade, dataset create/destroy, `zfs get/set` properties, dataset
  snapshots, raw send/recv, ARC tuning. These need `zpool`/`zfs` on the node
  itself. Scrub **status** is available, via `zfs pool status`.

Permissions differ per operation and the distinctions bite:

| Operation | Needs |
|-----------|-------|
| Pool list / status | `Sys.Audit` on `/` |
| Pool create / destroy | `Sys.Modify` on `/` — **`PVEAdmin` does NOT include it**; use `PVESysAdmin` |
| Storage CRUD | `Datastore.Allocate` on `/storage` (not the same as `Datastore.AllocateSpace`) |
| Replication read / write | `VM.Audit` / `VM.Replicate` on `/vms/<vmid>` |

### `pve zfs pool list`

List ZFS pools across the cluster. `--node` restricts to one node.

### `pve zfs pool status`

Show one pool's vdev tree, health, errors and last scrub result — the closest
the API gets to scrub management.

| Flag | Type | Description |
|------|------|-------------|
| `--node` | string | Node the pool is on (required) |
| `--pool` | string | Pool name (required) |

### `pve zfs pool create`

Create a ZFS pool on a node. **THE NAMED DEVICES ARE WIPED.**

Raid levels and their minimum device counts:

| Level | Min devices | Notes |
|-------|-------------|-------|
| `single` | 1 | one disk, no redundancy |
| `mirror` | 2 | n-way mirror |
| `raid10` | 4 | striped mirrors — needs an EVEN device count |
| `raidz` | 3 | single parity |
| `raidz2` | 4 | double parity |
| `raidz3` | 5 | triple parity |
| `draid` | 3 | distributed-spare raidz, single parity |
| `draid2` | 4 | distributed-spare, double parity |
| `draid3` | 5 | distributed-spare, triple parity |

`ashift` cannot be changed after creation; the default of 12 (4 KiB sectors) is
correct for effectively all modern disks.

```bash
pve zfs pool create --node pve1 --name tank --raid-level mirror \
  --devices /dev/sdb,/dev/sdc

# Register it as a PVE storage at the same time
pve zfs pool create --node pve1 --name tank --raid-level raidz2 \
  --devices /dev/sdb,/dev/sdc,/dev/sdd,/dev/sde --add-storage

# dRAID with two distributed spares
pve zfs pool create --node pve1 --name tank --raid-level draid2 \
  --devices /dev/sd{b,c,d,e,f,g} --draid-data 4 --draid-spares 2
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Node to create the pool on (required) |
| `--name` | string | | Pool name (required) |
| `--raid-level` | string | | `single`\|`mirror`\|`raid10`\|`raidz`\|`raidz2`\|`raidz3`\|`draid`\|`draid2`\|`draid3` (required) |
| `--devices` | strings | | Block devices to use (required, comma-separated) |
| `--ashift` | int | `12` | Sector size exponent 9–16 |
| `--compression` | string | `on` | `on`\|`off`\|`gzip`\|`lz4`\|`lzjb`\|`zle`\|`zstd` |
| `--add-storage` | bool | `false` | Also register the pool as a PVE storage |
| `--draid-data` | int | | Data devices per redundancy group (draid only) |
| `--draid-spares` | int | | Distributed spares (draid only) |

### `pve zfs pool destroy`

Destroy a ZFS pool and everything stored on it — every dataset, zvol and
snapshot, including VM disks backed by it. **Irreversible**; confirmation
requires typing the pool name back.

By default the member disks keep their ZFS labels, which prevents reusing them
in a new pool; `--cleanup-disks` wipes them. `--cleanup-config` removes the PVE
storage entries that referenced the pool.

| Flag | Type | Description |
|------|------|-------------|
| `--node` | string | Node the pool is on (required) |
| `--name` | string | Pool name (required) |
| `--cleanup-disks` | bool | Also wipe the member disks so they can be reused |
| `--cleanup-config` | bool | Also remove PVE storage entries referencing this pool |
| `--force` | bool | Required alongside `--yes` for non-interactive destroy |

### `pve zfs storage list`

List configured zfspool storages.

### `pve zfs storage create`

Add a zfspool storage entry backed by an existing ZFS pool or dataset. This
does **not** create the pool — use `zfs pool create` (or `--add-storage` there)
first.

```bash
pve zfs storage create --storage tank-vms --pool tank/vms --content images,rootdir
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--storage` | string | | Storage ID (required) |
| `--pool` | string | | Backing ZFS pool or dataset (required) |
| `--content` | string | | Content types, e.g. `images,rootdir` |
| `--nodes` | string | all | Restrict to these nodes |
| `--blocksize` | string | | `volblocksize` for new zvols, e.g. `16k` |
| `--sparse` | bool | `false` | Thin-provision new zvols |
| `--mountpoint` | string | | Override the dataset mountpoint |
| `--bwlimit` | string | | Bandwidth limit for migration/restore |
| `--disable` | bool | `false` | Mark the storage inactive |

### `pve zfs storage update`

Change a zfspool storage's settings. **The backing pool cannot be changed** —
Proxmox declares it fixed, so it is never sent. Recreate the storage entry if
the pool must change. Same flags as `create` (without `--pool`), plus
`--delete strings` to reset settings to their default.

### `pve zfs storage delete`

Unregister a storage from the cluster configuration. This removes only the PVE
storage entry — the ZFS pool, its datasets and all data survive; use
`zfs pool destroy` to remove those.

### `pve zfs replication`

Manage Proxmox storage replication jobs (alias `repl`). Replication
periodically zfs-sends a guest's disks to another node so a failover target
holds a recent copy. It works only on zfspool storage — a Proxmox limitation.

Job IDs have the form `<vmid>-<number>`, e.g. `100-0`; the number lets one
guest replicate to several targets. The VMID part must be 3–9 digits (the
API's own pattern).

| Subcommand | Description |
|------------|-------------|
| `list` | List configured replication jobs |
| `status` | Per-job last sync, next sync, duration and failure count on a node (`--node` required). `fail_count` is cumulative, not current |
| `create` | Create a job (`--id`, `--target`, `--schedule` default `*/15`, `--rate`, `--comment`, `--disable`) |
| `update` | Change a job (same flags as `create`, plus `--delete`) |
| `run` | Trigger a job now, outside its schedule (`--node`, `--id`) |
| `delete` | Remove a job — the replicated data on the target is removed too unless `--keep`; `--force` needed when the target is unreachable |

---

## Container Management

### `pve container list`

List LXC containers.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--node` | string | | Filter by node |
| `--status` | string | | Filter by status: `running`, `stopped` |
| `--name` | string | | Filter by name (substring match) |
| `--template` | bool | `false` | Show only templates |

### `pve container summary`

Show container summary statistics. No flags.

### `pve container show`

Show container configuration details.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |

### `pve container status`

Show container runtime status.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |

### `pve container create`

Create a new LXC container.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |
| `--ostemplate` | string | | **yes** | OS template volume ID |
| `--storage` | string | | **yes** | Root disk storage |
| `--hostname` | string | | | Container hostname |
| `--description` | string | | | Description |
| `--password` | string | | | Root password |
| `--ssh-keys` | string | | | SSH public keys |
| `--cpus` | int | | | Number of CPU cores |
| `--memory` | int64 | | | Memory in MB |
| `--swap` | int64 | | | Swap in MB |
| `--nesting` | bool | `false` | | Enable nesting |
| `--unprivileged` | bool | `false` | | Unprivileged container |
| `--onboot` | bool | `false` | | Start on boot |
| `--protected` | bool | `false` | | Protection flag |

### `pve container start`

Start a container.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |

### `pve container stop`

Force-stop a container.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |

### `pve container shutdown`

Gracefully shut down a container.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |
| `--timeout` | int | `60` | | Shutdown timeout in seconds |

### `pve container restart`

Restart a container.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |

### `pve container delete`

Delete a container.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |
| `--purge` | bool | `false` | | Purge container data |

### `pve container clone`

Clone a container.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Source node |
| `--vmid` | int | | **yes** | Source container ID |
| `--newid` | int | | **yes** | New container ID |
| `--hostname` | string | | | New hostname |
| `--description` | string | | | Description |
| `--storage` | string | | | Target storage |
| `--target` | string | | | Target node |
| `--snapshot` | string | | | Snapshot to clone from |
| `--full` | bool | `false` | | Full clone (default: linked) |

---

## Container Snapshots

### `pve container snapshot create`

Create a container snapshot.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |
| `--name` | string | | **yes** | Snapshot name |
| `--description` | string | | | Description |

### `pve container snapshot list`

List container snapshots.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |

### `pve container snapshot rollback`

Rollback container to a snapshot.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |
| `--name` | string | | **yes** | Snapshot name |

### `pve container snapshot delete`

Delete a container snapshot.

| Flag | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `--node` | string | | **yes** | Node name |
| `--vmid` | int | | **yes** | Container ID |
| `--name` | string | | **yes** | Snapshot name |

---

## VM Selection Patterns

VM commands that accept `--vmid` support these selection patterns:

| Pattern | Example | Description |
|---------|---------|-------------|
| Single ID | `4103` | One VM |
| Comma-separated | `4101,4102,4103` | Multiple specific VMs |
| Range | `7201-7205` | All VMs in range |
| Wildcard | `72*` | Pattern matching |
| Interactive | `i` | Checkbox-style selection UI |

The `--vmname` flag accepts comma-separated VM names as an alternative to IDs.

---

## Backup File Format

Backup volume IDs use the format `<STORAGE_ID>:<CONTENT_TYPE>/<PATH>`.

Examples:
- File-based: `local:backup/vzdump-qemu-4103-2025_08_06.vma.zst`
- PBS: `backup-pbs:backup/vm/4103/2025-08-05T12:16:44Z`

---

## Snapshot Naming

- Maximum snapshot **name** length: **40 characters**, applied to the full assembled name `<prefix>-<vmname>-<YYYYMMDD-HHMM>` (there is no separate limit on the `--prefix` input)
- Invalid characters are automatically cleaned (replaced with `-`, runs collapsed, leading/trailing trimmed)
- When using `--prefix`, a timestamp is appended automatically; for long VM names the `-YYYYMMDD-HHMM` suffix may be **truncated** because the 40-char cap applies to the whole name (e.g. `bulkdemo-docs-vm-03-20260626` — the `-HHMM` is cut)
- The keyword `vmstate` in prefix or name enables RAM inclusion

---

## Access Management

The `pve access` group manages API users, tokens, and role assignments via the
`/access/*` REST endpoints — the same operations the `pveum` CLI performs, but
without SSH. It is the in-binary successor to `scripts/pve-token.sh`.

### Required privileges

Mutations (`create`, `add-token`, `revoke-token`, `remove`) hit endpoints that
require **`User.Modify`** on `/access` and **`Permissions.Modify`** on the ACL
target path. A VM-scoped API token cannot perform these — authenticate as
`root@pve` (set `PVE_PASSWORD`) or a `PVEUserAdmin` service account:

```bash
export PVE_HOST=pve1
export PVE_USER=root@pve
export PVE_PASSWORD=<root password>
```

### `pve access create`

Create a user (if missing) and an API token, then assign a role at `/`.
The token value is printed once with ready-to-use shell exports.

```bash
pve access create --user automation --token prod
pve access create --user snapshot@pam --token t1 --role PVEAuditor
```

`--user` accepts either a bare name (uses `--realm`, default `pve`) or a full
`user@realm`. An explicit `--realm` overrides any realm embedded in `--user`.

### `pve access add-token`

Add a token to an **existing** user (the user is not created). Errors clearly
if the user does not exist.

```bash
pve access add-token --user automation --token ci
```

### `pve access revoke-token`

Revoke a single token; the user and any other tokens are preserved.

```bash
pve access revoke-token --user automation --token ci
```

### `pve access remove`

Delete a user and all its tokens.

```bash
pve access remove --user automation
```

### `pve access list`

List a user's API tokens (read-only; requires `User.Modify` or equivalent read
on the user).

```bash
pve access list --user automation
```

### `pve access grant`

Assign a role on an ACL path without recreating the user or token — the REST
equivalent of `pveum acl modify <path> --users <u> --roles <r>`. Previously the only
way to change a token's role was to revoke and reissue it, which changes the
secret.

The grant targets the **user** by default. That is deliberate: a token created
with privilege separation disabled (privsep 0, which `access create` uses)
inherits the user's privileges and its own token-level ACL is ignored — a
token-only grant silently has no effect. `--to-token` overrides, and the
command warns when that combination would be a no-op.

```bash
# Enable ZFS/cluster-health/HA reporting (these check '/')
pve access grant --user snapshot --path / --grant-role PVEAuditor

# Enable node status / disks / SMART (these check '/nodes')
pve access grant --user snapshot --path /nodes --grant-role PVEAuditor
```

### `pve access acl-list`

Show the cluster ACL (`GET /access/acl`). With `--user`, only entries for that
user and its tokens are shown. Read-only.

```bash
pve access acl-list
pve access acl-list --user snapshot
```

### Shared flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--user` | — | Username or `user@realm` (required) |
| `--token` | — | Token name (required for create/add-token/revoke-token) |
| `--realm` | `pve` | Auth realm (`pam`, `pve`, `ldap`) |
| `--role` | `PVEVMAdmin` | Role to assign at `/` |
| `--nodes-audit` | off | Also grant `PVEAuditor` on `/nodes` (adds `Sys.Audit`, needed by `pve node disks *`) |
| `--grant-role` | — | Role for `grant` (required there; deliberately separate from `--role` so a permission mutation cannot inherit create's `PVEVMAdmin` default) |
| `--path` | `/` | ACL path for `grant` (e.g. `/` or `/nodes`) |
| `--to-token` | off | `grant` to the token instead of the user |
| `--dry-run` | off | Print the steps without executing |
