package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/disks"
)

var (
	disksNodeFlag string
	disksPoolFlag string
	disksDiskFlag string
)

// disksCmd is the "disks" group, added under the node command.
// All subcommands are read-only and require Sys.Audit on /nodes.
var disksCmd = &cobra.Command{
	Use:   "disks",
	Short: "ZFS pool and physical disk reporting",
	Long: `Report ZFS pool usage, physical disks, and SMART health via the Proxmox API.

These commands query /nodes/{node}/disks/* which requires Sys.Audit on /nodes.
Grant it with:
  pveum aclmod /nodes -token '<user>@<realm>!<token>' -role PVEAuditor

Examples:
  pve node disks zfs
  pve node disks zfs --node pve1
  pve node disks zfs-detail --node pve1 --pool rpool
  pve node disks list --node pve1
  pve node disks smart --node pve1 --disk /dev/sda`,
}

var disksZFSCmd = &cobra.Command{
	Use:   "zfs",
	Short: "List ZFS pool usage across the cluster",
	Long: `Display ZFS pool size, allocation, free space, fragmentation, and health.

Equivalent to 'pve zfs pool list', which is the primary home for ZFS commands
and sits alongside pool create/destroy, storage management and replication.
This form continues to work and is not deprecated.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pools, err := disksOps.GetZFSPools(disksNodeFlag)
		if err != nil {
			return err
		}
		disks.DisplayZFSPools(pools)
		return nil
	},
}

var disksZFSDetailCmd = &cobra.Command{
	Use:   "zfs-detail",
	Short: "Show the device tree for a single ZFS pool",
	Long: `Display the vdev tree and error counters for a specific ZFS pool.

Equivalent to 'pve zfs pool status', which is the primary home for ZFS commands.
This form continues to work and is not deprecated.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if disksNodeFlag == "" || disksPoolFlag == "" {
			return fmt.Errorf("both --node and --pool flags are required")
		}
		detail, err := disksOps.GetZFSPoolDetail(disksNodeFlag, disksPoolFlag)
		if err != nil {
			return err
		}
		disks.DisplayZFSPoolDetail(detail)
		return nil
	},
}

var disksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List physical disks on a node",
	Long:  "Display physical disks with size, model, wearout, health, and usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		if disksNodeFlag == "" {
			return fmt.Errorf("--node flag is required")
		}
		list, err := disksOps.GetDisks(disksNodeFlag)
		if err != nil {
			return err
		}
		disks.DisplayDisks(disksNodeFlag, list)
		return nil
	},
}

var disksSMARTCmd = &cobra.Command{
	Use:   "smart",
	Short: "Show SMART health for a disk",
	Long:  "Display SMART overall health and attributes for a specific disk device",
	RunE: func(cmd *cobra.Command, args []string) error {
		if disksNodeFlag == "" || disksDiskFlag == "" {
			return fmt.Errorf("both --node and --disk flags are required")
		}
		status, err := disksOps.GetDiskSMART(disksNodeFlag, disksDiskFlag)
		if err != nil {
			return err
		}
		disks.DisplaySMART(status)
		return nil
	},
}

func initDisksCommands() {
	disksCmd.AddCommand(disksZFSCmd)
	disksCmd.AddCommand(disksZFSDetailCmd)
	disksCmd.AddCommand(disksListCmd)
	disksCmd.AddCommand(disksSMARTCmd)

	disksZFSCmd.Flags().StringVar(&disksNodeFlag, "node", "", "Filter by specific node (optional)")

	disksZFSDetailCmd.Flags().StringVar(&disksNodeFlag, "node", "", "Node name (required)")
	disksZFSDetailCmd.Flags().StringVar(&disksPoolFlag, "pool", "", "Pool name (required)")
	_ = disksZFSDetailCmd.MarkFlagRequired("node")
	_ = disksZFSDetailCmd.MarkFlagRequired("pool")

	disksListCmd.Flags().StringVar(&disksNodeFlag, "node", "", "Node name (required)")
	_ = disksListCmd.MarkFlagRequired("node")

	disksSMARTCmd.Flags().StringVar(&disksNodeFlag, "node", "", "Node name (required)")
	disksSMARTCmd.Flags().StringVar(&disksDiskFlag, "disk", "", "Disk device path, e.g. /dev/sda (required)")
	_ = disksSMARTCmd.MarkFlagRequired("node")
	_ = disksSMARTCmd.MarkFlagRequired("disk")
}
