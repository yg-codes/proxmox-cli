package main

import (
	"github.com/spf13/cobra"
)

// clusterCmd represents the cluster command group
var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage Proxmox cluster resources",
	Long: `Manage cluster-wide resources including tasks, storage, and networking.

Examples:
  pve cluster task list
  pve cluster storage list-backup
  pve cluster network list --node pve1`,
}

func initClusterCommands() {
	rootCmd.AddCommand(clusterCmd)

	// Add task as subcommand of cluster
	clusterCmd.AddCommand(taskCmd)
	initTaskCommands()

	// Add storage as subcommand of cluster
	clusterCmd.AddCommand(storageCmd)
	initStorageCommands()

	// Add network as subcommand of cluster
	clusterCmd.AddCommand(networkCmd)
	initNetworkCommands()

	// Add health as subcommand of cluster
	initClusterHealthCommand()

	// Add backup-job (vzdump job CRUD) as subcommand of cluster
	initClusterBackupJobCommands()
}
