package vm

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// CloneOptions holds the optional parameters for cloning a VM.
type CloneOptions struct {
	// NewID is the VMID of the clone. Required.
	NewID string
	// Name is the hostname for the new VM.
	Name string
	// Description for the new VM.
	Description string
	// Pool adds the new VM to a resource pool.
	Pool string
	// SnapName clones from a snapshot rather than the current state.
	SnapName string
	// Storage is the target storage. Only used for a full clone.
	Storage string
	// Format is the target disk format (raw, qcow2, vmdk). Only used when
	// moving to a different storage.
	Format string
	// Target is the target node. Only allowed if the VM is on shared storage.
	Target string
	// Full forces a full copy of all disks. A normal VM is always fully
	// cloned; for a template Proxmox tries a linked clone unless this is set.
	Full bool
	// BWLimit overrides the I/O bandwidth limit, in KiB/s.
	BWLimit int
}

// MigrateOptions holds the optional parameters for migrating a VM.
type MigrateOptions struct {
	// Target is the destination node. Required.
	Target string
	// Online performs a live migration for a running VM.
	Online bool
	// WithLocalDisks enables live storage migration for local disks.
	WithLocalDisks bool
	// TargetStorage maps disks onto storage on the target node.
	TargetStorage string
	// MigrationType is "secure" or "insecure".
	MigrationType string
	// MigrationNetwork is the CIDR of the network used for migration.
	MigrationNetwork string
	// BWLimit overrides the I/O bandwidth limit, in KiB/s.
	BWLimit int
	// Force allows migrating VMs with local devices. Root only.
	Force bool
}

// MoveDiskOptions holds the optional parameters for moving a VM disk.
type MoveDiskOptions struct {
	// Disk is the drive to move, e.g. "scsi0". Required.
	Disk string
	// Storage is the target storage. Mutually exclusive with TargetVMID.
	Storage string
	// TargetVMID reassigns the disk to another VM instead of moving storage.
	TargetVMID string
	// TargetDisk is the drive name on the target VM. Used with TargetVMID.
	TargetDisk string
	// Format is the target disk format (raw, qcow2, vmdk).
	Format string
	// Delete removes the original disk after a successful copy.
	Delete bool
	// BWLimit overrides the I/O bandwidth limit, in KiB/s.
	BWLimit int
	// Digest guards against a concurrent config modification.
	Digest string
}

// MigratePrecondition describes whether and how a VM can be migrated.
type MigratePrecondition struct {
	// Running reports whether the VM is currently running, which decides
	// whether the migration must be online.
	Running bool
	// AllowedNodes lists nodes the VM may migrate to. It is only populated
	// for VMs on local storage.
	AllowedNodes []string
	// NotAllowedNodes maps a node to the reasons it is unavailable.
	NotAllowedNodes map[string][]string
	// LocalDisks lists disks on local storage. These block a plain migration
	// unless WithLocalDisks is set.
	LocalDisks []LocalDisk
	// LocalResources lists passed-through devices that block migration.
	LocalResources []string
}

// LocalDisk is a VM disk residing on node-local storage.
type LocalDisk struct {
	DriveName string
	Size      int64
	Shared    bool
	CDROM     bool
}

// CloneVM creates a copy of a VM or template and returns the task UPID.
func (ops *Operations) CloneVM(vmid string, opts *CloneOptions) (string, error) {
	if opts == nil || opts.NewID == "" {
		return "", fmt.Errorf("clone requires a new VMID")
	}

	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return "", err
	}

	data := url.Values{}
	data.Set("newid", opts.NewID)
	setIfNotEmpty(data, "name", opts.Name)
	setIfNotEmpty(data, "description", opts.Description)
	setIfNotEmpty(data, "pool", opts.Pool)
	setIfNotEmpty(data, "snapname", opts.SnapName)
	setIfNotEmpty(data, "storage", opts.Storage)
	setIfNotEmpty(data, "format", opts.Format)
	setIfNotEmpty(data, "target", opts.Target)
	if opts.Full {
		data.Set("full", "1")
	}
	if opts.BWLimit > 0 {
		data.Set("bwlimit", strconv.Itoa(opts.BWLimit))
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/clone", node, vmid)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to clone VM %s: %w", vmid, err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Clone VM %s to %s initiated, task: %s", vmid, opts.NewID, upid)
	return upid, nil
}

// GetMigratePrecondition reports whether a VM can migrate and what would
// block it. Call this before MigrateVM: a migration that fails partway is far
// more costly than one that is never started.
func (ops *Operations) GetMigratePrecondition(vmid string) (*MigratePrecondition, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/migrate", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get migration preconditions for VM %s: %w", vmid, err)
	}

	pre := &MigratePrecondition{
		NotAllowedNodes: make(map[string][]string),
	}

	if running, ok := resp["running"].(float64); ok {
		pre.Running = running != 0
	}

	if nodes, ok := resp["allowed_nodes"].([]interface{}); ok {
		for _, n := range nodes {
			if name, ok := n.(string); ok {
				pre.AllowedNodes = append(pre.AllowedNodes, name)
			}
		}
	}

	// not_allowed_nodes maps a node name to the reasons it is blocked. An
	// empty reason set means the node is not actually blocked, so only record
	// nodes that carry a real reason.
	if blocked, ok := resp["not_allowed_nodes"].(map[string]interface{}); ok {
		for name, detail := range blocked {
			reasons := extractBlockReasons(detail)
			if len(reasons) > 0 {
				pre.NotAllowedNodes[name] = reasons
			}
		}
	}

	if disks, ok := resp["local_disks"].([]interface{}); ok {
		for _, d := range disks {
			entry, ok := d.(map[string]interface{})
			if !ok {
				continue
			}
			disk := LocalDisk{}
			if name, ok := entry["drivename"].(string); ok {
				disk.DriveName = name
			}
			if size, ok := entry["size"].(float64); ok {
				disk.Size = int64(size)
			}
			if shared, ok := entry["shared"].(float64); ok {
				disk.Shared = shared != 0
			}
			if cdrom, ok := entry["cdrom"].(float64); ok {
				disk.CDROM = cdrom != 0
			}
			pre.LocalDisks = append(pre.LocalDisks, disk)
		}
	}

	if resources, ok := resp["local_resources"].([]interface{}); ok {
		for _, r := range resources {
			if name, ok := r.(string); ok {
				pre.LocalResources = append(pre.LocalResources, name)
			}
		}
	}

	return pre, nil
}

// MigrateVM migrates a VM to another node and returns the task UPID.
func (ops *Operations) MigrateVM(vmid string, opts *MigrateOptions) (string, error) {
	if opts == nil || opts.Target == "" {
		return "", fmt.Errorf("migration requires a target node")
	}

	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return "", err
	}

	if node == opts.Target {
		return "", fmt.Errorf("VM %s is already on node %s", vmid, opts.Target)
	}

	data := url.Values{}
	data.Set("target", opts.Target)
	setIfNotEmpty(data, "targetstorage", opts.TargetStorage)
	setIfNotEmpty(data, "migration_type", opts.MigrationType)
	setIfNotEmpty(data, "migration_network", opts.MigrationNetwork)
	if opts.Online {
		data.Set("online", "1")
	}
	if opts.WithLocalDisks {
		data.Set("with-local-disks", "1")
	}
	if opts.Force {
		data.Set("force", "1")
	}
	if opts.BWLimit > 0 {
		data.Set("bwlimit", strconv.Itoa(opts.BWLimit))
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/migrate", node, vmid)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to migrate VM %s to %s: %w", vmid, opts.Target, err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Migrate VM %s from %s to %s initiated, task: %s", vmid, node, opts.Target, upid)
	return upid, nil
}

// ResizeVMDisk extends a VM disk and returns the task UPID.
//
// size is either an absolute target ("32G") or an increment ("+4G"). The
// Proxmox API only extends volumes: it has no shrink operation, and a size
// smaller than the current one is rejected rather than silently truncating
// the disk.
func (ops *Operations) ResizeVMDisk(vmid, disk, size, digest string) (string, error) {
	if disk == "" {
		return "", fmt.Errorf("resize requires a disk")
	}
	if err := ValidateResizeSize(size); err != nil {
		return "", err
	}

	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return "", err
	}

	data := url.Values{}
	data.Set("disk", disk)
	data.Set("size", size)
	setIfNotEmpty(data, "digest", digest)

	path := fmt.Sprintf("/nodes/%s/qemu/%s/resize", node, vmid)
	resp, err := ops.client.Put(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to resize disk %s on VM %s: %w", disk, vmid, err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Resize disk %s on VM %s to %s initiated, task: %s", disk, vmid, size, upid)
	return upid, nil
}

// MoveVMDisk moves a disk to a different storage, or reassigns it to another
// VM, and returns the task UPID.
func (ops *Operations) MoveVMDisk(vmid string, opts *MoveDiskOptions) (string, error) {
	if opts == nil || opts.Disk == "" {
		return "", fmt.Errorf("move requires a disk")
	}
	if opts.Storage == "" && opts.TargetVMID == "" {
		return "", fmt.Errorf("move requires either a target storage or a target VM")
	}
	if opts.Storage != "" && opts.TargetVMID != "" {
		return "", fmt.Errorf("move accepts a target storage or a target VM, not both")
	}

	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return "", err
	}

	data := url.Values{}
	data.Set("disk", opts.Disk)
	setIfNotEmpty(data, "storage", opts.Storage)
	setIfNotEmpty(data, "target-vmid", opts.TargetVMID)
	setIfNotEmpty(data, "target-disk", opts.TargetDisk)
	setIfNotEmpty(data, "format", opts.Format)
	setIfNotEmpty(data, "digest", opts.Digest)
	if opts.Delete {
		data.Set("delete", "1")
	}
	if opts.BWLimit > 0 {
		data.Set("bwlimit", strconv.Itoa(opts.BWLimit))
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/move_disk", node, vmid)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to move disk %s on VM %s: %w", opts.Disk, vmid, err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Move disk %s on VM %s initiated, task: %s", opts.Disk, vmid, upid)
	return upid, nil
}

// ConvertToTemplate converts a VM into a template.
//
// This is one-way: Proxmox has no API to convert a template back into a
// regular VM, and a template cannot be started.
func (ops *Operations) ConvertToTemplate(vmid, disk string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	setIfNotEmpty(data, "disk", disk)

	path := fmt.Sprintf("/nodes/%s/qemu/%s/template", node, vmid)
	if _, err := ops.client.Post(path, data); err != nil {
		return fmt.Errorf("failed to convert VM %s to a template: %w", vmid, err)
	}

	ops.logger.Infof("Converted VM %s to a template", vmid)
	return nil
}

// ValidateResizeSize checks a resize size against the shape the API accepts:
// an optional "+" for an increment, digits, and an optional K/M/G/T suffix.
//
// This exists to turn the two mistakes users actually make — asking for a
// shrink, or omitting the unit — into a clear message instead of a bare API
// error. The API pattern is '\+?\d+(\.\d+)?[KMGT]?', which has no negative
// form: volumes can only be extended.
func ValidateResizeSize(size string) error {
	if size == "" {
		return fmt.Errorf("resize requires a size, e.g. 32G or +4G")
	}

	if strings.HasPrefix(size, "-") {
		return fmt.Errorf("cannot shrink a disk: Proxmox only supports extending volumes, so %q is not a valid size", size)
	}

	body := strings.TrimPrefix(size, "+")
	if body == "" {
		return fmt.Errorf("invalid size %q: expected a number, e.g. 32G or +4G", size)
	}

	// A trailing unit is optional; without one the API interprets the value as
	// bytes, which is rarely what a user means.
	if unit := body[len(body)-1]; unit >= 'A' && unit <= 'Z' {
		if !strings.ContainsRune("KMGT", rune(unit)) {
			return fmt.Errorf("invalid size unit %q in %q: expected K, M, G or T", string(unit), size)
		}
		body = body[:len(body)-1]
	}

	if body == "" {
		return fmt.Errorf("invalid size %q: expected a number before the unit, e.g. 32G", size)
	}

	if _, err := strconv.ParseFloat(body, 64); err != nil {
		return fmt.Errorf("invalid size %q: expected a number with an optional K/M/G/T suffix", size)
	}

	return nil
}

// extractBlockReasons renders the per-node detail from not_allowed_nodes into
// human-readable reasons.
func extractBlockReasons(detail interface{}) []string {
	entry, ok := detail.(map[string]interface{})
	if !ok {
		return nil
	}

	var reasons []string

	if storages, ok := entry["unavailable_storages"].([]interface{}); ok && len(storages) > 0 {
		names := make([]string, 0, len(storages))
		for _, s := range storages {
			if name, ok := s.(string); ok {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			reasons = append(reasons, "unavailable storage: "+strings.Join(names, ", "))
		}
	}

	if resources, ok := entry["blocking_ha_resources"].([]interface{}); ok && len(resources) > 0 {
		for _, r := range resources {
			res, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := res["sid"].(string)
			cause, _ := res["cause"].(string)
			switch {
			case id != "" && cause != "":
				reasons = append(reasons, fmt.Sprintf("HA resource %s (%s)", id, cause))
			case id != "":
				reasons = append(reasons, "HA resource "+id)
			}
		}
	}

	return reasons
}

// setIfNotEmpty adds a form value only when it carries a value, so optional
// parameters are omitted rather than sent empty.
func setIfNotEmpty(data url.Values, key, value string) {
	if value != "" {
		data.Set(key, value)
	}
}

// ParseDiskSizeBytes extracts the size of a disk from its config value.
//
// A Proxmox disk spec looks like
//
//	local-ceph:vm-4001-disk-1,iothread=1,size=10G,ssd=1
//
// so the size is one comma-separated key among several and may carry a K/M/G/T
// suffix. Returns 0 with no error when no size key is present — some entries
// (cdrom, cloudinit) legitimately have none, and treating that as an error
// would block operations that are actually fine.
func ParseDiskSizeBytes(spec string) (int64, error) {
	for _, part := range strings.Split(spec, ",") {
		value, ok := strings.CutPrefix(strings.TrimSpace(part), "size=")
		if !ok {
			continue
		}
		if value == "" {
			return 0, fmt.Errorf("disk spec has an empty size= value")
		}

		multiplier := int64(1)
		switch unit := value[len(value)-1]; unit {
		case 'K', 'k':
			multiplier = 1 << 10
		case 'M', 'm':
			multiplier = 1 << 20
		case 'G', 'g':
			multiplier = 1 << 30
		case 'T', 't':
			multiplier = 1 << 40
		default:
			if unit < '0' || unit > '9' {
				return 0, fmt.Errorf("unrecognised size unit %q in %q", string(unit), value)
			}
		}
		if multiplier > 1 {
			value = value[:len(value)-1]
		}

		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse disk size %q: %w", value, err)
		}
		if n < 0 {
			return 0, fmt.Errorf("disk size %q is negative", value)
		}
		return int64(n * float64(multiplier)), nil
	}
	return 0, nil // no size key — not an error
}
