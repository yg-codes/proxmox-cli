package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/resource"
)

var (
	// Resource-specific flags
	resourceTypeFlag   string
	resourceNodeFlag   string
	resourceStatusFlag string
)

var resourceCmd = &cobra.Command{
	Use:   "resource",
	Short: "Resource monitoring and statistics",
	Long:  "Monitor and analyze cluster resource usage including CPU, memory, disk, and network",
}

var resourceStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cluster resource statistics",
	Long:  "Display aggregated cluster resource statistics including nodes, VMs, and storage",
	RunE: func(cmd *cobra.Command, args []string) error {
		stats, err := resourceOps.GetClusterStats()
		if err != nil {
			return fmt.Errorf("failed to get cluster stats: %w", err)
		}

		resource.DisplayClusterStats(stats)
		return nil
	},
}

var resourceCapacityCmd = &cobra.Command{
	Use:   "capacity",
	Short: "Show cluster capacity and overcommit analysis",
	Long: `Display per-node and cluster CPU/RAM overcommit ratios and an N-1 failover
verdict (whether the cluster can hold all allocated RAM if the largest node
is lost, while retaining quorum).

Note: RAM figures are ALLOCATED (VM maxmem), not actual in-guest used memory.
Actual-used analysis requires an external metrics server (Grafana/Prometheus).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		analysis, err := resourceOps.GetCapacityAnalysis()
		if err != nil {
			return fmt.Errorf("failed to compute capacity analysis: %w", err)
		}

		resource.DisplayCapacityAnalysis(analysis)
		return nil
	},
}

var resourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cluster resources",
	Long:  "List all cluster resources with optional filtering by type, node, or status",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := &resource.ResourceFilter{
			Type:   resourceTypeFlag,
			Node:   resourceNodeFlag,
			Status: resourceStatusFlag,
		}

		resources, err := resourceOps.GetClusterResources(filter)
		if err != nil {
			return fmt.Errorf("failed to list resources: %w", err)
		}

		// Display based on type filter
		if resourceTypeFlag == "node" || resourceTypeFlag == "" {
			if len(resources.Nodes) > 0 {
				resource.DisplayNodeResources(resources.Nodes)
			}
		}

		if resourceTypeFlag == "qemu" || resourceTypeFlag == "lxc" || resourceTypeFlag == "" {
			if len(resources.VMs) > 0 {
				resource.DisplayVMResources(resources.VMs)
			}
		}

		if resourceTypeFlag == "storage" || resourceTypeFlag == "" {
			if len(resources.Storage) > 0 {
				resource.DisplayStorageResources(resources.Storage)
			}
		}

		return nil
	},
}

var resourceNodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "List node resources",
	Long:  "Display resource usage for all cluster nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := &resource.ResourceFilter{
			Type:   "node",
			Node:   resourceNodeFlag,
			Status: resourceStatusFlag,
		}

		resources, err := resourceOps.GetClusterResources(filter)
		if err != nil {
			return fmt.Errorf("failed to list node resources: %w", err)
		}

		resource.DisplayNodeResources(resources.Nodes)
		return nil
	},
}

var resourceVMsCmd = &cobra.Command{
	Use:   "vms",
	Short: "List VM resources",
	Long:  "Display resource usage for all VMs (both QEMU and LXC)",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := &resource.ResourceFilter{
			Type:   resourceTypeFlag,
			Node:   resourceNodeFlag,
			Status: resourceStatusFlag,
		}

		// If no specific type, get both qemu and lxc
		if filter.Type == "" {
			filter.Type = "vm"
		}

		resources, err := resourceOps.GetClusterResources(filter)
		if err != nil {
			return fmt.Errorf("failed to list VM resources: %w", err)
		}

		resource.DisplayVMResources(resources.VMs)
		return nil
	},
}

var resourceStoragesCmd = &cobra.Command{
	Use:   "storages",
	Short: "List storage resources",
	Long:  "Display resource usage for all storage resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := &resource.ResourceFilter{
			Type:   "storage",
			Node:   resourceNodeFlag,
			Status: resourceStatusFlag,
		}

		resources, err := resourceOps.GetClusterResources(filter)
		if err != nil {
			return fmt.Errorf("failed to list storage resources: %w", err)
		}

		resource.DisplayStorageResources(resources.Storage)
		return nil
	},
}

var resourceNodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Show detailed node resource usage",
	Long:  "Display detailed resource information for a specific node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")

		if nodeName == "" {
			return fmt.Errorf("--node flag is required")
		}

		nodeRes, err := resourceOps.GetNodeResources(nodeName)
		if err != nil {
			return fmt.Errorf("failed to get node resources: %w", err)
		}

		resource.DisplayNodeDetails(nodeRes)
		return nil
	},
}

var resourceVMCmd = &cobra.Command{
	Use:   "vm",
	Short: "Show detailed VM resource usage",
	Long:  "Display detailed resource information for a specific VM",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		vmType, _ := cmd.Flags().GetString("type")

		if nodeName == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		if vmType == "" {
			vmType = "qemu" // Default to qemu
		}

		vmRes, err := resourceOps.GetVMResources(nodeName, vmType, vmid)
		if err != nil {
			return fmt.Errorf("failed to get VM resources: %w", err)
		}

		resource.DisplayVMDetails(vmRes)
		return nil
	},
}

var resourceHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show resource usage history",
	Long:  "Display historical resource usage data (RRD data) for a node or VM",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		vmType, _ := cmd.Flags().GetString("type")
		timeframe, _ := cmd.Flags().GetString("timeframe")

		if nodeName == "" {
			return fmt.Errorf("--node flag is required")
		}

		if timeframe == "" {
			timeframe = "hour" // Default to hour
		}

		var rrdData *resource.RRDData
		var err error
		var resourceName string

		if vmid > 0 {
			// Get VM history
			if vmType == "" {
				vmType = "qemu"
			}
			resourceName = fmt.Sprintf("VM %d", vmid)
			rrdData, err = resourceOps.GetVMRRDData(nodeName, vmType, vmid, timeframe)
		} else {
			// Get node history
			resourceName = fmt.Sprintf("Node %s", nodeName)
			rrdData, err = resourceOps.GetNodeRRDData(nodeName, timeframe)
		}

		if err != nil {
			return fmt.Errorf("failed to get resource history: %w", err)
		}

		resource.DisplayRRDData(rrdData, resourceName)
		return nil
	},
}

func initResourceCommands() {
	// Note: resourceCmd is now added by cmd_node.go
	// rootCmd.AddCommand(resourceCmd)

	resourceCmd.AddCommand(resourceStatsCmd)
	resourceCmd.AddCommand(resourceCapacityCmd)
	resourceCmd.AddCommand(resourceListCmd)
	resourceCmd.AddCommand(resourceNodesCmd)
	resourceCmd.AddCommand(resourceVMsCmd)
	resourceCmd.AddCommand(resourceStoragesCmd)
	resourceCmd.AddCommand(resourceNodeCmd)
	resourceCmd.AddCommand(resourceVMCmd)
	resourceCmd.AddCommand(resourceHistoryCmd)

	// Flags for list command
	resourceListCmd.Flags().StringVar(&resourceTypeFlag, "type", "", "Filter by type (node, qemu, lxc, storage)")
	resourceListCmd.Flags().StringVar(&resourceNodeFlag, "node", "", "Filter by node")
	resourceListCmd.Flags().StringVar(&resourceStatusFlag, "status", "", "Filter by status")

	// Flags for nodes command
	resourceNodesCmd.Flags().StringVar(&resourceNodeFlag, "node", "", "Filter by specific node")
	resourceNodesCmd.Flags().StringVar(&resourceStatusFlag, "status", "", "Filter by status (online, offline)")

	// Flags for vms command
	resourceVMsCmd.Flags().StringVar(&resourceTypeFlag, "type", "", "Filter by VM type (qemu, lxc)")
	resourceVMsCmd.Flags().StringVar(&resourceNodeFlag, "node", "", "Filter by node")
	resourceVMsCmd.Flags().StringVar(&resourceStatusFlag, "status", "", "Filter by status (running, stopped)")

	// Flags for storages command
	resourceStoragesCmd.Flags().StringVar(&resourceNodeFlag, "node", "", "Filter by node")
	resourceStoragesCmd.Flags().StringVar(&resourceStatusFlag, "status", "", "Filter by status")

	// Flags for node command
	resourceNodeCmd.Flags().String("node", "", "Node name (required)")
	_ = resourceNodeCmd.MarkFlagRequired("node")

	// Flags for vm command
	resourceVMCmd.Flags().String("node", "", "Node name (required)")
	resourceVMCmd.Flags().Int("vmid", 0, "VM ID (required)")
	resourceVMCmd.Flags().String("type", "qemu", "VM type (qemu or lxc)")
	_ = resourceVMCmd.MarkFlagRequired("node")
	_ = resourceVMCmd.MarkFlagRequired("vmid")

	// Flags for history command
	resourceHistoryCmd.Flags().String("node", "", "Node name (required)")
	resourceHistoryCmd.Flags().Int("vmid", 0, "VM ID (optional, for VM history)")
	resourceHistoryCmd.Flags().String("type", "qemu", "VM type (qemu or lxc)")
	resourceHistoryCmd.Flags().String("timeframe", "hour", "Timeframe (hour, day, week, month, year)")
	_ = resourceHistoryCmd.MarkFlagRequired("node")
}
