package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// vmSuspendCmd suspends running VMs
var vmSuspendCmd = &cobra.Command{
	Use:   "suspend",
	Short: "Suspend VM(s)",
	Long: `Suspend virtual machines.

By default the VM is paused in RAM: it stops executing but stays resident on
the node, and resumes almost instantly. With --to-disk the VM state is written
out to storage and the memory is released, which survives a node reboot but
takes longer to save and restore.

Suspending to disk additionally requires Datastore.AllocateSpace on the state
storage, and is refused by Proxmox for HA-managed VMs.

Examples:
  # Pause a VM in RAM
  pve vm suspend --vmid 4103

  # Suspend to disk, letting Proxmox pick the state storage
  pve vm suspend --vmid 4103 --to-disk

  # Suspend to disk on a specific storage
  pve vm suspend --vmid 4103 --to-disk --state-storage local-zfs`,
	RunE: runSuspendCommand,
}

// vmResumeCmd resumes suspended VMs
var vmResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume suspended VM(s)",
	Long: `Resume suspended virtual machines.

Resumes VMs that were paused in RAM or suspended to disk by 'pve vm suspend'.

Note that a paused VM still reports its status as "running" in VM listings;
only the QEMU-level state distinguishes it. This command therefore checks each
VM's actual run state and skips any that are not paused.

Examples:
  pve vm resume --vmid 4103
  pve vm resume --vmname web01,web02`,
	RunE: runResumeCommand,
}

// initVMLifecycleFlags registers flags for the suspend and resume commands.
func initVMLifecycleFlags() {
	vmSuspendCmd.Flags().StringSlice("vmid", []string{}, "VM IDs (comma-separated)")
	vmSuspendCmd.Flags().StringSlice("vmname", []string{}, "VM names (comma-separated)")
	vmSuspendCmd.Flags().Bool("to-disk", false, "Suspend to disk instead of pausing in RAM")
	vmSuspendCmd.Flags().String("state-storage", "", "Storage for the VM state (requires --to-disk)")

	vmResumeCmd.Flags().StringSlice("vmid", []string{}, "VM IDs (comma-separated)")
	vmResumeCmd.Flags().StringSlice("vmname", []string{}, "VM names (comma-separated)")
}

// filterPausedVMs splits VMs into those that are paused and those that are not.
//
// This deliberately re-fetches each VM's status instead of trusting the VM
// records passed in: the cluster VM list endpoint does not return qmpstatus, so
// a paused VM appears as "running" there. Filtering on that stale field would
// skip exactly the VMs a resume is meant to act on.
func filterPausedVMs(vms []*vm.VM) (paused []*vm.VM, skipped []*vm.VM) {
	for _, vmInstance := range vms {
		status, err := vmOps.GetVMStatus(vmInstance.VMID)
		if err != nil {
			logger.Warnf("Could not read status for VM %s: %v", vmInstance.VMID, err)
			skipped = append(skipped, vmInstance)
			continue
		}

		if status.Paused {
			paused = append(paused, vmInstance)
		} else {
			skipped = append(skipped, vmInstance)
		}
	}
	return paused, skipped
}

// reportSkippedNotPaused prints the VMs excluded because they are not paused.
func reportSkippedNotPaused(skipped []*vm.VM) {
	if len(skipped) == 0 {
		return
	}
	fmt.Printf("ℹ️  Skipping %d VM(s) not paused (nothing to resume):\n", len(skipped))
	for _, vmInstance := range skipped {
		fmt.Printf("  • VM %s (%s)\n", vmInstance.VMID, vmInstance.Name)
	}
	fmt.Println()
}

func runSuspendCommand(cmd *cobra.Command, args []string) error {
	vmids, _ := cmd.Flags().GetStringSlice("vmid")
	vmnames, _ := cmd.Flags().GetStringSlice("vmname")
	toDisk, _ := cmd.Flags().GetBool("to-disk")
	stateStorage, _ := cmd.Flags().GetString("state-storage")

	if len(vmids) == 0 && len(vmnames) == 0 {
		return fmt.Errorf("either --vmid or --vmname must be specified")
	}

	if stateStorage != "" && !toDisk {
		return fmt.Errorf("--state-storage requires --to-disk")
	}

	vms, err := resolveVMs(vmids, vmnames)
	if err != nil {
		return err
	}

	// Only a running VM can be suspended.
	running, skipped := filterRunningVMs(vms)
	reportSkippedVMs(skipped, "suspend")
	if len(running) == 0 {
		fmt.Println("No running VMs to suspend")
		return nil
	}

	action := "suspend (pause in RAM)"
	if toDisk {
		action = "suspend to disk"
	}
	logger.Infof("Suspending %d VM(s) (todisk=%v)", len(running), toDisk)

	if dryRun {
		printDryRunHeader()
		for _, vmInstance := range running {
			details := "pause in RAM"
			if toDisk {
				details = "suspend to disk"
				if stateStorage != "" {
					details += " on " + stateStorage
				}
			}
			printDryRunAction("suspend", vmInstance.VMID, vmInstance.Name, details)
		}
		printDryRunSummary("suspend", len(running))
		return nil
	}

	if !confirmOperation(fmt.Sprintf("%s %d VM(s)?", action, len(running))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupts(cancel, bulkMgr)

	if err := bulkMgr.SuspendVMs(ctx, running, toDisk, stateStorage); err != nil {
		return fmt.Errorf("bulk VM suspend failed: %w", err)
	}

	bulkMgr.PrintSummary()
	return nil
}

func runResumeCommand(cmd *cobra.Command, args []string) error {
	vmids, _ := cmd.Flags().GetStringSlice("vmid")
	vmnames, _ := cmd.Flags().GetStringSlice("vmname")

	if len(vmids) == 0 && len(vmnames) == 0 {
		return fmt.Errorf("either --vmid or --vmname must be specified")
	}

	vms, err := resolveVMs(vmids, vmnames)
	if err != nil {
		return err
	}

	paused, skipped := filterPausedVMs(vms)
	reportSkippedNotPaused(skipped)
	if len(paused) == 0 {
		fmt.Println("No paused VMs to resume")
		return nil
	}

	logger.Infof("Resuming %d VM(s)", len(paused))

	if dryRun {
		printDryRunHeader()
		for _, vmInstance := range paused {
			printDryRunAction("resume", vmInstance.VMID, vmInstance.Name, "")
		}
		printDryRunSummary("resume", len(paused))
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Resume %d VM(s)?", len(paused))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupts(cancel, bulkMgr)

	if err := bulkMgr.ResumeVMs(ctx, paused); err != nil {
		return fmt.Errorf("bulk VM resume failed: %w", err)
	}

	bulkMgr.PrintSummary()
	return nil
}
