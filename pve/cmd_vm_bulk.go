package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/backup"
	"github.com/yg-codes/proxmox-cli/pkg/storage"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// Flags for bulk operations
var (
	bulkStorageFlag  string
	bulkModeFlag     string
	bulkCompressFlag string
	bulkVMIDsFlag    string
)

// bulkCmd represents the bulk operations command group
var bulkCmd = &cobra.Command{
	Use:   "bulk",
	Short: "Bulk operations on all VMs",
	Long: `Perform operations on all VMs in the cluster.

Bulk operations automatically discover and act on all VMs that match
the operation criteria (e.g., all stopped VMs for start, all running VMs for stop).

Available bulk operations:
  start    - Start all stopped VMs
  stop     - Stop all running VMs (force)
  shutdown - Gracefully shut down all running VMs
  reboot   - Reboot all running VMs
  backup   - Backup all VMs`,
}

// bulkStartCmd starts all stopped VMs
var bulkStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start all stopped VMs",
	Long: `Start all stopped VMs in the cluster.

This command automatically finds all stopped VMs and starts them.

Examples:
  # Dry-run to see what would be started
  pve vm bulk start --dry-run

  # Start all stopped VMs with confirmation
  pve vm bulk start

  # Start all stopped VMs without confirmation
  pve vm bulk start --yes

  # Start all stopped VMs on a specific node
  pve vm bulk start --node pve-node5

  # Start specific stopped VMs
  pve vm bulk start --vmid 4101,4102,4103`,
	RunE: runBulkStartCommand,
}

// bulkStopCmd stops all running VMs
var bulkStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all running VMs",
	Long: `Stop all running VMs in the cluster.

This command automatically finds all running VMs and stops them.
WARNING: This is a force stop, not a graceful shutdown.

Examples:
  # Dry-run to see what would be stopped
  pve vm bulk stop --dry-run

  # Stop all running VMs with confirmation
  pve vm bulk stop

  # Stop all running VMs without confirmation
  pve vm bulk stop --yes

  # Stop all running VMs on a specific node
  pve vm bulk stop --node pve-node5

  # Stop specific running VMs
  pve vm bulk stop --vmid 4101,4102,4103`,
	RunE: runBulkStopCommand,
}

// bulkShutdownCmd gracefully shuts down all running VMs
var bulkShutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Gracefully shut down all running VMs",
	Long: `Gracefully shut down all running VMs in the cluster using ACPI shutdown.

This command automatically finds all running VMs and sends an ACPI shutdown signal.
Unlike bulk stop (which force-kills VMs), this allows VMs to shut down cleanly.

Examples:
  # Dry-run to see what would be shut down
  pve vm bulk shutdown --dry-run

  # Gracefully shut down all running VMs with confirmation
  pve vm bulk shutdown

  # Shut down without confirmation
  pve vm bulk shutdown --yes

  # Shut down all running VMs on a specific node
  pve vm bulk shutdown --node pve-node5

  # Shut down specific running VMs
  pve vm bulk shutdown --vmid 4101,4102,4103`,
	RunE: runBulkShutdownCommand,
}

// bulkRebootCmd reboots all running VMs
var bulkRebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot all running VMs",
	Long: `Reboot all running VMs in the cluster.

This command automatically finds all running VMs and reboots each one by
shutting it down (ACPI) and starting it again, which applies any pending
configuration changes.

Examples:
  # Dry-run to see what would be rebooted
  pve vm bulk reboot --dry-run

  # Reboot all running VMs with confirmation
  pve vm bulk reboot

  # Reboot without confirmation
  pve vm bulk reboot --yes

  # Reboot all running VMs on a specific node
  pve vm bulk reboot --node pve-node5

  # Reboot specific running VMs
  pve vm bulk reboot --vmid 4101,4102,4103`,
	RunE: runBulkRebootCommand,
}

// bulkBackupCmd backs up all VMs
var bulkBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup all VMs",
	Long: `Backup all VMs in the cluster to specified storage.

This command automatically finds all VMs and backs them up.

Examples:
  # Dry-run to see what would be backed up
  pve vm bulk backup --storage local-zfs --dry-run

  # Backup all VMs with snapshot mode
  pve vm bulk backup --storage local-zfs

  # Backup all VMs with specific mode and compression
  pve vm bulk backup --storage local-zfs --mode suspend --compress gzip --yes

  # Backup all VMs on a specific node
  pve vm bulk backup --storage local-zfs --node pve-node5

  # Backup specific VMs
  pve vm bulk backup --storage local-zfs --vmid 4101,4102`,
	RunE: runBulkBackupCommand,
}

// initBulkCommands sets up the bulk command hierarchy
func initBulkCommands() {
	// Add bulk subcommands
	bulkCmd.AddCommand(bulkStartCmd)
	bulkCmd.AddCommand(bulkStopCmd)
	bulkCmd.AddCommand(bulkShutdownCmd)
	bulkCmd.AddCommand(bulkRebootCmd)
	bulkCmd.AddCommand(bulkBackupCmd)

	// Bulk backup flags
	bulkBackupCmd.Flags().StringVar(&bulkStorageFlag, "storage", "", "Storage for backup (required)")
	bulkBackupCmd.Flags().StringVar(&bulkModeFlag, "mode", "snapshot", "Backup mode: snapshot, suspend, or stop")
	bulkBackupCmd.Flags().StringVar(&bulkCompressFlag, "compress", "zstd", "Compression: zstd, gzip, or lzo")
	_ = bulkBackupCmd.MarkFlagRequired("storage")

	// Bulk VM ID filter flags
	bulkStartCmd.Flags().StringVar(&bulkVMIDsFlag, "vmid", "", "Comma-separated VM IDs to target (default: all matching VMs)")
	bulkStopCmd.Flags().StringVar(&bulkVMIDsFlag, "vmid", "", "Comma-separated VM IDs to target (default: all matching VMs)")
	bulkShutdownCmd.Flags().StringVar(&bulkVMIDsFlag, "vmid", "", "Comma-separated VM IDs to target (default: all matching VMs)")
	bulkRebootCmd.Flags().StringVar(&bulkVMIDsFlag, "vmid", "", "Comma-separated VM IDs to target (default: all matching VMs)")
	bulkBackupCmd.Flags().StringVar(&bulkVMIDsFlag, "vmid", "", "Comma-separated VM IDs to target (default: all matching VMs)")
}

// filterVMsByNode filters VMs to those on the specified node.
// If node is empty, returns the input slice unchanged.
func filterVMsByNode(vms []*vm.VM, node string) []*vm.VM {
	if node == "" {
		return vms
	}
	var filtered []*vm.VM
	for _, v := range vms {
		if v.Node == node {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// filterVMsByIDs filters VMs to only those matching the provided IDs.
// If vmidsStr is empty, returns the input slice unchanged.
func filterVMsByIDs(vms []*vm.VM, vmidsStr string) ([]*vm.VM, error) {
	if vmidsStr == "" {
		return vms, nil
	}

	wantSet := make(map[string]bool)
	for _, id := range strings.Split(vmidsStr, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			wantSet[id] = true
		}
	}

	var filtered []*vm.VM
	for _, v := range vms {
		if wantSet[v.VMID] {
			filtered = append(filtered, v)
			delete(wantSet, v.VMID)
		}
	}

	if len(wantSet) > 0 {
		var notFound []string
		for id := range wantSet {
			notFound = append(notFound, id)
		}
		return filtered, fmt.Errorf("VM IDs not found: %s", strings.Join(notFound, ", "))
	}

	return filtered, nil
}

// runBulkStartCommand handles the bulk start operation
func runBulkStartCommand(cmd *cobra.Command, args []string) error {
	printDryRunHeader()

	// Get all VMs
	allVMs, err := vmOps.GetAllVMs()
	if err != nil {
		return fmt.Errorf("failed to get VMs: %w", err)
	}

	// Filter by node if specified
	allVMs = filterVMsByNode(allVMs, vmNodeFlag)

	// Filter by VM IDs if specified
	allVMs, err = filterVMsByIDs(allVMs, bulkVMIDsFlag)
	if err != nil {
		return err
	}

	// Filter to stopped VMs
	var stoppedVMs []*vm.VM
	for _, vmInstance := range allVMs {
		if !vmInstance.Running {
			stoppedVMs = append(stoppedVMs, vmInstance)
		}
	}

	if len(stoppedVMs) == 0 {
		fmt.Println("ℹ️  No stopped VMs found")
		return nil
	}

	// Display list
	fmt.Printf("\nFound %d stopped VM(s):\n", len(stoppedVMs))
	for _, vmInstance := range stoppedVMs {
		fmt.Printf("  • VM %s (%s) - stopped\n", vmInstance.VMID, vmInstance.Name)
	}
	fmt.Println()

	// Dry-run mode - just show what would happen
	if dryRun {
		for _, vmInstance := range stoppedVMs {
			printDryRunAction("start", vmInstance.VMID, vmInstance.Name, "")
		}
		printDryRunSummary("start", len(stoppedVMs))
		return nil
	}

	// Confirm operation
	if !confirmOperation(fmt.Sprintf("Start %d stopped VM(s)?", len(stoppedVMs))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	// Execute bulk operation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupts(cancel, bulkMgr)

	logger.Infof("Starting %d VM(s)", len(stoppedVMs))
	opErr := bulkMgr.StartVMs(ctx, stoppedVMs)
	bulkMgr.PrintSummary()
	if opErr != nil {
		return fmt.Errorf("bulk VM start failed: %w", opErr)
	}
	return nil
}

// runBulkStopCommand handles the bulk stop operation
func runBulkStopCommand(cmd *cobra.Command, args []string) error {
	printDryRunHeader()

	// Get all VMs
	allVMs, err := vmOps.GetAllVMs()
	if err != nil {
		return fmt.Errorf("failed to get VMs: %w", err)
	}

	// Filter by node if specified
	allVMs = filterVMsByNode(allVMs, vmNodeFlag)

	// Filter by VM IDs if specified
	allVMs, err = filterVMsByIDs(allVMs, bulkVMIDsFlag)
	if err != nil {
		return err
	}

	// Filter to running VMs
	var runningVMs []*vm.VM
	for _, vmInstance := range allVMs {
		if vmInstance.Running {
			runningVMs = append(runningVMs, vmInstance)
		}
	}

	if len(runningVMs) == 0 {
		fmt.Println("ℹ️  No running VMs found")
		return nil
	}

	// Display list
	fmt.Printf("\n⚠️  Found %d running VM(s):\n", len(runningVMs))
	for _, vmInstance := range runningVMs {
		fmt.Printf("  • VM %s (%s) - running\n", vmInstance.VMID, vmInstance.Name)
	}
	fmt.Println()

	// Dry-run mode - just show what would happen
	if dryRun {
		for _, vmInstance := range runningVMs {
			printDryRunAction("force stop", vmInstance.VMID, vmInstance.Name, "")
		}
		printDryRunSummary("force stop", len(runningVMs))
		return nil
	}

	// Confirm operation with extra warning
	fmt.Println("⚠️  WARNING: This will FORCE STOP all running VMs (not graceful shutdown)")
	if !confirmOperation(fmt.Sprintf("Force stop %d running VM(s)?", len(runningVMs))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	// Execute bulk operation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupts(cancel, bulkMgr)

	logger.Infof("Stopping %d VM(s)", len(runningVMs))
	opErr := bulkMgr.StopVMs(ctx, runningVMs)
	bulkMgr.PrintSummary()
	if opErr != nil {
		return fmt.Errorf("bulk VM stop failed: %w", opErr)
	}
	return nil
}

// runBulkShutdownCommand handles the bulk shutdown operation
func runBulkShutdownCommand(cmd *cobra.Command, args []string) error {
	printDryRunHeader()

	allVMs, err := vmOps.GetAllVMs()
	if err != nil {
		return fmt.Errorf("failed to get VMs: %w", err)
	}

	allVMs = filterVMsByNode(allVMs, vmNodeFlag)

	// Filter by VM IDs if specified
	allVMs, err = filterVMsByIDs(allVMs, bulkVMIDsFlag)
	if err != nil {
		return err
	}

	var runningVMs []*vm.VM
	for _, vmInstance := range allVMs {
		if vmInstance.Running {
			runningVMs = append(runningVMs, vmInstance)
		}
	}

	if len(runningVMs) == 0 {
		fmt.Println("ℹ️  No running VMs found")
		return nil
	}

	fmt.Printf("\nFound %d running VM(s):\n", len(runningVMs))
	for _, vmInstance := range runningVMs {
		fmt.Printf("  • VM %s (%s) - running\n", vmInstance.VMID, vmInstance.Name)
	}
	fmt.Println()

	if dryRun {
		for _, vmInstance := range runningVMs {
			printDryRunAction("shutdown", vmInstance.VMID, vmInstance.Name, "")
		}
		printDryRunSummary("shutdown", len(runningVMs))
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Gracefully shut down %d running VM(s)?", len(runningVMs))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupts(cancel, bulkMgr)

	logger.Infof("Shutting down %d VM(s)", len(runningVMs))
	opErr := bulkMgr.ShutdownVMs(ctx, runningVMs)
	bulkMgr.PrintSummary()
	if opErr != nil {
		return fmt.Errorf("bulk VM shutdown failed: %w", opErr)
	}
	return nil
}

// runBulkRebootCommand handles the bulk reboot operation
func runBulkRebootCommand(cmd *cobra.Command, args []string) error {
	printDryRunHeader()

	allVMs, err := vmOps.GetAllVMs()
	if err != nil {
		return fmt.Errorf("failed to get VMs: %w", err)
	}

	allVMs = filterVMsByNode(allVMs, vmNodeFlag)

	// Filter by VM IDs if specified
	allVMs, err = filterVMsByIDs(allVMs, bulkVMIDsFlag)
	if err != nil {
		return err
	}

	var runningVMs []*vm.VM
	for _, vmInstance := range allVMs {
		if vmInstance.Running {
			runningVMs = append(runningVMs, vmInstance)
		}
	}

	if len(runningVMs) == 0 {
		fmt.Println("ℹ️  No running VMs found")
		return nil
	}

	fmt.Printf("\nFound %d running VM(s):\n", len(runningVMs))
	for _, vmInstance := range runningVMs {
		fmt.Printf("  • VM %s (%s) - running\n", vmInstance.VMID, vmInstance.Name)
	}
	fmt.Println()

	if dryRun {
		for _, vmInstance := range runningVMs {
			printDryRunAction("reboot", vmInstance.VMID, vmInstance.Name, "")
		}
		printDryRunSummary("reboot", len(runningVMs))
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Reboot %d running VM(s)?", len(runningVMs))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupts(cancel, bulkMgr)

	logger.Infof("Rebooting %d VM(s)", len(runningVMs))
	opErr := bulkMgr.RebootVMs(ctx, runningVMs)
	bulkMgr.PrintSummary()
	if opErr != nil {
		return fmt.Errorf("bulk VM reboot failed: %w", opErr)
	}
	return nil
}

// runBulkBackupCommand handles the bulk backup operation
func runBulkBackupCommand(cmd *cobra.Command, args []string) error {
	printDryRunHeader()

	// Get all VMs
	allVMs, err := vmOps.GetAllVMs()
	if err != nil {
		return fmt.Errorf("failed to get VMs: %w", err)
	}

	// Filter by node if specified
	allVMs = filterVMsByNode(allVMs, vmNodeFlag)

	// Filter by VM IDs if specified
	allVMs, err = filterVMsByIDs(allVMs, bulkVMIDsFlag)
	if err != nil {
		return err
	}

	if len(allVMs) == 0 {
		fmt.Println("ℹ️  No VMs found")
		return nil
	}

	// Display list
	fmt.Printf("\nFound %d VM(s) to backup:\n", len(allVMs))
	for _, vmInstance := range allVMs {
		status := "stopped"
		if vmInstance.Running {
			status = "running"
		}
		fmt.Printf("  • VM %s (%s) - %s\n", vmInstance.VMID, vmInstance.Name, status)
	}
	fmt.Println()

	// Validate storage (skip in dry-run)
	if !dryRun {
		storageOps := storage.NewOperations(client, logger)
		if err := storageOps.ValidateStorage(bulkStorageFlag); err != nil {
			return fmt.Errorf("storage validation failed: %w", err)
		}
	}

	// Validate mode
	var mode backup.BackupMode
	switch bulkModeFlag {
	case "snapshot":
		mode = backup.BackupModeSnapshot
	case "suspend":
		mode = backup.BackupModeSuspend
	case "stop":
		mode = backup.BackupModeStop
	default:
		return fmt.Errorf("invalid mode: %s (must be snapshot, suspend, or stop)", bulkModeFlag)
	}

	// Dry-run mode - just show what would happen
	if dryRun {
		for _, vmInstance := range allVMs {
			details := fmt.Sprintf("storage=%s, mode=%s, compress=%s", bulkStorageFlag, bulkModeFlag, bulkCompressFlag)
			printDryRunAction("backup", vmInstance.VMID, vmInstance.Name, details)
		}
		printDryRunSummary("backup", len(allVMs))
		return nil
	}

	// Confirm operation
	confirmMsg := fmt.Sprintf("Backup %d VM(s) to storage '%s' with mode '%s'?",
		len(allVMs), bulkStorageFlag, bulkModeFlag)
	if !confirmOperation(confirmMsg) {
		fmt.Println("Operation cancelled")
		return nil
	}

	// Execute backups
	backupOps := backup.NewOperations(client, vmOps, logger)
	logger.Infof("Creating backups for %d VM(s)", len(allVMs))

	successCount := 0
	var failures []error
	for _, vmInstance := range allVMs {
		fmt.Printf("\nBacking up VM %s (%s)...\n", vmInstance.VMID, vmInstance.Name)
		if err := backupOps.CreateBackup(cmd.Context(), vmInstance.VMID, bulkStorageFlag, mode, bulkCompressFlag); err != nil {
			logger.Errorf("Failed to backup VM %s: %v", vmInstance.VMID, err)
			failures = append(failures, fmt.Errorf("VM %s: %w", vmInstance.VMID, err))
		} else {
			successCount++
		}
	}

	fmt.Printf("\n✅ Successfully backed up %d/%d VMs\n", successCount, len(allVMs))
	return joinTargetErrors(len(allVMs), failures)
}
