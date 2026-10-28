package vm

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// CloudInitPending is one cloudinit setting that differs between the VM's
// current config and what the cloudinit drive was generated from.
//
// The API reports drift as key/value pairs. A non-empty list means the guest
// would receive different settings on its next cloudinit run than it has now —
// regenerating the drive is what commits them.
type CloudInitPending struct {
	Key   string
	Value string
}

// cloudInitDumpTypes are the sections the dump endpoint can render. Verified
// against a live cluster: any other value is rejected with an enum error.
var cloudInitDumpTypes = []string{"user", "network", "meta"}

// GetCloudInitPending returns the cloudinit settings staged but not yet applied.
// API: GET /nodes/{node}/qemu/{vmid}/cloudinit
func (ops *Operations) GetCloudInitPending(vmid string) ([]*CloudInitPending, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	resp, err := ops.client.Get(fmt.Sprintf("/nodes/%s/qemu/%s/cloudinit", node, vmid), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read cloudinit state for VM %s: %w", vmid, err)
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}

	out := make([]*CloudInitPending, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		entry := &CloudInitPending{}
		entry.Key, _ = m["key"].(string)
		if v, ok := m["value"].(string); ok {
			// The API returns these percent-encoded (an ssh key comes back with
			// %20 for every space). Decoding makes the output usable; on a
			// decode failure the raw value is kept rather than dropped.
			if decoded, err := url.QueryUnescape(v); err == nil {
				entry.Value = decoded
			} else {
				entry.Value = v
			}
		}
		if entry.Key != "" {
			out = append(out, entry)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// RegenerateCloudInit rebuilds the VM's cloudinit drive from its current config.
//
// The guest only sees changed cloudinit settings after this AND a reboot: the
// drive is read at boot. Regenerating alone changes nothing inside a running
// guest, which is the detail most easily missed.
//
// API: PUT /nodes/{node}/qemu/{vmid}/cloudinit
func (ops *Operations) RegenerateCloudInit(vmid string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/cloudinit", node, vmid)
	if _, err := ops.client.Put(path, url.Values{}); err != nil {
		return fmt.Errorf("failed to regenerate the cloudinit drive for VM %s: %w", vmid, err)
	}
	ops.logger.Infof("Regenerated the cloudinit drive for VM %s", vmid)
	return nil
}

// DumpCloudInit returns the rendered cloudinit configuration the guest will
// read. dumpType must be one of user, network or meta.
//
// This is the generated output, not the VM config that produced it — the useful
// thing to check when a guest is not picking up an expected setting.
//
// API: GET /nodes/{node}/qemu/{vmid}/cloudinit/dump?type=...
func (ops *Operations) DumpCloudInit(vmid, dumpType string) (string, error) {
	if err := ValidateCloudInitDumpType(dumpType); err != nil {
		return "", err
	}

	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return "", err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/cloudinit/dump?type=%s",
		node, vmid, url.QueryEscape(dumpType))
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to dump %s cloudinit data for VM %s: %w", dumpType, vmid, err)
	}

	if data, ok := resp["data"].(string); ok {
		return data, nil
	}
	return "", fmt.Errorf("unexpected cloudinit dump response for VM %s", vmid)
}

// ValidateCloudInitDumpType checks the type against the API's enum, so a typo
// is named locally rather than returning an enum error from the server.
func ValidateCloudInitDumpType(dumpType string) error {
	if dumpType == "" {
		return fmt.Errorf("a dump type is required: one of %s", strings.Join(cloudInitDumpTypes, ", "))
	}
	for _, t := range cloudInitDumpTypes {
		if dumpType == t {
			return nil
		}
	}
	return fmt.Errorf("unknown cloudinit dump type %q; valid types are: %s",
		dumpType, strings.Join(cloudInitDumpTypes, ", "))
}

// CloudInitDumpTypes returns the valid dump types, for help text and shell
// completion.
func CloudInitDumpTypes() []string {
	out := make([]string, len(cloudInitDumpTypes))
	copy(out, cloudInitDumpTypes)
	return out
}

// CloudInitConfigKeys are the VM config keys that feed the cloudinit drive.
//
// These are set through the normal config endpoint (pve vm config set), NOT
// through the cloudinit endpoint — that one only reports drift and regenerates.
// Listing them lets the CLI show the current cloudinit configuration alongside
// the pending changes.
// This matches $cloudinitoptions in qemu-server's PVE/API2/Qemu.pm, which the
// source names as the canonical list, plus ipconfigN matched by prefix.
var CloudInitConfigKeys = []string{
	"cicustom", "cipassword", "citype", "ciupgrade", "ciuser",
	"nameserver", "searchdomain", "sshkeys",
}

// CloudInitTypes are the valid citype values. opennebula is easy to miss —
// the commonly-cited pair is nocloud (the Linux default) and configdrive2 (the
// Windows default).
var CloudInitTypes = []string{"configdrive2", "nocloud", "opennebula"}

// IsCloudInitKey reports whether a config key feeds cloudinit. ipconfigN is
// matched by prefix, since the index is unbounded.
func IsCloudInitKey(key string) bool {
	for _, k := range CloudInitConfigKeys {
		if key == k {
			return true
		}
	}
	return strings.HasPrefix(key, "ipconfig")
}
