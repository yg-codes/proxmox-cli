package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/disks"
	"github.com/yg-codes/proxmox-cli/pkg/storage"
)

// zfsCmd groups every ZFS operation the Proxmox REST API exposes.
var zfsCmd = &cobra.Command{
	Use:   "zfs",
	Short: "Manage ZFS pools, ZFS storages and replication",
	Long: `Manage ZFS through the Proxmox REST API.

What the API covers, and what it does not, is worth stating plainly — the gap is
in Proxmox, not in this CLI:

  Covered here            pool create/destroy, pool listing and status,
                          zfspool storage CRUD, replication jobs (which are
                          ZFS send/recv under the hood)

  NOT in the REST API     zpool scrub (trigger), import/export, vdev add /
                          attach / detach / replace / offline / online, trim,
                          upgrade, dataset create/destroy, zfs get/set
                          properties, dataset snapshots, raw send/recv, ARC
                          tuning

Those uncovered operations have no API endpoint at all, so no API client can
perform them — they need zpool/zfs on the node itself. Scrub *status* IS
available, via 'pve zfs pool status'.

Examples:
  pve zfs pool list
  pve zfs pool status --node pve1 --pool rpool
  pve zfs pool create --node pve1 --name tank --raid-level mirror --devices /dev/sdb,/dev/sdc
  pve zfs storage list
  pve zfs replication list`,
}

// ----- pool ----------------------------------------------------------------

var zfsPoolCmd = &cobra.Command{
	Use:   "pool",
	Short: "Create, destroy and inspect ZFS pools",
}

var zfsPoolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ZFS pools across the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		pools, err := disksOps.GetZFSPools(node)
		if err != nil {
			return err
		}
		disks.DisplayZFSPools(pools)
		return nil
	},
}

var zfsPoolStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show a pool's vdev tree, health, errors and last scrub",
	Long: `Show one ZFS pool's state in detail.

This includes the last scrub result, which is the closest the API gets to scrub
management — there is no endpoint to trigger one. Proxmox runs a monthly scrub
from cron on the node instead.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		pool, _ := cmd.Flags().GetString("pool")
		if node == "" || pool == "" {
			return fmt.Errorf("both --node and --pool are required")
		}
		detail, err := disksOps.GetZFSPoolDetail(node, pool)
		if err != nil {
			return err
		}
		disks.DisplayZFSPoolDetail(detail)
		return nil
	},
}

var zfsPoolCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a ZFS pool from block devices (destroys their contents)",
	Long: `Create a ZFS pool on a node.

THE NAMED DEVICES ARE WIPED. Everything on them is lost.

Raid levels and their minimum device counts:

  single  1     one disk, no redundancy
  mirror  2     n-way mirror
  raid10  4     striped mirrors — needs an EVEN device count
  raidz   3     single parity
  raidz2  4     double parity
  raidz3  5     triple parity
  draid   3     distributed-spare raidz, single parity
  draid2  4     distributed-spare, double parity
  draid3  5     distributed-spare, triple parity

ashift cannot be changed after creation. The default of 12 (4 KiB sectors) is
correct for effectively all modern disks; set it only if you know why.

Examples:
  pve zfs pool create --node pve1 --name tank --raid-level mirror \
    --devices /dev/sdb,/dev/sdc

  # Register it as a PVE storage at the same time
  pve zfs pool create --node pve1 --name tank --raid-level raidz2 \
    --devices /dev/sdb,/dev/sdc,/dev/sdd,/dev/sde --add-storage

  # dRAID with two distributed spares
  pve zfs pool create --node pve1 --name tank --raid-level draid2 \
    --devices /dev/sd{b,c,d,e,f,g} --draid-data 4 --draid-spares 2`,
	RunE: runZFSPoolCreate,
}

var zfsPoolDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a ZFS pool (irreversible)",
	Long: `Destroy a ZFS pool and everything stored on it.

THIS CANNOT BE UNDONE. Every dataset, zvol and snapshot in the pool goes with
it, including VM disks backed by it.

By default the member disks keep their ZFS labels, which prevents reusing them
in a new pool; --cleanup-disks wipes them. --cleanup-config removes the PVE
storage entries that referenced the pool, which otherwise remain pointing at
something that no longer exists.

Confirmation requires typing the pool name back, not y/N.`,
	RunE: runZFSPoolDestroy,
}

// ----- storage -------------------------------------------------------------

var zfsStorageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage zfspool storage entries",
}

var zfsStorageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured zfspool storages",
	RunE: func(cmd *cobra.Command, args []string) error {
		ops := storage.NewOperations(client, logger)
		entries, err := ops.ListZFSStorages()
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("No zfspool storages configured")
			return nil
		}
		fmt.Printf("\n%-16s %-22s %-22s %-10s %s\n", "STORAGE", "POOL", "CONTENT", "NODES", "STATE")
		fmt.Println(strings.Repeat("-", 88))
		for _, e := range entries {
			state := "active"
			if v, ok := e["disable"]; ok && isTruthy(v) {
				state = "disabled"
			}
			fmt.Printf("%-16s %-22s %-22s %-10s %s\n",
				str(e["storage"]), str(e["pool"]), str(e["content"]),
				strOr(e["nodes"], "all"), state)
		}
		fmt.Println(strings.Repeat("-", 88))
		fmt.Printf("Total: %d\n", len(entries))
		return nil
	},
}

var zfsStorageCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Register a ZFS pool or dataset as a PVE storage",
	Long: `Add a zfspool storage entry backed by an existing ZFS pool or dataset.

This does not create the pool — use 'pve zfs pool create' for that, or
--add-storage there to do both in one step.

Examples:
  pve zfs storage create --storage tank-vms --pool tank/vms --content images,rootdir
  pve zfs storage create --storage tank-vms --pool tank/vms --sparse --blocksize 16k`,
	RunE: runZFSStorageCreate,
}

var zfsStorageUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change a zfspool storage's settings",
	Long: `Update a zfspool storage.

The backing pool cannot be changed — Proxmox declares it fixed, so this command
does not send it. Recreate the storage entry if the pool must change.`,
	RunE: runZFSStorageUpdate,
}

var zfsStorageDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove a storage entry (the ZFS pool and its data are kept)",
	Long: `Unregister a storage from the cluster configuration.

This removes only the PVE storage entry. The ZFS pool, its datasets and all
data survive — use 'pve zfs pool destroy' to remove those.`,
	RunE: runZFSStorageDelete,
}

func initZFSCommands() {
	rootCmd.AddCommand(zfsCmd)

	zfsCmd.AddCommand(zfsPoolCmd)
	zfsPoolCmd.AddCommand(zfsPoolListCmd, zfsPoolStatusCmd, zfsPoolCreateCmd, zfsPoolDestroyCmd)

	zfsPoolListCmd.Flags().String("node", "", "Only show pools on this node")
	zfsPoolStatusCmd.Flags().String("node", "", "Node the pool is on (required)")
	zfsPoolStatusCmd.Flags().String("pool", "", "Pool name (required)")

	zfsPoolCreateCmd.Flags().String("node", "", "Node to create the pool on (required)")
	zfsPoolCreateCmd.Flags().String("name", "", "Pool name (required)")
	zfsPoolCreateCmd.Flags().StringSlice("devices", nil, "Block devices to use (required, comma-separated)")
	zfsPoolCreateCmd.Flags().String("raid-level", "", "single|mirror|raid10|raidz|raidz2|raidz3|draid|draid2|draid3 (required)")
	zfsPoolCreateCmd.Flags().Int("ashift", 0, "Sector size exponent 9-16 (default 12; cannot be changed later)")
	zfsPoolCreateCmd.Flags().String("compression", "", "on|off|gzip|lz4|lzjb|zle|zstd (default on)")
	zfsPoolCreateCmd.Flags().Bool("add-storage", false, "Also register the pool as a PVE storage")
	zfsPoolCreateCmd.Flags().Int("draid-data", 0, "Data devices per redundancy group (draid levels only)")
	zfsPoolCreateCmd.Flags().Int("draid-spares", 0, "Distributed spares (draid levels only)")

	zfsPoolDestroyCmd.Flags().String("node", "", "Node the pool is on (required)")
	zfsPoolDestroyCmd.Flags().String("name", "", "Pool name (required)")
	zfsPoolDestroyCmd.Flags().Bool("cleanup-config", false, "Also remove PVE storage entries referencing this pool")
	zfsPoolDestroyCmd.Flags().Bool("cleanup-disks", false, "Also wipe the member disks so they can be reused")
	zfsPoolDestroyCmd.Flags().Bool("force", false, "Required alongside --yes for non-interactive destroy")

	zfsCmd.AddCommand(zfsStorageCmd)
	zfsStorageCmd.AddCommand(zfsStorageListCmd, zfsStorageCreateCmd, zfsStorageUpdateCmd, zfsStorageDeleteCmd)

	for _, c := range []*cobra.Command{zfsStorageCreateCmd, zfsStorageUpdateCmd} {
		c.Flags().String("storage", "", "Storage ID (required)")
		c.Flags().String("content", "", "Content types, e.g. images,rootdir")
		c.Flags().String("nodes", "", "Restrict to these nodes (default: all)")
		c.Flags().String("blocksize", "", "volblocksize for new zvols, e.g. 16k")
		c.Flags().Bool("sparse", false, "Thin-provision new zvols")
		c.Flags().Bool("disable", false, "Mark the storage inactive")
		c.Flags().String("mountpoint", "", "Override the dataset mountpoint")
		c.Flags().String("bwlimit", "", "Bandwidth limit for migration/restore")
	}
	zfsStorageCreateCmd.Flags().String("pool", "", "Backing ZFS pool or dataset (required)")
	zfsStorageUpdateCmd.Flags().StringSlice("delete", nil, "Settings to reset to their default")
	zfsStorageDeleteCmd.Flags().String("storage", "", "Storage ID (required)")
}

func runZFSPoolCreate(cmd *cobra.Command, args []string) error {
	node, _ := cmd.Flags().GetString("node")
	if node == "" {
		return fmt.Errorf("--node must be specified")
	}

	opts := &disks.CreatePoolOptions{}
	opts.Name, _ = cmd.Flags().GetString("name")
	opts.Devices, _ = cmd.Flags().GetStringSlice("devices")
	opts.RaidLevel, _ = cmd.Flags().GetString("raid-level")
	opts.Ashift, _ = cmd.Flags().GetInt("ashift")
	opts.Compression, _ = cmd.Flags().GetString("compression")
	opts.AddStorage, _ = cmd.Flags().GetBool("add-storage")
	opts.DraidData, _ = cmd.Flags().GetInt("draid-data")
	opts.DraidSpares, _ = cmd.Flags().GetInt("draid-spares")

	// Validate before printing the plan, so an impossible request is rejected
	// with a reason rather than shown as if it would work.
	if err := disks.ValidateCreatePool(opts); err != nil {
		return err
	}

	fmt.Printf("Create ZFS pool %q on node %s:\n\n", opts.Name, node)
	fmt.Printf("  raid level:  %s\n", opts.RaidLevel)
	fmt.Printf("  devices:     %s\n", strings.Join(opts.Devices, ", "))
	if opts.Ashift > 0 {
		fmt.Printf("  ashift:      %d\n", opts.Ashift)
	}
	if opts.Compression != "" {
		fmt.Printf("  compression: %s\n", opts.Compression)
	}
	if opts.DraidData > 0 || opts.DraidSpares > 0 {
		fmt.Printf("  draid:       data=%d spares=%d\n", opts.DraidData, opts.DraidSpares)
	}
	if opts.AddStorage {
		fmt.Printf("  storage:     also registered as a PVE storage\n")
	}
	fmt.Println()
	fmt.Printf("  ⚠️  ALL DATA on %s will be destroyed.\n\n", strings.Join(opts.Devices, ", "))

	if dryRun {
		printDryRunHeader()
		printDryRunAction("zfs pool create", opts.Name, "",
			fmt.Sprintf("%s across %d device(s) on %s", opts.RaidLevel, len(opts.Devices), node))
		printDryRunSummary("zfs pool create", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Create pool %q, wiping %d device(s)?", opts.Name, len(opts.Devices))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	upid, err := disksOps.CreatePool(node, opts)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Create of pool %q started (task %s)\n", opts.Name, upid)
	return nil
}

func runZFSPoolDestroy(cmd *cobra.Command, args []string) error {
	node, _ := cmd.Flags().GetString("node")
	name, _ := cmd.Flags().GetString("name")
	if node == "" || name == "" {
		return fmt.Errorf("both --node and --name are required")
	}

	opts := &disks.DeletePoolOptions{}
	opts.CleanupConfig, _ = cmd.Flags().GetBool("cleanup-config")
	opts.CleanupDisks, _ = cmd.Flags().GetBool("cleanup-disks")

	// Read the pool first so the operator sees what they are destroying, and so
	// a typo in the name fails before any confirmation is asked for.
	detail, err := disksOps.GetZFSPoolDetail(node, name)
	if err != nil {
		return fmt.Errorf("could not read pool %q on %s: %w", name, node, err)
	}

	fmt.Printf("Destroy ZFS pool %q on node %s\n\n", name, node)
	fmt.Printf("  state:  %s\n", detail.State)
	if detail.Scan != "" {
		fmt.Printf("  scan:   %s\n", detail.Scan)
	}
	fmt.Println()
	fmt.Println("  This is IRREVERSIBLE. Every dataset, zvol and snapshot in the pool")
	fmt.Println("  is destroyed, including any VM disks stored on it.")
	fmt.Println()
	if opts.CleanupDisks {
		fmt.Println("  --cleanup-disks: member disks are wiped and become reusable")
	} else {
		fmt.Println("  member disks keep their ZFS labels (pass --cleanup-disks to wipe them)")
	}
	if opts.CleanupConfig {
		fmt.Println("  --cleanup-config: PVE storage entries for this pool are removed")
	} else {
		fmt.Println("  ⚠️  PVE storage entries referencing this pool are KEPT and will be broken")
	}
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("zfs pool destroy", name, "", "on node "+node)
		printDryRunSummary("zfs pool destroy", 1)
		return nil
	}

	force, _ := cmd.Flags().GetBool("force")
	if cfg.IsAutoConfirm() || batchMode {
		if !force {
			return fmt.Errorf("refusing to destroy pool %q non-interactively without --force\n"+
				"  pve zfs pool destroy --node %s --name %s --yes --force", name, node, name)
		}
		logger.Warnf("Destroying ZFS pool %s on %s non-interactively (--yes --force)", name, node)
	} else {
		fmt.Printf("Type the pool name %q to confirm destruction (anything else aborts): ", name)
		var typed string
		_, _ = fmt.Scanln(&typed)
		if strings.TrimSpace(typed) != name {
			fmt.Printf("Got %q, expected %q — aborting.\n", strings.TrimSpace(typed), name)
			return nil
		}
	}

	upid, err := disksOps.DeletePool(node, name, opts)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Destroy of pool %q started (task %s)\n", name, upid)
	return nil
}

func zfsStorageOptsFromFlags(cmd *cobra.Command) *storage.ZFSStorageOptions {
	opts := &storage.ZFSStorageOptions{}
	opts.Storage, _ = cmd.Flags().GetString("storage")
	opts.Content, _ = cmd.Flags().GetString("content")
	opts.Nodes, _ = cmd.Flags().GetString("nodes")
	opts.Blocksize, _ = cmd.Flags().GetString("blocksize")
	opts.Mountpoint, _ = cmd.Flags().GetString("mountpoint")
	opts.BWLimit, _ = cmd.Flags().GetString("bwlimit")
	opts.Sparse, _ = cmd.Flags().GetBool("sparse")
	opts.Disable, _ = cmd.Flags().GetBool("disable")
	return opts
}

func runZFSStorageCreate(cmd *cobra.Command, args []string) error {
	opts := zfsStorageOptsFromFlags(cmd)
	opts.Pool, _ = cmd.Flags().GetString("pool")
	if opts.Storage == "" {
		return fmt.Errorf("--storage must be specified")
	}
	if opts.Pool == "" {
		return fmt.Errorf("--pool must be specified")
	}

	fmt.Printf("Create zfspool storage %q backed by %q\n", opts.Storage, opts.Pool)
	if dryRun {
		printDryRunHeader()
		printDryRunAction("zfs storage create", opts.Storage, "", "pool "+opts.Pool)
		printDryRunSummary("zfs storage create", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Create storage %q?", opts.Storage)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	ops := storage.NewOperations(client, logger)
	if err := ops.CreateZFSStorage(opts); err != nil {
		return err
	}
	fmt.Printf("✅ Storage %q created\n", opts.Storage)
	return nil
}

func runZFSStorageUpdate(cmd *cobra.Command, args []string) error {
	opts := zfsStorageOptsFromFlags(cmd)
	if opts.Storage == "" {
		return fmt.Errorf("--storage must be specified")
	}
	deleteKeys, _ := cmd.Flags().GetStringSlice("delete")

	ops := storage.NewOperations(client, logger)

	// Carry the digest so a concurrent edit is rejected rather than silently
	// overwritten.
	if current, err := ops.GetStorageConfig(opts.Storage); err == nil {
		if d, ok := current["digest"].(string); ok {
			opts.Digest = d
		}
	}

	fmt.Printf("Update storage %q\n", opts.Storage)
	if dryRun {
		printDryRunHeader()
		printDryRunAction("zfs storage update", opts.Storage, "", strings.Join(deleteKeys, ","))
		printDryRunSummary("zfs storage update", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Update storage %q?", opts.Storage)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := ops.UpdateZFSStorage(opts, deleteKeys); err != nil {
		return err
	}
	fmt.Printf("✅ Storage %q updated\n", opts.Storage)
	return nil
}

func runZFSStorageDelete(cmd *cobra.Command, args []string) error {
	storageID, _ := cmd.Flags().GetString("storage")
	if storageID == "" {
		return fmt.Errorf("--storage must be specified")
	}

	fmt.Printf("Remove storage entry %q from the cluster config.\n", storageID)
	fmt.Println("  The ZFS pool and all its data are kept.")
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("zfs storage delete", storageID, "", "config entry only")
		printDryRunSummary("zfs storage delete", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Remove storage entry %q?", storageID)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	ops := storage.NewOperations(client, logger)
	if err := ops.DeleteStorage(storageID); err != nil {
		return err
	}
	fmt.Printf("✅ Storage %q removed from the config\n", storageID)
	return nil
}

// str renders an API field as a string for table output.
func str(v interface{}) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%v", v)
}

func strOr(v interface{}, fallback string) string {
	if v == nil || fmt.Sprintf("%v", v) == "" {
		return fallback
	}
	return fmt.Sprintf("%v", v)
}

func isTruthy(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t == "1" || strings.EqualFold(t, "true")
	default:
		return false
	}
}
