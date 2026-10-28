package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/bulk"
	"github.com/yg-codes/proxmox-cli/pkg/snapshot"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

var snapshotPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview snapshots across the whole cluster",
	Long: `Show every snapshot in the cluster as a flat table.

Unlike 'list', this defaults to the entire cluster and needs no --vmid.
The summary line reports how many VMs were scanned, so an empty table can be
read as "nothing to do" rather than "the scan failed".

Examples:
  # Every snapshot in the cluster
  pve vm snapshot preview

  # Only one node
  pve vm snapshot preview --node pve-node1

  # Snapshots older than 30 days
  pve vm snapshot preview --older-than 30d

  # Snapshots matching a name pattern
  pve vm snapshot preview --name-pattern 'pre-update-*'

  # Narrow to specific VMs (selectors work: all, running, 972*, 4001-4005)
  pve vm snapshot preview --vmid running`,
	RunE: runSnapshotPreviewCommand,
}

var snapshotPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete snapshots across the cluster, by filter",
	Long: `Delete snapshots matching a filter, across one or many VMs.

At least one filter (--older-than, --name-pattern, --vmid/--vmname) is
REQUIRED. There is deliberately no form that deletes every snapshot in the
cluster in one command.

The full list of matched snapshots is always printed before anything is
deleted. Confirmation requires typing the exact snapshot count.

Examples:
  # Preview first — always do this
  pve vm snapshot prune --older-than 30d --dry-run

  # Delete snapshots older than 30 days
  pve vm snapshot prune --older-than 30d

  # Delete by name pattern, on one node
  pve vm snapshot prune --name-pattern 'pre-update-*' --node pve-node1

  # Combine filters (AND): old AND matching the pattern, on these VMs
  pve vm snapshot prune --vmid 4001,4002 --older-than 7d --name-pattern 'test-*'`,
	RunE: runSnapshotPruneCommand,
}

// resolveScanTargets determines which VMs a cluster-wide snapshot command
// should inspect, and reports whether the caller narrowed the VM set.
func resolveScanTargets(cmd *cobra.Command) (vms []*vm.VM, narrowed bool, err error) {
	vmids, _ := cmd.Flags().GetStringSlice("vmid")
	vmnames, _ := cmd.Flags().GetStringSlice("vmname")

	if len(vmids) > 0 || len(vmnames) > 0 {
		vms, err = resolveVMs(vmids, vmnames)
		if err != nil {
			return nil, false, err
		}
		return vms, true, nil
	}

	allVMs, err := vmOps.GetAllVMs()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get VMs: %w", err)
	}
	return filterVMsByNode(allVMs, vmNodeFlag), false, nil
}

// scanFilterFromFlags builds the snapshot filter from command flags.
func scanFilterFromFlags(cmd *cobra.Command) (snapshot.ScanFilter, error) {
	olderThanStr, _ := cmd.Flags().GetString("older-than")
	namePattern, _ := cmd.Flags().GetString("name-pattern")

	olderThan, err := snapshot.ParseDuration(olderThanStr)
	if err != nil {
		return snapshot.ScanFilter{}, err
	}

	return snapshot.ScanFilter{
		OlderThan:   olderThan,
		NamePattern: namePattern,
	}, nil
}

func runSnapshotPreviewCommand(cmd *cobra.Command, args []string) error {
	vms, _, err := resolveScanTargets(cmd)
	if err != nil {
		return err
	}

	if len(vms) == 0 {
		fmt.Println("No VMs matched.")
		return nil
	}

	filter, err := scanFilterFromFlags(cmd)
	if err != nil {
		return err
	}

	result, err := snapOps.ScanSnapshots(vms, filter, 3)
	if err != nil {
		return err
	}

	snapshot.DisplayClusterSnapshots(result)
	return nil
}

func runSnapshotPruneCommand(cmd *cobra.Command, args []string) error {
	vmids, _ := cmd.Flags().GetStringSlice("vmid")
	vmnames, _ := cmd.Flags().GetStringSlice("vmname")

	filter, err := scanFilterFromFlags(cmd)
	if err != nil {
		return err
	}

	// Refuse an unfiltered prune. Deleting every snapshot in the cluster is
	// not something that should be reachable by omitting arguments.
	if filter.IsEmpty() && len(vmids) == 0 && len(vmnames) == 0 {
		return fmt.Errorf("prune requires at least one filter: --older-than, --name-pattern, or --vmid/--vmname\n" +
			"       (refusing to delete every snapshot in the cluster)")
	}

	vms, _, err := resolveScanTargets(cmd)
	if err != nil {
		return err
	}

	if len(vms) == 0 {
		fmt.Println("No VMs matched — nothing to prune.")
		return nil
	}

	result, err := snapOps.ScanSnapshots(vms, filter, 3)
	if err != nil {
		return err
	}

	// Always show the full target set, in both dry-run and real mode.
	fmt.Println()
	snapshot.DisplayClusterSnapshots(result)
	fmt.Println()

	if len(result.Snapshots) == 0 {
		fmt.Println("No snapshots matched the filter — nothing to delete.")
		return nil
	}

	// A partial scan means the target set may be incomplete. Say so before
	// asking to delete, so an operator is not consenting to a view they think
	// is whole.
	if len(result.Failed) > 0 {
		logger.Warnf("%d VM(s) could not be read; the target set above is incomplete", len(result.Failed))
	}

	if dryRun {
		printDryRunHeader()
		for _, cs := range result.Snapshots {
			printDryRunAction("delete snapshot", cs.VM.VMID, cs.VM.Name,
				fmt.Sprintf("'%s'", cs.Snapshot.Name))
		}
		fmt.Printf("\n[DRY RUN] Nothing deleted. Re-run without --dry-run to apply.\n")
		return nil
	}

	// Require typing the count. An operator has to read the number off the
	// table above to answer, which a fixed confirmation word does not force.
	count := len(result.Snapshots)
	fmt.Printf("Delete %d snapshot(s) across %d VM(s)? This cannot be undone.\n",
		count, countDistinctVMs(result.Snapshots))
	if !confirmExactText(fmt.Sprintf("Type '%d' to confirm: ", count), strconv.Itoa(count)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	pairs := make([]bulk.SnapshotRef, 0, count)
	for _, cs := range result.Snapshots {
		pairs = append(pairs, bulk.SnapshotRef{
			VM:           cs.VM,
			SnapshotName: cs.Snapshot.Name,
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleInterrupts(cancel, bulkMgr)

	if err := bulkMgr.DeleteSnapshotPairs(ctx, pairs); err != nil {
		return fmt.Errorf("bulk snapshot prune failed: %w", err)
	}

	bulkMgr.PrintSummary()
	return nil
}

// countDistinctVMs counts how many distinct VMs a result set touches.
func countDistinctVMs(snaps []snapshot.ClusterSnapshot) int {
	seen := make(map[string]struct{}, len(snaps))
	for _, cs := range snaps {
		seen[cs.VM.VMID] = struct{}{}
	}
	return len(seen)
}

// initSnapshotClusterCommands registers the cluster-wide snapshot commands.
func initSnapshotClusterCommands() {
	for _, c := range []*cobra.Command{snapshotPreviewCmd, snapshotPruneCmd} {
		c.Flags().StringSlice("vmid", nil, "VM IDs or selectors (comma-separated; supports all, running, stopped, 72*, 7201-7205)")
		c.Flags().StringSlice("vmname", nil, "VM names (comma-separated)")
		c.Flags().String("older-than", "", "Only snapshots older than this age (e.g. 30d, 12h)")
		c.Flags().String("name-pattern", "", "Only snapshots whose name matches this glob (e.g. 'pre-update-*')")
	}

	snapshotCmd.AddCommand(snapshotPreviewCmd)
	snapshotCmd.AddCommand(snapshotPruneCmd)
}
