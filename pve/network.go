package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/network"
)

var (
	// Network-specific flags
	networkTypeFlag      string
	networkActiveFlag    string
	networkAddressFlag   string
	networkNetmaskFlag   string
	networkGatewayFlag   string
	networkCommentsFlag  string
	networkAutostartFlag bool
	networkVLANAwareFlag bool
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Network management",
	Long:  "Manage network interfaces, SDN zones, virtual networks, and firewall rules",
}

var networkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List network interfaces",
	Long:  "List all network interfaces on a node with optional filtering",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")

		if node == "" {
			return fmt.Errorf("--node flag is required")
		}

		filter := &network.NetworkFilter{
			Node: node,
			Type: networkTypeFlag,
		}

		// Handle active flag (optional boolean)
		if networkActiveFlag != "" {
			switch networkActiveFlag {
			case "true":
				active := true
				filter.Active = &active
			case "false":
				active := false
				filter.Active = &active
			}
		}

		interfaces, err := networkOps.GetNetworkInterfaces(node, filter)
		if err != nil {
			return fmt.Errorf("failed to list network interfaces: %w", err)
		}

		network.DisplayNetworkInterfaces(interfaces)
		return nil
	},
}

var networkSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show network summary",
	Long:  "Display a summary of network interfaces by type",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")

		if node == "" {
			return fmt.Errorf("--node flag is required")
		}

		interfaces, err := networkOps.GetNetworkInterfaces(node, nil)
		if err != nil {
			return fmt.Errorf("failed to get network interfaces: %w", err)
		}

		network.DisplayNetworkSummary(interfaces, node)
		return nil
	},
}

var networkShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show network interface details",
	Long:  "Display detailed information about a specific network interface",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		iface, _ := cmd.Flags().GetString("iface")

		if node == "" || iface == "" {
			return fmt.Errorf("--node and --iface flags are required")
		}

		ifaceDetails, err := networkOps.GetNetworkInterface(node, iface)
		if err != nil {
			return fmt.Errorf("failed to get network interface: %w", err)
		}

		network.DisplayInterfaceDetails(ifaceDetails)
		return nil
	},
}

var networkCreateBridgeCmd = &cobra.Command{
	Use:   "create-bridge",
	Short: "Create a new bridge interface",
	Long:  "Create a new bridge network interface on a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		iface, _ := cmd.Flags().GetString("iface")
		bridgePorts, _ := cmd.Flags().GetString("bridge-ports")

		if node == "" || iface == "" || bridgePorts == "" {
			return fmt.Errorf("--node, --iface, and --bridge-ports flags are required")
		}

		opts := &network.BridgeOptions{
			Node:        node,
			Iface:       iface,
			Type:        "bridge",
			Autostart:   networkAutostartFlag,
			BridgePorts: bridgePorts,
			Address:     networkAddressFlag,
			Netmask:     networkNetmaskFlag,
			Gateway:     networkGatewayFlag,
			Comments:    networkCommentsFlag,
			VLANAware:   networkVLANAwareFlag,
		}

		err := networkOps.CreateBridge(opts)
		if err != nil {
			return fmt.Errorf("failed to create bridge: %w", err)
		}

		fmt.Printf("✅ Bridge %s created successfully on node %s\n", iface, node)
		fmt.Println("⚠️  Network configuration is pending. Run 'network apply' to activate changes.")
		return nil
	},
}

var networkDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a network interface",
	Long:  "Delete a network interface from a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		iface, _ := cmd.Flags().GetString("iface")

		if node == "" || iface == "" {
			return fmt.Errorf("--node and --iface flags are required")
		}

		err := networkOps.DeleteNetworkInterface(node, iface)
		if err != nil {
			return fmt.Errorf("failed to delete network interface: %w", err)
		}

		fmt.Printf("✅ Network interface %s deleted successfully\n", iface)
		fmt.Println("⚠️  Network configuration is pending. Run 'network apply' to activate changes.")
		return nil
	},
}

var networkApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply pending network configuration",
	Long:  "Apply all pending network configuration changes on a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")

		if node == "" {
			return fmt.Errorf("--node flag is required")
		}

		err := networkOps.ApplyNetworkConfig(node)
		if err != nil {
			return fmt.Errorf("failed to apply network configuration: %w", err)
		}

		fmt.Printf("✅ Network configuration applied successfully on node %s\n", node)
		fmt.Println("⚠️  Node networking may be restarted. Check connectivity.")
		return nil
	},
}

var networkRevertCmd = &cobra.Command{
	Use:   "revert",
	Short: "Revert pending network configuration",
	Long:  "Revert all pending network configuration changes on a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")

		if node == "" {
			return fmt.Errorf("--node flag is required")
		}

		err := networkOps.RevertNetworkConfig(node)
		if err != nil {
			return fmt.Errorf("failed to revert network configuration: %w", err)
		}

		fmt.Printf("✅ Pending network configuration reverted on node %s\n", node)
		return nil
	},
}

// SDN commands

var sdnCmd = &cobra.Command{
	Use:   "sdn",
	Short: "Software Defined Network management",
	Long:  "Manage SDN zones, virtual networks, and subnets",
}

var sdnZonesCmd = &cobra.Command{
	Use:   "zones",
	Short: "List SDN zones",
	Long:  "List all SDN zones with optional filtering",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := &network.SDNFilter{
			Type: networkTypeFlag,
		}

		zones, err := networkOps.GetSDNZones(filter)
		if err != nil {
			return fmt.Errorf("failed to list SDN zones: %w", err)
		}

		network.DisplaySDNZones(zones)
		return nil
	},
}

var sdnVnetsCmd = &cobra.Command{
	Use:   "vnets",
	Short: "List SDN virtual networks",
	Long:  "List all SDN virtual networks, optionally filtered by zone",
	RunE: func(cmd *cobra.Command, args []string) error {
		zone, _ := cmd.Flags().GetString("zone")

		vnets, err := networkOps.GetSDNVNets(zone)
		if err != nil {
			return fmt.Errorf("failed to list SDN virtual networks: %w", err)
		}

		network.DisplaySDNVNets(vnets)
		return nil
	},
}

// Firewall commands

var firewallCmd = &cobra.Command{
	Use:   "firewall",
	Short: "Firewall management",
	Long:  "Manage node and cluster firewall rules",
}

var firewallRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List firewall rules",
	Long:  "List all firewall rules for a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")

		if node == "" {
			return fmt.Errorf("--node flag is required")
		}

		rules, err := networkOps.GetFirewallRules(node)
		if err != nil {
			return fmt.Errorf("failed to list firewall rules: %w", err)
		}

		network.DisplayFirewallRules(rules, node)
		return nil
	},
}

func initNetworkCommands() {
	// Note: networkCmd is now added by cmd_cluster.go
	// rootCmd.AddCommand(networkCmd)

	// Main network commands
	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkSummaryCmd)
	networkCmd.AddCommand(networkShowCmd)
	networkCmd.AddCommand(networkCreateBridgeCmd)
	networkCmd.AddCommand(networkDeleteCmd)
	networkCmd.AddCommand(networkApplyCmd)
	networkCmd.AddCommand(networkRevertCmd)

	// SDN commands
	networkCmd.AddCommand(sdnCmd)
	sdnCmd.AddCommand(sdnZonesCmd)
	sdnCmd.AddCommand(sdnVnetsCmd)

	// Firewall commands
	networkCmd.AddCommand(firewallCmd)
	firewallCmd.AddCommand(firewallRulesCmd)

	// Flags for list command
	networkListCmd.Flags().String("node", "", "Node name (required)")
	networkListCmd.Flags().StringVar(&networkTypeFlag, "type", "", "Filter by interface type (bridge, bond, eth, vlan)")
	networkListCmd.Flags().StringVar(&networkActiveFlag, "active", "", "Filter by active status (true/false)")
	_ = networkListCmd.MarkFlagRequired("node")

	// Flags for summary command
	networkSummaryCmd.Flags().String("node", "", "Node name (required)")
	_ = networkSummaryCmd.MarkFlagRequired("node")

	// Flags for show command
	networkShowCmd.Flags().String("node", "", "Node name (required)")
	networkShowCmd.Flags().String("iface", "", "Interface name (required)")
	_ = networkShowCmd.MarkFlagRequired("node")
	_ = networkShowCmd.MarkFlagRequired("iface")

	// Flags for create-bridge command
	networkCreateBridgeCmd.Flags().String("node", "", "Node name (required)")
	networkCreateBridgeCmd.Flags().String("iface", "", "Interface name, e.g., vmbr1 (required)")
	networkCreateBridgeCmd.Flags().String("bridge-ports", "", "Bridge ports, e.g., eth1 (required)")
	networkCreateBridgeCmd.Flags().StringVar(&networkAddressFlag, "address", "", "IP address")
	networkCreateBridgeCmd.Flags().StringVar(&networkNetmaskFlag, "netmask", "", "Network mask")
	networkCreateBridgeCmd.Flags().StringVar(&networkGatewayFlag, "gateway", "", "Gateway")
	networkCreateBridgeCmd.Flags().StringVar(&networkCommentsFlag, "comments", "", "Comments")
	networkCreateBridgeCmd.Flags().BoolVar(&networkAutostartFlag, "autostart", true, "Start on boot")
	networkCreateBridgeCmd.Flags().BoolVar(&networkVLANAwareFlag, "vlan-aware", false, "Enable VLAN awareness")
	_ = networkCreateBridgeCmd.MarkFlagRequired("node")
	_ = networkCreateBridgeCmd.MarkFlagRequired("iface")
	_ = networkCreateBridgeCmd.MarkFlagRequired("bridge-ports")

	// Flags for delete command
	networkDeleteCmd.Flags().String("node", "", "Node name (required)")
	networkDeleteCmd.Flags().String("iface", "", "Interface name (required)")
	_ = networkDeleteCmd.MarkFlagRequired("node")
	_ = networkDeleteCmd.MarkFlagRequired("iface")

	// Flags for apply command
	networkApplyCmd.Flags().String("node", "", "Node name (required)")
	_ = networkApplyCmd.MarkFlagRequired("node")

	// Flags for revert command
	networkRevertCmd.Flags().String("node", "", "Node name (required)")
	_ = networkRevertCmd.MarkFlagRequired("node")

	// Flags for SDN zones command
	sdnZonesCmd.Flags().StringVar(&networkTypeFlag, "type", "", "Filter by zone type (vlan, vxlan, qinq, simple)")

	// Flags for SDN vnets command
	sdnVnetsCmd.Flags().String("zone", "", "Filter by zone name")

	// Flags for firewall rules command
	firewallRulesCmd.Flags().String("node", "", "Node name (required)")
	_ = firewallRulesCmd.MarkFlagRequired("node")
}
