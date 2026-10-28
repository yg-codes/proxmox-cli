package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/node"
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node management commands",
	Long:  "Manage Proxmox cluster nodes, services, and power operations",
}

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all cluster nodes",
	Long:  "Display all nodes in the Proxmox cluster with their status and resource usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodes, err := nodeOps.GetNodes()
		if err != nil {
			return fmt.Errorf("failed to list nodes: %w", err)
		}

		node.DisplayNodes(nodes)
		return nil
	},
}

var nodeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get node status and resource usage",
	Long:  "Display detailed status and resource metrics for a specific node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		if nodeName == "" {
			return fmt.Errorf("--node flag is required")
		}

		nodeStatus, err := nodeOps.GetNodeStatus(nodeName)
		if err != nil {
			return fmt.Errorf("failed to get node status: %w", err)
		}

		node.DisplayNodeStatus(nodeStatus)
		return nil
	},
}

var nodeServicesCmd = &cobra.Command{
	Use:   "services",
	Short: "List node services",
	Long:  "Display all services running on a specific node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		if nodeName == "" {
			return fmt.Errorf("--node flag is required")
		}

		services, err := nodeOps.GetNodeServices(nodeName)
		if err != nil {
			return fmt.Errorf("failed to list services: %w", err)
		}

		node.DisplayServices(nodeName, services)
		return nil
	},
}

var nodeServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage individual service",
	Long:  "Start, stop, restart, or view status of a specific node service",
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a service",
	Long:  "Start a specific service on a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		serviceName, _ := cmd.Flags().GetString("service")

		if nodeName == "" || serviceName == "" {
			return fmt.Errorf("both --node and --service flags are required")
		}

		if dryRun {
			fmt.Printf("🔍 DRY-RUN: Would start service %s on node %s\n", serviceName, nodeName)
			return nil
		}

		if !autoConfirm && !batchMode {
			fmt.Printf("⚠️  Are you sure you want to start service %s on node %s? (yes/no): ", serviceName, nodeName)
			if !confirmAction() {
				fmt.Println("❌ Operation cancelled")
				return nil
			}
		}

		err := nodeOps.StartService(nodeName, serviceName)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Service %s started on node %s\n", serviceName, nodeName)
		return nil
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a service",
	Long:  "Stop a specific service on a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		serviceName, _ := cmd.Flags().GetString("service")

		if nodeName == "" || serviceName == "" {
			return fmt.Errorf("both --node and --service flags are required")
		}

		if dryRun {
			fmt.Printf("🔍 DRY-RUN: Would stop service %s on node %s\n", serviceName, nodeName)
			return nil
		}

		if !autoConfirm && !batchMode {
			fmt.Printf("⚠️  Are you sure you want to stop service %s on node %s? (yes/no): ", serviceName, nodeName)
			if !confirmAction() {
				fmt.Println("❌ Operation cancelled")
				return nil
			}
		}

		err := nodeOps.StopService(nodeName, serviceName)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Service %s stopped on node %s\n", serviceName, nodeName)
		return nil
	},
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart a service",
	Long:  "Restart a specific service on a node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		serviceName, _ := cmd.Flags().GetString("service")

		if nodeName == "" || serviceName == "" {
			return fmt.Errorf("both --node and --service flags are required")
		}

		if dryRun {
			fmt.Printf("🔍 DRY-RUN: Would restart service %s on node %s\n", serviceName, nodeName)
			return nil
		}

		if !autoConfirm && !batchMode {
			fmt.Printf("⚠️  Are you sure you want to restart service %s on node %s? (yes/no): ", serviceName, nodeName)
			if !confirmAction() {
				fmt.Println("❌ Operation cancelled")
				return nil
			}
		}

		err := nodeOps.RestartService(nodeName, serviceName)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Service %s restarted on node %s\n", serviceName, nodeName)
		return nil
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get service status",
	Long:  "Display detailed status of a specific service",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		serviceName, _ := cmd.Flags().GetString("service")

		if nodeName == "" || serviceName == "" {
			return fmt.Errorf("both --node and --service flags are required")
		}

		svc, err := nodeOps.GetServiceStatus(nodeName, serviceName)
		if err != nil {
			return fmt.Errorf("failed to get service status: %w", err)
		}

		node.DisplayServiceStatus(nodeName, svc)
		return nil
	},
}

var nodeRebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot a node",
	Long:  "Reboot a specific Proxmox node (requires confirmation)",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		if nodeName == "" {
			return fmt.Errorf("--node flag is required")
		}

		if dryRun {
			fmt.Printf("🔍 DRY-RUN: Would reboot node %s\n", nodeName)
			return nil
		}

		if !autoConfirm && !batchMode {
			fmt.Printf("⚠️  WARNING: This will reboot node %s!\n", nodeName)
			fmt.Printf("Are you sure you want to continue? (yes/no): ")
			if !confirmAction() {
				fmt.Println("❌ Operation cancelled")
				return nil
			}
		}

		err := nodeOps.RebootNode(nodeName)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Reboot command sent to node %s\n", nodeName)
		return nil
	},
}

var nodeShutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Shutdown a node",
	Long:  "Shutdown a specific Proxmox node (requires confirmation)",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		if nodeName == "" {
			return fmt.Errorf("--node flag is required")
		}

		if dryRun {
			fmt.Printf("🔍 DRY-RUN: Would shutdown node %s\n", nodeName)
			return nil
		}

		if !autoConfirm && !batchMode {
			fmt.Printf("⚠️  WARNING: This will shutdown node %s!\n", nodeName)
			fmt.Printf("Are you sure you want to continue? (yes/no): ")
			if !confirmAction() {
				fmt.Println("❌ Operation cancelled")
				return nil
			}
		}

		err := nodeOps.ShutdownNode(nodeName)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Shutdown command sent to node %s\n", nodeName)
		return nil
	},
}

var nodeVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Get node version information",
	Long:  "Display version information for a specific node",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		if nodeName == "" {
			return fmt.Errorf("--node flag is required")
		}

		version, err := nodeOps.GetNodeVersion(nodeName)
		if err != nil {
			return fmt.Errorf("failed to get node version: %w", err)
		}

		node.DisplayVersionInfo(nodeName, version)
		return nil
	},
}

func initNodeCommands() {
	// Note: nodeCmd is now added by main.go as the top-level 'node' command
	// rootCmd.AddCommand(nodeCmd)

	nodeCmd.AddCommand(nodeListCmd)
	nodeCmd.AddCommand(nodeStatusCmd)
	nodeCmd.AddCommand(nodeServicesCmd)
	nodeCmd.AddCommand(nodeServiceCmd)
	nodeCmd.AddCommand(nodeRebootCmd)
	nodeCmd.AddCommand(nodeShutdownCmd)
	nodeCmd.AddCommand(nodeVersionCmd)

	nodeServiceCmd.AddCommand(serviceStartCmd)
	nodeServiceCmd.AddCommand(serviceStopCmd)
	nodeServiceCmd.AddCommand(serviceRestartCmd)
	nodeServiceCmd.AddCommand(serviceStatusCmd)

	// Flags for node status
	nodeStatusCmd.Flags().String("node", "", "Node name (required)")
	_ = nodeStatusCmd.MarkFlagRequired("node")

	// Flags for node services
	nodeServicesCmd.Flags().String("node", "", "Node name (required)")
	_ = nodeServicesCmd.MarkFlagRequired("node")

	// Flags for service operations
	serviceStartCmd.Flags().String("node", "", "Node name (required)")
	serviceStartCmd.Flags().String("service", "", "Service name (required)")
	_ = serviceStartCmd.MarkFlagRequired("node")
	_ = serviceStartCmd.MarkFlagRequired("service")

	serviceStopCmd.Flags().String("node", "", "Node name (required)")
	serviceStopCmd.Flags().String("service", "", "Service name (required)")
	_ = serviceStopCmd.MarkFlagRequired("node")
	_ = serviceStopCmd.MarkFlagRequired("service")

	serviceRestartCmd.Flags().String("node", "", "Node name (required)")
	serviceRestartCmd.Flags().String("service", "", "Service name (required)")
	_ = serviceRestartCmd.MarkFlagRequired("node")
	_ = serviceRestartCmd.MarkFlagRequired("service")

	serviceStatusCmd.Flags().String("node", "", "Node name (required)")
	serviceStatusCmd.Flags().String("service", "", "Service name (required)")
	_ = serviceStatusCmd.MarkFlagRequired("node")
	_ = serviceStatusCmd.MarkFlagRequired("service")

	// Flags for node reboot
	nodeRebootCmd.Flags().String("node", "", "Node name (required)")
	_ = nodeRebootCmd.MarkFlagRequired("node")

	// Flags for node shutdown
	nodeShutdownCmd.Flags().String("node", "", "Node name (required)")
	_ = nodeShutdownCmd.MarkFlagRequired("node")

	// Flags for node version
	nodeVersionCmd.Flags().String("node", "", "Node name (required)")
	_ = nodeVersionCmd.MarkFlagRequired("node")
}

// Helper function for confirmation (reused from existing code)
func confirmAction() bool {
	var response string
	_, _ = fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "yes" || response == "y"
}
