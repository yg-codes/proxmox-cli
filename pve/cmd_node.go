package main

// initNodeRootCommands sets up the node command hierarchy
// The existing nodeCmd from node.go becomes the top-level 'node' command
func initNodeRootCommands() {
	// Add the existing nodeCmd to root
	rootCmd.AddCommand(nodeCmd)

	// Initialize the node commands (from node.go)
	initNodeCommands()

	// Add resource as subcommand of node
	nodeCmd.AddCommand(resourceCmd)

	// Initialize resource commands
	initResourceCommands()

	// Add disks (ZFS/SMART reporting) as subcommand of node
	nodeCmd.AddCommand(disksCmd)
	initDisksCommands()

	// Add capability preflight as subcommand of node
	initNodeCapabilityCommand()

	// Add APT repository management as subcommand of node
	initNodeAptCommand()
}
