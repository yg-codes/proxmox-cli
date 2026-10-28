package vm

import (
	"fmt"
	"net/url"
	"strings"
)

// CreateOptions holds the parameters for creating a VM.
//
// The QEMU create endpoint accepts on the order of a hundred parameters. This
// covers the practical subset plus an Extra escape hatch, rather than mirroring
// every one — a flag nobody sets is still a flag everyone has to read past.
type CreateOptions struct {
	// VMID is the new VM's ID. Required.
	VMID string
	// Node is the node to create it on. Required.
	Node string

	Name        string
	Description string
	Cores       int
	Sockets     int
	Memory      int
	OSType      string
	Machine     string
	BIOS        string
	CPU         string
	SCSIHW      string
	Boot        string
	Agent       string
	Pool        string
	// Start powers the VM on once created.
	Start bool

	// Disks, network devices and drives, as raw device specs keyed by device
	// name — "scsi0" => "local-zfs:32", "net0" => "virtio,bridge=vmbr0".
	// These are passed through verbatim because their syntax is Proxmox's, and
	// re-inventing it behind typed flags would only add a translation layer to
	// get wrong.
	Devices map[string]string

	// Extra carries any additional API parameter not covered above.
	Extra map[string]string
}

// DestroyOptions holds the parameters for destroying a VM.
type DestroyOptions struct {
	// Purge removes the VMID from backup jobs, HA resources and replication
	// config as part of the destroy.
	//
	// This is not merely tidiness: without it, Proxmox REFUSES to destroy a VM
	// that is HA-managed or has replication jobs ("unable to remove VM N - used
	// in HA resources and purge parameter not set"). Verified against
	// PVE/API2/Qemu.pm destroy_vm. The CLI therefore defaults it on.
	Purge bool
	// DestroyUnreferencedDisks removes disks on the VM's storages that belong
	// to this VMID but are not referenced by its config.
	DestroyUnreferencedDisks bool
	// SkipLock bypasses the config lock. Proxmox restricts it to root@pam
	// (raise_param_exc "Only root may use this option"), and it does NOT skip
	// the running-VM or protection checks — only the lock. Not exposed as a CLI
	// flag: a lock almost always means another task is mid-flight.
	SkipLock bool
}

// CreateVM creates a new VM and returns the UPID of the create task.
// API: POST /nodes/{node}/qemu
func (ops *Operations) CreateVM(opts *CreateOptions) (string, error) {
	if opts == nil {
		return "", fmt.Errorf("create requires options")
	}
	if opts.VMID == "" {
		return "", fmt.Errorf("create requires a VMID")
	}
	if opts.Node == "" {
		return "", fmt.Errorf("create requires a target node")
	}

	data := url.Values{}
	data.Set("vmid", opts.VMID)

	setIfNotEmpty(data, "name", opts.Name)
	setIfNotEmpty(data, "description", opts.Description)
	setIfNotEmpty(data, "ostype", opts.OSType)
	setIfNotEmpty(data, "machine", opts.Machine)
	setIfNotEmpty(data, "bios", opts.BIOS)
	setIfNotEmpty(data, "cpu", opts.CPU)
	setIfNotEmpty(data, "scsihw", opts.SCSIHW)
	setIfNotEmpty(data, "boot", opts.Boot)
	setIfNotEmpty(data, "agent", opts.Agent)
	setIfNotEmpty(data, "pool", opts.Pool)

	if opts.Cores > 0 {
		data.Set("cores", fmt.Sprintf("%d", opts.Cores))
	}
	if opts.Sockets > 0 {
		data.Set("sockets", fmt.Sprintf("%d", opts.Sockets))
	}
	if opts.Memory > 0 {
		data.Set("memory", fmt.Sprintf("%d", opts.Memory))
	}
	if opts.Start {
		data.Set("start", "1")
	}

	// Devices first, then Extra: an explicit --raw wins over a device flag for
	// the same key, so the escape hatch can always override.
	for k, v := range opts.Devices {
		data.Set(k, v)
	}
	for k, v := range opts.Extra {
		data.Set(k, v)
	}

	resp, err := ops.client.Post(fmt.Sprintf("/nodes/%s/qemu", opts.Node), data)
	if err != nil {
		return "", fmt.Errorf("failed to create VM %s: %w", opts.VMID, err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Create of VM %s on %s initiated, task: %s", opts.VMID, opts.Node, upid)
	return upid, nil
}

// DestroyVM destroys a VM and its disks, returning the UPID of the task.
//
// This is irreversible: the disks go with the VM and no snapshot survives it.
// Proxmox itself refuses when protection is set or the VM is running; the CLI
// checks both first so the operator gets a clear message instead of an API
// error, but those server-side refusals are the real backstop.
//
// API: DELETE /nodes/{node}/qemu/{vmid}
func (ops *Operations) DestroyVM(vmid string, opts *DestroyOptions) (string, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return "", err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s", node, vmid)

	if opts != nil {
		params := url.Values{}
		if opts.Purge {
			params.Set("purge", "1")
		}
		if opts.DestroyUnreferencedDisks {
			params.Set("destroy-unreferenced-disks", "1")
		}
		if opts.SkipLock {
			params.Set("skiplock", "1")
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	resp, err := ops.client.Delete(path)
	if err != nil {
		return "", fmt.Errorf("failed to destroy VM %s: %w", vmid, err)
	}

	upid, _ := resp["data"].(string)
	ops.logger.Infof("Destroy of VM %s on %s initiated, task: %s", vmid, node, upid)
	return upid, nil
}

// IsProtected reports whether a VM has protection mode enabled.
//
// A protected VM cannot be destroyed, and the API's refusal arrives only after
// the operator has already confirmed. Checking first turns that into an
// explanation. Proxmox returns this field as a number, a bool or a string
// depending on how it was set, so all three are handled.
func (ops *Operations) IsProtected(vmid string) (bool, error) {
	config, err := ops.GetVMConfig(vmid)
	if err != nil {
		return false, err
	}
	switch v := config["protection"].(type) {
	case float64:
		return v != 0, nil
	case bool:
		return v, nil
	case string:
		return v == "1" || strings.EqualFold(v, "true"), nil
	default:
		return false, nil
	}
}

// NextVMID returns the next free VMID the cluster suggests.
// API: GET /cluster/nextid
func (ops *Operations) NextVMID() (string, error) {
	resp, err := ops.client.Get("/cluster/nextid", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get next free VMID: %w", err)
	}
	switch v := resp["data"].(type) {
	case string:
		return v, nil
	case float64:
		return fmt.Sprintf("%.0f", v), nil
	default:
		return "", fmt.Errorf("unexpected nextid response type %T", resp["data"])
	}
}

// VMIDExists reports whether a VMID is already in use in the cluster.
//
// create fails on a duplicate anyway, but the API error is opaque; and for
// destroy this is how "already gone" is distinguished from "wrong ID".
//
// Only FindVMNode's own "not found" verdict is treated as absence. Any other
// error (unreachable cluster, auth failure) is returned as an error rather than
// reported as "does not exist" — a false "absent" would let create collide with
// a live VM, so this must not guess.
func (ops *Operations) VMIDExists(vmid string) (bool, error) {
	if _, err := ops.FindVMNode(vmid); err != nil {
		if strings.Contains(err.Error(), "not found on any node") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
