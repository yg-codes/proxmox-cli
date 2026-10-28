package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// vmCreateCmd creates a new VM.
var vmCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new VM",
	Long: `Create a new virtual machine.

The API accepts roughly a hundred parameters. This exposes the practical subset
as flags and provides --raw for anything else, so an unusual option does not
require a new release.

Disks and network devices are passed through as Proxmox device specs, because
that syntax is the API's and translating it behind typed flags would only add a
layer to get wrong:

  --disk scsi0=local-zfs:32        a new 32 GiB disk on local-zfs
  --net  net0=virtio,bridge=vmbr0  a virtio NIC on vmbr0
  --cdrom ide2=local:iso/x.iso     an ISO to boot from

Examples:
  # Minimal: allocate the next free VMID on a node
  pve vm create --node pve1 --name web01 --cores 2 --memory 4096 \
    --disk scsi0=local-zfs:32 --net net0=virtio,bridge=vmbr0

  # Explicit VMID, boot from an ISO, power on when done
  pve vm create --node pve1 --vmid 200 --name web01 --cores 2 --memory 4096 \
    --disk scsi0=local-zfs:32 --net net0=virtio,bridge=vmbr0 \
    --cdrom ide2=local:iso/rocky10.iso --boot 'order=ide2;scsi0' --start

  # Anything not covered by a flag
  pve vm create --node pve1 --name web01 --raw numa=1 --raw hotplug=disk,network`,
	RunE: runVMCreateCommand,
}

// vmDestroyCmd destroys a VM. Deliberately narrow — see the guards in
// runDestroyCommand.
var vmDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a VM and its disks (irreversible)",
	Long: `Destroy a virtual machine and all of its disks.

THIS CANNOT BE UNDONE. The disks go with the VM, and no snapshot of the VM
survives it. There is no Proxmox-side recycle bin.

Because of that, this command is deliberately narrower than every other VM
command:

  * --vmid takes exactly ONE id. No ranges, no comma-separated lists, no
    wildcards, and none of the running/stopped/all keywords. There is no bulk
    variant and it is not in the interactive menu.
  * The VM must be stopped.
  * A VM with protection enabled is refused. Clear it deliberately first
    (pve vm config set --vmid N --set protection=0) — inherited protection on a
    clone is a real signal, not an obstacle.
  * Confirmation requires typing the VMID back, not y/N. --yes alone will not
    destroy a VM; --force is additionally required, so a -y left over from
    another command cannot delete anything.

Destroying a VM also purges the references to it: backup jobs, HA resources and
replication entries naming this VMID are removed as part of the operation. That
is the default because a stale entry points at a VM that no longer exists, and a
future VM reusing the id would silently inherit it. Pass --no-purge to keep them.

Examples:
  # Preview only
  pve vm destroy --vmid 999 --dry-run

  # Interactive: prompts for the VMID to be typed back
  pve vm destroy --vmid 999

  # Non-interactive (scripts): both flags required
  pve vm destroy --vmid 999 --yes --force

  # Leave backup-job/HA/replication references in place
  pve vm destroy --vmid 999 --no-purge --yes --force`,
	RunE: runDestroyCommand,
}

func initVMCreateFlags() {
	vmCreateCmd.Flags().String("node", "", "Node to create the VM on (required)")
	vmCreateCmd.Flags().String("vmid", "", "VM ID (default: next free ID from the cluster)")
	vmCreateCmd.Flags().String("name", "", "VM name")
	vmCreateCmd.Flags().String("description", "", "VM description")
	vmCreateCmd.Flags().Int("cores", 0, "CPU cores per socket")
	vmCreateCmd.Flags().Int("sockets", 0, "CPU sockets")
	vmCreateCmd.Flags().Int("memory", 0, "Memory in MiB")
	vmCreateCmd.Flags().String("ostype", "", "Guest OS type, e.g. l26, win11")
	vmCreateCmd.Flags().String("machine", "", "Machine type, e.g. q35")
	vmCreateCmd.Flags().String("bios", "", "Firmware: seabios or ovmf")
	vmCreateCmd.Flags().String("cpu", "", "CPU model, e.g. host")
	vmCreateCmd.Flags().String("scsihw", "", "SCSI controller, e.g. virtio-scsi-single")
	vmCreateCmd.Flags().String("boot", "", "Boot order, e.g. 'order=scsi0;net0'")
	vmCreateCmd.Flags().String("agent", "", "QEMU guest agent: 1 to enable")
	vmCreateCmd.Flags().String("pool", "", "Add the VM to this resource pool")
	vmCreateCmd.Flags().Bool("start", false, "Start the VM once created")

	// StringArray, not StringSlice: a device spec contains commas
	// (net0=virtio,bridge=vmbr0) and a slice flag would split it into broken
	// fragments. This is the same defect that shipped on config --set.
	vmCreateCmd.Flags().StringArray("disk", []string{}, "Disk as dev=spec, e.g. scsi0=local-zfs:32 (repeatable)")
	vmCreateCmd.Flags().StringArray("net", []string{}, "NIC as dev=spec, e.g. net0=virtio,bridge=vmbr0 (repeatable)")
	vmCreateCmd.Flags().StringArray("cdrom", []string{}, "CD/DVD as dev=spec, e.g. ide2=local:iso/x.iso (repeatable)")
	vmCreateCmd.Flags().StringArray("raw", []string{}, "Any other API parameter as key=value (repeatable)")

	vmDestroyCmd.Flags().String("vmid", "", "The single VM ID to destroy (or use --vmname)")
	vmDestroyCmd.Flags().String("vmname", "", "VM name, matched EXACTLY (no partial matching for destroy)")
	// Purge defaults ON. Without it, backup jobs, HA resources and replication
	// entries keep referring to a VMID that no longer exists, and the next VM
	// to reuse that id silently inherits them. Destroying a VM should not leave
	// its references behind, so opting OUT is the explicit choice.
	vmDestroyCmd.Flags().Bool("no-purge", false,
		"Keep backup-job, HA and replication entries that reference this VMID (they are removed by default)")
	vmDestroyCmd.Flags().Bool("destroy-unreferenced-disks", false,
		"Also remove disks on the VM's storages that belong to this VMID but are not in its config")
	vmDestroyCmd.Flags().Bool("force", false,
		"Required alongside --yes for non-interactive destroy; --yes alone is not sufficient")
}

func runVMCreateCommand(cmd *cobra.Command, args []string) error {
	node, _ := cmd.Flags().GetString("node")
	if node == "" {
		return fmt.Errorf("--node must be specified")
	}

	opts := &vm.CreateOptions{Node: node, Devices: map[string]string{}, Extra: map[string]string{}}
	opts.VMID, _ = cmd.Flags().GetString("vmid")
	opts.Name, _ = cmd.Flags().GetString("name")
	opts.Description, _ = cmd.Flags().GetString("description")
	opts.Cores, _ = cmd.Flags().GetInt("cores")
	opts.Sockets, _ = cmd.Flags().GetInt("sockets")
	opts.Memory, _ = cmd.Flags().GetInt("memory")
	opts.OSType, _ = cmd.Flags().GetString("ostype")
	opts.Machine, _ = cmd.Flags().GetString("machine")
	opts.BIOS, _ = cmd.Flags().GetString("bios")
	opts.CPU, _ = cmd.Flags().GetString("cpu")
	opts.SCSIHW, _ = cmd.Flags().GetString("scsihw")
	opts.Boot, _ = cmd.Flags().GetString("boot")
	opts.Agent, _ = cmd.Flags().GetString("agent")
	opts.Pool, _ = cmd.Flags().GetString("pool")
	opts.Start, _ = cmd.Flags().GetBool("start")

	// Device flags all share the dev=spec shape.
	for _, flag := range []string{"disk", "net", "cdrom"} {
		values, _ := cmd.Flags().GetStringArray(flag)
		parsed, err := parseKeyValuePairs(values)
		if err != nil {
			return fmt.Errorf("--%s: %w", flag, err)
		}
		for k, v := range parsed {
			opts.Devices[k] = v
		}
	}

	raw, _ := cmd.Flags().GetStringArray("raw")
	extra, err := parseKeyValuePairs(raw)
	if err != nil {
		return fmt.Errorf("--raw: %w", err)
	}
	opts.Extra = extra

	// Resolve a VMID before showing the plan, so the operator sees the actual
	// id rather than "next free" and can spot a collision.
	if opts.VMID == "" {
		next, err := vmOps.NextVMID()
		if err != nil {
			return fmt.Errorf("no --vmid given and the next free ID could not be determined: %w", err)
		}
		opts.VMID = next
		fmt.Printf("Using next free VMID from the cluster: %s\n\n", opts.VMID)
	} else {
		exists, err := vmOps.VMIDExists(opts.VMID)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("VMID %s is already in use", opts.VMID)
		}
	}

	fmt.Printf("Create VM %s on node %s:\n\n", opts.VMID, node)
	if opts.Name != "" {
		fmt.Printf("  name:     %s\n", opts.Name)
	}
	if opts.Cores > 0 || opts.Sockets > 0 {
		fmt.Printf("  cpu:      %d core(s), %d socket(s)\n", opts.Cores, opts.Sockets)
	}
	if opts.Memory > 0 {
		fmt.Printf("  memory:   %d MiB\n", opts.Memory)
	}
	for _, k := range sortedKeys(opts.Devices) {
		fmt.Printf("  %-9s %s\n", k+":", opts.Devices[k])
	}
	for _, k := range sortedKeys(opts.Extra) {
		fmt.Printf("  %-9s %s\n", k+":", opts.Extra[k])
	}
	if opts.Start {
		fmt.Printf("  start:    yes, once created\n")
	}
	fmt.Println()

	if len(opts.Devices) == 0 {
		fmt.Println("  ⚠️  no disk or network device specified — the VM will have neither")
		fmt.Println()
	}

	if dryRun {
		printDryRunHeader()
		printDryRunAction("create", opts.VMID, opts.Name, "on node "+node)
		printDryRunSummary("create", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Create VM %s on %s?", opts.VMID, node)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	upid, err := vmOps.CreateVM(opts)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Create of VM %s started (task %s)\n", opts.VMID, upid)
	return nil
}

func runDestroyCommand(cmd *cobra.Command, args []string) error {
	rawVMID, _ := cmd.Flags().GetString("vmid")

	// Reject every multi-VM selector this CLI supports elsewhere, BEFORE any
	// name resolution. Destroy is the one command where a selector expanding to
	// more than intended is unrecoverable, so it accepts a single literal id.
	if rawVMID != "" {
		if err := rejectMultiVMSelector(rawVMID); err != nil {
			return err
		}
	}

	// --vmname resolves through resolveSingleVMID, which requires an EXACT
	// name match and refuses an ambiguous one rather than picking a candidate.
	vmid, err := vmTargetFromFlags(cmd)
	if err != nil {
		return err
	}

	exists, err := vmOps.VMIDExists(vmid)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("VM %s does not exist", vmid)
	}

	status, err := vmOps.GetVMStatus(vmid)
	if err != nil {
		return err
	}
	if status.Running || status.Paused {
		return fmt.Errorf("VM %s must be stopped before it can be destroyed (currently %s)",
			vmid, status.Status)
	}

	// Protection is checked before confirming, so a protected VM never gets as
	// far as asking the operator to type its id.
	protected, err := vmOps.IsProtected(vmid)
	if err != nil {
		return err
	}
	if protected {
		return fmt.Errorf("VM %s has protection enabled and will not be destroyed\n"+
			"  Clear it deliberately first if that is really what you want:\n"+
			"    pve vm config set --vmid %s --set protection=0", vmid, vmid)
	}

	noPurge, _ := cmd.Flags().GetBool("no-purge")
	opts := &vm.DestroyOptions{Purge: !noPurge}
	opts.DestroyUnreferencedDisks, _ = cmd.Flags().GetBool("destroy-unreferenced-disks")

	config, err := vmOps.GetVMConfig(vmid)
	if err != nil {
		return err
	}

	fmt.Printf("Destroy VM %s (%s) on node %s\n\n", vmid, status.Name, status.Node)
	fmt.Println("  This is IRREVERSIBLE. The following disks are destroyed with it:")
	disks := diskEntries(config)
	if len(disks) == 0 {
		fmt.Println("    (none found in the config)")
	}
	for _, d := range disks {
		fmt.Printf("    %-10s %s\n", d.device+":", d.spec)
	}
	fmt.Println()
	if opts.Purge {
		fmt.Println("  purge: backup-job, HA and replication entries for this VMID are also removed")
	} else {
		fmt.Println("  ⚠️  --no-purge: backup-job/HA/replication entries for this VMID are KEPT.")
		fmt.Println("      They will refer to a VM that no longer exists, and a future VM")
		fmt.Println("      reusing this VMID would inherit them.")
	}
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("destroy", vmid, status.Name, fmt.Sprintf("%d disk(s) on node %s", len(disks), status.Node))
		printDryRunSummary("destroy", 1)
		return nil
	}

	// A declined confirmation and a missing --force are different outcomes. An
	// operator answering "no" is a successful no-op (rc=0); a script that asked
	// for a destroy without --force made a usage error and must see a non-zero
	// exit, or it will read "refused" as "done".
	switch confirmDestroy(vmid, cmd) {
	case destroyConfirmed:
	case destroyDeclined:
		fmt.Println("Operation cancelled")
		return nil
	case destroyNeedsForce:
		return fmt.Errorf("refusing to destroy VM %s non-interactively without --force\n"+
			"  pve vm destroy --vmid %s --yes --force", vmid, vmid)
	}

	upid, err := vmOps.DestroyVM(vmid, opts)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Destroy of VM %s started (task %s)\n", vmid, upid)
	return nil
}

// rejectMultiVMSelector refuses any --vmid value that is not a single literal
// VMID. Every other VM command accepts ranges, lists, globs and keywords; for
// destroy each of those is a way to delete more than was meant.
func rejectMultiVMSelector(vmid string) error {
	switch {
	case strings.ContainsAny(vmid, ",-*?"):
		return fmt.Errorf("destroy accepts exactly one VMID: %q looks like a range, list or pattern.\n"+
			"  Destroy is irreversible, so it has no bulk form — run it once per VM", vmid)
	case vmid == "all" || vmid == "running" || vmid == "stopped" || vmid == "i":
		return fmt.Errorf("destroy does not accept the %q selector — it takes one explicit VMID", vmid)
	}
	for _, r := range vmid {
		if r < '0' || r > '9' {
			return fmt.Errorf("VMID %q is not numeric", vmid)
		}
	}
	return nil
}

// destroyDecision distinguishes the three outcomes of asking to destroy, which
// must not share an exit code: a declined prompt is a successful no-op, while a
// missing --force is a usage error a script has to be able to detect.
type destroyDecision int

const (
	destroyConfirmed destroyDecision = iota
	destroyDeclined
	destroyNeedsForce
)

// confirmDestroy requires more than a y/N. Interactively the operator types the
// VMID back; non-interactively both --yes and --force are required, so a --yes
// carried over from another command cannot destroy anything on its own.
func confirmDestroy(vmid string, cmd *cobra.Command) destroyDecision {
	force, _ := cmd.Flags().GetBool("force")

	if cfg.IsAutoConfirm() || batchMode {
		if !force {
			return destroyNeedsForce
		}
		logger.Warnf("Destroying VM %s non-interactively (--yes --force)", vmid)
		return destroyConfirmed
	}

	fmt.Printf("Type the VMID %s to confirm destruction (anything else aborts): ", vmid)
	var typed string
	_, _ = fmt.Scanln(&typed)
	if strings.TrimSpace(typed) != vmid {
		fmt.Printf("Got %q, expected %q — aborting.\n", strings.TrimSpace(typed), vmid)
		return destroyDeclined
	}
	return destroyConfirmed
}

// sortedKeys returns a map's keys in a stable order, so the plan a user reads
// matches the plan they read last time.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedConfigKeys is sortedKeys for an API config map.
func sortedConfigKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// diskEntry is one disk device from a VM config.
type diskEntry struct {
	device string
	spec   string
}

// diskEntries lists the disk devices in a config, so destroy can show exactly
// what is about to be deleted. Non-disk devices (net, serial) and cdrom/none
// entries are excluded — a cdrom is not destroyed with the VM.
func diskEntries(config map[string]interface{}) []diskEntry {
	prefixes := []string{"scsi", "virtio", "sata", "ide", "efidisk", "tpmstate", "unused"}
	var out []diskEntry
	for _, key := range sortedConfigKeys(config) {
		matched := false
		for _, p := range prefixes {
			if strings.HasPrefix(key, p) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		spec := formatConfigValue(config[key])
		// media=cdrom entries reference an ISO, not a VM-owned volume.
		if strings.Contains(spec, "media=cdrom") || strings.HasPrefix(spec, "none") {
			continue
		}
		out = append(out, diskEntry{device: key, spec: spec})
	}
	return out
}
