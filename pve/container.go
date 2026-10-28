package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/container"
)

var (
	// Container-specific flags
	containerNodeFlag     string
	containerStatusFlag   string
	containerNameFlag     string
	containerTemplateFlag bool
)

var containerCmd = &cobra.Command{
	Use:   "container",
	Short: "LXC container management",
	Long:  "Manage LXC containers including creation, lifecycle, snapshots, and cloning",
}

var containerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List LXC containers",
	Long:  "List all LXC containers in the cluster with optional filtering",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := &container.ContainerFilter{
			Node:     containerNodeFlag,
			Status:   containerStatusFlag,
			Name:     containerNameFlag,
			Template: containerTemplateFlag,
		}

		containers, err := containerOps.GetContainers(filter)
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}

		container.DisplayContainers(containers)
		return nil
	},
}

var containerSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show container summary",
	Long:  "Display summary statistics for all containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		containers, err := containerOps.GetContainers(nil)
		if err != nil {
			return fmt.Errorf("failed to get containers: %w", err)
		}

		container.DisplayContainerSummary(containers)
		return nil
	},
}

var containerShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show container details",
	Long:  "Display detailed information about a specific container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		ct, err := containerOps.GetContainer(node, vmid)
		if err != nil {
			return fmt.Errorf("failed to get container: %w", err)
		}

		container.DisplayContainerDetails(ct)
		return nil
	},
}

var containerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show container status",
	Long:  "Display current status and resource usage for a container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		status, err := containerOps.GetContainerStatus(node, vmid)
		if err != nil {
			return fmt.Errorf("failed to get container status: %w", err)
		}

		container.DisplayContainerStatus(status)
		return nil
	},
}

var containerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new container",
	Long:  "Create a new LXC container from an OS template",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		ostemplate, _ := cmd.Flags().GetString("ostemplate")
		hostname, _ := cmd.Flags().GetString("hostname")
		storage, _ := cmd.Flags().GetString("storage")

		if node == "" || vmid == 0 || ostemplate == "" || storage == "" {
			return fmt.Errorf("--node, --vmid, --ostemplate, and --storage flags are required")
		}

		config := &container.ContainerConfig{
			VMID:       vmid,
			Node:       node,
			OSTemplate: ostemplate,
			Hostname:   hostname,
			Storage:    storage,
		}

		// Optional flags
		if desc, _ := cmd.Flags().GetString("description"); desc != "" {
			config.Description = desc
		}
		if password, _ := cmd.Flags().GetString("password"); password != "" {
			config.Password = password
		}
		if sshkeys, _ := cmd.Flags().GetString("ssh-keys"); sshkeys != "" {
			config.SSHKeys = sshkeys
		}

		// Resource flags
		if cpus, _ := cmd.Flags().GetInt("cpus"); cpus > 0 {
			config.CPUs = cpus
		}
		if memory, _ := cmd.Flags().GetInt64("memory"); memory > 0 {
			config.Memory = memory * 1024 * 1024 // Convert MB to bytes
		}
		if swap, _ := cmd.Flags().GetInt64("swap"); swap > 0 {
			config.Swap = swap * 1024 * 1024
		}

		// Feature flags
		config.Nesting, _ = cmd.Flags().GetBool("nesting")
		config.Unprivileged, _ = cmd.Flags().GetBool("unprivileged")
		config.OnBoot, _ = cmd.Flags().GetBool("onboot")
		config.Protected, _ = cmd.Flags().GetBool("protected")

		taskID, err := containerOps.CreateContainer(config)
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}

		fmt.Printf("✅ Container creation initiated (Task ID: %s)\n", taskID)
		fmt.Printf("Use 'task show --taskid %s' to monitor progress\n", taskID)
		return nil
	},
}

var containerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a container",
	Long:  "Start a stopped container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		taskID, err := containerOps.StartContainer(node, vmid)
		if err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		fmt.Printf("✅ Container %d start initiated (Task ID: %s)\n", vmid, taskID)
		return nil
	},
}

var containerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a container",
	Long:  "Forcefully stop a running container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		taskID, err := containerOps.StopContainer(node, vmid)
		if err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}

		fmt.Printf("✅ Container %d stop initiated (Task ID: %s)\n", vmid, taskID)
		return nil
	},
}

var containerShutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Shutdown a container",
	Long:  "Gracefully shutdown a running container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		timeout, _ := cmd.Flags().GetInt("timeout")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		taskID, err := containerOps.ShutdownContainer(node, vmid, timeout)
		if err != nil {
			return fmt.Errorf("failed to shutdown container: %w", err)
		}

		fmt.Printf("✅ Container %d shutdown initiated (Task ID: %s)\n", vmid, taskID)
		return nil
	},
}

var containerRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart a container",
	Long:  "Restart a running container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		taskID, err := containerOps.RestartContainer(node, vmid)
		if err != nil {
			return fmt.Errorf("failed to restart container: %w", err)
		}

		fmt.Printf("✅ Container %d restart initiated (Task ID: %s)\n", vmid, taskID)
		return nil
	},
}

var containerDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a container",
	Long:  "Delete a container and optionally purge its data",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		purge, _ := cmd.Flags().GetBool("purge")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		taskID, err := containerOps.DeleteContainer(node, vmid, purge)
		if err != nil {
			return fmt.Errorf("failed to delete container: %w", err)
		}

		fmt.Printf("✅ Container %d deletion initiated (Task ID: %s)\n", vmid, taskID)
		return nil
	},
}

var containerCloneCmd = &cobra.Command{
	Use:   "clone",
	Short: "Clone a container",
	Long:  "Create a clone of an existing container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		newid, _ := cmd.Flags().GetInt("newid")

		if node == "" || vmid == 0 || newid == 0 {
			return fmt.Errorf("--node, --vmid, and --newid flags are required")
		}

		options := &container.ContainerCloneOptions{
			NewID: newid,
		}

		if hostname, _ := cmd.Flags().GetString("hostname"); hostname != "" {
			options.Hostname = hostname
		}
		if desc, _ := cmd.Flags().GetString("description"); desc != "" {
			options.Description = desc
		}
		if storage, _ := cmd.Flags().GetString("storage"); storage != "" {
			options.Storage = storage
		}
		if target, _ := cmd.Flags().GetString("target"); target != "" {
			options.Target = target
		}
		if snapshot, _ := cmd.Flags().GetString("snapshot"); snapshot != "" {
			options.SnapName = snapshot
		}

		options.Full, _ = cmd.Flags().GetBool("full")

		taskID, err := containerOps.CloneContainer(node, vmid, options)
		if err != nil {
			return fmt.Errorf("failed to clone container: %w", err)
		}

		fmt.Printf("✅ Container %d clone initiated to %d (Task ID: %s)\n", vmid, newid, taskID)
		return nil
	},
}

var containerSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage container snapshots",
	Long:  "Create, list, rollback, and delete container snapshots",
}

var containerSnapshotCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a snapshot",
	Long:  "Create a snapshot of a container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		snapname, _ := cmd.Flags().GetString("name")

		if node == "" || vmid == 0 || snapname == "" {
			return fmt.Errorf("--node, --vmid, and --name flags are required")
		}

		desc, _ := cmd.Flags().GetString("description")

		taskID, err := containerOps.CreateSnapshot(node, vmid, snapname, desc)
		if err != nil {
			return fmt.Errorf("failed to create snapshot: %w", err)
		}

		fmt.Printf("✅ Snapshot '%s' creation initiated (Task ID: %s)\n", snapname, taskID)
		return nil
	},
}

var containerSnapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List snapshots",
	Long:  "List all snapshots for a container",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")

		if node == "" || vmid == 0 {
			return fmt.Errorf("--node and --vmid flags are required")
		}

		snapshots, err := containerOps.ListSnapshots(node, vmid)
		if err != nil {
			return fmt.Errorf("failed to list snapshots: %w", err)
		}

		container.DisplaySnapshots(vmid, snapshots)
		return nil
	},
}

var containerSnapshotRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback to snapshot",
	Long:  "Rollback a container to a specific snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		snapname, _ := cmd.Flags().GetString("name")

		if node == "" || vmid == 0 || snapname == "" {
			return fmt.Errorf("--node, --vmid, and --name flags are required")
		}

		taskID, err := containerOps.RollbackSnapshot(node, vmid, snapname)
		if err != nil {
			return fmt.Errorf("failed to rollback snapshot: %w", err)
		}

		fmt.Printf("✅ Rollback to snapshot '%s' initiated (Task ID: %s)\n", snapname, taskID)
		return nil
	},
}

var containerSnapshotDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a snapshot",
	Long:  "Delete a specific snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		vmid, _ := cmd.Flags().GetInt("vmid")
		snapname, _ := cmd.Flags().GetString("name")

		if node == "" || vmid == 0 || snapname == "" {
			return fmt.Errorf("--node, --vmid, and --name flags are required")
		}

		taskID, err := containerOps.DeleteSnapshot(node, vmid, snapname)
		if err != nil {
			return fmt.Errorf("failed to delete snapshot: %w", err)
		}

		fmt.Printf("✅ Snapshot '%s' deletion initiated (Task ID: %s)\n", snapname, taskID)
		return nil
	},
}

func initContainerCommands() {
	rootCmd.AddCommand(containerCmd)

	containerCmd.AddCommand(containerListCmd)
	containerCmd.AddCommand(containerSummaryCmd)
	containerCmd.AddCommand(containerShowCmd)
	containerCmd.AddCommand(containerStatusCmd)
	containerCmd.AddCommand(containerCreateCmd)
	containerCmd.AddCommand(containerStartCmd)
	containerCmd.AddCommand(containerStopCmd)
	containerCmd.AddCommand(containerShutdownCmd)
	containerCmd.AddCommand(containerRestartCmd)
	containerCmd.AddCommand(containerDeleteCmd)
	containerCmd.AddCommand(containerCloneCmd)
	containerCmd.AddCommand(containerSnapshotCmd)

	// Snapshot subcommands
	containerSnapshotCmd.AddCommand(containerSnapshotCreateCmd)
	containerSnapshotCmd.AddCommand(containerSnapshotListCmd)
	containerSnapshotCmd.AddCommand(containerSnapshotRollbackCmd)
	containerSnapshotCmd.AddCommand(containerSnapshotDeleteCmd)

	// List command flags
	containerListCmd.Flags().StringVar(&containerNodeFlag, "node", "", "Filter by node")
	containerListCmd.Flags().StringVar(&containerStatusFlag, "status", "", "Filter by status (running, stopped)")
	containerListCmd.Flags().StringVar(&containerNameFlag, "name", "", "Filter by name")
	containerListCmd.Flags().BoolVar(&containerTemplateFlag, "template", false, "Show only templates")

	// Show command flags
	containerShowCmd.Flags().String("node", "", "Node name (required)")
	containerShowCmd.Flags().Int("vmid", 0, "Container ID (required)")
	_ = containerShowCmd.MarkFlagRequired("node")
	_ = containerShowCmd.MarkFlagRequired("vmid")

	// Status command flags
	containerStatusCmd.Flags().String("node", "", "Node name (required)")
	containerStatusCmd.Flags().Int("vmid", 0, "Container ID (required)")
	_ = containerStatusCmd.MarkFlagRequired("node")
	_ = containerStatusCmd.MarkFlagRequired("vmid")

	// Create command flags
	containerCreateCmd.Flags().String("node", "", "Node name (required)")
	containerCreateCmd.Flags().Int("vmid", 0, "Container ID (required)")
	containerCreateCmd.Flags().String("ostemplate", "", "OS template (required)")
	containerCreateCmd.Flags().String("hostname", "", "Hostname")
	containerCreateCmd.Flags().String("storage", "", "Storage location (required)")
	containerCreateCmd.Flags().String("description", "", "Description")
	containerCreateCmd.Flags().String("password", "", "Root password")
	containerCreateCmd.Flags().String("ssh-keys", "", "SSH public keys")
	containerCreateCmd.Flags().Int("cpus", 0, "Number of CPU cores")
	containerCreateCmd.Flags().Int64("memory", 0, "Memory in MB")
	containerCreateCmd.Flags().Int64("swap", 0, "Swap in MB")
	containerCreateCmd.Flags().Bool("nesting", false, "Enable nesting")
	containerCreateCmd.Flags().Bool("unprivileged", false, "Create as unprivileged container")
	containerCreateCmd.Flags().Bool("onboot", false, "Start on boot")
	containerCreateCmd.Flags().Bool("protected", false, "Protection flag")
	_ = containerCreateCmd.MarkFlagRequired("node")
	_ = containerCreateCmd.MarkFlagRequired("vmid")
	_ = containerCreateCmd.MarkFlagRequired("ostemplate")
	_ = containerCreateCmd.MarkFlagRequired("storage")

	// Start/Stop/Restart/Delete command flags
	for _, cmd := range []*cobra.Command{containerStartCmd, containerStopCmd, containerRestartCmd, containerDeleteCmd} {
		cmd.Flags().String("node", "", "Node name (required)")
		cmd.Flags().Int("vmid", 0, "Container ID (required)")
		_ = cmd.MarkFlagRequired("node")
		_ = cmd.MarkFlagRequired("vmid")
	}

	// Shutdown specific flags
	containerShutdownCmd.Flags().String("node", "", "Node name (required)")
	containerShutdownCmd.Flags().Int("vmid", 0, "Container ID (required)")
	containerShutdownCmd.Flags().Int("timeout", 60, "Shutdown timeout in seconds")
	_ = containerShutdownCmd.MarkFlagRequired("node")
	_ = containerShutdownCmd.MarkFlagRequired("vmid")

	// Delete specific flags
	containerDeleteCmd.Flags().Bool("purge", false, "Purge container data")

	// Clone command flags
	containerCloneCmd.Flags().String("node", "", "Source node name (required)")
	containerCloneCmd.Flags().Int("vmid", 0, "Source container ID (required)")
	containerCloneCmd.Flags().Int("newid", 0, "New container ID (required)")
	containerCloneCmd.Flags().String("hostname", "", "New hostname")
	containerCloneCmd.Flags().String("description", "", "Description")
	containerCloneCmd.Flags().String("storage", "", "Target storage")
	containerCloneCmd.Flags().String("target", "", "Target node")
	containerCloneCmd.Flags().String("snapshot", "", "Snapshot name to clone from")
	containerCloneCmd.Flags().Bool("full", false, "Create full clone (default is linked)")
	_ = containerCloneCmd.MarkFlagRequired("node")
	_ = containerCloneCmd.MarkFlagRequired("vmid")
	_ = containerCloneCmd.MarkFlagRequired("newid")

	// Snapshot create flags
	containerSnapshotCreateCmd.Flags().String("node", "", "Node name (required)")
	containerSnapshotCreateCmd.Flags().Int("vmid", 0, "Container ID (required)")
	containerSnapshotCreateCmd.Flags().String("name", "", "Snapshot name (required)")
	containerSnapshotCreateCmd.Flags().String("description", "", "Description")
	_ = containerSnapshotCreateCmd.MarkFlagRequired("node")
	_ = containerSnapshotCreateCmd.MarkFlagRequired("vmid")
	_ = containerSnapshotCreateCmd.MarkFlagRequired("name")

	// Snapshot list flags
	containerSnapshotListCmd.Flags().String("node", "", "Node name (required)")
	containerSnapshotListCmd.Flags().Int("vmid", 0, "Container ID (required)")
	_ = containerSnapshotListCmd.MarkFlagRequired("node")
	_ = containerSnapshotListCmd.MarkFlagRequired("vmid")

	// Snapshot rollback flags
	containerSnapshotRollbackCmd.Flags().String("node", "", "Node name (required)")
	containerSnapshotRollbackCmd.Flags().Int("vmid", 0, "Container ID (required)")
	containerSnapshotRollbackCmd.Flags().String("name", "", "Snapshot name (required)")
	_ = containerSnapshotRollbackCmd.MarkFlagRequired("node")
	_ = containerSnapshotRollbackCmd.MarkFlagRequired("vmid")
	_ = containerSnapshotRollbackCmd.MarkFlagRequired("name")

	// Snapshot delete flags
	containerSnapshotDeleteCmd.Flags().String("node", "", "Node name (required)")
	containerSnapshotDeleteCmd.Flags().Int("vmid", 0, "Container ID (required)")
	containerSnapshotDeleteCmd.Flags().String("name", "", "Snapshot name (required)")
	_ = containerSnapshotDeleteCmd.MarkFlagRequired("node")
	_ = containerSnapshotDeleteCmd.MarkFlagRequired("vmid")
	_ = containerSnapshotDeleteCmd.MarkFlagRequired("name")
}
