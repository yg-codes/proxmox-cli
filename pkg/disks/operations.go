package disks

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Operations handles ZFS pool and physical-disk reporting operations.
// All operations are read-only (GET) and hit the /nodes/{node}/disks/* API,
// which requires Sys.Audit on /nodes.
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new disks operations handler.
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}
	return &Operations{client: client, logger: logger}
}

// permHint wraps a permission error (HTTP 403) from a /disks/* endpoint with a
// clear, actionable message about the required Sys.Audit ACL. Non-403 errors
// pass through unchanged.
func permHint(path string, err error) error {
	var apiErr *api.ProxmoxAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf(
			"insufficient permission for %s: this endpoint requires Sys.Audit on /nodes. "+
				"Grant it with: pveum aclmod /nodes -token '<user>@<realm>!<token>' -role PVEAuditor",
			path,
		)
	}
	return err
}

// getNodeNames returns the list of node names in the cluster.
func (ops *Operations) getNodeNames() ([]string, error) {
	resp, err := ops.client.Get("/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid nodes response")
	}
	names := make([]string, 0, len(items))
	for _, n := range items {
		if m, ok := n.(map[string]interface{}); ok {
			if name, ok := m["node"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// GetZFSPools returns ZFS pools across all cluster nodes (or a single node when
// nodeFilter is non-empty).
// API: GET /nodes/{node}/disks/zfs
func (ops *Operations) GetZFSPools(nodeFilter string) ([]*ZFSPool, error) {
	ops.logger.Debug("Fetching ZFS pools")

	nodes, err := ops.getNodeNames()
	if err != nil {
		return nil, err
	}

	var pools []*ZFSPool
	for _, nodeName := range nodes {
		if nodeFilter != "" && nodeName != nodeFilter {
			continue
		}

		path := fmt.Sprintf("/nodes/%s/disks/zfs", nodeName)
		resp, err := ops.client.Get(path, nil)
		if err != nil {
			return nil, permHint(path, err)
		}

		items, ok := resp["items"].([]interface{})
		if !ok {
			continue
		}
		for _, item := range items {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			pools = append(pools, parseZFSPool(nodeName, m))
		}
	}

	ops.logger.Infof("Found %d ZFS pools", len(pools))
	return pools, nil
}

// GetZFSPoolDetail returns the device tree for a single pool on a node.
// API: GET /nodes/{node}/disks/zfs/{name}
func (ops *Operations) GetZFSPoolDetail(nodeName, pool string) (*ZFSPoolDetail, error) {
	ops.logger.Debugf("Fetching ZFS pool detail for %s on %s", pool, nodeName)

	path := fmt.Sprintf("/nodes/%s/disks/zfs/%s", nodeName, pool)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, permHint(path, err)
	}

	detail := &ZFSPoolDetail{Node: nodeName, Name: pool}
	if s, ok := resp["state"].(string); ok {
		detail.State = s
	}
	if s, ok := resp["scan"].(string); ok {
		detail.Scan = s
	}
	if s, ok := resp["errors"].(string); ok {
		detail.Errors = s
	}
	if children, ok := resp["children"].([]interface{}); ok {
		detail.Root = &ZFSVdev{Name: pool, Type: "pool", State: detail.State}
		detail.Root.Children = parseVdevChildren(children, 1)
	}

	return detail, nil
}

// GetDisks returns the physical disks on a node.
// API: GET /nodes/{node}/disks/list
func (ops *Operations) GetDisks(nodeName string) ([]*Disk, error) {
	ops.logger.Debugf("Fetching disks for node %s", nodeName)

	path := fmt.Sprintf("/nodes/%s/disks/list", nodeName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, permHint(path, err)
	}

	var disks []*Disk
	items, ok := resp["items"].([]interface{})
	if !ok {
		return disks, nil
	}
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		disks = append(disks, parseDisk(nodeName, m))
	}

	ops.logger.Infof("Found %d disks on node %s", len(disks), nodeName)
	return disks, nil
}

// GetDiskSMART returns SMART health for a single disk device.
// API: GET /nodes/{node}/disks/smart?disk={dev}
func (ops *Operations) GetDiskSMART(nodeName, dev string) (*SMARTStatus, error) {
	ops.logger.Debugf("Fetching SMART for %s on %s", dev, nodeName)

	path := fmt.Sprintf("/nodes/%s/disks/smart", nodeName)
	resp, err := ops.client.Get(path, map[string]string{"disk": dev})
	if err != nil {
		return nil, permHint(path, err)
	}

	status := &SMARTStatus{Node: nodeName, DevPath: dev}
	if h, ok := resp["health"].(string); ok {
		status.Health = h
	}
	if t, ok := resp["type"].(string); ok {
		status.Type = t
	}
	if txt, ok := resp["text"].(string); ok {
		status.Text = txt
	}
	// Attribute table (ATA disks) arrives under "attributes".
	if attrs, ok := resp["attributes"].([]interface{}); ok {
		for _, a := range attrs {
			if m, ok := a.(map[string]interface{}); ok {
				status.Attributes = append(status.Attributes, parseSMARTAttribute(m))
			}
		}
	}

	return status, nil
}

// --- parse helpers ---

func toInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

func toFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func toString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func parseZFSPool(nodeName string, m map[string]interface{}) *ZFSPool {
	p := &ZFSPool{
		Node:   nodeName,
		Name:   toString(m, "name"),
		Size:   toInt64(m, "size"),
		Alloc:  toInt64(m, "alloc"),
		Free:   toInt64(m, "free"),
		Frag:   toFloat(m, "frag"),
		Dedup:  toFloat(m, "dedup"),
		Health: toString(m, "health"),
	}
	if p.Size > 0 {
		p.UsePercent = (float64(p.Alloc) / float64(p.Size)) * 100
	}
	return p
}

func parseVdevChildren(children []interface{}, level int) []*ZFSVdev {
	var out []*ZFSVdev
	for _, c := range children {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		v := &ZFSVdev{
			Name:  toString(m, "name"),
			Type:  toString(m, "type"),
			State: toString(m, "state"),
			Read:  toInt64(m, "read"),
			Write: toInt64(m, "write"),
			Cksum: toInt64(m, "cksum"),
			Msg:   toString(m, "msg"),
			Level: level,
		}
		if sub, ok := m["children"].([]interface{}); ok {
			v.Children = parseVdevChildren(sub, level+1)
		}
		out = append(out, v)
	}
	return out
}

func parseDisk(nodeName string, m map[string]interface{}) *Disk {
	d := &Disk{
		Node:    nodeName,
		DevPath: toString(m, "devpath"),
		Model:   toString(m, "model"),
		Serial:  toString(m, "serial"),
		Size:    toInt64(m, "size"),
		Type:    toString(m, "type"),
		Vendor:  toString(m, "vendor"),
		Health:  toString(m, "health"),
		Used:    toString(m, "used"),
		RPM:     toInt64(m, "rpm"),
		Wearout: -1,
	}
	// wearout: Proxmox returns "N/A" or a number.
	if w, ok := m["wearout"].(float64); ok {
		d.Wearout = int(w)
	}
	if g, ok := m["gpt"].(float64); ok {
		d.GPT = g == 1
	}
	if d.Health == "" {
		d.Health = "UNKNOWN"
	}
	return d
}

func parseSMARTAttribute(m map[string]interface{}) *SMARTAttribute {
	a := &SMARTAttribute{
		Name:    toString(m, "name"),
		Flags:   toString(m, "flags"),
		Failing: toString(m, "fail"),
		Raw:     toString(m, "raw"),
	}
	a.ID = int(toInt64(m, "id"))
	a.Value = int(toInt64(m, "value"))
	a.Worst = int(toInt64(m, "worst"))
	a.Threshold = int(toInt64(m, "threshold"))
	// Some fields come back as strings; normalize whitespace on the raw value.
	a.Raw = strings.TrimSpace(a.Raw)
	return a
}
