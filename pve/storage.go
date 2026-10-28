package main

import (
	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/storage"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage and list storage resources",
	Long: `List and manage Proxmox storage resources.

Storage operations allow you to:
- List backup-capable storages
- List VM disk storages
- Check storage status and capacity`,
}

var listBackupStoragesCmd = &cobra.Command{
	Use:   "list-backup",
	Short: "List backup-capable storages",
	Long: `List all storages that support backup operations.

Shows storage name, type, status, available space, and total space.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create storage operations
		storageOps := storage.NewOperations(client, logger)

		// Display backup storages
		return storageOps.DisplayBackupStorages()
	},
}

var listVMStoragesCmd = &cobra.Command{
	Use:   "list-vm",
	Short: "List VM disk storages",
	Long: `List all storages that support VM disk images.

Shows storage name, type, status, content types, and available space.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create storage operations
		storageOps := storage.NewOperations(client, logger)

		// Display VM storages
		return storageOps.DisplayVMStorages()
	},
}

func initStorageCommands() {
	// Add storage subcommands
	storageCmd.AddCommand(listBackupStoragesCmd)
	storageCmd.AddCommand(listVMStoragesCmd)

	// Note: storageCmd is now added by cmd_cluster.go
	// rootCmd.AddCommand(storageCmd)
}
