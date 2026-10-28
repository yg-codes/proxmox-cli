package main

// initVMRootCommands sets up the VM command hierarchy
// The existing vmCmd from main.go becomes the top-level 'vm' command
func initVMRootCommands() {
	// Add the existing vmCmd to root
	rootCmd.AddCommand(vmCmd)

	// Add direct VM commands (list, details, start, stop, shutdown)
	vmCmd.AddCommand(vmListCmd)
	vmListCmd.Flags().StringVar(&vmStatusFlag, "status", "", "Filter by status: running, stopped")
	vmCmd.AddCommand(vmDetailsCmd)
	vmCmd.AddCommand(vmStartCmd)
	vmCmd.AddCommand(vmStopCmd)
	vmCmd.AddCommand(vmShutdownCmd)
	vmCmd.AddCommand(vmRebootCmd)
	vmCmd.AddCommand(vmResetCmd)
	vmCmd.AddCommand(vmSuspendCmd)
	vmCmd.AddCommand(vmResumeCmd)

	// Add config as subcommand of vm and initialize its subcommands
	vmCmd.AddCommand(vmConfigCmd)
	vmConfigCmd.AddCommand(vmConfigGetCmd)
	vmConfigCmd.AddCommand(vmConfigSetCmd)
	vmConfigCmd.AddCommand(vmConfigPendingCmd)
	// Provisioning commands: clone, migrate, resize, move-disk, template
	vmCmd.AddCommand(vmCloneCmd)
	vmCmd.AddCommand(vmMigrateCmd)
	vmCmd.AddCommand(vmResizeCmd)
	vmCmd.AddCommand(vmMoveDiskCmd)
	vmCmd.AddCommand(vmTemplateCmd)
	vmCmd.AddCommand(vmCreateCmd)
	vmCmd.AddCommand(vmDestroyCmd)
	vmCmd.AddCommand(vmAgentCmd)
	vmCmd.AddCommand(vmCloudInitCmd)
	vmCmd.AddCommand(vmFirewallCmd)

	initVMLifecycleFlags()
	initVMConfigFlags()
	initVMProvisioningFlags()
	initVMCreateFlags()
	initVMAgentCommands()
	initVMCloudInitCommands()
	initVMFirewallCommands()

	// Add snapshot as subcommand of vm and initialize its subcommands
	vmCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotRollbackCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)
	initSnapshotClusterCommands()

	// Add backup as subcommand of vm and initialize its subcommands
	vmCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupDeleteCmd)

	// Add bulk operations as subcommand of vm
	vmCmd.AddCommand(bulkCmd)
	initBulkCommands()
}
