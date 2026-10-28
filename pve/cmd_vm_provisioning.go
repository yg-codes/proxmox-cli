package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/storage"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// vmCloneCmd clones a VM or template
var vmCloneCmd = &cobra.Command{
	Use:   "clone",
	Short: "Clone a VM or template",
	Long: `Create a copy of a virtual machine or template.

A normal VM is always fully cloned. A template is linked-cloned by default,
which is near-instant and shares disk data with the template — pass --full to
copy the disks instead. A linked clone depends on its template, so the template
cannot be deleted while linked clones exist.

Cloning to another node with --target is only allowed if the VM is on shared
storage.

Examples:
  # Clone VM 4103 to a new VM 7999
  pve vm clone --vmid 4103 --newid 7999 --name web02

  # Full clone onto a specific storage
  pve vm clone --vmid 9000 --newid 7999 --full --storage local-zfs

  # Clone from a snapshot
  pve vm clone --vmid 4103 --newid 7999 --snapname pre-upgrade`,
	RunE: runCloneCommand,
}

// vmMigrateCmd migrates a VM to another node
var vmMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate a VM to another node",
	Long: `Migrate a virtual machine to another cluster node.

Migration preconditions are checked and displayed first: which nodes the VM may
move to, any disks on local storage, and any passed-through devices that would
block the move. A migration that fails partway is far more disruptive than one
that is never started.

A running VM requires --online for a live migration. Disks on local storage
require --with-local-disks, which copies them to the target node and takes
considerably longer than a shared-storage migration.

Examples:
  # Show preconditions without migrating
  pve vm migrate --vmid 4103 --target pve-node5 --dry-run

  # Live-migrate a running VM on shared storage
  pve vm migrate --vmid 4103 --target pve-node5 --online

  # Migrate with local disks
  pve vm migrate --vmid 4103 --target pve-node5 --online --with-local-disks`,
	RunE: runMigrateCommand,
}

// vmResizeCmd extends a VM disk
var vmResizeCmd = &cobra.Command{
	Use:   "resize",
	Short: "Extend a VM disk",
	Long: `Extend a virtual machine disk.

The size is either an absolute target ("32G") or an increment ("+4G").

Disks can only be extended. Proxmox has no shrink operation, so a size smaller
than the current one is rejected. Growing the disk also does not grow the
filesystem inside the guest — that remains a separate step in the guest OS.

Examples:
  # Grow scsi0 by 4 GiB
  pve vm resize --vmid 4103 --disk scsi0 --size +4G

  # Set scsi0 to 32 GiB
  pve vm resize --vmid 4103 --disk scsi0 --size 32G`,
	RunE: runResizeCommand,
}

// vmMoveDiskCmd moves a VM disk
var vmMoveDiskCmd = &cobra.Command{
	Use:   "move-disk",
	Short: "Move a VM disk to another storage or VM",
	Long: `Move a virtual machine disk to a different storage, or reassign it
to another VM.

Without --delete the original disk is kept as an unused volume, so the move is
reversible and the space is not reclaimed until you remove it. With --delete
the source is removed once the copy succeeds.

Examples:
  # Move scsi0 to another storage, keeping the original as unused
  pve vm move-disk --vmid 4103 --disk scsi0 --storage local-zfs

  # Move and delete the source
  pve vm move-disk --vmid 4103 --disk scsi0 --storage local-zfs --delete

  # Reassign a disk to another VM
  pve vm move-disk --vmid 4103 --disk scsi1 --target-vmid 7999`,
	RunE: runMoveDiskCommand,
}

// vmTemplateCmd converts a VM into a template
var vmTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "Convert a VM into a template",
	Long: `Convert a virtual machine into a template.

This is one-way. Proxmox provides no API to convert a template back into a
regular VM, and a template cannot be started — it exists to be cloned. The VM
must be stopped.

Examples:
  pve vm template --vmid 9000`,
	RunE: runTemplateCommand,
}

// initVMProvisioningFlags registers flags for the provisioning commands.
func initVMProvisioningFlags() {
	vmCloneCmd.Flags().String("vmid", "", "Source VM ID (or use --vmname)")
	vmCloneCmd.Flags().String("vmname", "", "Source VM name, matched exactly (alternative to --vmid)")
	vmCloneCmd.Flags().String("newid", "", "VM ID for the clone")
	vmCloneCmd.Flags().String("name", "", "Name for the new VM")
	vmCloneCmd.Flags().String("description", "", "Description for the new VM")
	vmCloneCmd.Flags().String("pool", "", "Add the new VM to this resource pool")
	vmCloneCmd.Flags().String("snapname", "", "Clone from this snapshot instead of the current state")
	vmCloneCmd.Flags().String("storage", "", "Target storage (full clone only)")
	vmCloneCmd.Flags().String("format", "", "Target disk format: raw, qcow2 or vmdk")
	vmCloneCmd.Flags().String("target", "", "Target node (requires shared storage)")
	vmCloneCmd.Flags().Bool("full", false, "Copy all disks instead of creating a linked clone")
	vmCloneCmd.Flags().Int("bwlimit", 0, "I/O bandwidth limit in KiB/s")

	vmMigrateCmd.Flags().String("vmid", "", "VM ID (or use --vmname)")
	vmMigrateCmd.Flags().String("vmname", "", "VM name, matched exactly (alternative to --vmid)")
	vmMigrateCmd.Flags().String("target", "", "Target node")
	vmMigrateCmd.Flags().Bool("online", false, "Live-migrate a running VM")
	vmMigrateCmd.Flags().Bool("with-local-disks", false, "Also migrate disks on local storage")
	vmMigrateCmd.Flags().String("target-storage", "", "Target storage mapping")
	vmMigrateCmd.Flags().String("migration-type", "", "Migration type: secure or insecure")
	vmMigrateCmd.Flags().String("migration-network", "", "CIDR of the network used for migration")
	vmMigrateCmd.Flags().Int("bwlimit", 0, "I/O bandwidth limit in KiB/s")

	vmResizeCmd.Flags().String("vmid", "", "VM ID (or use --vmname)")
	vmResizeCmd.Flags().String("vmname", "", "VM name, matched exactly (alternative to --vmid)")
	vmResizeCmd.Flags().String("disk", "", "Disk to resize, e.g. scsi0")
	vmResizeCmd.Flags().String("size", "", "New size (32G) or increment (+4G)")

	vmMoveDiskCmd.Flags().String("vmid", "", "VM ID (or use --vmname)")
	vmMoveDiskCmd.Flags().String("vmname", "", "VM name, matched exactly (alternative to --vmid)")
	vmMoveDiskCmd.Flags().String("disk", "", "Disk to move, e.g. scsi0")
	vmMoveDiskCmd.Flags().String("storage", "", "Target storage")
	vmMoveDiskCmd.Flags().String("target-vmid", "", "Reassign the disk to this VM")
	vmMoveDiskCmd.Flags().String("target-disk", "", "Drive name on the target VM")
	vmMoveDiskCmd.Flags().String("format", "", "Target disk format: raw, qcow2 or vmdk")
	vmMoveDiskCmd.Flags().Bool("delete", false, "Delete the source disk after a successful copy")
	vmMoveDiskCmd.Flags().Int("bwlimit", 0, "I/O bandwidth limit in KiB/s")

	vmTemplateCmd.Flags().String("vmid", "", "VM ID (or use --vmname)")
	vmTemplateCmd.Flags().String("vmname", "", "VM name, matched exactly (alternative to --vmid)")
	vmTemplateCmd.Flags().String("disk", "", "Only convert this disk")
}

func runCloneCommand(cmd *cobra.Command, args []string) error {
	vmid, err := vmTargetFromFlags(cmd)
	if err != nil {
		return err
	}
	newID, _ := cmd.Flags().GetString("newid")

	if newID == "" {
		return fmt.Errorf("--newid must be specified")
	}
	if vmid == newID {
		return fmt.Errorf("--newid must differ from --vmid")
	}
	if err := vm.ValidateVMID(newID); err != nil {
		return err
	}

	// A clone onto an existing VMID fails server-side; catching it here gives
	// a clearer message than the API's.
	if vmOps.VMExists(newID) {
		return fmt.Errorf("VM %s already exists — choose a different --newid", newID)
	}

	opts := &vm.CloneOptions{NewID: newID}
	opts.Name, _ = cmd.Flags().GetString("name")
	opts.Description, _ = cmd.Flags().GetString("description")
	opts.Pool, _ = cmd.Flags().GetString("pool")
	opts.SnapName, _ = cmd.Flags().GetString("snapname")
	opts.Storage, _ = cmd.Flags().GetString("storage")
	opts.Format, _ = cmd.Flags().GetString("format")
	opts.Target, _ = cmd.Flags().GetString("target")
	opts.Full, _ = cmd.Flags().GetBool("full")
	opts.BWLimit, _ = cmd.Flags().GetInt("bwlimit")

	source, err := vmOps.GetVMStatus(vmid)
	if err != nil {
		return err
	}

	// Only a template can be linked-cloned: Proxmox always fully copies the
	// disks of a normal VM, whatever --full says. Reporting "linked clone" for
	// a normal VM would misstate both how long the clone takes and how much
	// space it uses, so derive the label from the source rather than the flag.
	sourceConfig, err := vmOps.GetVMConfig(vmid)
	if err != nil {
		return err
	}
	isTemplate := isTemplateConfig(sourceConfig)

	var cloneKind string
	switch {
	case !isTemplate && opts.Full:
		cloneKind = "full clone"
	case !isTemplate:
		cloneKind = "full clone (source is not a template, so a linked clone is not possible)"
	case opts.Full:
		cloneKind = "full clone of a template"
	default:
		cloneKind = "linked clone (shares disk data with the template)"
	}

	fmt.Printf("Clone VM %s (%s) → VM %s\n", vmid, source.Name, newID)
	fmt.Printf("  mode:     %s\n", cloneKind)
	if opts.Name != "" {
		fmt.Printf("  name:     %s\n", opts.Name)
	}
	if opts.Storage != "" {
		fmt.Printf("  storage:  %s\n", opts.Storage)
	}
	if opts.SnapName != "" {
		fmt.Printf("  snapshot: %s\n", opts.SnapName)
	}
	if opts.Target != "" {
		fmt.Printf("  target:   %s (requires shared storage)\n", opts.Target)
	}
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("clone", vmid, source.Name, fmt.Sprintf("%s → VM %s", cloneKind, newID))
		printDryRunSummary("clone", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Clone VM %s to VM %s?", vmid, newID)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	upid, err := vmOps.CloneVM(vmid, opts)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Clone of VM %s started (task %s)\n", vmid, upid)
	fmt.Println("   A full clone can take a while; check progress with 'pve cluster task list'.")
	return nil
}

func runMigrateCommand(cmd *cobra.Command, args []string) error {
	vmid, err := vmTargetFromFlags(cmd)
	if err != nil {
		return err
	}
	target, _ := cmd.Flags().GetString("target")

	if target == "" {
		return fmt.Errorf("--target must be specified")
	}

	opts := &vm.MigrateOptions{Target: target}
	opts.Online, _ = cmd.Flags().GetBool("online")
	opts.WithLocalDisks, _ = cmd.Flags().GetBool("with-local-disks")
	opts.TargetStorage, _ = cmd.Flags().GetString("target-storage")
	opts.MigrationType, _ = cmd.Flags().GetString("migration-type")
	opts.MigrationNetwork, _ = cmd.Flags().GetString("migration-network")
	opts.BWLimit, _ = cmd.Flags().GetInt("bwlimit")

	// Read preconditions before doing anything: they say whether this
	// migration can work at all, and are cheap compared to a failed attempt.
	pre, err := vmOps.GetMigratePrecondition(vmid)
	if err != nil {
		return err
	}

	fmt.Printf("Migration preconditions for VM %s → %s:\n\n", vmid, target)
	fmt.Printf("  VM state:       %s\n", runStateLabel(pre.Running))
	if len(pre.AllowedNodes) > 0 {
		fmt.Printf("  allowed nodes:  %s\n", strings.Join(pre.AllowedNodes, ", "))
	}
	for node, reasons := range pre.NotAllowedNodes {
		fmt.Printf("  blocked node:   %s (%s)\n", node, strings.Join(reasons, "; "))
	}
	if len(pre.LocalDisks) > 0 {
		fmt.Printf("  local disks:    %s\n", describeLocalDisks(pre.LocalDisks))
	}
	if len(pre.LocalResources) > 0 {
		fmt.Printf("  local devices:  %s\n", strings.Join(pre.LocalResources, ", "))
	}
	fmt.Println()

	// Surface the blockers the API would otherwise reject mid-flight.
	var blockers []string
	if reasons, blocked := pre.NotAllowedNodes[target]; blocked {
		blockers = append(blockers, fmt.Sprintf("target node %s is not available: %s", target, strings.Join(reasons, "; ")))
	}
	if len(pre.AllowedNodes) > 0 && !contains(pre.AllowedNodes, target) {
		blockers = append(blockers, fmt.Sprintf("target node %s is not in the allowed set (%s)", target, strings.Join(pre.AllowedNodes, ", ")))
	}
	if len(pre.LocalDisks) > 0 && !opts.WithLocalDisks {
		blockers = append(blockers, "VM has disks on local storage — pass --with-local-disks to copy them to the target")
	}
	if pre.Running && !opts.Online {
		blockers = append(blockers, "VM is running — pass --online for a live migration, or stop it first")
	}
	if len(pre.LocalResources) > 0 {
		blockers = append(blockers, fmt.Sprintf("VM has local devices that block migration: %s", strings.Join(pre.LocalResources, ", ")))
	}

	if len(blockers) > 0 {
		fmt.Println("❌ This migration would fail:")
		for _, blocker := range blockers {
			fmt.Printf("  • %s\n", blocker)
		}
		fmt.Println()
		return fmt.Errorf("migration preconditions not met for VM %s", vmid)
	}

	if dryRun {
		printDryRunHeader()
		printDryRunAction("migrate", vmid, "", fmt.Sprintf("→ node %s", target))
		printDryRunSummary("migrate", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Migrate VM %s to %s?", vmid, target)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	upid, err := vmOps.MigrateVM(vmid, opts)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Migration of VM %s to %s started (task %s)\n", vmid, target, upid)
	fmt.Println("   Check progress with 'pve cluster task list'.")
	return nil
}

func runResizeCommand(cmd *cobra.Command, args []string) error {
	vmid, err := vmTargetFromFlags(cmd)
	if err != nil {
		return err
	}
	disk, _ := cmd.Flags().GetString("disk")
	size, _ := cmd.Flags().GetString("size")

	if disk == "" {
		return fmt.Errorf("--disk must be specified, e.g. --disk scsi0")
	}
	if err := vm.ValidateResizeSize(size); err != nil {
		return err
	}

	// Read the config both to show the current size and to guard the write
	// against a concurrent modification.
	config, err := vmOps.GetVMConfig(vmid)
	if err != nil {
		return err
	}

	current, ok := config[disk]
	if !ok {
		return fmt.Errorf("VM %s has no disk %q", vmid, disk)
	}

	digest, _ := config["digest"].(string)

	fmt.Printf("Resize disk %s on VM %s:\n\n", disk, vmid)
	fmt.Printf("  current: %s\n", formatConfigValue(current))
	fmt.Printf("  change:  %s\n", size)
	fmt.Println()
	fmt.Println("  Note: this extends the disk only. Growing the filesystem inside")
	fmt.Println("  the guest is a separate step.")
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("resize", vmid, "", fmt.Sprintf("%s → %s", disk, size))
		printDryRunSummary("resize", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Resize %s on VM %s to %s?", disk, vmid, size)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	upid, err := vmOps.ResizeVMDisk(vmid, disk, size, digest)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Resize of %s on VM %s started (task %s)\n", disk, vmid, upid)
	return nil
}

func runMoveDiskCommand(cmd *cobra.Command, args []string) error {
	vmid, err := vmTargetFromFlags(cmd)
	if err != nil {
		return err
	}

	opts := &vm.MoveDiskOptions{}
	opts.Disk, _ = cmd.Flags().GetString("disk")
	opts.Storage, _ = cmd.Flags().GetString("storage")
	opts.TargetVMID, _ = cmd.Flags().GetString("target-vmid")
	opts.TargetDisk, _ = cmd.Flags().GetString("target-disk")
	opts.Format, _ = cmd.Flags().GetString("format")
	opts.Delete, _ = cmd.Flags().GetBool("delete")
	opts.BWLimit, _ = cmd.Flags().GetInt("bwlimit")

	if opts.Disk == "" {
		return fmt.Errorf("--disk must be specified, e.g. --disk scsi0")
	}
	if opts.Storage == "" && opts.TargetVMID == "" {
		return fmt.Errorf("either --storage or --target-vmid must be specified")
	}
	if opts.Storage != "" && opts.TargetVMID != "" {
		return fmt.Errorf("--storage and --target-vmid are mutually exclusive")
	}

	config, err := vmOps.GetVMConfig(vmid)
	if err != nil {
		return err
	}

	current, ok := config[opts.Disk]
	if !ok {
		return fmt.Errorf("VM %s has no disk %q", vmid, opts.Disk)
	}

	if digest, ok := config["digest"].(string); ok {
		opts.Digest = digest
	}

	destination := "storage " + opts.Storage
	if opts.TargetVMID != "" {
		destination = "VM " + opts.TargetVMID
	}

	fmt.Printf("Move disk %s on VM %s:\n\n", opts.Disk, vmid)
	fmt.Printf("  current:     %s\n", formatConfigValue(current))
	fmt.Printf("  destination: %s\n", destination)

	// Capacity pre-check. A move that runs out of space fails partway through,
	// after copying gigabytes, so it is worth one API read to refuse up front.
	if opts.Storage != "" {
		if err := checkMoveDiskCapacity(vmid, opts.Storage, formatConfigValue(current)); err != nil {
			return err
		}
	}
	if opts.Delete {
		fmt.Printf("  source:      deleted after a successful copy\n")
	} else {
		fmt.Printf("  source:      kept as an unused volume\n")
	}
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("move-disk", vmid, "", fmt.Sprintf("%s → %s", opts.Disk, destination))
		printDryRunSummary("move-disk", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Move %s on VM %s to %s?", opts.Disk, vmid, destination)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	upid, err := vmOps.MoveVMDisk(vmid, opts)
	if err != nil {
		return err
	}

	fmt.Printf("✅ Move of %s on VM %s started (task %s)\n", opts.Disk, vmid, upid)
	return nil
}

// checkMoveDiskCapacity refuses a move-disk when the target storage plainly
// cannot hold the disk, and reports what it found either way.
//
// Two deliberate non-failures, because a check that cannot tell "no space" from
// "no reading" would refuse operations on a healthy cluster:
//
//   - If the disk spec carries no size, or the target storage is not configured
//     on the VM's node, it prints what it knows and lets Proxmox decide.
//   - It reads capacity from the node the VM is on. That is the right node for a
//     local store; for a shared store (Ceph, NFS) every node reports the same
//     pool, so it is also correct there.
func checkMoveDiskCapacity(vmid, targetStorage, diskSpec string) error {
	needed, err := vm.ParseDiskSizeBytes(diskSpec)
	if err != nil {
		fmt.Printf("  capacity:    not checked (%v)\n", err)
		return nil
	}
	if needed == 0 {
		fmt.Printf("  capacity:    not checked (disk spec carries no size)\n")
		return nil
	}

	status, err := vmOps.GetVMStatus(vmid)
	if err != nil {
		fmt.Printf("  capacity:    not checked (could not resolve the VM's node: %v)\n", err)
		return nil
	}

	storageOps := storage.NewOperations(client, logger)
	cap, err := storageOps.GetStorageCapacity(status.Node, targetStorage)
	if err != nil {
		fmt.Printf("  capacity:    not checked (%v)\n", err)
		return nil
	}
	if cap == nil {
		fmt.Printf("  capacity:    not checked (storage %q is not configured on node %s)\n",
			targetStorage, status.Node)
		return nil
	}

	scope := "node-local"
	if cap.Shared {
		scope = "shared"
	}
	fmt.Printf("  capacity:    %s free on %s (%s, %s), disk needs %s\n",
		formatBytes(cap.AvailByte), targetStorage, cap.Type, scope, formatBytes(needed))

	if !cap.Active {
		fmt.Printf("  ⚠️  storage %q is not active on node %s\n", targetStorage, status.Node)
	}

	if cap.AvailByte < needed {
		return fmt.Errorf("not enough space on %s: %s free, but disk %s needs %s",
			targetStorage, formatBytes(cap.AvailByte), diskSpec, formatBytes(needed))
	}

	// A move that just fits leaves the pool at capacity, which breaks the next
	// write rather than this one. Warn without refusing — the operator may be
	// deliberately consolidating.
	if remaining := cap.AvailByte - needed; remaining < cap.TotalByte/10 {
		fmt.Printf("  ⚠️  only %s would remain free (<10%% of %s)\n",
			formatBytes(remaining), formatBytes(cap.TotalByte))
	}

	return nil
}

func runTemplateCommand(cmd *cobra.Command, args []string) error {
	vmid, err := vmTargetFromFlags(cmd)
	if err != nil {
		return err
	}
	disk, _ := cmd.Flags().GetString("disk")

	status, err := vmOps.GetVMStatus(vmid)
	if err != nil {
		return err
	}

	// Proxmox refuses to convert a running VM, and the conversion cannot be
	// undone, so check before asking for confirmation.
	if status.Running || status.Paused {
		return fmt.Errorf("VM %s must be stopped before it can be converted to a template", vmid)
	}

	fmt.Printf("Convert VM %s (%s) into a template.\n\n", vmid, status.Name)
	fmt.Println("  This is one-way: a template cannot be started, and Proxmox")
	fmt.Println("  provides no way to convert it back into a regular VM.")
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("template", vmid, status.Name, "convert to template (one-way)")
		printDryRunSummary("template", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Convert VM %s into a template? This cannot be undone.", vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := vmOps.ConvertToTemplate(vmid, disk); err != nil {
		return err
	}

	fmt.Printf("✅ VM %s converted into a template\n", vmid)
	return nil
}

// isTemplateConfig reports whether a VM config describes a template. Proxmox
// sets the "template" key to 1 on templates and omits it otherwise; it arrives
// as a JSON number.
func isTemplateConfig(config map[string]interface{}) bool {
	switch v := config["template"].(type) {
	case float64:
		return v != 0
	case bool:
		return v
	case string:
		return v == "1"
	default:
		return false
	}
}

// runStateLabel renders a VM run state for the precondition summary.
func runStateLabel(running bool) string {
	if running {
		return "running (needs --online)"
	}
	return "stopped"
}

// describeLocalDisks renders local disks with their sizes.
func describeLocalDisks(disks []vm.LocalDisk) string {
	parts := make([]string, 0, len(disks))
	for _, disk := range disks {
		if disk.Size > 0 {
			parts = append(parts, fmt.Sprintf("%s (%s)", disk.DriveName, formatBytes(disk.Size)))
			continue
		}
		parts = append(parts, disk.DriveName)
	}
	return strings.Join(parts, ", ")
}

// formatBytes renders a byte count in binary units.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTP"[exp])
}

// contains reports whether a string slice holds a value.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
