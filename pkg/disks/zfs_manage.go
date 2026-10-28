package disks

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// ZFS RAID levels the create endpoint accepts, with the minimum device count
// Proxmox enforces for each. Validating locally turns an opaque API rejection
// into a message naming the shortfall, before any disk is touched.
var zfsRaidLevels = map[string]int{
	"single": 1,
	"mirror": 2,
	"raid10": 4,
	"raidz":  3,
	"raidz2": 4,
	"raidz3": 5,
	"draid":  3,
	"draid2": 4,
	"draid3": 5,
}

// zfsCompressionAlgos are the values the create endpoint accepts.
var zfsCompressionAlgos = []string{"on", "off", "gzip", "lz4", "lzjb", "zle", "zstd"}

// CreatePoolOptions holds the parameters for creating a ZFS pool.
type CreatePoolOptions struct {
	// Name is the pool name. Required.
	Name string
	// Devices are the block devices to build the pool from. Required.
	Devices []string
	// RaidLevel is one of zfsRaidLevels. Required.
	RaidLevel string
	// Ashift is the pool's sector-size exponent, 9-16. Proxmox defaults to 12
	// (4 KiB), which is right for essentially all modern disks. It cannot be
	// changed after creation.
	Ashift int
	// Compression defaults to "on" (which means lz4 in current ZFS).
	Compression string
	// AddStorage also registers the pool as a PVE storage. This needs
	// Datastore.Allocate on /storage in addition to Sys.Modify on /.
	AddStorage bool
	// DraidData is the number of data devices per redundancy group (dRAID only).
	DraidData int
	// DraidSpares is the number of distributed spares (dRAID only).
	DraidSpares int
}

// DeletePoolOptions holds the parameters for destroying a ZFS pool.
type DeletePoolOptions struct {
	// CleanupConfig removes or marks unavailable the PVE storage entries that
	// reference this pool. Needs Datastore.Allocate on /storage.
	CleanupConfig bool
	// CleanupDisks wipes the member disks so they can be reused. Without it the
	// disks keep their ZFS labels and cannot be re-added to a new pool.
	CleanupDisks bool
}

// modifyPermHint annotates a 403 from a ZFS write endpoint.
//
// This is deliberately separate from permHint: the read endpoints need
// Sys.Audit (PVEAuditor), while create and delete need Sys.Modify, which
// PVEAuditor and even PVEAdmin do NOT grant. Reusing the read hint would send
// an operator to grant a role that cannot fix the failure.
func modifyPermHint(action string, err error) error {
	var apiErr *api.ProxmoxAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf(
			"insufficient permission to %s: this requires Sys.Modify on / "+
				"(and Datastore.Allocate on /storage when touching storage config).\n"+
				"  Neither PVEAuditor nor PVEAdmin includes Sys.Modify — use PVESysAdmin or Administrator:\n"+
				"    pveum aclmod / -user '<user>@<realm>' -role PVESysAdmin",
			action)
	}
	return err
}

// ValidateCreatePool checks a pool request against the API's own rules before
// sending it, so a bad request fails with an explanation rather than a 400.
func ValidateCreatePool(opts *CreatePoolOptions) error {
	if opts == nil {
		return fmt.Errorf("create requires options")
	}
	if opts.Name == "" {
		return fmt.Errorf("pool name is required")
	}
	if len(opts.Devices) == 0 {
		return fmt.Errorf("at least one device is required, e.g. --devices /dev/sdb,/dev/sdc")
	}

	minDisks, ok := zfsRaidLevels[opts.RaidLevel]
	if !ok {
		return fmt.Errorf("unknown raid level %q; valid levels are: %s",
			opts.RaidLevel, strings.Join(sortedRaidLevels(), ", "))
	}
	if len(opts.Devices) < minDisks {
		return fmt.Errorf("raid level %s needs at least %d device(s), got %d",
			opts.RaidLevel, minDisks, len(opts.Devices))
	}
	// raid10 stripes mirrors, so the device count must be even.
	if opts.RaidLevel == "raid10" && len(opts.Devices)%2 != 0 {
		return fmt.Errorf("raid10 needs an even number of devices, got %d", len(opts.Devices))
	}
	if opts.RaidLevel == "single" && len(opts.Devices) != 1 {
		return fmt.Errorf("raid level single takes exactly one device, got %d", len(opts.Devices))
	}

	if opts.Ashift != 0 && (opts.Ashift < 9 || opts.Ashift > 16) {
		return fmt.Errorf("ashift must be between 9 and 16, got %d", opts.Ashift)
	}

	if opts.Compression != "" {
		valid := false
		for _, a := range zfsCompressionAlgos {
			if opts.Compression == a {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown compression %q; valid values are: %s",
				opts.Compression, strings.Join(zfsCompressionAlgos, ", "))
		}
	}

	isDraid := strings.HasPrefix(opts.RaidLevel, "draid")
	if !isDraid && (opts.DraidData > 0 || opts.DraidSpares > 0) {
		return fmt.Errorf("--draid-data and --draid-spares apply only to a draid raid level, not %s",
			opts.RaidLevel)
	}
	if opts.DraidData < 0 || opts.DraidSpares < 0 {
		return fmt.Errorf("draid data and spares counts cannot be negative")
	}

	return nil
}

// CreatePool creates a ZFS pool and returns the UPID of the worker task.
//
// This is destructive to the named devices: their existing contents are lost.
// API: POST /nodes/{node}/disks/zfs
func (ops *Operations) CreatePool(node string, opts *CreatePoolOptions) (string, error) {
	if node == "" {
		return "", fmt.Errorf("a node is required")
	}
	if err := ValidateCreatePool(opts); err != nil {
		return "", err
	}

	data := url.Values{}
	data.Set("name", opts.Name)
	data.Set("devices", strings.Join(opts.Devices, ","))
	data.Set("raidlevel", opts.RaidLevel)
	if opts.Ashift > 0 {
		data.Set("ashift", fmt.Sprintf("%d", opts.Ashift))
	}
	if opts.Compression != "" {
		data.Set("compression", opts.Compression)
	}
	if opts.AddStorage {
		data.Set("add_storage", "1")
	}
	// draid-config is a property string, not separate parameters.
	if parts := draidConfig(opts); parts != "" {
		data.Set("draid-config", parts)
	}

	path := fmt.Sprintf("/nodes/%s/disks/zfs", node)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", modifyPermHint(fmt.Sprintf("create ZFS pool %q on %s", opts.Name, node), err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Create of ZFS pool %s on %s initiated, task: %s", opts.Name, node, upid)
	return upid, nil
}

// DeletePool destroys a ZFS pool and returns the UPID of the worker task.
// API: DELETE /nodes/{node}/disks/zfs/{name}
func (ops *Operations) DeletePool(node, name string, opts *DeletePoolOptions) (string, error) {
	if node == "" || name == "" {
		return "", fmt.Errorf("both a node and a pool name are required")
	}

	path := fmt.Sprintf("/nodes/%s/disks/zfs/%s", node, name)
	if opts != nil {
		params := url.Values{}
		if opts.CleanupConfig {
			params.Set("cleanup-config", "1")
		}
		if opts.CleanupDisks {
			params.Set("cleanup-disks", "1")
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	resp, err := ops.client.Delete(path)
	if err != nil {
		return "", modifyPermHint(fmt.Sprintf("destroy ZFS pool %q on %s", name, node), err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Destroy of ZFS pool %s on %s initiated, task: %s", name, node, upid)
	return upid, nil
}

// draidConfig renders the draid-config property string, or "" when the options
// carry no dRAID settings.
func draidConfig(opts *CreatePoolOptions) string {
	var parts []string
	if opts.DraidData > 0 {
		parts = append(parts, fmt.Sprintf("data=%d", opts.DraidData))
	}
	if opts.DraidSpares > 0 {
		parts = append(parts, fmt.Sprintf("spares=%d", opts.DraidSpares))
	}
	return strings.Join(parts, ",")
}

// sortedRaidLevels lists the valid raid levels in a stable, sensible order for
// error messages — grouped by kind rather than alphabetically.
func sortedRaidLevels() []string {
	return []string{"single", "mirror", "raid10", "raidz", "raidz2", "raidz3", "draid", "draid2", "draid3"}
}

// RaidLevelMinDisks returns the minimum device count for a raid level, and
// whether the level is known.
func RaidLevelMinDisks(level string) (int, bool) {
	n, ok := zfsRaidLevels[level]
	return n, ok
}
